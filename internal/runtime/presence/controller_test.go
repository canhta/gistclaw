package presence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestController_DelaysInitialEmitAndKeepsAliveUntilOutput(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	emits := 0
	stops := 0

	ctrl := NewController(Options{
		StartupDelay:      20 * time.Millisecond,
		KeepaliveInterval: 10 * time.Millisecond,
		MaxDuration:       200 * time.Millisecond,
		StartFn: func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			emits++
			return nil
		},
		StopFn: func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			stops++
			return nil
		},
	})

	ctrl.Start(context.Background())

	time.Sleep(10 * time.Millisecond)
	mu.Lock()
	beforeDelay := emits
	mu.Unlock()
	if beforeDelay != 0 {
		t.Fatalf("expected no emits before startup delay, got %d", beforeDelay)
	}

	time.Sleep(35 * time.Millisecond)
	mu.Lock()
	afterStart := emits
	mu.Unlock()
	if afterStart < 2 {
		t.Fatalf("expected initial emit and keepalive, got %d emits", afterStart)
	}

	ctrl.MarkOutputStarted()
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	finalEmits := emits
	finalStops := stops
	mu.Unlock()
	if finalStops != 1 {
		t.Fatalf("expected one stop after output, got %d", finalStops)
	}

	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if emits != finalEmits {
		t.Fatalf("expected no more emits after stop, got %d then %d", finalEmits, emits)
	}
}

func TestController_TripsBreakerAfterConsecutiveFailures(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	attempts := 0
	ctrl := NewController(Options{
		StartupDelay:           5 * time.Millisecond,
		KeepaliveInterval:      5 * time.Millisecond,
		MaxDuration:            200 * time.Millisecond,
		MaxConsecutiveFailures: 2,
		StartFn: func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			attempts++
			return errors.New("gone")
		},
	})

	ctrl.Start(context.Background())
	time.Sleep(30 * time.Millisecond)

	mu.Lock()
	firstCount := attempts
	mu.Unlock()
	if firstCount != 2 {
		t.Fatalf("expected breaker to stop after 2 failed emits, got %d", firstCount)
	}

	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if attempts != firstCount {
		t.Fatalf("expected breaker to stop further emits, got %d then %d", firstCount, attempts)
	}
}
