package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/authority"
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

func newCollaborationRuntimeWithProviderAndTools(t *testing.T, prov *MockProvider) (*Runtime, *store.DB, *MockProvider) {
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
	reg, closer, err := tools.BuildRegistry(context.Background(), tools.BuildOptions{})
	if err != nil {
		t.Fatalf("BuildRegistry failed: %v", err)
	}
	if closer != nil {
		t.Cleanup(func() { _ = closer.Close() })
	}

	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	return rt, db, prov
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
		CWD:           t.TempDir(),
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
		CWD:           t.TempDir(),
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

func TestRuntime_DelegateTaskToolReturnsLatestAssistantMessage(t *testing.T) {
	rt, _ := newCollaborationRuntime(t, []GenerateResult{
		{Content: "Assistant ready.", InputTokens: 2, OutputTokens: 3, StopReason: "end_turn"},
		{Content: "Research findings ready.", InputTokens: 4, OutputTokens: 5, StopReason: "end_turn"},
	})
	if err := rt.SetDefaultExecutionSnapshot(model.ExecutionSnapshot{
		TeamID: "default",
		Agents: map[string]model.AgentProfile{
			"assistant": {
				AgentID:         "assistant",
				BaseProfile:     model.BaseProfileOperator,
				ToolFamilies:    []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyDelegate},
				DelegationKinds: []model.DelegationKind{model.DelegationKindResearch},
			},
			"researcher": {
				AgentID:      "researcher",
				BaseProfile:  model.BaseProfileResearch,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyWebRead},
			},
		},
	}); err != nil {
		t.Fatalf("SetDefaultExecutionSnapshot failed: %v", err)
	}

	parent := startFrontRun(t, rt, "Coordinate research")
	result, err := rt.DelegateTaskTool(context.Background(), tools.DelegateTaskRequest{
		ControllerSessionID: parent.SessionID,
		Kind:                model.DelegationKindResearch,
		Objective:           "Inspect OpenClaw",
	})
	if err != nil {
		t.Fatalf("DelegateTaskTool failed: %v", err)
	}
	if result.Output != "Research findings ready." {
		t.Fatalf("expected latest assistant message, got %q", result.Output)
	}
}

func TestSelectSpecialistForKind_PrefersSpecialtyMatch(t *testing.T) {
	t.Parallel()

	agentID, err := selectSpecialistForKind(
		map[string]model.AgentProfile{
			"market-researcher": {
				AgentID:     "market-researcher",
				BaseProfile: model.BaseProfileResearch,
				Specialties: []string{"competition", "pricing"},
			},
			"docs-researcher": {
				AgentID:     "docs-researcher",
				BaseProfile: model.BaseProfileResearch,
				Specialties: []string{"docs", "api", "messaging"},
			},
		},
		model.DelegationKindResearch,
		"Research the API docs for connector messaging flows.",
	)
	if err != nil {
		t.Fatalf("selectSpecialistForKind failed: %v", err)
	}
	if agentID != "docs-researcher" {
		t.Fatalf("expected docs-researcher, got %q", agentID)
	}
}

func TestRuntime_DelegateTaskToolReturnsBudgetInterruptionReason(t *testing.T) {
	rt, _ := newCollaborationRuntime(t, []GenerateResult{
		{Content: "Assistant ready.", InputTokens: 2, OutputTokens: 3, StopReason: "end_turn"},
		{Content: "Still reviewing.", InputTokens: 300, OutputTokens: 300, StopReason: "tool_use"},
	})
	rt.budget.PerRunTokenCap = 500
	if err := rt.SetDefaultExecutionSnapshot(model.ExecutionSnapshot{
		TeamID: "default",
		Agents: map[string]model.AgentProfile{
			"assistant": {
				AgentID:         "assistant",
				BaseProfile:     model.BaseProfileOperator,
				ToolFamilies:    []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyDelegate},
				DelegationKinds: []model.DelegationKind{model.DelegationKindResearch},
			},
			"researcher": {
				AgentID:      "researcher",
				BaseProfile:  model.BaseProfileResearch,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyWebRead},
			},
		},
	}); err != nil {
		t.Fatalf("SetDefaultExecutionSnapshot failed: %v", err)
	}

	parent := startFrontRun(t, rt, "Coordinate research")
	result, err := rt.DelegateTaskTool(context.Background(), tools.DelegateTaskRequest{
		ControllerSessionID: parent.SessionID,
		Kind:                model.DelegationKindResearch,
		Objective:           "Inspect OpenClaw",
	})
	if err != nil {
		t.Fatalf("DelegateTaskTool failed: %v", err)
	}
	if result.Status != model.RunStatusInterrupted {
		t.Fatalf("expected interrupted child run, got %s", result.Status)
	}
	if !strings.Contains(result.Output, "per-run token budget") {
		t.Fatalf("expected budget interruption reason, got %q", result.Output)
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
	alphaProject, err := ActivateProjectPath(ctx, db, alphaRoot, "alpha-project", "operator")
	if err != nil {
		t.Fatalf("activate alpha project: %v", err)
	}
	betaRoot := filepath.Join(t.TempDir(), "beta-project")
	writeRuntimeTeamFixture(t, betaRoot, "Beta Team")
	betaProject, err := ActivateProjectPath(ctx, db, betaRoot, "beta-project", "operator")
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
		CWD:             alphaRoot,
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
		CWD:             betaRoot,
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
	teamSpec := "name: " + name + "\nfront_agent: assistant\nagents:\n  - id: assistant\n    soul_file: assistant.soul.yaml\n    base_profile: operator\n    tool_families: [repo_read, delegate]\n    delegation_kinds: [research]\n    can_message: []\n"
	if err := os.WriteFile(filepath.Join(teamDir, "team.yaml"), []byte(teamSpec), 0o644); err != nil {
		t.Fatalf("write runtime team yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamDir, "assistant.soul.yaml"), []byte("role: front assistant\n"), 0o644); err != nil {
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

func TestRuntime_InspectConversationReportsActiveRunAndPendingGateSummary(t *testing.T) {
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
	_, err = db.RawDB().ExecContext(ctx, `
		INSERT INTO conversation_gates
		 (id, conversation_id, run_id, session_id, kind, status, approval_id, title, body, options_json, metadata_json, language_hint, created_at)
		VALUES ('gate-1', ?, 'run-active', 'session-front', 'approval', 'pending', 'approval-1', 'Approval required for shell_exec', 'Blocked action: touch created.txt.', '[]', '{}', 'en', datetime('now'))`,
		conv.ID,
	)
	if err != nil {
		t.Fatalf("insert conversation gate: %v", err)
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
	if status.PendingGateCount != 1 {
		t.Fatalf("expected 1 pending gate, got %d", status.PendingGateCount)
	}
	if status.ActiveGate.ID != "gate-1" {
		t.Fatalf("expected active gate gate-1, got %q", status.ActiveGate.ID)
	}
	if status.ActiveGate.Title != "Approval required for shell_exec" {
		t.Fatalf("expected active gate title to be loaded, got %q", status.ActiveGate.Title)
	}
}

func TestRuntime_StartFrontSessionIncludesDirectoryContextInProviderInstructions(t *testing.T) {
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

	projectPath := t.TempDir()
	for path, body := range map[string]string{
		"README.md": "# Front Session Repo\n",
		"go.mod":    "module example.com/front\n\ngo 1.24\n",
	} {
		abs := filepath.Join(projectPath, path)
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
		CWD:           projectPath,
	}); err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}

	if len(prov.Requests) != 1 {
		t.Fatalf("expected 1 provider request, got %d", len(prov.Requests))
	}
	instructions := prov.Requests[0].Instructions
	for _, want := range []string{"Working directory:", "README.md", "go.mod", "module example.com/front"} {
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
		CWD:           workspaceRoot,
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
		CWD:           workspaceRoot,
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
		CWD:           t.TempDir(),
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
		CWD:             workspaceRoot,
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
		CWD:             workspaceRoot,
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

func TestRuntime_ReceiveInboundMessageResolvesFrontAgentWhenUnset(t *testing.T) {
	rt, db := newCollaborationRuntime(t, []GenerateResult{
		{Content: "done", StopReason: "end_turn"},
	})
	if err := rt.SetDefaultExecutionSnapshot(model.ExecutionSnapshot{
		TeamID:       "default",
		FrontAgentID: "lead",
		Agents: map[string]model.AgentProfile{
			"lead": {
				AgentID:      "lead",
				BaseProfile:  model.BaseProfileOperator,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead},
			},
		},
	}); err != nil {
		t.Fatalf("SetDefaultExecutionSnapshot failed: %v", err)
	}

	run, err := rt.ReceiveInboundMessage(context.Background(), InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		Body:            "Inspect the repo.",
		SourceMessageID: "msg-1",
		CWD:             t.TempDir(),
	})
	if err != nil {
		t.Fatalf("ReceiveInboundMessage failed: %v", err)
	}
	if run.AgentID != "lead" {
		t.Fatalf("run agent_id = %q, want %q", run.AgentID, "lead")
	}

	var sessionAgentID string
	if err := db.RawDB().QueryRow(
		"SELECT agent_id FROM sessions WHERE id = ?",
		run.SessionID,
	).Scan(&sessionAgentID); err != nil {
		t.Fatalf("query session agent_id: %v", err)
	}
	if sessionAgentID != "lead" {
		t.Fatalf("session agent_id = %q, want %q", sessionAgentID, "lead")
	}
}

func TestRuntime_ReceiveInboundMessageRejectsRemoteConnectorWithAutoApproveElevated(t *testing.T) {
	rt, db := newCollaborationRuntime(t, []GenerateResult{
		{Content: "unsafe", StopReason: "end_turn"},
	})
	if _, err := db.RawDB().Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES
		 ('approval_mode', 'auto_approve', datetime('now')),
		 ('host_access_mode', 'elevated', datetime('now'))`,
	); err != nil {
		t.Fatalf("insert authority settings: %v", err)
	}

	_, err := rt.ReceiveInboundMessage(context.Background(), InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		FrontAgentID:    "assistant",
		Body:            "Inspect the repo.",
		SourceMessageID: "tg-1",
		CWD:             t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected remote connector run to be rejected")
	}
	if !strings.Contains(err.Error(), "auto_approve") || !strings.Contains(err.Error(), "elevated") {
		t.Fatalf("expected auto_approve + elevated rejection, got %v", err)
	}

	run, err := rt.StartFrontSession(context.Background(), StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Inspect the repo.",
		CWD:           t.TempDir(),
	})
	if err != nil {
		t.Fatalf("expected local web front session to be allowed, got %v", err)
	}

	env, err := authority.DecodeEnvelope(run.AuthorityJSON)
	if err != nil {
		t.Fatalf("DecodeEnvelope: %v", err)
	}
	if env.ApprovalMode != authority.ApprovalModeAutoApprove || env.HostAccessMode != authority.HostAccessModeElevated {
		t.Fatalf("web run authority = %+v, want auto_approve + elevated", env)
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
		CWD:             t.TempDir(),
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

func TestRuntime_ReceiveInboundMessagePromotesNaturalPromptPreferencesIntoProjectMemory(t *testing.T) {
	rt, _ := newCollaborationRuntime(t, []GenerateResult{
		{Content: "Front ready.", InputTokens: 10, OutputTokens: 12, StopReason: "end_turn"},
	})
	workspaceRoot := t.TempDir()
	ctx := context.Background()

	run, err := rt.ReceiveInboundMessage(ctx, InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID: "assistant",
		Body: "Create a small folder named demo in the active workspace for a developer-facing notes page about evaluating self-hosted assistants. " +
			"Keep the tone technical and avoid marketing fluff. " +
			"If tooling is needed, prefer bun-based workflows and keep lockfile churn isolated. " +
			"Use Codex CLI for code changes.",
		CWD: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("ReceiveInboundMessage failed: %v", err)
	}

	items, err := rt.memory.Search(ctx, model.MemoryQuery{
		ProjectID: run.ProjectID,
		AgentID:   "assistant",
		Scope:     "team",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 durable memory item from inbound natural prompt, got %d", len(items))
	}
	want := "Keep the tone technical and avoid marketing fluff. If tooling is needed, prefer bun-based workflows and keep lockfile churn isolated. Use Codex CLI for code changes."
	if got := items[0].Content; got != want {
		t.Fatalf("expected durable memory %q, got %q", want, got)
	}
	if got := items[0].Provenance; got != "prompt_preference_summary" {
		t.Fatalf("expected prompt_preference_summary provenance, got %q", got)
	}
}

func TestRuntime_ReceiveInboundMessageApprovalRequestCreatesConversationGateAndOutboundPrompt(t *testing.T) {
	prov := NewMockProvider([]GenerateResult{
		{
			ToolCalls: []model.ToolCallRequest{
				{ID: "call-touch", ToolName: "shell_exec", InputJSON: []byte(`{"command":"touch created.txt"}`)},
			},
			StopReason: "tool_calls",
		},
	}, nil)
	rt, db, _ := newCollaborationRuntimeWithProviderAndTools(t, prov)
	workspaceRoot := t.TempDir()
	if err := rt.SetDefaultExecutionSnapshot(workspaceWriteSnapshot()); err != nil {
		t.Fatalf("SetDefaultExecutionSnapshot failed: %v", err)
	}

	run, err := rt.ReceiveInboundMessage(context.Background(), InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		FrontAgentID:    "patcher",
		Body:            "Please create created.txt",
		SourceMessageID: "tg-gate-1",
		LanguageHint:    "vi",
		CWD:             workspaceRoot,
	})
	if err != nil {
		t.Fatalf("ReceiveInboundMessage failed: %v", err)
	}
	if run.Status != model.RunStatusNeedsApproval {
		t.Fatalf("expected needs_approval run, got %q", run.Status)
	}

	var gateID string
	var gateKind string
	var gateStatus string
	var approvalID string
	var languageHint string
	if err := db.RawDB().QueryRow(
		`SELECT id, kind, status, COALESCE(approval_id, ''), COALESCE(language_hint, '')
		 FROM conversation_gates
		 WHERE run_id = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		run.ID,
	).Scan(&gateID, &gateKind, &gateStatus, &approvalID, &languageHint); err != nil {
		t.Fatalf("query conversation gate: %v", err)
	}
	if gateID == "" || gateKind != "approval" || gateStatus != "pending" || approvalID == "" {
		t.Fatalf("unexpected gate row id=%q kind=%q status=%q approval_id=%q", gateID, gateKind, gateStatus, approvalID)
	}
	if languageHint != "vi" {
		t.Fatalf("expected gate language hint %q, got %q", "vi", languageHint)
	}

	var assistantPromptCount int
	if err := db.RawDB().QueryRow(
		`SELECT count(*)
		 FROM session_messages
		 WHERE session_id = ? AND kind = 'assistant'`,
		run.SessionID,
	).Scan(&assistantPromptCount); err != nil {
		t.Fatalf("count assistant gate prompts: %v", err)
	}
	if assistantPromptCount == 0 {
		t.Fatal("expected conversational gate prompt to be recorded as an assistant session message")
	}

	var outboundPromptCount int
	if err := db.RawDB().QueryRow(
		`SELECT count(*)
		 FROM outbound_intents
		 WHERE run_id = ?`,
		run.ID,
	).Scan(&outboundPromptCount); err != nil {
		t.Fatalf("count outbound prompts: %v", err)
	}
	if outboundPromptCount == 0 {
		t.Fatal("expected gate prompt to queue an outbound intent for the bound Telegram chat")
	}

	var metadataJSON string
	var messageText string
	if err := db.RawDB().QueryRow(
		`SELECT message_text, COALESCE(metadata_json, '{}')
		 FROM outbound_intents
		 WHERE run_id = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		run.ID,
	).Scan(&messageText, &metadataJSON); err != nil {
		t.Fatalf("load approval prompt metadata: %v", err)
	}
	for _, want := range []string{"Cần phê duyệt", "Phê duyệt", "Từ chối"} {
		if !strings.Contains(messageText+"\n"+metadataJSON, want) {
			t.Fatalf("expected localized approval prompt copy to include %q, got message=%q metadata=%s", want, messageText, metadataJSON)
		}
	}
	for _, want := range []string{`"action_buttons"`, `"/approve `, `"Từ chối"`, ` deny`} {
		if !strings.Contains(metadataJSON, want) {
			t.Fatalf("expected approval prompt metadata to include %q, got %s", want, metadataJSON)
		}
	}
}

func TestResolveApproval_ApprovedResolvesConversationGateForInboundTelegramRun(t *testing.T) {
	prov := NewMockProvider([]GenerateResult{
		{
			ToolCalls: []model.ToolCallRequest{
				{ID: "call-touch", ToolName: "shell_exec", InputJSON: []byte(`{"command":"touch created.txt"}`)},
			},
			StopReason: "tool_calls",
		},
		{Content: "Done.", StopReason: "end_turn"},
	}, nil)
	rt, db, _ := newCollaborationRuntimeWithProviderAndTools(t, prov)
	workspaceRoot := t.TempDir()
	if err := rt.SetDefaultExecutionSnapshot(workspaceWriteSnapshot()); err != nil {
		t.Fatalf("SetDefaultExecutionSnapshot failed: %v", err)
	}

	run, err := rt.ReceiveInboundMessage(context.Background(), InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		FrontAgentID:    "patcher",
		Body:            "Please create created.txt",
		LanguageHint:    "vi",
		SourceMessageID: "tg-gate-2",
		CWD:             workspaceRoot,
	})
	if err != nil {
		t.Fatalf("ReceiveInboundMessage failed: %v", err)
	}

	var gateID string
	var approvalID string
	if err := db.RawDB().QueryRow(
		`SELECT id, COALESCE(approval_id, '')
		 FROM conversation_gates
		 WHERE run_id = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		run.ID,
	).Scan(&gateID, &approvalID); err != nil {
		t.Fatalf("query conversation gate: %v", err)
	}
	if gateID == "" || approvalID == "" {
		t.Fatalf("expected gate and approval ids, got gate=%q approval=%q", gateID, approvalID)
	}

	if err := rt.ResolveApproval(context.Background(), approvalID, "approved"); err != nil {
		t.Fatalf("ResolveApproval failed: %v", err)
	}

	var gateStatus string
	if err := db.RawDB().QueryRow(
		`SELECT status
		 FROM conversation_gates
		 WHERE id = ?`,
		gateID,
	).Scan(&gateStatus); err != nil {
		t.Fatalf("query resolved gate status: %v", err)
	}
	if gateStatus != "resolved" {
		t.Fatalf("expected resolved gate status, got %q", gateStatus)
	}
	var localizedResolutionCount int
	if err := db.RawDB().QueryRow(
		`SELECT count(*)
		 FROM session_messages
		 WHERE session_id = ? AND body = ?`,
		run.SessionID,
		"Đã phê duyệt ngay trong chat. Đang tiếp tục tác vụ.",
	).Scan(&localizedResolutionCount); err != nil {
		t.Fatalf("count localized approval resolution messages: %v", err)
	}
	if localizedResolutionCount == 0 {
		t.Fatal("expected localized approval resolution message after resolving a Vietnamese gate")
	}
}

func TestRuntime_HandleConversationGateReplyApproveCommandBypassesResolverModel(t *testing.T) {
	prov := NewMockProvider([]GenerateResult{
		{
			ToolCalls: []model.ToolCallRequest{
				{ID: "call-touch", ToolName: "shell_exec", InputJSON: []byte(`{"command":"touch created.txt"}`)},
			},
			StopReason: "tool_calls",
		},
		{Content: "Done.", StopReason: "end_turn"},
	}, nil)
	rt, db, prov := newCollaborationRuntimeWithProviderAndTools(t, prov)
	if err := rt.SetDefaultExecutionSnapshot(workspaceWriteSnapshot()); err != nil {
		t.Fatalf("SetDefaultExecutionSnapshot failed: %v", err)
	}
	workspaceRoot := t.TempDir()

	run, err := rt.ReceiveInboundMessage(context.Background(), InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		FrontAgentID:    "patcher",
		Body:            "Please create created.txt",
		SourceMessageID: "tg-gate-3",
		CWD:             workspaceRoot,
	})
	if err != nil {
		t.Fatalf("ReceiveInboundMessage failed: %v", err)
	}

	var approvalID string
	if err := db.RawDB().QueryRow(
		`SELECT approval_id
		 FROM conversation_gates
		 WHERE run_id = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		run.ID,
	).Scan(&approvalID); err != nil {
		t.Fatalf("query gate approval id: %v", err)
	}

	outcome, err := rt.HandleConversationGateReply(context.Background(), ConversationGateReplyCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		Body:            "/approve " + approvalID + " allow-once",
		SourceMessageID: "tg-gate-4",
	})
	if err != nil {
		t.Fatalf("HandleConversationGateReply failed: %v", err)
	}
	if !outcome.Handled {
		t.Fatal("expected approve command to be consumed by the active conversation gate")
	}

	rt.WaitAsync()

	resolved, err := rt.loadRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("load resolved run: %v", err)
	}
	if resolved.Status != model.RunStatusCompleted {
		t.Fatalf("expected run to complete after approval command, got %q", resolved.Status)
	}

	if got := len(prov.Requests); got != 2 {
		t.Fatalf("expected 2 provider requests (initial run + resumed run) without gate resolver model call, got %d", got)
	}
}

func TestRuntime_HandleConversationGateReplyClarifiesAmbiguousReplyWithoutStartingNewRun(t *testing.T) {
	prov := NewMockProvider([]GenerateResult{
		{
			ToolCalls: []model.ToolCallRequest{
				{ID: "call-touch", ToolName: "shell_exec", InputJSON: []byte(`{"command":"touch created.txt"}`)},
			},
			StopReason: "tool_calls",
		},
		{
			Content:    `{"action":"clarify","confidence":"low","reply_text":"I couldn't tell whether you want to approve or deny that. Reply yes or no."}`,
			StopReason: "end_turn",
		},
	}, nil)
	rt, db, _ := newCollaborationRuntimeWithProviderAndTools(t, prov)
	if err := rt.SetDefaultExecutionSnapshot(workspaceWriteSnapshot()); err != nil {
		t.Fatalf("SetDefaultExecutionSnapshot failed: %v", err)
	}
	workspaceRoot := t.TempDir()

	run, err := rt.ReceiveInboundMessage(context.Background(), InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		FrontAgentID:    "patcher",
		Body:            "Please create created.txt",
		SourceMessageID: "tg-gate-5",
		CWD:             workspaceRoot,
	})
	if err != nil {
		t.Fatalf("ReceiveInboundMessage failed: %v", err)
	}

	outcome, err := rt.HandleConversationGateReply(context.Background(), ConversationGateReplyCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		Body:            "maybe later",
		SourceMessageID: "tg-gate-6",
	})
	if err != nil {
		t.Fatalf("HandleConversationGateReply failed: %v", err)
	}
	if !outcome.Handled {
		t.Fatal("expected ambiguous gate reply to be consumed by the active conversation gate")
	}

	refreshed, err := rt.loadRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("load refreshed run: %v", err)
	}
	if refreshed.Status != model.RunStatusNeedsApproval {
		t.Fatalf("expected run to remain blocked after ambiguous reply, got %q", refreshed.Status)
	}

	var runCount int
	if err := db.RawDB().QueryRow(`SELECT count(*) FROM runs`).Scan(&runCount); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runCount != 1 {
		t.Fatalf("expected no new run to start while clarifying a gate reply, got %d runs", runCount)
	}

	var latestAssistant string
	if err := db.RawDB().QueryRow(
		`SELECT body
		 FROM session_messages
		 WHERE session_id = ? AND kind = 'assistant'
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		run.SessionID,
	).Scan(&latestAssistant); err != nil {
		t.Fatalf("load latest assistant clarification: %v", err)
	}
	if !strings.Contains(latestAssistant, "approve or deny") {
		t.Fatalf("expected clarification message in session history, got %q", latestAssistant)
	}
}

func TestRuntime_HandleConversationGateReplyResolverPromptSupportsMultilingualReplies(t *testing.T) {
	prov := NewMockProvider([]GenerateResult{
		{
			ToolCalls: []model.ToolCallRequest{
				{ID: "call-touch", ToolName: "shell_exec", InputJSON: []byte(`{"command":"touch created.txt"}`)},
			},
			StopReason: "tool_calls",
		},
		{
			Content:    `{"action":"clarify","confidence":"low","reply_text":"Tôi chưa rõ bạn muốn duyệt hay từ chối. Bạn có thể trả lời bằng bất kỳ ngôn ngữ nào."}`,
			StopReason: "end_turn",
		},
	}, nil)
	rt, _, prov := newCollaborationRuntimeWithProviderAndTools(t, prov)
	if err := rt.SetDefaultExecutionSnapshot(workspaceWriteSnapshot()); err != nil {
		t.Fatalf("SetDefaultExecutionSnapshot failed: %v", err)
	}
	workspaceRoot := t.TempDir()

	if _, err := rt.ReceiveInboundMessage(context.Background(), InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		FrontAgentID:    "patcher",
		Body:            "Please create created.txt",
		SourceMessageID: "tg-gate-7",
		CWD:             workspaceRoot,
	}); err != nil {
		t.Fatalf("ReceiveInboundMessage failed: %v", err)
	}

	if _, err := rt.HandleConversationGateReply(context.Background(), ConversationGateReplyCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		Body:            "duyet nhe",
		SourceMessageID: "tg-gate-8",
		LanguageHint:    "vi",
	}); err != nil {
		t.Fatalf("HandleConversationGateReply failed: %v", err)
	}

	if got := len(prov.Requests); got != 2 {
		t.Fatalf("expected 2 provider requests, got %d", got)
	}
	instructions := prov.Requests[1].Instructions
	for _, want := range []string{
		"Interpret approvals and denials in any language",
		"mixed-language replies",
		"reply_text in the user's language",
		"Reply language hint: vi",
	} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("expected multilingual gate resolver prompt to include %q, got:\n%s", want, instructions)
		}
	}
}

func TestRuntime_HandleConversationGateReplyUsesLocalizedClarificationFallback(t *testing.T) {
	prov := NewMockProvider([]GenerateResult{
		{
			ToolCalls: []model.ToolCallRequest{
				{ID: "call-touch", ToolName: "shell_exec", InputJSON: []byte(`{"command":"touch created.txt"}`)},
			},
			StopReason: "tool_calls",
		},
		{
			Content:    `not-json`,
			StopReason: "end_turn",
		},
	}, nil)
	rt, db, _ := newCollaborationRuntimeWithProviderAndTools(t, prov)
	if err := rt.SetDefaultExecutionSnapshot(workspaceWriteSnapshot()); err != nil {
		t.Fatalf("SetDefaultExecutionSnapshot failed: %v", err)
	}
	workspaceRoot := t.TempDir()

	run, err := rt.ReceiveInboundMessage(context.Background(), InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		FrontAgentID:    "patcher",
		Body:            "Please create created.txt",
		LanguageHint:    "vi",
		SourceMessageID: "tg-gate-9",
		CWD:             workspaceRoot,
	})
	if err != nil {
		t.Fatalf("ReceiveInboundMessage failed: %v", err)
	}

	if _, err := rt.HandleConversationGateReply(context.Background(), ConversationGateReplyCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		Body:            "de em xem",
		LanguageHint:    "vi",
		SourceMessageID: "tg-gate-10",
	}); err != nil {
		t.Fatalf("HandleConversationGateReply failed: %v", err)
	}

	var latestAssistant string
	if err := db.RawDB().QueryRow(
		`SELECT body
		 FROM session_messages
		 WHERE session_id = ? AND kind = 'assistant'
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		run.SessionID,
	).Scan(&latestAssistant); err != nil {
		t.Fatalf("load latest assistant clarification: %v", err)
	}
	if !strings.Contains(latestAssistant, "phê duyệt hay từ chối") {
		t.Fatalf("expected localized clarification fallback, got %q", latestAssistant)
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
		CWD:           t.TempDir(),
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
		`INSERT INTO memory_items (id, project_id, agent_id, scope, content, source, created_at, updated_at)
		 VALUES ('memory-1', 'proj-memory', 'assistant', 'local', 'keep memory', 'manual', datetime('now'), datetime('now'))`,
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
