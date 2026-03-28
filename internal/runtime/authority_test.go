package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
)

type metadataOnlyConnector struct {
	metadata model.ConnectorMetadata
}

func (c *metadataOnlyConnector) Metadata() model.ConnectorMetadata { return c.metadata }

func (c *metadataOnlyConnector) Start(context.Context) error { return nil }

func (c *metadataOnlyConnector) Notify(context.Context, string, model.ReplayDelta, string) error {
	return nil
}

func (c *metadataOnlyConnector) Drain(context.Context) error { return nil }

func TestReceiveInboundMessageRejectsRegisteredRemoteConnectorWithAutoApproveElevated(t *testing.T) {
	rt, db := newCollaborationRuntime(t, []GenerateResult{
		{Content: "unsafe", StopReason: "end_turn"},
	})
	rt.SetConnectors([]model.Connector{
		&metadataOnlyConnector{
			metadata: model.ConnectorMetadata{
				ID:       "remote_bridge",
				Exposure: model.ConnectorExposureRemote,
			},
		},
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
			ConnectorID: "remote_bridge",
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
		t.Fatal("expected remote connector run to be rejected")
	}
	if !errors.Is(err, ErrRemoteConnectorUnsafeAuthority) {
		t.Fatalf("expected ErrRemoteConnectorUnsafeAuthority, got %v", err)
	}
	if !strings.Contains(err.Error(), "auto_approve") || !strings.Contains(err.Error(), "elevated") {
		t.Fatalf("expected auto_approve + elevated rejection, got %v", err)
	}
}
