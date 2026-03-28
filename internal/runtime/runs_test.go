package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/authority"
	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

func setupRunTestDeps(t *testing.T) (*store.DB, *conversations.ConversationStore, *memory.Store, *tools.Registry) {
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

type recordingReplaySink struct {
	events []model.ReplayDelta
}

func (s *recordingReplaySink) Emit(_ context.Context, _ string, evt model.ReplayDelta) error {
	s.events = append(s.events, evt)
	return nil
}

type streamingProvider struct{}

func (p *streamingProvider) ID() string { return "streaming" }

func (p *streamingProvider) Generate(ctx context.Context, _ GenerateRequest, stream StreamSink) (GenerateResult, error) {
	if stream != nil {
		if err := stream.OnDelta(ctx, "Hel"); err != nil {
			return GenerateResult{}, err
		}
		if err := stream.OnDelta(ctx, "lo"); err != nil {
			return GenerateResult{}, err
		}
		if err := stream.OnComplete(); err != nil {
			return GenerateResult{}, err
		}
	}
	return GenerateResult{
		Content:      "Hello",
		InputTokens:  10,
		OutputTokens: 5,
		StopReason:   "end_turn",
	}, nil
}

type blockingProvider struct{}

func (p *blockingProvider) ID() string { return "blocking" }

func (p *blockingProvider) Generate(ctx context.Context, _ GenerateRequest, _ StreamSink) (GenerateResult, error) {
	<-ctx.Done()
	return GenerateResult{}, ctx.Err()
}

type cwdAwareTool struct {
	cwd string
}

func (t *cwdAwareTool) Name() string { return "cwd_aware" }

func (t *cwdAwareTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:       t.Name(),
		Family:     model.ToolFamilyRuntimeCapability,
		Risk:       model.RiskLow,
		SideEffect: "read",
		Approval:   "never",
	}
}

func (t *cwdAwareTool) Invoke(ctx context.Context, _ model.ToolCall) (model.ToolResult, error) {
	meta, ok := tools.InvocationContextFrom(ctx)
	if !ok {
		return model.ToolResult{}, fmt.Errorf("missing invocation context")
	}
	t.cwd = meta.CWD
	return model.ToolResult{Output: `{"ok":true}`}, nil
}

type authorityAwareTool struct {
	env authority.Envelope
}

func (t *authorityAwareTool) Name() string { return "authority_aware" }

func (t *authorityAwareTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:       t.Name(),
		Family:     model.ToolFamilyRuntimeCapability,
		Risk:       model.RiskLow,
		SideEffect: "read",
		Approval:   "never",
	}
}

func (t *authorityAwareTool) Invoke(ctx context.Context, _ model.ToolCall) (model.ToolResult, error) {
	meta, ok := tools.InvocationContextFrom(ctx)
	if !ok {
		return model.ToolResult{}, fmt.Errorf("missing invocation context")
	}
	t.env = meta.Authority
	return model.ToolResult{Output: `{"ok":true}`}, nil
}

type specOnlyTool struct {
	spec model.ToolSpec
}

func (t *specOnlyTool) Name() string { return t.spec.Name }

func (t *specOnlyTool) Spec() model.ToolSpec { return t.spec }

func (t *specOnlyTool) Invoke(context.Context, model.ToolCall) (model.ToolResult, error) {
	return model.ToolResult{Output: `{"ok":true}`}, nil
}

type loggingTool struct{}

func (t *loggingTool) Name() string { return "logging_tool" }

func (t *loggingTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:       t.Name(),
		Family:     model.ToolFamilyRuntimeCapability,
		Risk:       model.RiskLow,
		SideEffect: "read",
		Approval:   "never",
	}
}

func (t *loggingTool) Invoke(ctx context.Context, _ model.ToolCall) (model.ToolResult, error) {
	meta, ok := tools.InvocationContextFrom(ctx)
	if !ok || meta.LogSink == nil {
		return model.ToolResult{}, fmt.Errorf("missing tool log sink")
	}
	if err := meta.LogSink.Record(ctx, tools.ToolLogRecord{
		Stream:     "stdout",
		Text:       "planning files\n",
		OccurredAt: time.Date(2026, time.March, 26, 4, 0, 0, 0, time.UTC),
	}); err != nil {
		return model.ToolResult{}, err
	}
	if err := meta.LogSink.Record(ctx, tools.ToolLogRecord{
		Stream:     "stderr",
		Text:       "warning: fallback path\n",
		OccurredAt: time.Date(2026, time.March, 26, 4, 0, 1, 0, time.UTC),
	}); err != nil {
		return model.ToolResult{}, err
	}
	return model.ToolResult{Output: `{"ok":true}`}, nil
}

func TestRunEngine_StartAndComplete(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "task completed", InputTokens: 50, OutputTokens: 100, StopReason: "end_turn"},
		},
		nil,
	)
	sink := &model.NoopEventSink{}

	rt := New(db, cs, reg, mem, prov, sink)
	ctx := context.Background()

	run, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-1",
		AgentID:        "agent-a",
		Objective:      "test task",
		CWD:            t.TempDir(),
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
}

func TestRunEngine_FailsHungProviderTurnsAfterTimeout(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	rt := New(db, cs, reg, mem, &blockingProvider{}, &model.NoopEventSink{})
	rt.providerTimeout = 20 * time.Millisecond

	started := time.Now()
	run, err := rt.Start(context.Background(), StartRun{
		ConversationID: "conv-hung-provider",
		AgentID:        "assistant",
		Objective:      "coordinate a task",
		CWD:            t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected provider timeout error")
	}
	if time.Since(started) > 250*time.Millisecond {
		t.Fatalf("expected hung provider to time out quickly, took %s", time.Since(started))
	}
	if run.Status != model.RunStatusFailed {
		t.Fatalf("expected failed run after provider timeout, got %s", run.Status)
	}

	var failedEvents int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = ? AND kind = 'run_failed'",
		run.ID,
	).Scan(&failedEvents); err != nil {
		t.Fatalf("query run_failed events: %v", err)
	}
	if failedEvents != 1 {
		t.Fatalf("expected 1 run_failed event, got %d", failedEvents)
	}
}

func TestRunEngine_AdvertisesOnlyAllowedToolsForCurrentAgent(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	reg.Register(&specOnlyTool{spec: model.ToolSpec{Name: "session_spawn", Family: model.ToolFamilyDelegate, Risk: model.RiskLow}})
	reg.Register(&specOnlyTool{spec: model.ToolSpec{Name: "write_new_file", Family: model.ToolFamilyRepoWrite, Risk: model.RiskMedium, SideEffect: "create"}})
	reg.Register(&specOnlyTool{spec: model.ToolSpec{Name: "coder_exec", Family: model.ToolFamilyRepoWrite, Risk: model.RiskHigh, SideEffect: "exec_write"}})
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "delegating", InputTokens: 3, OutputTokens: 5, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})

	run, err := rt.Start(context.Background(), StartRun{
		ConversationID: "conv-visible-tools",
		AgentID:        "assistant",
		Objective:      "coordinate the task",
		CWD:            t.TempDir(),
		ExecutionSnapshotJSON: mustSnapshotJSON(t, model.ExecutionSnapshot{
			TeamID: "default",
			Agents: map[string]model.AgentProfile{
				"assistant": {
					AgentID:         "assistant",
					BaseProfile:     model.BaseProfileOperator,
					ToolFamilies:    []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyDelegate},
					DelegationKinds: []model.DelegationKind{model.DelegationKindResearch},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if run.Status != model.RunStatusCompleted {
		t.Fatalf("expected completed run, got %s", run.Status)
	}

	if len(prov.Requests) != 1 {
		t.Fatalf("expected 1 provider request, got %d", len(prov.Requests))
	}

	gotNames := make(map[string]bool, len(prov.Requests[0].ToolSpecs))
	for _, spec := range prov.Requests[0].ToolSpecs {
		gotNames[spec.Name] = true
	}
	if !gotNames["session_spawn"] {
		t.Fatalf("expected session_spawn to be visible, got %+v", gotNames)
	}
	if gotNames["write_new_file"] {
		t.Fatalf("expected write_new_file to be hidden, got %+v", gotNames)
	}
	if gotNames["coder_exec"] {
		t.Fatalf("expected coder_exec to be hidden from read_heavy assistant, got %+v", gotNames)
	}
}

func TestRunEngine_AdvertisesCoderExecToScopedWriteAgent(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	reg.Register(&specOnlyTool{spec: model.ToolSpec{Name: "coder_exec", Family: model.ToolFamilyRepoWrite, Risk: model.RiskHigh, SideEffect: "exec_write"}})
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "patch complete", InputTokens: 3, OutputTokens: 5, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})

	run, err := rt.Start(context.Background(), StartRun{
		ConversationID: "conv-patcher-tools",
		AgentID:        "patcher",
		Objective:      "write the change",
		CWD:            t.TempDir(),
		ExecutionSnapshotJSON: mustSnapshotJSON(t, model.ExecutionSnapshot{
			TeamID: "default",
			Agents: map[string]model.AgentProfile{
				"patcher": {
					AgentID:      "patcher",
					BaseProfile:  model.BaseProfileWrite,
					ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyRepoWrite},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if run.Status != model.RunStatusCompleted {
		t.Fatalf("expected completed run, got %s", run.Status)
	}
	if len(prov.Requests) != 1 {
		t.Fatalf("expected 1 provider request, got %d", len(prov.Requests))
	}

	gotNames := make(map[string]bool, len(prov.Requests[0].ToolSpecs))
	for _, spec := range prov.Requests[0].ToolSpecs {
		gotNames[spec.Name] = true
	}
	if !gotNames["coder_exec"] {
		t.Fatalf("expected coder_exec to be visible to patcher, got %+v", gotNames)
	}
}

func TestRunEngine_SessionSpawnToolCreatesChildRun(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{
				Content: "I will delegate this to patcher.",
				ToolCalls: []model.ToolCallRequest{
					{
						ID:       "call-spawn",
						ToolName: "session_spawn",
						InputJSON: []byte(`{
							"agent_id":"researcher",
							"prompt":"Research OpenClaw and report back."
						}`),
					},
				},
				InputTokens:  4,
				OutputTokens: 6,
			},
			{Content: "OpenClaw uses first-class session tools.", InputTokens: 5, OutputTokens: 9, StopReason: "end_turn"},
			{Content: "Research complete.", InputTokens: 6, OutputTokens: 10, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	tools.RegisterCollaborationTools(reg, tools.CollaborationHandlers{
		Spawn: rt.SpawnTool,
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

	run, err := rt.StartFrontSession(context.Background(), StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Coordinate research.",
		CWD:           t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}
	if run.Status != model.RunStatusCompleted {
		t.Fatalf("expected completed run, got %s", run.Status)
	}

	var childCount int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM runs WHERE parent_run_id = ? AND agent_id = 'researcher'",
		run.ID,
	).Scan(&childCount)
	if err != nil {
		t.Fatalf("query child runs: %v", err)
	}
	if childCount != 1 {
		t.Fatalf("expected 1 child researcher run, got %d", childCount)
	}
}

func TestRunEngine_DelegateTaskToolCreatesChildRun(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{
				Content: "I will delegate this research.",
				ToolCalls: []model.ToolCallRequest{
					{
						ID:        "call-delegate",
						ToolName:  "delegate_task",
						InputJSON: []byte(`{"kind":"research","objective":"Research OpenClaw and report back."}`),
					},
				},
				InputTokens:  4,
				OutputTokens: 6,
			},
			{Content: "OpenClaw uses first-class session tools.", InputTokens: 5, OutputTokens: 9, StopReason: "end_turn"},
			{Content: "Research complete.", InputTokens: 6, OutputTokens: 10, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	tools.RegisterCollaborationTools(reg, tools.CollaborationHandlers{
		Spawn:        rt.SpawnTool,
		DelegateTask: rt.DelegateTaskTool,
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

	run, err := rt.StartFrontSession(context.Background(), StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Research OpenClaw and summarize the result.",
		CWD:           t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}
	if run.Status != model.RunStatusCompleted {
		t.Fatalf("expected completed run, got %s", run.Status)
	}

	var childCount int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM runs WHERE parent_run_id = ? AND agent_id = 'researcher'",
		run.ID,
	).Scan(&childCount)
	if err != nil {
		t.Fatalf("query child runs: %v", err)
	}
	if childCount != 1 {
		t.Fatalf("expected 1 child researcher run, got %d", childCount)
	}
}

func TestRunEngine_SessionSpawnIsDeniedWhenRuntimeRecommendsDirect(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{
				Content: "I will delegate this.",
				ToolCalls: []model.ToolCallRequest{
					{
						ID:       "call-spawn",
						ToolName: "session_spawn",
						InputJSON: []byte(`{
							"agent_id":"researcher",
							"prompt":"Research OpenClaw and report back."
						}`),
					},
				},
				InputTokens:  4,
				OutputTokens: 6,
			},
			{Content: "I handled it directly.", InputTokens: 5, OutputTokens: 8, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	tools.RegisterCollaborationTools(reg, tools.CollaborationHandlers{
		Spawn: rt.SpawnTool,
	})

	if err := rt.SetDefaultExecutionSnapshot(model.ExecutionSnapshot{
		TeamID: "default",
		Agents: map[string]model.AgentProfile{
			"assistant": {
				AgentID:                     "assistant",
				BaseProfile:                 model.BaseProfileOperator,
				ToolFamilies:                []model.ToolFamily{model.ToolFamilyConnectorCapability, model.ToolFamilyDelegate},
				DelegationKinds:             []model.DelegationKind{model.DelegationKindResearch},
				SpecialistSummaryVisibility: model.SpecialistSummaryFull,
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

	run, err := rt.StartFrontSession(context.Background(), StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "List my Zalo contacts and send hello to Anh on Zalo.",
		CWD:           t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}
	if run.Status != model.RunStatusCompleted {
		t.Fatalf("expected completed run, got %s", run.Status)
	}

	var childCount int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM runs WHERE parent_run_id = ?",
		run.ID,
	).Scan(&childCount); err != nil {
		t.Fatalf("query child runs: %v", err)
	}
	if childCount != 0 {
		t.Fatalf("expected no child runs when delegation is denied, got %d", childCount)
	}

	var payload string
	if err := db.RawDB().QueryRow(
		`SELECT payload_json
		 FROM events
		 WHERE run_id = ?
		   AND kind = 'tool_call_recorded'
		   AND json_extract(payload_json, '$.tool_name') = 'session_spawn'
		 ORDER BY created_at ASC, id ASC
		 LIMIT 1`,
		run.ID,
	).Scan(&payload); err != nil {
		t.Fatalf("query session_spawn tool event: %v", err)
	}
	if !strings.Contains(payload, "runtime recommends direct execution") {
		t.Fatalf("expected session_spawn denial payload to mention direct execution, got %q", payload)
	}
}

func TestRunEngine_SessionSpawnPausesParentUntilApprovedChildCompletes(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	cs := conversations.NewConversationStore(db)
	mem := memory.NewStore(db, cs)
	reg, closer, err := tools.BuildRegistry(context.Background(), tools.BuildOptions{})
	if err != nil {
		t.Fatalf("BuildRegistry failed: %v", err)
	}
	if closer != nil {
		t.Cleanup(func() { _ = closer.Close() })
	}

	prov := NewMockProvider(
		[]GenerateResult{
			{
				ToolCalls: []model.ToolCallRequest{
					{
						ID:       "call-spawn",
						ToolName: "session_spawn",
						InputJSON: []byte(`{
							"agent_id":"patcher",
							"prompt":"Create created.txt in the workspace."
						}`),
					},
				},
				InputTokens:  4,
				OutputTokens: 6,
			},
			{
				ToolCalls: []model.ToolCallRequest{
					{
						ID:        "call-shell",
						ToolName:  "shell_exec",
						InputJSON: []byte(`{"command":"touch created.txt"}`),
					},
				},
				InputTokens:  5,
				OutputTokens: 7,
			},
			{Content: "Created file.", InputTokens: 6, OutputTokens: 8, StopReason: "end_turn"},
			{Content: "QA can review now.", InputTokens: 7, OutputTokens: 9, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	tools.RegisterCollaborationTools(reg, tools.CollaborationHandlers{
		Spawn: rt.SpawnTool,
	})

	if err := rt.SetDefaultExecutionSnapshot(model.ExecutionSnapshot{
		TeamID: "default",
		Agents: map[string]model.AgentProfile{
			"assistant": {
				AgentID:         "assistant",
				BaseProfile:     model.BaseProfileOperator,
				ToolFamilies:    []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyDelegate},
				DelegationKinds: []model.DelegationKind{model.DelegationKindWrite},
			},
			"patcher": {
				AgentID:      "patcher",
				BaseProfile:  model.BaseProfileWrite,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyRepoWrite},
			},
		},
	}); err != nil {
		t.Fatalf("SetDefaultExecutionSnapshot failed: %v", err)
	}

	workspaceRoot := t.TempDir()
	parent, err := rt.StartFrontSession(context.Background(), StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Implement the workspace write by delegating the file creation to patcher.",
		CWD:           workspaceRoot,
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}
	if parent.Status != model.RunStatusActive {
		t.Fatalf("expected parent run to stay active while child waits on approval, got %s", parent.Status)
	}
	if len(prov.Requests) != 2 {
		t.Fatalf("expected 2 provider requests before approval, got %d", len(prov.Requests))
	}

	var assistantMessagesBeforeApproval int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM session_messages WHERE session_id = ? AND kind = ?",
		parent.SessionID,
		model.MessageAssistant,
	).Scan(&assistantMessagesBeforeApproval); err != nil {
		t.Fatalf("query assistant session messages: %v", err)
	}
	if assistantMessagesBeforeApproval != 0 {
		t.Fatalf("expected no assistant reply before spawned work finishes, got %d messages", assistantMessagesBeforeApproval)
	}

	var completedEvents int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = ? AND kind = 'run_completed'",
		parent.ID,
	).Scan(&completedEvents); err != nil {
		t.Fatalf("query parent completion events: %v", err)
	}
	if completedEvents != 0 {
		t.Fatalf("expected parent run to stay incomplete before approval, got %d completion events", completedEvents)
	}

	var childID string
	if err := db.RawDB().QueryRow(
		"SELECT id FROM runs WHERE parent_run_id = ? ORDER BY created_at ASC LIMIT 1",
		parent.ID,
	).Scan(&childID); err != nil {
		t.Fatalf("query child run: %v", err)
	}

	var ticketID string
	if err := db.RawDB().QueryRow(
		"SELECT id FROM approvals WHERE run_id = ? AND status = 'pending' ORDER BY created_at ASC LIMIT 1",
		childID,
	).Scan(&ticketID); err != nil {
		t.Fatalf("query child approval: %v", err)
	}

	if err := rt.ResolveApproval(context.Background(), ticketID, "approved"); err != nil {
		t.Fatalf("ResolveApproval approved: %v", err)
	}

	parent, err = rt.loadRun(context.Background(), parent.ID)
	if err != nil {
		t.Fatalf("reload parent run: %v", err)
	}
	if parent.Status != model.RunStatusCompleted {
		t.Fatalf("expected parent run completed after child approval, got %s", parent.Status)
	}

	child, err := rt.loadRun(context.Background(), childID)
	if err != nil {
		t.Fatalf("reload child run: %v", err)
	}
	if child.Status != model.RunStatusCompleted {
		t.Fatalf("expected child run completed after approval, got %s", child.Status)
	}

	if _, err := os.Stat(filepath.Join(workspaceRoot, "created.txt")); err != nil {
		t.Fatalf("expected approved child command to create file: %v", err)
	}

	if len(prov.Requests) != 4 {
		t.Fatalf("expected 4 provider requests after child completion resumes parent, got %d", len(prov.Requests))
	}

	var parentToolCallEvents int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = ? AND kind = 'tool_call_recorded'",
		parent.ID,
	).Scan(&parentToolCallEvents); err != nil {
		t.Fatalf("query parent tool_call_recorded events: %v", err)
	}
	if parentToolCallEvents < 2 {
		t.Fatalf("expected parent run to record an updated session_spawn result, got %d tool_call_recorded events", parentToolCallEvents)
	}

	var sawWorkerResult bool
	var spawnToolCallCount int
	for _, evt := range prov.Requests[3].ConversationCtx {
		switch evt.Kind {
		case "session_message_added":
			var payload struct {
				Body string `json:"body"`
			}
			if err := json.Unmarshal(evt.PayloadJSON, &payload); err != nil {
				t.Fatalf("unmarshal resumed parent mailbox event: %v", err)
			}
			if strings.Contains(payload.Body, "Created file.") {
				sawWorkerResult = true
			}
		case "tool_call_recorded":
			var payload struct {
				ToolName   string          `json:"tool_name"`
				OutputJSON json.RawMessage `json:"output_json"`
			}
			if err := json.Unmarshal(evt.PayloadJSON, &payload); err != nil {
				t.Fatalf("unmarshal resumed parent tool payload: %v", err)
			}
			if payload.ToolName != "session_spawn" {
				continue
			}
			spawnToolCallCount++
			var toolResult struct {
				Output string `json:"output"`
				Error  string `json:"error"`
			}
			if err := json.Unmarshal(payload.OutputJSON, &toolResult); err != nil {
				t.Fatalf("unmarshal resumed session_spawn tool result: %v", err)
			}
			var output struct {
				Status model.RunStatus `json:"status"`
				Output string          `json:"output"`
			}
			if err := json.Unmarshal([]byte(toolResult.Output), &output); err != nil {
				t.Fatalf("unmarshal resumed session_spawn output: %v", err)
			}
			if output.Status != model.RunStatusCompleted {
				t.Fatalf(
					"expected resumed session_spawn status %q, got %q (tool result=%s)",
					model.RunStatusCompleted,
					output.Status,
					toolResult.Output,
				)
			}
			if !strings.Contains(output.Output, "Created file.") {
				t.Fatalf("expected resumed session_spawn output to include child result, got %q", output.Output)
			}
		}
	}
	if !sawWorkerResult {
		t.Fatalf("expected resumed parent context to include child result, got %+v", prov.Requests[3].ConversationCtx)
	}
	if spawnToolCallCount != 1 {
		t.Fatalf("expected exactly 1 session_spawn tool result in resumed context, got %d", spawnToolCallCount)
	}
}

func TestRunEngine_SessionSpawnInterruptsParentWhenChildInterrupts(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	cs := conversations.NewConversationStore(db)
	mem := memory.NewStore(db, cs)
	reg, closer, err := tools.BuildRegistry(context.Background(), tools.BuildOptions{})
	if err != nil {
		t.Fatalf("BuildRegistry failed: %v", err)
	}
	if closer != nil {
		t.Cleanup(func() { _ = closer.Close() })
	}

	prov := NewMockProvider(
		[]GenerateResult{
			{
				ToolCalls: []model.ToolCallRequest{
					{
						ID:       "call-spawn",
						ToolName: "session_spawn",
						InputJSON: []byte(`{
							"agent_id":"patcher",
							"prompt":"Run the missing binary in the workspace."
						}`),
					},
				},
				InputTokens:  4,
				OutputTokens: 6,
			},
			{
				ToolCalls: []model.ToolCallRequest{
					{
						ID:        "call-shell",
						ToolName:  "shell_exec",
						InputJSON: []byte(`{"command":"missing-command created.txt"}`),
					},
				},
				InputTokens:  5,
				OutputTokens: 7,
			},
			{Content: "This should not be emitted.", InputTokens: 6, OutputTokens: 8, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	tools.RegisterCollaborationTools(reg, tools.CollaborationHandlers{
		Spawn: rt.SpawnTool,
	})

	if err := rt.SetDefaultExecutionSnapshot(model.ExecutionSnapshot{
		TeamID: "default",
		Agents: map[string]model.AgentProfile{
			"assistant": {
				AgentID:         "assistant",
				BaseProfile:     model.BaseProfileOperator,
				ToolFamilies:    []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyDelegate},
				DelegationKinds: []model.DelegationKind{model.DelegationKindWrite},
			},
			"patcher": {
				AgentID:      "patcher",
				BaseProfile:  model.BaseProfileWrite,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyRepoWrite},
			},
		},
	}); err != nil {
		t.Fatalf("SetDefaultExecutionSnapshot failed: %v", err)
	}

	parent, err := rt.StartFrontSession(context.Background(), StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Implement the workspace write by delegating the missing-binary check to patcher.",
		CWD:           t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}
	if parent.Status != model.RunStatusActive {
		t.Fatalf("expected parent run to stay active while child waits on approval, got %s", parent.Status)
	}

	var childID string
	if err := db.RawDB().QueryRow(
		"SELECT id FROM runs WHERE parent_run_id = ? ORDER BY created_at ASC LIMIT 1",
		parent.ID,
	).Scan(&childID); err != nil {
		t.Fatalf("query child run: %v", err)
	}

	var ticketID string
	if err := db.RawDB().QueryRow(
		"SELECT id FROM approvals WHERE run_id = ? AND status = 'pending' ORDER BY created_at ASC LIMIT 1",
		childID,
	).Scan(&ticketID); err != nil {
		t.Fatalf("query child approval: %v", err)
	}

	if err := rt.ResolveApproval(context.Background(), ticketID, "approved"); err != nil {
		t.Fatalf("ResolveApproval approved: %v", err)
	}

	parent, err = rt.loadRun(context.Background(), parent.ID)
	if err != nil {
		t.Fatalf("reload parent run: %v", err)
	}
	if parent.Status != model.RunStatusInterrupted {
		t.Fatalf("expected parent run interrupted after child interruption, got %s", parent.Status)
	}

	child, err := rt.loadRun(context.Background(), childID)
	if err != nil {
		t.Fatalf("reload child run: %v", err)
	}
	if child.Status != model.RunStatusInterrupted {
		t.Fatalf("expected child run interrupted after approved tool error, got %s", child.Status)
	}

	var completedEvents int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = ? AND kind = 'run_completed'",
		parent.ID,
	).Scan(&completedEvents); err != nil {
		t.Fatalf("query parent completion events: %v", err)
	}
	if completedEvents != 0 {
		t.Fatalf("expected parent run to stay incomplete after child interruption, got %d completion events", completedEvents)
	}

	if len(prov.Requests) != 2 {
		t.Fatalf("expected provider to not resume parent after child interruption, got %d requests", len(prov.Requests))
	}
}

func TestRunEngine_IncludesSoulInstructionsInProviderRequests(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "Coordinator ready.", InputTokens: 5, OutputTokens: 7, StopReason: "end_turn"},
			{Content: "Research findings ready.", InputTokens: 6, OutputTokens: 8, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})

	if err := rt.SetDefaultExecutionSnapshot(model.ExecutionSnapshot{
		TeamID: "default",
		Agents: map[string]model.AgentProfile{
			"assistant": {
				AgentID:         "assistant",
				Role:            "operator-facing coordinator",
				Instructions:    "must route external research through researcher",
				BaseProfile:     model.BaseProfileOperator,
				ToolFamilies:    []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyDelegate},
				DelegationKinds: []model.DelegationKind{model.DelegationKindResearch},
			},
			"researcher": {
				AgentID:      "researcher",
				Role:         "research specialist",
				Instructions: "prefer primary sources and return concise findings",
				BaseProfile:  model.BaseProfileResearch,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyWebRead},
			},
		},
	}); err != nil {
		t.Fatalf("SetDefaultExecutionSnapshot failed: %v", err)
	}

	parent, err := rt.StartFrontSession(context.Background(), StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Coordinate research.",
		CWD:           t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}

	child, err := rt.Spawn(context.Background(), SpawnCommand{
		ControllerSessionID: parent.SessionID,
		AgentID:             "researcher",
		Prompt:              "Inspect OpenClaw.",
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	if child.Status != model.RunStatusCompleted {
		t.Fatalf("expected child run completed, got %s", child.Status)
	}
	if len(prov.Requests) != 2 {
		t.Fatalf("expected 2 provider requests, got %d", len(prov.Requests))
	}

	for _, want := range []string{
		"operator-facing coordinator",
		"must route external research through researcher",
		"Delegation kinds: research",
	} {
		if !strings.Contains(prov.Requests[0].Instructions, want) {
			t.Fatalf("expected front instructions to include %q, got:\n%s", want, prov.Requests[0].Instructions)
		}
	}
	for _, want := range []string{
		"research specialist",
		"prefer primary sources and return concise findings",
	} {
		if !strings.Contains(prov.Requests[1].Instructions, want) {
			t.Fatalf("expected child instructions to include %q, got:\n%s", want, prov.Requests[1].Instructions)
		}
	}
}

func TestRunEngine_IncludesExecutionRecommendationAndSpecialistRosterInProviderRequests(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "done", InputTokens: 4, OutputTokens: 6, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})

	if err := rt.SetDefaultExecutionSnapshot(model.ExecutionSnapshot{
		TeamID: "default",
		Agents: map[string]model.AgentProfile{
			"assistant": {
				AgentID:                     "assistant",
				Role:                        "front assistant",
				Instructions:                "prefer direct execution before delegation",
				BaseProfile:                 model.BaseProfileOperator,
				ToolFamilies:                []model.ToolFamily{model.ToolFamilyConnectorCapability, model.ToolFamilyDelegate},
				DelegationKinds:             []model.DelegationKind{model.DelegationKindResearch, model.DelegationKindWrite},
				SpecialistSummaryVisibility: model.SpecialistSummaryFull,
			},
			"patcher": {
				AgentID:      "patcher",
				Role:         "scoped write specialist",
				BaseProfile:  model.BaseProfileWrite,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyRepoWrite},
			},
			"researcher": {
				AgentID:      "researcher",
				Role:         "research specialist",
				BaseProfile:  model.BaseProfileResearch,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyWebRead},
			},
		},
	}); err != nil {
		t.Fatalf("SetDefaultExecutionSnapshot failed: %v", err)
	}

	if _, err := rt.Start(context.Background(), StartRun{
		ConversationID: "conv-recommendation",
		AgentID:        "assistant",
		Objective:      "List my Zalo contacts and send hello to Anh on Zalo.",
		CWD:            t.TempDir(),
	}); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if len(prov.Requests) != 1 {
		t.Fatalf("expected 1 provider request, got %d", len(prov.Requests))
	}

	instructions := prov.Requests[0].Instructions
	for _, want := range []string{
		"Execution recommendation:",
		"Mode: direct",
		"Rationale:",
		"Specialists available:",
		"patcher",
		"researcher",
		"front assistant",
	} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("expected provider instructions to include %q, got:\n%s", want, instructions)
		}
	}
}

func TestRunEngine_ApprovalRequestedEmitsReplayDelta(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	cs := conversations.NewConversationStore(db)
	mem := memory.NewStore(db, cs)
	reg, closer, err := tools.BuildRegistry(context.Background(), tools.BuildOptions{})
	if err != nil {
		t.Fatalf("BuildRegistry failed: %v", err)
	}
	if closer != nil {
		t.Cleanup(func() { _ = closer.Close() })
	}
	sink := &recordingReplaySink{}
	prov := NewMockProvider([]GenerateResult{
		{
			ToolCalls: []model.ToolCallRequest{
				{ID: "call-shell", ToolName: "shell_exec", InputJSON: []byte(`{"command":"touch created.txt"}`)},
			},
			StopReason: "tool_calls",
		},
	}, nil)
	rt := New(db, cs, reg, mem, prov, sink)

	run, err := rt.Start(context.Background(), StartRun{
		ConversationID: "conv-approval-replay",
		AgentID:        "patcher",
		Objective:      "mutate shell",
		CWD:            t.TempDir(),
		ExecutionSnapshotJSON: mustSnapshotJSON(t, model.ExecutionSnapshot{
			TeamID: "default",
			Agents: map[string]model.AgentProfile{
				"patcher": {
					AgentID:      "patcher",
					BaseProfile:  model.BaseProfileWrite,
					ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyRepoWrite},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if run.Status != model.RunStatusNeedsApproval {
		t.Fatalf("expected needs_approval, got %s", run.Status)
	}

	for _, evt := range sink.events {
		if evt.Kind == "approval_requested" {
			return
		}
	}
	t.Fatalf("expected replay sink to emit approval_requested, got %+v", sink.events)
}

func TestRunEngine_ContinueAndResumeLoadRun(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "done", InputTokens: 5, OutputTokens: 7, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	ctx := context.Background()

	run, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-continue",
		AgentID:        "agent-a",
		Objective:      "finish task",
		CWD:            t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	continued, err := rt.Continue(ctx, ContinueRun{RunID: run.ID})
	if err != nil {
		t.Fatalf("Continue failed: %v", err)
	}
	if continued.ID != run.ID || continued.Status != model.RunStatusCompleted {
		t.Fatalf("unexpected continued run: %+v", continued)
	}

	resumed, err := rt.Resume(ctx, ResumeRun{RunID: run.ID})
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if resumed.ID != run.ID || resumed.Status != model.RunStatusCompleted {
		t.Fatalf("unexpected resumed run: %+v", resumed)
	}
}

func TestRunEngine_SubmitTaskStartsWebConversation(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "done", InputTokens: 5, OutputTokens: 7, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	ctx := context.Background()

	run, err := rt.SubmitTask(ctx, "review repository", t.TempDir())
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}
	if run.Status != model.RunStatusCompleted {
		t.Fatalf("expected completed run, got %q", run.Status)
	}

	var key string
	if err := db.RawDB().QueryRowContext(ctx,
		"SELECT key FROM conversations WHERE id = ?",
		run.ConversationID,
	).Scan(&key); err != nil {
		t.Fatalf("query conversation key: %v", err)
	}
	if !strings.HasPrefix(key, "web:local:default:main:") {
		t.Fatalf("unexpected submit task conversation key %q", key)
	}
}

func TestRunEngine_PassesCWDIntoToolInvocationContext(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	tool := &cwdAwareTool{}
	reg.Register(tool)
	prov := NewMockProvider(
		[]GenerateResult{
			{
				ToolCalls: []model.ToolCallRequest{
					{ID: "call-root", ToolName: tool.Name(), InputJSON: []byte(`{}`)},
				},
				StopReason: "tool_calls",
			},
			{Content: "done", StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})

	workspaceRoot := t.TempDir()
	if _, err := rt.Start(context.Background(), StartRun{
		ConversationID: "conv-workspace-tool",
		AgentID:        "agent-a",
		Objective:      "use tool",
		CWD:            workspaceRoot,
		ExecutionSnapshotJSON: mustSnapshotJSON(t, model.ExecutionSnapshot{
			TeamID: "default",
			Agents: map[string]model.AgentProfile{
				"agent-a": {
					AgentID:      "agent-a",
					BaseProfile:  model.BaseProfileOperator,
					ToolFamilies: []model.ToolFamily{model.ToolFamilyRuntimeCapability},
				},
			},
		}),
	}); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if tool.cwd != workspaceRoot {
		t.Fatalf("expected tool to receive cwd %q, got %q", workspaceRoot, tool.cwd)
	}
}

func TestRunEngine_PassesAuthorityIntoToolInvocationContext(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	tool := &authorityAwareTool{}
	reg.Register(tool)
	prov := NewMockProvider(
		[]GenerateResult{
			{
				ToolCalls: []model.ToolCallRequest{
					{ID: "call-authority", ToolName: tool.Name(), InputJSON: []byte(`{}`)},
				},
				StopReason: "tool_calls",
			},
			{Content: "done", StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})

	rawAuthority, err := json.Marshal(authority.Envelope{
		ApprovalMode:   authority.ApprovalModeAutoApprove,
		HostAccessMode: authority.HostAccessModeElevated,
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := rt.Start(context.Background(), StartRun{
		ConversationID: "conv-authority-tool",
		AgentID:        "agent-a",
		Objective:      "use tool",
		CWD:            t.TempDir(),
		AuthorityJSON:  rawAuthority,
		ExecutionSnapshotJSON: mustSnapshotJSON(t, model.ExecutionSnapshot{
			TeamID: "default",
			Agents: map[string]model.AgentProfile{
				"agent-a": {
					AgentID:      "agent-a",
					BaseProfile:  model.BaseProfileOperator,
					ToolFamilies: []model.ToolFamily{model.ToolFamilyRuntimeCapability},
				},
			},
		}),
	}); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if tool.env.HostAccessMode != authority.HostAccessModeElevated {
		t.Fatalf("expected elevated host access, got %q", tool.env.HostAccessMode)
	}
	if tool.env.ApprovalMode != authority.ApprovalModeAutoApprove {
		t.Fatalf("expected auto approve mode, got %q", tool.env.ApprovalMode)
	}
}

func TestRunEngine_UsesPersistedAuthoritySettingsByDefault(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	tool := &authorityAwareTool{}
	reg.Register(tool)
	prov := NewMockProvider(
		[]GenerateResult{
			{
				ToolCalls: []model.ToolCallRequest{
					{ID: "call-authority-defaults", ToolName: tool.Name(), InputJSON: []byte(`{}`)},
				},
				StopReason: "tool_calls",
			},
			{Content: "done", StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})

	if _, err := db.RawDB().Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES
		 ('approval_mode', 'auto_approve', datetime('now')),
		 ('host_access_mode', 'elevated', datetime('now'))`,
	); err != nil {
		t.Fatalf("insert authority settings: %v", err)
	}

	run, err := rt.Start(context.Background(), StartRun{
		ConversationID: "conv-authority-defaults",
		AgentID:        "agent-a",
		Objective:      "use tool",
		CWD:            t.TempDir(),
		ExecutionSnapshotJSON: mustSnapshotJSON(t, model.ExecutionSnapshot{
			TeamID: "default",
			Agents: map[string]model.AgentProfile{
				"agent-a": {
					AgentID:      "agent-a",
					BaseProfile:  model.BaseProfileOperator,
					ToolFamilies: []model.ToolFamily{model.ToolFamilyRuntimeCapability},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	env, err := authority.DecodeEnvelope(run.AuthorityJSON)
	if err != nil {
		t.Fatalf("DecodeEnvelope: %v", err)
	}
	if env.HostAccessMode != authority.HostAccessModeElevated {
		t.Fatalf("run host access mode = %q, want %q", env.HostAccessMode, authority.HostAccessModeElevated)
	}
	if env.ApprovalMode != authority.ApprovalModeAutoApprove {
		t.Fatalf("run approval mode = %q, want %q", env.ApprovalMode, authority.ApprovalModeAutoApprove)
	}
	if tool.env.HostAccessMode != authority.HostAccessModeElevated {
		t.Fatalf("tool host access mode = %q, want %q", tool.env.HostAccessMode, authority.HostAccessModeElevated)
	}
	if tool.env.ApprovalMode != authority.ApprovalModeAutoApprove {
		t.Fatalf("tool approval mode = %q, want %q", tool.env.ApprovalMode, authority.ApprovalModeAutoApprove)
	}
}

func TestRunEngine_ChildRunInheritsParentAuthority(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})

	rawAuthority, err := json.Marshal(authority.Envelope{
		ApprovalMode:   authority.ApprovalModePrompt,
		HostAccessMode: authority.HostAccessModeElevated,
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	parent, err := rt.Start(context.Background(), StartRun{
		ConversationID: "conv-authority-inherit",
		AgentID:        "assistant",
		Objective:      "coordinate",
		CWD:            t.TempDir(),
		AuthorityJSON:  rawAuthority,
	})
	if err != nil {
		t.Fatalf("Start parent failed: %v", err)
	}

	childID := generateID()
	if err := rt.createRun(context.Background(), childID, parent.ID, StartRun{
		ConversationID: parent.ConversationID,
		AgentID:        "worker",
		Objective:      "inspect files",
		CWD:            parent.CWD,
	}); err != nil {
		t.Fatalf("create child run: %v", err)
	}

	child, err := rt.loadRun(context.Background(), childID)
	if err != nil {
		t.Fatalf("load child run: %v", err)
	}

	env, err := authority.DecodeEnvelope(child.AuthorityJSON)
	if err != nil {
		t.Fatalf("DecodeEnvelope: %v", err)
	}
	if env.ApprovalMode != authority.ApprovalModePrompt {
		t.Fatalf("child approval mode = %q, want %q", env.ApprovalMode, authority.ApprovalModePrompt)
	}
	if env.HostAccessMode != authority.HostAccessModeElevated {
		t.Fatalf("child host access mode = %q, want %q", env.HostAccessMode, authority.HostAccessModeElevated)
	}
}

func TestRunEngine_EmitsTurnDeltasToEventSink(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	sink := &recordingReplaySink{}
	rt := New(db, cs, reg, mem, &streamingProvider{}, sink)

	run, err := rt.Start(context.Background(), StartRun{
		ConversationID: "conv-stream",
		AgentID:        "agent-a",
		Objective:      "stream text",
		CWD:            t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if run.Status != model.RunStatusCompleted {
		t.Fatalf("expected status %q, got %q", model.RunStatusCompleted, run.Status)
	}

	var deltaTexts []string
	var sawTurnCompleted bool
	var completedContent string
	for _, evt := range sink.events {
		switch evt.Kind {
		case "turn_delta":
			var payload struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(evt.PayloadJSON, &payload); err != nil {
				t.Fatalf("unmarshal turn_delta payload: %v", err)
			}
			deltaTexts = append(deltaTexts, payload.Text)
		case "turn_completed":
			sawTurnCompleted = true
			var payload struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal(evt.PayloadJSON, &payload); err != nil {
				t.Fatalf("unmarshal turn_completed payload: %v", err)
			}
			completedContent = payload.Content
		}
	}

	if len(deltaTexts) != 2 {
		t.Fatalf("expected 2 turn deltas, got %d", len(deltaTexts))
	}
	if deltaTexts[0] != "Hel" || deltaTexts[1] != "lo" {
		t.Fatalf("unexpected turn deltas: %v", deltaTexts)
	}
	if !sawTurnCompleted {
		t.Fatal("expected turn_completed replay event")
	}
	if completedContent != "Hello" {
		t.Fatalf("expected turn_completed payload content %q, got %q", "Hello", completedContent)
	}
}

func TestRunEngine_LifecycleEventsJournaled(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
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
		ConversationID: "conv-2",
		AgentID:        "agent-a",
		Objective:      "lifecycle test",
		CWD:            t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	rows, err := db.RawDB().QueryContext(ctx,
		"SELECT kind FROM events WHERE run_id = ? ORDER BY created_at ASC",
		run.ID,
	)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	defer rows.Close()

	var kinds []string
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			t.Fatalf("scan kind: %v", err)
		}
		kinds = append(kinds, kind)
	}

	if len(kinds) < 2 {
		t.Fatalf("expected at least 2 lifecycle events, got %d: %v", len(kinds), kinds)
	}

	hasStarted := false
	hasCompleted := false
	for _, kind := range kinds {
		if kind == "run_started" {
			hasStarted = true
		}
		if kind == "run_completed" {
			hasCompleted = true
		}
	}
	if !hasStarted {
		t.Fatal("missing 'run_started' event")
	}
	if !hasCompleted {
		t.Fatal("missing 'run_completed' event")
	}
}

func TestRunEngine_PersistsProviderModelID(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "done", InputTokens: 10, OutputTokens: 20, StopReason: "end_turn", ModelID: "gpt-5.4"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})

	run, err := rt.Start(context.Background(), StartRun{
		ConversationID: "conv-model-id",
		AgentID:        "agent-a",
		Objective:      "persist model identity",
		CWD:            t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var runModelID string
	if err := db.RawDB().QueryRow(
		"SELECT COALESCE(model_id, '') FROM runs WHERE id = ?",
		run.ID,
	).Scan(&runModelID); err != nil {
		t.Fatalf("query run model_id: %v", err)
	}
	if runModelID != "gpt-5.4" {
		t.Fatalf("expected run model_id gpt-5.4, got %q", runModelID)
	}

	var receiptModelID string
	if err := db.RawDB().QueryRow(
		"SELECT COALESCE(model_id, '') FROM receipts WHERE run_id = ?",
		run.ID,
	).Scan(&receiptModelID); err != nil {
		t.Fatalf("query receipt model_id: %v", err)
	}
	if receiptModelID != "gpt-5.4" {
		t.Fatalf("expected receipt model_id gpt-5.4, got %q", receiptModelID)
	}
}

func TestRunEngine_StartRollsBackWhenRunStartedEventFails(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(nil, nil)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)
	ctx := context.Background()

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

	_, err = rt.Start(ctx, StartRun{
		ConversationID: "conv-atomic-start",
		AgentID:        "agent-a",
		Objective:      "should rollback",
		CWD:            t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected start error, got nil")
	}

	var runCount int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM runs WHERE conversation_id = 'conv-atomic-start'",
	).Scan(&runCount)
	if err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runCount != 0 {
		t.Fatalf("expected 0 runs after rollback, got %d", runCount)
	}
}

func TestRunEngine_RunCompletedAndReceiptAreAtomic(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "done", InputTokens: 10, OutputTokens: 20, StopReason: "end_turn"},
		},
		nil,
	)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)
	ctx := context.Background()

	_, err := db.RawDB().Exec(
		`CREATE TRIGGER fail_receipts_before_insert
		 BEFORE INSERT ON receipts
		 BEGIN
		   SELECT RAISE(FAIL, 'boom');
		 END;`,
	)
	if err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	_, err = rt.Start(ctx, StartRun{
		ConversationID: "conv-atomic-complete",
		AgentID:        "agent-a",
		Objective:      "should fail on receipt",
		CWD:            t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected completion error, got nil")
	}

	var completedEvents int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE conversation_id = 'conv-atomic-complete' AND kind = 'run_completed'",
	).Scan(&completedEvents)
	if err != nil {
		t.Fatalf("count run_completed events: %v", err)
	}
	if completedEvents != 0 {
		t.Fatalf("expected 0 run_completed events after rollback, got %d", completedEvents)
	}

	var completedRuns int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM runs WHERE conversation_id = 'conv-atomic-complete' AND status = 'completed'",
	).Scan(&completedRuns)
	if err != nil {
		t.Fatalf("count completed runs: %v", err)
	}
	if completedRuns != 0 {
		t.Fatalf("expected 0 completed runs after rollback, got %d", completedRuns)
	}
}

func TestRunEngine_NeverWritesToStoreDirectly(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "done", InputTokens: 10, OutputTokens: 20, StopReason: "end_turn"},
		},
		nil,
	)
	sink := &model.NoopEventSink{}

	rt := New(db, cs, reg, mem, prov, sink)
	ctx := context.Background()

	_, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-3",
		AgentID:        "agent-a",
		Objective:      "store test",
		CWD:            t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	rows, err := db.RawDB().QueryContext(ctx,
		"SELECT id, conversation_id FROM events WHERE conversation_id = '' OR conversation_id IS NULL",
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var orphaned int
	for rows.Next() {
		orphaned++
	}
	if orphaned > 0 {
		t.Fatalf("found %d events without conversation_id (written outside AppendEvent path)", orphaned)
	}
}

func TestRunEngine_RejectsCompetingRootRun(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(nil, nil)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)
	ctx := context.Background()

	_, err := db.RawDB().Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
		 VALUES ('root-1', 'conv-busy', 'agent-a', 'active', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert active root run: %v", err)
	}

	_, err = rt.Start(ctx, StartRun{
		ConversationID: "conv-busy",
		AgentID:        "agent-b",
		Objective:      "should be blocked",
		CWD:            t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected competing root run error, got nil")
	}
	if !strings.Contains(err.Error(), "competing root run active") {
		t.Fatalf("expected ErrConversationBusy, got %v", err)
	}
}

func TestRunEngine_NeverImportsWeb(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "github.com/canhta/gistclaw/internal/runtime")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list failed: %v\n%s", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "internal/web") {
			t.Fatalf("internal/runtime must not import internal/web, found: %s", line)
		}
	}
}

func TestBudgetGuard_PerRunCapExhaustion(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "turn 1", InputTokens: 60000, OutputTokens: 50000, StopReason: "end_turn"},
		},
		nil,
	)
	sink := &model.NoopEventSink{}

	rt := New(db, cs, reg, mem, prov, sink)
	rt.budget.PerRunTokenCap = 100000
	ctx := context.Background()

	run, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-budget-1",
		AgentID:        "agent-a",
		Objective:      "budget test",
		CWD:            t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if run.Status != model.RunStatusCompleted {
		t.Fatalf("expected status %q, got %q", model.RunStatusCompleted, run.Status)
	}

	var budgetEventCount int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = ? AND kind = 'budget_exhausted'",
		run.ID,
	).Scan(&budgetEventCount)
	if err != nil {
		t.Fatalf("query budget events: %v", err)
	}
	t.Logf("budget_exhausted events: %d", budgetEventCount)
}

func TestBudgetGuard_DailyCapBlocksNewRun(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	sink := &model.NoopEventSink{}

	_, err := db.RawDB().Exec(
		`INSERT INTO receipts (id, run_id, cost_usd, created_at)
		 VALUES ('r1', 'old-run', 15.0, datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert receipt: %v", err)
	}

	prov := NewMockProvider(nil, nil)
	rt := New(db, cs, reg, mem, prov, sink)
	rt.budget.DailyCostCapUSD = 10.0
	ctx := context.Background()

	_, err = rt.Start(ctx, StartRun{
		ConversationID: "conv-daily-cap",
		AgentID:        "agent-a",
		Objective:      "should be blocked",
		CWD:            t.TempDir(),
		AccountID:      "local",
	})
	if err == nil {
		t.Fatal("expected error from daily cap, got nil")
	}
	if !strings.Contains(err.Error(), "daily") {
		t.Fatalf("expected daily cap error, got: %v", err)
	}
}

func TestBudgetGuard_RollingWindow_NotUTCMidnight(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	sink := &model.NoopEventSink{}

	twentyThreeHoursAgo := time.Now().UTC().Add(-23 * time.Hour).Format("2006-01-02 15:04:05")
	_, err := db.RawDB().Exec(
		`INSERT INTO receipts (id, run_id, cost_usd, created_at)
		 VALUES ('r-rolling', 'old-run-2', 15.0, ?)`,
		twentyThreeHoursAgo,
	)
	if err != nil {
		t.Fatalf("insert receipt: %v", err)
	}

	prov := NewMockProvider(nil, nil)
	rt := New(db, cs, reg, mem, prov, sink)
	rt.budget.DailyCostCapUSD = 10.0
	ctx := context.Background()

	_, err = rt.Start(ctx, StartRun{
		ConversationID: "conv-rolling",
		AgentID:        "agent-a",
		Objective:      "should be blocked by rolling window",
		CWD:            t.TempDir(),
		AccountID:      "local",
	})
	if err == nil {
		t.Fatal("expected error from rolling window cap, got nil")
	}
}

func TestBudgetGuard_ActiveChildBudgetNotInBudgetGuard(t *testing.T) {
	bg := BudgetGuard{}
	_ = bg.PerRunTokenCap
	_ = bg.DailyCostCapUSD
}

func TestRunEngine_ContextCompaction_At75Percent(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "turn 1 with lots of tokens", InputTokens: 80000, OutputTokens: 80000, StopReason: "continue"},
			{Content: "turn 2 after compaction", InputTokens: 5000, OutputTokens: 5000, StopReason: "end_turn"},
		},
		nil,
	)
	sink := &model.NoopEventSink{}

	rt := New(db, cs, reg, mem, prov, sink)
	rt.contextWindowSize = 200000
	ctx := context.Background()

	run, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-compact",
		AgentID:        "agent-a",
		Objective:      "compaction test",
		CWD:            t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if run.Status != model.RunStatusCompleted {
		t.Fatalf("expected completed, got %q", run.Status)
	}

	var compactionCount int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = ? AND kind = 'context_compacted'",
		run.ID,
	).Scan(&compactionCount)
	if err != nil {
		t.Fatalf("query compaction events: %v", err)
	}
	if compactionCount == 0 {
		t.Fatal("expected at least 1 context_compacted event")
	}
}

func TestRunEngine_InterruptsOnEmptyNonTerminalTurn(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "", InputTokens: 100, OutputTokens: 50, StopReason: "continue"},
		},
		nil,
	)

	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	run, err := rt.Start(context.Background(), StartRun{
		ConversationID: "conv-empty-nonterminal",
		AgentID:        "agent-a",
		Objective:      "should not hang forever",
		CWD:            t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected empty non-terminal turn to interrupt the run")
	}
	if run.Status != model.RunStatusInterrupted {
		t.Fatalf("expected interrupted run, got %q", run.Status)
	}
	if prov.CallCount() != 1 {
		t.Fatalf("expected provider to stop after 1 call, got %d", prov.CallCount())
	}

	var interruptedCount int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = ? AND kind = 'run_interrupted'",
		run.ID,
	).Scan(&interruptedCount); err != nil {
		t.Fatalf("query interrupted events: %v", err)
	}
	if interruptedCount != 1 {
		t.Fatalf("expected 1 run_interrupted event, got %d", interruptedCount)
	}
}

func TestRunEngine_MemoryContextReadIsJournaled(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
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
		ConversationID: "conv-memory-read",
		AgentID:        "agent-a",
		Objective:      "journal memory reads",
		CWD:            t.TempDir(),
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
		t.Fatalf("count memory read events: %v", err)
	}
	if readEvents == 0 {
		t.Fatal("expected memory_context_loaded event")
	}
}

func TestRunEngine_DoesNotPromoteOrdinaryCompletedRootRunIntoProjectMemory(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "Built the OpenClaw launch page and verified the files.", InputTokens: 12, OutputTokens: 18, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	ctx := context.Background()

	run, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-memory-promotion",
		AgentID:        "assistant",
		Objective:      "Ship the launch page",
		CWD:            t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if run.ProjectID == "" {
		t.Fatal("expected completed run to carry project id")
	}

	items, err := mem.Search(ctx, model.MemoryQuery{
		ProjectID: run.ProjectID,
		AgentID:   "assistant",
		Scope:     "team",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no durable memory for ordinary completed run, got %d", len(items))
	}

	var promotedEvents int
	if err := db.RawDB().QueryRowContext(ctx,
		`SELECT count(*) FROM events WHERE conversation_id = ? AND kind = 'memory_promoted'`,
		run.ConversationID,
	).Scan(&promotedEvents); err != nil {
		t.Fatalf("count memory_promoted events: %v", err)
	}
	if promotedEvents != 0 {
		t.Fatalf("expected 0 memory_promoted events, got %d", promotedEvents)
	}
}

func TestRunEngine_PromotesExplicitMemoryRequestIntoProjectMemory(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "The repo uses pnpm workspaces.", InputTokens: 12, OutputTokens: 10, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	ctx := context.Background()

	run, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-memory-explicit",
		AgentID:        "assistant",
		Objective:      "Remember this for future runs: the repo uses pnpm workspaces.",
		CWD:            t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	items, err := mem.Search(ctx, model.MemoryQuery{
		ProjectID: run.ProjectID,
		AgentID:   "assistant",
		Scope:     "team",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 durable memory item for explicit request, got %d", len(items))
	}
	if got := items[0].Content; got != "the repo uses pnpm workspaces." {
		t.Fatalf("expected durable memory to store only the reusable fact, got %q", got)
	}
	if got := items[0].Provenance; got != "explicit_memory_request" {
		t.Fatalf("expected explicit memory provenance, got %q", got)
	}

	var promotedEvents int
	if err := db.RawDB().QueryRowContext(ctx,
		`SELECT count(*) FROM events WHERE conversation_id = ? AND kind = 'memory_promoted'`,
		run.ConversationID,
	).Scan(&promotedEvents); err != nil {
		t.Fatalf("count memory_promoted events: %v", err)
	}
	if promotedEvents != 1 {
		t.Fatalf("expected 1 memory_promoted event, got %d", promotedEvents)
	}
}

func TestRunEngine_DoesNotPromoteVagueExplicitMemoryPromptIntoProjectMemory(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "Understood. I'll keep that in mind for future runs.", InputTokens: 8, OutputTokens: 12, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	ctx := context.Background()

	run, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-memory-vague",
		AgentID:        "assistant",
		Objective:      "Remember this for future runs.",
		CWD:            t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	items, err := mem.Search(ctx, model.MemoryQuery{
		ProjectID: run.ProjectID,
		AgentID:   "assistant",
		Scope:     "team",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no durable memory for vague remember prompt, got %d", len(items))
	}

	var promotedEvents int
	if err := db.RawDB().QueryRowContext(ctx,
		`SELECT count(*) FROM events WHERE conversation_id = ? AND kind = 'memory_promoted'`,
		run.ConversationID,
	).Scan(&promotedEvents); err != nil {
		t.Fatalf("count memory_promoted events: %v", err)
	}
	if promotedEvents != 0 {
		t.Fatalf("expected 0 memory_promoted events, got %d", promotedEvents)
	}
}

func TestRunEngine_PromotesNaturalPromptPreferencesIntoProjectMemory(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "Done.", InputTokens: 12, OutputTokens: 8, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	ctx := context.Background()

	run, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-memory-natural",
		AgentID:        "assistant",
		Objective:      "Please create a tiny static developer notes page. Keep the tone technical and aimed at developers evaluating self-hosted assistants. If tooling is needed, prefer bun-based workflows and keep lockfile churn isolated. Use Codex CLI for code changes, then review and verify before wrapping up.",
		CWD:            t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	items, err := mem.Search(ctx, model.MemoryQuery{
		ProjectID: run.ProjectID,
		AgentID:   "assistant",
		Scope:     "team",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 durable memory item for prompt preferences, got %d", len(items))
	}
	want := "Keep the tone technical and aimed at developers evaluating self-hosted assistants. If tooling is needed, prefer bun-based workflows and keep lockfile churn isolated. Use Codex CLI for code changes."
	if got := items[0].Content; got != want {
		t.Fatalf("expected prompt preference memory %q, got %q", want, got)
	}
	if got := items[0].Provenance; got != "prompt_preference_summary" {
		t.Fatalf("expected prompt preference provenance, got %q", got)
	}
}

func TestRunEngine_PromotesNaturalPromptPreferencesBeforeRunSucceeds(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	rt := New(db, cs, reg, mem, &blockingProvider{}, &model.NoopEventSink{})
	rt.providerTimeout = 20 * time.Millisecond
	ctx := context.Background()

	run, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-memory-natural-fail",
		AgentID:        "assistant",
		Objective:      "Please create a tiny static developer notes page. Keep the tone technical and aimed at developers evaluating self-hosted assistants. If tooling is needed, prefer bun-based workflows and keep lockfile churn isolated. Use Codex CLI for code changes, then review and verify before wrapping up.",
		CWD:            t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected provider timeout error")
	}

	items, searchErr := mem.Search(ctx, model.MemoryQuery{
		ProjectID: run.ProjectID,
		AgentID:   "assistant",
		Scope:     "team",
		Limit:     10,
	})
	if searchErr != nil {
		t.Fatalf("Search failed: %v", searchErr)
	}
	if len(items) != 1 {
		t.Fatalf("expected prompt preference memory to persist before run success, got %d", len(items))
	}
}

func TestRunEngine_ExecutesToolCallsAndCarriesResultsIntoNextTurn(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	tool := &recordingRuntimeTool{
		name:   "repo_read",
		result: `{"summary":"README loaded"}`,
	}
	reg.Register(tool)

	prov := NewMockProvider(
		[]GenerateResult{
			{
				Content: "I need to inspect the repo first.",
				ToolCalls: []model.ToolCallRequest{
					{
						ID:        "call-readme",
						ToolName:  "repo_read",
						InputJSON: []byte(`{"path":"README.md"}`),
					},
				},
				InputTokens:  10,
				OutputTokens: 5,
				StopReason:   "tool_calls",
			},
			{
				Content:      "The README is loaded.",
				InputTokens:  8,
				OutputTokens: 6,
				StopReason:   "end_turn",
			},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	ctx := context.Background()

	run, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-tool-loop",
		AgentID:        "agent-a",
		Objective:      "Inspect README and summarize it",
		CWD:            t.TempDir(),
		ExecutionSnapshotJSON: mustSnapshotJSON(t, model.ExecutionSnapshot{
			TeamID: "default",
			Agents: map[string]model.AgentProfile{
				"agent-a": {
					AgentID:      "agent-a",
					BaseProfile:  model.BaseProfileOperator,
					ToolFamilies: []model.ToolFamily{model.ToolFamilyRuntimeCapability},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if run.Status != model.RunStatusCompleted {
		t.Fatalf("expected completed, got %q", run.Status)
	}

	if len(tool.calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(tool.calls))
	}
	if string(tool.calls[0].InputJSON) != `{"path":"README.md"}` {
		t.Fatalf("unexpected tool input: %s", tool.calls[0].InputJSON)
	}

	if len(prov.Requests) != 2 {
		t.Fatalf("expected 2 provider requests, got %d", len(prov.Requests))
	}
	if len(prov.Requests[0].ToolSpecs) != 1 {
		t.Fatalf("expected first request to advertise 1 tool, got %d", len(prov.Requests[0].ToolSpecs))
	}
	if prov.Requests[0].ToolSpecs[0].Name != "repo_read" {
		t.Fatalf("unexpected tool spec name %q", prov.Requests[0].ToolSpecs[0].Name)
	}

	var sawToolEvent bool
	for _, ev := range prov.Requests[1].ConversationCtx {
		if ev.Kind != "tool_call_recorded" {
			continue
		}
		sawToolEvent = true
		var payload struct {
			ToolCallID string          `json:"tool_call_id"`
			ToolName   string          `json:"tool_name"`
			InputJSON  json.RawMessage `json:"input_json"`
			OutputJSON json.RawMessage `json:"output_json"`
			Decision   string          `json:"decision"`
		}
		if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil {
			t.Fatalf("unmarshal tool payload: %v", err)
		}
		if payload.ToolCallID != "call-readme" {
			t.Fatalf("unexpected tool call id %q", payload.ToolCallID)
		}
		if payload.ToolName != "repo_read" {
			t.Fatalf("unexpected tool name %q", payload.ToolName)
		}
		if payload.Decision != "allow" {
			t.Fatalf("unexpected decision %q", payload.Decision)
		}
	}
	if !sawToolEvent {
		t.Fatal("expected second provider request to include tool_call_recorded context")
	}

	var toolCallCount int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM tool_calls WHERE run_id = ? AND tool_name = 'repo_read' AND decision = 'allow'",
		run.ID,
	).Scan(&toolCallCount); err != nil {
		t.Fatalf("query tool_calls: %v", err)
	}
	if toolCallCount != 1 {
		t.Fatalf("expected 1 projected tool call, got %d", toolCallCount)
	}
}

func TestRunEngine_ProviderInstructionsIncludeWorkspaceSnapshot(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "done", InputTokens: 10, OutputTokens: 20, StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	ctx := context.Background()

	workspaceRoot := t.TempDir()
	for path, body := range map[string]string{
		"README.md":  "# Demo Repo\n\nThis repository is for runtime testing.\n",
		"go.mod":     "module example.com/demo\n\ngo 1.24\n",
		"cmd/app.go": "package main\n\nfunc main() {}\n",
	} {
		abs := filepath.Join(workspaceRoot, path)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", abs, err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", abs, err)
		}
	}

	if _, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-workspace-context",
		AgentID:        "agent-a",
		Objective:      "review the repo",
		CWD:            workspaceRoot,
	}); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if len(prov.Requests) != 1 {
		t.Fatalf("expected 1 provider request, got %d", len(prov.Requests))
	}

	instructions := prov.Requests[0].Instructions
	for _, want := range []string{
		"review the repo",
		"Working directory:",
		"README.md",
		"go.mod",
		"module example.com/demo",
	} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("expected provider instructions to include %q, got:\n%s", want, instructions)
		}
	}
}

func TestRunEngine_JournalsToolLogsAndEmitsReplayDeltas(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	reg.Register(&loggingTool{})
	prov := NewMockProvider(
		[]GenerateResult{
			{
				Content: "Run the logging tool.",
				ToolCalls: []model.ToolCallRequest{
					{ID: "call-log", ToolName: "logging_tool", InputJSON: []byte(`{}`)},
				},
				InputTokens:  4,
				OutputTokens: 5,
			},
			{Content: "Logs captured.", InputTokens: 3, OutputTokens: 4, StopReason: "end_turn"},
		},
		nil,
	)
	sink := &recordingReplaySink{}
	rt := New(db, cs, reg, mem, prov, sink)

	run, err := rt.Start(context.Background(), StartRun{
		ConversationID: "conv-tool-log",
		AgentID:        "assistant",
		Objective:      "capture tool logs",
		CWD:            t.TempDir(),
		ExecutionSnapshotJSON: mustSnapshotJSON(t, model.ExecutionSnapshot{
			TeamID: "default",
			Agents: map[string]model.AgentProfile{
				"assistant": {
					AgentID:      "assistant",
					BaseProfile:  model.BaseProfileOperator,
					ToolFamilies: []model.ToolFamily{model.ToolFamilyRuntimeCapability},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if run.Status != model.RunStatusCompleted {
		t.Fatalf("expected completed run, got %s", run.Status)
	}

	var eventCount int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = ? AND kind = 'tool_log_recorded'",
		run.ID,
	).Scan(&eventCount); err != nil {
		t.Fatalf("query tool_log_recorded events: %v", err)
	}
	if eventCount != 2 {
		t.Fatalf("expected 2 tool_log_recorded events, got %d", eventCount)
	}

	var sawStdout bool
	var sawStderr bool
	for _, evt := range sink.events {
		if evt.Kind != "tool_log_recorded" {
			continue
		}
		var payload struct {
			ToolCallID string `json:"tool_call_id"`
			ToolName   string `json:"tool_name"`
			Stream     string `json:"stream"`
			Text       string `json:"text"`
		}
		if err := json.Unmarshal(evt.PayloadJSON, &payload); err != nil {
			t.Fatalf("unmarshal tool log payload: %v", err)
		}
		if payload.ToolCallID != "call-log" || payload.ToolName != "logging_tool" {
			t.Fatalf("unexpected tool log payload %+v", payload)
		}
		switch payload.Stream {
		case "stdout":
			sawStdout = payload.Text == "planning files\n"
		case "stderr":
			sawStderr = payload.Text == "warning: fallback path\n"
		}
	}
	if !sawStdout || !sawStderr {
		t.Fatalf("expected stdout and stderr replay deltas, got %+v", sink.events)
	}
}

func TestStartFrontSession_ProviderContextUsesSessionMailboxNotConversationWideEvents(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "Front reply 1.", InputTokens: 10, OutputTokens: 12, StopReason: "end_turn"},
			{Content: "Worker reply.", InputTokens: 8, OutputTokens: 10, StopReason: "end_turn"},
			{Content: "Front reply 2.", InputTokens: 11, OutputTokens: 13, StopReason: "end_turn"},
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
		InitialPrompt: "First prompt.",
		CWD:           workspaceRoot,
	})
	if err != nil {
		t.Fatalf("first StartFrontSession failed: %v", err)
	}

	if _, err := rt.Spawn(ctx, SpawnCommand{
		ControllerSessionID: first.SessionID,
		AgentID:             "researcher",
		Prompt:              "Worker prompt.",
	}); err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	if _, err := rt.StartFrontSession(ctx, StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Second prompt.",
		CWD:           workspaceRoot,
	}); err != nil {
		t.Fatalf("second StartFrontSession failed: %v", err)
	}

	if len(prov.Requests) != 3 {
		t.Fatalf("expected 3 provider requests, got %d", len(prov.Requests))
	}

	gotBodies := make([]string, 0, len(prov.Requests[2].ConversationCtx))
	for _, ev := range prov.Requests[2].ConversationCtx {
		if ev.Kind != "session_message_added" {
			t.Fatalf("expected session-scoped conversation events, got %q", ev.Kind)
		}
		var payload struct {
			Body       string `json:"body"`
			Provenance struct {
				Kind string `json:"kind"`
			} `json:"provenance"`
		}
		if err := json.Unmarshal(ev.PayloadJSON, &payload); err != nil {
			t.Fatalf("unmarshal provider context payload: %v", err)
		}
		gotBodies = append(gotBodies, payload.Body)
		if payload.Provenance.Kind == "" {
			t.Fatalf("expected provider context payload to carry provenance, got %s", string(ev.PayloadJSON))
		}
	}

	gotJoined := strings.Join(gotBodies, " | ")
	if strings.Contains(gotJoined, "Worker prompt.") || strings.Contains(gotJoined, "Worker reply.") {
		t.Fatalf("expected worker-only history to stay out of front-session context, got %q", gotJoined)
	}
	for _, want := range []string{"First prompt.", "Front reply 1.", "Second prompt."} {
		if !strings.Contains(gotJoined, want) {
			t.Fatalf("expected provider context to include %q, got %q", want, gotJoined)
		}
	}
}

type recordingRuntimeTool struct {
	name   string
	result string
	calls  []model.ToolCall
}

func (t *recordingRuntimeTool) Name() string {
	return t.name
}

func (t *recordingRuntimeTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.name,
		Description:     "test tool",
		InputSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"}}}`,
		Family:          model.ToolFamilyRuntimeCapability,
		Risk:            model.RiskLow,
		Approval:        "never",
	}
}

func (t *recordingRuntimeTool) Invoke(_ context.Context, call model.ToolCall) (model.ToolResult, error) {
	t.calls = append(t.calls, call)
	return model.ToolResult{Output: t.result}, nil
}
