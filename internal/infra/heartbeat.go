package infra

import (
	"context"
)

// AgentHealthChecker is implemented by opencode.Service and claudecode.Service.
type AgentHealthChecker interface {
	Name() string
	IsAlive(ctx context.Context) bool
}

// AgentHealthResult is the result of a single health check.
type AgentHealthResult struct {
	Name  string
	Alive bool
}

// Heartbeat owns Tier 1 (Telegram liveness) and Tier 2 (agent health) checks.
// The check methods are called by the service loops in app.Run — Heartbeat itself
// does not own any goroutine or ticker.
type Heartbeat struct {
	notifier   Notifier
	checkers   []AgentHealthChecker
	operatorID int64
}

// NewHeartbeat creates a Heartbeat. notifier may be nil.
func NewHeartbeat(notifier Notifier, checkers []AgentHealthChecker, operatorID int64) *Heartbeat {
	return &Heartbeat{
		notifier:   notifier,
		checkers:   checkers,
		operatorID: operatorID,
	}
}

// CheckAgents runs IsAlive on all registered checkers and returns results.
// This is called by the Tier 2 heartbeat loop in app.Run.
func (h *Heartbeat) CheckAgents(ctx context.Context) []AgentHealthResult {
	results := make([]AgentHealthResult, len(h.checkers))
	for i, c := range h.checkers {
		results[i] = AgentHealthResult{Name: c.Name(), Alive: c.IsAlive(ctx)}
	}
	return results
}
