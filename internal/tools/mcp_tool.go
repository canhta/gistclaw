package tools

import (
	"context"
	"fmt"

	"github.com/canhta/gistclaw/internal/mcp"
	"github.com/canhta/gistclaw/internal/providers"
)

// mcpTool wraps a single MCP tool discovered from mcp.Manager.
// Tool names are already namespaced as "{serverName}__{toolName}" by the manager.
type mcpTool struct {
	manager mcp.Manager
	def     providers.Tool
}

func (m *mcpTool) Definition() providers.Tool { return m.def }

func (m *mcpTool) Execute(ctx context.Context, input map[string]any) ToolResult {
	result, err := m.manager.CallTool(ctx, m.def.Name, input)
	if err != nil {
		return ToolResult{ForLLM: fmt.Sprintf("mcp %s error: %v", m.def.Name, err)}
	}
	if result == "" {
		return ToolResult{ForLLM: "mcp: tool call cancelled"}
	}
	return ToolResult{ForLLM: result}
}

// NewMCPTools returns Tool instances for all tools discovered from the MCP manager.
// Tool names are pre-namespaced as "{serverName}__{toolName}".
func NewMCPTools(manager mcp.Manager) []Tool {
	allTools := manager.GetAllTools()
	result := make([]Tool, 0, len(allTools))
	for _, def := range allTools {
		d := def // capture
		result = append(result, &mcpTool{
			manager: manager,
			def:     d,
		})
	}
	return result
}
