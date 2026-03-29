package app

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAppNodeInventoryStatusReportsConnectorsRunsAndCapabilities(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := Config{
		StorageRoot:  filepath.Join(root, "storage"),
		StateDir:     filepath.Join(root, "state"),
		DatabasePath: filepath.Join(root, "state", "gistclaw.db"),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "test-key",
		},
		Telegram: TelegramConfig{
			BotToken: "telegram-token",
			AgentID:  "assistant",
		},
	}

	application, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("bootstrap app: %v", err)
	}
	defer func() { _ = application.Stop() }()

	var activeProjectID string
	if err := application.db.RawDB().QueryRowContext(
		context.Background(),
		"SELECT value FROM settings WHERE key = 'active_project_id'",
	).Scan(&activeProjectID); err != nil {
		t.Fatalf("load active project id: %v", err)
	}
	if _, err := application.db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, project_id, objective, cwd, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"run-root",
		"conv-root",
		"assistant",
		activeProjectID,
		"Review the repo layout",
		root,
		"active",
		"2026-03-29 09:30:00",
		"2026-03-29 09:31:00",
	); err != nil {
		t.Fatalf("insert run: %v", err)
	}

	status, err := application.NodeInventoryStatus(context.Background())
	if err != nil {
		t.Fatalf("node inventory status: %v", err)
	}

	if status.Summary.Connectors < 1 {
		t.Fatalf("expected configured connectors, got %+v", status.Summary)
	}
	if status.Summary.RunNodes < 1 {
		t.Fatalf("expected run nodes, got %+v", status.Summary)
	}
	if status.Summary.Capabilities < 1 {
		t.Fatalf("expected capabilities, got %+v", status.Summary)
	}
	if len(status.Connectors) == 0 || status.Connectors[0].ID != "telegram" {
		t.Fatalf("unexpected connectors: %+v", status.Connectors)
	}
	if len(status.Runs) == 0 || status.Runs[0].ID != "run-root" {
		t.Fatalf("unexpected runs: %+v", status.Runs)
	}
	if status.Runs[0].Kind != "root" {
		t.Fatalf("expected root run kind, got %+v", status.Runs[0])
	}
	foundCapability := false
	for _, capability := range status.Capabilities {
		if capability.Name == "connector_send" {
			foundCapability = true
			break
		}
	}
	if !foundCapability {
		t.Fatalf("expected connector_send capability in %+v", status.Capabilities)
	}
}
