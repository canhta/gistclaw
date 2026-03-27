package protocol

import "testing"

func TestNewUserMessageResolvesThreadAndText(t *testing.T) {
	t.Parallel()

	text := "hello"
	msg := NewUserMessage("acct-1", TMessage{
		MsgID:   "msg-1",
		UIDFrom: "user-1",
		IDTo:    "acct-1",
		Content: Content{String: &text},
	})

	if msg.Type() != ThreadTypeUser {
		t.Fatalf("expected user thread type, got %d", msg.Type())
	}
	if msg.ThreadID() != "user-1" {
		t.Fatalf("expected thread user-1, got %q", msg.ThreadID())
	}
	if msg.SenderID() != "user-1" {
		t.Fatalf("expected sender user-1, got %q", msg.SenderID())
	}
	if msg.MessageID() != "msg-1" {
		t.Fatalf("expected message id msg-1, got %q", msg.MessageID())
	}
	if msg.Text() != "hello" {
		t.Fatalf("expected text hello, got %q", msg.Text())
	}
}
