// internal/gateway/errors_test.go
package gateway

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestClassifyError_Terminal(t *testing.T) {
	terminal := []error{
		errors.New("400 Bad Request"),
		errors.New("401 Unauthorized"),
		errors.New("403 Forbidden"),
		errors.New("invalid model"),
		errors.New("openai: chat completions: 400 Bad Request"),
	}
	for _, err := range terminal {
		if got := classifyError(err); got != errKindTerminal {
			t.Errorf("classifyError(%q) = %v; want errKindTerminal", err, got)
		}
	}
}

func TestClassifyError_RateLimit(t *testing.T) {
	rateLimited := []error{
		errors.New("429 Too Many Requests"),
		errors.New("openai: chat completions: 429 Too Many Requests"),
		errors.New("rate limit exceeded"),
		errors.New("Rate Limit Exceeded"),
		fmt.Errorf("wrapped: %w", errors.New("too many requests")),
	}
	for _, err := range rateLimited {
		if got := classifyError(err); got != errKindRateLimit {
			t.Errorf("classifyError(%q) = %v; want errKindRateLimit", err, got)
		}
	}
}

func TestClassifyError_Retryable(t *testing.T) {
	retryable := []error{
		errors.New("500 Internal Server Error"),
		errors.New("502 Bad Gateway"),
		errors.New("503 Service Unavailable"),
		errors.New("504 Gateway Timeout"),
		errors.New("openai: chat completions: 503 Service Unavailable"),
		errors.New("request timeout"),
		errors.New("deadline exceeded"),
		context.DeadlineExceeded,
		context.Canceled,
	}
	for _, err := range retryable {
		if got := classifyError(err); got != errKindRetryable {
			t.Errorf("classifyError(%q) = %v; want errKindRetryable", err, got)
		}
	}
}

func TestClassifyError_Nil(t *testing.T) {
	if got := classifyError(nil); got != errKindTerminal {
		t.Errorf("classifyError(nil) = %v; want errKindTerminal", got)
	}
}
