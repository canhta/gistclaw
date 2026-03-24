package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

// FrontSessionStarter is the minimal runtime interface required by InboundDispatcher.
// Defined here (consuming package) per the interfaces-in-consuming-package rule.
type FrontSessionStarter interface {
	StartFrontSession(ctx context.Context, req runtime.StartFrontSession) (model.Run, error)
}

// InboundDispatcher normalizes inbound Telegram updates and converts them
// into rt.StartFrontSession() calls with a stable chat/thread identity.
//
//	Telegram Update
//	     │
//	NormalizeUpdate()
//	     │
//	model.Envelope
//	     │
//	InboundDispatcher.Dispatch()
//	     └── rt.StartFrontSession(StartFrontSession{ConversationKey, InitialPrompt, ...})
type InboundDispatcher struct {
	rt             FrontSessionStarter
	defaultAgentID string
	workspaceRoot  string
}

// NewInboundDispatcher creates a dispatcher that routes inbound envelopes to rt.StartFrontSession().
// defaultAgentID is the agent assigned to new runs (e.g. "coordinator").
// workspaceRoot is passed through to StartRun; may be empty if read from settings.
func NewInboundDispatcher(rt FrontSessionStarter, defaultAgentID, workspaceRoot string) *InboundDispatcher {
	return &InboundDispatcher{
		rt:             rt,
		defaultAgentID: defaultAgentID,
		workspaceRoot:  workspaceRoot,
	}
}

// Dispatch converts env into a runtime.StartRun, resolving the conversation first.
// Returns an error if env.Text is empty (no objective to run).
func (d *InboundDispatcher) Dispatch(ctx context.Context, env model.Envelope) error {
	if strings.TrimSpace(env.Text) == "" {
		return fmt.Errorf("telegram: inbound dispatch: text is required")
	}

	_, err := d.rt.StartFrontSession(ctx, runtime.StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: env.ConnectorID,
			AccountID:   env.AccountID,
			ExternalID:  env.ConversationID,
			ThreadID:    env.ThreadID,
		},
		FrontAgentID:  d.defaultAgentID,
		InitialPrompt: env.Text,
		WorkspaceRoot: d.workspaceRoot,
	})
	if err != nil {
		return fmt.Errorf("telegram: inbound dispatch: start front session: %w", err)
	}

	return nil
}
