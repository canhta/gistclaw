package telegram

import (
	"strings"
	"testing"
)

func TestInbound_DMNormalizes(t *testing.T) {
	upd := Update{
		UpdateID: 100,
		Message: &Message{
			MessageID: 42,
			Chat: Chat{
				ID:   123456,
				Type: "private",
			},
			Text: "review this repo",
		},
	}

	env, err := NormalizeUpdate(upd)
	if err != nil {
		t.Fatalf("NormalizeUpdate: %v", err)
	}

	if env.ConnectorID != "telegram" {
		t.Errorf("ConnectorID: expected telegram, got %q", env.ConnectorID)
	}
	if env.AccountID != "123456" {
		t.Errorf("AccountID: expected 123456, got %q", env.AccountID)
	}
	if env.MessageID != "42" {
		t.Errorf("MessageID: expected 42, got %q", env.MessageID)
	}
	if env.ThreadID != "main" {
		t.Errorf("ThreadID: expected main, got %q", env.ThreadID)
	}
	if env.Text != "review this repo" {
		t.Errorf("Text: expected %q, got %q", "review this repo", env.Text)
	}
}

func TestInbound_GroupChatRejected(t *testing.T) {
	upd := Update{
		UpdateID: 101,
		Message: &Message{
			MessageID: 10,
			Chat: Chat{
				ID:   -1001234567,
				Type: "supergroup",
			},
			Text: "hello group",
		},
	}

	_, err := NormalizeUpdate(upd)
	if err == nil {
		t.Fatal("expected error for group chat, got nil")
	}
	if !strings.Contains(err.Error(), "DM only") {
		t.Errorf("expected 'DM only' in error, got: %v", err)
	}
}

func TestInbound_UnrecognizedCommandRejected(t *testing.T) {
	upd := Update{
		UpdateID: 102,
		Message: &Message{
			MessageID: 11,
			Chat: Chat{
				ID:   99,
				Type: "private",
			},
			Text: "/unknown_command",
		},
	}

	_, err := NormalizeUpdate(upd)
	if err == nil {
		t.Fatal("expected error for unrecognized command, got nil")
	}
	if !strings.Contains(err.Error(), "unrecognized command") {
		t.Errorf("expected 'unrecognized command' in error, got: %v", err)
	}
}

func TestInbound_MessageTextVerbatim(t *testing.T) {
	text := "explain the auth module in plain English"
	upd := Update{
		UpdateID: 103,
		Message: &Message{
			MessageID: 55,
			Chat: Chat{
				ID:   777,
				Type: "private",
			},
			Text: text,
		},
	}

	env, err := NormalizeUpdate(upd)
	if err != nil {
		t.Fatalf("NormalizeUpdate: %v", err)
	}
	if env.Text != text {
		t.Errorf("expected Text %q, got %q", text, env.Text)
	}
}

func TestInbound_RecognizedCommandsPass(t *testing.T) {
	for _, cmd := range []string{"/start", "/help", "/run", "/task"} {
		upd := Update{
			UpdateID: 200,
			Message: &Message{
				MessageID: 200,
				Chat:      Chat{ID: 1, Type: "private"},
				Text:      cmd,
			},
		}
		env, err := NormalizeUpdate(upd)
		if err != nil {
			t.Errorf("recognized command %q should not error: %v", cmd, err)
			continue
		}
		if env.ConnectorID != "telegram" {
			t.Errorf("%q: expected ConnectorID telegram, got %q", cmd, env.ConnectorID)
		}
	}
}
