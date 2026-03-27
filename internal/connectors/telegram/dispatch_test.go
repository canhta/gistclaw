package telegram

import (
	"context"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

// stubIngress records the ReceiveInboundMessage calls made to it.
type stubIngress struct {
	calls []runtime.InboundMessageCommand
}

func (s *stubIngress) ReceiveInboundMessage(_ context.Context, req runtime.InboundMessageCommand) (model.Run, error) {
	s.calls = append(s.calls, req)
	return model.Run{ID: "test-run"}, nil
}

func TestInboundDispatcher_DispatchesEnvelopeToRuntime(t *testing.T) {
	ingress := &stubIngress{}
	dispatcher := NewInboundDispatcher(ingress, "assistant")

	env := model.Envelope{
		ConnectorID:    "telegram",
		AccountID:      "123456",
		ActorID:        "123456",
		ConversationID: "123456",
		ThreadID:       "main",
		MessageID:      "42",
		Text:           "review the auth module",
	}

	if err := dispatcher.Dispatch(context.Background(), env); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if len(ingress.calls) != 1 {
		t.Fatalf("expected 1 ReceiveInboundMessage call, got %d", len(ingress.calls))
	}
	call := ingress.calls[0]
	if call.Body != env.Text {
		t.Errorf("Body: expected %q, got %q", env.Text, call.Body)
	}
	if call.FrontAgentID != "assistant" {
		t.Errorf("FrontAgentID: expected %q, got %q", "assistant", call.FrontAgentID)
	}
	if call.ConversationKey.ConnectorID != env.ConnectorID {
		t.Errorf("ConnectorID: expected %q, got %q", env.ConnectorID, call.ConversationKey.ConnectorID)
	}
	if call.ConversationKey.AccountID != env.AccountID {
		t.Errorf("AccountID: expected %q, got %q", env.AccountID, call.ConversationKey.AccountID)
	}
	if call.ConversationKey.ExternalID != env.ConversationID {
		t.Errorf("ExternalID: expected %q, got %q", env.ConversationID, call.ConversationKey.ExternalID)
	}
	if call.ConversationKey.ThreadID != env.ThreadID {
		t.Errorf("ThreadID: expected %q, got %q", env.ThreadID, call.ConversationKey.ThreadID)
	}
	if call.SourceMessageID != env.MessageID {
		t.Errorf("SourceMessageID: expected %q, got %q", env.MessageID, call.SourceMessageID)
	}
}

func TestInboundDispatcher_PassesLanguageHintToRuntime(t *testing.T) {
	ingress := &stubIngress{}
	dispatcher := NewInboundDispatcher(ingress, "assistant")

	env := model.Envelope{
		ConnectorID:    "telegram",
		AccountID:      "123456",
		ActorID:        "123456",
		ConversationID: "123456",
		ThreadID:       "main",
		MessageID:      "43",
		Text:           "duyet giup toi",
		Metadata: map[string]string{
			"language_hint": "vi",
		},
	}

	if err := dispatcher.Dispatch(context.Background(), env); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if len(ingress.calls) != 1 {
		t.Fatalf("expected 1 ReceiveInboundMessage call, got %d", len(ingress.calls))
	}
	if got := ingress.calls[0].LanguageHint; got != "vi" {
		t.Fatalf("expected language hint %q, got %q", "vi", got)
	}
}

func TestInboundDispatcher_EmptyTextIsRejected(t *testing.T) {
	ingress := &stubIngress{}
	dispatcher := NewInboundDispatcher(ingress, "assistant")

	env := model.Envelope{
		ConnectorID: "telegram",
		AccountID:   "123456",
		ThreadID:    "main",
		Text:        "",
	}

	if err := dispatcher.Dispatch(context.Background(), env); err == nil {
		t.Fatal("expected error for empty text, got nil")
	}
	if len(ingress.calls) != 0 {
		t.Fatalf("expected no ReceiveInboundMessage calls for empty text, got %d", len(ingress.calls))
	}
}
