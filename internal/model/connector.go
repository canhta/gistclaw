package model

import "context"

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
//	│  ID()                — connector identifier    │
//	└────────────────────────────────────────────────┘
type Connector interface {
	// ID returns the stable connector identifier stored in outbound_intents.connector_id.
	ID() string

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
