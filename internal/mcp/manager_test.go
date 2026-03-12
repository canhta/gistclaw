// internal/mcp/manager_test.go
package mcp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/mcp"
)

// ---------------------------------------------------------------------------
// NewMCPManager with empty config: succeeds, returns manager with no tools.
// ---------------------------------------------------------------------------

func TestNewMCPManager_EmptyConfig(t *testing.T) {
	m := mcp.NewMCPManager(map[string]mcp.MCPServerConfig{}, config.Tuning{})
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
	m := mcp.NewMCPManager(configs, config.Tuning{})
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
	m := mcp.NewMCPManager(map[string]mcp.MCPServerConfig{}, config.Tuning{})
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
	m := mcp.NewMCPManager(map[string]mcp.MCPServerConfig{}, config.Tuning{})
	_, err := m.CallTool(context.Background(), "notnamespaced", map[string]any{})
	if err == nil {
		t.Fatal("expected error for malformed tool name")
	}
}

// ---------------------------------------------------------------------------
// Tool namespacing: double underscore convention verified via CallTool routing.
// The "__" separator is what routes "server__tool" to the right server.
// A name without "__" must error; a name with "__" must route (or report missing).
// Live-server tests with real tool listings are in integration tests.
// ---------------------------------------------------------------------------

func TestCallTool_DoubleUnderscoreRouting(t *testing.T) {
	m := mcp.NewMCPManager(map[string]mcp.MCPServerConfig{}, config.Tuning{})

	cases := []struct {
		name    string
		tool    string
		wantErr bool
		wantMsg string
	}{
		{
			name:    "single underscore — not a separator, must error",
			tool:    "server_tool",
			wantErr: true,
		},
		{
			name:    "double underscore routes to server (not connected → not available)",
			tool:    "myserver__read_file",
			wantErr: false,
			wantMsg: "not available",
		},
		{
			name:    "multiple underscores — split on first occurrence",
			tool:    "srv__tool__subname",
			wantErr: false,
			wantMsg: "not available",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := m.CallTool(context.Background(), tc.tool, map[string]any{})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got result=%q", result)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantMsg != "" && !strings.Contains(result, tc.wantMsg) {
				t.Errorf("result %q does not contain %q", result, tc.wantMsg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetAllTools: returns []providers.Tool, non-nil even when no servers connected.
// ---------------------------------------------------------------------------

func TestGetAllTools_AlwaysNonNil(t *testing.T) {
	m := mcp.NewMCPManager(nil, config.Tuning{})
	tools := m.GetAllTools()
	if tools == nil {
		t.Fatal("GetAllTools must return non-nil slice")
	}
}

// ---------------------------------------------------------------------------
// CallTool: cancelled parent context → clean nil return (AGENTS.md rule).
// ---------------------------------------------------------------------------

func TestCallTool_ContextCanceled_ReturnsNil(t *testing.T) {
	m := mcp.NewMCPManager(map[string]mcp.MCPServerConfig{}, config.Tuning{})

	// Use an already-cancelled context to simulate caller cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// The server name "srv" doesn't exist, so we'd normally get "not available".
	// But we need a connected server for the cancellation path to be reached.
	// Since we have no live server, we test the clean-shutdown contract at the
	// "unknown server" level — that path already returns ("", nil).
	// The Canceled branch in CallTool is exercised by the live integration path.
	// Here we verify that a pre-cancelled context on an unknown server still
	// returns ("...", nil) — i.e. no error propagates to the caller.
	result, err := m.CallTool(ctx, "srv__tool", map[string]any{})
	if err != nil {
		t.Fatalf("expected nil error with cancelled context on unknown server, got: %v", err)
	}
	_ = result
}

// ---------------------------------------------------------------------------
// Tuning: custom timeouts are respected (zero values use defaults, no panic).
// ---------------------------------------------------------------------------

func TestNewMCPManager_TuningZeroUsesDefaults(t *testing.T) {
	// Zero Tuning means defaults kick in; manager must still construct without panic.
	m := mcp.NewMCPManager(map[string]mcp.MCPServerConfig{}, config.Tuning{
		MCPConnectTimeout: 0,
		MCPCallTimeout:    0,
	})
	if m == nil {
		t.Fatal("expected non-nil manager with zero Tuning")
	}
	_ = m.Close()
}
