package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/sessions"
)

var ErrDeliveryNotFound = fmt.Errorf("runtime: delivery not found")
var ErrDeliveryNotRetryable = fmt.Errorf("runtime: delivery not retryable")
var ErrRouteNotFound = fmt.Errorf("runtime: route not found")
var ErrRouteNotActive = fmt.Errorf("runtime: route not active")

type BindRouteCommand struct {
	SessionID   string
	ThreadID    string
	ConnectorID string
	AccountID   string
	ExternalID  string
}

func (r *Runtime) ListSessions(ctx context.Context, conversationID string, limit int) ([]model.Session, error) {
	return sessions.NewService(r.store, r.convStore).ListConversationSessions(ctx, conversationID, limit)
}

func (r *Runtime) ListAllSessions(ctx context.Context, filter sessions.SessionListFilter) ([]model.Session, error) {
	return sessions.NewService(r.store, r.convStore).ListSessions(ctx, filter)
}

func (r *Runtime) ListAllSessionsPage(ctx context.Context, filter sessions.SessionListFilter) (sessions.PageResult[model.Session], error) {
	return sessions.NewService(r.store, r.convStore).ListSessionsPage(ctx, filter)
}

func (r *Runtime) ListRoutes(ctx context.Context, filter sessions.RouteListFilter) ([]model.RouteDirectoryItem, error) {
	return sessions.NewService(r.store, r.convStore).ListRoutes(ctx, filter)
}

func (r *Runtime) ListRoutesPage(ctx context.Context, filter sessions.RouteListFilter) (sessions.PageResult[model.RouteDirectoryItem], error) {
	return sessions.NewService(r.store, r.convStore).ListRoutesPage(ctx, filter)
}

func (r *Runtime) SessionHistory(ctx context.Context, sessionID string, limit int) (model.Session, []model.SessionMessage, error) {
	return sessions.NewService(r.store, r.convStore).LoadSessionMailbox(ctx, sessionID, limit)
}

func (r *Runtime) ConnectorDeliveryHealth(ctx context.Context) ([]model.ConnectorDeliveryHealth, error) {
	return sessions.NewService(r.store, r.convStore).ListConnectorDeliveryHealth(ctx)
}

func (r *Runtime) ListDeliveries(ctx context.Context, filter sessions.DeliveryQueueFilter) ([]model.DeliveryQueueItem, error) {
	return sessions.NewService(r.store, r.convStore).ListDeliveryQueue(ctx, filter)
}

func (r *Runtime) ListDeliveriesPage(ctx context.Context, filter sessions.DeliveryQueueFilter) (sessions.PageResult[model.DeliveryQueueItem], error) {
	return sessions.NewService(r.store, r.convStore).ListDeliveryQueuePage(ctx, filter)
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

func (r *Runtime) DeactivateRoute(ctx context.Context, routeID string) (model.RouteDirectoryItem, error) {
	svc := sessions.NewService(r.store, r.convStore)
	route, err := svc.LoadRoute(ctx, routeID)
	if err != nil {
		if errors.Is(err, sessions.ErrSessionRouteNotFound) {
			return model.RouteDirectoryItem{}, ErrRouteNotFound
		}
		return model.RouteDirectoryItem{}, err
	}
	if route.Status != "active" {
		return model.RouteDirectoryItem{}, ErrRouteNotActive
	}

	payload, err := json.Marshal(map[string]any{
		"route_id": route.ID,
	})
	if err != nil {
		return model.RouteDirectoryItem{}, fmt.Errorf("marshal session_unbound payload: %w", err)
	}

	if err := r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: route.ConversationID,
		Kind:           "session_unbound",
		PayloadJSON:    payload,
	}); err != nil {
		return model.RouteDirectoryItem{}, fmt.Errorf("journal session_unbound: %w", err)
	}

	return svc.LoadRoute(ctx, route.ID)
}

func (r *Runtime) BindRoute(ctx context.Context, cmd BindRouteCommand) (model.RouteDirectoryItem, error) {
	svc := sessions.NewService(r.store, r.convStore)
	session, err := svc.LoadSession(ctx, cmd.SessionID)
	if err != nil {
		return model.RouteDirectoryItem{}, err
	}

	evt, err := newSessionBoundEvent(
		session.ConversationID,
		"",
		conversations.ConversationKey{
			ConnectorID: cmd.ConnectorID,
			AccountID:   cmd.AccountID,
			ExternalID:  cmd.ExternalID,
			ThreadID:    cmd.ThreadID,
		},
		session.ID,
		time.Now().UTC(),
	)
	if err != nil {
		return model.RouteDirectoryItem{}, err
	}
	if err := r.convStore.AppendEvent(ctx, evt); err != nil {
		return model.RouteDirectoryItem{}, fmt.Errorf("journal session_bound: %w", err)
	}
	return svc.LoadRoute(ctx, evt.ID)
}

func (r *Runtime) SendRoute(ctx context.Context, routeID, fromSessionID, body string) (model.Run, error) {
	svc := sessions.NewService(r.store, r.convStore)
	route, err := svc.LoadRoute(ctx, routeID)
	if err != nil {
		if errors.Is(err, sessions.ErrSessionRouteNotFound) {
			return model.Run{}, ErrRouteNotFound
		}
		return model.Run{}, err
	}
	if route.Status != "active" {
		return model.Run{}, ErrRouteNotActive
	}
	return r.SendSession(ctx, SendSessionCommand{
		FromSessionID: fromSessionID,
		ToSessionID:   route.SessionID,
		Body:          body,
	})
}

func (r *Runtime) SendRouteAsync(ctx context.Context, routeID, fromSessionID, body string) (model.Run, error) {
	svc := sessions.NewService(r.store, r.convStore)
	route, err := svc.LoadRoute(ctx, routeID)
	if err != nil {
		if errors.Is(err, sessions.ErrSessionRouteNotFound) {
			return model.Run{}, ErrRouteNotFound
		}
		return model.Run{}, err
	}
	if route.Status != "active" {
		return model.Run{}, ErrRouteNotActive
	}
	return r.SendSessionAsync(ctx, SendSessionCommand{
		FromSessionID: fromSessionID,
		ToSessionID:   route.SessionID,
		Body:          body,
	})
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
