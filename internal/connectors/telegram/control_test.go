package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
)

type stubTelegramRuntime struct {
	mu           sync.Mutex
	calls        []runtime.InboundMessageCommand
	gateCalls    []runtime.ConversationGateReplyCommand
	gateOutcome  runtime.ConversationGateReplyOutcome
	gateErr      error
	status       runtime.ConversationStatus
	statusErr    error
	inspectedKey conversations.ConversationKey
	reset        runtime.ConversationResetOutcome
	resetErr     error
	resetKey     conversations.ConversationKey
}

func (s *stubTelegramRuntime) ReceiveInboundMessage(_ context.Context, req runtime.InboundMessageCommand) (model.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, req)
	return model.Run{ID: "run-inbound", SessionID: "session-inbound"}, nil
}

func (s *stubTelegramRuntime) HandleConversationGateReply(_ context.Context, req runtime.ConversationGateReplyCommand) (runtime.ConversationGateReplyOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gateCalls = append(s.gateCalls, req)
	return s.gateOutcome, s.gateErr
}

func (s *stubTelegramRuntime) InspectConversation(_ context.Context, key conversations.ConversationKey) (runtime.ConversationStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inspectedKey = key
	return s.status, s.statusErr
}

func (s *stubTelegramRuntime) ResetConversation(_ context.Context, key conversations.ConversationKey) (runtime.ConversationResetOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetKey = key
	return s.reset, s.resetErr
}

func (s *stubTelegramRuntime) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *stubTelegramRuntime) gateCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.gateCalls)
}

type stubTelegramSender struct {
	mu       sync.Mutex
	messages []sentTelegramMessage
}

type sentTelegramMessage struct {
	chatID string
	text   string
}

func (s *stubTelegramSender) Send(_ context.Context, chatID, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, sentTelegramMessage{chatID: chatID, text: text})
	return nil
}

func (s *stubTelegramSender) sentCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

func (s *stubTelegramSender) first() sentTelegramMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messages[0]
}

func newTelegramControlConnector(t *testing.T, rt *stubTelegramRuntime) *Connector {
	t.Helper()

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cs := conversations.NewConversationStore(db)
	return NewConnector("test-token", db, cs, rt, "assistant")
}

func TestConnector_HandleEnvelopeRoutesHelpToNativeReply(t *testing.T) {
	rt := &stubTelegramRuntime{}
	connector := newTelegramControlConnector(t, rt)
	sender := &stubTelegramSender{}
	connector.sender = sender

	err := connector.handleEnvelope(context.Background(), model.Envelope{
		ConnectorID:    "telegram",
		AccountID:      "123",
		ConversationID: "123",
		ThreadID:       "main",
		Text:           "/help",
	})
	if err != nil {
		t.Fatalf("handleEnvelope: %v", err)
	}
	if rt.callCount() != 0 {
		t.Fatalf("expected no inbound run for /help, got %d calls", rt.callCount())
	}
	if sender.sentCount() != 1 {
		t.Fatalf("expected 1 native reply, got %d", sender.sentCount())
	}
	if got := sender.first(); got.chatID != "123" || !strings.Contains(got.text, "Message me naturally") {
		t.Fatalf("unexpected native reply: %+v", got)
	}
}

func TestConnector_HandleEnvelopeRoutesStatusToNativeReply(t *testing.T) {
	rt := &stubTelegramRuntime{
		status: runtime.ConversationStatus{
			Exists: true,
			ActiveRun: model.Run{
				ID:        "run-active-1234",
				Objective: "review the repo",
				Status:    model.RunStatusActive,
			},
			LatestRootRun: model.Run{
				ID:        "run-active-1234",
				Objective: "review the repo",
				Status:    model.RunStatusActive,
			},
			PendingApprovals: 1,
		},
	}
	connector := newTelegramControlConnector(t, rt)
	sender := &stubTelegramSender{}
	connector.sender = sender

	err := connector.handleEnvelope(context.Background(), model.Envelope{
		ConnectorID:    "telegram",
		AccountID:      "123",
		ConversationID: "123",
		ThreadID:       "main",
		Text:           "/status",
	})
	if err != nil {
		t.Fatalf("handleEnvelope: %v", err)
	}
	if rt.callCount() != 0 {
		t.Fatalf("expected no inbound run for /status, got %d calls", rt.callCount())
	}
	if sender.sentCount() != 1 {
		t.Fatalf("expected 1 native reply, got %d", sender.sentCount())
	}
	reply := sender.first().text
	for _, want := range []string{"Active run", "run-acti", "review the repo", "1 pending approval"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("expected status reply to include %q, got:\n%s", want, reply)
		}
	}
	if strings.Contains(strings.ToLower(reply), "web ui") {
		t.Fatalf("expected Telegram status reply to stay native to chat, got:\n%s", reply)
	}
}

func TestConnector_HandleEnvelopeRoutesLocalizedHelpToNativeReply(t *testing.T) {
	rt := &stubTelegramRuntime{}
	connector := newTelegramControlConnector(t, rt)
	sender := &stubTelegramSender{}
	connector.sender = sender

	err := connector.handleEnvelope(context.Background(), model.Envelope{
		ConnectorID:    "telegram",
		AccountID:      "123",
		ConversationID: "123",
		ThreadID:       "main",
		Text:           "/help",
		Metadata: map[string]string{
			"language_hint": "vi",
		},
	})
	if err != nil {
		t.Fatalf("handleEnvelope: %v", err)
	}
	if sender.sentCount() != 1 {
		t.Fatalf("expected 1 native reply, got %d", sender.sentCount())
	}
	if got := sender.first(); got.chatID != "123" || !strings.Contains(got.text, "Nhắn cho mình tự nhiên") {
		t.Fatalf("unexpected localized native reply: %+v", got)
	}
}

func TestConnector_HandleEnvelopeRoutesResetToNativeReply(t *testing.T) {
	rt := &stubTelegramRuntime{reset: runtime.ConversationResetCleared}
	connector := newTelegramControlConnector(t, rt)
	sender := &stubTelegramSender{}
	connector.sender = sender

	err := connector.handleEnvelope(context.Background(), model.Envelope{
		ConnectorID:    "telegram",
		AccountID:      "123",
		ConversationID: "123",
		ThreadID:       "main",
		Text:           "/reset",
	})
	if err != nil {
		t.Fatalf("handleEnvelope: %v", err)
	}
	if rt.callCount() != 0 {
		t.Fatalf("expected no inbound run for /reset, got %d calls", rt.callCount())
	}
	if sender.sentCount() != 1 {
		t.Fatalf("expected 1 native reply, got %d", sender.sentCount())
	}
	reply := sender.first().text
	for _, want := range []string{"Chat reset", "History cleared"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("expected reset reply to include %q, got:\n%s", want, reply)
		}
	}
	if rt.resetKey != (conversations.ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "123",
		ExternalID:  "123",
		ThreadID:    "main",
	}) {
		t.Fatalf("unexpected reset conversation key: %+v", rt.resetKey)
	}
}

func TestConnector_HandleEnvelopePassesUnknownSlashTextToRuntime(t *testing.T) {
	rt := &stubTelegramRuntime{}
	connector := newTelegramControlConnector(t, rt)
	sender := &stubTelegramSender{}
	connector.sender = sender

	err := connector.handleEnvelope(context.Background(), model.Envelope{
		ConnectorID:    "telegram",
		AccountID:      "123",
		ConversationID: "123",
		ThreadID:       "main",
		MessageID:      "42",
		Text:           "/review the repo",
	})
	if err != nil {
		t.Fatalf("handleEnvelope: %v", err)
	}
	if rt.callCount() != 1 {
		t.Fatalf("expected inbound run for unknown slash text, got %d calls", rt.callCount())
	}
	if sender.sentCount() != 0 {
		t.Fatalf("expected no native reply for unknown slash text, got %d", sender.sentCount())
	}
	if got := rt.calls[0].Body; got != "/review the repo" {
		t.Fatalf("expected body %q, got %q", "/review the repo", got)
	}
}

func TestConnector_HandleEnvelopeConsumesActiveConversationGateReply(t *testing.T) {
	rt := &stubTelegramRuntime{
		gateOutcome: runtime.ConversationGateReplyOutcome{
			Handled:  true,
			GateID:   "gate-1",
			Decision: "approved",
		},
	}
	connector := newTelegramControlConnector(t, rt)
	sender := &stubTelegramSender{}
	connector.sender = sender

	err := connector.handleEnvelope(context.Background(), model.Envelope{
		ConnectorID:    "telegram",
		AccountID:      "123",
		ConversationID: "123",
		ThreadID:       "main",
		MessageID:      "43",
		Text:           "/approve approval-1 allow-once",
	})
	if err != nil {
		t.Fatalf("handleEnvelope: %v", err)
	}
	if rt.gateCallCount() != 1 {
		t.Fatalf("expected gate handler to be consulted once, got %d calls", rt.gateCallCount())
	}
	if rt.callCount() != 0 {
		t.Fatalf("expected no inbound run when gate reply is consumed, got %d calls", rt.callCount())
	}
	if sender.sentCount() != 0 {
		t.Fatalf("expected no immediate native reply when runtime consumes gate reply, got %d sends", sender.sentCount())
	}
	if got := rt.gateCalls[0].Body; got != "/approve approval-1 allow-once" {
		t.Fatalf("expected gate body %q, got %q", "/approve approval-1 allow-once", got)
	}
}

func TestConnector_HandleEnvelopeFallsBackToInboundWhenNoConversationGateConsumes(t *testing.T) {
	rt := &stubTelegramRuntime{
		gateOutcome: runtime.ConversationGateReplyOutcome{Handled: false},
	}
	connector := newTelegramControlConnector(t, rt)

	err := connector.handleEnvelope(context.Background(), model.Envelope{
		ConnectorID:    "telegram",
		AccountID:      "123",
		ConversationID: "123",
		ThreadID:       "main",
		MessageID:      "44",
		Text:           "review the repo",
	})
	if err != nil {
		t.Fatalf("handleEnvelope: %v", err)
	}
	if rt.gateCallCount() != 1 {
		t.Fatalf("expected gate handler to be consulted once, got %d calls", rt.gateCallCount())
	}
	if rt.callCount() != 1 {
		t.Fatalf("expected inbound run when no gate consumes the message, got %d calls", rt.callCount())
	}
}

func TestConnector_HandleUpdateAnswersCallbackQuery(t *testing.T) {
	rt := &stubTelegramRuntime{
		gateOutcome: runtime.ConversationGateReplyOutcome{Handled: true},
	}
	connector := newTelegramControlConnector(t, rt)

	var answered atomic.Int32
	var callbackQueryID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "answerCallbackQuery") {
			answered.Add(1)
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			callbackQueryID = r.Form.Get("callback_query_id")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()

	connector.outbound.bot.apiBase = srv.URL + "/bot"

	connector.handleUpdate(context.Background(), Update{
		UpdateID: 204,
		CallbackQuery: &CallbackQuery{
			ID:   "cbq-2",
			From: User{ID: 123},
			Data: "/approve ticket-1 allow-once",
			Message: &Message{
				MessageID: 89,
				Chat: Chat{
					ID:   123,
					Type: "private",
				},
			},
		},
	})

	if answered.Load() != 1 {
		t.Fatalf("expected answerCallbackQuery to be called once, got %d", answered.Load())
	}
	if callbackQueryID != "cbq-2" {
		t.Fatalf("expected callback_query_id %q, got %q", "cbq-2", callbackQueryID)
	}
}
