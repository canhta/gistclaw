package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
)

type stubFrontSessionStarter struct {
	mu    sync.Mutex
	calls []runtime.InboundMessageCommand
}

func (s *stubFrontSessionStarter) ReceiveInboundMessage(_ context.Context, req runtime.InboundMessageCommand) (model.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, req)
	return model.Run{ID: "run-front", SessionID: "session-front"}, nil
}

func (s *stubFrontSessionStarter) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *stubFrontSessionStarter) InspectConversation(context.Context, conversations.ConversationKey) (runtime.ConversationStatus, error) {
	return runtime.ConversationStatus{}, nil
}

func newTelegramConnectorTestDB(t *testing.T) (*store.DB, *conversations.ConversationStore) {
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

func TestConnector_StartDispatchesInboundAndDrainsOutbound(t *testing.T) {
	db, cs := newTelegramConnectorTestDB(t)
	starter := &stubFrontSessionStarter{}

	var getUpdatesCalls atomic.Int32
	var sendMessageCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "getUpdates"):
			n := getUpdatesCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if n == 1 {
				_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":1,"message":{"message_id":42,"chat":{"id":999,"type":"private"},"text":"inspect docs"}}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
		case strings.Contains(r.URL.Path, "sendMessage"):
			sendMessageCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	if _, err := db.RawDB().Exec(
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at)
		 VALUES ('intent-1', 'run-1', 'telegram', '999', 'queued reply', 'dedupe-1', 'pending', 0, datetime('now'))`,
	); err != nil {
		t.Fatalf("seed outbound intent: %v", err)
	}

	connector := NewConnector("testtoken", db, cs, starter, "assistant", "")
	connector.outbound.bot.apiBase = srv.URL + "/bot"
	connector.drainInterval = 10 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- connector.Start(ctx)
	}()

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if starter.callCount() > 0 && sendMessageCalls.Load() > 0 {
			cancel()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if starter.callCount() == 0 {
		t.Fatal("expected inbound update to dispatch an inbound message")
	}
	if sendMessageCalls.Load() == 0 {
		t.Fatal("expected pending outbound intent to be drained and delivered")
	}

	if err := <-errCh; err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		t.Fatalf("Start returned unexpected error: %v", err)
	}
}
