package telegram

import (
	"context"
	"log"
	"time"

	controlconnector "github.com/canhta/gistclaw/internal/connectors/control"
	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
)

type MessageSender interface {
	Send(ctx context.Context, chatID, text string) error
}

type ConnectorRuntime interface {
	ReceiveInboundMessage(ctx context.Context, req runtime.InboundMessageCommand) (model.Run, error)
	InspectConversation(ctx context.Context, key conversations.ConversationKey) (runtime.ConversationStatus, error)
}

type Connector struct {
	outbound      *OutboundDispatcher
	sender        MessageSender
	inbound       *InboundDispatcher
	commands      *controlconnector.Dispatcher
	drainInterval time.Duration
}

func NewConnector(
	token string,
	db *store.DB,
	cs *conversations.ConversationStore,
	rt ConnectorRuntime,
	defaultAgentID string,
	workspaceRoot string,
) *Connector {
	outbound := NewOutboundDispatcher(token, db, cs)
	inbound := NewInboundDispatcher(rt, defaultAgentID, workspaceRoot)
	connector := &Connector{
		outbound:      outbound,
		sender:        outbound,
		inbound:       inbound,
		commands:      controlconnector.NewDispatcher(rt),
		drainInterval: 250 * time.Millisecond,
	}
	outbound.bot.commandSpecs = controlconnector.DefaultCommandSpecs()

	outbound.bot.handler = connector.handleUpdate

	return connector
}

func (c *Connector) ID() string {
	return c.outbound.ID()
}

func (c *Connector) Notify(ctx context.Context, chatID string, delta model.ReplayDelta, dedupeKey string) error {
	return c.outbound.Notify(ctx, chatID, delta, dedupeKey)
}

func (c *Connector) Drain(ctx context.Context) error {
	return c.outbound.Drain(ctx)
}

func (c *Connector) Start(ctx context.Context) error {
	ticker := time.NewTicker(c.drainInterval)
	defer ticker.Stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.outbound.bot.Start(ctx)
	}()

	for {
		if err := c.outbound.FlushDrafts(ctx); err != nil {
			log.Printf("telegram: draft flush warning: %v", err)
		}

		if err := c.Drain(ctx); err != nil {
			log.Printf("telegram: drain warning: %v", err)
		}

		select {
		case err := <-errCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

var _ model.Connector = (*Connector)(nil)

func (c *Connector) handleUpdate(ctx context.Context, upd Update) {
	env, err := NormalizeUpdate(upd)
	if err != nil {
		log.Printf("telegram: normalize update: %v", err)
		return
	}
	if err := c.handleEnvelope(ctx, env); err != nil {
		log.Printf("telegram: dispatch update: %v", err)
	}
}

func (c *Connector) handleEnvelope(ctx context.Context, env model.Envelope) error {
	reply, handled, err := c.commands.Dispatch(ctx, env)
	if err != nil {
		return err
	}
	if handled {
		return c.sender.Send(ctx, env.ConversationID, reply)
	}
	return c.inbound.Dispatch(ctx, env)
}
