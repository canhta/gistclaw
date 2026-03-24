package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
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
				ID:        "tc-001",
				ToolName:  "workspace_apply",
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

func TestStarterWorkflow_RepoPatchRunsAsWorkerFlow(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)
	prov := NewMockProvider([]GenerateResult{
		{Content: "I will coordinate the patch flow.", InputTokens: 9, OutputTokens: 11, StopReason: "end_turn"},
		{Content: "Patch drafted.", InputTokens: 8, OutputTokens: 14, StopReason: "end_turn"},
		{Content: "Verification passed.", InputTokens: 7, OutputTokens: 12, StopReason: "end_turn"},
	}, nil)
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
		InitialPrompt: "Prepare a patch and verify it.",
		WorkspaceRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}

	patcher, err := rt.Spawn(ctx, SpawnCommand{
		ControllerRunID: front.ID,
		AgentID:         "patcher",
		Prompt:          "Draft the patch for main.go.",
	})
	if err != nil {
		t.Fatalf("Spawn patcher failed: %v", err)
	}
	verifier, err := rt.Spawn(ctx, SpawnCommand{
		ControllerRunID: front.ID,
		AgentID:         "verifier",
		Prompt:          "Verify the proposed patch.",
	})
	if err != nil {
		t.Fatalf("Spawn verifier failed: %v", err)
	}

	if err := rt.Announce(ctx, AnnounceCommand{
		WorkerRunID: patcher.ID,
		TargetRunID: front.ID,
		Body:        "Patch ready for review.",
	}); err != nil {
		t.Fatalf("Patch announce failed: %v", err)
	}
	if err := rt.Announce(ctx, AnnounceCommand{
		WorkerRunID: verifier.ID,
		TargetRunID: front.ID,
		Body:        "Verification passed.",
	}); err != nil {
		t.Fatalf("Verification announce failed: %v", err)
	}

	if patcher.ParentRunID != front.ID {
		t.Fatalf("expected patcher parent %s, got %s", front.ID, patcher.ParentRunID)
	}
	if verifier.ParentRunID != front.ID {
		t.Fatalf("expected verifier parent %s, got %s", front.ID, verifier.ParentRunID)
	}

	var announceCount int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM session_messages WHERE session_id = ? AND kind = 'announce'",
		front.SessionID,
	).Scan(&announceCount); err != nil {
		t.Fatalf("query announce messages: %v", err)
	}
	if announceCount != 2 {
		t.Fatalf("expected 2 worker announcements on front session, got %d", announceCount)
	}
}
