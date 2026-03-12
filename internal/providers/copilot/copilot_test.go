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

func TestCopilotName(t *testing.T) {
	p := copilotprovider.New("localhost:4321")
	if p.Name() != "copilot" {
		t.Errorf("Name() = %q, want %q", p.Name(), "copilot")
	}
}

// TestCopilotChatNoBridge verifies that Chat returns a descriptive error when
// no Copilot bridge is listening at the given address. This is the expected
// behaviour in unit-test environments.
func TestCopilotChatNoBridge(t *testing.T) {
	// Use a port that is (almost certainly) not listening in CI.
	p := copilotprovider.New("localhost:19321")
	_, err := p.Chat(context.Background(), []providers.Message{
		{Role: "user", Content: "hello"},
	}, nil)
	if err == nil {
		t.Fatal("expected error when bridge is not running, got nil")
	}
	// The error must mention "copilot" and something about connection/bridge.
	msg := err.Error()
	if !strings.Contains(msg, "copilot") {
		t.Errorf("error %q should contain %q", msg, "copilot")
	}
}

// TestCopilotChatDoesNotPanic verifies that Chat never panics,
// even with nil inputs and no bridge.
func TestCopilotChatDoesNotPanic(t *testing.T) {
	p := copilotprovider.New("localhost:19321")
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Chat panicked: %v", r)
		}
	}()
	p.Chat(context.Background(), nil, nil) //nolint:errcheck
}
