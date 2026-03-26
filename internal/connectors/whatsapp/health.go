package whatsapp

import (
	"sync"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

type HealthState struct {
	now               func() time.Time
	webhookStaleAfter time.Duration

	mu                  sync.Mutex
	lastWebhook         time.Time
	lastDeliverySuccess time.Time
	lastFailureAt       time.Time
	lastFailureSummary  string
}

func NewHealthState(now func() time.Time) *HealthState {
	if now == nil {
		now = time.Now
	}
	return &HealthState{
		now:               func() time.Time { return now().UTC() },
		webhookStaleAfter: 30 * time.Minute,
	}
}

func (h *HealthState) markWebhook(at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastWebhook = at.UTC()
}

func (h *HealthState) markDeliverySuccess(at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastDeliverySuccess = at.UTC()
}

func (h *HealthState) markFailure(at time.Time, summary string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastFailureAt = at.UTC()
	h.lastFailureSummary = summary
}

func (h *HealthState) snapshot() model.ConnectorHealthSnapshot {
	now := h.now()

	h.mu.Lock()
	defer h.mu.Unlock()

	snapshot := model.ConnectorHealthSnapshot{
		ConnectorID: "whatsapp",
		CheckedAt:   now,
	}

	latestSuccess := h.lastWebhook
	if h.lastDeliverySuccess.After(latestSuccess) {
		latestSuccess = h.lastDeliverySuccess
	}

	switch {
	case !h.lastFailureAt.IsZero() && h.lastFailureAt.After(latestSuccess):
		snapshot.State = model.ConnectorHealthDegraded
		snapshot.Summary = h.lastFailureSummary
	case !h.lastWebhook.IsZero() && now.Sub(h.lastWebhook) > h.webhookStaleAfter:
		snapshot.State = model.ConnectorHealthDegraded
		snapshot.Summary = "webhook activity stale"
	case !h.lastWebhook.IsZero():
		snapshot.State = model.ConnectorHealthHealthy
		snapshot.Summary = "webhook activity recent"
	case !h.lastDeliverySuccess.IsZero():
		snapshot.State = model.ConnectorHealthHealthy
		snapshot.Summary = "outbound delivery healthy; awaiting webhook traffic"
	default:
		snapshot.State = model.ConnectorHealthUnknown
		snapshot.Summary = "awaiting webhook traffic"
	}

	return snapshot
}
