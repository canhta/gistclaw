// internal/providers/tracking.go
package providers

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/infra"
)

// trackingProvider wraps any LLMProvider and calls CostGuard.Track on every
// successful Chat response. Errors from the inner provider are passed through
// unchanged; CostGuard.Track is NOT called on error.
type trackingProvider struct {
	inner LLMProvider
	guard *infra.CostGuard
}

// NewTrackingProvider returns an LLMProvider that delegates to inner and
// automatically tracks cost after each successful Chat call.
// app.NewApp wires this once:
//
//	rawLLM    := providers.New(cfg, store)
//	trackedLLM := providers.NewTrackingProvider(rawLLM, costGuard)
func NewTrackingProvider(inner LLMProvider, guard *infra.CostGuard) LLMProvider {
	return &trackingProvider{inner: inner, guard: guard}
}

// Chat delegates to the inner provider, then calls guard.Track with the
// cost from the response Usage. If tracking fails, the error is logged
// but not returned to the caller — the response was already obtained.
func (t *trackingProvider) Chat(ctx context.Context, messages []Message, tools []Tool) (*LLMResponse, error) {
	resp, err := t.inner.Chat(ctx, messages, tools)
	if err != nil {
		return nil, err
	}
	if trackErr := t.guard.Track(ctx, resp.Usage.TotalCostUSD); trackErr != nil {
		log.Error().
			Err(trackErr).
			Float64("cost_usd", resp.Usage.TotalCostUSD).
			Str("provider", t.inner.Name()).
			Msg("providers/tracking: failed to track cost")
	}
	return resp, nil
}

// Name delegates to the inner provider.
func (t *trackingProvider) Name() string { return t.inner.Name() }
