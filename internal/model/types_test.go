package model

import (
	"context"
	"testing"
)

func TestProviderError_ImplementsError(t *testing.T) {
	var err error = &ProviderError{
		Code:    ErrRateLimit,
		Message: "too many requests",
	}

	if got := err.Error(); got != "rate_limit: too many requests" {
		t.Fatalf("expected %q, got %q", "rate_limit: too many requests", got)
	}
}

func TestNoopEventSink_AlwaysSucceeds(t *testing.T) {
	sink := &NoopEventSink{}
	err := sink.Emit(context.Background(), "run-123", ReplayDelta{
		RunID: "run-123",
		Kind:  "test",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestAllProviderCodes_Defined(t *testing.T) {
	codes := []ProviderErrorCode{
		ErrRateLimit,
		ErrContextWindowExceeded,
		ErrModelRefusal,
		ErrProviderTimeout,
		ErrMalformedResponse,
	}

	seen := make(map[ProviderErrorCode]bool)
	for _, code := range codes {
		if code == "" {
			t.Fatal("provider error code must not be empty")
		}
		if seen[code] {
			t.Fatalf("duplicate provider error code: %s", code)
		}
		seen[code] = true
	}
	if len(seen) != 5 {
		t.Fatalf("expected 5 distinct codes, got %d", len(seen))
	}
}

func TestRunStatus_AllDefined(t *testing.T) {
	statuses := []RunStatus{
		RunStatusPending,
		RunStatusActive,
		RunStatusNeedsApproval,
		RunStatusCompleted,
		RunStatusInterrupted,
		RunStatusFailed,
	}

	for _, status := range statuses {
		if status == "" {
			t.Fatal("RunStatus must not be empty")
		}
	}
}

func TestNoopEventSink_ImplementsInterface(t *testing.T) {
	var _ RunEventSink = &NoopEventSink{}
}
