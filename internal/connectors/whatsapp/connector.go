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
	health        *HealthState
}

func NewConnector(phoneNumberID, accessToken string, db *store.DB, cs *conversations.ConversationStore, health *HealthState) *Connector {
	if health == nil {
		health = NewHealthState(nil)
	}
	return &Connector{
		outbound:      NewOutboundDispatcher(phoneNumberID, accessToken, db, cs, health),
		drainInterval: time.Second,
		health:        health,
	}
}

func (c *Connector) Metadata() model.ConnectorMetadata {
	return model.NormalizeConnectorMetadata(model.ConnectorMetadata{
		ID:       c.outbound.ID(),
		Exposure: model.ConnectorExposureRemote,
	})
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

	for {
		if err := c.Drain(ctx); err != nil {
			c.health.markFailure(time.Now().UTC(), "drain: "+err.Error())
			log.Printf("whatsapp: drain warning: %v", err)
		} else {
			c.health.markDeliverySuccess(time.Now().UTC())
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

var _ model.Connector = (*Connector)(nil)
