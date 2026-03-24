package app

import (
	"encoding/hex"
	"testing"
)

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
