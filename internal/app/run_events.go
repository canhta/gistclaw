package app

import (
	"context"
	"database/sql"
	"sync"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/sessions"
	"github.com/canhta/gistclaw/internal/store"
)

type runEventFanout struct {
	sinks []model.RunEventSink
}

func newRunEventFanout(sinks ...model.RunEventSink) *runEventFanout {
	filtered := make([]model.RunEventSink, 0, len(sinks))
	for _, sink := range sinks {
		if sink != nil {
			filtered = append(filtered, sink)
		}
	}
	return &runEventFanout{sinks: filtered}
}

func (f *runEventFanout) Emit(ctx context.Context, runID string, evt model.ReplayDelta) error {
	for _, sink := range f.sinks {
		if err := sink.Emit(ctx, runID, evt); err != nil {
			return err
		}
	}
	return nil
}

type connectorRouteNotifier struct {
	db         *store.DB
	mu         sync.RWMutex
	connectors map[string]model.Connector
}

func newConnectorRouteNotifier(db *store.DB) *connectorRouteNotifier {
	return &connectorRouteNotifier{
		db:         db,
		connectors: make(map[string]model.Connector),
	}
}

func (n *connectorRouteNotifier) SetConnectors(connectors []model.Connector) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.connectors = make(map[string]model.Connector, len(connectors))
	for _, connector := range connectors {
		if connector == nil {
			continue
		}
		n.connectors[connector.ID()] = connector
	}
}

func (n *connectorRouteNotifier) Emit(ctx context.Context, runID string, evt model.ReplayDelta) error {
	if n == nil || n.db == nil {
		return nil
	}
	if evt.Kind != "turn_delta" && evt.Kind != "turn_completed" && evt.Kind != "approval_requested" {
		return nil
	}

	route, err := n.loadRouteForEvent(ctx, runID, evt.Kind)
	if err == sessions.ErrSessionRouteNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	if route.ConnectorID == "" || route.ConnectorID == "web" || route.ExternalID == "" {
		return nil
	}

	n.mu.RLock()
	connector := n.connectors[route.ConnectorID]
	n.mu.RUnlock()
	if connector == nil {
		return nil
	}

	return connector.Notify(ctx, route.ExternalID, evt, "")
}

func (n *connectorRouteNotifier) loadRouteForEvent(ctx context.Context, runID, kind string) (model.SessionRoute, error) {
	sessionID, conversationID, err := n.loadRunRouteContext(ctx, runID)
	if err != nil {
		return model.SessionRoute{}, err
	}

	sessionSvc := sessions.NewService(n.db, nil)
	if sessionID != "" {
		route, err := sessionSvc.LoadRouteBySession(ctx, sessionID)
		if err == nil {
			return route, nil
		}
		if err != sessions.ErrSessionRouteNotFound {
			return model.SessionRoute{}, err
		}
	}
	if kind != "approval_requested" || conversationID == "" {
		return model.SessionRoute{}, sessions.ErrSessionRouteNotFound
	}

	var route model.SessionRoute
	err = n.db.RawDB().QueryRowContext(ctx,
		`SELECT bind.id, bind.session_id, bind.thread_id, bind.connector_id, bind.account_id, bind.external_id,
		        bind.status, bind.created_at
		 FROM session_bindings bind
		 JOIN sessions sess ON sess.id = bind.session_id
		 WHERE bind.conversation_id = ? AND bind.status = 'active'
		 ORDER BY CASE sess.role WHEN 'front' THEN 0 ELSE 1 END,
		          bind.created_at DESC,
		          bind.id DESC
		 LIMIT 1`,
		conversationID,
	).Scan(
		&route.ID,
		&route.SessionID,
		&route.ThreadID,
		&route.ConnectorID,
		&route.AccountID,
		&route.ExternalID,
		&route.Status,
		&route.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return model.SessionRoute{}, sessions.ErrSessionRouteNotFound
	}
	if err != nil {
		return model.SessionRoute{}, err
	}
	return route, nil
}

func (n *connectorRouteNotifier) loadRunRouteContext(ctx context.Context, runID string) (string, string, error) {
	var sessionID string
	var conversationID string
	err := n.db.RawDB().QueryRowContext(ctx,
		`SELECT COALESCE(session_id, ''), COALESCE(conversation_id, '')
		 FROM runs
		 WHERE id = ?`,
		runID,
	).Scan(&sessionID, &conversationID)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	return sessionID, conversationID, nil
}

var _ model.RunEventSink = (*runEventFanout)(nil)
var _ model.RunEventSink = (*connectorRouteNotifier)(nil)
