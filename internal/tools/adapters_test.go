// Package tools_test tests the adapter wrappers in the tools package.
// scheduler_tool and mcp_tool require real service instances (DB, MCP server)
// and are covered at the integration/gateway layer instead.
package tools_test

import (
	"context"
	"errors"
	"testing"

	"github.com/canhta/gistclaw/internal/tools"
)

// --- mock SearchProvider ---

type mockSearchProvider struct {
	results []tools.SearchResult
	err     error
}

func (m *mockSearchProvider) Search(_ context.Context, _ string, _ int) ([]tools.SearchResult, error) {
	return m.results, m.err
}

func (m *mockSearchProvider) Name() string { return "mock" }

// --- mock WebFetcher ---

type mockWebFetcher struct {
	content string
	err     error
}

func (m *mockWebFetcher) Fetch(_ context.Context, _ string) (string, error) {
	return m.content, m.err
}

// ---------------------------------------------------------------------------
// web_search_tool tests
// ---------------------------------------------------------------------------

func TestWebSearchToolDefinition(t *testing.T) {
	tool := tools.NewWebSearchTool(nil)
	if tool.Definition().Name != "web_search" {
		t.Errorf("want name %q, got %q", "web_search", tool.Definition().Name)
	}
}

func TestWebSearchToolNilProvider(t *testing.T) {
	tool := tools.NewWebSearchTool(nil)
	result := tool.Execute(context.Background(), nil)
	if result.ForLLM == "" {
		t.Error("ForLLM must not be empty")
	}
}

func TestWebSearchToolEmptyQuery(t *testing.T) {
	tool := tools.NewWebSearchTool(&mockSearchProvider{})
	result := tool.Execute(context.Background(), map[string]any{})
	if result.ForLLM == "" {
		t.Error("ForLLM must not be empty for empty query")
	}
}

func TestWebSearchToolNoResults(t *testing.T) {
	tool := tools.NewWebSearchTool(&mockSearchProvider{results: nil, err: nil})
	result := tool.Execute(context.Background(), map[string]any{"query": "something"})
	if result.ForLLM == "" {
		t.Error("ForLLM must not be empty for zero results")
	}
}

func TestWebSearchToolSearchError(t *testing.T) {
	tool := tools.NewWebSearchTool(&mockSearchProvider{err: errors.New("network error")})
	result := tool.Execute(context.Background(), map[string]any{"query": "something"})
	if result.ForLLM == "" {
		t.Error("ForLLM must not be empty on search error")
	}
}

func TestWebSearchToolSuccess(t *testing.T) {
	tool := tools.NewWebSearchTool(&mockSearchProvider{
		results: []tools.SearchResult{
			{Title: "Go", URL: "https://go.dev", Snippet: "The Go language"},
		},
	})
	result := tool.Execute(context.Background(), map[string]any{"query": "golang"})
	if result.ForLLM == "" {
		t.Error("ForLLM must not be empty on success")
	}
}

// ---------------------------------------------------------------------------
// web_fetch_tool tests
// ---------------------------------------------------------------------------

func TestWebFetchToolDefinition(t *testing.T) {
	tool := tools.NewWebFetchTool(&mockWebFetcher{})
	if tool.Definition().Name != "web_fetch" {
		t.Errorf("want name %q, got %q", "web_fetch", tool.Definition().Name)
	}
}

func TestWebFetchToolEmptyURL(t *testing.T) {
	tool := tools.NewWebFetchTool(&mockWebFetcher{})
	result := tool.Execute(context.Background(), map[string]any{})
	if result.ForLLM == "" {
		t.Error("ForLLM must not be empty for empty url")
	}
}

func TestWebFetchToolFetchError(t *testing.T) {
	tool := tools.NewWebFetchTool(&mockWebFetcher{err: errors.New("timeout")})
	result := tool.Execute(context.Background(), map[string]any{"url": "https://example.com"})
	if result.ForLLM == "" {
		t.Error("ForLLM must not be empty on fetch error")
	}
}

func TestWebFetchToolSuccess(t *testing.T) {
	tool := tools.NewWebFetchTool(&mockWebFetcher{content: "page content"})
	result := tool.Execute(context.Background(), map[string]any{"url": "https://example.com"})
	if result.ForLLM != "page content" {
		t.Errorf("want %q, got %q", "page content", result.ForLLM)
	}
}
