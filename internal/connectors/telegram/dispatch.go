package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

// RunStarter is the minimal runtime interface required by InboundDispatcher.
// Defined here (consuming package) per the interfaces-in-consuming-package rule.
type RunStarter interface {
	Start(ctx context.Context, req runtime.StartRun) (model.Run, error)
}

// InboundDispatcher normalizes inbound Telegram updates and converts them
// into rt.Start() calls, resolving or creating a conversation record first.
//
//	Telegram Update
//	     │
//	NormalizeUpdate()
//	     │
//	model.Envelope
//	     │
//	InboundDispatcher.Dispatch()
//	     ├── convStore.Resolve(key) → conversationID
//	     └── rt.Start(StartRun{ConversationID, Objective, ...})
type InboundDispatcher struct {
	cs            *conversations.ConversationStore
	rt            RunStarter
	defaultAgentID string
	workspaceRoot  string
}

// NewInboundDispatcher creates a dispatcher that routes inbound envelopes to rt.Start().
// defaultAgentID is the agent assigned to new runs (e.g. "coordinator").
// workspaceRoot is passed through to StartRun; may be empty if read from settings.
func NewInboundDispatcher(cs *conversations.ConversationStore, rt RunStarter, defaultAgentID, workspaceRoot string) *InboundDispatcher {
	return &InboundDispatcher{
		cs:             cs,
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

	conv, err := d.cs.Resolve(ctx, conversations.ConversationKey{
		ConnectorID: env.ConnectorID,
		AccountID:   env.AccountID,
		ExternalID:  env.MessageID,
		ThreadID:    env.ThreadID,
	})
	if err != nil {
		return fmt.Errorf("telegram: inbound dispatch: resolve conversation: %w", err)
	}

	_, err = d.rt.Start(ctx, runtime.StartRun{
		ConversationID: conv.ID,
		AgentID:        d.defaultAgentID,
		Objective:      env.Text,
		WorkspaceRoot:  d.workspaceRoot,
		AccountID:      env.AccountID,
	})
	if err != nil {
		return fmt.Errorf("telegram: inbound dispatch: start run: %w", err)
	}

	return nil
}
