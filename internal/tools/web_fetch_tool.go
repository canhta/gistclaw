package tools

import (
	"context"
	"fmt"

	"github.com/canhta/gistclaw/internal/providers"
)

// webFetchTool wraps the existing WebFetcher.
type webFetchTool struct {
	fetcher WebFetcher
}

// NewWebFetchTool constructs the web_fetch tool.
func NewWebFetchTool(fetcher WebFetcher) Tool {
	return &webFetchTool{fetcher: fetcher}
}

func (w *webFetchTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        "web_fetch",
		Description: "Fetch the content of a URL.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{"type": "string", "description": "URL to fetch"},
			},
			"required": []string{"url"},
		},
	}
}

func (w *webFetchTool) Execute(ctx context.Context, input map[string]any) ToolResult {
	url, _ := input["url"].(string)
	if url == "" {
		return ToolResult{ForLLM: "web_fetch: url is required"}
	}
	content, err := w.fetcher.Fetch(ctx, url)
	if err != nil {
		return ToolResult{ForLLM: fmt.Sprintf("web_fetch error: %v", err)}
	}
	return ToolResult{ForLLM: content}
}
