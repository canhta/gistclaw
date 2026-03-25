package runtime

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		WorkspaceRoot:  t.TempDir(),
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
		WorkspaceRoot:  t.TempDir(),
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
		WorkspaceRoot:  t.TempDir(),
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
		WorkspaceRoot:  t.TempDir(),
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
		WorkspaceRoot:  t.TempDir(),
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
		WorkspaceRoot:  t.TempDir(),
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
		WorkspaceRoot:  t.TempDir(),
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
		WorkspaceRoot:  t.TempDir(),
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
		WorkspaceRoot:  t.TempDir(),
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
		t.Fatalf("count memory read events: %v", err)
	}
	if readEvents == 0 {
		t.Fatal("expected memory_context_loaded event")
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
		WorkspaceRoot:  workspaceRoot,
	}); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if len(prov.Requests) != 1 {
		t.Fatalf("expected 1 provider request, got %d", len(prov.Requests))
	}

	instructions := prov.Requests[0].Instructions
	for _, want := range []string{
		"review the repo",
		"Workspace root:",
		"README.md",
		"go.mod",
		"module example.com/demo",
	} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("expected provider instructions to include %q, got:\n%s", want, instructions)
		}
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

	first, err := rt.StartFrontSession(ctx, StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "First prompt.",
		WorkspaceRoot: t.TempDir(),
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
		WorkspaceRoot: t.TempDir(),
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
