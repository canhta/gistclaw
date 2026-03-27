package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	authpkg "github.com/canhta/gistclaw/internal/auth"
	"github.com/canhta/gistclaw/internal/model"
)

type stubCommandConnector struct {
	id       string
	snapshot model.ConnectorHealthSnapshot
}

func (c *stubCommandConnector) ID() string { return c.id }

func (c *stubCommandConnector) Start(context.Context) error { return nil }

func (c *stubCommandConnector) Notify(context.Context, string, model.ReplayDelta, string) error {
	return nil
}

func (c *stubCommandConnector) Drain(context.Context) error { return nil }

func (c *stubCommandConnector) ConnectorHealthSnapshot() model.ConnectorHealthSnapshot {
	return c.snapshot
}

func TestApp_RunTaskAndInspect(t *testing.T) {
	application := setupCommandApp(t)
	ctx := context.Background()

	run, err := application.RunTask(ctx, "review the repository")
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if run.Status == "" {
		t.Fatal("expected non-empty run status")
	}

	status, err := application.InspectStatus(ctx)
	if err != nil {
		t.Fatalf("InspectStatus failed: %v", err)
	}
	if status.ActiveRuns != 0 || status.PendingApprovals != 0 {
		t.Fatalf("unexpected status counts: %+v", status)
	}

	runs, err := application.ListRuns(ctx)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("expected one run %q, got %+v", run.ID, runs)
	}

	replay, err := application.LoadReplay(ctx, run.ID)
	if err != nil {
		t.Fatalf("LoadReplay failed: %v", err)
	}
	if len(replay.Events) == 0 {
		t.Fatal("expected replay events")
	}

	_, err = application.db.RawDB().Exec(
		`INSERT INTO settings (key, value, updated_at)
		 VALUES ('admin_token', 'app-token', datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
	)
	if err != nil {
		t.Fatalf("insert admin token: %v", err)
	}

	token, err := application.AdminToken(ctx)
	if err != nil {
		t.Fatalf("AdminToken failed: %v", err)
	}
	if token != "app-token" {
		t.Fatalf("expected app-token, got %q", token)
	}
}

func TestApp_RunTaskRejectsEmptyObjective(t *testing.T) {
	application := setupCommandApp(t)

	if _, err := application.RunTask(context.Background(), "   "); err == nil {
		t.Fatal("expected RunTask to reject empty objective")
	}
}

func TestApp_PrepareGeneratesAdminToken(t *testing.T) {
	application := setupCommandApp(t)

	if err := application.Prepare(context.Background()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	token, err := application.AdminToken(context.Background())
	if err != nil {
		t.Fatalf("AdminToken failed: %v", err)
	}
	if len(token) != 64 {
		t.Fatalf("expected 64-char hex token, got %q", token)
	}
	for _, r := range token {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			t.Fatalf("expected hex token, got %q", token)
		}
	}
}

func TestApp_PrepareReconcilesInterruptedRuns(t *testing.T) {
	application := setupCommandApp(t)

	_, err := application.db.RawDB().Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
		 VALUES ('stale-run', 'conv-stale', 'agent-a', 'active', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert stale run: %v", err)
	}

	if err := application.Prepare(context.Background()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	var status string
	err = application.db.RawDB().QueryRow(
		"SELECT status FROM runs WHERE id = 'stale-run'",
	).Scan(&status)
	if err != nil {
		t.Fatalf("query stale run: %v", err)
	}
	if status != "interrupted" {
		t.Fatalf("expected interrupted status, got %q", status)
	}
}

func TestApp_SetPassword(t *testing.T) {
	application := setupCommandApp(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 27, 6, 0, 0, 0, time.UTC)

	if err := application.SetPassword(ctx, "app-secret-pass", now); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if err := authpkg.VerifyPassword(ctx, application.db, "app-secret-pass"); err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
}

func TestApp_ConnectorHealthReturnsReporterSnapshots(t *testing.T) {
	application := &App{
		connectors: []model.Connector{
			&stubCommandConnector{
				id: "telegram",
				snapshot: model.ConnectorHealthSnapshot{
					ConnectorID: "telegram",
					State:       model.ConnectorHealthDegraded,
					Summary:     "poll loop stale",
				},
			},
			&stubCommandConnector{
				id: "whatsapp",
				snapshot: model.ConnectorHealthSnapshot{
					ConnectorID: "whatsapp",
					State:       model.ConnectorHealthHealthy,
					Summary:     "webhook activity recent",
				},
			},
		},
	}

	snapshots, err := application.ConnectorHealth(context.Background())
	if err != nil {
		t.Fatalf("ConnectorHealth failed: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 connector snapshots, got %d", len(snapshots))
	}
	if snapshots[0].ConnectorID != "telegram" || snapshots[0].Summary != "poll loop stale" {
		t.Fatalf("unexpected first snapshot: %+v", snapshots[0])
	}
	if snapshots[1].ConnectorID != "whatsapp" || snapshots[1].State != model.ConnectorHealthHealthy {
		t.Fatalf("unexpected second snapshot: %+v", snapshots[1])
	}
}

func TestConfiguredConnectorHealth_UsesRecentPersistedSnapshot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state", "runtime.db")
	db, err := storeWiring(Config{DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("storeWiring failed: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	raw, err := json.Marshal(model.ConnectorHealthSnapshot{
		ConnectorID: "telegram",
		State:       model.ConnectorHealthHealthy,
		Summary:     "poll loop healthy",
		CheckedAt:   now,
	})
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if _, err := db.RawDB().Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		"connector_health.telegram",
		string(raw),
	); err != nil {
		t.Fatalf("insert connector health: %v", err)
	}

	cfg := Config{
		DatabasePath: dbPath,
		StateDir:     filepath.Dir(dbPath),
		StorageRoot:  t.TempDir(),
		Provider: ProviderConfig{
			Name:   "openai",
			APIKey: "sk-test",
		},
		Telegram: TelegramConfig{
			BotToken: "telegram-test-token",
		},
	}

	snapshots, err := ConfiguredConnectorHealth(context.Background(), cfg, db)
	if err != nil {
		t.Fatalf("ConfiguredConnectorHealth failed: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 connector snapshot, got %d", len(snapshots))
	}
	if snapshots[0].ConnectorID != "telegram" {
		t.Fatalf("expected telegram snapshot, got %+v", snapshots[0])
	}
	if snapshots[0].State != model.ConnectorHealthHealthy || snapshots[0].Summary != "poll loop healthy" {
		t.Fatalf("expected persisted healthy snapshot, got %+v", snapshots[0])
	}
}

func TestConfiguredConnectorHealth_IgnoresStalePersistedSnapshot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state", "runtime.db")
	db, err := storeWiring(Config{DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("storeWiring failed: %v", err)
	}
	defer db.Close()

	raw, err := json.Marshal(model.ConnectorHealthSnapshot{
		ConnectorID: "telegram",
		State:       model.ConnectorHealthHealthy,
		Summary:     "poll loop healthy",
		CheckedAt:   time.Now().UTC().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if _, err := db.RawDB().Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		"connector_health.telegram",
		string(raw),
	); err != nil {
		t.Fatalf("insert connector health: %v", err)
	}

	cfg := Config{
		DatabasePath: dbPath,
		StateDir:     filepath.Dir(dbPath),
		StorageRoot:  t.TempDir(),
		Provider: ProviderConfig{
			Name:   "openai",
			APIKey: "sk-test",
		},
		Telegram: TelegramConfig{
			BotToken: "telegram-test-token",
		},
	}

	snapshots, err := ConfiguredConnectorHealth(context.Background(), cfg, db)
	if err != nil {
		t.Fatalf("ConfiguredConnectorHealth failed: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 connector snapshot, got %d", len(snapshots))
	}
	if snapshots[0].ConnectorID != "telegram" {
		t.Fatalf("expected telegram snapshot, got %+v", snapshots[0])
	}
	if snapshots[0].State != model.ConnectorHealthUnknown || snapshots[0].Summary != "awaiting first poll" {
		t.Fatalf("expected fallback connector snapshot, got %+v", snapshots[0])
	}
}

// startMockAnthropicServer starts a local httptest.Server that returns minimal
// valid Anthropic Messages API responses, and points the provider at it via
// ANTHROPIC_BASE_URL. Must be called before Bootstrap.
func startMockAnthropicServer(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id":   "msg_test",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "mock response"},
			},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
}

func setupCommandApp(t *testing.T) *App {
	t.Helper()
	startMockAnthropicServer(t)

	cfg := Config{
		DatabasePath: filepath.Join(t.TempDir(), "state", "runtime.db"),
		StateDir:     filepath.Join(t.TempDir(), "state"),
		StorageRoot:  t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
	}

	application, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	t.Cleanup(func() { _ = application.Stop() })
	return application
}
