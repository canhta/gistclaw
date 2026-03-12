// internal/providers/llm_test.go
package providers_test

import (
	"context"
	"testing"

	"github.com/canhta/gistclaw/internal/providers"
)

// Compile-time check: a mock must satisfy LLMProvider.
type mockProvider struct{ name string }

func (m *mockProvider) Chat(_ context.Context, _ []providers.Message, _ []providers.Tool) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content:  "hello",
		ToolCall: nil,
		Usage: providers.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalCostUSD:     0.001,
		},
	}, nil
}
func (m *mockProvider) Name() string { return m.name }

func TestLLMProviderInterface(t *testing.T) {
	var p providers.LLMProvider = &mockProvider{name: "mock"}
	if p.Name() != "mock" {
		t.Errorf("Name() = %q, want %q", p.Name(), "mock")
	}
}

func TestLLMResponseUsageZeroValue(t *testing.T) {
	var u providers.Usage
	// Zero value is valid — providers with opaque billing return 0.
	if u.TotalCostUSD != 0 {
		t.Errorf("zero Usage.TotalCostUSD should be 0")
	}
	if u.PromptTokens != 0 || u.CompletionTokens != 0 {
		t.Errorf("zero Usage token counts should be 0")
	}
}

func TestMessageFields(t *testing.T) {
	msg := providers.Message{
		Role:       "tool",
		Content:    "result",
		ToolCallID: "call_abc",
		ToolName:   "web_search",
	}
	if msg.Role != "tool" {
		t.Errorf("Role = %q, want %q", msg.Role, "tool")
	}
	if msg.ToolCallID != "call_abc" {
		t.Errorf("ToolCallID = %q, want %q", msg.ToolCallID, "call_abc")
	}
}

func TestToolCallFields(t *testing.T) {
	tc := providers.ToolCall{
		ID:        "call_xyz",
		Name:      "web_fetch",
		InputJSON: `{"url":"https://example.com"}`,
	}
	if tc.Name != "web_fetch" {
		t.Errorf("ToolCall.Name = %q", tc.Name)
	}
	if tc.InputJSON == "" {
		t.Error("ToolCall.InputJSON should not be empty")
	}
}

func TestToolFields(t *testing.T) {
	tool := providers.Tool{
		Name:        "web_search",
		Description: "Search the web",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
		},
	}
	if tool.Name != "web_search" {
		t.Errorf("Tool.Name = %q", tool.Name)
	}
	if tool.InputSchema["type"] != "object" {
		t.Errorf("InputSchema type mismatch")
	}
}
