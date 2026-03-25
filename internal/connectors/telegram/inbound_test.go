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

func TestInbound_SlashCommandsArePreserved(t *testing.T) {
	for _, cmd := range []string{"/start", "/help", "/status"} {
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
			t.Errorf("slash-prefixed text %q should not error: %v", cmd, err)
			continue
		}
		if env.Text != cmd {
			t.Errorf("%q: expected text to be preserved, got %q", cmd, env.Text)
		}
	}
}

func TestInbound_LegacyTaskAliasesArePreservedAsPlainText(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "run alias",
			text: "/run review the repo",
			want: "/run review the repo",
		},
		{
			name: "task alias",
			text: "/task review the repo",
			want: "/task review the repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upd := Update{
				UpdateID: 201,
				Message: &Message{
					MessageID: 201,
					Chat:      Chat{ID: 1, Type: "private"},
					Text:      tt.text,
				},
			}

			env, err := NormalizeUpdate(upd)
			if err != nil {
				t.Fatalf("NormalizeUpdate: %v", err)
			}
			if env.Text != tt.want {
				t.Fatalf("expected preserved text %q, got %q", tt.want, env.Text)
			}
		})
	}
}

func TestInbound_SlashPrefixedChatPassesThrough(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "unknown command",
			text: "/unknown_command",
			want: "/unknown_command",
		},
		{
			name: "filesystem-like path",
			text: "/Users/canh/Projects/OSS/gistclaw",
			want: "/Users/canh/Projects/OSS/gistclaw",
		},
		{
			name: "slash note with spaces",
			text: "/review the auth package",
			want: "/review the auth package",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upd := Update{
				UpdateID: 202,
				Message: &Message{
					MessageID: 202,
					Chat:      Chat{ID: 99, Type: "private"},
					Text:      tt.text,
				},
			}

			env, err := NormalizeUpdate(upd)
			if err != nil {
				t.Fatalf("NormalizeUpdate: %v", err)
			}
			if env.Text != tt.want {
				t.Fatalf("expected text %q, got %q", tt.want, env.Text)
			}
		})
	}
}
