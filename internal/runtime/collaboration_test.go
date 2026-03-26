package runtime

import (
	"context"
	"os"
	"path/filepath"
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

func TestRuntime_SpawnToolReturnsLatestAssistantMessage(t *testing.T) {
	rt, _ := newCollaborationRuntime(t, []GenerateResult{
		{Content: "Coordinator ready.", InputTokens: 2, OutputTokens: 3, StopReason: "end_turn"},
		{Content: "Research findings ready.", InputTokens: 4, OutputTokens: 5, StopReason: "end_turn"},
	})

	parent := startFrontRun(t, rt, "Coordinate research")
	result, err := rt.SpawnTool(context.Background(), tools.SessionSpawnRequest{
		ControllerSessionID: parent.SessionID,
		AgentID:             "researcher",
		Prompt:              "Inspect OpenClaw",
	})
	if err != nil {
		t.Fatalf("SpawnTool failed: %v", err)
	}
	if result.Output != "Research findings ready." {
		t.Fatalf("expected latest assistant message, got %q", result.Output)
	}
}

func TestRuntime_SpawnAllowsSameSpecialistTwiceFromSameControllerSession(t *testing.T) {
	rt, _ := newCollaborationRuntime(t, []GenerateResult{
		{Content: "Coordinator ready.", InputTokens: 2, OutputTokens: 3, StopReason: "end_turn"},
		{Content: "Research batch one.", InputTokens: 4, OutputTokens: 5, StopReason: "end_turn"},
		{Content: "Research batch two.", InputTokens: 4, OutputTokens: 5, StopReason: "end_turn"},
	})

	parent := startFrontRun(t, rt, "Coordinate repeated research")

	first, err := rt.Spawn(context.Background(), SpawnCommand{
		ControllerSessionID: parent.SessionID,
		AgentID:             "researcher",
		Prompt:              "Inspect OpenClaw history",
	})
	if err != nil {
		t.Fatalf("first Spawn failed: %v", err)
	}

	second, err := rt.Spawn(context.Background(), SpawnCommand{
		ControllerSessionID: parent.SessionID,
		AgentID:             "researcher",
		Prompt:              "Inspect OpenClaw build targets",
	})
	if err != nil {
		t.Fatalf("second Spawn failed: %v", err)
	}
	if first.SessionID == second.SessionID {
		t.Fatalf("expected unique worker sessions, got %q", first.SessionID)
	}
	if first.ID == second.ID {
		t.Fatalf("expected unique worker runs, got %q", first.ID)
	}
}

func TestRuntime_StartFrontSessionScopesSameConversationKeyByProject(t *testing.T) {
	rt, db := newCollaborationRuntime(t, []GenerateResult{
		{Content: "alpha done", InputTokens: 2, OutputTokens: 3, StopReason: "end_turn"},
		{Content: "beta done", InputTokens: 2, OutputTokens: 3, StopReason: "end_turn"},
	})
	ctx := context.Background()

	alphaRoot := filepath.Join(t.TempDir(), "alpha-project")
	writeRuntimeTeamFixture(t, alphaRoot, "Alpha Team")
	alphaProject, err := ActivateWorkspace(ctx, db, alphaRoot, "alpha-project", "operator")
	if err != nil {
		t.Fatalf("activate alpha project: %v", err)
	}
	betaRoot := filepath.Join(t.TempDir(), "beta-project")
	writeRuntimeTeamFixture(t, betaRoot, "Beta Team")
	betaProject, err := ActivateWorkspace(ctx, db, betaRoot, "beta-project", "operator")
	if err != nil {
		t.Fatalf("activate beta project: %v", err)
	}

	if err := SetActiveProject(ctx, db, alphaProject.ID); err != nil {
		t.Fatalf("set alpha active: %v", err)
	}
	key := conversations.ConversationKey{
		ConnectorID: "web",
		AccountID:   "local",
		ExternalID:  "assistant",
		ThreadID:    "main",
	}
	alphaRun, err := rt.StartFrontSession(ctx, StartFrontSession{
		ConversationKey: key,
		FrontAgentID:    "assistant",
		InitialPrompt:   "inspect alpha",
		WorkspaceRoot:   alphaRoot,
	})
	if err != nil {
		t.Fatalf("start alpha front session: %v", err)
	}

	if err := SetActiveProject(ctx, db, betaProject.ID); err != nil {
		t.Fatalf("set beta active: %v", err)
	}
	betaRun, err := rt.StartFrontSession(ctx, StartFrontSession{
		ConversationKey: key,
		FrontAgentID:    "assistant",
		InitialPrompt:   "inspect beta",
		WorkspaceRoot:   betaRoot,
	})
	if err != nil {
		t.Fatalf("start beta front session: %v", err)
	}

	if alphaRun.ConversationID == betaRun.ConversationID {
		t.Fatalf("expected same inbound key to resolve to different conversations per project, got %q", alphaRun.ConversationID)
	}
	if alphaRun.SessionID == betaRun.SessionID {
		t.Fatalf("expected same inbound key to resolve to different front sessions per project, got %q", alphaRun.SessionID)
	}
}

func writeRuntimeTeamFixture(t *testing.T, workspaceRoot, name string) {
	t.Helper()

	teamDir := filepath.Join(workspaceRoot, ".gistclaw", "teams", "default")
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime team dir: %v", err)
	}
	teamSpec := "name: " + name + "\nfront_agent: assistant\nagents:\n  - id: assistant\n    soul_file: assistant.soul.yaml\n    role: coordinator\n    tool_posture: read_heavy\n"
	if err := os.WriteFile(filepath.Join(teamDir, "team.yaml"), []byte(teamSpec), 0o644); err != nil {
		t.Fatalf("write runtime team yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamDir, "assistant.soul.yaml"), []byte("role: coordinator\ntool_posture: read_heavy\n"), 0o644); err != nil {
		t.Fatalf("write runtime soul: %v", err)
	}
}

func TestRuntime_LatestAssistantMessageReturnsEmptyWhenMissing(t *testing.T) {
	rt, _ := newCollaborationRuntime(t, nil)

	body, err := rt.latestAssistantMessage(context.Background(), "missing-session")
	if err != nil {
		t.Fatalf("latestAssistantMessage failed: %v", err)
	}
	if body != "" {
		t.Fatalf("expected empty body, got %q", body)
	}
}

func TestRuntime_InspectConversationReturnsMissingWhenConversationDoesNotExist(t *testing.T) {
	rt, _ := newCollaborationRuntime(t, nil)

	status, err := rt.InspectConversation(context.Background(), conversations.ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct-1",
		ExternalID:  "chat-1",
		ThreadID:    "main",
	})
	if err != nil {
		t.Fatalf("InspectConversation failed: %v", err)
	}
	if status.Exists {
		t.Fatalf("expected missing conversation, got %+v", status)
	}
}

func TestRuntime_InspectConversationReportsActiveRunAndPendingApprovals(t *testing.T) {
	rt, db := newCollaborationRuntime(t, nil)
	ctx := context.Background()

	key := conversations.ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct-1",
		ExternalID:  "chat-1",
		ThreadID:    "main",
	}
	conv, err := rt.convStore.Resolve(ctx, key)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	_, err = db.RawDB().ExecContext(ctx, `
		INSERT INTO runs (id, conversation_id, agent_id, objective, status, created_at, updated_at)
		VALUES ('run-old', ?, 'assistant', 'old task', 'completed', datetime('now', '-10 minutes'), datetime('now', '-10 minutes')),
		       ('run-active', ?, 'assistant', 'review the repo', 'active', datetime('now'), datetime('now'))`,
		conv.ID, conv.ID,
	)
	if err != nil {
		t.Fatalf("insert runs: %v", err)
	}

	_, err = db.RawDB().ExecContext(ctx, `
		INSERT INTO approvals (id, run_id, tool_name, fingerprint, status, created_at)
		VALUES ('approval-1', 'run-active', 'exec', 'fp-1', 'pending', datetime('now'))`)
	if err != nil {
		t.Fatalf("insert approval: %v", err)
	}

	status, err := rt.InspectConversation(ctx, key)
	if err != nil {
		t.Fatalf("InspectConversation failed: %v", err)
	}
	if !status.Exists {
		t.Fatal("expected conversation status to exist")
	}
	if status.ActiveRun.ID != "run-active" {
		t.Fatalf("expected active run run-active, got %q", status.ActiveRun.ID)
	}
	if status.LatestRootRun.ID != "run-active" {
		t.Fatalf("expected latest root run run-active, got %q", status.LatestRootRun.ID)
	}
	if status.PendingApprovals != 1 {
		t.Fatalf("expected 1 pending approval, got %d", status.PendingApprovals)
	}
}

func TestRuntime_StartFrontSessionIncludesWorkspaceContextInProviderInstructions(t *testing.T) {
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
	prov := NewMockProvider([]GenerateResult{
		{Content: "I inspected the repo.", InputTokens: 12, OutputTokens: 18, StopReason: "end_turn"},
	}, nil)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})

	workspaceRoot := t.TempDir()
	for path, body := range map[string]string{
		"README.md": "# Front Session Repo\n",
		"go.mod":    "module example.com/front\n\ngo 1.24\n",
	} {
		abs := filepath.Join(workspaceRoot, path)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", abs, err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", abs, err)
		}
	}

	if _, err := rt.StartFrontSession(context.Background(), StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "review the repo",
		WorkspaceRoot: workspaceRoot,
	}); err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}

	if len(prov.Requests) != 1 {
		t.Fatalf("expected 1 provider request, got %d", len(prov.Requests))
	}
	instructions := prov.Requests[0].Instructions
	for _, want := range []string{"Workspace root:", "README.md", "go.mod", "module example.com/front"} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("expected provider instructions to include %q, got:\n%s", want, instructions)
		}
	}
}

func TestRuntime_StartFrontSessionReusesExistingAssistantSession(t *testing.T) {
	rt, db := newCollaborationRuntime(t, []GenerateResult{
		{Content: "First pass complete.", InputTokens: 10, OutputTokens: 12, StopReason: "end_turn"},
		{Content: "Second pass complete.", InputTokens: 11, OutputTokens: 13, StopReason: "end_turn"},
	})
	workspaceRoot := t.TempDir()

	first, err := rt.StartFrontSession(context.Background(), StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Inspect the repo",
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("first StartFrontSession failed: %v", err)
	}
	second, err := rt.StartFrontSession(context.Background(), StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Summarize the repo",
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("second StartFrontSession failed: %v", err)
	}

	if first.SessionID == "" || second.SessionID == "" {
		t.Fatal("expected front runs to carry a durable session ID")
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("expected assistant session reuse, got %s then %s", first.SessionID, second.SessionID)
	}

	var count int
	err = db.RawDB().QueryRow(
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

func TestRuntime_SendSessionWakesFrontSessionWithNewRootRun(t *testing.T) {
	rt, db := newCollaborationRuntime(t, []GenerateResult{
		{Content: "Front ready.", InputTokens: 10, OutputTokens: 12, StopReason: "end_turn"},
		{Content: "Follow-up reply.", InputTokens: 11, OutputTokens: 13, StopReason: "end_turn"},
	})
	front := startFrontRun(t, rt, "Inspect the repo.")

	run, err := rt.SendSession(context.Background(), SendSessionCommand{
		ToSessionID: front.SessionID,
		Body:        "What changed?",
	})
	if err != nil {
		t.Fatalf("SendSession failed: %v", err)
	}
	if run.SessionID != front.SessionID {
		t.Fatalf("expected follow-up run to reuse front session %q, got %q", front.SessionID, run.SessionID)
	}
	if run.ID == front.ID {
		t.Fatalf("expected follow-up run to create a new run, got original %q", run.ID)
	}
	if run.ParentRunID != "" {
		t.Fatalf("expected front follow-up run to remain a root run, got parent %q", run.ParentRunID)
	}

	svc := sessions.NewService(db, conversations.NewConversationStore(db))
	_, history, err := svc.LoadSessionMailbox(context.Background(), front.SessionID, 10)
	if err != nil {
		t.Fatalf("LoadSessionMailbox failed: %v", err)
	}
	if len(history) != 4 {
		t.Fatalf("expected 4 front-session messages after send, got %d", len(history))
	}
	if history[2].Kind != model.MessageUser || history[2].Body != "What changed?" {
		t.Fatalf("expected user follow-up message, got kind=%q body=%q", history[2].Kind, history[2].Body)
	}
	if history[3].Kind != model.MessageAssistant || history[3].Body != "Follow-up reply." {
		t.Fatalf("expected assistant follow-up reply, got kind=%q body=%q", history[3].Kind, history[3].Body)
	}
}

func TestRuntime_SendSessionWakesWorkerSessionWithSiblingChildRun(t *testing.T) {
	rt, db := newCollaborationRuntime(t, []GenerateResult{
		{Content: "Front ready.", InputTokens: 10, OutputTokens: 12, StopReason: "end_turn"},
		{Content: "Worker ready.", InputTokens: 8, OutputTokens: 9, StopReason: "end_turn"},
		{Content: "Worker follow-up complete.", InputTokens: 7, OutputTokens: 10, StopReason: "end_turn"},
	})
	front, worker := startParentAndChildRuns(t, rt)

	run, err := rt.SendSession(context.Background(), SendSessionCommand{
		FromSessionID: front.SessionID,
		ToSessionID:   worker.SessionID,
		Body:          "Keep checking tests.",
	})
	if err != nil {
		t.Fatalf("SendSession failed: %v", err)
	}
	if run.SessionID != worker.SessionID {
		t.Fatalf("expected follow-up run to target worker session %q, got %q", worker.SessionID, run.SessionID)
	}
	if run.ParentRunID != worker.ParentRunID {
		t.Fatalf("expected worker follow-up run parent %q, got %q", worker.ParentRunID, run.ParentRunID)
	}

	svc := sessions.NewService(db, conversations.NewConversationStore(db))
	_, history, err := svc.LoadSessionMailbox(context.Background(), worker.SessionID, 10)
	if err != nil {
		t.Fatalf("LoadSessionMailbox failed: %v", err)
	}
	if len(history) != 4 {
		t.Fatalf("expected 4 worker-session messages after send, got %d", len(history))
	}
	if history[2].Kind != model.MessageAgentSend || history[2].Body != "Keep checking tests." {
		t.Fatalf("expected agent_send follow-up message, got kind=%q body=%q", history[2].Kind, history[2].Body)
	}
	if history[3].Kind != model.MessageAssistant || history[3].Body != "Worker follow-up complete." {
		t.Fatalf("expected worker follow-up reply, got kind=%q body=%q", history[3].Kind, history[3].Body)
	}
}

func TestRuntime_ReceiveInboundMessageReusesBoundFrontSessionWithConnectorProvenance(t *testing.T) {
	rt, db := newCollaborationRuntime(t, []GenerateResult{
		{Content: "Front ready.", InputTokens: 10, OutputTokens: 12, StopReason: "end_turn"},
		{Content: "Telegram follow-up.", InputTokens: 11, OutputTokens: 13, StopReason: "end_turn"},
	})
	workspaceRoot := t.TempDir()

	first, err := rt.ReceiveInboundMessage(context.Background(), InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		FrontAgentID:    "assistant",
		Body:            "Inspect the repo.",
		SourceMessageID: "tg-1",
		WorkspaceRoot:   workspaceRoot,
	})
	if err != nil {
		t.Fatalf("first ReceiveInboundMessage failed: %v", err)
	}

	second, err := rt.ReceiveInboundMessage(context.Background(), InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		FrontAgentID:    "assistant",
		Body:            "What changed?",
		SourceMessageID: "tg-2",
		WorkspaceRoot:   workspaceRoot,
	})
	if err != nil {
		t.Fatalf("second ReceiveInboundMessage failed: %v", err)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("expected bound front session reuse, got %q then %q", first.SessionID, second.SessionID)
	}
	if second.ID == first.ID {
		t.Fatalf("expected new run for inbound follow-up, got original %q", second.ID)
	}

	svc := sessions.NewService(db, conversations.NewConversationStore(db))
	_, history, err := svc.LoadSessionMailbox(context.Background(), first.SessionID, 10)
	if err != nil {
		t.Fatalf("LoadSessionMailbox failed: %v", err)
	}
	if len(history) != 4 {
		t.Fatalf("expected 4 mailbox messages, got %d", len(history))
	}
	if history[2].Kind != model.MessageUser || history[2].Body != "What changed?" {
		t.Fatalf("expected inbound follow-up user message, got kind=%q body=%q", history[2].Kind, history[2].Body)
	}
	if history[2].Provenance.SourceConnectorID != "telegram" || history[2].Provenance.SourceThreadID != "thread-1" {
		t.Fatalf("expected telegram provenance on inbound follow-up, got %+v", history[2].Provenance)
	}
	if history[2].Provenance.SourceMessageID != "tg-2" {
		t.Fatalf("expected source message ID tg-2 on inbound follow-up, got %+v", history[2].Provenance)
	}
	if history[3].Kind != model.MessageAssistant || history[3].Body != "Telegram follow-up." {
		t.Fatalf("expected assistant follow-up reply, got kind=%q body=%q", history[3].Kind, history[3].Body)
	}
}

func TestRuntime_ReceiveInboundMessageDedupesDuplicateSourceMessageID(t *testing.T) {
	rt, db := newCollaborationRuntime(t, []GenerateResult{
		{Content: "Front ready.", InputTokens: 10, OutputTokens: 12, StopReason: "end_turn"},
		{Content: "duplicate should not trigger", InputTokens: 1, OutputTokens: 1, StopReason: "end_turn"},
	})

	cmd := InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		FrontAgentID:    "assistant",
		Body:            "Inspect the repo.",
		SourceMessageID: "tg-42",
		WorkspaceRoot:   t.TempDir(),
	}

	first, err := rt.ReceiveInboundMessage(context.Background(), cmd)
	if err != nil {
		t.Fatalf("first ReceiveInboundMessage failed: %v", err)
	}
	second, err := rt.ReceiveInboundMessage(context.Background(), cmd)
	if err != nil {
		t.Fatalf("second ReceiveInboundMessage failed: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected duplicate inbound to return original run %q, got %q", first.ID, second.ID)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("expected duplicate inbound to reuse original session %q, got %q", first.SessionID, second.SessionID)
	}

	svc := sessions.NewService(db, conversations.NewConversationStore(db))
	_, history, err := svc.LoadSessionMailbox(context.Background(), first.SessionID, 10)
	if err != nil {
		t.Fatalf("LoadSessionMailbox failed: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 mailbox messages after duplicate delivery, got %d", len(history))
	}
	if history[0].Provenance.SourceMessageID != "tg-42" {
		t.Fatalf("expected source message ID to round-trip, got %+v", history[0].Provenance)
	}

	var runCount int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM runs WHERE session_id = ?",
		first.SessionID,
	).Scan(&runCount); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runCount != 1 {
		t.Fatalf("expected 1 run after duplicate delivery, got %d", runCount)
	}

	var receiptCount int
	if err := db.RawDB().QueryRow(
		`SELECT count(*) FROM inbound_receipts
		 WHERE conversation_id = ? AND source_message_id = ?`,
		first.ConversationID,
		"tg-42",
	).Scan(&receiptCount); err != nil {
		t.Fatalf("count inbound receipts: %v", err)
	}
	if receiptCount != 1 {
		t.Fatalf("expected 1 inbound receipt after duplicate delivery, got %d", receiptCount)
	}
}

func TestRuntime_RetrySessionDeliveryRequeuesTerminalIntent(t *testing.T) {
	rt, db := newCollaborationRuntime(t, []GenerateResult{
		{Content: "Front ready.", InputTokens: 10, OutputTokens: 12, StopReason: "end_turn"},
	})

	run, err := rt.StartFrontSession(context.Background(), StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Inspect the repo.",
		WorkspaceRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}

	var intentID string
	if err := db.RawDB().QueryRow(
		`SELECT id
		 FROM outbound_intents
		 WHERE run_id = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		run.ID,
	).Scan(&intentID); err != nil {
		t.Fatalf("load outbound intent: %v", err)
	}
	if _, err := db.RawDB().Exec(
		`UPDATE outbound_intents
		 SET status='terminal', attempts=3, last_attempt_at=datetime('now')
		 WHERE id = ?`,
		intentID,
	); err != nil {
		t.Fatalf("mark terminal intent: %v", err)
	}

	intent, err := rt.RetrySessionDelivery(context.Background(), run.SessionID, intentID)
	if err != nil {
		t.Fatalf("RetrySessionDelivery failed: %v", err)
	}
	if intent.ID != intentID || intent.RunID != run.ID {
		t.Fatalf("unexpected retried intent identity: %+v", intent)
	}
	if intent.Status != "pending" || intent.Attempts != 0 || intent.LastAttemptAt != nil {
		t.Fatalf("expected pending reset intent, got %+v", intent)
	}

	assertRunEvent(t, db, run.ID, "delivery_redrive_requested")
}

func TestRuntime_ResetConversationReturnsMissingWhenConversationDoesNotExist(t *testing.T) {
	rt, _ := newCollaborationRuntime(t, nil)

	outcome, err := rt.ResetConversation(context.Background(), conversations.ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct-1",
		ExternalID:  "chat-1",
		ThreadID:    "main",
	})
	if err != nil {
		t.Fatalf("ResetConversation failed: %v", err)
	}
	if outcome != ConversationResetMissing {
		t.Fatalf("expected missing outcome, got %q", outcome)
	}
}

func TestRuntime_ResetConversationReturnsBusyForActiveRun(t *testing.T) {
	rt, db := newCollaborationRuntime(t, nil)
	ctx := context.Background()

	conv, err := rt.convStore.Resolve(ctx, conversations.ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct-1",
		ExternalID:  "chat-1",
		ThreadID:    "main",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if _, err := db.RawDB().ExecContext(ctx,
		`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
		 VALUES ('run-active', ?, 'assistant', 'active', datetime('now'), datetime('now'))`,
		conv.ID,
	); err != nil {
		t.Fatalf("insert active run: %v", err)
	}

	outcome, err := rt.ResetConversation(ctx, conversations.ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct-1",
		ExternalID:  "chat-1",
		ThreadID:    "main",
	})
	if err != nil {
		t.Fatalf("ResetConversation failed: %v", err)
	}
	if outcome != ConversationResetBusy {
		t.Fatalf("expected busy outcome, got %q", outcome)
	}

	var count int
	if err := db.RawDB().QueryRowContext(ctx,
		`SELECT count(*) FROM conversations WHERE id = ?`,
		conv.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count conversations: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected busy reset to preserve conversation, got %d rows", count)
	}
}

func TestRuntime_ResetConversationClearsHistoryButPreservesMemory(t *testing.T) {
	rt, db := newCollaborationRuntime(t, nil)
	ctx := context.Background()

	conv, err := rt.convStore.Resolve(ctx, conversations.ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct-1",
		ExternalID:  "chat-1",
		ThreadID:    "main",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	for _, stmt := range []string{
		`INSERT INTO runs (id, conversation_id, agent_id, session_id, status, created_at, updated_at)
		 VALUES ('run-reset', 'CONV_ID', 'assistant', 'sess-reset', 'completed', datetime('now'), datetime('now'))`,
		`INSERT INTO sessions (id, conversation_id, key, agent_id, role, status, created_at)
		 VALUES ('sess-reset', 'CONV_ID', 'front-reset', 'assistant', 'front', 'active', datetime('now'))`,
		`INSERT INTO events (id, conversation_id, run_id, kind, payload_json, created_at)
		 VALUES ('evt-reset', 'CONV_ID', 'run-reset', 'run_started', x'7b7d', datetime('now'))`,
	} {
		if _, err := db.RawDB().ExecContext(ctx, strings.ReplaceAll(stmt, "CONV_ID", conv.ID)); err != nil {
			t.Fatalf("seed fixture: %v", err)
		}
	}
	if _, err := db.RawDB().ExecContext(ctx,
		`INSERT INTO memory_items (id, agent_id, scope, content, source, created_at, updated_at)
		 VALUES ('memory-1', 'assistant', 'local', 'keep memory', 'manual', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("insert memory: %v", err)
	}

	outcome, err := rt.ResetConversation(ctx, conversations.ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct-1",
		ExternalID:  "chat-1",
		ThreadID:    "main",
	})
	if err != nil {
		t.Fatalf("ResetConversation failed: %v", err)
	}
	if outcome != ConversationResetCleared {
		t.Fatalf("expected cleared outcome, got %q", outcome)
	}

	status, err := rt.InspectConversation(ctx, conversations.ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct-1",
		ExternalID:  "chat-1",
		ThreadID:    "main",
	})
	if err != nil {
		t.Fatalf("InspectConversation failed: %v", err)
	}
	if status.Exists {
		t.Fatalf("expected reset conversation to disappear, got %+v", status)
	}

	var memoryCount int
	if err := db.RawDB().QueryRowContext(ctx,
		`SELECT count(*) FROM memory_items WHERE id = 'memory-1'`,
	).Scan(&memoryCount); err != nil {
		t.Fatalf("count memory items: %v", err)
	}
	if memoryCount != 1 {
		t.Fatalf("expected memory to survive reset, got %d rows", memoryCount)
	}
}
