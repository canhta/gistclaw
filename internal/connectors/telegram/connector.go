package telegram

import (
	"context"
	"log"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

type Connector struct {
	outbound      *OutboundDispatcher
	inbound       *InboundDispatcher
	drainInterval time.Duration
}

func NewConnector(
	token string,
	db *store.DB,
	cs *conversations.ConversationStore,
	rt FrontSessionStarter,
	defaultAgentID string,
	workspaceRoot string,
) *Connector {
	outbound := NewOutboundDispatcher(token, db, cs)
	inbound := NewInboundDispatcher(rt, defaultAgentID, workspaceRoot)

	outbound.bot.handler = func(ctx context.Context, upd Update) {
		env, err := NormalizeUpdate(upd)
		if err != nil {
			log.Printf("telegram: normalize update: %v", err)
			return
		}
		if err := inbound.Dispatch(ctx, env); err != nil {
			log.Printf("telegram: dispatch update: %v", err)
		}
	}

	return &Connector{
		outbound:      outbound,
		inbound:       inbound,
		drainInterval: time.Second,
	}
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
