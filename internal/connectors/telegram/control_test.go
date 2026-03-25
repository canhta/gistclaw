package telegram

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
)

type stubTelegramRuntime struct {
	mu           sync.Mutex
	calls        []runtime.InboundMessageCommand
	status       runtime.ConversationStatus
	statusErr    error
	inspectedKey conversations.ConversationKey
}

func (s *stubTelegramRuntime) ReceiveInboundMessage(_ context.Context, req runtime.InboundMessageCommand) (model.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, req)
	return model.Run{ID: "run-inbound", SessionID: "session-inbound"}, nil
}

func (s *stubTelegramRuntime) InspectConversation(_ context.Context, key conversations.ConversationKey) (runtime.ConversationStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inspectedKey = key
	return s.status, s.statusErr
}

func (s *stubTelegramRuntime) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
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
	return NewConnector("test-token", db, cs, rt, "assistant", "")
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
