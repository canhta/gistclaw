package whatsapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

type stubInboundReceiver struct {
	mu     sync.Mutex
	calls  []runtime.InboundMessageCommand
	status runtime.ConversationStatus
	reset  runtime.ConversationResetOutcome
	key    conversations.ConversationKey
}

func (s *stubInboundReceiver) ReceiveInboundMessage(_ context.Context, req runtime.InboundMessageCommand) (model.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, req)
	return model.Run{ID: "run-whatsapp", SessionID: "session-whatsapp"}, nil
}

func (s *stubInboundReceiver) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *stubInboundReceiver) InspectConversation(context.Context, conversations.ConversationKey) (runtime.ConversationStatus, error) {
	return s.status, nil
}

func (s *stubInboundReceiver) ResetConversation(_ context.Context, key conversations.ConversationKey) (runtime.ConversationResetOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.key = key
	return s.reset, nil
}

type stubWhatsAppSender struct {
	mu       sync.Mutex
	messages []string
}

func (s *stubWhatsAppSender) Send(_ context.Context, _ string, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, text)
	return nil
}

func (s *stubWhatsAppSender) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

func (s *stubWhatsAppSender) first() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messages[0]
}

func TestNormalizeWebhookPayload_TextMessageToEnvelope(t *testing.T) {
	payload := []byte(`{
	  "object":"whatsapp_business_account",
	  "entry":[{
	    "changes":[{
	      "field":"messages",
	      "value":{
	        "metadata":{"phone_number_id":"phone-123"},
	        "messages":[{
	          "from":"15551234567",
	          "id":"wamid.42",
	          "timestamp":"1711296000",
	          "type":"text",
	          "text":{"body":"review the auth module"}
	        }]
	      }
	    }]
	  }]
	}`)

	envelopes, err := NormalizeWebhookPayload(payload)
	if err != nil {
		t.Fatalf("NormalizeWebhookPayload: %v", err)
	}
	if len(envelopes) != 1 {
		t.Fatalf("expected 1 envelope, got %d", len(envelopes))
	}
	env := envelopes[0]
	if env.ConnectorID != "whatsapp" {
		t.Fatalf("expected connector whatsapp, got %q", env.ConnectorID)
	}
	if env.AccountID != "phone-123" || env.ConversationID != "15551234567" {
		t.Fatalf("unexpected conversation identity: account=%q conversation=%q", env.AccountID, env.ConversationID)
	}
	if env.ThreadID != "main" {
		t.Fatalf("expected thread main, got %q", env.ThreadID)
	}
	if env.Text != "review the auth module" {
		t.Fatalf("expected text body, got %q", env.Text)
	}
}

func TestWebhookHandler_VerifyChallenge(t *testing.T) {
	handler := NewWebhookHandler("verify-token", "assistant", t.TempDir(), &stubInboundReceiver{}, nil)

	req := httptest.NewRequest(http.MethodGet,
		"/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=verify-token&hub.challenge=12345", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if strings.TrimSpace(rr.Body.String()) != "12345" {
		t.Fatalf("expected challenge body, got %q", rr.Body.String())
	}
}

func TestWebhookHandler_RejectsWrongVerifyToken(t *testing.T) {
	handler := NewWebhookHandler("verify-token", "assistant", t.TempDir(), &stubInboundReceiver{}, nil)

	req := httptest.NewRequest(http.MethodGet,
		"/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=wrong&hub.challenge=12345", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestWebhookHandler_DispatchesInboundTextMessages(t *testing.T) {
	receiver := &stubInboundReceiver{}
	handler := NewWebhookHandler("verify-token", "assistant", t.TempDir(), receiver, nil)

	body := `{
	  "object":"whatsapp_business_account",
	  "entry":[{
	    "changes":[{
	      "field":"messages",
	      "value":{
	        "metadata":{"phone_number_id":"phone-123"},
	        "messages":[{
	          "from":"15551234567",
	          "id":"wamid.42",
	          "timestamp":"1711296000",
	          "type":"text",
	          "text":{"body":"review the auth module"}
	        }]
	      }
	    }]
	  }]
	}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if receiver.callCount() != 1 {
		t.Fatalf("expected 1 inbound runtime call, got %d", receiver.callCount())
	}
	call := receiver.calls[0]
	if call.Body != "review the auth module" {
		t.Fatalf("expected body to pass through, got %q", call.Body)
	}
	if call.ConversationKey != (conversations.ConversationKey{
		ConnectorID: "whatsapp",
		AccountID:   "phone-123",
		ExternalID:  "15551234567",
		ThreadID:    "main",
	}) {
		t.Fatalf("unexpected conversation key: %+v", call.ConversationKey)
	}
	if call.SourceMessageID != "wamid.42" {
		t.Fatalf("expected source message ID to pass through, got %q", call.SourceMessageID)
	}
}

func TestWebhookHandler_RoutesHelpCommandToNativeReply(t *testing.T) {
	receiver := &stubInboundReceiver{}
	sender := &stubWhatsAppSender{}
	handler := NewWebhookHandler("verify-token", "assistant", t.TempDir(), receiver, sender)

	body := `{
	  "object":"whatsapp_business_account",
	  "entry":[{
	    "changes":[{
	      "field":"messages",
	      "value":{
	        "metadata":{"phone_number_id":"phone-123"},
	        "messages":[{
	          "from":"15551234567",
	          "id":"wamid.help",
	          "timestamp":"1711296000",
	          "type":"text",
	          "text":{"body":"/help"}
	        }]
	      }
	    }]
	  }]
	}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if receiver.callCount() != 0 {
		t.Fatalf("expected no inbound runtime calls for /help, got %d", receiver.callCount())
	}
	if sender.callCount() != 1 {
		t.Fatalf("expected 1 native reply, got %d", sender.callCount())
	}
	if !strings.Contains(sender.first(), "Message me naturally") {
		t.Fatalf("unexpected native reply: %q", sender.first())
	}
}

func TestWebhookHandler_RoutesResetCommandToNativeReply(t *testing.T) {
	receiver := &stubInboundReceiver{reset: runtime.ConversationResetCleared}
	sender := &stubWhatsAppSender{}
	handler := NewWebhookHandler("verify-token", "assistant", t.TempDir(), receiver, sender)

	body := `{
	  "object":"whatsapp_business_account",
	  "entry":[{
	    "changes":[{
	      "field":"messages",
	      "value":{
	        "metadata":{"phone_number_id":"phone-123"},
	        "messages":[{
	          "from":"15551234567",
	          "id":"wamid.reset",
	          "timestamp":"1711296000",
	          "type":"text",
	          "text":{"body":"/reset"}
	        }]
	      }
	    }]
	  }]
	}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if receiver.callCount() != 0 {
		t.Fatalf("expected no inbound runtime calls for /reset, got %d", receiver.callCount())
	}
	if sender.callCount() != 1 {
		t.Fatalf("expected 1 native reply, got %d", sender.callCount())
	}
	reply := sender.first()
	for _, want := range []string{"Chat reset", "History cleared"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("expected reset reply to include %q, got %q", want, reply)
		}
	}
	if receiver.key != (conversations.ConversationKey{
		ConnectorID: "whatsapp",
		AccountID:   "phone-123",
		ExternalID:  "15551234567",
		ThreadID:    "main",
	}) {
		t.Fatalf("unexpected reset conversation key: %+v", receiver.key)
	}
}
