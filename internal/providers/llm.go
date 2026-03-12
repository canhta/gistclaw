// internal/providers/llm.go
package providers

import "context"

// Usage represents token consumption and cost for a single Chat call.
// All providers must populate TotalCostUSD.
// Providers with opaque billing (copilot, codex-oauth — billing is opaque
// on unofficial/gRPC backends) return 0.
// The trackingProvider decorator always calls CostGuard.Track(); a zero value is a
// valid no-op — it does not trigger soft-stop thresholds.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalCostUSD     float64 // required; 0 if provider cannot determine cost
}

// LLMResponse is the return value of a single Chat call.
type LLMResponse struct {
	Content  string
	ToolCall *ToolCall // nil if no tool call in this turn
	Usage    Usage     // always populated
}

// Message is a single turn in the conversation history.
type Message struct {
	Role    string // "user", "assistant", "system", "tool"
	Content string
	// For tool result messages (Role == "tool")
	ToolCallID string
	ToolName   string
}

// ToolCall is a tool invocation requested by the model.
type ToolCall struct {
	ID        string
	Name      string
	InputJSON string // raw JSON string of arguments
}

// Tool describes a function the model may call.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any // JSON Schema object
}

// LLMProvider is the interface all concrete provider implementations satisfy.
type LLMProvider interface {
	// Chat sends messages to the model with optional tools and returns the response.
	// Returns a non-nil *LLMResponse on success with Usage always populated.
	Chat(ctx context.Context, messages []Message, tools []Tool) (*LLMResponse, error)

	// Name returns the provider identifier, e.g. "openai", "copilot", "codex".
	Name() string
}
