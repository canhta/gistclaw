package replay

import (
	"context"
	"testing"

	"github.com/canhta/gistclaw/internal/store"
)

func setupReplayDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestReplay_LoadRunFromJournal(t *testing.T) {
	db := setupReplayDB(t)
	ctx := context.Background()

	_, err := db.RawDB().Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
		 VALUES ('run-r1', 'conv-r1', 'agent-a', 'completed', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}
	_, err = db.RawDB().Exec(
		`INSERT INTO events (id, conversation_id, run_id, kind, created_at)
		 VALUES ('evt-1', 'conv-r1', 'run-r1', 'run_started', datetime('now', '-2 seconds'))`,
	)
	if err != nil {
		t.Fatalf("insert event 1: %v", err)
	}
	_, err = db.RawDB().Exec(
		`INSERT INTO events (id, conversation_id, run_id, kind, created_at)
		 VALUES ('evt-2', 'conv-r1', 'run-r1', 'run_completed', datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert event 2: %v", err)
	}

	svc := NewService(db)
	replay, err := svc.LoadRun(ctx, "run-r1")
	if err != nil {
		t.Fatalf("LoadRun failed: %v", err)
	}
	if replay.RunID != "run-r1" {
		t.Fatalf("expected %q, got %q", "run-r1", replay.RunID)
	}
	if len(replay.Events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(replay.Events))
	}
}

func TestReplay_LoadRunIncludesExecutionSnapshot(t *testing.T) {
	db := setupReplayDB(t)
	ctx := context.Background()

	snapshotJSON := []byte(`{"team_id":"Repo Task Team","agents":{"assistant":{"agent_id":"assistant","tool_profile":"operator_facing","capabilities":["operator_facing","spawn"]}}}`)
	_, err := db.RawDB().Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, team_id, status, execution_snapshot_json, created_at, updated_at)
		 VALUES ('run-snapshot', 'conv-snapshot', 'assistant', 'Repo Task Team', 'completed', ?, datetime('now'), datetime('now'))`,
		snapshotJSON,
	)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}

	svc := NewService(db)
	replay, err := svc.LoadRun(ctx, "run-snapshot")
	if err != nil {
		t.Fatalf("LoadRun failed: %v", err)
	}
	if replay.TeamID != "Repo Task Team" {
		t.Fatalf("expected team id, got %q", replay.TeamID)
	}
	if string(replay.ExecutionSnapshotJSON) != string(snapshotJSON) {
		t.Fatalf("expected snapshot json %s, got %s", snapshotJSON, replay.ExecutionSnapshotJSON)
	}
}

func TestLoadGraph_UsesSessionLineageInsteadOfHandoffEdges(t *testing.T) {
	db := setupReplayDB(t)
	ctx := context.Background()

	_, err := db.RawDB().Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
		 VALUES ('run-front', 'conv-sessions', 'assistant', 'completed', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert front run: %v", err)
	}
	_, err = db.RawDB().Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, parent_run_id, status, created_at, updated_at)
		 VALUES ('run-worker', 'conv-sessions', 'researcher', 'run-front', 'completed', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert worker run: %v", err)
	}

	svc := NewService(db)
	graph, err := svc.LoadGraph(ctx, "run-front")
	if err != nil {
		t.Fatalf("LoadGraph failed: %v", err)
	}
	if len(graph.Edges) != 1 {
		t.Fatalf("expected 1 lineage edge, got %d", len(graph.Edges))
	}
	if graph.Edges[0].From != "run-front" || graph.Edges[0].To != "run-worker" {
		t.Fatalf("unexpected edge: %+v", graph.Edges[0])
	}
}

func TestReceipt_ContainsRequiredFields(t *testing.T) {
	db := setupReplayDB(t)
	ctx := context.Background()

	_, err := db.RawDB().Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, input_tokens, output_tokens, model_lane, created_at, updated_at)
		 VALUES ('run-rcpt', 'conv-rcpt', 'agent-a', 'completed', 100, 200, 'cheap', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}
	_, err = db.RawDB().Exec(
		`INSERT INTO receipts (id, run_id, input_tokens, output_tokens, cost_usd, model_lane, created_at)
		 VALUES ('rcpt-1', 'run-rcpt', 100, 200, 0.05, 'cheap', datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert receipt: %v", err)
	}

	svc := NewService(db)
	receipt, err := svc.Build(ctx, "run-rcpt")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if receipt.RunID != "run-rcpt" {
		t.Fatalf("expected %q, got %q", "run-rcpt", receipt.RunID)
	}
	if receipt.InputTokens != 100 {
		t.Fatalf("expected 100 input tokens, got %d", receipt.InputTokens)
	}
	if receipt.OutputTokens != 200 {
		t.Fatalf("expected 200 output tokens, got %d", receipt.OutputTokens)
	}
}

func TestPreviewPackage_MakesNoModelCalls(t *testing.T) {
	db := setupReplayDB(t)
	ctx := context.Background()

	_, err := db.RawDB().Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
		 VALUES ('run-prev', 'conv-prev', 'agent-a', 'completed', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}
	_, err = db.RawDB().Exec(
		`INSERT INTO receipts (id, run_id, input_tokens, output_tokens, cost_usd, created_at)
		 VALUES ('rcpt-prev', 'run-prev', 50, 60, 0.01, datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert receipt: %v", err)
	}

	svc := NewService(db)
	pkg, err := svc.BuildPreviewPackage(ctx, "run-prev")
	if err != nil {
		t.Fatalf("BuildPreviewPackage failed: %v", err)
	}
	if pkg.RunID != "run-prev" {
		t.Fatalf("expected %q, got %q", "run-prev", pkg.RunID)
	}
}
