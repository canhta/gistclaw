package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

func setupOutboundDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedOutboundRun(t *testing.T, db *store.DB, cs *conversations.ConversationStore, runID string) string {
	t.Helper()

	conv, err := cs.Resolve(context.Background(), conversations.ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct-1",
		ExternalID:  "chat-1",
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

func TestOutbound_StartedEventDelivers(t *testing.T) {
	db := setupOutboundDB(t)
	cs := conversations.NewConversationStore(db)

	var delivered atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "sendMessage") {
			delivered.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
		}
	}))
	defer srv.Close()

	dispatcher := NewOutboundDispatcher("testtoken", db, cs)
	dispatcher.bot.apiBase = srv.URL + "/bot"

	chatID := "999"
	runID := "run-start-1"
	if err := dispatcher.Notify(context.Background(), chatID, model.ReplayDelta{
		RunID: runID,
		Kind:  "run_started",
	}, runID+"-started"); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	if delivered.Load() == 0 {
		t.Fatal("expected sendMessage call for run_started event")
	}
}

func TestOutbound_BlockedEventDelivers(t *testing.T) {
	db := setupOutboundDB(t)
	cs := conversations.NewConversationStore(db)

	var delivered atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "sendMessage") {
			delivered.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":2}}`))
		}
	}))
	defer srv.Close()

	dispatcher := NewOutboundDispatcher("testtoken", db, cs)
	dispatcher.bot.apiBase = srv.URL + "/bot"

	if err := dispatcher.Notify(context.Background(), "111", model.ReplayDelta{
		RunID: "run-blocked",
		Kind:  "run_blocked",
	}, "run-blocked-key"); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	if delivered.Load() == 0 {
		t.Fatal("expected sendMessage for run_blocked event")
	}
}

func TestOutbound_IgnoredEventTypes(t *testing.T) {
	db := setupOutboundDB(t)
	cs := conversations.NewConversationStore(db)

	var sendCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "sendMessage") {
			sendCount.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer srv.Close()

	dispatcher := NewOutboundDispatcher("testtoken", db, cs)
	dispatcher.bot.apiBase = srv.URL + "/bot"

	for _, kind := range []string{"tokens_charged", "memory_promoted", "turn_started"} {
		_ = dispatcher.Notify(context.Background(), "222", model.ReplayDelta{
			RunID: "run-ignored",
			Kind:  kind,
		}, "key-"+kind)
	}

	if sendCount.Load() != 0 {
		t.Fatalf("expected 0 sendMessage calls for ignored event types, got %d", sendCount.Load())
	}
}

func TestOutbound_DedupeKeyPreventsDoubleDelivery(t *testing.T) {
	db := setupOutboundDB(t)
	cs := conversations.NewConversationStore(db)

	var delivered atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "sendMessage") {
			delivered.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":3}}`))
		}
	}))
	defer srv.Close()

	dispatcher := NewOutboundDispatcher("testtoken", db, cs)
	dispatcher.bot.apiBase = srv.URL + "/bot"

	dedupeKey := "unique-key-abc"
	delta := model.ReplayDelta{RunID: "run-dedup", Kind: "run_started"}

	// First notification — should deliver.
	_ = dispatcher.Notify(context.Background(), "333", delta, dedupeKey)
	// Second notification with same key — must not deliver again.
	_ = dispatcher.Notify(context.Background(), "333", delta, dedupeKey)

	if delivered.Load() > 1 {
		t.Fatalf("expected at most 1 delivery with same dedupe key, got %d", delivered.Load())
	}
}

func TestOutbound_FinishedEventDelivers(t *testing.T) {
	db := setupOutboundDB(t)
	cs := conversations.NewConversationStore(db)

	var delivered atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "sendMessage") {
			delivered.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":4}}`))
		}
	}))
	defer srv.Close()

	dispatcher := NewOutboundDispatcher("testtoken", db, cs)
	dispatcher.bot.apiBase = srv.URL + "/bot"

	if err := dispatcher.Notify(context.Background(), "444", model.ReplayDelta{
		RunID: "run-fin",
		Kind:  "run_completed",
	}, "run-fin-key"); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	if delivered.Load() == 0 {
		t.Fatal("expected sendMessage for run_completed event")
	}
}

// ── Retry queue tests ─────────────────────────────────────────────────────────

func TestRetry_RetriesOnAPIError(t *testing.T) {
	db := setupOutboundDB(t)
	cs := conversations.NewConversationStore(db)

	var callCount atomic.Int32
	// Fail first 2 calls, succeed on 3rd.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "sendMessage") {
			n := callCount.Add(1)
			if n < 3 {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":10}}`))
		}
	}))
	defer srv.Close()

	dispatcher := NewOutboundDispatcher("testtoken", db, cs)
	dispatcher.bot.apiBase = srv.URL + "/bot"
	dispatcher.retryDelay = 0 // no real delay in tests

	if err := dispatcher.Notify(context.Background(), "555", model.ReplayDelta{
		RunID: "run-retry",
		Kind:  "run_started",
	}, "run-retry-key"); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	if callCount.Load() < 3 {
		t.Fatalf("expected at least 3 attempts (retry until success), got %d", callCount.Load())
	}
}

func TestRetry_TerminalAfterMaxAttempts(t *testing.T) {
	db := setupOutboundDB(t)
	cs := conversations.NewConversationStore(db)
	convID := seedOutboundRun(t, db, cs, "run-terminal")

	var callCount atomic.Int32
	// Always fail.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "sendMessage") {
			callCount.Add(1)
			http.Error(w, "always fail", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	dispatcher := NewOutboundDispatcher("testtoken", db, cs)
	dispatcher.bot.apiBase = srv.URL + "/bot"
	dispatcher.retryDelay = 0
	dispatcher.maxAttempts = 5

	_ = dispatcher.Notify(context.Background(), "666", model.ReplayDelta{
		RunID: "run-terminal",
		Kind:  "run_started",
	}, "run-terminal-key")

	if callCount.Load() != 5 {
		t.Fatalf("expected exactly 5 attempts (max), got %d", callCount.Load())
	}

	// delivery_failed event must be in the journal.
	var count int
	_ = db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE kind = 'delivery_failed'",
	).Scan(&count)
	if count == 0 {
		t.Fatal("expected delivery_failed journal event after max attempts")
	}

	var eventConversationID string
	var eventRunID string
	_ = db.RawDB().QueryRow(
		"SELECT conversation_id, run_id FROM events WHERE kind = 'delivery_failed' ORDER BY created_at DESC, id DESC LIMIT 1",
	).Scan(&eventConversationID, &eventRunID)
	if eventConversationID != convID || eventRunID != "run-terminal" {
		t.Fatalf("expected delivery_failed event to attach to conversation=%q run=%q, got conversation=%q run=%q", convID, "run-terminal", eventConversationID, eventRunID)
	}
}

func TestRetry_DeliveryFailedPayload(t *testing.T) {
	db := setupOutboundDB(t)
	cs := conversations.NewConversationStore(db)
	seedOutboundRun(t, db, cs, "run-payload")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer srv.Close()

	dispatcher := NewOutboundDispatcher("testtoken", db, cs)
	dispatcher.bot.apiBase = srv.URL + "/bot"
	dispatcher.retryDelay = 0
	dispatcher.maxAttempts = 1

	_ = dispatcher.Notify(context.Background(), "777", model.ReplayDelta{
		RunID: "run-payload",
		Kind:  "run_started",
	}, "run-payload-key")

	var payload string
	_ = db.RawDB().QueryRow(
		"SELECT payload_json FROM events WHERE kind = 'delivery_failed'",
	).Scan(&payload)

	var m map[string]any
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		t.Fatalf("delivery_failed payload is not valid JSON: %v", err)
	}
	for _, field := range []string{"intent_id", "chat_id", "event_kind", "error"} {
		if m[field] == nil {
			t.Errorf("delivery_failed payload missing field %q", field)
		}
	}
}

func TestRetry_DrainPendingOnStartup(t *testing.T) {
	db := setupOutboundDB(t)
	cs := conversations.NewConversationStore(db)

	// Seed a pending intent manually.
	_, err := db.RawDB().Exec(
		`INSERT INTO outbound_intents (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts)
		 VALUES ('intent-1', 'run-drain', 'telegram', '888', 'hello', 'drain-key', 'pending', 0)`,
	)
	if err != nil {
		t.Fatalf("seed intent: %v", err)
	}

	var delivered atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "sendMessage") {
			delivered.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":20}}`))
		}
	}))
	defer srv.Close()

	dispatcher := NewOutboundDispatcher("testtoken", db, cs)
	dispatcher.bot.apiBase = srv.URL + "/bot"
	dispatcher.retryDelay = 0

	if err := dispatcher.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	if delivered.Load() == 0 {
		t.Fatal("expected pending intent to be delivered during Drain")
	}
}
