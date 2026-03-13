// Package tools defines the Tool interface and ToolEngine registry used by the gateway.
package tools

import (
	"context"
	"fmt"

	"github.com/canhta/gistclaw/internal/providers"
)

// Tool is the interface all gateway tools must implement.
type Tool interface {
	// Definition returns the tool metadata and JSON schema for the LLM.
	Definition() providers.Tool

	// Execute runs the tool with the given input (pre-unmarshaled from tc.InputJSON).
	// Must never return an empty ForLLM — use an error message if execution fails.
	Execute(ctx context.Context, input map[string]any) ToolResult
}

// ToolResult carries the output of a tool execution through two channels.
type ToolResult struct {
	// ForLLM is the content returned to the message loop. Never empty.
	ForLLM string

	// ForUser is shown to the user separately if non-empty.
	// Empty string means: send nothing to the user separately (NOT a fallback to ForLLM).
	ForUser string
}

// ToolEngine is a registry of Tools.
// It replaces the switch statement in gateway/service.go.
type ToolEngine struct {
	tools map[string]Tool
	order []string // insertion order
}

// NewToolEngine constructs an empty ToolEngine.
func NewToolEngine() *ToolEngine {
	return &ToolEngine{tools: make(map[string]Tool)}
}

// Register adds a Tool to the engine. Panics if a tool with the same name is
// registered twice (programming error, caught at startup).
func (e *ToolEngine) Register(t Tool) {
	name := t.Definition().Name
	if _, exists := e.tools[name]; exists {
		panic(fmt.Sprintf("tools: duplicate registration for %q", name))
	}
	e.tools[name] = t
	e.order = append(e.order, name)
}

// Definitions returns tool metadata for all registered tools (for the LLM tool list).
// Order reflects registration order.
func (e *ToolEngine) Definitions() []providers.Tool {
	defs := make([]providers.Tool, 0, len(e.order))
	for _, name := range e.order {
		defs = append(defs, e.tools[name].Definition())
	}
	return defs
}

// Execute dispatches to the registered tool by name.
// name is tc.Name at the call site; input is the pre-unmarshaled arguments.
func (e *ToolEngine) Execute(ctx context.Context, name string, input map[string]any) ToolResult {
	t, ok := e.tools[name]
	if !ok {
		return ToolResult{ForLLM: fmt.Sprintf("unknown tool: %q", name)}
	}
	return t.Execute(ctx, input)
}
