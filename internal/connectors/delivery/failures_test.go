package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
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

func seedRun(t *testing.T, db *store.DB, cs *conversations.ConversationStore, runID string) string {
	t.Helper()

	conv, err := cs.Resolve(context.Background(), conversations.ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct-1",
		ExternalID:  "chat-1",
		ThreadID:    "main",
	})
	if err != nil {
		t.Fatalf("resolve conversation: %v", err)
	}

	_, err = db.RawDB().Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
		 VALUES (?, ?, ?, 'completed', datetime('now'), datetime('now'))`,
		runID, conv.ID, "assistant",
	)
	if err != nil {
		t.Fatalf("seed run: %v", err)
	}

	_, err = db.RawDB().Exec(
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at)
		 VALUES ('intent-1', ?, 'telegram', 'chat-1', 'hello', 'dedupe-1', 'terminal', 3, datetime('now'))`,
		runID,
	)
	if err != nil {
		t.Fatalf("seed outbound intent: %v", err)
	}

	return conv.ID
}

func TestAppendDeliveryFailedEvent_AppendsRunScopedEvent(t *testing.T) {
	db, cs := setupDB(t)
	convID := seedRun(t, db, cs, "run-1")

	err := AppendDeliveryFailedEvent(
		context.Background(),
		db,
		cs,
		"intent-1",
		"telegram",
		"chat-1",
		"run_completed",
		errors.New("boom"),
	)
	if err != nil {
		t.Fatalf("AppendDeliveryFailedEvent: %v", err)
	}

	var eventConversationID string
	var eventRunID string
	var payload string
	if err := db.RawDB().QueryRow(
		`SELECT conversation_id, run_id, payload_json
		 FROM events
		 WHERE kind = 'delivery_failed'
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
	).Scan(&eventConversationID, &eventRunID, &payload); err != nil {
		t.Fatalf("query delivery_failed event: %v", err)
	}
	if eventConversationID != convID || eventRunID != "run-1" {
		t.Fatalf("expected run-scoped event conversation=%q run=%q, got conversation=%q run=%q", convID, "run-1", eventConversationID, eventRunID)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	for _, field := range []string{"intent_id", "chat_id", "event_kind", "connector_id", "error"} {
		if decoded[field] == nil {
			t.Fatalf("expected payload field %q, got %+v", field, decoded)
		}
	}
}

func TestAppendDeliveryFailedEvent_SkipsMissingRunContext(t *testing.T) {
	db, cs := setupDB(t)

	_, err := db.RawDB().Exec(
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at)
		 VALUES ('intent-orphan', '', 'telegram', 'chat-1', 'hello', 'dedupe-orphan', 'terminal', 1, datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("seed outbound intent: %v", err)
	}

	err = AppendDeliveryFailedEvent(
		context.Background(),
		db,
		cs,
		"intent-orphan",
		"telegram",
		"chat-1",
		"run_completed",
		errors.New("boom"),
	)
	if err != nil {
		t.Fatalf("AppendDeliveryFailedEvent: %v", err)
	}

	var count int
	if err := db.RawDB().QueryRow(
		"SELECT count(*) FROM events WHERE kind = 'delivery_failed'",
	).Scan(&count); err != nil {
		t.Fatalf("count delivery_failed events: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no delivery_failed events without run context, got %d", count)
	}
}
