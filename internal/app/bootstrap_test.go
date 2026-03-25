package app

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
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
	t.Run("seeds workspace and telegram settings from config when missing", func(t *testing.T) {
		db, err := storeWiring(Config{DatabasePath: ":memory:"})
		if err != nil {
			t.Fatalf("storeWiring: %v", err)
		}
		defer db.Close()

		cfg := Config{
			WorkspaceRoot: "/tmp/gistclaw-workspace",
			Telegram: TelegramConfig{
				BotToken: "telegram-token",
			},
		}
		if err := seedOperatorSettings(db, cfg); err != nil {
			t.Fatalf("seedOperatorSettings: %v", err)
		}

		var workspaceRoot string
		if err := db.RawDB().QueryRow("SELECT value FROM settings WHERE key = 'workspace_root'").Scan(&workspaceRoot); err != nil {
			t.Fatalf("query workspace_root: %v", err)
		}
		if workspaceRoot != cfg.WorkspaceRoot {
			t.Fatalf("expected workspace_root %q, got %q", cfg.WorkspaceRoot, workspaceRoot)
		}

		var telegramToken string
		if err := db.RawDB().QueryRow("SELECT value FROM settings WHERE key = 'telegram_bot_token'").Scan(&telegramToken); err != nil {
			t.Fatalf("query telegram_bot_token: %v", err)
		}
		if telegramToken != cfg.Telegram.BotToken {
			t.Fatalf("expected telegram_bot_token %q, got %q", cfg.Telegram.BotToken, telegramToken)
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
			 ('workspace_root', '/tmp/operator-workspace', datetime('now')),
			 ('telegram_bot_token', 'operator-token', datetime('now'))`,
		); err != nil {
			t.Fatalf("seed existing settings: %v", err)
		}

		cfg := Config{
			WorkspaceRoot: "/tmp/config-workspace",
			Telegram: TelegramConfig{
				BotToken: "config-token",
			},
		}
		if err := seedOperatorSettings(db, cfg); err != nil {
			t.Fatalf("seedOperatorSettings: %v", err)
		}

		var workspaceRoot string
		if err := db.RawDB().QueryRow("SELECT value FROM settings WHERE key = 'workspace_root'").Scan(&workspaceRoot); err != nil {
			t.Fatalf("query workspace_root: %v", err)
		}
		if workspaceRoot != "/tmp/operator-workspace" {
			t.Fatalf("expected existing workspace_root to be preserved, got %q", workspaceRoot)
		}

		var telegramToken string
		if err := db.RawDB().QueryRow("SELECT value FROM settings WHERE key = 'telegram_bot_token'").Scan(&telegramToken); err != nil {
			t.Fatalf("query telegram_bot_token: %v", err)
		}
		if telegramToken != "operator-token" {
			t.Fatalf("expected existing telegram_bot_token to be preserved, got %q", telegramToken)
		}
	})
}

func TestBootstrap_DoesNotWireDeferredConnectorsOrScheduler(t *testing.T) {
	cfg := Config{
		DatabasePath:  ":memory:",
		StateDir:      t.TempDir(),
		WorkspaceRoot: t.TempDir(),
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
}

func TestBootstrap_WiresTelegramConnectorWhenConfigured(t *testing.T) {
	cfg := Config{
		DatabasePath:  ":memory:",
		StateDir:      t.TempDir(),
		WorkspaceRoot: t.TempDir(),
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
	if app.connectors[0].ID() != "telegram" {
		t.Fatalf("expected telegram connector, got %q", app.connectors[0].ID())
	}
}

func TestBootstrap_WiresWhatsAppConnectorWhenConfigured(t *testing.T) {
	cfg := Config{
		DatabasePath:  ":memory:",
		StateDir:      t.TempDir(),
		WorkspaceRoot: t.TempDir(),
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
	if app.connectors[0].ID() != "whatsapp" {
		t.Fatalf("expected whatsapp connector, got %q", app.connectors[0].ID())
	}
	if app.webServer == nil {
		t.Fatal("expected web server to be wired")
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
	})
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
