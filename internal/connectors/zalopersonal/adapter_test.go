package zalopersonal

import (
	"testing"

	"github.com/canhta/gistclaw/internal/connectors/zalopersonal/protocol"
)

func TestIncomingMessageFromProtocolMessage(t *testing.T) {
	t.Parallel()

	text := "review auth"
	msg := protocol.NewUserMessage("acct-1", protocol.TMessage{
		MsgID:   "msg-1",
		UIDFrom: "user-1",
		IDTo:    "acct-1",
		Content: protocol.Content{String: &text},
	})

	incoming, ok := incomingMessageFromProtocolMessage("acct-1", "vi", msg)
	if !ok {
		t.Fatal("expected protocol message to convert")
	}
	if incoming.AccountID != "acct-1" {
		t.Fatalf("expected account acct-1, got %q", incoming.AccountID)
	}
	if incoming.SenderID != "user-1" {
		t.Fatalf("expected sender user-1, got %q", incoming.SenderID)
	}
	if incoming.ConversationID != "user-1" {
		t.Fatalf("expected conversation user-1, got %q", incoming.ConversationID)
	}
	if incoming.MessageID != "msg-1" {
		t.Fatalf("expected message id msg-1, got %q", incoming.MessageID)
	}
	if incoming.Text != "review auth" {
		t.Fatalf("expected text review auth, got %q", incoming.Text)
	}
	if incoming.LanguageHint != "vi" {
		t.Fatalf("expected language vi, got %q", incoming.LanguageHint)
	}
	if !incoming.IsDirect {
		t.Fatal("expected direct message")
	}
}
