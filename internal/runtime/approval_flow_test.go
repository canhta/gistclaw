package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

func TestRunEngine_ApprovalRequestCreatesTicketAndPausesRun(t *testing.T) {
	rt, db, _ := newApprovalRuntime(t, []GenerateResult{
		{
			ToolCalls: []model.ToolCallRequest{
				{ID: "call-touch", ToolName: "shell_exec", InputJSON: []byte(`{"command":"touch created.txt"}`)},
			},
			StopReason: "tool_calls",
		},
	})
	workspaceRoot := t.TempDir()

	run, err := rt.Start(context.Background(), StartRun{
		ConversationID:        "conv-approval-pause",
		AgentID:               "patcher",
		Objective:             "mutate shell",
		WorkspaceRoot:         workspaceRoot,
		ExecutionSnapshotJSON: mustSnapshotJSON(t, workspaceWriteSnapshot()),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if run.Status != model.RunStatusNeedsApproval {
		t.Fatalf("expected needs_approval, got %q", run.Status)
	}

	var ticketID, toolName, argsJSON string
	if err := db.RawDB().QueryRow(
		"SELECT id, tool_name, CAST(args_json AS TEXT) FROM approvals WHERE run_id = ? AND status = 'pending' LIMIT 1",
		run.ID,
	).Scan(&ticketID, &toolName, &argsJSON); err != nil {
		t.Fatalf("query approval ticket: %v", err)
	}
	if ticketID == "" || toolName != "shell_exec" {
		t.Fatalf("unexpected approval ticket: id=%q tool=%q", ticketID, toolName)
	}
	if argsJSON != `{"command":"touch created.txt"}` {
		t.Fatalf("unexpected args_json %q", argsJSON)
	}

	var toolCallCount int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM tool_calls WHERE run_id = ?",
		run.ID,
	).Scan(&toolCallCount); err != nil {
		t.Fatalf("query tool_calls: %v", err)
	}
	if toolCallCount != 0 {
		t.Fatalf("expected no recorded tool call before approval, got %d", toolCallCount)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "created.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected file to not exist before approval, stat err=%v", err)
	}
}

func TestResolveApproval_ApprovedExecutesToolAndResumesRun(t *testing.T) {
	prov := NewMockProvider([]GenerateResult{
		{
			ToolCalls: []model.ToolCallRequest{
				{ID: "call-touch", ToolName: "shell_exec", InputJSON: []byte(`{"command":"touch created.txt"}`)},
			},
			StopReason: "tool_calls",
		},
		{Content: "done", StopReason: "end_turn"},
	}, nil)
	rt, db, prov := newApprovalRuntimeWithProvider(t, prov)
	workspaceRoot := t.TempDir()

	run, err := rt.Start(context.Background(), StartRun{
		ConversationID:        "conv-approval-approve",
		AgentID:               "patcher",
		Objective:             "mutate shell",
		WorkspaceRoot:         workspaceRoot,
		ExecutionSnapshotJSON: mustSnapshotJSON(t, workspaceWriteSnapshot()),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var ticketID string
	if err := db.RawDB().QueryRow(
		"SELECT id FROM approvals WHERE run_id = ? AND status = 'pending' LIMIT 1",
		run.ID,
	).Scan(&ticketID); err != nil {
		t.Fatalf("query approval ticket: %v", err)
	}

	if err := rt.ResolveApproval(context.Background(), ticketID, "approved"); err != nil {
		t.Fatalf("ResolveApproval approved: %v", err)
	}

	run, err = rt.loadRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if run.Status != model.RunStatusCompleted {
		t.Fatalf("expected completed run, got %q", run.Status)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "created.txt")); err != nil {
		t.Fatalf("expected approved command to create file: %v", err)
	}

	var decision, approvalID string
	if err := db.RawDB().QueryRow(
		"SELECT decision, COALESCE(approval_id, '') FROM tool_calls WHERE run_id = ? AND tool_name = 'shell_exec' LIMIT 1",
		run.ID,
	).Scan(&decision, &approvalID); err != nil {
		t.Fatalf("query recorded tool call: %v", err)
	}
	if decision != string(model.DecisionAllow) || approvalID != ticketID {
		t.Fatalf("unexpected recorded approval tool call: decision=%q approval_id=%q", decision, approvalID)
	}

	if len(prov.Requests) != 2 {
		t.Fatalf("expected provider to resume for second request, got %d requests", len(prov.Requests))
	}
	if !containsEventKind(prov.Requests[1].ConversationCtx, "tool_call_recorded") {
		t.Fatalf("expected resumed provider request to include tool_call_recorded context")
	}
}

func TestResolveApproval_ApprovedCoderExecutesToolAndResumesRun(t *testing.T) {
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
	reg.Register(&fakeCoderExecTool{})
	prov := NewMockProvider([]GenerateResult{
		{
			ToolCalls: []model.ToolCallRequest{
				{ID: "call-coder", ToolName: "coder_exec", InputJSON: []byte(`{"backend":"codex","prompt":"Create created.txt"}`)},
			},
			StopReason: "tool_calls",
		},
		{Content: "done", StopReason: "end_turn"},
	}, nil)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	workspaceRoot := t.TempDir()

	run, err := rt.Start(context.Background(), StartRun{
		ConversationID:        "conv-approval-coder",
		AgentID:               "patcher",
		Objective:             "mutate via coder",
		WorkspaceRoot:         workspaceRoot,
		ExecutionSnapshotJSON: mustSnapshotJSON(t, workspaceWriteSnapshot()),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var ticketID string
	if err := db.RawDB().QueryRow(
		"SELECT id FROM approvals WHERE run_id = ? AND status = 'pending' LIMIT 1",
		run.ID,
	).Scan(&ticketID); err != nil {
		t.Fatalf("query approval ticket: %v", err)
	}

	if err := rt.ResolveApproval(context.Background(), ticketID, "approved"); err != nil {
		t.Fatalf("ResolveApproval approved: %v", err)
	}

	run, err = rt.loadRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if run.Status != model.RunStatusCompleted {
		t.Fatalf("expected completed run, got %q", run.Status)
	}
	data, err := os.ReadFile(filepath.Join(workspaceRoot, "created.txt"))
	if err != nil {
		t.Fatalf("expected approved coder tool to create file: %v", err)
	}
	if string(data) != "created by coder\n" {
		t.Fatalf("unexpected created.txt content %q", string(data))
	}

	var decision, approvalID string
	if err := db.RawDB().QueryRow(
		"SELECT decision, COALESCE(approval_id, '') FROM tool_calls WHERE run_id = ? AND tool_name = 'coder_exec' LIMIT 1",
		run.ID,
	).Scan(&decision, &approvalID); err != nil {
		t.Fatalf("query recorded tool call: %v", err)
	}
	if decision != string(model.DecisionAllow) || approvalID != ticketID {
		t.Fatalf("unexpected recorded approval tool call: decision=%q approval_id=%q", decision, approvalID)
	}

	if len(prov.Requests) != 2 {
		t.Fatalf("expected provider to resume for second request, got %d requests", len(prov.Requests))
	}
	if !containsEventKind(prov.Requests[1].ConversationCtx, "tool_call_recorded") {
		t.Fatalf("expected resumed provider request to include tool_call_recorded context")
	}
}

func TestResolveApproval_ApprovedShellExecRunsApprovedCommand(t *testing.T) {
	prov := NewMockProvider([]GenerateResult{
		{
			ToolCalls: []model.ToolCallRequest{
				{
					ID:        "call-pipe",
					ToolName:  "shell_exec",
					InputJSON: []byte(`{"command":"printf 'hello\\n' | tee created.txt >/dev/null"}`),
				},
			},
			StopReason: "tool_calls",
		},
		{Content: "done", StopReason: "end_turn"},
	}, nil)
	rt, db, prov := newApprovalRuntimeWithProvider(t, prov)
	workspaceRoot := t.TempDir()

	run, err := rt.Start(context.Background(), StartRun{
		ConversationID:        "conv-approval-approved-shell",
		AgentID:               "patcher",
		Objective:             "use approved shell syntax",
		WorkspaceRoot:         workspaceRoot,
		ExecutionSnapshotJSON: mustSnapshotJSON(t, workspaceWriteSnapshot()),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var ticketID string
	if err := db.RawDB().QueryRow(
		"SELECT id FROM approvals WHERE run_id = ? AND status = 'pending' LIMIT 1",
		run.ID,
	).Scan(&ticketID); err != nil {
		t.Fatalf("query approval ticket: %v", err)
	}

	if err := rt.ResolveApproval(context.Background(), ticketID, "approved"); err != nil {
		t.Fatalf("ResolveApproval approved: %v", err)
	}

	run, err = rt.loadRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if run.Status != model.RunStatusCompleted {
		t.Fatalf("expected completed run, got %q", run.Status)
	}

	data, err := os.ReadFile(filepath.Join(workspaceRoot, "created.txt"))
	if err != nil {
		t.Fatalf("expected approved command to create file: %v", err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("expected created.txt to contain hello, got %q", string(data))
	}

	if len(prov.Requests) != 2 {
		t.Fatalf("expected provider to resume after approved shell_exec, got %d requests", len(prov.Requests))
	}
}

func TestResolveApproval_ApprovedToolErrorInterruptsRunWithoutResumingProvider(t *testing.T) {
	prov := NewMockProvider([]GenerateResult{
		{
			ToolCalls: []model.ToolCallRequest{
				{ID: "call-missing", ToolName: "shell_exec", InputJSON: []byte(`{"command":"missing-command created.txt"}`)},
			},
			StopReason: "tool_calls",
		},
	}, nil)
	rt, db, prov := newApprovalRuntimeWithProvider(t, prov)
	workspaceRoot := t.TempDir()

	run, err := rt.Start(context.Background(), StartRun{
		ConversationID:        "conv-approval-error",
		AgentID:               "patcher",
		Objective:             "mutate shell with failing command",
		WorkspaceRoot:         workspaceRoot,
		ExecutionSnapshotJSON: mustSnapshotJSON(t, workspaceWriteSnapshot()),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var ticketID string
	if err := db.RawDB().QueryRow(
		"SELECT id FROM approvals WHERE run_id = ? AND status = 'pending' LIMIT 1",
		run.ID,
	).Scan(&ticketID); err != nil {
		t.Fatalf("query approval ticket: %v", err)
	}

	if err := rt.ResolveApproval(context.Background(), ticketID, "approved"); err != nil {
		t.Fatalf("ResolveApproval approved: %v", err)
	}

	run, err = rt.loadRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if run.Status != model.RunStatusInterrupted {
		t.Fatalf("expected interrupted run after approved tool error, got %q", run.Status)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "created.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected failing approved command to not create file, stat err=%v", err)
	}

	var decision, approvalID string
	var outputJSON []byte
	if err := db.RawDB().QueryRow(
		"SELECT decision, COALESCE(approval_id, ''), output_json FROM tool_calls WHERE run_id = ? AND tool_name = 'shell_exec' LIMIT 1",
		run.ID,
	).Scan(&decision, &approvalID, &outputJSON); err != nil {
		t.Fatalf("query recorded tool call: %v", err)
	}
	if decision != string(model.DecisionAllow) || approvalID != ticketID {
		t.Fatalf("unexpected recorded approval tool call: decision=%q approval_id=%q", decision, approvalID)
	}
	var result model.ToolResult
	if err := json.Unmarshal(outputJSON, &result); err != nil {
		t.Fatalf("unmarshal tool result: %v", err)
	}
	if result.Error == "" {
		t.Fatalf("expected approved tool result to include error, got %+v", result)
	}

	if len(prov.Requests) != 1 {
		t.Fatalf("expected provider to not resume after approved tool error, got %d requests", len(prov.Requests))
	}
}

func TestResolveApproval_DeniedInterruptsRun(t *testing.T) {
	rt, db, _ := newApprovalRuntime(t, []GenerateResult{
		{
			ToolCalls: []model.ToolCallRequest{
				{ID: "call-touch", ToolName: "shell_exec", InputJSON: []byte(`{"command":"touch denied.txt"}`)},
			},
			StopReason: "tool_calls",
		},
	})
	workspaceRoot := t.TempDir()

	run, err := rt.Start(context.Background(), StartRun{
		ConversationID:        "conv-approval-deny",
		AgentID:               "patcher",
		Objective:             "mutate shell",
		WorkspaceRoot:         workspaceRoot,
		ExecutionSnapshotJSON: mustSnapshotJSON(t, workspaceWriteSnapshot()),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var ticketID string
	if err := db.RawDB().QueryRow(
		"SELECT id FROM approvals WHERE run_id = ? AND status = 'pending' LIMIT 1",
		run.ID,
	).Scan(&ticketID); err != nil {
		t.Fatalf("query approval ticket: %v", err)
	}

	if err := rt.ResolveApproval(context.Background(), ticketID, "denied"); err != nil {
		t.Fatalf("ResolveApproval denied: %v", err)
	}

	run, err = rt.loadRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if run.Status != model.RunStatusInterrupted {
		t.Fatalf("expected interrupted run, got %q", run.Status)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "denied.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected denied command to not create file, stat err=%v", err)
	}

	var decision string
	var outputJSON []byte
	if err := db.RawDB().QueryRow(
		"SELECT decision, output_json FROM tool_calls WHERE run_id = ? AND tool_name = 'shell_exec' LIMIT 1",
		run.ID,
	).Scan(&decision, &outputJSON); err != nil {
		t.Fatalf("query denied tool call: %v", err)
	}
	if decision != string(model.DecisionDeny) {
		t.Fatalf("expected deny decision, got %q", decision)
	}
	var result model.ToolResult
	if err := json.Unmarshal(outputJSON, &result); err != nil {
		t.Fatalf("unmarshal denied result: %v", err)
	}
	if result.Error == "" {
		t.Fatalf("expected denied result to include error, got %+v", result)
	}
}

func newApprovalRuntime(t *testing.T, responses []GenerateResult) (*Runtime, *store.DB, *MockProvider) {
	t.Helper()
	return newApprovalRuntimeWithProvider(t, NewMockProvider(responses, nil))
}

func newApprovalRuntimeWithProvider(t *testing.T, prov *MockProvider) (*Runtime, *store.DB, *MockProvider) {
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
		t.Fatalf("BuildRegistry: %v", err)
	}
	if closer != nil {
		t.Cleanup(func() { _ = closer.Close() })
	}

	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	return rt, db, prov
}

func workspaceWriteSnapshot() model.ExecutionSnapshot {
	return model.ExecutionSnapshot{
		TeamID: "default",
		Agents: map[string]model.AgentProfile{
			"patcher": {
				AgentID:      "patcher",
				ToolProfile:  "workspace_write",
				Capabilities: []model.AgentCapability{model.CapWorkspaceWrite},
			},
		},
	}
}

func containsEventKind(events []model.Event, want string) bool {
	for _, event := range events {
		if event.Kind == want {
			return true
		}
	}
	return false
}

type fakeCoderExecTool struct{}

func (t *fakeCoderExecTool) Name() string { return "coder_exec" }

func (t *fakeCoderExecTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:       t.Name(),
		Risk:       model.RiskHigh,
		SideEffect: "exec_write",
	}
}

func (t *fakeCoderExecTool) Invoke(ctx context.Context, _ model.ToolCall) (model.ToolResult, error) {
	meta, ok := tools.InvocationContextFrom(ctx)
	if !ok || meta.WorkspaceRoot == "" {
		return model.ToolResult{}, tools.ErrWorkspaceRequired
	}
	target := filepath.Join(meta.WorkspaceRoot, "created.txt")
	if err := os.WriteFile(target, []byte("created by coder\n"), 0o644); err != nil {
		return model.ToolResult{}, err
	}
	return model.ToolResult{
		Output: `{"backend":"codex","command":"codex exec --sandbox workspace-write","cwd":".","stdout":"created created.txt","stderr":"","exit_code":0,"timed_out":false,"truncated":false,"effect":"exec_write"}`,
	}, nil
}
