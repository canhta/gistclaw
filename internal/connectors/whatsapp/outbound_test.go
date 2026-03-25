package whatsapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

func setupDB(t *testing.T) (*store.DB, *conversations.ConversationStore) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, conversations.NewConversationStore(db)
}

func seedOutboundRun(t *testing.T, db *store.DB, cs *conversations.ConversationStore, runID string) string {
	t.Helper()

	conv, err := cs.Resolve(context.Background(), conversations.ConversationKey{
		ConnectorID: "whatsapp",
		AccountID:   "phone-123",
		ExternalID:  "15551234567",
		ThreadID:    "main",
	})
	if err != nil {
		t.Fatalf("Resolve conversation: %v", err)
	}

	_, err = db.RawDB().Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
		 VALUES (?, ?, ?, 'completed', datetime('now'), datetime('now'))`,
		runID, conv.ID, "assistant",
	)
	if err != nil {
		t.Fatalf("seed run: %v", err)
	}

	return conv.ID
}

func TestOutboundDispatcher_IDReturnsWhatsapp(t *testing.T) {
	db, cs := setupDB(t)
	d := NewOutboundDispatcher("phone-id", "token", db, cs)
	if d.ID() != "whatsapp" {
		t.Errorf("ID: got %q, want %q", d.ID(), "whatsapp")
	}
}

func TestOutboundDispatcher_NotifySendsToMetaAPI(t *testing.T) {
	var capturedBodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		capturedBodies = append(capturedBodies, body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"messages":[{"id":"wamid.test"}]}`))
	}))
	defer srv.Close()

	db, cs := setupDB(t)
	d := newWithBaseURL("phone-123", "test-token", db, cs, srv.URL)

	delta := model.ReplayDelta{
		RunID:      "run-1",
		Kind:       "run_completed",
		OccurredAt: time.Now(),
	}
	if err := d.Notify(context.Background(), "+1234567890", delta, "dedupe-1"); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	if len(capturedBodies) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(capturedBodies))
	}
	body := capturedBodies[0]
	if body["messaging_product"] != "whatsapp" {
		t.Errorf("messaging_product: got %v, want whatsapp", body["messaging_product"])
	}
	if body["to"] != "+1234567890" {
		t.Errorf("to: got %v, want +1234567890", body["to"])
	}
	textBlock, _ := body["text"].(map[string]any)
	if textBlock == nil || textBlock["body"] == "" {
		t.Errorf("text.body: missing or empty")
	}
}

func TestOutboundDispatcher_SendWritesControlReplyIntent(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"messages":[{"id":"wamid.control"}]}`))
	}))
	defer srv.Close()

	db, cs := setupDB(t)
	d := newWithBaseURL("phone-123", "test-token", db, cs, srv.URL)

	if err := d.Send(context.Background(), "+1234567890", "native help reply"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 API call, got %d", callCount)
	}

	var status string
	var runID sql.NullString
	if err := db.RawDB().QueryRowContext(context.Background(),
		`SELECT status, run_id FROM outbound_intents WHERE chat_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`,
		"+1234567890",
	).Scan(&status, &runID); err != nil {
		t.Fatalf("query control reply intent: %v", err)
	}
	if status != "delivered" {
		t.Fatalf("expected delivered control reply, got %q", status)
	}
	if runID.Valid {
		t.Fatalf("expected control reply run_id to be NULL, got %q", runID.String)
	}
}

func TestOutboundDispatcher_NotifyDeduplicates(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte(`{"messages":[{"id":"wamid.test"}]}`))
	}))
	defer srv.Close()

	db, cs := setupDB(t)
	d := newWithBaseURL("phone-123", "token", db, cs, srv.URL)

	delta := model.ReplayDelta{RunID: "run-2", Kind: "run_completed", OccurredAt: time.Now()}
	_ = d.Notify(context.Background(), "+1234567890", delta, "dedupe-same")
	_ = d.Notify(context.Background(), "+1234567890", delta, "dedupe-same")

	if callCount != 1 {
		t.Errorf("expected 1 API call (deduplication), got %d", callCount)
	}
}

func TestOutboundDispatcher_IgnoredEventKindIsNoOp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	db, cs := setupDB(t)
	d := newWithBaseURL("phone-123", "token", db, cs, srv.URL)

	// "memory_context_loaded" is not in the allowed set.
	delta := model.ReplayDelta{RunID: "run-3", Kind: "memory_context_loaded", OccurredAt: time.Now()}
	if err := d.Notify(context.Background(), "+1234567890", delta, "dedupe-ignored"); err != nil {
		t.Fatalf("Notify: unexpected error: %v", err)
	}
}

func TestOutboundDispatcher_DrainDeliversPending(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte(`{"messages":[{"id":"wamid.test"}]}`))
	}))
	defer srv.Close()

	db, cs := setupDB(t)
	d := newWithBaseURL("phone-123", "token", db, cs, srv.URL)

	// Insert a pending intent directly.
	_, err := db.RawDB().Exec(
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at)
		 VALUES ('intent-drain', 'run-drain', 'whatsapp', '+999', 'hello', 'dk-drain', 'pending', 0, datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert intent: %v", err)
	}

	if err := d.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if callCount == 0 {
		t.Error("expected at least one API call during Drain")
	}
}

func TestOutboundDispatcher_StartIsNoOp(t *testing.T) {
	db, cs := setupDB(t)
	d := NewOutboundDispatcher("phone-id", "token", db, cs)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Start with already-cancelled context should return quickly.
	_ = d.Start(ctx)
}

func TestOutboundDispatcher_TerminalFailureJournalsDeliveryFailed(t *testing.T) {
	db, cs := setupDB(t)
	convID := seedOutboundRun(t, db, cs, "run-whatsapp-terminal")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "always fail", http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := newWithBaseURL("phone-123", "token", db, cs, srv.URL)
	d.retryDelay = 0
	d.maxAttempts = 3

	err := d.Notify(context.Background(), "+15551234567", model.ReplayDelta{
		RunID:      "run-whatsapp-terminal",
		Kind:       "run_completed",
		OccurredAt: time.Now(),
	}, "dedupe-terminal")
	if err == nil {
		t.Fatal("expected Notify to fail after max attempts")
	}

	var status string
	if err := db.RawDB().QueryRow(
		"SELECT status FROM outbound_intents WHERE dedupe_key = 'dedupe-terminal'",
	).Scan(&status); err != nil {
		t.Fatalf("query outbound intent status: %v", err)
	}
	if status != "terminal" {
		t.Fatalf("expected terminal status after max attempts, got %q", status)
	}

	var eventConversationID string
	var eventRunID string
	if err := db.RawDB().QueryRow(
		"SELECT conversation_id, run_id FROM events WHERE kind = 'delivery_failed' ORDER BY created_at DESC, id DESC LIMIT 1",
	).Scan(&eventConversationID, &eventRunID); err != nil {
		t.Fatalf("query delivery_failed event: %v", err)
	}
	if eventConversationID != convID || eventRunID != "run-whatsapp-terminal" {
		t.Fatalf("expected delivery_failed to attach to conversation=%q run=%q, got conversation=%q run=%q", convID, "run-whatsapp-terminal", eventConversationID, eventRunID)
	}
}
