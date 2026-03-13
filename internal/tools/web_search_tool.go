package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/providers"
)

const webSearchResultCount = 5

// webSearchTool wraps the existing SearchProvider.
type webSearchTool struct {
	search SearchProvider
}

// NewWebSearchTool constructs the web_search tool.
// search may be nil — the tool will return an error message if called without a provider.
func NewWebSearchTool(search SearchProvider) Tool {
	return &webSearchTool{search: search}
}

func (w *webSearchTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        "web_search",
		Description: "Search the web for current information.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query"},
			},
			"required": []string{"query"},
		},
	}
}

func (w *webSearchTool) Execute(ctx context.Context, input map[string]any) ToolResult {
	if w.search == nil {
		return ToolResult{ForLLM: "web_search is unavailable: no search API key configured"}
	}
	query, _ := input["query"].(string)
	if query == "" {
		return ToolResult{ForLLM: "web_search: query is required"}
	}
	results, err := w.search.Search(ctx, query, webSearchResultCount)
	if err != nil {
		return ToolResult{ForLLM: fmt.Sprintf("web_search error: %v", err)}
	}
	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s\n", i+1, r.Title, r.URL, r.Snippet))
	}
	if sb.Len() == 0 {
		return ToolResult{ForLLM: "web_search: no results found"}
	}
	return ToolResult{ForLLM: sb.String()}
}
