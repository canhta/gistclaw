package telegram

import (
	"sync"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

type healthState struct {
	now        func() time.Time
	staleAfter time.Duration

	mu                 sync.Mutex
	lastPollSuccess    time.Time
	lastDrainSuccess   time.Time
	lastFailureAt      time.Time
	lastFailureSummary string
}

func newHealthState(now func() time.Time) *healthState {
	if now == nil {
		now = time.Now
	}
	return &healthState{
		now:        func() time.Time { return now().UTC() },
		staleAfter: 5 * time.Minute,
	}
}

func (h *healthState) markPollSuccess(at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastPollSuccess = at.UTC()
}

func (h *healthState) markDrainSuccess(at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastDrainSuccess = at.UTC()
}

func (h *healthState) markFailure(at time.Time, summary string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastFailureAt = at.UTC()
	h.lastFailureSummary = summary
}

func (h *healthState) snapshot() model.ConnectorHealthSnapshot {
	now := h.now()

	h.mu.Lock()
	defer h.mu.Unlock()

	snapshot := model.ConnectorHealthSnapshot{
		ConnectorID: "telegram",
		CheckedAt:   now,
	}

	latestSuccess := h.lastPollSuccess
	if h.lastDrainSuccess.After(latestSuccess) {
		latestSuccess = h.lastDrainSuccess
	}

	switch {
	case !h.lastFailureAt.IsZero() && h.lastFailureAt.After(latestSuccess):
		snapshot.State = model.ConnectorHealthDegraded
		snapshot.Summary = h.lastFailureSummary
		snapshot.RestartSuggested = true
	case h.lastPollSuccess.IsZero():
		snapshot.State = model.ConnectorHealthDegraded
		snapshot.Summary = "no successful poll yet"
		snapshot.RestartSuggested = true
	case now.Sub(h.lastPollSuccess) > h.staleAfter:
		snapshot.State = model.ConnectorHealthDegraded
		snapshot.Summary = "poll loop stale"
		snapshot.RestartSuggested = true
	default:
		snapshot.State = model.ConnectorHealthHealthy
		snapshot.Summary = "poll loop healthy"
	}

	return snapshot
}
