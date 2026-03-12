// internal/infra/cost_test.go
package infra_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/canhta/gistclaw/internal/infra"
	"github.com/canhta/gistclaw/internal/store"
)

func newTestCostGuard(t *testing.T, limitUSD float64) *infra.CostGuard {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return infra.NewCostGuard(s, limitUSD, nil) // nil notifier for unit tests
}

func TestCostGuardTrackBelowThreshold(t *testing.T) {
	g := newTestCostGuard(t, 10.0)
	ctx := context.Background()
	// Track 50% of limit — no notification expected.
	if err := g.Track(ctx, 5.0); err != nil {
		t.Fatalf("Track: %v", err)
	}
}

func TestCostGuardCurrentTotal(t *testing.T) {
	g := newTestCostGuard(t, 10.0)
	ctx := context.Background()
	_ = g.Track(ctx, 3.0)
	_ = g.Track(ctx, 2.0)
	if got := g.CurrentUSD(); got != 5.0 {
		t.Errorf("CurrentUSD: got %v, want 5.0", got)
	}
}

func TestCostGuardConcurrentTrack(t *testing.T) {
	g := newTestCostGuard(t, 100.0)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = g.Track(ctx, 1.0)
		}()
	}
	wg.Wait()
	if got := g.CurrentUSD(); got != 10.0 {
		t.Errorf("concurrent CurrentUSD: got %v, want 10.0", got)
	}
}

func TestCostGuardZeroIsNoOp(t *testing.T) {
	g := newTestCostGuard(t, 10.0)
	ctx := context.Background()
	// Providers with opaque billing return 0; Track(0) must not panic or error.
	if err := g.Track(ctx, 0); err != nil {
		t.Fatalf("Track(0): %v", err)
	}
	if got := g.CurrentUSD(); got != 0 {
		t.Errorf("after Track(0): CurrentUSD = %v, want 0", got)
	}
}
