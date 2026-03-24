package app

import (
	"context"
	"path/filepath"
	"testing"
)

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

func setupCommandApp(t *testing.T) *App {
	t.Helper()

	cfg := Config{
		DatabasePath:  filepath.Join(t.TempDir(), "state", "runtime.db"),
		StateDir:      filepath.Join(t.TempDir(), "state"),
		WorkspaceRoot: t.TempDir(),
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
