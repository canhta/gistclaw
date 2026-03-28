package model

import (
	"context"
	"strings"
)

type ConnectorExposure string

const (
	ConnectorExposureLocal  ConnectorExposure = "local"
	ConnectorExposureRemote ConnectorExposure = "remote"
)

type ConnectorMetadata struct {
	ID       string
	Exposure ConnectorExposure
}

func NormalizeConnectorMetadata(meta ConnectorMetadata) ConnectorMetadata {
	meta.ID = strings.TrimSpace(meta.ID)
	switch meta.Exposure {
	case ConnectorExposureLocal, ConnectorExposureRemote:
	default:
		meta.Exposure = ConnectorExposureRemote
	}
	return meta
}

// Connector is the interface implemented by every inbound/outbound channel adapter
// (Telegram, WhatsApp, email, Slack, etc.). The runtime depends only on this
// interface — never on a concrete connector type.
//
//	┌────────────────────────────────────────────────┐
//	│              model.Connector                   │
//	│                                                │
//	│  Start(ctx)          — long-poll / webhook     │
//	│  Notify(ctx, …)      — outbound delivery       │
//	│  Drain(ctx)          — resume pending intents  │
//	│  Metadata()          — connector descriptor    │
//	└────────────────────────────────────────────────┘
type Connector interface {
	// Metadata returns the stable connector descriptor used by runtime policy,
	// routing, and connector capability registration.
	Metadata() ConnectorMetadata

	// Start runs the connector's receive loop until ctx is cancelled.
	// For polling connectors (Telegram long-poll) this blocks until shutdown.
	// For webhook connectors this may return immediately after registration.
	Start(ctx context.Context) error

	// Notify records an outbound intent and attempts immediate delivery.
	// dedupeKey prevents re-delivery if the same key has already been delivered.
	Notify(ctx context.Context, chatID string, delta ReplayDelta, dedupeKey string) error

	// Drain delivers all pending or retrying outbound intents from a prior session.
	// Called once on startup before the connector begins receiving new events.
	Drain(ctx context.Context) error
}
