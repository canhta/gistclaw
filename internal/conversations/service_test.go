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

func TestConversationStore_FindDoesNotCreateConversation(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	key := ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct-1",
		ExternalID:  "chat-1",
		ThreadID:    "main",
	}

	conv, found, err := cs.Find(ctx, key)
	if err != nil {
		t.Fatalf("Find missing conversation failed: %v", err)
	}
	if found {
		t.Fatalf("expected missing conversation, got %+v", conv)
	}

	created, err := cs.Resolve(ctx, key)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	foundConv, found, err := cs.Find(ctx, key)
	if err != nil {
		t.Fatalf("Find existing conversation failed: %v", err)
	}
	if !found {
		t.Fatal("expected existing conversation to be found")
	}
	if foundConv.ID != created.ID {
		t.Fatalf("expected found conversation %q, got %q", created.ID, foundConv.ID)
	}
}

func TestConversationStore_AppendEventProjectsRunLifecycle(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	payload, err := json.Marshal(map[string]any{
		"agent_id":       "agent-a",
		"objective":      "ship it",
		"cwd":            t.TempDir(),
		"authority_json": json.RawMessage(`{"approval_mode":"prompt"}`),
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
		"model_id":      "gpt-5.4",
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
		"model_id":      "gpt-5.4",
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
	var modelLane string
	var modelID string
	err = db.RawDB().QueryRowContext(ctx,
		"SELECT status, input_tokens, output_tokens, COALESCE(model_lane, ''), COALESCE(model_id, '') FROM runs WHERE id = 'run-life'",
	).Scan(&status, &inputTokens, &outputTokens, &modelLane, &modelID)
	if err != nil {
		t.Fatalf("query run projection: %v", err)
	}
	if status != "completed" || inputTokens != 10 || outputTokens != 20 || modelLane != "cheap" || modelID != "gpt-5.4" {
		t.Fatalf("unexpected run projection: status=%s input=%d output=%d lane=%s model=%s", status, inputTokens, outputTokens, modelLane, modelID)
	}

	var receiptCount int
	var receiptModelID string
	err = db.RawDB().QueryRowContext(ctx,
		"SELECT count(*), COALESCE(MAX(model_id), '') FROM receipts WHERE run_id = 'run-life'",
	).Scan(&receiptCount, &receiptModelID)
	if err != nil {
		t.Fatalf("query receipt projection: %v", err)
	}
	if receiptCount != 1 {
		t.Fatalf("expected 1 receipt, got %d", receiptCount)
	}
	if receiptModelID != "gpt-5.4" {
		t.Fatalf("expected receipt model_id gpt-5.4, got %q", receiptModelID)
	}
}

func TestConversationStore_AppendEventProjectsApprovalLifecycle(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	startPayload, err := json.Marshal(map[string]any{
		"agent_id":       "agent-a",
		"objective":      "edit repo",
		"cwd":            t.TempDir(),
		"authority_json": json.RawMessage(`{"approval_mode":"prompt"}`),
	})
	if err != nil {
		t.Fatalf("marshal start payload: %v", err)
	}

	if err := cs.AppendEvent(ctx, model.Event{
		ID:             "evt-approval-run-started",
		ConversationID: "conv-approval",
		RunID:          "run-approval",
		Kind:           "run_started",
		PayloadJSON:    startPayload,
	}); err != nil {
		t.Fatalf("AppendEvent run_started failed: %v", err)
	}

	if err := cs.AppendEvent(ctx, model.Event{
		ID:             "evt-approval-requested",
		ConversationID: "conv-approval",
		RunID:          "run-approval",
		Kind:           "approval_requested",
		PayloadJSON:    []byte(`{"approval_id":"ticket-1"}`),
	}); err != nil {
		t.Fatalf("AppendEvent approval_requested failed: %v", err)
	}

	var status string
	if err := db.RawDB().QueryRowContext(ctx,
		"SELECT status FROM runs WHERE id = 'run-approval'",
	).Scan(&status); err != nil {
		t.Fatalf("query needs approval status: %v", err)
	}
	if status != "needs_approval" {
		t.Fatalf("expected needs_approval after approval_requested, got %q", status)
	}

	if err := cs.AppendEvent(ctx, model.Event{
		ID:             "evt-run-resumed",
		ConversationID: "conv-approval",
		RunID:          "run-approval",
		Kind:           "run_resumed",
		PayloadJSON:    []byte(`{"approval_id":"ticket-1","decision":"approved"}`),
	}); err != nil {
		t.Fatalf("AppendEvent run_resumed failed: %v", err)
	}

	if err := db.RawDB().QueryRowContext(ctx,
		"SELECT status FROM runs WHERE id = 'run-approval'",
	).Scan(&status); err != nil {
		t.Fatalf("query resumed status: %v", err)
	}
	if status != "active" {
		t.Fatalf("expected active after run_resumed, got %q", status)
	}
}

func TestConversationStore_AppendEventProjectsConversationGateLifecycle(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	startPayload, err := json.Marshal(map[string]any{
		"agent_id":       "assistant",
		"objective":      "inspect repo",
		"cwd":            t.TempDir(),
		"authority_json": json.RawMessage(`{"approval_mode":"prompt"}`),
		"session_id":     "sess-front",
	})
	if err != nil {
		t.Fatalf("marshal start payload: %v", err)
	}

	if err := cs.AppendEvent(ctx, model.Event{
		ID:             "evt-gate-run-started",
		ConversationID: "conv-gate",
		RunID:          "run-gate",
		Kind:           "run_started",
		PayloadJSON:    startPayload,
	}); err != nil {
		t.Fatalf("AppendEvent run_started failed: %v", err)
	}

	openPayload, err := json.Marshal(map[string]any{
		"gate_id":       "gate-1",
		"session_id":    "sess-front",
		"kind":          "approval",
		"status":        "pending",
		"approval_id":   "ticket-1",
		"title":         "Approval required",
		"body":          "Approve the shell command",
		"options":       []string{"approve", "deny"},
		"metadata":      map[string]any{"tool_name": "shell_exec"},
		"language_hint": "vi",
	})
	if err != nil {
		t.Fatalf("marshal gate open payload: %v", err)
	}

	if err := cs.AppendEvent(ctx, model.Event{
		ID:             "evt-gate-opened",
		ConversationID: "conv-gate",
		RunID:          "run-gate",
		Kind:           "conversation_gate_opened",
		PayloadJSON:    openPayload,
	}); err != nil {
		t.Fatalf("AppendEvent conversation_gate_opened failed: %v", err)
	}

	var kind string
	var status string
	var approvalID string
	var sessionID string
	var title string
	var body string
	var optionsJSON string
	var metadataJSON string
	var languageHint string
	err = db.RawDB().QueryRowContext(ctx,
		`SELECT kind, status, COALESCE(approval_id, ''), session_id, title, body, options_json, metadata_json, COALESCE(language_hint, '')
		 FROM conversation_gates
		 WHERE id = 'gate-1'`,
	).Scan(&kind, &status, &approvalID, &sessionID, &title, &body, &optionsJSON, &metadataJSON, &languageHint)
	if err != nil {
		t.Fatalf("query conversation gate projection: %v", err)
	}
	if kind != "approval" || status != "pending" || approvalID != "ticket-1" || sessionID != "sess-front" {
		t.Fatalf("unexpected gate projection kind=%q status=%q approval_id=%q session_id=%q", kind, status, approvalID, sessionID)
	}
	if title != "Approval required" || body != "Approve the shell command" {
		t.Fatalf("unexpected gate text title=%q body=%q", title, body)
	}
	if !strings.Contains(optionsJSON, `"approve"`) || !strings.Contains(metadataJSON, `"tool_name":"shell_exec"`) {
		t.Fatalf("expected options/metadata to round-trip, got options=%q metadata=%q", optionsJSON, metadataJSON)
	}
	if languageHint != "vi" {
		t.Fatalf("expected language hint %q, got %q", "vi", languageHint)
	}

	resolvePayload, err := json.Marshal(map[string]any{
		"gate_id":  "gate-1",
		"status":   "resolved",
		"decision": "approved",
	})
	if err != nil {
		t.Fatalf("marshal gate resolve payload: %v", err)
	}

	if err := cs.AppendEvent(ctx, model.Event{
		ID:             "evt-gate-resolved",
		ConversationID: "conv-gate",
		RunID:          "run-gate",
		Kind:           "conversation_gate_resolved",
		PayloadJSON:    resolvePayload,
	}); err != nil {
		t.Fatalf("AppendEvent conversation_gate_resolved failed: %v", err)
	}

	var resolvedStatus string
	var resolvedAt sql.NullString
	err = db.RawDB().QueryRowContext(ctx,
		`SELECT status, resolved_at
		 FROM conversation_gates
		 WHERE id = 'gate-1'`,
	).Scan(&resolvedStatus, &resolvedAt)
	if err != nil {
		t.Fatalf("query resolved conversation gate: %v", err)
	}
	if resolvedStatus != "resolved" {
		t.Fatalf("expected resolved gate status, got %q", resolvedStatus)
	}
	if !resolvedAt.Valid {
		t.Fatal("expected resolved_at to be populated")
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
		"cwd":            "/tmp/work",
		"authority_json": json.RawMessage(`{"approval_mode":"prompt"}`),
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
	var reason string
	var replacedBy sql.NullString
	if err := db.RawDB().QueryRowContext(ctx,
		`SELECT deactivation_reason, replaced_by_route_id
		 FROM session_bindings
		 WHERE id = 'evt-session-bound-1'`,
	).Scan(&reason, &replacedBy); err != nil {
		t.Fatalf("query first binding reason: %v", err)
	}
	if bindings[0].id != "evt-session-bound-1" || bindings[0].status != "inactive" || !bindings[0].deactivatedAt.Valid {
		t.Fatalf("unexpected first binding history row: %+v", bindings[0])
	}
	if reason != "replaced" || !replacedBy.Valid || replacedBy.String != "evt-session-bound-2" {
		t.Fatalf("unexpected first binding deactivation metadata reason=%q replaced_by=%q", reason, replacedBy.String)
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
	var reason string
	err = db.RawDB().QueryRowContext(ctx,
		`SELECT status, deactivated_at, deactivation_reason
		 FROM session_bindings
		 WHERE id = 'bind-1'`,
	).Scan(&status, &deactivatedAt, &reason)
	if err != nil {
		t.Fatalf("query session binding projection: %v", err)
	}
	if status != "inactive" || !deactivatedAt.Valid || reason != "deactivated" {
		t.Fatalf("expected inactive binding with deactivation metadata, got status=%q deactivated_valid=%v reason=%q", status, deactivatedAt.Valid, reason)
	}
}

func TestConversationStore_AppendEventProjectsDeliveryRedrive(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	_, err := db.RawDB().ExecContext(ctx,
		`INSERT INTO runs
		 (id, conversation_id, agent_id, session_id, objective, cwd, authority_json, status, created_at, updated_at)
		 VALUES ('run-front', 'conv-redrive', 'assistant', 'sess-front', 'inspect repo', ?, ?, 'completed', datetime('now'), datetime('now'))`,
		t.TempDir(),
		[]byte(`{"approval_mode":"prompt"}`),
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

func TestConversationStore_ResolvePersistsStructuredKeyFields(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	key := ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct-1",
		ExternalID:  "chat-1",
		ThreadID:    "main",
		ProjectID:   "proj-1",
	}

	conv, err := cs.Resolve(ctx, key)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	var connectorID, accountID, externalID, threadID, projectID string
	err = db.RawDB().QueryRowContext(ctx,
		`SELECT connector_id, account_id, external_id, thread_id, project_id
		 FROM conversations
		 WHERE id = ?`,
		conv.ID,
	).Scan(&connectorID, &accountID, &externalID, &threadID, &projectID)
	if err != nil {
		t.Fatalf("query conversation fields: %v", err)
	}

	if connectorID != key.ConnectorID || accountID != key.AccountID || externalID != key.ExternalID || threadID != key.ThreadID || projectID != key.ProjectID {
		t.Fatalf(
			"unexpected structured conversation fields connector=%q account=%q external=%q thread=%q project=%q",
			connectorID,
			accountID,
			externalID,
			threadID,
			projectID,
		)
	}
}

func TestConversationStore_ResetConversationDeletesOnlyTargetData(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	for _, stmt := range []string{
		`INSERT INTO conversations (id, key, connector_id, account_id, external_id, thread_id, project_id, created_at)
		 VALUES ('conv-reset', 'telegram|acct-1|chat-1|main', 'telegram', 'acct-1', 'chat-1', 'main', 'proj-reset', datetime('now')),
		        ('conv-keep', 'telegram|acct-2|chat-2|main', 'telegram', 'acct-2', 'chat-2', 'main', 'proj-keep', datetime('now'))`,
		`INSERT INTO runs (id, conversation_id, agent_id, session_id, status, created_at, updated_at)
		 VALUES ('run-reset', 'conv-reset', 'assistant', 'sess-reset', 'completed', datetime('now'), datetime('now')),
		        ('run-keep', 'conv-keep', 'assistant', 'sess-keep', 'completed', datetime('now'), datetime('now'))`,
		`INSERT INTO sessions (id, conversation_id, key, agent_id, role, status, created_at)
		 VALUES ('sess-reset', 'conv-reset', 'front-reset', 'assistant', 'front', 'active', datetime('now')),
		        ('sess-keep', 'conv-keep', 'front-keep', 'assistant', 'front', 'active', datetime('now'))`,
		`INSERT INTO session_messages (id, session_id, sender_session_id, kind, body, created_at)
		 VALUES ('msg-reset', 'sess-reset', '', 'user', 'reset me', datetime('now')),
		        ('msg-keep', 'sess-keep', '', 'user', 'keep me', datetime('now'))`,
		`INSERT INTO session_bindings (id, conversation_id, thread_id, session_id, connector_id, account_id, external_id, status, created_at)
		 VALUES ('bind-reset', 'conv-reset', 'main', 'sess-reset', 'telegram', 'acct-1', 'chat-1', 'active', datetime('now')),
		        ('bind-keep', 'conv-keep', 'main', 'sess-keep', 'telegram', 'acct-2', 'chat-2', 'active', datetime('now'))`,
		`INSERT INTO inbound_receipts (id, conversation_id, connector_id, account_id, thread_id, source_message_id, run_id, session_id, session_message_id, created_at)
		 VALUES ('receipt-reset', 'conv-reset', 'telegram', 'acct-1', 'main', 'tg-1', 'run-reset', 'sess-reset', 'msg-reset', datetime('now')),
		        ('receipt-keep', 'conv-keep', 'telegram', 'acct-2', 'main', 'tg-2', 'run-keep', 'sess-keep', 'msg-keep', datetime('now'))`,
		`INSERT INTO tool_calls (id, run_id, tool_name, decision, created_at)
		 VALUES ('tool-reset', 'run-reset', 'exec', 'allow', datetime('now')),
		        ('tool-keep', 'run-keep', 'exec', 'allow', datetime('now'))`,
		`INSERT INTO approvals (id, run_id, tool_name, fingerprint, status, created_at)
		 VALUES ('approval-reset', 'run-reset', 'exec', 'fp-reset', 'approved', datetime('now')),
		        ('approval-keep', 'run-keep', 'exec', 'fp-keep', 'approved', datetime('now'))`,
		`INSERT INTO receipts (id, run_id, created_at)
		 VALUES ('run-receipt-reset', 'run-reset', datetime('now')),
		        ('run-receipt-keep', 'run-keep', datetime('now'))`,
		`INSERT INTO outbound_intents (id, run_id, connector_id, chat_id, message_text, status, attempts, created_at)
		 VALUES ('intent-reset', 'run-reset', 'telegram', 'chat-1', 'reset reply', 'pending', 0, datetime('now')),
		        ('intent-keep', 'run-keep', 'telegram', 'chat-2', 'keep reply', 'pending', 0, datetime('now'))`,
		`INSERT INTO run_summaries (id, run_id, project_id, content, token_count, created_at, updated_at)
		 VALUES ('summary-reset', 'run-reset', 'proj-reset', 'reset summary', 10, datetime('now'), datetime('now')),
		        ('summary-keep', 'run-keep', 'proj-keep', 'keep summary', 10, datetime('now'), datetime('now'))`,
		`INSERT INTO events (id, conversation_id, run_id, kind, payload_json, created_at)
		 VALUES ('evt-reset', 'conv-reset', 'run-reset', 'run_started', x'7b7d', datetime('now')),
		        ('evt-keep', 'conv-keep', 'run-keep', 'run_started', x'7b7d', datetime('now'))`,
		`INSERT INTO memory_items (id, project_id, agent_id, scope, content, source, created_at, updated_at)
		 VALUES ('memory-1', 'proj-memory', 'assistant', 'local', 'keep memory', 'manual', datetime('now'), datetime('now'))`,
	} {
		if _, err := db.RawDB().ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed fixture: %v", err)
		}
	}

	if err := cs.ResetConversation(ctx, "conv-reset"); err != nil {
		t.Fatalf("ResetConversation failed: %v", err)
	}

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{name: "reset conversation deleted", query: `SELECT count(*) FROM conversations WHERE id = 'conv-reset'`, want: 0},
		{name: "reset runs deleted", query: `SELECT count(*) FROM runs WHERE conversation_id = 'conv-reset'`, want: 0},
		{name: "reset sessions deleted", query: `SELECT count(*) FROM sessions WHERE conversation_id = 'conv-reset'`, want: 0},
		{name: "reset session messages deleted", query: `SELECT count(*) FROM session_messages WHERE session_id = 'sess-reset'`, want: 0},
		{name: "reset bindings deleted", query: `SELECT count(*) FROM session_bindings WHERE conversation_id = 'conv-reset'`, want: 0},
		{name: "reset inbound receipts deleted", query: `SELECT count(*) FROM inbound_receipts WHERE conversation_id = 'conv-reset'`, want: 0},
		{name: "reset tool calls deleted", query: `SELECT count(*) FROM tool_calls WHERE run_id = 'run-reset'`, want: 0},
		{name: "reset approvals deleted", query: `SELECT count(*) FROM approvals WHERE run_id = 'run-reset'`, want: 0},
		{name: "reset receipts deleted", query: `SELECT count(*) FROM receipts WHERE run_id = 'run-reset'`, want: 0},
		{name: "reset outbound intents deleted", query: `SELECT count(*) FROM outbound_intents WHERE run_id = 'run-reset'`, want: 0},
		{name: "reset summaries deleted", query: `SELECT count(*) FROM run_summaries WHERE run_id = 'run-reset'`, want: 0},
		{name: "reset events deleted", query: `SELECT count(*) FROM events WHERE conversation_id = 'conv-reset'`, want: 0},
		{name: "keep conversation preserved", query: `SELECT count(*) FROM conversations WHERE id = 'conv-keep'`, want: 1},
		{name: "keep runs preserved", query: `SELECT count(*) FROM runs WHERE conversation_id = 'conv-keep'`, want: 1},
		{name: "memory preserved", query: `SELECT count(*) FROM memory_items WHERE id = 'memory-1'`, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got int
			if err := db.RawDB().QueryRowContext(ctx, tt.query).Scan(&got); err != nil {
				t.Fatalf("query count: %v", err)
			}
			if got != tt.want {
				t.Fatalf("%s: got %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

func TestConversationStore_ResetConversationIsAtomic(t *testing.T) {
	db := setupTestStore(t)
	cs := NewConversationStore(db)
	ctx := context.Background()

	for _, stmt := range []string{
		`INSERT INTO conversations (id, key, connector_id, account_id, external_id, thread_id, project_id, created_at)
		 VALUES ('conv-reset', 'telegram|acct-1|chat-1|main', 'telegram', 'acct-1', 'chat-1', 'main', 'proj-reset', datetime('now'))`,
		`INSERT INTO runs (id, conversation_id, agent_id, session_id, status, created_at, updated_at)
		 VALUES ('run-reset', 'conv-reset', 'assistant', 'sess-reset', 'completed', datetime('now'), datetime('now'))`,
		`INSERT INTO sessions (id, conversation_id, key, agent_id, role, status, created_at)
		 VALUES ('sess-reset', 'conv-reset', 'front-reset', 'assistant', 'front', 'active', datetime('now'))`,
		`INSERT INTO session_messages (id, session_id, sender_session_id, kind, body, created_at)
		 VALUES ('msg-reset', 'sess-reset', '', 'user', 'reset me', datetime('now'))`,
		`CREATE TRIGGER fail_runs_delete
		 BEFORE DELETE ON runs
		 BEGIN
		   SELECT RAISE(ABORT, 'forced runs delete failure');
		 END;`,
	} {
		if _, err := db.RawDB().ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed fixture: %v", err)
		}
	}

	err := cs.ResetConversation(ctx, "conv-reset")
	if err == nil || !strings.Contains(err.Error(), "delete runs") {
		t.Fatalf("expected delete runs failure, got %v", err)
	}

	for _, tt := range []struct {
		name  string
		query string
		want  int
	}{
		{name: "conversation restored", query: `SELECT count(*) FROM conversations WHERE id = 'conv-reset'`, want: 1},
		{name: "run restored", query: `SELECT count(*) FROM runs WHERE id = 'run-reset'`, want: 1},
		{name: "session restored", query: `SELECT count(*) FROM sessions WHERE id = 'sess-reset'`, want: 1},
		{name: "session message restored", query: `SELECT count(*) FROM session_messages WHERE id = 'msg-reset'`, want: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got int
			if err := db.RawDB().QueryRowContext(ctx, tt.query).Scan(&got); err != nil {
				t.Fatalf("query count: %v", err)
			}
			if got != tt.want {
				t.Fatalf("%s: got %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}
