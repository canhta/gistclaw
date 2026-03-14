// internal/providers/errors.go
package providers

import (
	"context"
	"errors"
	"strings"
)

// ErrKind classifies an LLM provider error into a retry strategy.
type ErrKind int

const (
	// ErrKindTerminal means fail fast — don't retry (4xx except 429, format errors).
	ErrKindTerminal ErrKind = iota
	// ErrKindRetryable means retry with exponential backoff (5xx, timeout).
	ErrKindRetryable
	// ErrKindRateLimit means retry with backoff and notify the user (429).
	ErrKindRateLimit
	// ErrKindContextWindow means the context limit was exceeded — compress history and retry once.
	// Appended last so existing iota values are unchanged.
	ErrKindContextWindow
)

// ClassifyError maps an LLM provider error to a retry strategy.
// Uses string matching because each provider wraps HTTP errors differently;
// this keeps classification provider-agnostic without requiring type assertions.
func ClassifyError(err error) ErrKind {
	if err == nil {
		return ErrKindTerminal
	}

	// context.DeadlineExceeded / context.Canceled → retryable timeout
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrKindRetryable
	}

	msg := err.Error()
	lower := strings.ToLower(msg)

	// Context window exceeded — checked BEFORE the 5xx block so that error strings
	// like "500 context_length_exceeded" are classified as context-window, not retryable.
	if strings.Contains(lower, "context_length_exceeded") ||
		strings.Contains(lower, "maximum context length") ||
		strings.Contains(lower, "too many tokens") ||
		strings.Contains(lower, "reduce the length") ||
		strings.Contains(lower, "tokens in your prompt") {
		return ErrKindContextWindow
	}

	// 429 rate limit
	if strings.Contains(msg, "429") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "too many requests") {
		return ErrKindRateLimit
	}

	// 5xx server errors
	for _, code := range []string{"500", "502", "503", "504"} {
		if strings.Contains(msg, code) {
			return ErrKindRetryable
		}
	}

	// Timeout / deadline keywords (some SDKs surface these in message text)
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline") {
		return ErrKindRetryable
	}

	return ErrKindTerminal
}
