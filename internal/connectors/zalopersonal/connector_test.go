package zalopersonal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

type stubInboundRuntime struct {
	mu    sync.Mutex
	calls []runtime.InboundMessageCommand
}

func (s *stubInboundRuntime) ReceiveInboundMessage(_ context.Context, req runtime.InboundMessageCommand) (model.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, req)
	return model.Run{ID: "run-zalo", SessionID: "session-zalo"}, nil
}

func (s *stubInboundRuntime) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

type stubSessionListener struct {
	messages     chan IncomingMessage
	errors       chan error
	disconnected chan error
	startErr     error
	startCalls   int32
	stopCalls    int32
	onStart      func()
}

func newStubSessionListener() *stubSessionListener {
	return &stubSessionListener{
		messages:     make(chan IncomingMessage, 4),
		errors:       make(chan error, 4),
		disconnected: make(chan error, 4),
	}
}

func (s *stubSessionListener) Start(context.Context) error {
	atomic.AddInt32(&s.startCalls, 1)
	if s.onStart != nil {
		s.onStart()
	}
	return s.startErr
}

func (s *stubSessionListener) Stop() {
	atomic.AddInt32(&s.stopCalls, 1)
}

func (s *stubSessionListener) Messages() <-chan IncomingMessage { return s.messages }
func (s *stubSessionListener) Errors() <-chan error             { return s.errors }
func (s *stubSessionListener) Disconnected() <-chan error       { return s.disconnected }

func TestConnectorStart(t *testing.T) {
	t.Run("missing credentials marks connector degraded without crashing", func(t *testing.T) {
		db := setupZaloOutboundDB(t)
		cs := conversations.NewConversationStore(db)
		rt := &stubInboundRuntime{}

		connector := NewConnector(db, cs, rt, "assistant")
		connector.credentialPollInterval = 10 * time.Millisecond
		connector.reconnectDelay = 10 * time.Millisecond

		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
		defer cancel()

		err := connector.Start(ctx)
		if err != context.DeadlineExceeded && err != context.Canceled {
			t.Fatalf("expected context shutdown, got %v", err)
		}
		snapshot := connector.ConnectorHealthSnapshot()
		if snapshot.State != model.ConnectorHealthDegraded {
			t.Fatalf("expected degraded state, got %#v", snapshot)
		}
		if snapshot.Summary != "not authenticated" {
			t.Fatalf("expected not authenticated summary, got %#v", snapshot)
		}
	})

	t.Run("missing credentials does not drain pending outbound intents", func(t *testing.T) {
		db := setupZaloOutboundDB(t)
		cs := conversations.NewConversationStore(db)
		rt := &stubInboundRuntime{}

		if _, err := db.RawDB().ExecContext(context.Background(),
			`INSERT INTO outbound_intents
			 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at)
			 VALUES ('intent-pending', NULL, 'zalo_personal', 'user-1', 'hello', NULL, 'pending', 0, datetime('now'))`,
		); err != nil {
			t.Fatalf("insert outbound intent: %v", err)
		}

		connector := NewConnector(db, cs, rt, "assistant")
		connector.credentialPollInterval = 10 * time.Millisecond
		connector.reconnectDelay = 10 * time.Millisecond
		connector.outbound.maxAttempts = 1
		connector.outbound.retryDelay = 1 * time.Millisecond

		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
		defer cancel()

		err := connector.Start(ctx)
		if err != context.DeadlineExceeded && err != context.Canceled {
			t.Fatalf("expected context shutdown, got %v", err)
		}

		var status string
		var attempts int
		if err := db.RawDB().QueryRowContext(context.Background(),
			`SELECT status, attempts FROM outbound_intents WHERE id = 'intent-pending'`,
		).Scan(&status, &attempts); err != nil {
			t.Fatalf("load outbound intent: %v", err)
		}
		if status != "pending" || attempts != 0 {
			t.Fatalf("expected pending outbound intent to remain untouched, got status=%q attempts=%d", status, attempts)
		}
	})

	t.Run("saved credentials login and inbound message dispatch", func(t *testing.T) {
		db := setupZaloOutboundDB(t)
		cs := conversations.NewConversationStore(db)
		rt := &stubInboundRuntime{}
		if err := SaveStoredCredentials(context.Background(), db, StoredCredentials{
			AccountID:   "acct-1",
			DisplayName: "Canh",
			IMEI:        "imei-123",
			Cookie:      "zpw_sek=abc123",
			UserAgent:   "Mozilla/5.0",
			Language:    "vi",
		}); err != nil {
			t.Fatalf("SaveStoredCredentials: %v", err)
		}

		listener := newStubSessionListener()
		listener.onStart = func() {
			listener.messages <- IncomingMessage{
				AccountID:      "acct-1",
				SenderID:       "user-1",
				ConversationID: "user-1",
				MessageID:      "msg-1",
				Text:           "review auth",
				IsDirect:       true,
				LanguageHint:   "vi",
			}
		}

		connector := NewConnector(db, cs, rt, "assistant")
		connector.login = func(ctx context.Context, creds StoredCredentials) (*listenerSession, error) {
			return &listenerSession{AccountID: creds.AccountID, Language: creds.Language}, nil
		}
		connector.newListener = func(*listenerSession) (SessionListener, error) {
			return listener, nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		defer cancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- connector.Start(ctx)
		}()

		deadline := time.Now().Add(50 * time.Millisecond)
		for time.Now().Before(deadline) {
			if rt.callCount() > 0 {
				cancel()
				break
			}
			time.Sleep(5 * time.Millisecond)
		}

		if rt.callCount() != 1 {
			t.Fatalf("expected 1 inbound runtime call, got %d", rt.callCount())
		}
		call := rt.calls[0]
		if call.ConversationKey != (conversations.ConversationKey{
			ConnectorID: "zalo_personal",
			AccountID:   "acct-1",
			ExternalID:  "user-1",
			ThreadID:    "main",
		}) {
			t.Fatalf("unexpected conversation key: %+v", call.ConversationKey)
		}
		if call.SourceMessageID != "msg-1" || call.Body != "review auth" || call.LanguageHint != "vi" {
			t.Fatalf("unexpected inbound call: %+v", call)
		}
		snapshot := connector.ConnectorHealthSnapshot()
		if snapshot.State != model.ConnectorHealthHealthy {
			t.Fatalf("expected healthy connector snapshot, got %#v", snapshot)
		}
		if err := <-errCh; err != context.Canceled && err != context.DeadlineExceeded {
			t.Fatalf("expected context shutdown, got %v", err)
		}
	})

	t.Run("listener disconnect triggers reconnect", func(t *testing.T) {
		db := setupZaloOutboundDB(t)
		cs := conversations.NewConversationStore(db)
		rt := &stubInboundRuntime{}
		if err := SaveStoredCredentials(context.Background(), db, StoredCredentials{
			AccountID: "acct-1",
			IMEI:      "imei-123",
			Cookie:    "zpw_sek=abc123",
			UserAgent: "Mozilla/5.0",
		}); err != nil {
			t.Fatalf("SaveStoredCredentials: %v", err)
		}

		first := newStubSessionListener()
		first.onStart = func() {
			first.disconnected <- fmt.Errorf("socket closed")
		}
		second := newStubSessionListener()

		var loginCalls atomic.Int32
		var listenerCalls atomic.Int32
		connector := NewConnector(db, cs, rt, "assistant")
		connector.reconnectDelay = 10 * time.Millisecond
		connector.login = func(ctx context.Context, creds StoredCredentials) (*listenerSession, error) {
			loginCalls.Add(1)
			return &listenerSession{AccountID: creds.AccountID}, nil
		}
		connector.newListener = func(*listenerSession) (SessionListener, error) {
			n := listenerCalls.Add(1)
			if n == 1 {
				return first, nil
			}
			return second, nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		defer cancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- connector.Start(ctx)
		}()

		deadline := time.Now().Add(60 * time.Millisecond)
		for time.Now().Before(deadline) {
			if loginCalls.Load() >= 2 && listenerCalls.Load() >= 2 {
				cancel()
				break
			}
			time.Sleep(5 * time.Millisecond)
		}

		if loginCalls.Load() < 2 {
			t.Fatalf("expected login to be retried, got %d calls", loginCalls.Load())
		}
		if listenerCalls.Load() < 2 {
			t.Fatalf("expected listener to be recreated, got %d calls", listenerCalls.Load())
		}
		if err := <-errCh; err != context.Canceled && err != context.DeadlineExceeded {
			t.Fatalf("expected context shutdown, got %v", err)
		}
	})
}

func TestConnectorSendText(t *testing.T) {
	t.Run("stored credentials send text through connector sender", func(t *testing.T) {
		db := setupZaloOutboundDB(t)
		cs := conversations.NewConversationStore(db)
		rt := &stubInboundRuntime{}
		if err := SaveStoredCredentials(context.Background(), db, StoredCredentials{
			AccountID: "acct-1",
			IMEI:      "imei-123",
			Cookie:    "zpw_sek=abc123",
			UserAgent: "Mozilla/5.0",
			Language:  "vi",
		}); err != nil {
			t.Fatalf("SaveStoredCredentials: %v", err)
		}

		connector := NewConnector(db, cs, rt, "assistant")
		var gotChatID string
		var gotText string
		connector.sendText = func(_ context.Context, creds StoredCredentials, chatID, text string) error {
			if creds.AccountID != "acct-1" {
				t.Fatalf("unexpected creds: %+v", creds)
			}
			gotChatID = chatID
			gotText = text
			return nil
		}

		if err := connector.SendText(context.Background(), "user-1", "xin chao"); err != nil {
			t.Fatalf("SendText: %v", err)
		}
		if gotChatID != "user-1" || gotText != "xin chao" {
			t.Fatalf("unexpected send args: chatID=%q text=%q", gotChatID, gotText)
		}
	})

	t.Run("missing credentials fails send text", func(t *testing.T) {
		db := setupZaloOutboundDB(t)
		cs := conversations.NewConversationStore(db)
		rt := &stubInboundRuntime{}

		connector := NewConnector(db, cs, rt, "assistant")
		if err := connector.SendText(context.Background(), "user-1", "xin chao"); err == nil {
			t.Fatal("expected SendText to fail without stored credentials")
		}
	})
}
