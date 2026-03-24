package conversations

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

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

func TestConversationStore_AppendEventProjectsSummary(t *testing.T) {
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

func TestConversationStore_AppendEventProjectsSessions(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	sessionPayload, err := json.Marshal(map[string]any{
		"session_id":            "sess-front",
		"key":                   "front:conv-session",
		"agent_id":              "assistant",
		"role":                  "front",
		"parent_session_id":     "",
		"controller_session_id": "",
		"status":                "active",
	})
	if err != nil {
		t.Fatalf("marshal session payload: %v", err)
	}

	err = cs.AppendEvent(ctx, model.Event{
		ID:             "evt-session-opened",
		ConversationID: "conv-session",
		RunID:          "run-front",
		Kind:           "session_opened",
		PayloadJSON:    sessionPayload,
	})
	if err != nil {
		t.Fatalf("AppendEvent session_opened failed: %v", err)
	}

	messagePayload, err := json.Marshal(map[string]any{
		"message_id":        "msg-1",
		"session_id":        "sess-front",
		"sender_session_id": "",
		"kind":              "assistant",
		"body":              "Hello.",
		"provenance": map[string]any{
			"kind":          "assistant_turn",
			"source_run_id": "run-front",
		},
	})
	if err != nil {
		t.Fatalf("marshal message payload: %v", err)
	}

	err = cs.AppendEvent(ctx, model.Event{
		ID:             "evt-session-message",
		ConversationID: "conv-session",
		RunID:          "run-front",
		Kind:           "session_message_added",
		PayloadJSON:    messagePayload,
	})
	if err != nil {
		t.Fatalf("AppendEvent session_message_added failed: %v", err)
	}

	var role string
	err = db.RawDB().QueryRowContext(ctx,
		"SELECT role FROM sessions WHERE id = 'sess-front'",
	).Scan(&role)
	if err != nil {
		t.Fatalf("query session projection: %v", err)
	}
	if role != "front" {
		t.Fatalf("expected session role %q, got %q", "front", role)
	}

	var kind string
	var provenanceJSON string
	err = db.RawDB().QueryRowContext(ctx,
		"SELECT kind, provenance_json FROM session_messages WHERE id = 'msg-1'",
	).Scan(&kind, &provenanceJSON)
	if err != nil {
		t.Fatalf("query session message projection: %v", err)
	}
	if kind != "assistant" {
		t.Fatalf("expected session message kind %q, got %q", "assistant", kind)
	}
	if !strings.Contains(provenanceJSON, `"kind":"assistant_turn"`) || !strings.Contains(provenanceJSON, `"source_run_id":"run-front"`) {
		t.Fatalf("expected provenance to round-trip, got %q", provenanceJSON)
	}
}

func TestConversationStore_AppendEventProjectsRunSessionID(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	startPayload, err := json.Marshal(map[string]any{
		"agent_id":       "assistant",
		"objective":      "Help with the repo",
		"workspace_root": "/tmp/work",
		"session_id":     "sess-front",
	})
	if err != nil {
		t.Fatalf("marshal start payload: %v", err)
	}

	err = cs.AppendEvent(ctx, model.Event{
		ID:             "evt-run-start",
		ConversationID: "conv-session-run",
		RunID:          "run-front",
		Kind:           "run_started",
		PayloadJSON:    startPayload,
	})
	if err != nil {
		t.Fatalf("AppendEvent run_started failed: %v", err)
	}

	var sessionID string
	err = db.RawDB().QueryRowContext(ctx,
		"SELECT session_id FROM runs WHERE id = 'run-front'",
	).Scan(&sessionID)
	if err != nil {
		t.Fatalf("query projected run session_id: %v", err)
	}
	if sessionID != "sess-front" {
		t.Fatalf("expected run session_id %q, got %q", "sess-front", sessionID)
	}
}

func TestConversationStore_AppendEventProjectsSessionBinding(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	bindingPayload, err := json.Marshal(map[string]any{
		"thread_id":    "main",
		"session_id":   "sess-front",
		"connector_id": "telegram",
		"account_id":   "acct-1",
		"external_id":  "chat-1",
		"status":       "active",
	})
	if err != nil {
		t.Fatalf("marshal binding payload: %v", err)
	}

	err = cs.AppendEvent(ctx, model.Event{
		ID:             "evt-session-bound",
		ConversationID: "conv-binding",
		RunID:          "run-front",
		Kind:           "session_bound",
		PayloadJSON:    bindingPayload,
	})
	if err != nil {
		t.Fatalf("AppendEvent session_bound failed: %v", err)
	}

	var sessionID string
	var connectorID string
	var accountID string
	var externalID string
	var status string
	err = db.RawDB().QueryRowContext(ctx,
		`SELECT session_id, connector_id, account_id, external_id, status
		 FROM session_bindings
		 WHERE conversation_id = 'conv-binding' AND thread_id = 'main'`,
	).Scan(&sessionID, &connectorID, &accountID, &externalID, &status)
	if err != nil {
		t.Fatalf("query session binding projection: %v", err)
	}
	if sessionID != "sess-front" || connectorID != "telegram" || accountID != "acct-1" || externalID != "chat-1" || status != "active" {
		t.Fatalf(
			"unexpected session binding projection: session_id=%q connector_id=%q account_id=%q external_id=%q status=%q",
			sessionID,
			connectorID,
			accountID,
			externalID,
			status,
		)
	}
}

func TestConversationStore_AppendEventProjectsSessionBindingReplacementHistory(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	firstPayload, err := json.Marshal(map[string]any{
		"thread_id":    "main",
		"session_id":   "sess-front-1",
		"connector_id": "telegram",
		"account_id":   "acct-1",
		"external_id":  "chat-1",
		"status":       "active",
	})
	if err != nil {
		t.Fatalf("marshal first binding payload: %v", err)
	}
	firstAt := time.Now().UTC().Add(-time.Minute)
	if err := cs.AppendEvent(ctx, model.Event{
		ID:             "evt-session-bound-1",
		ConversationID: "conv-binding",
		RunID:          "run-front-1",
		Kind:           "session_bound",
		PayloadJSON:    firstPayload,
		CreatedAt:      firstAt,
	}); err != nil {
		t.Fatalf("AppendEvent first session_bound failed: %v", err)
	}

	secondPayload, err := json.Marshal(map[string]any{
		"thread_id":    "main",
		"session_id":   "sess-front-2",
		"connector_id": "telegram",
		"account_id":   "acct-1",
		"external_id":  "chat-1",
		"status":       "active",
	})
	if err != nil {
		t.Fatalf("marshal second binding payload: %v", err)
	}
	secondAt := time.Now().UTC()
	if err := cs.AppendEvent(ctx, model.Event{
		ID:             "evt-session-bound-2",
		ConversationID: "conv-binding",
		RunID:          "run-front-2",
		Kind:           "session_bound",
		PayloadJSON:    secondPayload,
		CreatedAt:      secondAt,
	}); err != nil {
		t.Fatalf("AppendEvent second session_bound failed: %v", err)
	}

	rows, err := db.RawDB().QueryContext(ctx,
		`SELECT id, session_id, status, deactivated_at
		 FROM session_bindings
		 WHERE conversation_id = 'conv-binding'
		 ORDER BY created_at ASC, id ASC`,
	)
	if err != nil {
		t.Fatalf("query session binding history: %v", err)
	}
	defer rows.Close()

	type bindingRow struct {
		id            string
		sessionID     string
		status        string
		deactivatedAt sql.NullString
	}
	var bindings []bindingRow
	for rows.Next() {
		var row bindingRow
		if err := rows.Scan(&row.id, &row.sessionID, &row.status, &row.deactivatedAt); err != nil {
			t.Fatalf("scan session binding history: %v", err)
		}
		bindings = append(bindings, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate session binding history: %v", err)
	}
	if len(bindings) != 2 {
		t.Fatalf("expected 2 session bindings, got %d", len(bindings))
	}
	if bindings[0].id != "evt-session-bound-1" || bindings[0].status != "inactive" || !bindings[0].deactivatedAt.Valid {
		t.Fatalf("unexpected first binding history row: %+v", bindings[0])
	}
	if bindings[1].id != "evt-session-bound-2" || bindings[1].status != "active" || bindings[1].deactivatedAt.Valid {
		t.Fatalf("unexpected second binding history row: %+v", bindings[1])
	}
}

func TestConversationStore_AppendEventProjectsSessionUnbound(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	if _, err := db.RawDB().ExecContext(ctx,
		`INSERT INTO session_bindings
		 (id, conversation_id, thread_id, session_id, connector_id, account_id, external_id, status, created_at)
		 VALUES ('bind-1', 'conv-binding', 'main', 'sess-front', 'telegram', 'acct-1', 'chat-1', 'active', datetime('now'))`,
	); err != nil {
		t.Fatalf("insert session binding: %v", err)
	}

	err := cs.AppendEvent(ctx, model.Event{
		ID:             "evt-session-unbound",
		ConversationID: "conv-binding",
		RunID:          "run-front",
		Kind:           "session_unbound",
		PayloadJSON:    []byte(`{"route_id":"bind-1"}`),
	})
	if err != nil {
		t.Fatalf("AppendEvent session_unbound failed: %v", err)
	}

	var status string
	var deactivatedAt sql.NullString
	err = db.RawDB().QueryRowContext(ctx,
		`SELECT status, deactivated_at
		 FROM session_bindings
		 WHERE id = 'bind-1'`,
	).Scan(&status, &deactivatedAt)
	if err != nil {
		t.Fatalf("query session binding projection: %v", err)
	}
	if status != "inactive" || !deactivatedAt.Valid {
		t.Fatalf("expected inactive binding with deactivated_at, got status=%q deactivated_valid=%v", status, deactivatedAt.Valid)
	}
}

func TestConversationStore_AppendEventProjectsDeliveryRedrive(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	_, err := db.RawDB().ExecContext(ctx,
		`INSERT INTO runs
		 (id, conversation_id, agent_id, session_id, objective, workspace_root, status, created_at, updated_at)
		 VALUES ('run-front', 'conv-redrive', 'assistant', 'sess-front', 'inspect repo', ?, 'completed', datetime('now'), datetime('now'))`,
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}
	_, err = db.RawDB().ExecContext(ctx,
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at, last_attempt_at)
		 VALUES ('intent-1', 'run-front', 'telegram', 'chat-1', 'reply', 'dedupe-1', 'terminal', 3, datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert outbound intent: %v", err)
	}

	err = cs.AppendEvent(ctx, model.Event{
		ID:             "evt-redrive",
		ConversationID: "conv-redrive",
		RunID:          "run-front",
		Kind:           "delivery_redrive_requested",
		PayloadJSON:    []byte(`{"intent_id":"intent-1"}`),
	})
	if err != nil {
		t.Fatalf("AppendEvent delivery_redrive_requested failed: %v", err)
	}

	var status string
	var attempts int
	var lastAttempt sql.NullString
	err = db.RawDB().QueryRowContext(ctx,
		`SELECT status, attempts, last_attempt_at
		 FROM outbound_intents
		 WHERE id = 'intent-1'`,
	).Scan(&status, &attempts, &lastAttempt)
	if err != nil {
		t.Fatalf("query outbound intent projection: %v", err)
	}
	if status != "pending" || attempts != 0 || lastAttempt.Valid {
		t.Fatalf("unexpected redrive projection status=%q attempts=%d last_attempt_valid=%v", status, attempts, lastAttempt.Valid)
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
