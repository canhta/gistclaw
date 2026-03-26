package whatsapp

import (
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

func TestHealthState_MarksMissingWebhookActivityAsDegraded(t *testing.T) {
	now := time.Date(2026, 3, 26, 21, 0, 0, 0, time.UTC)
	state := NewHealthState(func() time.Time { return now })
	state.markWebhook(now.Add(-45 * time.Minute))

	snapshot := state.snapshot()
	if snapshot.State != model.ConnectorHealthDegraded {
		t.Fatalf("expected degraded snapshot, got %#v", snapshot)
	}
	if snapshot.RestartSuggested {
		t.Fatalf("did not expect restart suggestion, got %#v", snapshot)
	}
}

func TestHealthState_RecentDeliveryIsHealthyWithoutWebhookTraffic(t *testing.T) {
	now := time.Date(2026, 3, 26, 21, 0, 0, 0, time.UTC)
	state := NewHealthState(func() time.Time { return now })
	state.markDeliverySuccess(now.Add(-time.Minute))

	snapshot := state.snapshot()
	if snapshot.State != model.ConnectorHealthHealthy {
		t.Fatalf("expected healthy snapshot, got %#v", snapshot)
	}
	if snapshot.Summary == "" {
		t.Fatalf("expected health summary, got %#v", snapshot)
	}
}
