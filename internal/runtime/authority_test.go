package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
)

func TestReceiveInboundMessageRejectsZaloPersonalConnectorWithAutoApproveElevated(t *testing.T) {
	rt, db := newCollaborationRuntime(t, []GenerateResult{
		{Content: "unsafe", StopReason: "end_turn"},
	})
	if _, err := db.RawDB().Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES
		 ('approval_mode', 'auto_approve', datetime('now')),
		 ('host_access_mode', 'elevated', datetime('now'))`,
	); err != nil {
		t.Fatalf("insert authority settings: %v", err)
	}

	_, err := rt.ReceiveInboundMessage(context.Background(), InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "zalo_personal",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		FrontAgentID:    "assistant",
		Body:            "Inspect the repo.",
		SourceMessageID: "zalo-1",
		CWD:             t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected zalo personal connector run to be rejected")
	}
	if !errors.Is(err, ErrRemoteConnectorUnsafeAuthority) {
		t.Fatalf("expected ErrRemoteConnectorUnsafeAuthority, got %v", err)
	}
	if !strings.Contains(err.Error(), "auto_approve") || !strings.Contains(err.Error(), "elevated") {
		t.Fatalf("expected auto_approve + elevated rejection, got %v", err)
	}
}
