package telegram

import (
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

func TestHealthState_MarksStalePollLoopAsDegraded(t *testing.T) {
	now := time.Date(2026, 3, 26, 21, 0, 0, 0, time.UTC)
	state := newHealthState(func() time.Time { return now })
	state.markPollSuccess(now.Add(-10 * time.Minute))

	snapshot := state.snapshot()
	if snapshot.State != model.ConnectorHealthDegraded {
		t.Fatalf("expected degraded snapshot, got %#v", snapshot)
	}
	if !snapshot.RestartSuggested {
		t.Fatalf("expected restart suggestion, got %#v", snapshot)
	}
}

func TestHealthState_RecentPollIsHealthy(t *testing.T) {
	now := time.Date(2026, 3, 26, 21, 0, 0, 0, time.UTC)
	state := newHealthState(func() time.Time { return now })
	state.markPollSuccess(now.Add(-time.Minute))
	state.markDrainSuccess(now.Add(-30 * time.Second))

	snapshot := state.snapshot()
	if snapshot.State != model.ConnectorHealthHealthy {
		t.Fatalf("expected healthy snapshot, got %#v", snapshot)
	}
	if snapshot.RestartSuggested {
		t.Fatalf("did not expect restart suggestion, got %#v", snapshot)
	}
}

func TestHealthState_WaitsForFirstPollBeforeSuggestingRestart(t *testing.T) {
	now := time.Date(2026, 3, 26, 21, 0, 0, 0, time.UTC)
	state := newHealthState(func() time.Time { return now })

	snapshot := state.snapshot()
	if snapshot.State != model.ConnectorHealthDegraded {
		t.Fatalf("expected degraded snapshot, got %#v", snapshot)
	}
	if snapshot.Summary != "no successful poll yet" {
		t.Fatalf("expected startup summary, got %#v", snapshot)
	}
	if snapshot.RestartSuggested {
		t.Fatalf("did not expect restart suggestion before first poll, got %#v", snapshot)
	}
}
