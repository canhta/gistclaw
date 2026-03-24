package whatsapp

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
	drainInterval time.Duration
}

func NewConnector(phoneNumberID, accessToken string, db *store.DB, cs *conversations.ConversationStore) *Connector {
	return &Connector{
		outbound:      NewOutboundDispatcher(phoneNumberID, accessToken, db, cs),
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

	for {
		if err := c.Drain(ctx); err != nil {
			log.Printf("whatsapp: drain warning: %v", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

var _ model.Connector = (*Connector)(nil)
