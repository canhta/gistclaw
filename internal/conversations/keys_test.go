package conversations

import "testing"

func TestConversationKey_SameInputSameKey(t *testing.T) {
	k1 := ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct1",
		ExternalID:  "chat123",
		ThreadID:    "thread1",
	}
	k2 := ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct1",
		ExternalID:  "chat123",
		ThreadID:    "thread1",
	}

	if k1.Normalize() != k2.Normalize() {
		t.Fatalf("same input must produce same key: %q != %q", k1.Normalize(), k2.Normalize())
	}
}

func TestConversationKey_MissingThreadNormalizesToMain(t *testing.T) {
	k := ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct1",
		ExternalID:  "chat123",
		ThreadID:    "",
	}

	if got := k.Normalize(); got != "telegram:acct1:chat123:main" {
		t.Fatalf("expected %q, got %q", "telegram:acct1:chat123:main", got)
	}
}

func TestConversationKey_ActorIDDoesNotAffectKey(t *testing.T) {
	k1 := ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct1",
		ExternalID:  "chat123",
		ThreadID:    "main",
	}
	k2 := ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct1",
		ExternalID:  "chat123",
		ThreadID:    "main",
	}

	if k1.Normalize() != k2.Normalize() {
		t.Fatal("actor should not affect key")
	}
}

func TestConversationKey_NoTeamID(t *testing.T) {
	_ = ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct1",
		ExternalID:  "chat123",
		ThreadID:    "main",
	}
}

func TestConversationKey_TeamReassignmentDoesNotChangeKey(t *testing.T) {
	k1 := ConversationKey{ConnectorID: "tg", AccountID: "a", ExternalID: "c", ThreadID: "main"}
	k2 := ConversationKey{ConnectorID: "tg", AccountID: "a", ExternalID: "c", ThreadID: "main"}
	if k1.Normalize() != k2.Normalize() {
		t.Fatal("expected same key regardless of team assignment")
	}
}

func TestConversationKey_EscapesColons(t *testing.T) {
	k := ConversationKey{
		ConnectorID: "conn:or",
		AccountID:   "acct",
		ExternalID:  "ext",
		ThreadID:    "main",
	}

	if got := k.Normalize(); got == "conn:or:acct:ext:main" {
		t.Fatalf("colons in components must be escaped, got %q", got)
	}
}
