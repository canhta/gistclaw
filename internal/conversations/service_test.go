package conversations

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

func setupTestStore(t *testing.T) *store.DB {
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

func TestConversationStore_AppendEventAndRetrieve(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	key := ConversationKey{
		ConnectorID: "cli",
		AccountID:   "local",
		ExternalID:  "conv1",
		ThreadID:    "main",
	}

	conv, err := cs.Resolve(ctx, key)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if conv.ID == "" {
		t.Fatal("expected non-empty conversation ID")
	}

	evt := model.Event{
		ID:             "evt-1",
		ConversationID: conv.ID,
		RunID:          "run-1",
		Kind:           "run_started",
		PayloadJSON:    []byte(`{"objective":"test task"}`),
	}

	err = cs.AppendEvent(ctx, evt)
	if err != nil {
		t.Fatalf("AppendEvent failed: %v", err)
	}

	events, err := cs.ListEvents(ctx, conv.ID, 10)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Kind != "run_started" {
		t.Fatalf("expected kind %q, got %q", "run_started", events[0].Kind)
	}
}

func TestConversationStore_ActiveRootRunArbitration(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	key := ConversationKey{
		ConnectorID: "cli",
		AccountID:   "local",
		ExternalID:  "conv2",
		ThreadID:    "main",
	}

	conv, err := cs.Resolve(ctx, key)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	_, err = db.RawDB().ExecContext(ctx,
		`INSERT INTO runs (id, conversation_id, agent_id, status) VALUES (?, ?, ?, ?)`,
		"run-1", conv.ID, "agent-a", "active",
	)
	if err != nil {
		t.Fatalf("insert run-1: %v", err)
	}

	ref, err := cs.ActiveRootRun(ctx, conv.ID)
	if err != nil {
		t.Fatalf("ActiveRootRun failed: %v", err)
	}
	if ref.ID != "run-1" {
		t.Fatalf("expected run-1, got %q", ref.ID)
	}

	_, err = db.RawDB().ExecContext(ctx,
		`INSERT INTO runs (id, conversation_id, agent_id, status) VALUES (?, ?, ?, ?)`,
		"run-2", conv.ID, "agent-b", "active",
	)
	if err == nil {
		t.Fatal("expected second active root run insert to fail, got nil")
	}
}

func TestConversationStore_MissingThreadNormalization(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	key1 := ConversationKey{
		ConnectorID: "cli",
		AccountID:   "local",
		ExternalID:  "conv3",
		ThreadID:    "",
	}
	key2 := ConversationKey{
		ConnectorID: "cli",
		AccountID:   "local",
		ExternalID:  "conv3",
		ThreadID:    "main",
	}

	conv1, err := cs.Resolve(ctx, key1)
	if err != nil {
		t.Fatalf("Resolve key1 failed: %v", err)
	}
	conv2, err := cs.Resolve(ctx, key2)
	if err != nil {
		t.Fatalf("Resolve key2 failed: %v", err)
	}

	if conv1.ID != conv2.ID {
		t.Fatalf("missing thread and 'main' thread should resolve to same conversation: %q != %q", conv1.ID, conv2.ID)
	}
}

func TestConversationStore_ResolveIdempotent(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	key := ConversationKey{
		ConnectorID: "cli",
		AccountID:   "local",
		ExternalID:  "conv4",
		ThreadID:    "main",
	}

	conv1, err := cs.Resolve(ctx, key)
	if err != nil {
		t.Fatalf("first Resolve failed: %v", err)
	}
	conv2, err := cs.Resolve(ctx, key)
	if err != nil {
		t.Fatalf("second Resolve failed: %v", err)
	}

	if conv1.ID != conv2.ID {
		t.Fatalf("Resolve must be idempotent: %q != %q", conv1.ID, conv2.ID)
	}
}

func TestConversationStore_AppendEventProjectsRunLifecycle(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	payload, err := json.Marshal(map[string]any{
		"agent_id":       "agent-a",
		"objective":      "ship it",
		"workspace_root": t.TempDir(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	err = cs.AppendEvent(ctx, model.Event{
		ID:             "evt-run-started",
		ConversationID: "conv-run",
		RunID:          "run-life",
		Kind:           "run_started",
		PayloadJSON:    payload,
	})
	if err != nil {
		t.Fatalf("AppendEvent run_started failed: %v", err)
	}

	turnPayload, err := json.Marshal(map[string]any{
		"content":       "done",
		"input_tokens":  10,
		"output_tokens": 20,
		"model_lane":    "cheap",
	})
	if err != nil {
		t.Fatalf("marshal turn payload: %v", err)
	}

	err = cs.AppendEvent(ctx, model.Event{
		ID:             "evt-turn",
		ConversationID: "conv-run",
		RunID:          "run-life",
		Kind:           "turn_completed",
		PayloadJSON:    turnPayload,
	})
	if err != nil {
		t.Fatalf("AppendEvent turn_completed failed: %v", err)
	}

	completedPayload, err := json.Marshal(map[string]any{
		"input_tokens":  10,
		"output_tokens": 20,
		"cost_usd":      0.01,
		"model_lane":    "cheap",
	})
	if err != nil {
		t.Fatalf("marshal completed payload: %v", err)
	}

	err = cs.AppendEvent(ctx, model.Event{
		ID:             "evt-run-completed",
		ConversationID: "conv-run",
		RunID:          "run-life",
		Kind:           "run_completed",
		PayloadJSON:    completedPayload,
	})
	if err != nil {
		t.Fatalf("AppendEvent run_completed failed: %v", err)
	}

	var status string
	var inputTokens int
	var outputTokens int
	err = db.RawDB().QueryRowContext(ctx,
		"SELECT status, input_tokens, output_tokens FROM runs WHERE id = 'run-life'",
	).Scan(&status, &inputTokens, &outputTokens)
	if err != nil {
		t.Fatalf("query run projection: %v", err)
	}
	if status != "completed" || inputTokens != 10 || outputTokens != 20 {
		t.Fatalf("unexpected run projection: status=%s input=%d output=%d", status, inputTokens, outputTokens)
	}

	var receiptCount int
	err = db.RawDB().QueryRowContext(ctx,
		"SELECT count(*) FROM receipts WHERE run_id = 'run-life'",
	).Scan(&receiptCount)
	if err != nil {
		t.Fatalf("query receipt projection: %v", err)
	}
	if receiptCount != 1 {
		t.Fatalf("expected 1 receipt, got %d", receiptCount)
	}
}

func TestConversationStore_AppendEventProjectsDelegationsAndSummary(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	startPayload, err := json.Marshal(map[string]any{
		"agent_id":  "agent-a",
		"objective": "root task",
	})
	if err != nil {
		t.Fatalf("marshal start payload: %v", err)
	}

	err = cs.AppendEvent(ctx, model.Event{
		ID:             "evt-root-started",
		ConversationID: "conv-delegation",
		RunID:          "root-run",
		Kind:           "run_started",
		PayloadJSON:    startPayload,
	})
	if err != nil {
		t.Fatalf("AppendEvent run_started failed: %v", err)
	}

	delegationPayload, err := json.Marshal(map[string]any{
		"root_run_id":     "root-run",
		"target_agent_id": "agent-b",
		"objective":       "child task",
	})
	if err != nil {
		t.Fatalf("marshal delegation payload: %v", err)
	}

	err = cs.AppendEvent(ctx, model.Event{
		ID:             "evt-delegation-active",
		ConversationID: "conv-delegation",
		RunID:          "child-run",
		ParentRunID:    "root-run",
		Kind:           "delegation_created",
		PayloadJSON:    delegationPayload,
	})
	if err != nil {
		t.Fatalf("AppendEvent delegation_created failed: %v", err)
	}

	queuedPayload, err := json.Marshal(map[string]any{
		"root_run_id":     "root-run",
		"target_agent_id": "agent-c",
	})
	if err != nil {
		t.Fatalf("marshal queued payload: %v", err)
	}

	err = cs.AppendEvent(ctx, model.Event{
		ID:             "evt-delegation-queued",
		ConversationID: "conv-delegation",
		RunID:          "root-run",
		ParentRunID:    "root-run",
		Kind:           "delegation_queued",
		PayloadJSON:    queuedPayload,
	})
	if err != nil {
		t.Fatalf("AppendEvent delegation_queued failed: %v", err)
	}

	summaryPayload, err := json.Marshal(map[string]any{
		"summary_id":  "sum-1",
		"run_id":      "root-run",
		"content":     "summary content",
		"token_count": 12,
	})
	if err != nil {
		t.Fatalf("marshal summary payload: %v", err)
	}

	err = cs.AppendEvent(ctx, model.Event{
		ID:             "evt-summary",
		ConversationID: "conv-delegation",
		RunID:          "root-run",
		Kind:           "summary_upserted",
		PayloadJSON:    summaryPayload,
	})
	if err != nil {
		t.Fatalf("AppendEvent summary_upserted failed: %v", err)
	}

	var activeDelegations int
	var queuedDelegations int
	err = db.RawDB().QueryRowContext(ctx,
		"SELECT count(*) FROM delegations WHERE root_run_id = 'root-run' AND status = 'active'",
	).Scan(&activeDelegations)
	if err != nil {
		t.Fatalf("count active delegations: %v", err)
	}
	err = db.RawDB().QueryRowContext(ctx,
		"SELECT count(*) FROM delegations WHERE root_run_id = 'root-run' AND status = 'queued'",
	).Scan(&queuedDelegations)
	if err != nil {
		t.Fatalf("count queued delegations: %v", err)
	}
	if activeDelegations != 1 || queuedDelegations != 1 {
		t.Fatalf("unexpected delegation counts active=%d queued=%d", activeDelegations, queuedDelegations)
	}

	var summaryCount int
	err = db.RawDB().QueryRowContext(ctx,
		"SELECT count(*) FROM run_summaries WHERE run_id = 'root-run'",
	).Scan(&summaryCount)
	if err != nil {
		t.Fatalf("count summaries: %v", err)
	}
	if summaryCount != 1 {
		t.Fatalf("expected 1 summary, got %d", summaryCount)
	}
}

func TestConversationStore_AppendEventProjectsFailureAndInterruption(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	startPayload, err := json.Marshal(map[string]any{"agent_id": "agent-a"})
	if err != nil {
		t.Fatalf("marshal start payload: %v", err)
	}

	for _, runID := range []string{"run-failed", "run-interrupted"} {
		err = cs.AppendEvent(ctx, model.Event{
			ID:             "evt-start-" + runID,
			ConversationID: "conv-" + runID,
			RunID:          runID,
			Kind:           "run_started",
			PayloadJSON:    startPayload,
		})
		if err != nil {
			t.Fatalf("AppendEvent run_started for %s failed: %v", runID, err)
		}
	}

	if err := cs.AppendEvent(ctx, model.Event{
		ID:             "evt-budget",
		ConversationID: "conv-run-failed",
		RunID:          "run-failed",
		Kind:           "budget_exhausted",
	}); err != nil {
		t.Fatalf("AppendEvent budget_exhausted failed: %v", err)
	}

	if err := cs.AppendEvent(ctx, model.Event{
		ID:             "evt-failed",
		ConversationID: "conv-run-failed",
		RunID:          "run-failed",
		Kind:           "run_failed",
	}); err != nil {
		t.Fatalf("AppendEvent run_failed failed: %v", err)
	}

	if err := cs.AppendEvent(ctx, model.Event{
		ID:             "evt-interrupted",
		ConversationID: "conv-run-interrupted",
		RunID:          "run-interrupted",
		Kind:           "run_interrupted",
	}); err != nil {
		t.Fatalf("AppendEvent run_interrupted failed: %v", err)
	}

	var failedStatus string
	err = db.RawDB().QueryRowContext(ctx,
		"SELECT status FROM runs WHERE id = 'run-failed'",
	).Scan(&failedStatus)
	if err != nil {
		t.Fatalf("query failed run: %v", err)
	}
	if failedStatus != "failed" {
		t.Fatalf("expected failed status, got %q", failedStatus)
	}

	var interruptedStatus string
	err = db.RawDB().QueryRowContext(ctx,
		"SELECT status FROM runs WHERE id = 'run-interrupted'",
	).Scan(&interruptedStatus)
	if err != nil {
		t.Fatalf("query interrupted run: %v", err)
	}
	if interruptedStatus != "interrupted" {
		t.Fatalf("expected interrupted status, got %q", interruptedStatus)
	}
}

func TestConversationStore_DBAccessor(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)

	if cs.DB() != db {
		t.Fatal("expected DB accessor to return underlying db")
	}
}
