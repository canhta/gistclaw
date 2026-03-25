package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeMCPFactory struct {
	servers map[string]*fakeMCPConnection
	err     error
}

func (f *fakeMCPFactory) Connect(_ context.Context, cfg MCPServerConfig) (MCPConnection, error) {
	if f.err != nil {
		return nil, f.err
	}
	server, ok := f.servers[cfg.ID]
	if !ok {
		return nil, errors.New("unknown fake server")
	}
	return server, nil
}

type fakeMCPConnection struct {
	tools     []MCPRemoteTool
	callName  string
	callArgs  map[string]any
	result    MCPToolCallResult
	closeHits int
}

func (f *fakeMCPConnection) ListTools(_ context.Context) ([]MCPRemoteTool, error) {
	return f.tools, nil
}

func (f *fakeMCPConnection) CallTool(_ context.Context, name string, args map[string]any) (MCPToolCallResult, error) {
	f.callName = name
	f.callArgs = args
	return f.result, nil
}

func (f *fakeMCPConnection) Close() error {
	f.closeHits++
	return nil
}

func TestBuildRegistry_LoadsConfiguredMCPTools(t *testing.T) {
	factory := &fakeMCPFactory{
		servers: map[string]*fakeMCPConnection{
			"github": {
				tools: []MCPRemoteTool{
					{Name: "search_repositories", Description: "Search repos", InputSchemaJSON: `{"type":"object"}`},
					{Name: "ignored_tool", Description: "Ignore", InputSchemaJSON: `{"type":"object"}`},
				},
			},
		},
	}

	reg, closer, err := BuildRegistry(context.Background(), BuildOptions{
		MCP: MCPOptions{
			Servers: []MCPServerConfig{
				{
					ID:        "github",
					Transport: "stdio",
					Command:   []string{"fake-mcp"},
					Tools: []MCPToolConfig{
						{Name: "search_repositories", Alias: "github_search_repositories", Risk: model.RiskLow, Enabled: true},
					},
				},
			},
		},
		MCPFactory: factory,
	})
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	t.Cleanup(func() {
		if closer != nil {
			_ = closer.Close()
		}
	})

	if _, ok := reg.Get("github_search_repositories"); !ok {
		t.Fatal("expected configured MCP alias to be registered")
	}
	if _, ok := reg.Get("ignored_tool"); ok {
		t.Fatal("expected unconfigured remote tool to stay hidden")
	}
}

func TestBuildRegistry_RejectsMissingConfiguredRemoteTool(t *testing.T) {
	factory := &fakeMCPFactory{
		servers: map[string]*fakeMCPConnection{
			"github": {
				tools: []MCPRemoteTool{
					{Name: "search_repositories", Description: "Search repos", InputSchemaJSON: `{"type":"object"}`},
				},
			},
		},
	}

	_, closer, err := BuildRegistry(context.Background(), BuildOptions{
		MCP: MCPOptions{
			Servers: []MCPServerConfig{
				{
					ID:        "github",
					Transport: "stdio",
					Command:   []string{"fake-mcp"},
					Tools: []MCPToolConfig{
						{Name: "missing_tool", Alias: "github_missing_tool", Risk: model.RiskLow, Enabled: true},
					},
				},
			},
		},
		MCPFactory: factory,
	})
	if closer != nil {
		_ = closer.Close()
	}
	if err == nil {
		t.Fatal("expected missing configured remote tool to fail")
	}
}

func TestMCPTool_InvokeCallsRemoteTool(t *testing.T) {
	conn := &fakeMCPConnection{
		tools: []MCPRemoteTool{
			{Name: "search_repositories", Description: "Search repos", InputSchemaJSON: `{"type":"object"}`},
		},
		result: MCPToolCallResult{
			Output:  `{"items":[{"name":"gistclaw"}]}`,
			IsError: false,
		},
	}
	factory := &fakeMCPFactory{
		servers: map[string]*fakeMCPConnection{"github": conn},
	}

	reg, closer, err := BuildRegistry(context.Background(), BuildOptions{
		MCP: MCPOptions{
			Servers: []MCPServerConfig{
				{
					ID:        "github",
					Transport: "stdio",
					Command:   []string{"fake-mcp"},
					Tools: []MCPToolConfig{
						{Name: "search_repositories", Alias: "github_search_repositories", Risk: model.RiskLow, Enabled: true},
					},
				},
			},
		},
		MCPFactory: factory,
	})
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	t.Cleanup(func() {
		if closer != nil {
			_ = closer.Close()
		}
	})

	tool, ok := reg.Get("github_search_repositories")
	if !ok {
		t.Fatal("expected MCP tool to exist")
	}

	got, err := tool.Invoke(context.Background(), model.ToolCall{
		ID:        "call-1",
		ToolName:  "github_search_repositories",
		InputJSON: []byte(`{"query":"gistclaw"}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if conn.callName != "search_repositories" {
		t.Fatalf("expected remote tool name, got %q", conn.callName)
	}
	if conn.callArgs["query"] != "gistclaw" {
		t.Fatalf("expected decoded args, got %+v", conn.callArgs)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload["items"] == nil {
		t.Fatalf("expected structured output, got %v", payload)
	}
}

func TestMCPTool_InvokeReturnsOutputAlongsideRemoteError(t *testing.T) {
	conn := &fakeMCPConnection{
		tools: []MCPRemoteTool{
			{Name: "search_repositories", Description: "Search repos", InputSchemaJSON: `{"type":"object"}`},
		},
		result: MCPToolCallResult{
			Output:  `{"error":"rate limited"}`,
			IsError: true,
		},
	}
	tool := newMCPTool(MCPToolConfig{
		Name:    "search_repositories",
		Alias:   "github_search_repositories",
		Risk:    model.RiskLow,
		Enabled: true,
	}, conn.tools[0], conn)

	got, err := tool.Invoke(context.Background(), model.ToolCall{
		ID:        "call-2",
		ToolName:  tool.Name(),
		InputJSON: []byte(`{"query":"gistclaw"}`),
	})
	if err == nil {
		t.Fatal("expected remote error to surface")
	}
	if got.Output != `{"error":"rate limited"}` {
		t.Fatalf("expected output to survive error path, got %q", got.Output)
	}
}

func TestNormalizeMCPToolOutput_StructuredContent(t *testing.T) {
	got, err := normalizeMCPToolOutput(&mcp.CallToolResult{
		StructuredContent: map[string]any{
			"items": []any{map[string]any{"name": "gistclaw"}},
		},
	})
	if err != nil {
		t.Fatalf("normalizeMCPToolOutput: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload["items"] == nil {
		t.Fatalf("expected structured content, got %v", payload)
	}
}

func TestNormalizeMCPToolOutput_TextJSON(t *testing.T) {
	got, err := normalizeMCPToolOutput(&mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: `{"items":[{"name":"gistclaw"}]}`},
		},
	})
	if err != nil {
		t.Fatalf("normalizeMCPToolOutput: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload["items"] == nil {
		t.Fatalf("expected JSON text to stay structured, got %v", payload)
	}
}

func TestSDKMCPFactory_RejectsUnsupportedTransport(t *testing.T) {
	_, err := (sdkMCPFactory{}).Connect(context.Background(), MCPServerConfig{
		ID:        "github",
		Transport: "sse",
		Command:   []string{"mcp-server"},
	})
	if err == nil {
		t.Fatal("expected unsupported transport to fail")
	}
}

func TestSDKMCPFactory_RejectsMissingCommand(t *testing.T) {
	_, err := (sdkMCPFactory{}).Connect(context.Background(), MCPServerConfig{
		ID:        "github",
		Transport: "stdio",
	})
	if err == nil {
		t.Fatal("expected missing command to fail")
	}
}

func TestExpandEnvMap_ExpandsValues(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret-token")

	got := expandEnvMap(map[string]string{
		"GITHUB_TOKEN": "${GITHUB_TOKEN}",
	})
	if len(got) != 1 || got[0] != "GITHUB_TOKEN=secret-token" {
		t.Fatalf("unexpected env expansion: %+v", got)
	}
}
