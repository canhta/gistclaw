package whatsapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

func newWhatsAppConnectorTestDB(t *testing.T) (*store.DB, *conversations.ConversationStore) {
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

func TestConnector_StartDrainsOutboundWhileRunning(t *testing.T) {
	db, cs := newWhatsAppConnectorTestDB(t)

	var sendCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/messages") {
			sendCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.test"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	if _, err := db.RawDB().Exec(
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at)
		 VALUES ('intent-1', 'run-1', 'whatsapp', '+15551234567', 'queued reply', 'dedupe-1', 'pending', 0, datetime('now'))`,
	); err != nil {
		t.Fatalf("seed outbound intent: %v", err)
	}

	connector := NewConnector("phone-123", "access-token", db, cs, nil)
	connector.outbound.apiBase = srv.URL
	connector.drainInterval = 10 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- connector.Start(ctx)
	}()

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if sendCalls.Load() > 0 {
			cancel()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if sendCalls.Load() == 0 {
		t.Fatal("expected pending outbound intent to be drained and delivered")
	}
	if snapshot := connector.ConnectorHealthSnapshot(); snapshot.State != model.ConnectorHealthHealthy {
		t.Fatalf("expected healthy connector snapshot, got %#v", snapshot)
	}
	if err := <-errCh; err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		t.Fatalf("Start returned unexpected error: %v", err)
	}
}
