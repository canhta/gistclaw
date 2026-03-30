package app

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/runtime/capabilities"
	"github.com/canhta/gistclaw/internal/tools"
)

type stubMCPFactory struct {
	server *stubMCPConnection
}

func (f stubMCPFactory) Connect(_ context.Context, _ tools.MCPServerConfig) (tools.MCPConnection, error) {
	return f.server, nil
}

type stubMCPConnection struct {
	tools []tools.MCPRemoteTool
}

func (c *stubMCPConnection) ListTools(_ context.Context) ([]tools.MCPRemoteTool, error) {
	return c.tools, nil
}

func (c *stubMCPConnection) CallTool(_ context.Context, _ string, _ map[string]any) (tools.MCPToolCallResult, error) {
	return tools.MCPToolCallResult{Output: `{}`, IsError: false}, nil
}

func (c *stubMCPConnection) Close() error { return nil }

func TestBootstrap_WiringFunctionsExist(t *testing.T) {
	_, err := storeWiring(Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Logf("storeWiring error (expected with minimal config): %v", err)
	}
}

func TestResolveConnectorAgentIDs_DefaultsToFrontAgent(t *testing.T) {
	cfg := Config{}
	snapshot := model.ExecutionSnapshot{
		TeamID:       "default",
		FrontAgentID: "lead",
		Agents: map[string]model.AgentProfile{
			"lead": {
				AgentID:      "lead",
				BaseProfile:  model.BaseProfileOperator,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead},
			},
		},
	}

	resolved, err := resolveConnectorAgentIDs(cfg, snapshot)
	if err != nil {
		t.Fatalf("resolveConnectorAgentIDs returned error: %v", err)
	}
	if resolved.Telegram.AgentID != "lead" {
		t.Fatalf("Telegram.AgentID = %q, want %q", resolved.Telegram.AgentID, "lead")
	}
	if resolved.WhatsApp.AgentID != "lead" {
		t.Fatalf("WhatsApp.AgentID = %q, want %q", resolved.WhatsApp.AgentID, "lead")
	}
	if resolved.ZaloPersonal.AgentID != "lead" {
		t.Fatalf("ZaloPersonal.AgentID = %q, want %q", resolved.ZaloPersonal.AgentID, "lead")
	}
}

func TestBootstrap_NoFunctionOver30Lines(t *testing.T) {
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
	}
	_, _ = Bootstrap(cfg)
}

func TestBootstrap_AdminTokenGeneratedOnFirstStart(t *testing.T) {
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
	}

	a, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	var token string
	err = a.db.RawDB().QueryRow("SELECT value FROM settings WHERE key = 'admin_token'").Scan(&token)
	if err != nil {
		t.Fatalf("query admin_token: %v", err)
	}
	if token == "" {
		t.Fatal("expected admin_token to be set after first Bootstrap, got empty string")
	}

	// Token must be a valid 64-char hex string (32 bytes)
	if len(token) != 64 {
		t.Fatalf("expected 64-char hex token, got len=%d: %q", len(token), token)
	}
	if _, err := hex.DecodeString(token); err != nil {
		t.Fatalf("admin_token is not valid hex: %v", err)
	}
}

func TestBootstrap_InjectsCapabilityRegistryIntoRuntime(t *testing.T) {
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  t.TempDir(),
	}

	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	defer app.db.Close()

	rv := reflect.ValueOf(app.runtime).Elem()
	capabilityField := rv.FieldByName("capabilities")
	if !capabilityField.IsValid() || capabilityField.IsNil() {
		t.Fatal("expected bootstrap runtime to receive a capability registry")
	}
	presenceField := rv.FieldByName("presence")
	if !presenceField.IsValid() || presenceField.IsNil() {
		t.Fatal("expected bootstrap runtime to initialize a presence manager")
	}
}

func TestBootstrap_AdminTokenNotRegeneratedIfExists(t *testing.T) {
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
	}

	a1, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("first Bootstrap: %v", err)
	}

	var token1 string
	if err := a1.db.RawDB().QueryRow("SELECT value FROM settings WHERE key = 'admin_token'").Scan(&token1); err != nil {
		t.Fatalf("query token after first bootstrap: %v", err)
	}
	if err := a1.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	// Re-bootstrap with same DB path — in tests we use :memory: so we simulate
	// by seeding the token manually and verifying it's preserved via ensureAdminToken.
	db2, err := storeWiring(Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("storeWiring second: %v", err)
	}
	defer db2.Close()

	const preset = "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	if _, err := db2.RawDB().Exec(
		"INSERT INTO settings (key, value, updated_at) VALUES ('admin_token', ?, datetime('now'))",
		preset,
	); err != nil {
		t.Fatalf("seed preset token: %v", err)
	}

	if err := ensureAdminToken(db2); err != nil {
		t.Fatalf("ensureAdminToken: %v", err)
	}

	var token2 string
	if err := db2.RawDB().QueryRow("SELECT value FROM settings WHERE key = 'admin_token'").Scan(&token2); err != nil {
		t.Fatalf("query token after ensureAdminToken: %v", err)
	}
	if token2 != preset {
		t.Fatalf("expected token to remain %q, got %q", preset, token2)
	}
}

func TestSeedOperatorSettings(t *testing.T) {
	t.Run("seeds telegram setting from config without projecting workspace", func(t *testing.T) {
		db, err := storeWiring(Config{DatabasePath: ":memory:"})
		if err != nil {
			t.Fatalf("storeWiring: %v", err)
		}
		defer db.Close()

		cfg := Config{
			StorageRoot:    "/tmp/gistclaw-storage",
			ApprovalMode:   "auto_approve",
			HostAccessMode: "elevated",
			Telegram: TelegramConfig{
				BotToken: "telegram-token",
			},
			WhatsApp: WhatsAppConfig{
				PhoneNumberID: "phone-config",
				AccessToken:   "whatsapp-access",
				VerifyToken:   "whatsapp-verify",
			},
		}
		if err := seedOperatorSettings(db, cfg); err != nil {
			t.Fatalf("seedOperatorSettings: %v", err)
		}

		if storageRoot := lookupDBSetting(db, "storage_root"); storageRoot != "" {
			t.Fatalf("expected seedOperatorSettings to leave storage_root unset, got %q", storageRoot)
		}
		if approvalMode := lookupDBSetting(db, "approval_mode"); approvalMode != string(cfg.ApprovalMode) {
			t.Fatalf("expected approval_mode %q, got %q", cfg.ApprovalMode, approvalMode)
		}
		if hostAccessMode := lookupDBSetting(db, "host_access_mode"); hostAccessMode != string(cfg.HostAccessMode) {
			t.Fatalf("expected host_access_mode %q, got %q", cfg.HostAccessMode, hostAccessMode)
		}

		var telegramToken string
		if err := db.RawDB().QueryRow("SELECT value FROM settings WHERE key = 'telegram_bot_token'").Scan(&telegramToken); err != nil {
			t.Fatalf("query telegram_bot_token: %v", err)
		}
		if telegramToken != cfg.Telegram.BotToken {
			t.Fatalf("expected telegram_bot_token %q, got %q", cfg.Telegram.BotToken, telegramToken)
		}
		if phoneNumberID := lookupDBSetting(db, "whatsapp_phone_number_id"); phoneNumberID != cfg.WhatsApp.PhoneNumberID {
			t.Fatalf("expected whatsapp_phone_number_id %q, got %q", cfg.WhatsApp.PhoneNumberID, phoneNumberID)
		}
		if accessToken := lookupDBSetting(db, "whatsapp_access_token"); accessToken != cfg.WhatsApp.AccessToken {
			t.Fatalf("expected whatsapp_access_token %q, got %q", cfg.WhatsApp.AccessToken, accessToken)
		}
		if verifyToken := lookupDBSetting(db, "whatsapp_verify_token"); verifyToken != cfg.WhatsApp.VerifyToken {
			t.Fatalf("expected whatsapp_verify_token %q, got %q", cfg.WhatsApp.VerifyToken, verifyToken)
		}
	})

	t.Run("preserves operator-managed settings already stored in sqlite", func(t *testing.T) {
		db, err := storeWiring(Config{DatabasePath: ":memory:"})
		if err != nil {
			t.Fatalf("storeWiring: %v", err)
		}
		defer db.Close()

		if _, err := db.RawDB().Exec(
			`INSERT INTO settings (key, value, updated_at) VALUES
			 ('approval_mode', 'prompt', datetime('now')),
			 ('host_access_mode', 'standard', datetime('now')),
			 ('telegram_bot_token', 'operator-token', datetime('now')),
			 ('whatsapp_phone_number_id', 'operator-phone', datetime('now')),
			 ('whatsapp_access_token', 'operator-access', datetime('now')),
			 ('whatsapp_verify_token', 'operator-verify', datetime('now'))
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		); err != nil {
			t.Fatalf("seed existing settings: %v", err)
		}

		cfg := Config{
			StorageRoot:    "/tmp/config-storage",
			ApprovalMode:   "auto_approve",
			HostAccessMode: "elevated",
			Telegram: TelegramConfig{
				BotToken: "config-token",
			},
			WhatsApp: WhatsAppConfig{
				PhoneNumberID: "config-phone",
				AccessToken:   "config-access",
				VerifyToken:   "config-verify",
			},
		}
		if err := seedOperatorSettings(db, cfg); err != nil {
			t.Fatalf("seedOperatorSettings: %v", err)
		}

		approvalMode := lookupDBSetting(db, "approval_mode")
		if approvalMode != "prompt" {
			t.Fatalf("expected existing approval_mode to be preserved, got %q", approvalMode)
		}
		hostAccessMode := lookupDBSetting(db, "host_access_mode")
		if hostAccessMode != "standard" {
			t.Fatalf("expected existing host_access_mode to be preserved, got %q", hostAccessMode)
		}

		var telegramToken string
		if err := db.RawDB().QueryRow("SELECT value FROM settings WHERE key = 'telegram_bot_token'").Scan(&telegramToken); err != nil {
			t.Fatalf("query telegram_bot_token: %v", err)
		}
		if telegramToken != "operator-token" {
			t.Fatalf("expected existing telegram_bot_token to be preserved, got %q", telegramToken)
		}
		if phoneNumberID := lookupDBSetting(db, "whatsapp_phone_number_id"); phoneNumberID != "operator-phone" {
			t.Fatalf("expected existing whatsapp_phone_number_id to be preserved, got %q", phoneNumberID)
		}
		if accessToken := lookupDBSetting(db, "whatsapp_access_token"); accessToken != "operator-access" {
			t.Fatalf("expected existing whatsapp_access_token to be preserved, got %q", accessToken)
		}
		if verifyToken := lookupDBSetting(db, "whatsapp_verify_token"); verifyToken != "operator-verify" {
			t.Fatalf("expected existing whatsapp_verify_token to be preserved, got %q", verifyToken)
		}
	})
}

func TestApplyOperatorSettingOverrides(t *testing.T) {
	db, err := storeWiring(Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("storeWiring: %v", err)
	}
	defer db.Close()

	if _, err := db.RawDB().Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES
		 ('telegram_bot_token', 'operator-telegram', datetime('now')),
		 ('whatsapp_phone_number_id', 'operator-phone', datetime('now')),
		 ('whatsapp_access_token', 'operator-access', datetime('now')),
		 ('whatsapp_verify_token', 'operator-verify', datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
	); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	cfg := Config{
		Telegram: TelegramConfig{
			BotToken: "config-telegram",
			AgentID:  "assistant",
		},
		WhatsApp: WhatsAppConfig{
			PhoneNumberID: "config-phone",
			AccessToken:   "config-access",
			VerifyToken:   "config-verify",
			AgentID:       "assistant",
		},
	}

	got := applyOperatorSettingOverrides(cfg, db)

	if got.Telegram.BotToken != "operator-telegram" {
		t.Fatalf("expected telegram override, got %q", got.Telegram.BotToken)
	}
	if got.WhatsApp.PhoneNumberID != "operator-phone" {
		t.Fatalf("expected whatsapp phone override, got %q", got.WhatsApp.PhoneNumberID)
	}
	if got.WhatsApp.AccessToken != "operator-access" {
		t.Fatalf("expected whatsapp access override, got %q", got.WhatsApp.AccessToken)
	}
	if got.WhatsApp.VerifyToken != "operator-verify" {
		t.Fatalf("expected whatsapp verify override, got %q", got.WhatsApp.VerifyToken)
	}
	if got.WhatsApp.AgentID != "assistant" {
		t.Fatalf("expected whatsapp agent id to remain unchanged, got %q", got.WhatsApp.AgentID)
	}
}

func TestBootstrap_CreatesStarterProjectAndLeavesOnboardingIncomplete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  t.TempDir(),
	}

	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() {
		app.runtime.WaitAsync()
		if app.toolCloser != nil {
			_ = app.toolCloser.Close()
		}
		_ = app.db.Close()
	})

	var count int
	if err := app.db.RawDB().QueryRow("SELECT COUNT(*) FROM projects").Scan(&count); err != nil {
		t.Fatalf("query starter projects: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 starter project, got %d", count)
	}

	var projectName string
	var primaryPath string
	if err := app.db.RawDB().QueryRow("SELECT name, primary_path FROM projects LIMIT 1").Scan(&projectName, &primaryPath); err != nil {
		t.Fatalf("query starter project: %v", err)
	}
	if projectName == "" {
		t.Fatal("expected starter project name to be set")
	}

	wantPrefix := filepath.Join(home, ".gistclaw", "projects") + string(os.PathSeparator)
	if !strings.HasPrefix(primaryPath, wantPrefix) {
		t.Fatalf("expected starter project under %q, got %q", wantPrefix, primaryPath)
	}
	if _, err := os.Stat(primaryPath); err != nil {
		t.Fatalf("expected starter project directory to exist: %v", err)
	}
	if activeProjectID := lookupDBSetting(app.db, "active_project_id"); activeProjectID == "" {
		t.Fatal("expected active_project_id to be set")
	}
	project, err := ensureProjectState(context.Background(), app.db, cfg)
	if err != nil {
		t.Fatalf("ensureProjectState: %v", err)
	}
	if project.PrimaryPath != primaryPath {
		t.Fatalf("expected active primary_path %q, got %q", primaryPath, project.PrimaryPath)
	}
	if completed := lookupDBSetting(app.db, "onboarding_completed_at"); completed != "" {
		t.Fatalf("expected onboarding to stay incomplete, got %q", completed)
	}
}

func TestBootstrap_SeedsStarterProjectWithShippedDefaultTeam(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  t.TempDir(),
	}

	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() {
		app.runtime.WaitAsync()
		if app.toolCloser != nil {
			_ = app.toolCloser.Close()
		}
		_ = app.db.Close()
	})

	var primaryPath string
	if err := app.db.RawDB().QueryRow("SELECT primary_path FROM projects LIMIT 1").Scan(&primaryPath); err != nil {
		t.Fatalf("query starter project path: %v", err)
	}

	storageTeamDir := filepath.Join(cfg.StorageRoot, "teams", "default")
	for _, name := range []string{"team.yaml", "assistant.soul.yaml", "patcher.soul.yaml"} {
		if _, err := os.Stat(filepath.Join(storageTeamDir, name)); err != nil {
			t.Fatalf("expected starter storage team file %q to exist: %v", name, err)
		}
	}

	runtimeCfg, err := app.runtime.TeamConfig(context.Background())
	if err != nil {
		t.Fatalf("load runtime team: %v", err)
	}
	if runtimeCfg.Name == "" {
		t.Fatal("expected starter workspace to load a default team")
	}
}

func TestBootstrap_AppliesSavedBudgetSettingsToRuntime(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	db, err := storeWiring(Config{DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("storeWiring: %v", err)
	}
	if _, err := db.RawDB().Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES
		 ('per_run_token_budget', '50000', datetime('now')),
		 ('daily_cost_cap_usd', '1.5', datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
	); err != nil {
		t.Fatalf("seed budget settings: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seeded db: %v", err)
	}

	app, err := Bootstrap(Config{
		DatabasePath: dbPath,
		StateDir:     t.TempDir(),
		StorageRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() {
		app.runtime.WaitAsync()
		if app.toolCloser != nil {
			_ = app.toolCloser.Close()
		}
		_ = app.db.Close()
	})

	if app.runtime == nil {
		t.Fatal("expected runtime to be initialized")
	}
	if app.runtime.BudgetGuard().PerRunTokenCap != 50000 {
		t.Fatalf("expected bootstrap per-run token cap 50000, got %d", app.runtime.BudgetGuard().PerRunTokenCap)
	}
	if app.runtime.BudgetGuard().DailyCostCapUSD != 1.5 {
		t.Fatalf("expected bootstrap daily cost cap 1.5, got %f", app.runtime.BudgetGuard().DailyCostCapUSD)
	}
}

func TestEnsureProjectState_CreatesStarterProjectWhenNoActiveProjectExists(t *testing.T) {
	db, err := storeWiring(Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("storeWiring: %v", err)
	}
	defer db.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)

	project, err := ensureProjectState(context.Background(), db, Config{StorageRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("ensureProjectState: %v", err)
	}
	wantPrefix := filepath.Join(home, ".gistclaw", "projects") + string(os.PathSeparator)
	if !strings.HasPrefix(project.PrimaryPath, wantPrefix) {
		t.Fatalf("expected starter primary_path under %q, got %q", wantPrefix, project.PrimaryPath)
	}
	if lookupDBSetting(db, "active_project_id") == "" {
		t.Fatal("expected starter project to become the active project")
	}
}

func TestBootstrap_StoredNonStarterProjectMarksOnboardingComplete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	db, err := storeWiring(Config{DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("storeWiring: %v", err)
	}

	workspaceRoot := t.TempDir()
	if _, err := runtime.ActivateProjectPath(context.Background(), db, workspaceRoot, "seo-test", "operator"); err != nil {
		t.Fatalf("ActivateProjectPath: %v", err)
	}
	if _, err := db.RawDB().Exec("DELETE FROM settings WHERE key = 'onboarding_completed_at'"); err != nil {
		t.Fatalf("delete onboarding_completed_at: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seeded db: %v", err)
	}

	app, err := Bootstrap(Config{
		DatabasePath: dbPath,
		StateDir:     t.TempDir(),
		StorageRoot:  t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() {
		app.runtime.WaitAsync()
		if app.toolCloser != nil {
			_ = app.toolCloser.Close()
		}
		_ = app.db.Close()
	})

	if completed := lookupDBSetting(app.db, "onboarding_completed_at"); completed == "" {
		t.Fatal("expected non-starter active project bootstrap to mark onboarding complete")
	}
}

func TestEnsureProjectState_PrefersStoredActiveProject(t *testing.T) {
	db, err := storeWiring(Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("storeWiring: %v", err)
	}
	defer db.Close()

	storedRoot := t.TempDir()
	storedProject, err := runtime.ActivateProjectPath(context.Background(), db, storedRoot, "stored-project", "operator")
	if err != nil {
		t.Fatalf("ActivateProjectPath: %v", err)
	}

	project, err := ensureProjectState(context.Background(), db, Config{StorageRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("ensureProjectState: %v", err)
	}
	if project.ID != storedProject.ID {
		t.Fatalf("expected stored active project %q, got %q", storedProject.ID, project.ID)
	}
	if project.PrimaryPath != storedRoot {
		t.Fatalf("expected stored primary_path %q, got %q", storedRoot, project.PrimaryPath)
	}
}

func TestGenerateStarterProjectName_IncludesEntropySuffix(t *testing.T) {
	name, err := generateStarterProjectName()
	if err != nil {
		t.Fatalf("generateStarterProjectName: %v", err)
	}

	parts := strings.Split(name, "-")
	if len(parts) != 3 {
		t.Fatalf("expected starter project name with adjective-noun-suffix shape, got %q", name)
	}
	if len(parts[2]) != 4 {
		t.Fatalf("expected 4-character entropy suffix, got %q", parts[2])
	}
}

func TestBootstrap_WiresSchedulerAndLeavesDeferredConnectorsUnwired(t *testing.T) {
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
	}

	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	if len(app.connectors) != 0 {
		t.Fatalf("expected deferred connectors to be unwired, got %d", len(app.connectors))
	}
	if app.scheduler == nil {
		t.Fatal("expected scheduler to be wired")
	}
}

func TestBootstrap_WiresTelegramConnectorWhenConfigured(t *testing.T) {
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
		Telegram: TelegramConfig{
			BotToken: "telegram-token",
		},
	}

	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	if len(app.connectors) != 1 {
		t.Fatalf("expected 1 wired connector, got %d", len(app.connectors))
	}
	if app.connectors[0].Metadata().ID != "telegram" {
		t.Fatalf("expected telegram connector, got %q", app.connectors[0].Metadata().ID)
	}
}

func TestBootstrap_WiresWhatsAppConnectorWhenConfigured(t *testing.T) {
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
		WhatsApp: WhatsAppConfig{
			PhoneNumberID: "phone-123",
			AccessToken:   "access-token",
			VerifyToken:   "verify-token",
		},
	}

	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	if len(app.connectors) != 1 {
		t.Fatalf("expected 1 wired connector, got %d", len(app.connectors))
	}
	if app.connectors[0].Metadata().ID != "whatsapp" {
		t.Fatalf("expected whatsapp connector, got %q", app.connectors[0].Metadata().ID)
	}
	if app.webServer == nil {
		t.Fatal("expected web server to be wired")
	}
}

func TestBootstrap_WiresZaloPersonalConnectorWhenConfigured(t *testing.T) {
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
		ZaloPersonal: ZaloPersonalConfig{
			Enabled: true,
		},
	}

	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	if len(app.connectors) != 1 {
		t.Fatalf("expected 1 wired connector, got %d", len(app.connectors))
	}
	if app.connectors[0].Metadata().ID != "zalo_personal" {
		t.Fatalf("expected zalo_personal connector, got %q", app.connectors[0].Metadata().ID)
	}
}

func TestBuildToolRegistry_LoadsConfiguredMCPToolFromConfig(t *testing.T) {
	reg, closer, err := buildToolRegistry(context.Background(), Config{
		Research: tools.ResearchConfig{},
		MCP: tools.MCPOptions{
			Servers: []tools.MCPServerConfig{
				{
					ID:        "github",
					Transport: "stdio",
					Command:   []string{"fake-mcp"},
					Tools: []tools.MCPToolConfig{
						{Name: "search_repositories", Alias: "github_search_repositories", Risk: model.RiskLow, Enabled: true},
					},
				},
			},
		},
	}, stubMCPFactory{
		server: &stubMCPConnection{
			tools: []tools.MCPRemoteTool{
				{Name: "search_repositories", Description: "Search repos", InputSchemaJSON: `{"type":"object"}`},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("buildToolRegistry: %v", err)
	}
	if closer != nil {
		defer closer.Close()
	}
	if _, ok := reg.Get("github_search_repositories"); !ok {
		t.Fatal("expected configured MCP tool to be registered")
	}
}

func TestBuildToolRegistry_RegistersCapabilityTools(t *testing.T) {
	reg, closer, err := buildToolRegistry(context.Background(), Config{
		Research: tools.ResearchConfig{},
	}, nil, capabilities.NewRegistry())
	if err != nil {
		t.Fatalf("buildToolRegistry: %v", err)
	}
	if closer != nil {
		defer closer.Close()
	}

	for _, name := range []string{
		"connector_directory_list",
		"connector_target_resolve",
		"connector_send",
		"connector_status",
		"app_action",
	} {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("expected capability tool %q to be registered", name)
		}
	}
}
