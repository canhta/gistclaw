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
	HandleConversationGateReply(ctx context.Context, req runtime.ConversationGateReplyCommand) (runtime.ConversationGateReplyOutcome, error)
	InspectConversation(ctx context.Context, key conversations.ConversationKey) (runtime.ConversationStatus, error)
	ResetConversation(ctx context.Context, key conversations.ConversationKey) (runtime.ConversationResetOutcome, error)
}

type Connector struct {
	outbound      *OutboundDispatcher
	sender        MessageSender
	rt            ConnectorRuntime
	inbound       *InboundDispatcher
	commands      *controlconnector.Dispatcher
	drainInterval time.Duration
	health        *healthState
}

func NewConnector(
	token string,
	db *store.DB,
	cs *conversations.ConversationStore,
	rt ConnectorRuntime,
	defaultAgentID string,
) *Connector {
	health := newHealthState(nil)
	outbound := NewOutboundDispatcher(token, db, cs, health)
	inbound := NewInboundDispatcher(rt, defaultAgentID)
	connector := &Connector{
		outbound:      outbound,
		sender:        outbound,
		rt:            rt,
		inbound:       inbound,
		commands:      controlconnector.NewDispatcher(rt),
		drainInterval: 250 * time.Millisecond,
		health:        health,
	}
	outbound.bot.commandSpecs = controlconnector.DefaultCommandSpecs()
	outbound.bot.onPollSuccess = health.markPollSuccess
	outbound.bot.onPollError = func(at time.Time, err error) {
		health.markFailure(at, "poll: "+err.Error())
	}
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

func (c *Connector) ConnectorHealthSnapshot() model.ConnectorHealthSnapshot {
	return c.health.snapshot()
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
			c.health.markFailure(time.Now().UTC(), "draft flush: "+err.Error())
			log.Printf("telegram: draft flush warning: %v", err)
		}

		if err := c.Drain(ctx); err != nil {
			c.health.markFailure(time.Now().UTC(), "drain: "+err.Error())
			log.Printf("telegram: drain warning: %v", err)
		} else {
			c.health.markDrainSuccess(time.Now().UTC())
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
		return
	}
	if upd.CallbackQuery != nil {
		if err := c.outbound.bot.answerCallbackQuery(ctx, upd.CallbackQuery.ID); err != nil {
			log.Printf("telegram: answer callback query: %v", err)
		}
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

	gateOutcome, err := c.rt.HandleConversationGateReply(ctx, runtime.ConversationGateReplyCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: env.ConnectorID,
			AccountID:   env.AccountID,
			ExternalID:  env.ConversationID,
			ThreadID:    env.ThreadID,
		},
		Body:            env.Text,
		SourceMessageID: env.MessageID,
		LanguageHint:    env.Metadata["language_hint"],
		ProjectID:       env.Metadata["project_id"],
		CWD:             env.Metadata["cwd"],
	})
	if err != nil {
		return err
	}
	if gateOutcome.Handled {
		return nil
	}
	return c.inbound.Dispatch(ctx, env)
}
