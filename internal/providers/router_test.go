package providers_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/providers"
)

// routerMockProvider is a test double for LLMProvider used in router tests.
type routerMockProvider struct {
	name  string
	resp  *providers.LLMResponse
	err   error
	calls atomic.Int32
}

func (m *routerMockProvider) Chat(_ context.Context, _ []providers.Message, _ []providers.Tool) (*providers.LLMResponse, error) {
	m.calls.Add(1)
	return m.resp, m.err
}
func (m *routerMockProvider) Name() string { return m.name }

func TestRouter_SuccessOnFirst(t *testing.T) {
	p1 := &routerMockProvider{name: "p1", resp: &providers.LLMResponse{Content: "ok"}}
	p2 := &routerMockProvider{name: "p2", resp: &providers.LLMResponse{Content: "fallback"}}
	r := providers.NewProviderRouter([]providers.LLMProvider{p1, p2}, 5*time.Minute)
	resp, err := r.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("content: got %q, want %q", resp.Content, "ok")
	}
	if p2.calls.Load() != 0 {
		t.Error("p2 should not be called when p1 succeeds")
	}
}

func TestRouter_FallsBackOnRateLimit(t *testing.T) {
	p1 := &routerMockProvider{name: "p1", err: errors.New("429 rate limit exceeded")}
	p2 := &routerMockProvider{name: "p2", resp: &providers.LLMResponse{Content: "fallback"}}
	r := providers.NewProviderRouter([]providers.LLMProvider{p1, p2}, 5*time.Minute)
	resp, err := r.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "fallback" {
		t.Errorf("content: got %q, want fallback", resp.Content)
	}
}

func TestRouter_TerminalError_NoFallback(t *testing.T) {
	p1 := &routerMockProvider{name: "p1", err: errors.New("400 bad request")}
	p2 := &routerMockProvider{name: "p2", resp: &providers.LLMResponse{Content: "should not reach"}}
	r := providers.NewProviderRouter([]providers.LLMProvider{p1, p2}, 5*time.Minute)
	_, err := r.Chat(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error on terminal error")
	}
	if p2.calls.Load() != 0 {
		t.Error("p2 should not be called on terminal error")
	}
}

func TestRouter_CooldownSkipsProvider(t *testing.T) {
	p1 := &routerMockProvider{name: "p1", err: errors.New("429 rate limit")}
	p2 := &routerMockProvider{name: "p2", resp: &providers.LLMResponse{Content: "p2"}}
	r := providers.NewProviderRouter([]providers.LLMProvider{p1, p2}, 1*time.Hour)
	// First call: p1 rate-limited → falls back to p2, p1 goes on cooldown.
	_, _ = r.Chat(context.Background(), nil, nil)
	p1.calls.Store(0)
	p2.calls.Store(0)
	// Second call: p1 still on cooldown → goes straight to p2.
	resp, err := r.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p1.calls.Load() != 0 {
		t.Error("p1 should be skipped (on cooldown)")
	}
	if resp.Content != "p2" {
		t.Errorf("content: got %q, want p2", resp.Content)
	}
}

func TestRouter_ContextCanceled_ReturnsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	p1 := &routerMockProvider{name: "p1", err: context.Canceled}
	p2 := &routerMockProvider{name: "p2", resp: &providers.LLMResponse{Content: "should not reach"}}
	r := providers.NewProviderRouter([]providers.LLMProvider{p1, p2}, 5*time.Minute)
	_, err := r.Chat(ctx, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if p2.calls.Load() != 0 {
		t.Error("p2 should not be called when context is cancelled")
	}
}

func TestRouter_AllExhausted_ReturnsLastError(t *testing.T) {
	p1 := &routerMockProvider{name: "p1", err: errors.New("429 rate limit")}
	p2 := &routerMockProvider{name: "p2", err: errors.New("503 service unavailable")}
	r := providers.NewProviderRouter([]providers.LLMProvider{p1, p2}, 5*time.Minute)
	_, err := r.Chat(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestRouter_Name(t *testing.T) {
	p1 := &routerMockProvider{name: "copilot"}
	p2 := &routerMockProvider{name: "openai-key"}
	r := providers.NewProviderRouter([]providers.LLMProvider{p1, p2}, 5*time.Minute)
	name := r.Name()
	want := "router(copilot→openai-key)"
	if name != want {
		t.Errorf("Name(): got %q, want %q", name, want)
	}
}
