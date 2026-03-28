package presence

import (
	"context"
	"sync"
)

type Route struct {
	ConversationID string
	ConnectorID    string
	AccountID      string
	ExternalID     string
}

type Manager struct {
	mu          sync.Mutex
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
	m.mu.Lock()
	defer m.mu.Unlock()
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
	m.mu.Lock()
	ctrl, ok := m.controllers[key]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.controllers, key)
	m.mu.Unlock()
	ctrl.Stop()
}

func (m *Manager) MarkOutputStarted(route Route) {
	m.withController(route, func(ctrl *Controller) {
		ctrl.MarkOutputStarted()
	})
}

func (m *Manager) MarkPaused(route Route) {
	m.withController(route, func(ctrl *Controller) {
		ctrl.MarkPaused()
	})
}

func (r Route) key() string {
	return r.ConversationID + "|" + r.ConnectorID + "|" + r.AccountID + "|" + r.ExternalID
}

func (m *Manager) withController(route Route, fn func(*Controller)) {
	if m == nil || fn == nil {
		return
	}
	key := route.key()
	m.mu.Lock()
	ctrl, ok := m.controllers[key]
	m.mu.Unlock()
	if !ok {
		return
	}
	fn(ctrl)
}
