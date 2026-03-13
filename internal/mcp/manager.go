// internal/mcp/manager.go
package mcp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/providers"
)

const (
	defaultMCPConnectTimeout = 15 * time.Second
	defaultMCPCallTimeout    = 10 * time.Second
)

// Manager is the service boundary for MCP tool management.
// All service boundaries are Go interfaces; the concrete type is unexported.
type Manager interface {
	GetAllTools() []providers.Tool
	ServerStatus() map[string]bool
	CallTool(ctx context.Context, toolName string, input map[string]any) (string, error)
	Close() error
}

// connectedServer holds a live MCP client session and the tools it exposes.
type connectedServer struct {
	session *gomcp.ClientSession
	tools   []providers.Tool
}

// mcpManager manages connections to one or more MCP servers and provides
// a unified tool registry with namespaced tool names.
type mcpManager struct {
	mu             sync.RWMutex
	servers        map[string]*connectedServer // keyed by server name
	connectTimeout time.Duration
	callTimeout    time.Duration
}

// NewMCPManager creates a Manager and attempts to connect to all configured
// servers. Connection failures are logged at WARN level and the server is
// skipped — one bad server must not block startup.
func NewMCPManager(configs map[string]MCPServerConfig, tuning config.Tuning) Manager {
	connectTimeout := tuning.MCPConnectTimeout
	if connectTimeout == 0 {
		connectTimeout = defaultMCPConnectTimeout
	}
	callTimeout := tuning.MCPCallTimeout
	if callTimeout == 0 {
		callTimeout = defaultMCPCallTimeout
	}

	m := &mcpManager{
		servers:        make(map[string]*connectedServer),
		connectTimeout: connectTimeout,
		callTimeout:    callTimeout,
	}

	for name, cfg := range configs {
		srv, err := connectServer(name, cfg, connectTimeout)
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
func connectServer(name string, cfg MCPServerConfig, timeout time.Duration) (*connectedServer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client := gomcp.NewClient(&gomcp.Implementation{Name: "gistclaw"}, nil)

	var session *gomcp.ClientSession
	var err error

	switch {
	case cfg.Command != "":
		// Stdio transport using CommandTransport.
		env, envErr := buildEnv(cfg)
		if envErr != nil {
			return nil, fmt.Errorf("mcp: build env for %q: %w", name, envErr)
		}
		args := append([]string(nil), cfg.Args...)
		cmd := exec.Command(cfg.Command, args...) //nolint:gosec
		if len(env) > 0 {
			cmd.Env = env
		}
		transport := &gomcp.CommandTransport{Command: cmd}
		session, err = client.Connect(ctx, transport, nil)
	case cfg.URL != "":
		// SSE transport with optional custom headers via RoundTripper.
		transport := &gomcp.SSEClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: buildHTTPClient(cfg.Headers),
		}
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
		session: session,
		tools:   tools,
	}, nil
}

// buildEnv merges the server's Env list with KEY=VALUE pairs from EnvFile (if set).
// If EnvFile does not exist, a WARN is logged and the missing file is ignored.
func buildEnv(cfg MCPServerConfig) ([]string, error) {
	env := append([]string(nil), cfg.Env...)

	if cfg.EnvFile == "" {
		return env, nil
	}

	f, err := os.Open(cfg.EnvFile) //nolint:gosec
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Warn().Str("env_file", cfg.EnvFile).Msg("mcp: env_file not found, ignoring")
			return env, nil
		}
		return nil, fmt.Errorf("mcp: open env_file %q: %w", cfg.EnvFile, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		env = append(env, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("mcp: read env_file %q: %w", cfg.EnvFile, err)
	}

	return env, nil
}

// headerRoundTripper injects static headers into every request.
type headerRoundTripper struct {
	headers map[string]string
	base    http.RoundTripper
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	for k, v := range h.headers {
		r.Header.Set(k, v)
	}
	return h.base.RoundTrip(r)
}

// buildHTTPClient returns a default http.Client, or one with custom headers
// injected via a RoundTripper if headers is non-empty.
func buildHTTPClient(headers map[string]string) *http.Client {
	if len(headers) == 0 {
		return nil // SSEClientTransport uses http.DefaultClient when nil
	}
	return &http.Client{
		Transport: &headerRoundTripper{
			headers: headers,
			base:    http.DefaultTransport,
		},
	}
}

// GetAllTools returns all tools from all connected servers, each namespaced as
// "{serverName}__{toolName}". Returns an empty (non-nil) slice if no servers
// are connected.
func (m *mcpManager) GetAllTools() []providers.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	all := make([]providers.Tool, 0)
	for _, srv := range m.servers {
		all = append(all, srv.tools...)
	}
	return all
}

// ServerStatus returns a map of server name → connected (true) for all
// currently connected MCP servers. Returns an empty (non-nil) map if no
// servers are configured.
func (m *mcpManager) ServerStatus() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := make(map[string]bool)
	for name := range m.servers {
		status[name] = true // connected servers are live
	}
	return status
}

// CallTool calls a tool on the appropriate MCP server.
// toolName must be the namespaced form "{serverName}__{toolName}".
// Returns "MCP server '<name>' is not available" if server not connected (no error).
// Returns "MCP tool call timed out" if timeout (no error).
func (m *mcpManager) CallTool(ctx context.Context, toolName string, input map[string]any) (string, error) {
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

	callCtx, cancel := context.WithTimeout(ctx, m.callTimeout)
	defer cancel()

	result, err := srv.session.CallTool(callCtx, &gomcp.CallToolParams{
		Name:      rawTool,
		Arguments: input,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "MCP tool call timed out", nil
		}
		if errors.Is(err, context.Canceled) {
			return "", nil // clean shutdown — caller's context was cancelled
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
func (m *mcpManager) Close() error {
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
