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
	"github.com/canhta/gistclaw/internal/sessions"
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
		CWD:  t.TempDir(),
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
		CWD:  t.TempDir(),
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
		CWD: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}

	worker, err := rt.Spawn(ctx, SpawnCommand{
		ControllerSessionID: front.SessionID,
		AgentID:             "researcher",
		Prompt:              "Inspect the docs folder.",
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	if err := rt.Announce(ctx, AnnounceCommand{
		WorkerSessionID: worker.SessionID,
		TargetSessionID: front.SessionID,
		Body:            "Docs review finished with three follow-ups.",
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
		front.SessionID,
	).Scan(&announceCount)
	if err != nil {
		t.Fatalf("query announce messages: %v", err)
	}
	if announceCount != 1 {
		t.Fatalf("expected 1 announce message on front session, got %d", announceCount)
	}
}

func TestAcceptance_RuntimeExposesSessionDirectoryAndHistory(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "Front ready.", InputTokens: 10, OutputTokens: 12, StopReason: "end_turn"},
			{Content: "Worker ready.", InputTokens: 8, OutputTokens: 9, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	ctx := context.Background()

	front, err := rt.StartFrontSession(ctx, StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Inspect the repo.",
		CWD: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}

	worker, err := rt.Spawn(ctx, SpawnCommand{
		ControllerSessionID: front.SessionID,
		AgentID:             "researcher",
		Prompt:              "Inspect docs.",
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	if err := rt.Announce(ctx, AnnounceCommand{
		WorkerSessionID: worker.SessionID,
		TargetSessionID: front.SessionID,
		Body:            "Docs inspected.",
	}); err != nil {
		t.Fatalf("Announce failed: %v", err)
	}

	sessionsList, err := rt.ListSessions(ctx, front.ConversationID, 10)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessionsList) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessionsList))
	}
	if sessionsList[0].ID != front.SessionID {
		t.Fatalf("expected front session first in directory, got %q", sessionsList[0].ID)
	}

	session, history, err := rt.SessionHistory(ctx, front.SessionID, 10)
	if err != nil {
		t.Fatalf("SessionHistory failed: %v", err)
	}
	if session.ID != front.SessionID {
		t.Fatalf("expected session %q, got %q", front.SessionID, session.ID)
	}
	if len(history) != 3 {
		t.Fatalf("expected 3 front-session history messages, got %d", len(history))
	}
	if history[0].Body != "Inspect the repo." || history[1].Body != "Front ready." || history[2].Body != "Docs inspected." {
		t.Fatalf("unexpected session history bodies: %q / %q / %q", history[0].Body, history[1].Body, history[2].Body)
	}
}

func TestAcceptance_FrontMailboxSpansMultipleRuns(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "First reply.", InputTokens: 10, OutputTokens: 12, StopReason: "end_turn"},
			{Content: "Second reply.", InputTokens: 11, OutputTokens: 13, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	ctx := context.Background()
	workspaceRoot := t.TempDir()

	first, err := rt.StartFrontSession(ctx, StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "First prompt",
		CWD: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("first StartFrontSession failed: %v", err)
	}

	second, err := rt.StartFrontSession(ctx, StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Second prompt",
		CWD: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("second StartFrontSession failed: %v", err)
	}

	svc := sessions.NewService(db, cs)
	session, mailbox, err := svc.LoadThreadMailbox(ctx, first.ConversationID, "main", 10)
	if err != nil {
		t.Fatalf("LoadThreadMailbox failed: %v", err)
	}
	if session.ID != first.SessionID || session.ID != second.SessionID {
		t.Fatalf("expected mailbox session %q to match durable front session %q/%q", session.ID, first.SessionID, second.SessionID)
	}
	if len(mailbox) != 4 {
		t.Fatalf("expected 4 mailbox messages, got %d", len(mailbox))
	}
	if mailbox[0].Body != "First prompt" || mailbox[1].Body != "First reply." || mailbox[2].Body != "Second prompt" || mailbox[3].Body != "Second reply." {
		t.Fatalf(
			"expected cross-run prompt/reply mailbox history, got %q / %q / %q / %q",
			mailbox[0].Body,
			mailbox[1].Body,
			mailbox[2].Body,
			mailbox[3].Body,
		)
	}
}

func TestAcceptance_FrontMailboxIncludesAssistantReplies(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "Assistant reply.", InputTokens: 10, OutputTokens: 12, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	ctx := context.Background()

	run, err := rt.StartFrontSession(ctx, StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "User prompt.",
		CWD: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}

	svc := sessions.NewService(db, cs)
	session, mailbox, err := svc.LoadThreadMailbox(ctx, run.ConversationID, "main", 10)
	if err != nil {
		t.Fatalf("LoadThreadMailbox failed: %v", err)
	}
	if session.ID != run.SessionID {
		t.Fatalf("expected mailbox session %q, got %q", run.SessionID, session.ID)
	}
	if len(mailbox) != 2 {
		t.Fatalf("expected 2 mailbox messages, got %d", len(mailbox))
	}
	if mailbox[0].Kind != model.MessageUser || mailbox[0].Body != "User prompt." {
		t.Fatalf("expected first mailbox message to be the user prompt, got kind=%q body=%q", mailbox[0].Kind, mailbox[0].Body)
	}
	if mailbox[1].Kind != model.MessageAssistant || mailbox[1].Body != "Assistant reply." {
		t.Fatalf("expected second mailbox message to be assistant reply, got kind=%q body=%q", mailbox[1].Kind, mailbox[1].Body)
	}
}

func TestAcceptance_FrontSessionQueuesOutboundIntentForExternalRoute(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "Assistant reply.", InputTokens: 10, OutputTokens: 12, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	ctx := context.Background()

	run, err := rt.StartFrontSession(ctx, StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "User prompt.",
		CWD: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}

	var connectorID string
	var chatID string
	var messageText string
	var status string
	err = db.RawDB().QueryRowContext(ctx,
		`SELECT connector_id, chat_id, message_text, status
		 FROM outbound_intents
		 WHERE run_id = ?`,
		run.ID,
	).Scan(&connectorID, &chatID, &messageText, &status)
	if err != nil {
		t.Fatalf("query outbound intent: %v", err)
	}
	if connectorID != "telegram" || chatID != "chat-1" {
		t.Fatalf("unexpected outbound target: connector_id=%q chat_id=%q", connectorID, chatID)
	}
	if messageText != "Assistant reply." || status != "pending" {
		t.Fatalf("unexpected outbound payload: message_text=%q status=%q", messageText, status)
	}
}
