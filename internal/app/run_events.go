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
		meta := model.NormalizeConnectorMetadata(connector.Metadata())
		if meta.ID == "" {
			continue
		}
		n.connectors[meta.ID] = connector
	}
}

func (n *connectorRouteNotifier) Emit(ctx context.Context, runID string, evt model.ReplayDelta) error {
	if n == nil || n.db == nil {
		return nil
	}
	if evt.Kind != "turn_delta" && evt.Kind != "turn_completed" {
		return nil
	}

	route, err := n.loadRouteForEvent(ctx, runID, evt.Kind)
	if err == sessions.ErrSessionRouteNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	if route.ConnectorID == "" || route.ExternalID == "" {
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
	sessionID, _, err := n.loadRunRouteContext(ctx, runID)
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
	return model.SessionRoute{}, sessions.ErrSessionRouteNotFound
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
