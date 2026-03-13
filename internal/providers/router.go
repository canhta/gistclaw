// internal/providers/router.go
package providers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ProviderRouter implements LLMProvider with ordered fallback and per-provider cooldown.
type ProviderRouter struct {
	chain  []LLMProvider
	window time.Duration

	mu        sync.Mutex
	cooldowns map[string]time.Time
}

// NewProviderRouter constructs a ProviderRouter.
// chain: ordered list of providers, tried in sequence; must be non-empty.
// cooldownWindow: how long to pause a provider after a rate-limit error.
func NewProviderRouter(chain []LLMProvider, cooldownWindow time.Duration) *ProviderRouter {
	return &ProviderRouter{
		chain:     chain,
		window:    cooldownWindow,
		cooldowns: make(map[string]time.Time),
	}
}

// Chat tries each provider in order, skipping providers on cooldown.
// Propagates context cancellation immediately.
// On rate-limit error, puts the provider on cooldown and tries the next.
// On terminal error, returns immediately without fallback.
// Returns the last error if all providers are exhausted.
func (p *ProviderRouter) Chat(ctx context.Context, msgs []Message, tools []Tool) (*LLMResponse, error) {
	var lastErr error
	for _, pr := range p.chain {
		if p.isOnCooldown(pr.Name()) {
			continue
		}
		resp, err := pr.Chat(ctx, msgs, tools)
		if err == nil {
			return resp, nil
		}
		// Propagate context cancellation immediately.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		lastErr = err
		switch ClassifyError(err) {
		case ErrKindTerminal:
			return nil, err
		case ErrKindRateLimit:
			p.setCooldown(pr.Name())
			// try next provider
		case ErrKindRetryable:
			// try next provider
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("router: all providers exhausted: %w", lastErr)
	}
	return nil, fmt.Errorf("router: no providers available")
}

// Name returns a human-readable description of the router's provider chain.
func (p *ProviderRouter) Name() string {
	names := make([]string, len(p.chain))
	for i, pr := range p.chain {
		names[i] = pr.Name()
	}
	return "router(" + strings.Join(names, "→") + ")"
}

func (p *ProviderRouter) isOnCooldown(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	expiry, ok := p.cooldowns[name]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(p.cooldowns, name)
		return false
	}
	return true
}

func (p *ProviderRouter) setCooldown(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cooldowns[name] = time.Now().Add(p.window)
}
