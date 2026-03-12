package infra_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/canhta/gistclaw/internal/infra"
)

type mockHealthChecker struct {
	name  string
	alive bool
	calls atomic.Int32
}

func (m *mockHealthChecker) Name() string { return m.name }
func (m *mockHealthChecker) IsAlive(_ context.Context) bool {
	m.calls.Add(1)
	return m.alive
}

func TestNewHeartbeatNotNil(t *testing.T) {
	hb := infra.NewHeartbeat(nil, nil, 0)
	if hb == nil {
		t.Fatal("NewHeartbeat returned nil")
	}
}

func TestHeartbeatAgentHealthCheckerInterface(t *testing.T) {
	// Verify mockHealthChecker satisfies AgentHealthChecker.
	var _ infra.AgentHealthChecker = &mockHealthChecker{}
}

func TestHeartbeatCheckAgents(t *testing.T) {
	alive := &mockHealthChecker{name: "opencode", alive: true}
	dead := &mockHealthChecker{name: "claudecode", alive: false}

	hb := infra.NewHeartbeat(nil, []infra.AgentHealthChecker{alive, dead}, 0)

	results := hb.CheckAgents(context.Background())
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		switch r.Name {
		case "opencode":
			if !r.Alive {
				t.Error("opencode should be alive")
			}
		case "claudecode":
			if r.Alive {
				t.Error("claudecode should be dead")
			}
		}
	}
}
