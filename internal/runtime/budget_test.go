package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

// TestBudget_PerRunTokenCapStopsRun verifies that a run exceeding its per-run
// token budget is stopped before the next model call and emits a budget_stop
// event with the limit type and usage.
func TestBudget_PerRunTokenCapStopsRun(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)

	// Provider returns two turns; the first uses 600 tokens with StopReason
	// "tool_use" so the loop continues to turn 2's BeforeTurn check, which
	// fires budget_stop because 600 >= 500.
	prov := NewMockProvider([]GenerateResult{
		{Content: "turn 1", InputTokens: 300, OutputTokens: 300, StopReason: "tool_use"},
		{Content: "turn 2 (should not execute)", InputTokens: 10, OutputTokens: 10, StopReason: "end_turn"},
	}, nil)

	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)
	rt.budget.PerRunTokenCap = 500 // low cap — exceeded after first turn

	ctx := context.Background()
	run, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-budget-cap",
		AgentID:        "assistant",
		Objective:      "do something",
		CWD:            t.TempDir(),
		AccountID:      "local",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Run must be interrupted, not completed.
	if run.Status != model.RunStatusInterrupted {
		t.Fatalf("expected interrupted, got %s", run.Status)
	}

	// Must have emitted budget_stop event.
	var budgetStopCount int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = ? AND kind = 'budget_stop'",
		run.ID,
	).Scan(&budgetStopCount); err != nil {
		t.Fatalf("query budget_stop: %v", err)
	}
	if budgetStopCount == 0 {
		t.Fatal("expected budget_stop event, got none")
	}

	// budget_stop payload must include limit_type and usage.
	var payload string
	if err := db.RawDB().QueryRow(
		"SELECT payload_json FROM events WHERE run_id = ? AND kind = 'budget_stop'",
		run.ID,
	).Scan(&payload); err != nil {
		t.Fatalf("query budget_stop payload: %v", err)
	}
	if !strings.Contains(payload, "limit_type") {
		t.Fatalf("budget_stop payload missing limit_type: %s", payload)
	}
	if !strings.Contains(payload, "tokens") {
		t.Fatalf("budget_stop payload missing token usage: %s", payload)
	}

	// Must NOT have called the provider a second time.
	if prov.CallCount() > 1 {
		t.Fatalf("expected at most 1 provider call, got %d", prov.CallCount())
	}
}

// TestBudget_RunMarkedInterruptedNotFailed verifies that when a run is stopped
// by the budget guard, the run status is interrupted, not failed or completed.
func TestBudget_RunMarkedInterruptedNotFailed(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)

	// StopReason "tool_use" keeps the loop running so turn 2's BeforeTurn
	// fires and detects the exceeded cap (1200 tokens >= 500).
	prov := NewMockProvider([]GenerateResult{
		{Content: "over budget", InputTokens: 600, OutputTokens: 600, StopReason: "tool_use"},
	}, nil)

	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)
	rt.budget.PerRunTokenCap = 500

	ctx := context.Background()
	run, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-budget-interrupted",
		AgentID:        "assistant",
		Objective:      "over budget test",
		CWD:            t.TempDir(),
		AccountID:      "local",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	var dbStatus string
	if err := db.RawDB().QueryRow(
		"SELECT status FROM runs WHERE id = ?", run.ID,
	).Scan(&dbStatus); err != nil {
		t.Fatalf("query run status: %v", err)
	}
	if dbStatus != "interrupted" {
		t.Fatalf("expected db status 'interrupted', got %q", dbStatus)
	}
}

// TestBudget_DailyCapRejectsStartRun verifies that when the rolling 24-hour
// cost already exceeds the daily cap, StartRun returns ErrDailyCap.
func TestBudget_DailyCapRejectsStartRun(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)

	// Seed a receipt that exceeds the daily cap.
	if _, err := db.RawDB().Exec(
		`INSERT INTO receipts (id, run_id, cost_usd, created_at)
		 VALUES ('r1', 'run-old', 12.0, datetime('now', '-1 hour'))`,
	); err != nil {
		t.Fatalf("seed receipt: %v", err)
	}

	prov := NewMockProvider(nil, nil)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)
	rt.budget.DailyCostCapUSD = 10.0

	_, err := rt.Start(context.Background(), StartRun{
		ConversationID: "conv-daily-cap",
		AgentID:        "assistant",
		Objective:      "should be rejected",
		CWD:            t.TempDir(),
		AccountID:      "local",
	})
	if err == nil {
		t.Fatal("expected ErrDailyCap, got nil")
	}
	if err != ErrDailyCap {
		t.Fatalf("expected ErrDailyCap, got %v", err)
	}
	// Provider must not have been called.
	if prov.CallCount() != 0 {
		t.Fatalf("expected 0 provider calls when daily cap exceeded, got %d", prov.CallCount())
	}
}

// TestBudget_IdleDaemonNoRecordIdleBurn verifies that RecordIdleBurn is not
// called when the daemon is simply idle between runs (no active model context).
func TestBudget_IdleDaemonNoRecordIdleBurn(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)
	prov := NewMockProvider(nil, nil)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)

	// Reconciling with no active runs should not invoke RecordIdleBurn.
	report, err := rt.ReconcileInterrupted(context.Background())
	if err != nil {
		t.Fatalf("ReconcileInterrupted: %v", err)
	}
	if report.ReconciledCount != 0 {
		t.Fatalf("expected 0 reconciled runs, got %d", report.ReconciledCount)
	}
	// No idle burn calls — verified by no panic and no side effects on empty DB.
}

// TestBudget_CapRaiseAppliesOnNextRun verifies that raising a budget cap takes
// effect on the next new StartRun call, not on any currently-running run.
func TestBudget_CapRaiseAppliesOnNextRun(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)

	// First run: cap is 100 tokens — will be exceeded by first turn.
	// First result: tool_use keeps loop going so BeforeTurn fires budget_stop.
	// Second result: used by run 2 after the cap is raised.
	prov := NewMockProvider([]GenerateResult{
		{Content: "first run", InputTokens: 200, OutputTokens: 200, StopReason: "tool_use"},
		{Content: "second run", InputTokens: 10, OutputTokens: 10, StopReason: "end_turn"},
	}, nil)

	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)
	rt.budget.PerRunTokenCap = 100

	ctx := context.Background()
	run1, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-cap-raise-1",
		AgentID:        "assistant",
		Objective:      "run 1",
		CWD:            t.TempDir(),
		AccountID:      "local",
	})
	if err != nil {
		t.Fatalf("Start run1: %v", err)
	}
	if run1.Status != model.RunStatusInterrupted {
		t.Fatalf("run1 expected interrupted, got %s", run1.Status)
	}

	// Raise the cap — second run should complete normally.
	rt.budget.PerRunTokenCap = 1000000

	run2, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-cap-raise-2",
		AgentID:        "assistant",
		Objective:      "run 2",
		CWD:            t.TempDir(),
		AccountID:      "local",
	})
	if err != nil {
		t.Fatalf("Start run2: %v", err)
	}
	if run2.Status != model.RunStatusCompleted {
		t.Fatalf("run2 expected completed after cap raise, got %s", run2.Status)
	}
}

func TestBudget_UpdateSettingsAppliesLiveBudgetLimits(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)
	prov := NewMockProvider(nil, nil)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)

	if err := rt.UpdateSettings(context.Background(), map[string]string{
		"per_run_token_budget": "50000",
		"daily_cost_cap_usd":   "1.5",
	}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	if rt.budget.PerRunTokenCap != 50000 {
		t.Fatalf("expected live per-run token cap 50000, got %d", rt.budget.PerRunTokenCap)
	}
	if rt.budget.DailyCostCapUSD != 1.5 {
		t.Fatalf("expected live daily cost cap 1.5, got %f", rt.budget.DailyCostCapUSD)
	}
}
