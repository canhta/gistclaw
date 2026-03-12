// internal/providers/copilot/copilot_test.go
package copilot_test

import (
	"context"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/providers"
	copilotprovider "github.com/canhta/gistclaw/internal/providers/copilot"
)

// Compile-time check: Provider must satisfy LLMProvider.
var _ providers.LLMProvider = (*copilotprovider.Provider)(nil)

func TestCopilotStubName(t *testing.T) {
	p := copilotprovider.New("localhost:4321")
	if p.Name() != "copilot" {
		t.Errorf("Name() = %q, want %q", p.Name(), "copilot")
	}
}

func TestCopilotStubReturnsNotAvailableError(t *testing.T) {
	p := copilotprovider.New("localhost:4321")
	_, err := p.Chat(context.Background(), []providers.Message{
		{Role: "user", Content: "hello"},
	}, nil)
	if err == nil {
		t.Fatal("expected error from copilot stub, got nil")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("error %q should contain %q", err.Error(), "not available")
	}
}

func TestCopilotStubDoesNotPanic(t *testing.T) {
	// Calling Chat must not panic regardless of inputs.
	p := copilotprovider.New("localhost:4321")
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Chat panicked: %v", r)
		}
	}()
	p.Chat(context.Background(), nil, nil) //nolint:errcheck
}
