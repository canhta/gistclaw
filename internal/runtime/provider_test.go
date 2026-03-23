package runtime

import (
	"context"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestMockProvider_ReturnsConfiguredResponses(t *testing.T) {
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "response 1", InputTokens: 10, OutputTokens: 20},
			{Content: "response 2", InputTokens: 15, OutputTokens: 25},
		},
		nil,
	)

	ctx := context.Background()
	req := GenerateRequest{Instructions: "test"}

	r1, err := prov.Generate(ctx, req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r1.Content != "response 1" {
		t.Fatalf("expected %q, got %q", "response 1", r1.Content)
	}

	r2, err := prov.Generate(ctx, req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r2.Content != "response 2" {
		t.Fatalf("expected %q, got %q", "response 2", r2.Content)
	}

	r3, err := prov.Generate(ctx, req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r3.Content != "mock response" {
		t.Fatalf("expected %q, got %q", "mock response", r3.Content)
	}
}

func TestMockProvider_WrapsErrorAsProviderError(t *testing.T) {
	prov := NewMockProvider(
		nil,
		[]error{
			&model.ProviderError{Code: model.ErrRateLimit, Message: "slow down", Retryable: true},
		},
	)

	_, err := prov.Generate(context.Background(), GenerateRequest{Instructions: "test"}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	provErr, ok := err.(*model.ProviderError)
	if !ok {
		t.Fatalf("expected *model.ProviderError, got %T", err)
	}
	if provErr.Code != model.ErrRateLimit {
		t.Fatalf("expected ErrRateLimit, got %s", provErr.Code)
	}
}

func TestProvider_AllFiveErrorCodesHandled(t *testing.T) {
	codes := []model.ProviderErrorCode{
		model.ErrRateLimit,
		model.ErrContextWindowExceeded,
		model.ErrModelRefusal,
		model.ErrProviderTimeout,
		model.ErrMalformedResponse,
	}

	for _, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			prov := NewMockProvider(
				nil,
				[]error{
					&model.ProviderError{
						Code:      code,
						Message:   "test " + string(code),
						Retryable: code == model.ErrRateLimit || code == model.ErrProviderTimeout,
					},
				},
			)

			_, err := prov.Generate(context.Background(), GenerateRequest{}, nil)
			if err == nil {
				t.Fatal("expected error")
			}

			provErr, ok := err.(*model.ProviderError)
			if !ok {
				t.Fatalf("expected *model.ProviderError, got %T", err)
			}
			if provErr.Code != code {
				t.Fatalf("expected code %s, got %s", code, provErr.Code)
			}
		})
	}
}

func TestMockProvider_ID(t *testing.T) {
	prov := NewMockProvider(nil, nil)
	if prov.ID() != "mock" {
		t.Fatalf("expected ID %q, got %q", "mock", prov.ID())
	}
}

func TestMockProvider_CallCount(t *testing.T) {
	prov := NewMockProvider(nil, nil)

	for i := 0; i < 3; i++ {
		_, _ = prov.Generate(context.Background(), GenerateRequest{}, nil)
	}
	if prov.CallCount() != 3 {
		t.Fatalf("expected 3 calls, got %d", prov.CallCount())
	}
}
