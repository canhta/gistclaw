package tools_test

import (
	"context"
	"testing"

	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/tools"
)

// echoTool is a minimal Tool implementation for testing.
type echoTool struct{ name string }

func (e *echoTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        e.name,
		Description: "echoes input",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}
}
func (e *echoTool) Execute(_ context.Context, input map[string]any) tools.ToolResult {
	return tools.ToolResult{ForLLM: "echo:" + e.name}
}

func TestToolEngine_RegisterAndDefinitions(t *testing.T) {
	engine := tools.NewToolEngine()
	engine.Register(&echoTool{name: "tool_a"})
	engine.Register(&echoTool{name: "tool_b"})
	defs := engine.Definitions()
	if len(defs) != 2 {
		t.Fatalf("Definitions: got %d, want 2", len(defs))
	}
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	for _, want := range []string{"tool_a", "tool_b"} {
		if !names[want] {
			t.Errorf("Definitions: missing tool %q", want)
		}
	}
}

func TestToolEngine_Register_DuplicatePanics(t *testing.T) {
	engine := tools.NewToolEngine()
	engine.Register(&echoTool{name: "dup"})
	defer func() {
		if r := recover(); r == nil {
			t.Error("Register duplicate: expected panic, got none")
		}
	}()
	engine.Register(&echoTool{name: "dup"})
}

func TestToolEngine_Execute_KnownTool(t *testing.T) {
	engine := tools.NewToolEngine()
	engine.Register(&echoTool{name: "my_tool"})
	result := engine.Execute(context.Background(), "my_tool", nil)
	if result.ForLLM != "echo:my_tool" {
		t.Errorf("Execute: got %q, want %q", result.ForLLM, "echo:my_tool")
	}
}

func TestToolEngine_Execute_UnknownTool(t *testing.T) {
	engine := tools.NewToolEngine()
	result := engine.Execute(context.Background(), "not_registered", nil)
	if result.ForLLM == "" {
		t.Error("Execute unknown tool: ForLLM should not be empty")
	}
}

func TestToolResult_ForUser_EmptyMeansNoUserOutput(t *testing.T) {
	// Document the contract: ForUser="" means send nothing to user (not fall back to ForLLM).
	result := tools.ToolResult{ForLLM: "internal", ForUser: ""}
	if result.ForUser != "" {
		t.Error("ForUser should default to empty")
	}
}
