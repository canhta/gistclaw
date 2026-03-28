package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

// InboundMessageReceiver is the minimal runtime interface required by InboundDispatcher.
// Defined here (consuming package) per the interfaces-in-consuming-package rule.
type InboundMessageReceiver interface {
	ReceiveInboundMessage(ctx context.Context, req runtime.InboundMessageCommand) (model.Run, error)
}

// InboundDispatcher normalizes inbound Telegram updates and converts them
// into rt.ReceiveInboundMessage() calls with a stable chat/thread identity.
//
//	Telegram Update
//	     │
//	NormalizeUpdate()
//	     │
//	model.Envelope
//	     │
//	InboundDispatcher.Dispatch()
//	     └── rt.ReceiveInboundMessage(InboundMessageCommand{ConversationKey, Body, ...})
type InboundDispatcher struct {
	rt             InboundMessageReceiver
	defaultAgentID string
}

// NewInboundDispatcher creates a dispatcher that routes inbound envelopes to rt.ReceiveInboundMessage().
// defaultAgentID is the front agent assigned to new runs for this connector account.
func NewInboundDispatcher(rt InboundMessageReceiver, defaultAgentID string) *InboundDispatcher {
	return &InboundDispatcher{
		rt:             rt,
		defaultAgentID: defaultAgentID,
	}
}

// Dispatch converts env into a runtime.StartRun, resolving the conversation first.
// Returns an error if env.Text is empty (no objective to run).
func (d *InboundDispatcher) Dispatch(ctx context.Context, env model.Envelope) error {
	if strings.TrimSpace(env.Text) == "" {
		return fmt.Errorf("telegram: inbound dispatch: text is required")
	}

	_, err := d.rt.ReceiveInboundMessage(ctx, runtime.InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: env.ConnectorID,
			AccountID:   env.AccountID,
			ExternalID:  env.ConversationID,
			ThreadID:    env.ThreadID,
		},
		FrontAgentID:    d.defaultAgentID,
		Body:            env.Text,
		SourceMessageID: env.MessageID,
		LanguageHint:    env.Metadata["language_hint"],
	})
	if err != nil {
		return fmt.Errorf("telegram: inbound dispatch: receive inbound message: %w", err)
	}

	return nil
}
