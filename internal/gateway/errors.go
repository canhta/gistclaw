// internal/gateway/errors.go
package gateway

import (
	"context"
	"errors"
	"strings"
)

// errKind classifies an LLM provider error into a retry strategy.
type errKind int

const (
	// errKindTerminal means fail fast — don't retry (4xx other than 429, format errors).
	errKindTerminal errKind = iota
	// errKindRetryable means retry with exponential backoff (5xx, timeout).
	errKindRetryable
	// errKindRateLimit means retry with backoff and notify the user (429).
	errKindRateLimit
)

// classifyError maps an LLM provider error to a retry strategy.
// Uses string matching because each provider wraps HTTP errors differently;
// this keeps classification provider-agnostic without requiring type assertions.
func classifyError(err error) errKind {
	if err == nil {
		return errKindTerminal
	}

	// context.DeadlineExceeded / context.Canceled → retryable timeout
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return errKindRetryable
	}

	msg := err.Error()
	lower := strings.ToLower(msg)

	// 429 rate limit
	if strings.Contains(msg, "429") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "too many requests") {
		return errKindRateLimit
	}

	// 5xx server errors
	for _, code := range []string{"500", "502", "503", "504"} {
		if strings.Contains(msg, code) {
			return errKindRetryable
		}
	}

	// Timeout / deadline keywords (some SDKs surface these in message text)
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline") {
		return errKindRetryable
	}

	return errKindTerminal
}
