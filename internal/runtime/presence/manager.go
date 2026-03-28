package presence

import "context"

type Route struct {
	ConversationID string
	ConnectorID    string
	AccountID      string
	ExternalID     string
}

type Manager struct {
	controllers map[string]*Controller
}

func NewManager() *Manager {
	return &Manager{controllers: make(map[string]*Controller)}
}

func (m *Manager) Start(route Route, opts Options) *Controller {
	if m == nil {
		return nil
	}
	key := route.key()
	if key == "" {
		return nil
	}
	if ctrl, ok := m.controllers[key]; ok {
		return ctrl
	}
	ctrl := NewController(opts)
	m.controllers[key] = ctrl
	ctrl.Start(context.Background())
	return ctrl
}

func (m *Manager) Stop(route Route) {
	if m == nil {
		return
	}
	key := route.key()
	ctrl, ok := m.controllers[key]
	if !ok {
		return
	}
	delete(m.controllers, key)
	ctrl.Stop()
}

func (r Route) key() string {
	return r.ConversationID + "|" + r.ConnectorID + "|" + r.AccountID + "|" + r.ExternalID
}
