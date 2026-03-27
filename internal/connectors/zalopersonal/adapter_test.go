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

func TestIncomingMessageFromProtocolMessageIgnoresNonTextDM(t *testing.T) {
	t.Parallel()

	msg := protocol.NewUserMessage("acct-1", protocol.TMessage{
		MsgID:   "msg-2",
		UIDFrom: "user-2",
		IDTo:    "acct-1",
		Content: protocol.Content{
			Raw: []byte(`{"title":"report.pdf","href":"https://example.com/report.pdf"}`),
		},
	})

	if incoming, ok := incomingMessageFromProtocolMessage("acct-1", "vi", msg); ok {
		t.Fatalf("expected non-text DM to be ignored, got %+v", incoming)
	}
}

func TestIncomingMessageFromProtocolMessageConvertsGroupMentions(t *testing.T) {
	t.Parallel()

	text := "@acct-1 review this"
	msg := protocol.NewGroupMessage("acct-1", protocol.TGroupMessage{
		TMessage: protocol.TMessage{
			MsgID:   "msg-group-1",
			UIDFrom: "user-3",
			IDTo:    "group-1",
			Content: protocol.Content{String: &text},
		},
		Mentions: []*protocol.TMention{
			{UID: "acct-1", Pos: 0, Len: 7, Type: protocol.MentionEach},
		},
	})

	incoming, ok := incomingMessageFromProtocolMessage("acct-1", "vi", msg)
	if !ok {
		t.Fatal("expected group message to convert")
	}
	if incoming.IsDirect {
		t.Fatalf("expected group message, got %+v", incoming)
	}
	if incoming.ConversationID != "group-1" {
		t.Fatalf("expected group conversation, got %+v", incoming)
	}
	if !incoming.Mentioned {
		t.Fatalf("expected group mention flag, got %+v", incoming)
	}
}
