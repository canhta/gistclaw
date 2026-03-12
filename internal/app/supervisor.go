package app

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/rs/zerolog/log"
)

// PermanentFailure is returned by WithRestart when a service exhausts its restart budget.
// The caller (App.Run) decides what to do: cancel root context (critical services) or
// notify the operator and continue (non-critical services).
type PermanentFailure struct {
	Name string
	Err  error
}

func (e PermanentFailure) Error() string {
	return fmt.Sprintf("service %s permanently failed: %v", e.Name, e.Err)
}

func (e PermanentFailure) Unwrap() error { return e.Err }

// WithRestart wraps fn in a restart loop with exponential backoff.
//
// maxAttempts=0 means unlimited restarts (use for gateway.Service).
//
// Return contract:
//   - nil            — clean shutdown (ctx cancelled) or clean fn exit (fn returned nil)
//   - PermanentFailure — fn failed maxAttempts times within window
//   - other error    — BUG: fn returned an unexpected non-nil, non-context error after unlimited retries
func WithRestart(name string, maxAttempts int, window time.Duration, fn func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		attempts := 0
		windowStart := time.Now()
		var lastErr error

		for {
			err := fn(ctx)

			// Clean shutdown — context cancelled while fn was running.
			if ctx.Err() != nil {
				return nil
			}

			// Clean exit — fn finished without error.
			if err == nil {
				return nil
			}

			lastErr = err

			// Reset window counter if the window has elapsed.
			now := time.Now()
			if window > 0 && now.Sub(windowStart) > window {
				attempts = 0
				windowStart = now
			}
			attempts++

			if maxAttempts > 0 && attempts >= maxAttempts {
				return PermanentFailure{Name: name, Err: lastErr}
			}

			log.Warn().
				Str("service", name).
				Err(err).
				Int("attempt", attempts).
				Msg("service crashed, restarting")

			time.Sleep(restartDelay(attempts))
		}
	}
}

// restartDelay returns exponential backoff capped at 30 seconds.
func restartDelay(attempt int) time.Duration {
	const maxDelay = 30 * time.Second
	delay := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}
