package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

// TestStarterWorkflow_PreviewOnly verifies that a preview-only run never emits
// a workspace_apply event, but does emit a preview_completed event.
func TestStarterWorkflow_PreviewOnly(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)

	// Provider returns a tool call that would normally trigger workspace apply.
	prov := NewMockProvider([]GenerateResult{
		{
			Content: "I will apply a patch",
			ToolCalls: []model.ToolCallRequest{{
				ID:       "tc-001",
				ToolName: "workspace_apply",
				InputJSON: []byte(`{"path":"main.go","content":"package main\n"}`),
			}},
			StopReason: "tool_use",
		},
		{Content: "Done.", InputTokens: 10, OutputTokens: 5, StopReason: "end_turn"},
	}, nil)

	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)
	ctx := context.Background()

	run, err := rt.Start(ctx, StartRun{
		ConversationID: "conv-preview",
		AgentID:        "coordinator",
		Objective:      "Apply a patch to main.go",
		WorkspaceRoot:  t.TempDir(),
		PreviewOnly:    true,
	})
	if err != nil {
		t.Fatalf("Start preview-only run: %v", err)
	}
	if run.Status != model.RunStatusCompleted {
		t.Fatalf("expected completed, got %s", run.Status)
	}

	var applyCount int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = ? AND kind = 'workspace_apply'",
		run.ID,
	).Scan(&applyCount); err != nil {
		t.Fatalf("query workspace_apply events: %v", err)
	}
	if applyCount != 0 {
		t.Fatalf("preview-only run must not emit workspace_apply events, got %d", applyCount)
	}

	var previewCount int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = ? AND kind = 'preview_completed'",
		run.ID,
	).Scan(&previewCount); err != nil {
		t.Fatalf("query preview_completed events: %v", err)
	}
	if previewCount == 0 {
		t.Fatal("expected preview_completed event in journal for preview-only run")
	}
}

// TestStarterWorkflow_ApplyWithoutApprovalRejected verifies that WorkspaceApplier.Apply
// returns ErrNoApproval when no valid approved ticket is provided.
func TestStarterWorkflow_ApplyWithoutApprovalRejected(t *testing.T) {
	workspaceRoot := t.TempDir()
	applier := tools.NewWorkspaceApplier(workspaceRoot)
	ctx := context.Background()

	_, err := applier.Apply(ctx, "run-no-approval", model.ApprovalTicket{}, []model.FileChange{
		{Path: "a.go", Content: []byte("package main\n")},
	})
	if err == nil {
		t.Fatal("expected error when applying without approval ticket")
	}
	if !errors.Is(err, tools.ErrNoApproval) {
		t.Fatalf("expected ErrNoApproval, got %v", err)
	}
}

// TestStarterWorkflow_FingerprintMismatchRejected verifies that an approved ticket
// whose fingerprint no longer matches the proposed action is rejected before apply.
func TestStarterWorkflow_FingerprintMismatchRejected(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	workspaceRoot := t.TempDir()

	// Create and approve a ticket for path "a.go".
	ticket, err := tools.CreateTicket(ctx, db, model.ApprovalRequest{
		RunID:      "run-fp",
		ToolName:   "workspace_apply",
		ArgsJSON:   []byte(`{"path":"a.go"}`),
		TargetPath: "a.go",
	})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if err := tools.ResolveTicket(ctx, db, ticket.ID, "approved"); err != nil {
		t.Fatalf("ResolveTicket: %v", err)
	}

	// Re-fetch so the struct reflects the updated status.
	ticket, err = tools.LoadTicket(ctx, db, ticket.ID)
	if err != nil {
		t.Fatalf("LoadTicket: %v", err)
	}

	// Try to apply with a DIFFERENT path — fingerprint will not match.
	applier := tools.NewWorkspaceApplierWithDB(workspaceRoot, db)
	_, err = applier.Apply(ctx, "run-fp", ticket, []model.FileChange{
		{Path: "different.go", Content: []byte("package main\n")},
	})
	if err == nil {
		t.Fatal("expected error for fingerprint mismatch")
	}
	if !errors.Is(err, tools.ErrTicketExpired) {
		t.Fatalf("expected ErrTicketExpired for fingerprint mismatch, got %v", err)
	}
}

// TestStarterWorkflow_VerificationResultAttached verifies that when a run
// appends a verification_completed event, it appears in the run's event list.
func TestStarterWorkflow_VerificationResultAttached(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)
	prov := NewMockProvider([]GenerateResult{
		{Content: "tests pass", InputTokens: 5, OutputTokens: 10, StopReason: "end_turn"},
	}, nil)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)
	ctx := context.Background()

	run, err := rt.Start(ctx, StartRun{
		ConversationID:    "conv-verify",
		AgentID:           "verifier",
		Objective:         "verify: run tests",
		WorkspaceRoot:     t.TempDir(),
		VerificationAgent: true,
	})
	if err != nil {
		t.Fatalf("Start verification run: %v", err)
	}

	var verifyCount int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE run_id = ? AND kind = 'verification_completed'",
		run.ID,
	).Scan(&verifyCount); err != nil {
		t.Fatalf("query verification_completed: %v", err)
	}
	if verifyCount == 0 {
		t.Fatalf("expected verification_completed event after verification agent run")
	}
}

// --- Team validation tests ---

// TestTeamValidation_MissingRequiredField verifies that a team spec missing
// a required top-level field (agents, capability_flags, or handoff_edges)
// fails validation with a descriptive error naming the missing field.
func TestTeamValidation_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		missing string
	}{
		{
			name:    "missing agents",
			yaml:    "name: default\ncapability_flags: {}\nhandoff_edges: []\n",
			missing: "agents",
		},
		{
			name:    "missing capability_flags",
			yaml:    "name: default\nagents: []\nhandoff_edges: []\n",
			missing: "capability_flags",
		},
		{
			name:    "missing handoff_edges",
			yaml:    "name: default\nagents: []\ncapability_flags: {}\n",
			missing: "handoff_edges",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadTeamSpec([]byte(tc.yaml))
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.missing) {
				t.Fatalf("expected error to mention %q, got: %v", tc.missing, err)
			}
		})
	}
}

// TestTeamValidation_UnknownAgentInEdge verifies that a handoff edge referencing
// an agent ID not declared in the agents list fails validation.
func TestTeamValidation_UnknownAgentInEdge(t *testing.T) {
	yaml := `
name: default
agents:
  - id: coordinator
    soul_file: coordinator.soul.yaml
capability_flags:
  coordinator: [operator_facing]
handoff_edges:
  - from: coordinator
    to: agent-UNKNOWN
`
	_, err := LoadTeamSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for handoff edge referencing unknown agent")
	}
	if !strings.Contains(err.Error(), "agent-UNKNOWN") {
		t.Fatalf("expected error to mention unknown agent ID, got: %v", err)
	}
}
