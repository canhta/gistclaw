package email

import (
	"context"
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

func TestOutboundDispatcher_IDReturnsEmail(t *testing.T) {
	db, cs := setupDB(t)
	d := NewOutboundDispatcher(SMTPConfig{Addr: "localhost:25", From: "bot@example.com"}, db, cs)
	if d.ID() != "email" {
		t.Errorf("ID: got %q, want %q", d.ID(), "email")
	}
}

func TestOutboundDispatcher_StartIsNoOp(t *testing.T) {
	db, cs := setupDB(t)
	d := NewOutboundDispatcher(SMTPConfig{Addr: "localhost:25", From: "bot@example.com"}, db, cs)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Start with already-cancelled context should return quickly.
	_ = d.Start(ctx)
}

func TestOutboundDispatcher_NotifySendsEmail(t *testing.T) {
	var capturedTo []string
	var capturedSubjects []string
	var capturedBodies []string

	db, cs := setupDB(t)
	d := NewOutboundDispatcher(SMTPConfig{Addr: "localhost:25", From: "bot@example.com"}, db, cs)
	d.sender = func(addr, from string, to []string, msg []byte) error {
		capturedTo = append(capturedTo, to...)
		// Parse subject and body from raw message bytes.
		raw := string(msg)
		for _, line := range splitLines(raw) {
			if len(line) > 9 && line[:9] == "Subject: " {
				capturedSubjects = append(capturedSubjects, line[9:])
			}
		}
		capturedBodies = append(capturedBodies, raw)
		return nil
	}

	delta := model.ReplayDelta{
		RunID:      "run-1",
		Kind:       "run_completed",
		OccurredAt: time.Now(),
	}
	if err := d.Notify(context.Background(), "user@example.com", delta, "dedupe-1"); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	if len(capturedTo) != 1 || capturedTo[0] != "user@example.com" {
		t.Errorf("to: got %v, want [user@example.com]", capturedTo)
	}
	if len(capturedSubjects) == 0 || capturedSubjects[0] == "" {
		t.Error("subject: missing or empty")
	}
	if len(capturedBodies) == 0 {
		t.Error("body: missing")
	}
}

func TestOutboundDispatcher_NotifyDeduplicates(t *testing.T) {
	callCount := 0

	db, cs := setupDB(t)
	d := NewOutboundDispatcher(SMTPConfig{Addr: "localhost:25", From: "bot@example.com"}, db, cs)
	d.sender = func(addr, from string, to []string, msg []byte) error {
		callCount++
		return nil
	}

	delta := model.ReplayDelta{RunID: "run-2", Kind: "run_completed", OccurredAt: time.Now()}
	_ = d.Notify(context.Background(), "user@example.com", delta, "dedupe-same")
	_ = d.Notify(context.Background(), "user@example.com", delta, "dedupe-same")

	if callCount != 1 {
		t.Errorf("expected 1 send (deduplication), got %d", callCount)
	}
}

func TestOutboundDispatcher_IgnoredEventKindIsNoOp(t *testing.T) {
	db, cs := setupDB(t)
	d := NewOutboundDispatcher(SMTPConfig{Addr: "localhost:25", From: "bot@example.com"}, db, cs)
	d.sender = func(addr, from string, to []string, msg []byte) error {
		t.Error("sender should not be called for ignored event kind")
		return nil
	}

	delta := model.ReplayDelta{RunID: "run-3", Kind: "memory_context_loaded", OccurredAt: time.Now()}
	if err := d.Notify(context.Background(), "user@example.com", delta, "dedupe-ignored"); err != nil {
		t.Fatalf("Notify: unexpected error: %v", err)
	}
}

func TestOutboundDispatcher_DrainDeliversPending(t *testing.T) {
	callCount := 0

	db, cs := setupDB(t)
	d := NewOutboundDispatcher(SMTPConfig{Addr: "localhost:25", From: "bot@example.com"}, db, cs)
	d.sender = func(addr, from string, to []string, msg []byte) error {
		callCount++
		return nil
	}

	// Insert a pending intent directly.
	_, err := db.RawDB().Exec(
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at)
		 VALUES ('intent-drain', 'run-drain', 'email', 'user@example.com', 'hello', 'dk-drain', 'pending', 0, datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert intent: %v", err)
	}

	if err := d.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if callCount == 0 {
		t.Error("expected at least one send during Drain")
	}
}

// splitLines splits a string by CRLF or LF.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			end := i
			if end > 0 && s[end-1] == '\r' {
				end--
			}
			lines = append(lines, s[start:end])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
