// internal/infra/heartbeat.go
package infra

import (
	"context"
	"fmt"
	"sync/atomic"
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

// Pinger checks whether the upstream messaging platform is reachable.
// Implemented by channel/telegram.TelegramChannel (calls GetMe).
type Pinger interface {
	Ping(ctx context.Context) error
}

// Heartbeat owns Tier 1 (Telegram liveness) and Tier 2 (agent health) checks.
// The check methods are called by the service loops in app.Run — Heartbeat itself
// does not own any goroutine or ticker.
//
// Tier 1: CheckTelegram calls pinger.Ping; after 3 consecutive failures it
// sends a WARN to the operator. The failure counter resets on success.
//
// Tier 2: CheckAgents calls IsAlive on all registered checkers and returns the
// results for app.Run to act on (log + notify for dead agents).
type Heartbeat struct {
	notifier            Notifier
	checkers            []AgentHealthChecker
	operatorID          int64
	pinger              Pinger // may be nil; Tier 1 disabled when nil
	consecutiveFailures atomic.Int32
}

const tier1WarnThreshold = 3

// NewHeartbeat creates a Heartbeat. notifier may be nil.
// pinger may be nil (Tier 1 check is skipped).
func NewHeartbeat(notifier Notifier, checkers []AgentHealthChecker, operatorID int64) *Heartbeat {
	return &Heartbeat{
		notifier:   notifier,
		checkers:   checkers,
		operatorID: operatorID,
	}
}

// WithPinger attaches a Pinger for Tier 1 liveness checks. Returns h for chaining.
func (h *Heartbeat) WithPinger(p Pinger) *Heartbeat {
	h.pinger = p
	return h
}

// CheckTelegram performs a Tier 1 liveness check by calling pinger.Ping.
// Returns true if the ping succeeded. After tier1WarnThreshold consecutive
// failures, sends a WARN message to the operator (once per streak).
// No-op (returns true) when pinger is nil.
func (h *Heartbeat) CheckTelegram(ctx context.Context) bool {
	if h.pinger == nil {
		return true
	}
	if err := h.pinger.Ping(ctx); err != nil {
		n := h.consecutiveFailures.Add(1)
		if n == tier1WarnThreshold && h.notifier != nil && h.operatorID != 0 {
			msg := fmt.Sprintf("⚠️ Telegram API unreachable (%d consecutive failures): %v", n, err)
			_ = h.notifier.SendMessage(ctx, h.operatorID, msg)
		}
		return false
	}
	h.consecutiveFailures.Store(0)
	return true
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
