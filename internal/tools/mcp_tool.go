package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/canhta/gistclaw/internal/model"
)

type mcpTool struct {
	alias      string
	remoteName string
	spec       model.ToolSpec
	conn       MCPConnection
}

func newMCPTool(cfg MCPToolConfig, remote MCPRemoteTool, conn MCPConnection) *mcpTool {
	return &mcpTool{
		alias:      cfg.Alias,
		remoteName: remote.Name,
		spec: model.ToolSpec{
			Name:            cfg.Alias,
			Description:     remote.Description,
			InputSchemaJSON: remote.InputSchemaJSON,
			Family:          model.ToolFamilyRuntimeCapability,
			Risk:            cfg.Risk,
			SideEffect:      "remote_read",
			Approval:        "never",
		},
		conn: conn,
	}
}

func (t *mcpTool) Name() string { return t.alias }

func (t *mcpTool) Spec() model.ToolSpec { return t.spec }

func (t *mcpTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	var args map[string]any
	if len(call.InputJSON) > 0 {
		if err := json.Unmarshal(call.InputJSON, &args); err != nil {
			return model.ToolResult{}, fmt.Errorf("mcp tool %q: decode input: %w", t.alias, err)
		}
	}
	if args == nil {
		args = make(map[string]any)
	}

	result, err := t.conn.CallTool(ctx, t.remoteName, args)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("mcp tool %q: %w", t.alias, err)
	}

	toolResult := model.ToolResult{Output: result.Output}
	if result.IsError {
		return toolResult, fmt.Errorf("mcp tool %q reported an error", t.alias)
	}
	return toolResult, nil
}
