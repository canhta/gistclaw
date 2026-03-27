package runtime

import (
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestParseApprovalCommandReplyUsesGateLanguageHintForUsageErrors(t *testing.T) {
	t.Parallel()

	_, err := parseApprovalCommandReply("/approve", model.ConversationGate{
		ApprovalID:   "ticket-1",
		LanguageHint: "vi",
	})
	if err == nil {
		t.Fatal("expected usage error for incomplete approve command")
	}
	if !strings.Contains(err.Error(), "Cú pháp") {
		t.Fatalf("expected localized usage message, got %q", err.Error())
	}
}
