package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type MCPOptions struct {
	Servers []MCPServerConfig `yaml:"servers"`
}

type MCPServerConfig struct {
	ID        string            `yaml:"id"`
	Transport string            `yaml:"transport"`
	Command   []string          `yaml:"command"`
	Env       map[string]string `yaml:"env"`
	Tools     []MCPToolConfig   `yaml:"tools"`
}

type MCPToolConfig struct {
	Name    string         `yaml:"name"`
	Alias   string         `yaml:"alias"`
	Risk    model.ToolRisk `yaml:"risk"`
	Enabled bool           `yaml:"enabled"`
}

type MCPRemoteTool struct {
	Name            string
	Description     string
	InputSchemaJSON string
}

type MCPToolCallResult struct {
	Output  string
	IsError bool
}

type MCPFactory interface {
	Connect(ctx context.Context, cfg MCPServerConfig) (MCPConnection, error)
}

type MCPConnection interface {
	ListTools(ctx context.Context) ([]MCPRemoteTool, error)
	CallTool(ctx context.Context, name string, args map[string]any) (MCPToolCallResult, error)
	Close() error
}

func loadMCPTools(ctx context.Context, reg *Registry, opts MCPOptions, factory MCPFactory) (io.Closer, error) {
	if len(opts.Servers) == 0 {
		return nil, nil
	}
	if factory == nil {
		factory = sdkMCPFactory{}
	}

	var closers multiCloser
	registered := make(map[string]bool)

	for _, server := range opts.Servers {
		if countEnabledTools(server.Tools) == 0 {
			continue
		}
		conn, err := factory.Connect(ctx, server)
		if err != nil {
			_ = closers.Close()
			return nil, fmt.Errorf("tools: connect mcp server %q: %w", server.ID, err)
		}

		remoteTools, err := conn.ListTools(ctx)
		if err != nil {
			_ = conn.Close()
			_ = closers.Close()
			return nil, fmt.Errorf("tools: list tools for server %q: %w", server.ID, err)
		}

		remoteByName := make(map[string]MCPRemoteTool, len(remoteTools))
		for _, remote := range remoteTools {
			remoteByName[remote.Name] = remote
		}

		for _, cfg := range server.Tools {
			if !cfg.Enabled {
				continue
			}
			remote, ok := remoteByName[cfg.Name]
			if !ok {
				_ = conn.Close()
				_ = closers.Close()
				return nil, fmt.Errorf("tools: configured mcp tool %q not found on server %q", cfg.Name, server.ID)
			}
			if registered[cfg.Alias] {
				_ = conn.Close()
				_ = closers.Close()
				return nil, fmt.Errorf("tools: duplicate registry tool alias %q", cfg.Alias)
			}
			reg.Register(newMCPTool(cfg, remote, conn))
			registered[cfg.Alias] = true
		}

		closers = append(closers, conn)
	}

	if len(closers) == 0 {
		return nil, nil
	}
	return closers, nil
}

func countEnabledTools(tools []MCPToolConfig) int {
	count := 0
	for _, tool := range tools {
		if tool.Enabled {
			count++
		}
	}
	return count
}

type sdkMCPFactory struct{}

func (sdkMCPFactory) Connect(ctx context.Context, cfg MCPServerConfig) (MCPConnection, error) {
	if cfg.Transport == "" {
		cfg.Transport = "stdio"
	}
	if cfg.Transport != "stdio" {
		return nil, fmt.Errorf("unsupported mcp transport %q", cfg.Transport)
	}
	if len(cfg.Command) == 0 {
		return nil, fmt.Errorf("mcp server %q: command is required", cfg.ID)
	}

	cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
	cmd.Env = append(os.Environ(), expandEnvMap(cfg.Env)...)

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "gistclaw",
		Version: "dev",
	}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return nil, err
	}
	return &sdkMCPConnection{session: session}, nil
}

func expandEnvMap(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	pairs := make([]string, 0, len(env))
	for key, value := range env {
		pairs = append(pairs, key+"="+os.ExpandEnv(value))
	}
	return pairs
}

type sdkMCPConnection struct {
	session *mcp.ClientSession
}

func (c *sdkMCPConnection) ListTools(ctx context.Context) ([]MCPRemoteTool, error) {
	result, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}
	tools := make([]MCPRemoteTool, 0, len(result.Tools))
	for _, tool := range result.Tools {
		schemaJSON := `{"type":"object"}`
		if tool.InputSchema != nil {
			raw, err := json.Marshal(tool.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("marshal mcp tool schema %q: %w", tool.Name, err)
			}
			schemaJSON = string(raw)
		}
		tools = append(tools, MCPRemoteTool{
			Name:            tool.Name,
			Description:     tool.Description,
			InputSchemaJSON: schemaJSON,
		})
	}
	return tools, nil
}

func (c *sdkMCPConnection) CallTool(ctx context.Context, name string, args map[string]any) (MCPToolCallResult, error) {
	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return MCPToolCallResult{}, err
	}
	output, err := normalizeMCPToolOutput(result)
	if err != nil {
		return MCPToolCallResult{}, err
	}
	return MCPToolCallResult{Output: output, IsError: result.IsError}, nil
}

func (c *sdkMCPConnection) Close() error {
	if c.session == nil {
		return nil
	}
	return c.session.Close()
}

func normalizeMCPToolOutput(result *mcp.CallToolResult) (string, error) {
	if result == nil {
		return `{"content":[]}`, nil
	}
	if result.StructuredContent != nil {
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return "", fmt.Errorf("marshal structured mcp result: %w", err)
		}
		return string(raw), nil
	}

	if len(result.Content) == 1 {
		raw, err := result.Content[0].MarshalJSON()
		if err != nil {
			return "", fmt.Errorf("marshal mcp content: %w", err)
		}
		var textPayload struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(raw, &textPayload); err == nil && strings.TrimSpace(textPayload.Text) != "" {
			text := strings.TrimSpace(textPayload.Text)
			if json.Valid([]byte(text)) {
				return text, nil
			}
			wrapped, err := json.Marshal(map[string]any{"text": text})
			if err != nil {
				return "", fmt.Errorf("marshal wrapped text mcp result: %w", err)
			}
			return string(wrapped), nil
		}
	}

	content := make([]json.RawMessage, 0, len(result.Content))
	for _, item := range result.Content {
		raw, err := item.MarshalJSON()
		if err != nil {
			return "", fmt.Errorf("marshal mcp content item: %w", err)
		}
		content = append(content, raw)
	}
	raw, err := json.Marshal(map[string]any{"content": content})
	if err != nil {
		return "", fmt.Errorf("marshal content-array mcp result: %w", err)
	}
	return string(raw), nil
}
