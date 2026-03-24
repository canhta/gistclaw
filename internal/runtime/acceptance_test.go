package runtime

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/replay"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

func setupMilestoneTestDeps(t *testing.T) (*store.DB, *conversations.ConversationStore, *memory.Store, *tools.Registry) {
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
	return db, cs, mem, reg
}

func TestAcceptance_EndToEnd(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "I analyzed the repo and found 3 issues.", InputTokens: 100, OutputTokens: 200, StopReason: "end_turn"},
		},
		nil,
	)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)
	ctx := context.Background()

	run, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-m1-e2e",
		AgentID:        "agent-lead",
		Objective:      "Review the codebase for common Go antipatterns",
		WorkspaceRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if run.Status != model.RunStatusCompleted {
		t.Fatalf("expected status %q, got %q", model.RunStatusCompleted, run.Status)
	}

	var receiptCount int
	err = db.RawDB().QueryRow("SELECT count(*) FROM receipts WHERE run_id = ?", run.ID).Scan(&receiptCount)
	if err != nil {
		t.Fatalf("query receipts: %v", err)
	}
	if receiptCount != 1 {
		t.Fatalf("expected 1 receipt, got %d", receiptCount)
	}

	var runStarted int
	var runCompleted int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = ? AND kind = 'run_started'",
		run.ID,
	).Scan(&runStarted)
	if err != nil || runStarted != 1 {
		t.Fatalf("expected 1 run_started event, got %d (err: %v)", runStarted, err)
	}
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = ? AND kind = 'run_completed'",
		run.ID,
	).Scan(&runCompleted)
	if err != nil || runCompleted != 1 {
		t.Fatalf("expected 1 run_completed event, got %d (err: %v)", runCompleted, err)
	}

	rp := replay.NewService(db)
	runReplay, err := rp.LoadRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("LoadRun failed: %v", err)
	}
	if len(runReplay.Events) < 2 {
		t.Fatalf("expected at least 2 replay events, got %d", len(runReplay.Events))
	}

	receipt, err := rp.Build(ctx, run.ID)
	if err != nil {
		t.Fatalf("Build receipt failed: %v", err)
	}
	if receipt.InputTokens != 100 {
		t.Fatalf("expected 100 input tokens in receipt, got %d", receipt.InputTokens)
	}
}

func TestAcceptance_RestartReconciles(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)
	prov := NewMockProvider(nil, nil)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)
	ctx := context.Background()

	for i, id := range []string{"stale-run-1", "stale-run-2"} {
		_, err := db.RawDB().Exec(
			`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
			 VALUES (?, ?, 'agent-a', 'active', datetime('now'), datetime('now'))`,
			id, fmt.Sprintf("conv-stale-%d", i+1),
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
		t.Fatalf("expected 2 reconciled runs, got %d", report.ReconciledCount)
	}

	for _, id := range []string{"stale-run-1", "stale-run-2"} {
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

func TestAcceptance_MemoryReadPathExercised(t *testing.T) {
	db, cs, _, reg := setupMilestoneTestDeps(t)
	mem := memory.NewStore(db, cs)

	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "done", InputTokens: 10, OutputTokens: 20, StopReason: "end_turn"},
		},
		nil,
	)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)
	ctx := context.Background()

	run, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-mem-spy",
		AgentID:        "agent-a",
		Objective:      "memory test",
		WorkspaceRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var readEvents int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = ? AND kind = 'memory_context_loaded'",
		run.ID,
	).Scan(&readEvents)
	if err != nil {
		t.Fatalf("query memory read events: %v", err)
	}
	if readEvents == 0 {
		t.Fatal("expected memory_context_loaded event")
	}
}

func TestAcceptance_IdleDaemonMakesZeroModelCalls(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)
	prov := NewMockProvider(nil, nil)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, _ = rt.ReconcileInterrupted(ctx)
	<-ctx.Done()

	if prov.CallCount() != 0 {
		t.Fatalf("idle daemon made %d model calls, expected 0", prov.CallCount())
	}
}

func TestAcceptance_FrontSessionCanSpawnAndReceiveAnnounce(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "I will coordinate the work.", InputTokens: 12, OutputTokens: 18, StopReason: "end_turn"},
			{Content: "Docs review complete.", InputTokens: 7, OutputTokens: 10, StopReason: "end_turn"},
		},
		nil,
	)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)
	ctx := context.Background()

	front, err := rt.StartFrontSession(ctx, StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Review the docs and summarize the outcome.",
		WorkspaceRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}

	worker, err := rt.Spawn(ctx, SpawnCommand{
		ControllerRunID: front.ID,
		AgentID:         "researcher",
		Prompt:          "Inspect the docs folder.",
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	if err := rt.Announce(ctx, AnnounceCommand{
		WorkerRunID: worker.ID,
		TargetRunID: front.ID,
		Body:        "Docs review finished with three follow-ups.",
	}); err != nil {
		t.Fatalf("Announce failed: %v", err)
	}

	rp := replay.NewService(db)
	runReplay, err := rp.LoadRun(ctx, front.ID)
	if err != nil {
		t.Fatalf("LoadRun failed: %v", err)
	}
	if len(runReplay.Events) == 0 {
		t.Fatal("expected replay events for front run")
	}

	receipt, err := rp.Build(ctx, front.ID)
	if err != nil {
		t.Fatalf("Build receipt failed: %v", err)
	}
	if receipt.RunID != front.ID {
		t.Fatalf("expected receipt for %s, got %s", front.ID, receipt.RunID)
	}

	var announceCount int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM session_messages WHERE session_id = ? AND kind = 'announce'",
		front.ID,
	).Scan(&announceCount)
	if err != nil {
		t.Fatalf("query announce messages: %v", err)
	}
	if announceCount != 1 {
		t.Fatalf("expected 1 announce message on front session, got %d", announceCount)
	}
}
