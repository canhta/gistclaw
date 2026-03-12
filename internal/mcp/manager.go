// internal/mcp/manager.go
package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/providers"
)

const mcpConnectTimeout = 15 * time.Second
const mcpCallTimeout = 10 * time.Second

// connectedServer holds a live MCP client session and the tools it exposes.
type connectedServer struct {
	name    string
	session *gomcp.ClientSession
	tools   []providers.Tool
}

// MCPManager manages connections to one or more MCP servers and provides
// a unified tool registry with namespaced tool names.
type MCPManager struct {
	mu      sync.RWMutex
	servers map[string]*connectedServer // keyed by server name
}

// NewMCPManager creates an MCPManager and attempts to connect to all configured
// servers. Connection failures are logged at WARN level and the server is
// skipped — one bad server must not block startup.
func NewMCPManager(configs map[string]MCPServerConfig) *MCPManager {
	m := &MCPManager{
		servers: make(map[string]*connectedServer),
	}

	for name, cfg := range configs {
		srv, err := connectServer(name, cfg)
		if err != nil {
			log.Warn().
				Str("server", name).
				Err(err).
				Msg("mcp: failed to connect to server, skipping")
			continue
		}
		m.servers[name] = srv
	}

	return m
}

// connectServer establishes a connection to a single MCP server and lists its
// tools. Returns an error if the connection or tool listing fails.
func connectServer(name string, cfg MCPServerConfig) (*connectedServer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mcpConnectTimeout)
	defer cancel()

	client := gomcp.NewClient(&gomcp.Implementation{Name: "gistclaw"}, nil)

	var session *gomcp.ClientSession
	var err error

	switch {
	case cfg.Command != "":
		// Stdio transport using CommandTransport.
		args := append([]string(nil), cfg.Args...)
		cmd := exec.Command(cfg.Command, args...) //nolint:gosec
		if len(cfg.Env) > 0 {
			cmd.Env = cfg.Env
		}
		transport := &gomcp.CommandTransport{Command: cmd}
		session, err = client.Connect(ctx, transport, nil)
	case cfg.URL != "":
		// SSE transport.
		transport := &gomcp.SSEClientTransport{Endpoint: cfg.URL}
		session, err = client.Connect(ctx, transport, nil)
	default:
		return nil, fmt.Errorf("mcp: server %q has neither command nor url", name)
	}

	if err != nil {
		return nil, fmt.Errorf("mcp: connect %q: %w", name, err)
	}

	// List tools from the server.
	listResult, err := session.ListTools(ctx, nil)
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("mcp: list tools from %q: %w", name, err)
	}

	tools := make([]providers.Tool, 0, len(listResult.Tools))
	for _, t := range listResult.Tools {
		var schema map[string]any
		if s, ok := t.InputSchema.(map[string]any); ok {
			schema = s
		}
		tools = append(tools, providers.Tool{
			Name:        name + "__" + t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}

	return &connectedServer{
		name:    name,
		session: session,
		tools:   tools,
	}, nil
}

// GetAllTools returns all tools from all connected servers, each namespaced as
// "{serverName}__{toolName}". Returns an empty (non-nil) slice if no servers
// are connected.
func (m *MCPManager) GetAllTools() []providers.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	all := make([]providers.Tool, 0)
	for _, srv := range m.servers {
		all = append(all, srv.tools...)
	}
	return all
}

// CallTool calls a tool on the appropriate MCP server.
// toolName must be the namespaced form "{serverName}__{toolName}".
// Returns "MCP server '<name>' is not available" if server not connected (no error).
// Returns "MCP tool call timed out" if timeout (no error).
func (m *MCPManager) CallTool(ctx context.Context, toolName string, input map[string]any) (string, error) {
	serverName, rawTool, err := splitToolName(toolName)
	if err != nil {
		return "", err
	}

	m.mu.RLock()
	srv, ok := m.servers[serverName]
	m.mu.RUnlock()

	if !ok {
		return fmt.Sprintf("MCP server %q is not available", serverName), nil
	}

	callCtx, cancel := context.WithTimeout(ctx, mcpCallTimeout)
	defer cancel()

	result, err := srv.session.CallTool(callCtx, &gomcp.CallToolParams{
		Name:      rawTool,
		Arguments: input,
	})
	if err != nil {
		if callCtx.Err() == context.DeadlineExceeded {
			return "MCP tool call timed out", nil
		}
		return fmt.Sprintf("MCP tool error: %v", err), nil
	}

	// Extract text content from the result.
	var parts []string
	for _, c := range result.Content {
		if tc, ok := c.(*gomcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n"), nil
}

// splitToolName splits "{serverName}__{toolName}" into its two parts.
func splitToolName(toolName string) (serverName, rawTool string, err error) {
	idx := strings.Index(toolName, "__")
	if idx < 0 {
		return "", "", fmt.Errorf("mcp: tool name %q missing server prefix (expected 'server__tool')", toolName)
	}
	return toolName[:idx], toolName[idx+2:], nil
}

// Close disconnects all connected MCP servers.
func (m *MCPManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []string
	for name, srv := range m.servers {
		if err := srv.session.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
		}
	}
	m.servers = make(map[string]*connectedServer)

	if len(errs) > 0 {
		return fmt.Errorf("mcp: close errors: %s", strings.Join(errs, "; "))
	}
	return nil
}
