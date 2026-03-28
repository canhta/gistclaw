package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

const defaultSearchEndpoint = "https://api.tavily.com/search"

type ResearchConfig struct {
	Provider   string `yaml:"provider"`
	APIKey     string `yaml:"api_key"`
	MaxResults int    `yaml:"max_results"`
	TimeoutSec int    `yaml:"timeout_sec"`
}

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type SearchBackend interface {
	Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error)
}

type WebSearchTool struct {
	backend SearchBackend
	cfg     ResearchConfig
}

func NewWebSearchTool(backend SearchBackend, cfg ResearchConfig) *WebSearchTool {
	return &WebSearchTool{backend: backend, cfg: normalizeResearchConfig(cfg)}
}

func (t *WebSearchTool) Name() string { return "web_search" }

func (t *WebSearchTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Search the web and return normalized result objects with title, URL, and snippet.",
		InputSchemaJSON: `{"type":"object","properties":{"query":{"type":"string"},"max_results":{"type":"integer","minimum":1,"maximum":10}},"required":["query"]}`,
		Family:          model.ToolFamilyWebRead,
		Intents:         []model.ToolIntent{model.ToolIntentResearchRead},
		Risk:            model.RiskLow,
		SideEffect:      "none",
		Approval:        "never",
	}
}

func (t *WebSearchTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	var input struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("web_search: decode input: %w", err)
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return model.ToolResult{}, fmt.Errorf("web_search: query is required")
	}
	if t.backend == nil {
		return model.ToolResult{}, fmt.Errorf("web_search: backend not configured")
	}

	limit := input.MaxResults
	if limit <= 0 {
		limit = t.cfg.MaxResults
	}
	if limit <= 0 {
		limit = 5
	}

	results, err := t.backend.Search(ctx, query, limit)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("web_search: %w", err)
	}
	if len(results) > limit {
		results = results[:limit]
	}

	output, err := json.Marshal(struct {
		Query   string         `json:"query"`
		Results []SearchResult `json:"results"`
	}{
		Query:   query,
		Results: results,
	})
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("web_search: encode output: %w", err)
	}
	return model.ToolResult{Output: string(output)}, nil
}

type tavilySearchBackend struct {
	apiKey   string
	endpoint string
	client   *http.Client
}

func newTavilySearchBackend(cfg ResearchConfig) *tavilySearchBackend {
	cfg = normalizeResearchConfig(cfg)
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	return &tavilySearchBackend{
		apiKey:   cfg.APIKey,
		endpoint: defaultSearchEndpoint,
		client:   newBoundedHTTPClient(timeout),
	}
}

func (b *tavilySearchBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	body := map[string]any{
		"api_key":     b.apiKey,
		"query":       query,
		"max_results": maxResults,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", defaultResearchUserAgent)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search request failed with status %d", resp.StatusCode)
	}

	var payload struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	results := make([]SearchResult, 0, len(payload.Results))
	for _, item := range payload.Results {
		results = append(results, SearchResult{
			Title:   strings.TrimSpace(item.Title),
			URL:     strings.TrimSpace(item.URL),
			Snippet: strings.TrimSpace(item.Content),
		})
	}
	return results, nil
}

func normalizeResearchConfig(cfg ResearchConfig) ResearchConfig {
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = 5
	}
	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 10
	}
	return cfg
}
