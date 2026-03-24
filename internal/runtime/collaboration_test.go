package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/sessions"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

func newCollaborationRuntime(t *testing.T, responses []GenerateResult) (*Runtime, *store.DB) {
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
	rt := New(db, cs, reg, mem, NewMockProvider(responses, nil), &model.NoopEventSink{})
	return rt, db
}

func startFrontRun(t *testing.T, rt *Runtime, prompt string) model.Run {
	t.Helper()

	run, err := rt.StartFrontSession(context.Background(), StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: prompt,
		WorkspaceRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}
	return run
}

func startParentAndChildRuns(t *testing.T, rt *Runtime) (model.Run, model.Run) {
	t.Helper()

	parent := startFrontRun(t, rt, "Investigate the repo")
	child, err := rt.Spawn(context.Background(), SpawnCommand{
		ControllerSessionID: parent.SessionID,
		AgentID:             "researcher",
		Prompt:              "Inspect the docs folder.",
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	return parent, child
}

func assertRunEvent(t *testing.T, db *store.DB, runID, kind string) {
	t.Helper()

	var count int
	err := db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = ? AND kind = ?",
		runID, kind,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query %s count: %v", kind, err)
	}
	if count == 0 {
		t.Fatalf("expected %s event for run %s", kind, runID)
	}
}

func TestRuntime_StartFrontSessionCreatesFrontRunAndSession(t *testing.T) {
	rt, db := newCollaborationRuntime(t, []GenerateResult{
		{Content: "I inspected the repo.", InputTokens: 12, OutputTokens: 18, StopReason: "end_turn"},
	})

	run, err := rt.StartFrontSession(context.Background(), StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Help me inspect this repo",
		WorkspaceRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.ParentRunID != "" {
		t.Fatalf("front run must not have a parent, got %q", run.ParentRunID)
	}

	var role string
	err = db.RawDB().QueryRow(
		"SELECT role FROM sessions WHERE agent_id = 'assistant' ORDER BY created_at ASC LIMIT 1",
	).Scan(&role)
	if err != nil {
		t.Fatalf("query session role: %v", err)
	}
	if role != string(model.SessionRoleFront) {
		t.Fatalf("expected front session role, got %q", role)
	}
}

func TestRuntime_StartFrontSessionReusesExistingAssistantSession(t *testing.T) {
	rt, db := newCollaborationRuntime(t, []GenerateResult{
		{Content: "First pass complete.", InputTokens: 10, OutputTokens: 12, StopReason: "end_turn"},
		{Content: "Second pass complete.", InputTokens: 11, OutputTokens: 13, StopReason: "end_turn"},
	})

	first := startFrontRun(t, rt, "Inspect the repo")
	second := startFrontRun(t, rt, "Summarize the repo")

	if first.SessionID == "" || second.SessionID == "" {
		t.Fatal("expected front runs to carry a durable session ID")
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("expected assistant session reuse, got %s then %s", first.SessionID, second.SessionID)
	}

	var count int
	err := db.RawDB().QueryRow(
		"SELECT count(*) FROM sessions WHERE agent_id = 'assistant' AND role = 'front'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("count front sessions: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 durable front session, got %d", count)
	}

	var bindingCount int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM session_bindings WHERE thread_id = 'main' AND session_id = ?",
		first.SessionID,
	).Scan(&bindingCount)
	if err != nil {
		t.Fatalf("count session bindings: %v", err)
	}
	if bindingCount != 1 {
		t.Fatalf("expected 1 active thread binding for durable front session, got %d", bindingCount)
	}
}

func TestRuntime_SpawnCreatesWorkerRunAndSession(t *testing.T) {
	rt, db := newCollaborationRuntime(t, []GenerateResult{
		{Content: "I inspected the repo.", InputTokens: 12, OutputTokens: 18, StopReason: "end_turn"},
		{Content: "Docs reviewed.", InputTokens: 8, OutputTokens: 14, StopReason: "end_turn"},
	})
	parent := startFrontRun(t, rt, "Investigate the repo")

	child, err := rt.Spawn(context.Background(), SpawnCommand{
		ControllerSessionID: parent.SessionID,
		AgentID:             "researcher",
		Prompt:              "Inspect the docs folder.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentRunID != parent.ID {
		t.Fatalf("expected parent %s, got %s", parent.ID, child.ParentRunID)
	}

	var role string
	err = db.RawDB().QueryRow(
		"SELECT role FROM sessions WHERE agent_id = 'researcher' ORDER BY created_at ASC LIMIT 1",
	).Scan(&role)
	if err != nil {
		t.Fatalf("query worker session role: %v", err)
	}
	if role != string(model.SessionRoleWorker) {
		t.Fatalf("expected worker session role, got %q", role)
	}
}

func TestRuntime_AnnouncePersistsInterAgentMessage(t *testing.T) {
	rt, db := newCollaborationRuntime(t, []GenerateResult{
		{Content: "I inspected the repo.", InputTokens: 12, OutputTokens: 18, StopReason: "end_turn"},
		{Content: "Docs reviewed.", InputTokens: 8, OutputTokens: 14, StopReason: "end_turn"},
	})
	parent, child := startParentAndChildRuns(t, rt)

	if err := rt.Announce(context.Background(), AnnounceCommand{
		WorkerSessionID: child.SessionID,
		TargetSessionID: parent.SessionID,
		Body:            "Tests passed.",
	}); err != nil {
		t.Fatal(err)
	}

	assertRunEvent(t, db, parent.ID, "session_message_added")

	var provenanceJSON string
	err := db.RawDB().QueryRow(
		`SELECT provenance_json
		 FROM session_messages
		 WHERE session_id = ? AND kind = 'announce'
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		parent.SessionID,
	).Scan(&provenanceJSON)
	if err != nil {
		t.Fatalf("query announce provenance: %v", err)
	}
	if !strings.Contains(provenanceJSON, `"kind":"inter_session"`) || !strings.Contains(provenanceJSON, child.SessionID) {
		t.Fatalf("expected inter-session provenance for announce, got %q", provenanceJSON)
	}
}

func TestRuntime_AnnounceRejectsCrossConversationSessions(t *testing.T) {
	rt, _ := newCollaborationRuntime(t, []GenerateResult{
		{Content: "Front one ready.", InputTokens: 10, OutputTokens: 12, StopReason: "end_turn"},
		{Content: "Worker ready.", InputTokens: 8, OutputTokens: 9, StopReason: "end_turn"},
		{Content: "Front two ready.", InputTokens: 11, OutputTokens: 13, StopReason: "end_turn"},
	})

	first := startFrontRun(t, rt, "Inspect repo one")
	worker, err := rt.Spawn(context.Background(), SpawnCommand{
		ControllerSessionID: first.SessionID,
		AgentID:             "researcher",
		Prompt:              "Inspect docs.",
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	second, err := rt.StartFrontSession(context.Background(), StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant-two",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Inspect repo two",
		WorkspaceRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}

	err = rt.Announce(context.Background(), AnnounceCommand{
		WorkerSessionID: worker.SessionID,
		TargetSessionID: second.SessionID,
		Body:            "This should not cross conversations.",
	})
	if err == nil || !strings.Contains(err.Error(), "across conversations") {
		t.Fatalf("expected cross-conversation error, got %v", err)
	}
}

func TestRuntime_AnnounceRejectsSessionWithoutRun(t *testing.T) {
	rt, db := newCollaborationRuntime(t, []GenerateResult{
		{Content: "Front ready.", InputTokens: 10, OutputTokens: 12, StopReason: "end_turn"},
	})
	target := startFrontRun(t, rt, "Inspect repo")

	convStore := conversations.NewConversationStore(db)
	sessionSvc := sessions.NewService(db, convStore)
	source, err := sessionSvc.SpawnWorkerSession(context.Background(), sessions.SpawnWorkerSession{
		ConversationID:      target.ConversationID,
		ParentSessionID:     target.SessionID,
		ControllerSessionID: target.SessionID,
		AgentID:             "orphan",
		InitialPrompt:       "No run backs this session.",
	})
	if err != nil {
		t.Fatalf("SpawnWorkerSession failed: %v", err)
	}

	err = rt.Announce(context.Background(), AnnounceCommand{
		WorkerSessionID: source.ID,
		TargetSessionID: target.SessionID,
		Body:            "This should fail.",
	})
	if err == nil || !strings.Contains(err.Error(), "has no runs") {
		t.Fatalf("expected missing-run error, got %v", err)
	}
}
