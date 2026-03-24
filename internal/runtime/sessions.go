package runtime

import (
	"context"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/sessions"
)

func (r *Runtime) ListSessions(ctx context.Context, conversationID string, limit int) ([]model.Session, error) {
	return sessions.NewService(r.store, r.convStore).ListConversationSessions(ctx, conversationID, limit)
}

func (r *Runtime) ListAllSessions(ctx context.Context, limit int) ([]model.Session, error) {
	return sessions.NewService(r.store, r.convStore).ListSessions(ctx, limit)
}

func (r *Runtime) SessionHistory(ctx context.Context, sessionID string, limit int) (model.Session, []model.SessionMessage, error) {
	return sessions.NewService(r.store, r.convStore).LoadSessionMailbox(ctx, sessionID, limit)
}

func (r *Runtime) SessionDeliveryState(ctx context.Context, sessionID string, limit int) ([]model.OutboundIntent, []model.DeliveryFailure, error) {
	svc := sessions.NewService(r.store, r.convStore)

	deliveries, err := svc.ListSessionOutboundIntents(ctx, sessionID, limit)
	if err != nil {
		return nil, nil, err
	}
	failures, err := svc.ListSessionDeliveryFailures(ctx, sessionID, limit)
	if err != nil {
		return nil, nil, err
	}
	return deliveries, failures, nil
}
