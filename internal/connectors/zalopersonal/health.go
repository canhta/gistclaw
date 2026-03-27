package zalopersonal

import (
	"sync"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

type HealthState struct {
	mu       sync.RWMutex
	snapshot model.ConnectorHealthSnapshot
}

func NewHealthState() *HealthState {
	return &HealthState{
		snapshot: model.ConnectorHealthSnapshot{
			ConnectorID: "zalo_personal",
			State:       model.ConnectorHealthUnknown,
			Summary:     "awaiting first authentication",
		},
	}
}

func (h *HealthState) snapshotCopy() model.ConnectorHealthSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.snapshot
}

func (h *HealthState) markUnknown(summary string) {
	h.set(model.ConnectorHealthUnknown, summary)
}

func (h *HealthState) markUnauthenticated() {
	h.set(model.ConnectorHealthDegraded, "not authenticated")
}

func (h *HealthState) markConnected() {
	h.set(model.ConnectorHealthHealthy, "connected")
}

func (h *HealthState) markDisconnected(summary string) {
	h.set(model.ConnectorHealthDegraded, summary)
}

func (h *HealthState) set(state model.ConnectorHealthState, summary string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.snapshot.ConnectorID = "zalo_personal"
	h.snapshot.State = state
	h.snapshot.Summary = summary
	h.snapshot.CheckedAt = time.Now().UTC()
	h.snapshot.RestartSuggested = state == model.ConnectorHealthDegraded
}
