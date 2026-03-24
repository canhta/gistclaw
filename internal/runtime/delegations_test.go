package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

func setupDelegationTestDeps(t *testing.T) (*Runtime, *store.DB) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cs := conversations.NewConversationStore(db)
	mem := memory.NewStore(db, cs)
	reg := tools.NewRegistry()
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "child done", InputTokens: 10, OutputTokens: 20, StopReason: "end_turn"},
		},
		nil,
	)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)
	return rt, db
}

func insertRootRun(t *testing.T, db *store.DB, runID, convID string, snapshot map[string]interface{}) {
	t.Helper()
	snapJSON, _ := json.Marshal(snapshot)
	_, err := db.RawDB().Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, execution_snapshot_json, created_at, updated_at)
		 VALUES (?, ?, 'agent-a', 'active', ?, datetime('now'), datetime('now'))`,
		runID, convID, snapJSON,
	)
	if err != nil {
		t.Fatalf("insert root run: %v", err)
	}
}

func TestDelegation_ValidEdgeCreatesChildRun(t *testing.T) {
	rt, db := setupDelegationTestDeps(t)
	ctx := context.Background()

	snapshot := map[string]interface{}{
		"handoff_edges": []map[string]string{
			{"from": "agent-a", "to": "agent-b"},
		},
	}
	insertRootRun(t, db, "root-1", "conv-d1", snapshot)

	run, err := rt.Delegate(ctx, DelegateRun{
		ParentRunID:   "root-1",
		TargetAgentID: "agent-b",
		Objective:     "delegated task",
	})
	if err != nil {
		t.Fatalf("Delegate failed: %v", err)
	}
	if run.ID == "" {
		t.Fatal("expected non-empty child run ID")
	}
	if run.ParentRunID != "root-1" {
		t.Fatalf("expected parent_run_id %q, got %q", "root-1", run.ParentRunID)
	}
}

func TestDelegation_InvalidEdgeJournalsErrorNotPanic(t *testing.T) {
	rt, db := setupDelegationTestDeps(t)
	ctx := context.Background()

	snapshot := map[string]interface{}{
		"handoff_edges": []map[string]string{
			{"from": "agent-a", "to": "agent-b"},
		},
	}
	insertRootRun(t, db, "root-2", "conv-d2", snapshot)

	_, err := rt.Delegate(ctx, DelegateRun{
		ParentRunID:   "root-2",
		TargetAgentID: "agent-c",
		Objective:     "should fail",
	})
	if err == nil {
		t.Fatal("expected error for invalid handoff edge")
	}

	var errEventCount int
	err2 := db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = 'root-2' AND kind = 'delegation_rejected'",
	).Scan(&errEventCount)
	if err2 != nil {
		t.Fatalf("query events: %v", err2)
	}
	if errEventCount == 0 {
		t.Fatal("expected delegation_rejected event in journal")
	}

	var rootStatus string
	err2 = db.RawDB().QueryRow("SELECT status FROM runs WHERE id = 'root-2'").Scan(&rootStatus)
	if err2 != nil {
		t.Fatalf("query root run: %v", err2)
	}
	if rootStatus != "active" {
		t.Fatalf("root run should still be active, got %q", rootStatus)
	}
}

func TestDelegation_FullBudgetQueues(t *testing.T) {
	rt, db := setupDelegationTestDeps(t)
	ctx := context.Background()

	snapshot := map[string]interface{}{
		"handoff_edges": []map[string]string{
			{"from": "agent-a", "to": "agent-b"},
			{"from": "agent-a", "to": "agent-c"},
			{"from": "agent-a", "to": "agent-d"},
			{"from": "agent-a", "to": "agent-e"},
		},
		"max_active_children": 3,
	}
	insertRootRun(t, db, "root-3", "conv-d3", snapshot)
	rt.maxActiveChildren = 3

	for _, agent := range []string{"agent-b", "agent-c", "agent-d"} {
		_, err := rt.Delegate(ctx, DelegateRun{
			ParentRunID:   "root-3",
			TargetAgentID: agent,
			Objective:     "task " + agent,
		})
		if err != nil {
			t.Fatalf("Delegate %s failed: %v", agent, err)
		}
	}

	_, err := rt.Delegate(ctx, DelegateRun{
		ParentRunID:   "root-3",
		TargetAgentID: "agent-e",
		Objective:     "should be queued",
	})
	if err != nil {
		t.Fatalf("4th Delegate failed: %v", err)
	}

	var activeCount int
	var queuedCount int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM delegations WHERE parent_run_id = 'root-3' AND status = 'active'",
	).Scan(&activeCount)
	if err != nil {
		t.Fatalf("query active: %v", err)
	}
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM delegations WHERE parent_run_id = 'root-3' AND status = 'queued'",
	).Scan(&queuedCount)
	if err != nil {
		t.Fatalf("query queued: %v", err)
	}

	if activeCount != 3 {
		t.Fatalf("expected 3 active delegations, got %d", activeCount)
	}
	if queuedCount != 1 {
		t.Fatalf("expected 1 queued delegation, got %d", queuedCount)
	}
}

func TestDelegation_SnapshotImmutability(t *testing.T) {
	rt, db := setupDelegationTestDeps(t)
	ctx := context.Background()

	snapshot := map[string]interface{}{
		"handoff_edges": []map[string]string{
			{"from": "agent-a", "to": "agent-b"},
		},
	}
	insertRootRun(t, db, "root-4", "conv-d4", snapshot)

	_, err := rt.Delegate(ctx, DelegateRun{
		ParentRunID:   "root-4",
		TargetAgentID: "agent-x",
		Objective:     "should fail",
	})
	if err == nil {
		t.Fatal("expected error: agent-x not in frozen snapshot")
	}
}

func TestDelegation_QueuedVisibleAfterRestart(t *testing.T) {
	rt, db := setupDelegationTestDeps(t)
	ctx := context.Background()

	_, err := db.RawDB().Exec(
		`INSERT INTO delegations (id, root_run_id, parent_run_id, target_agent_id, status, created_at)
		 VALUES ('del-q1', 'root-5', 'root-5', 'agent-b', 'queued', datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert delegation: %v", err)
	}

	_, err = db.RawDB().Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
		 VALUES ('root-5', 'conv-d5', 'agent-a', 'active', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}

	report, err := rt.ReconcileInterrupted(ctx)
	if err != nil {
		t.Fatalf("ReconcileInterrupted failed: %v", err)
	}
	_ = report

	var queuedCount int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM delegations WHERE id = 'del-q1' AND status = 'queued'",
	).Scan(&queuedCount)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if queuedCount != 1 {
		t.Fatalf("expected queued delegation to still exist, got count %d", queuedCount)
	}
}

func TestDelegation_RollsBackWhenDelegationEventFails(t *testing.T) {
	rt, db := setupDelegationTestDeps(t)
	ctx := context.Background()

	snapshot := map[string]interface{}{
		"handoff_edges": []map[string]string{
			{"from": "agent-a", "to": "agent-b"},
		},
	}
	insertRootRun(t, db, "root-rollback", "conv-d-rollback", snapshot)

	_, err := db.RawDB().Exec(
		`CREATE TRIGGER fail_events_before_insert
		 BEFORE INSERT ON events
		 BEGIN
		   SELECT RAISE(FAIL, 'boom');
		 END;`,
	)
	if err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	_, err = rt.Delegate(ctx, DelegateRun{
		ParentRunID:   "root-rollback",
		TargetAgentID: "agent-b",
		Objective:     "should rollback",
	})
	if err == nil {
		t.Fatal("expected delegation error, got nil")
	}

	var childRuns int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM runs WHERE parent_run_id = 'root-rollback'",
	).Scan(&childRuns)
	if err != nil {
		t.Fatalf("count child runs: %v", err)
	}
	if childRuns != 0 {
		t.Fatalf("expected 0 child runs after rollback, got %d", childRuns)
	}

	var delegations int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM delegations WHERE parent_run_id = 'root-rollback'",
	).Scan(&delegations)
	if err != nil {
		t.Fatalf("count delegations: %v", err)
	}
	if delegations != 0 {
		t.Fatalf("expected 0 delegations after rollback, got %d", delegations)
	}
}

func TestDelegation_NestedDelegationsUseRootBudget(t *testing.T) {
	rt, db := setupDelegationTestDeps(t)
	ctx := context.Background()

	snapshot := map[string]interface{}{
		"handoff_edges": []map[string]string{
			{"from": "agent-a", "to": "agent-b"},
			{"from": "agent-b", "to": "agent-c"},
		},
		"max_active_children": 1,
	}
	insertRootRun(t, db, "root-budget", "conv-d-budget", snapshot)

	child, err := rt.Delegate(ctx, DelegateRun{
		ParentRunID:   "root-budget",
		TargetAgentID: "agent-b",
		Objective:     "first child",
	})
	if err != nil {
		t.Fatalf("first delegation failed: %v", err)
	}

	queued, err := rt.Delegate(ctx, DelegateRun{
		ParentRunID:   child.ID,
		TargetAgentID: "agent-c",
		Objective:     "should queue under root budget",
	})
	if err != nil {
		t.Fatalf("nested delegation failed: %v", err)
	}
	if queued.Status != model.RunStatusPending {
		t.Fatalf("expected nested delegation to queue, got %q", queued.Status)
	}

	var queuedCount int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM delegations WHERE root_run_id = 'root-budget' AND status = 'queued'",
	).Scan(&queuedCount)
	if err != nil {
		t.Fatalf("count queued delegations: %v", err)
	}
	if queuedCount != 1 {
		t.Fatalf("expected 1 queued nested delegation, got %d", queuedCount)
	}
}

func TestReconcile_ActiveRunsBecomesInterrupted(t *testing.T) {
	rt, db := setupDelegationTestDeps(t)
	ctx := context.Background()

	for i, id := range []string{"run-a1", "run-a2"} {
		_, err := db.RawDB().Exec(
			`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
			 VALUES (?, ?, 'agent-a', 'active', datetime('now'), datetime('now'))`,
			id, fmt.Sprintf("conv-r-%d", i+1),
		)
		if err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}

	report, err := rt.ReconcileInterrupted(ctx)
	if err != nil {
		t.Fatalf("ReconcileInterrupted failed: %v", err)
	}
	if report.ReconciledCount != 2 {
		t.Fatalf("expected 2 reconciled, got %d", report.ReconciledCount)
	}

	for _, id := range []string{"run-a1", "run-a2"} {
		var status string
		err := db.RawDB().QueryRow("SELECT status FROM runs WHERE id = ?", id).Scan(&status)
		if err != nil {
			t.Fatalf("query %s: %v", id, err)
		}
		if status != "interrupted" {
			t.Fatalf("expected 'interrupted' for %s, got %q", id, status)
		}
	}
}

func TestReconcile_NeverAutoResumes(t *testing.T) {
	rt, db := setupDelegationTestDeps(t)
	ctx := context.Background()

	_, err := db.RawDB().Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
		 VALUES ('run-no-resume', 'conv-nr', 'agent-a', 'active', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	report, err := rt.ReconcileInterrupted(ctx)
	if err != nil {
		t.Fatalf("ReconcileInterrupted failed: %v", err)
	}
	if report.ReconciledCount != 1 {
		t.Fatalf("expected 1 reconciled, got %d", report.ReconciledCount)
	}

	var activeCount int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM runs WHERE status = 'active'",
	).Scan(&activeCount)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if activeCount != 0 {
		t.Fatalf("expected 0 active runs after reconcile, got %d", activeCount)
	}
}
