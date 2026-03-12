package app_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/canhta/gistclaw/internal/app"
)

var nopLogger = zerolog.Nop()

func TestWithRestartCleanShutdown(t *testing.T) {
	// fn returns nil → withRestart returns nil (clean exit)
	fn := func(ctx context.Context) error { return nil }
	wrapped := app.WithRestart(nopLogger, "svc", 3, time.Minute, fn)
	ctx := context.Background()
	if err := wrapped(ctx); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestWithRestartContextCancelled(t *testing.T) {
	// fn returns ctx.Err() → withRestart returns nil (clean shutdown)
	fn := func(ctx context.Context) error { return ctx.Err() }
	wrapped := app.WithRestart(nopLogger, "svc", 3, time.Minute, fn)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := wrapped(ctx); err != nil {
		t.Fatalf("expected nil on context cancel, got %v", err)
	}
}

func TestWithRestartPermanentFailure(t *testing.T) {
	// fn always fails → exhausts maxAttempts → returns PermanentFailure
	sentinel := errors.New("boom")
	fn := func(ctx context.Context) error { return sentinel }
	wrapped := app.WithRestart(nopLogger, "mysvc", 3, time.Minute, fn)
	err := wrapped(context.Background())
	var pf app.PermanentFailure
	if !errors.As(err, &pf) {
		t.Fatalf("expected PermanentFailure, got %T: %v", err, err)
	}
	if pf.Name != "mysvc" {
		t.Errorf("PermanentFailure.Name = %q, want %q", pf.Name, "mysvc")
	}
	if !errors.Is(pf.Err, sentinel) {
		t.Errorf("PermanentFailure.Err = %v, want %v", pf.Err, sentinel)
	}
}

func TestWithRestartRestartsOnTransientError(t *testing.T) {
	// fn fails twice then succeeds → withRestart returns nil
	var calls atomic.Int32
	fn := func(ctx context.Context) error {
		n := calls.Add(1)
		if n < 3 {
			return errors.New("transient")
		}
		return nil
	}
	wrapped := app.WithRestart(nopLogger, "svc", 5, time.Minute, fn)
	if err := wrapped(context.Background()); err != nil {
		t.Fatalf("expected nil after recovery, got %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", calls.Load())
	}
}

func TestWithRestartWindowReset(t *testing.T) {
	// maxAttempts=2 within a 10s window; fn always fails.
	// restartDelay(1)=1s, restartDelay(2) would be 2s but we never reach it.
	// Total elapsed before attempt 2: ~1s, well within the 10s window — counter
	// does NOT reset, so attempt 2 triggers PermanentFailure.
	// (A 50ms window would reset after each 1s sleep, causing an infinite loop.)
	fn := func(ctx context.Context) error { return errors.New("fail") }
	wrapped := app.WithRestart(nopLogger, "svc", 2, 10*time.Second, fn)
	err := wrapped(context.Background())
	var pf app.PermanentFailure
	if !errors.As(err, &pf) {
		t.Fatalf("expected PermanentFailure, got %v", err)
	}
}

func TestPermanentFailureError(t *testing.T) {
	pf := app.PermanentFailure{Name: "svc", Err: errors.New("root cause")}
	msg := pf.Error()
	if msg == "" {
		t.Error("PermanentFailure.Error() must not be empty")
	}
}
