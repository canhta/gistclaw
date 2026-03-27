package zalopersonal

import (
	"context"

	"github.com/canhta/gistclaw/internal/model"
)

// Connector is the cold-start connector shell for Zalo Personal. The transport
// implementation lands in later tasks; for now this keeps config/bootstrap and
// connector health wiring compile-safe.
type Connector struct{}

func NewConnector() *Connector {
	return &Connector{}
}

func (c *Connector) ID() string {
	return "zalo_personal"
}

func (c *Connector) Start(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (c *Connector) Notify(context.Context, string, model.ReplayDelta, string) error {
	return nil
}

func (c *Connector) Drain(context.Context) error {
	return nil
}

func (c *Connector) ConnectorHealthSnapshot() model.ConnectorHealthSnapshot {
	return model.ConnectorHealthSnapshot{
		ConnectorID: c.ID(),
		State:       model.ConnectorHealthUnknown,
		Summary:     "awaiting first authentication",
	}
}

var _ model.Connector = (*Connector)(nil)
