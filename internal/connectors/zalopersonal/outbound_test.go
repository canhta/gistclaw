package zalopersonal

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

type stubTextSender struct {
	sends    []string
	failures int
}

func (s *stubTextSender) SendText(_ context.Context, chatID, text string) error {
	if s.failures > 0 {
		s.failures--
		return fmt.Errorf("send failed")
	}
	s.sends = append(s.sends, chatID+"|"+text)
	return nil
}

func setupZaloOutboundDB(t *testing.T) *store.DB {
	t.Helper()

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedZaloOutboundRun(t *testing.T, db *store.DB, cs *conversations.ConversationStore, runID string) {
	t.Helper()

	conv, err := cs.Resolve(context.Background(), conversations.ConversationKey{
		ConnectorID: "zalo_personal",
		AccountID:   "acct-1",
		ExternalID:  "chat-1",
		ThreadID:    "main",
	})
	if err != nil {
		t.Fatalf("resolve conversation: %v", err)
	}
	if _, err := db.RawDB().Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
		 VALUES (?, ?, ?, 'completed', datetime('now'), datetime('now'))`,
		runID, conv.ID, "assistant",
	); err != nil {
		t.Fatalf("seed run: %v", err)
	}
}

func TestOutboundDispatcher(t *testing.T) {
	t.Parallel()

	t.Run("notify ignores unsupported event kinds", func(t *testing.T) {
		t.Parallel()

		db := setupZaloOutboundDB(t)
		cs := conversations.NewConversationStore(db)
		sender := &stubTextSender{}
		dispatcher := NewOutboundDispatcher(sender, db, cs)

		if err := dispatcher.Notify(context.Background(), "chat-1", model.ReplayDelta{
			RunID: "run-ignored",
			Kind:  "turn_started",
		}, "dedupe-ignored"); err != nil {
			t.Fatalf("Notify: %v", err)
		}
		if len(sender.sends) != 0 {
			t.Fatalf("expected no sends, got %+v", sender.sends)
		}
	})

	t.Run("notify enqueues and delivers allowed event kinds", func(t *testing.T) {
		t.Parallel()

		db := setupZaloOutboundDB(t)
		cs := conversations.NewConversationStore(db)
		sender := &stubTextSender{}
		dispatcher := NewOutboundDispatcher(sender, db, cs)

		if err := dispatcher.Notify(context.Background(), "chat-1", model.ReplayDelta{
			RunID: "run-start",
			Kind:  "run_started",
		}, "dedupe-start"); err != nil {
			t.Fatalf("Notify: %v", err)
		}
		if len(sender.sends) != 1 {
			t.Fatalf("expected 1 send, got %+v", sender.sends)
		}

		var status string
		err := db.RawDB().QueryRowContext(context.Background(),
			`SELECT status FROM outbound_intents WHERE connector_id = 'zalo_personal' ORDER BY created_at DESC, id DESC LIMIT 1`,
		).Scan(&status)
		if err != nil {
			t.Fatalf("query outbound intent: %v", err)
		}
		if status != "delivered" {
			t.Fatalf("expected delivered status, got %q", status)
		}
	})

	t.Run("notify dedupes by dedupe key", func(t *testing.T) {
		t.Parallel()

		db := setupZaloOutboundDB(t)
		cs := conversations.NewConversationStore(db)
		sender := &stubTextSender{}
		dispatcher := NewOutboundDispatcher(sender, db, cs)

		delta := model.ReplayDelta{RunID: "run-dedupe", Kind: "run_completed"}
		if err := dispatcher.Notify(context.Background(), "chat-1", delta, "same-key"); err != nil {
			t.Fatalf("first Notify: %v", err)
		}
		if err := dispatcher.Notify(context.Background(), "chat-1", delta, "same-key"); err != nil {
			t.Fatalf("second Notify: %v", err)
		}
		if len(sender.sends) != 1 {
			t.Fatalf("expected deduped sends, got %+v", sender.sends)
		}
	})

	t.Run("drain retries pending intents", func(t *testing.T) {
		t.Parallel()

		db := setupZaloOutboundDB(t)
		cs := conversations.NewConversationStore(db)
		sender := &stubTextSender{}
		dispatcher := NewOutboundDispatcher(sender, db, cs)

		if _, err := db.RawDB().ExecContext(context.Background(),
			`INSERT INTO outbound_intents
			 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at)
			 VALUES ('intent-1', NULL, 'zalo_personal', 'chat-1', 'hello', 'key-1', 'pending', 0, datetime('now'))`,
		); err != nil {
			t.Fatalf("insert outbound intent: %v", err)
		}

		if err := dispatcher.Drain(context.Background()); err != nil {
			t.Fatalf("Drain: %v", err)
		}
		if len(sender.sends) != 1 {
			t.Fatalf("expected 1 drained send, got %+v", sender.sends)
		}
	})

	t.Run("terminal failure appends delivery failed event when run context exists", func(t *testing.T) {
		t.Parallel()

		db := setupZaloOutboundDB(t)
		cs := conversations.NewConversationStore(db)
		seedZaloOutboundRun(t, db, cs, "run-failed")

		sender := &stubTextSender{failures: 2}
		dispatcher := NewOutboundDispatcher(sender, db, cs)
		dispatcher.maxAttempts = 2
		dispatcher.retryDelay = 0

		err := dispatcher.Notify(context.Background(), "chat-1", model.ReplayDelta{
			RunID: "run-failed",
			Kind:  "run_completed",
		}, "dedupe-failed")
		if err == nil {
			t.Fatal("expected Notify to fail after retries")
		}

		var status string
		var attempts int
		if err := db.RawDB().QueryRowContext(context.Background(),
			`SELECT status, attempts FROM outbound_intents WHERE dedupe_key = ?`,
			"dedupe-failed",
		).Scan(&status, &attempts); err != nil {
			t.Fatalf("query failed outbound intent: %v", err)
		}
		if status != "terminal" {
			t.Fatalf("expected terminal status, got %q", status)
		}
		if attempts != 2 {
			t.Fatalf("expected 2 attempts, got %d", attempts)
		}

		var eventKind string
		if err := db.RawDB().QueryRowContext(context.Background(),
			`SELECT kind FROM events WHERE run_id = ? ORDER BY created_at DESC LIMIT 1`,
			"run-failed",
		).Scan(&eventKind); err != nil {
			t.Fatalf("query delivery failure event: %v", err)
		}
		if eventKind != "delivery_failed" {
			t.Fatalf("expected delivery_failed event, got %q", eventKind)
		}
	})
}

func TestOutboundDispatcherSendPersistsConnectorCommand(t *testing.T) {
	t.Parallel()

	db := setupZaloOutboundDB(t)
	cs := conversations.NewConversationStore(db)
	sender := &stubTextSender{}
	dispatcher := NewOutboundDispatcher(sender, db, cs)

	if err := dispatcher.Send(context.Background(), "chat-1", "native help reply"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(sender.sends) != 1 {
		t.Fatalf("expected 1 send, got %+v", sender.sends)
	}

	var status string
	var runID sql.NullString
	var attemptedAt sql.NullTime
	err := db.RawDB().QueryRowContext(context.Background(),
		`SELECT status, run_id, last_attempt_at
		 FROM outbound_intents
		 WHERE connector_id = 'zalo_personal'
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
	).Scan(&status, &runID, &attemptedAt)
	if err != nil {
		t.Fatalf("query connector command intent: %v", err)
	}
	if status != "delivered" {
		t.Fatalf("expected delivered status, got %q", status)
	}
	if runID.Valid {
		t.Fatalf("expected NULL run_id, got %q", runID.String)
	}
	if !attemptedAt.Valid || attemptedAt.Time.IsZero() {
		t.Fatal("expected last_attempt_at to be recorded")
	}
}

var _ = time.Now
