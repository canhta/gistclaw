// internal/providers/tracking_test.go
package providers_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/canhta/gistclaw/internal/infra"
	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/store"
)

func newTestGuard(t *testing.T, limitUSD float64) *infra.CostGuard {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return infra.NewCostGuard(s, limitUSD, nil)
}

// fixedProvider always returns a response with the given cost.
type fixedProvider struct {
	costUSD  float64
	content  string
	callsErr bool
}

func (f *fixedProvider) Name() string { return "fixed" }
func (f *fixedProvider) Chat(_ context.Context, _ []providers.Message, _ []providers.Tool) (*providers.LLMResponse, error) {
	if f.callsErr {
		return nil, errors.New("provider error")
	}
	return &providers.LLMResponse{
		Content: f.content,
		Usage:   providers.Usage{PromptTokens: 10, CompletionTokens: 5, TotalCostUSD: f.costUSD},
	}, nil
}

func TestTrackingProviderNamePassthrough(t *testing.T) {
	guard := newTestGuard(t, 10.0)
	inner := &fixedProvider{costUSD: 0.01, content: "hi"}
	tracked := providers.NewTrackingProvider(inner, guard)
	if tracked.Name() != "fixed" {
		t.Errorf("Name() = %q, want %q", tracked.Name(), "fixed")
	}
}

func TestTrackingProviderTracksSuccessfulChat(t *testing.T) {
	guard := newTestGuard(t, 10.0)
	inner := &fixedProvider{costUSD: 0.05, content: "hello"}
	tracked := providers.NewTrackingProvider(inner, guard)

	ctx := context.Background()
	resp, err := tracked.Chat(ctx, []providers.Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("Content = %q, want %q", resp.Content, "hello")
	}
	if guard.CurrentUSD() != 0.05 {
		t.Errorf("CostGuard.CurrentUSD() = %v, want 0.05", guard.CurrentUSD())
	}
}

func TestTrackingProviderDoesNotTrackOnError(t *testing.T) {
	guard := newTestGuard(t, 10.0)
	inner := &fixedProvider{callsErr: true}
	tracked := providers.NewTrackingProvider(inner, guard)

	ctx := context.Background()
	_, err := tracked.Chat(ctx, nil, nil)
	if err == nil {
		t.Fatal("expected error from failing provider")
	}
	if guard.CurrentUSD() != 0 {
		t.Errorf("CostGuard.CurrentUSD() = %v after error, want 0", guard.CurrentUSD())
	}
}

func TestTrackingProviderAccumulatesMultipleCalls(t *testing.T) {
	guard := newTestGuard(t, 10.0)
	inner := &fixedProvider{costUSD: 0.10, content: "answer"}
	tracked := providers.NewTrackingProvider(inner, guard)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := tracked.Chat(ctx, nil, nil); err != nil {
			t.Fatalf("Chat %d error: %v", i, err)
		}
	}
	want := 0.30
	if got := guard.CurrentUSD(); got != want {
		t.Errorf("after 3 calls: CurrentUSD() = %v, want %v", got, want)
	}
}

func TestTrackingProviderZeroCostIsNoOp(t *testing.T) {
	// Providers with opaque billing return 0; CostGuard must treat this as no-op.
	guard := newTestGuard(t, 10.0)
	inner := &fixedProvider{costUSD: 0, content: "free reply"}
	tracked := providers.NewTrackingProvider(inner, guard)

	ctx := context.Background()
	if _, err := tracked.Chat(ctx, nil, nil); err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if guard.CurrentUSD() != 0 {
		t.Errorf("CostGuard.CurrentUSD() = %v after zero-cost chat, want 0", guard.CurrentUSD())
	}
}
