package protocol

import (
	"encoding/json"
	"testing"
)

func TestContentUnmarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("string content", func(t *testing.T) {
		var content Content
		if err := json.Unmarshal([]byte(`"hello world"`), &content); err != nil {
			t.Fatalf("Unmarshal string content: %v", err)
		}
		if content.String == nil || *content.String != "hello world" {
			t.Fatalf("expected string content, got %+v", content)
		}
		if len(content.Raw) != 0 {
			t.Fatalf("expected no raw content, got %s", string(content.Raw))
		}
	})

	t.Run("object content", func(t *testing.T) {
		var content Content
		if err := json.Unmarshal([]byte(`{"title":"report.pdf","href":"https://example.com/report.pdf"}`), &content); err != nil {
			t.Fatalf("Unmarshal object content: %v", err)
		}
		if content.String != nil {
			t.Fatalf("expected no string content, got %q", *content.String)
		}
		if string(content.Raw) == "" {
			t.Fatal("expected raw attachment content")
		}
	})
}

func TestContentAttachmentText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    string
		isImage bool
	}{
		{
			name:    "image with title",
			raw:     `{"title":"photo","href":"https://example.com/photo.jpg"}`,
			want:    "[User sent an image: photo]",
			isImage: true,
		},
		{
			name:    "image without title",
			raw:     `{"title":"","href":"https://example.com/photo.png"}`,
			want:    "[User sent an image]",
			isImage: true,
		},
		{
			name:    "file with title",
			raw:     `{"title":"report.pdf","href":"https://example.com/report.pdf"}`,
			want:    "[User sent a file: report.pdf]",
			isImage: false,
		},
		{
			name:    "unknown attachment",
			raw:     `{"type":"sticker"}`,
			want:    "[User sent a non-text message]",
			isImage: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := Content{Raw: []byte(tt.raw)}
			attachment := content.ParseAttachment()
			if attachment == nil {
				t.Fatal("expected parsed attachment")
			}
			if attachment.IsImage() != tt.isImage {
				t.Fatalf("expected isImage=%v, got %v", tt.isImage, attachment.IsImage())
			}
			if got := content.AttachmentText(); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

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

func TestNewUserMessageResolvesSelfSentMessage(t *testing.T) {
	t.Parallel()

	msg := NewUserMessage("acct-1", TMessage{
		MsgID:   "msg-self",
		UIDFrom: DefaultUIDSelf,
		IDTo:    "user-2",
	})

	if !msg.IsSelf() {
		t.Fatal("expected self-sent message")
	}
	if msg.ThreadID() != "user-2" {
		t.Fatalf("expected thread user-2, got %q", msg.ThreadID())
	}
	if msg.SenderID() != "acct-1" {
		t.Fatalf("expected sender acct-1, got %q", msg.SenderID())
	}
}

func TestNewGroupMessageResolvesMentionAndThread(t *testing.T) {
	t.Parallel()

	text := "@acct-1 review"
	msg := NewGroupMessage("acct-1", TGroupMessage{
		TMessage: TMessage{
			MsgID:   "group-msg-1",
			UIDFrom: "user-9",
			IDTo:    "group-1",
			Content: Content{String: &text},
		},
		Mentions: []*TMention{
			{UID: "acct-1", Pos: 0, Len: 7, Type: MentionEach},
		},
	})

	if msg.Type() != ThreadTypeGroup {
		t.Fatalf("expected group thread type, got %d", msg.Type())
	}
	if msg.ThreadID() != "group-1" {
		t.Fatalf("expected group thread, got %q", msg.ThreadID())
	}
	if !msg.MentionsAccount("acct-1") {
		t.Fatalf("expected group message to mention acct-1")
	}
	if msg.SenderID() != "user-9" {
		t.Fatalf("expected sender user-9, got %q", msg.SenderID())
	}
}
