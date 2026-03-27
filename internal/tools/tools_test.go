package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

func setupToolsDB(t *testing.T) *store.DB {
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

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	tool := &stubTool{name: "file_read"}
	reg.Register(tool)

	got, ok := reg.Get("file_read")
	if !ok {
		t.Fatal("expected to find tool 'file_read'")
	}
	if got.Name() != "file_read" {
		t.Fatalf("expected %q, got %q", "file_read", got.Name())
	}
}

func TestRegistry_ListReturnsAll(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&stubTool{name: "file_read"})
	reg.Register(&stubTool{name: "shell_exec"})

	specs := reg.List()
	if len(specs) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(specs))
	}
}

func TestPolicy_ReadOnlyProfileDeniesWrite(t *testing.T) {
	p := &Policy{Profile: "read_heavy"}
	agent := model.AgentProfile{
		Capabilities: []model.AgentCapability{model.CapReadHeavy},
		ToolProfile:  "read_heavy",
	}
	spec := model.ToolSpec{Name: "file_write", Risk: model.RiskMedium}

	decision := p.Decide(agent, model.RunProfile{}, spec)
	if decision.Mode != model.DecisionDeny {
		t.Fatalf("expected deny for write tool with read_heavy profile, got %s", decision.Mode)
	}
}

func TestPolicy_ScopedWriteProfileAsksForShellExec(t *testing.T) {
	p := &Policy{Profile: "scoped_write"}
	agent := model.AgentProfile{
		Capabilities: []model.AgentCapability{model.CapScopedWrite},
		ToolProfile:  "scoped_write",
	}
	spec := model.ToolSpec{Name: "shell_exec", Risk: model.RiskHigh}

	decision := p.Decide(agent, model.RunProfile{}, spec)
	if decision.Mode != model.DecisionAsk {
		t.Fatalf("expected ask for shell_exec with scoped_write profile, got %s", decision.Mode)
	}
}

func TestPolicy_ScopedWriteProfileAsksForCoderExec(t *testing.T) {
	p := &Policy{Profile: "scoped_write"}
	agent := model.AgentProfile{
		Capabilities: []model.AgentCapability{model.CapScopedWrite},
		ToolProfile:  "scoped_write",
	}
	spec := model.ToolSpec{Name: "coder_exec", Risk: model.RiskHigh, SideEffect: effectExecWrite}

	decision := p.Decide(agent, model.RunProfile{}, spec)
	if decision.Mode != model.DecisionAsk {
		t.Fatalf("expected ask for coder_exec with scoped_write profile, got %s", decision.Mode)
	}
}

func TestPolicy_ScopedWriteProfileAsksForRequiredApprovalTool(t *testing.T) {
	p := &Policy{Profile: "scoped_write"}
	agent := model.AgentProfile{
		Capabilities: []model.AgentCapability{model.CapScopedWrite},
		ToolProfile:  "scoped_write",
	}
	spec := model.ToolSpec{
		Name:       "write_new_file",
		Risk:       model.RiskMedium,
		SideEffect: effectCreate,
		Approval:   "required",
	}

	decision := p.Decide(agent, model.RunProfile{}, spec)
	if decision.Mode != model.DecisionAsk {
		t.Fatalf("expected ask for required approval tool with scoped_write profile, got %s", decision.Mode)
	}
}

func TestPolicy_ReadToolAlwaysAllowed(t *testing.T) {
	p := &Policy{Profile: "read_heavy"}
	agent := model.AgentProfile{
		Capabilities: []model.AgentCapability{model.CapReadHeavy},
		ToolProfile:  "read_heavy",
	}
	spec := model.ToolSpec{Name: "file_read", Risk: model.RiskLow}

	decision := p.Decide(agent, model.RunProfile{}, spec)
	if decision.Mode != model.DecisionAllow {
		t.Fatalf("expected allow for read tool, got %s", decision.Mode)
	}
}

func TestPolicy_DeniesSpawnForAgentWithoutCapability(t *testing.T) {
	p := &Policy{}
	agent := model.AgentProfile{
		AgentID:      "worker",
		Capabilities: []model.AgentCapability{model.CapReadHeavy},
		ToolProfile:  "read_heavy",
	}
	spec := model.ToolSpec{Name: "session_spawn", Risk: model.RiskLow}

	decision := p.Decide(agent, model.RunProfile{}, spec)
	if decision.Mode != model.DecisionDeny {
		t.Fatalf("expected deny, got %s", decision.Mode)
	}
}

func TestPolicy_AllowsSpawnForReadHeavyAgentWithCapability(t *testing.T) {
	p := &Policy{}
	agent := model.AgentProfile{
		AgentID:      "assistant",
		Capabilities: []model.AgentCapability{model.CapReadHeavy, model.CapSpawn},
		ToolProfile:  "read_heavy",
	}
	spec := model.ToolSpec{Name: "session_spawn", Risk: model.RiskLow}

	decision := p.Decide(agent, model.RunProfile{}, spec)
	if decision.Mode != model.DecisionAllow {
		t.Fatalf("expected allow, got %s", decision.Mode)
	}
}

func TestRegisterCollaborationTools_RegistersSessionSpawn(t *testing.T) {
	reg := NewRegistry()

	RegisterCollaborationTools(reg, CollaborationHandlers{
		Spawn: func(context.Context, SessionSpawnRequest) (SessionSpawnResult, error) {
			return SessionSpawnResult{}, nil
		},
	})

	if _, ok := reg.Get("session_spawn"); !ok {
		t.Fatal("expected session_spawn to be registered")
	}
}

func TestSessionSpawnTool_InvokeUsesAuthorizedSpawnTarget(t *testing.T) {
	var got SessionSpawnRequest
	tool := &SessionSpawnTool{
		spawn: func(_ context.Context, req SessionSpawnRequest) (SessionSpawnResult, error) {
			got = req
			return SessionSpawnResult{
				RunID:     "run-child",
				SessionID: "session-child",
				AgentID:   "researcher",
				Status:    model.RunStatusCompleted,
				Output:    "research complete",
			}, nil
		},
	}

	ctx := WithInvocationContext(context.Background(), InvocationContext{
		SessionID: "session-parent",
		Agent: model.AgentProfile{
			AgentID:  "assistant",
			CanSpawn: []string{"researcher"},
		},
	})
	result, err := tool.Invoke(ctx, model.ToolCall{
		ID:        "call-1",
		ToolName:  "session_spawn",
		InputJSON: []byte(`{"agent_id":"researcher","prompt":"inspect OpenClaw"}`),
	})
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if got.ControllerSessionID != "session-parent" || got.AgentID != "researcher" || got.Prompt != "inspect OpenClaw" {
		t.Fatalf("unexpected spawn request: %+v", got)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload["run_id"] != "run-child" {
		t.Fatalf("expected run-child output, got %+v", payload)
	}
}

func TestSessionSpawnTool_InvokeRejectsUndeclaredTarget(t *testing.T) {
	tool := &SessionSpawnTool{
		spawn: func(context.Context, SessionSpawnRequest) (SessionSpawnResult, error) {
			t.Fatal("spawn handler must not be called")
			return SessionSpawnResult{}, nil
		},
	}

	ctx := WithInvocationContext(context.Background(), InvocationContext{
		SessionID: "session-parent",
		Agent: model.AgentProfile{
			AgentID:  "assistant",
			CanSpawn: []string{"researcher"},
		},
	})
	_, err := tool.Invoke(ctx, model.ToolCall{
		ID:        "call-1",
		ToolName:  "session_spawn",
		InputJSON: []byte(`{"agent_id":"verifier","prompt":"inspect OpenClaw"}`),
	})
	if err == nil || err.Error() != "session_spawn: assistant cannot spawn verifier" {
		t.Fatalf("expected unauthorized target error, got %v", err)
	}
}

func TestApproval_FingerprintChangeCausesExpiry(t *testing.T) {
	db := setupToolsDB(t)
	ctx := context.Background()

	ticket, err := CreateTicket(ctx, db, model.ApprovalRequest{
		RunID:       "run-1",
		ToolName:    "file_write",
		ArgsJSON:    []byte(`{"path":"a.txt"}`),
		BindingJSON: []byte(`{"tool_name":"file_write","operands":["/workspace/a.txt"]}`),
	})
	if err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	newFingerprint := computeFingerprint(
		"file_write",
		[]byte(`{"path":"b.txt"}`),
		[]byte(`{"tool_name":"file_write","operands":["/workspace/b.txt"]}`),
	)

	err = VerifyTicket(ctx, db, ticket.ID, newFingerprint)
	if err == nil {
		t.Fatal("expected ErrTicketExpired for changed fingerprint")
	}
}

func TestApproval_SingleUse(t *testing.T) {
	db := setupToolsDB(t)
	ctx := context.Background()

	ticket, err := CreateTicket(ctx, db, model.ApprovalRequest{
		RunID:       "run-2",
		ToolName:    "file_write",
		ArgsJSON:    []byte(`{"path":"a.txt"}`),
		BindingJSON: []byte(`{"tool_name":"file_write","operands":["/workspace/a.txt"]}`),
	})
	if err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	err = ResolveTicket(ctx, db, ticket.ID, "approved")
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}

	err = ResolveTicket(ctx, db, ticket.ID, "approved")
	if err == nil {
		t.Fatal("expected ErrTicketExpired on second resolve")
	}
}

func TestComputeFingerprint_BindsBindingJSON(t *testing.T) {
	first := computeFingerprint(
		"scoped_apply",
		[]byte(`{"path":"README.md"}`),
		[]byte(`{"tool_name":"scoped_apply","operands":["README.md"]}`),
	)
	second := computeFingerprint(
		"scoped_apply",
		[]byte(`{"path":"README.md"}`),
		[]byte(`{"tool_name":"scoped_apply","operands":["README.md"]}`),
	)
	if first != second {
		t.Fatalf("expected deterministic fingerprint, got %q then %q", first, second)
	}

	changed := computeFingerprint(
		"scoped_apply",
		[]byte(`{"path":"README.md"}`),
		[]byte(`{"tool_name":"scoped_apply","operands":["main.go"]}`),
	)
	if changed == first {
		t.Fatal("expected binding_json to affect fingerprint")
	}
}

func TestScopedApplier_RejectsEscapeAttempt(t *testing.T) {
	wsRoot := t.TempDir()
	applier := NewScopedApplier(wsRoot)
	ctx := context.Background()

	changes := []model.FileChange{
		{Path: "../../etc/passwd", Content: []byte("hacked"), Op: "create"},
	}

	_, err := applier.Preview(ctx, "run-1", changes)
	if err == nil {
		t.Fatal("expected ErrEscapeAttempt for path traversal")
	}
}

func TestScopedApplier_AllowsValidPath(t *testing.T) {
	wsRoot := t.TempDir()
	applier := NewScopedApplier(wsRoot)
	ctx := context.Background()

	changes := []model.FileChange{
		{Path: "src/main.go", Content: []byte("package main"), Op: "create"},
	}

	preview, err := applier.Preview(ctx, "run-1", changes)
	if err != nil {
		t.Fatalf("Preview failed for valid path: %v", err)
	}
	if len(preview.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(preview.Changes))
	}
}

func TestScopedApply_RequiresApprovedTicketForWorkerRun(t *testing.T) {
	db := setupToolsDB(t)
	ctx := context.Background()
	workspaceRoot := t.TempDir()

	ticket, err := CreateTicket(ctx, db, model.ApprovalRequest{
		RunID:       "run-front",
		ToolName:    "scoped_apply",
		ArgsJSON:    []byte(`{"path":"main.go"}`),
		BindingJSON: []byte(`{"tool_name":"scoped_apply","operands":["main.go"]}`),
	})
	if err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}
	if err := ResolveTicket(ctx, db, ticket.ID, "approved"); err != nil {
		t.Fatalf("ResolveTicket failed: %v", err)
	}
	ticket, err = LoadTicket(ctx, db, ticket.ID)
	if err != nil {
		t.Fatalf("LoadTicket failed: %v", err)
	}

	applier := NewScopedApplierWithDB(workspaceRoot, db)
	_, err = applier.Apply(ctx, "run-worker", ticket, []model.FileChange{
		{Path: "main.go", Content: []byte("package main\n"), Op: "update"},
	})
	if err == nil {
		t.Fatal("expected run-bound approval failure")
	}
	if err != ErrNoApproval {
		t.Fatalf("expected ErrNoApproval, got %v", err)
	}
}

func TestShellExec_RejectsSemicolon(t *testing.T) {
	err := validateShellArgs("ls; rm -rf /")
	if err == nil {
		t.Fatal("expected rejection for semicolon")
	}
}

func TestShellExec_RejectsPipe(t *testing.T) {
	err := validateShellArgs("cat file | grep secret")
	if err == nil {
		t.Fatal("expected rejection for pipe")
	}
}

func TestShellExec_RejectsPathTraversal(t *testing.T) {
	err := validateShellArgs("cat ../../etc/passwd")
	if err == nil {
		t.Fatal("expected rejection for path traversal")
	}
}

func TestShellExec_RejectsNullByte(t *testing.T) {
	err := validateShellArgs("cat file\x00.txt")
	if err == nil {
		t.Fatal("expected rejection for null byte")
	}
}

func TestShellExec_AllowsSafeCommand(t *testing.T) {
	err := validateShellArgs("go test ./...")
	if err != nil {
		t.Fatalf("expected safe command to pass, got: %v", err)
	}
}

type stubTool struct {
	name string
}

func (s *stubTool) Name() string { return s.name }

func (s *stubTool) Spec() model.ToolSpec {
	risk := model.RiskLow
	if s.name == "file_write" || s.name == "shell_exec" {
		risk = model.RiskMedium
	}
	return model.ToolSpec{Name: s.name, Risk: risk}
}

func (s *stubTool) Invoke(_ context.Context, _ model.ToolCall) (model.ToolResult, error) {
	return model.ToolResult{Output: "ok"}, nil
}

var _ Tool = (*stubTool)(nil)
