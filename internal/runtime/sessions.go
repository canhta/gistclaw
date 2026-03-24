package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/sessions"
)

var ErrDeliveryNotFound = fmt.Errorf("runtime: delivery not found")
var ErrDeliveryNotRetryable = fmt.Errorf("runtime: delivery not retryable")

func (r *Runtime) ListSessions(ctx context.Context, conversationID string, limit int) ([]model.Session, error) {
	return sessions.NewService(r.store, r.convStore).ListConversationSessions(ctx, conversationID, limit)
}

func (r *Runtime) ListAllSessions(ctx context.Context, limit int) ([]model.Session, error) {
	return sessions.NewService(r.store, r.convStore).ListSessions(ctx, limit)
}

func (r *Runtime) ListRoutes(ctx context.Context, connectorID string, limit int) ([]model.RouteDirectoryItem, error) {
	return sessions.NewService(r.store, r.convStore).ListRoutes(ctx, connectorID, limit)
}

func (r *Runtime) SessionHistory(ctx context.Context, sessionID string, limit int) (model.Session, []model.SessionMessage, error) {
	return sessions.NewService(r.store, r.convStore).LoadSessionMailbox(ctx, sessionID, limit)
}

func (r *Runtime) ConnectorDeliveryHealth(ctx context.Context) ([]model.ConnectorDeliveryHealth, error) {
	return sessions.NewService(r.store, r.convStore).ListConnectorDeliveryHealth(ctx)
}

func (r *Runtime) ListDeliveries(ctx context.Context, connectorID, status string, limit int) ([]model.DeliveryQueueItem, error) {
	return sessions.NewService(r.store, r.convStore).ListDeliveryQueue(ctx, sessions.DeliveryQueueFilter{
		ConnectorID: connectorID,
		Status:      status,
		Limit:       limit,
	})
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

func (r *Runtime) RetrySessionDelivery(ctx context.Context, sessionID string, intentID string) (model.OutboundIntent, error) {
	svc := sessions.NewService(r.store, r.convStore)
	session, err := svc.LoadSession(ctx, sessionID)
	if err != nil {
		return model.OutboundIntent{}, err
	}

	intent, err := svc.LoadSessionOutboundIntent(ctx, sessionID, intentID)
	if err != nil {
		if errors.Is(err, sessions.ErrOutboundIntentNotFound) {
			return model.OutboundIntent{}, ErrDeliveryNotFound
		}
		return model.OutboundIntent{}, err
	}
	return r.retryDelivery(ctx, session.ConversationID, sessionID, intent)
}

func (r *Runtime) RetryDelivery(ctx context.Context, intentID string) (model.OutboundIntent, error) {
	svc := sessions.NewService(r.store, r.convStore)
	item, err := svc.LoadDeliveryQueueItem(ctx, intentID)
	if err != nil {
		if errors.Is(err, sessions.ErrOutboundIntentNotFound) {
			return model.OutboundIntent{}, ErrDeliveryNotFound
		}
		return model.OutboundIntent{}, err
	}
	return r.retryDelivery(ctx, item.ConversationID, item.SessionID, item.OutboundIntent)
}

func (r *Runtime) retryDelivery(ctx context.Context, conversationID, sessionID string, intent model.OutboundIntent) (model.OutboundIntent, error) {
	if intent.Status != "terminal" {
		return model.OutboundIntent{}, ErrDeliveryNotRetryable
	}

	payload, err := json.Marshal(map[string]any{
		"intent_id":       intent.ID,
		"previous_status": intent.Status,
		"connector_id":    intent.ConnectorID,
		"chat_id":         intent.ChatID,
	})
	if err != nil {
		return model.OutboundIntent{}, fmt.Errorf("marshal delivery_redrive_requested payload: %w", err)
	}

	if err := r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          intent.RunID,
		Kind:           "delivery_redrive_requested",
		PayloadJSON:    payload,
	}); err != nil {
		if errors.Is(err, conversations.ErrDeliveryNotRetryable) {
			return model.OutboundIntent{}, ErrDeliveryNotRetryable
		}
		return model.OutboundIntent{}, fmt.Errorf("journal delivery_redrive_requested: %w", err)
	}

	svc := sessions.NewService(r.store, r.convStore)
	intent, err = svc.LoadSessionOutboundIntent(ctx, sessionID, intent.ID)
	if err != nil {
		if errors.Is(err, sessions.ErrOutboundIntentNotFound) {
			return model.OutboundIntent{}, ErrDeliveryNotFound
		}
		return model.OutboundIntent{}, err
	}
	return intent, nil
}
