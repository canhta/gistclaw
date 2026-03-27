package zalopersonal

import "testing"

func TestNormalizeInboundMessage(t *testing.T) {
	t.Parallel()

	t.Run("dm text normalizes to envelope", func(t *testing.T) {
		t.Parallel()

		env, err := NormalizeInboundMessage(IncomingMessage{
			AccountID:      "acct-1",
			SenderID:       "user-1",
			ConversationID: "user-1",
			MessageID:      "msg-1",
			Text:           "  xin chao  ",
			IsDirect:       true,
			LanguageHint:   "vi",
		})
		if err != nil {
			t.Fatalf("NormalizeInboundMessage: %v", err)
		}
		if env.ConnectorID != "zalo_personal" {
			t.Fatalf("expected connector zalo_personal, got %q", env.ConnectorID)
		}
		if env.AccountID != "acct-1" || env.ActorID != "user-1" || env.ConversationID != "user-1" {
			t.Fatalf("unexpected identity: %+v", env)
		}
		if env.ThreadID != "main" {
			t.Fatalf("expected main thread, got %q", env.ThreadID)
		}
		if env.Text != "xin chao" {
			t.Fatalf("expected trimmed text, got %q", env.Text)
		}
		if env.Metadata["language_hint"] != "vi" {
			t.Fatalf("expected language hint vi, got %+v", env.Metadata)
		}
	})

	t.Run("non dm is rejected", func(t *testing.T) {
		t.Parallel()

		_, err := NormalizeInboundMessage(IncomingMessage{
			AccountID:      "acct-1",
			SenderID:       "user-1",
			ConversationID: "thread-1",
			MessageID:      "msg-1",
			Text:           "hello",
			IsDirect:       false,
		})
		if err == nil {
			t.Fatal("expected non-DM message to be rejected")
		}
	})
}
