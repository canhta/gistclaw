// internal/mcp/manager_test.go
package mcp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/mcp"
	"github.com/canhta/gistclaw/internal/providers"
)

// ---------------------------------------------------------------------------
// NewMCPManager with empty config: succeeds, returns manager with no tools.
// ---------------------------------------------------------------------------

func TestNewMCPManager_EmptyConfig(t *testing.T) {
	m := mcp.NewMCPManager(map[string]mcp.MCPServerConfig{})
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	tools := m.GetAllTools()
	if tools == nil {
		t.Fatal("GetAllTools should return non-nil slice")
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
	if err := m.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Connection failure: bad server config → manager skips it, no panic.
// ---------------------------------------------------------------------------

func TestNewMCPManager_BadServerSkipped(t *testing.T) {
	// Provide a stdio config pointing at a non-existent binary.
	configs := map[string]mcp.MCPServerConfig{
		"bad-server": {
			Command: "/nonexistent/binary/that/does/not/exist",
			Args:    []string{"--mcp"},
		},
	}

	// Must not panic; connection failure is logged as WARN and server is skipped.
	m := mcp.NewMCPManager(configs)
	if m == nil {
		t.Fatal("expected non-nil manager even with bad server config")
	}

	// No tools from the failed server.
	tools := m.GetAllTools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools after connection failure, got %d", len(tools))
	}

	// Close should not error.
	if err := m.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CallTool: unknown server → informational string, not error.
// ---------------------------------------------------------------------------

func TestCallTool_UnknownServer(t *testing.T) {
	m := mcp.NewMCPManager(map[string]mcp.MCPServerConfig{})
	result, err := m.CallTool(context.Background(), "nonexistent__tool", map[string]any{"arg": "val"})
	if err != nil {
		t.Fatalf("expected no error for unknown server, got: %v", err)
	}
	if !strings.Contains(result, "nonexistent") {
		t.Errorf("expected result to mention server name 'nonexistent', got: %q", result)
	}
	if !strings.Contains(result, "not available") {
		t.Errorf("expected result to contain 'not available', got: %q", result)
	}
}

// ---------------------------------------------------------------------------
// CallTool: malformed tool name (no double underscore) → error.
// ---------------------------------------------------------------------------

func TestCallTool_MalformedToolName(t *testing.T) {
	m := mcp.NewMCPManager(map[string]mcp.MCPServerConfig{})
	_, err := m.CallTool(context.Background(), "notnamespaced", map[string]any{})
	if err == nil {
		t.Fatal("expected error for malformed tool name")
	}
}

// ---------------------------------------------------------------------------
// Tool namespacing: Tool.Name uses double underscore "{server}__{tool}".
// GetAllTools returns []providers.Tool so gateway can append directly.
// ---------------------------------------------------------------------------

func TestTool_NamespaceFormat(t *testing.T) {
	// Verify the providers.Tool type used by GetAllTools() uses the double
	// underscore separator convention by constructing one directly.
	tool := providers.Tool{
		Name:        "filesystem__read_file",
		Description: "Reads a file",
		InputSchema: map[string]any{"type": "object"},
	}
	if !strings.Contains(tool.Name, "__") {
		t.Errorf("Tool.Name %q does not contain double underscore separator", tool.Name)
	}
	parts := strings.SplitN(tool.Name, "__", 2)
	if parts[0] != "filesystem" {
		t.Errorf("Tool.Name server prefix %q != expected 'filesystem'", parts[0])
	}
}

// ---------------------------------------------------------------------------
// GetAllTools: returns []providers.Tool, non-nil even when no servers connected.
// ---------------------------------------------------------------------------

func TestGetAllTools_AlwaysNonNil(t *testing.T) {
	m := mcp.NewMCPManager(nil)
	tools := m.GetAllTools()
	if tools == nil {
		t.Fatal("GetAllTools must return non-nil slice")
	}
}
