// internal/tools/search.go
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/canhta/gistclaw/internal/config"
)

// SearchResult is a single result returned by a SearchProvider.
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

// SearchProvider performs web searches and returns a ranked list of results.
type SearchProvider interface {
	Search(ctx context.Context, query string, count int) ([]SearchResult, error)
	Name() string
}

// NewSearchProvider auto-detects which provider to use based on which API key
// is set in cfg. Priority: Brave → Gemini → xAI → Perplexity → OpenRouter.
// Returns nil if no key is configured (caller should omit the web_search tool).
func NewSearchProvider(cfg config.Config) SearchProvider {
	return NewSearchProviderWithBaseURL(cfg, nil)
}

// NewSearchProviderWithBaseURL is like NewSearchProvider but allows overriding
// base URLs for each backend. Used in tests to point at httptest servers.
// Keys in baseURLs: "brave", "gemini", "xai", "perplexity", "openrouter".
func NewSearchProviderWithBaseURL(cfg config.Config, baseURLs map[string]string) SearchProvider {
	timeout := cfg.Tuning.WebSearchTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	base := func(key string) string {
		if baseURLs != nil {
			if u, ok := baseURLs[key]; ok {
				return u
			}
		}
		return ""
	}

	switch {
	case cfg.BraveAPIKey != "":
		u := base("brave")
		if u == "" {
			u = "https://api.search.brave.com/res/v1/web/search"
		}
		return &braveProvider{apiKey: cfg.BraveAPIKey, baseURL: u, client: client}
	case cfg.GeminiAPIKey != "":
		u := base("gemini")
		if u == "" {
			u = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"
		}
		return &geminiProvider{apiKey: cfg.GeminiAPIKey, baseURL: u, client: client}
	case cfg.XAIAPIKey != "":
		u := base("xai")
		if u == "" {
			u = "https://api.x.ai/v1/chat/completions"
		}
		return &xaiProvider{apiKey: cfg.XAIAPIKey, baseURL: u, client: client}
	case cfg.PerplexityAPIKey != "":
		u := base("perplexity")
		if u == "" {
			u = "https://api.perplexity.ai/chat/completions"
		}
		return &perplexityProvider{apiKey: cfg.PerplexityAPIKey, baseURL: u, client: client}
	case cfg.OpenRouterAPIKey != "":
		u := base("openrouter")
		if u == "" {
			u = "https://openrouter.ai/api/v1/chat/completions"
		}
		return &openrouterProvider{apiKey: cfg.OpenRouterAPIKey, baseURL: u, client: client}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Brave backend
// ---------------------------------------------------------------------------

type braveProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func (b *braveProvider) Name() string { return "brave" }

func (b *braveProvider) Search(ctx context.Context, query string, count int) ([]SearchResult, error) {
	endpoint := fmt.Sprintf("%s?q=%s&count=%d", b.baseURL, url.QueryEscape(query), count)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("brave: build request: %w", err)
	}
	req.Header.Set("X-Subscription-Token", b.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave: do request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brave: HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("brave: decode response: %w", err)
	}

	results := make([]SearchResult, 0, len(payload.Web.Results))
	for _, r := range payload.Web.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Description,
		})
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Gemini backend
// ---------------------------------------------------------------------------

type geminiProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func (g *geminiProvider) Name() string { return "gemini" }

func (g *geminiProvider) Search(ctx context.Context, query string, count int) ([]SearchResult, error) {
	body := map[string]any{
		"contents": []map[string]any{
			{"parts": []map[string]any{{"text": "Search the web for: " + query + "\nReturn top results as JSON array with fields title, url, snippet."}}},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s?key=%s", g.baseURL, g.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gemini: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: do request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini: HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("gemini: decode: %w", err)
	}

	// TODO: parse structured search results; returning synthesised text as single result for now.
	if len(payload.Candidates) == 0 {
		return nil, nil
	}
	text := ""
	if len(payload.Candidates[0].Content.Parts) > 0 {
		text = payload.Candidates[0].Content.Parts[0].Text
	}
	return []SearchResult{{Title: query, URL: "", Snippet: text}}, nil
}

// ---------------------------------------------------------------------------
// xAI Grok backend
// ---------------------------------------------------------------------------

type xaiProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func (x *xaiProvider) Name() string { return "xai" }

func (x *xaiProvider) Search(ctx context.Context, query string, count int) ([]SearchResult, error) {
	body := map[string]any{
		"model": "grok-3-latest",
		"messages": []map[string]any{
			{"role": "user", "content": fmt.Sprintf("Search the web for: %s\nReturn up to %d results as JSON array: [{\"title\":\"...\",\"url\":\"...\",\"snippet\":\"...\"}]", query, count)},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("xai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, x.baseURL,
		bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("xai: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+x.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := x.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xai: do request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xai: HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("xai: decode: %w", err)
	}

	// TODO: parse structured search results; returning synthesised text as single result for now.
	if len(payload.Choices) == 0 {
		return nil, nil
	}
	return []SearchResult{{Title: query, URL: "", Snippet: payload.Choices[0].Message.Content}}, nil
}

// ---------------------------------------------------------------------------
// Perplexity backend — uses sonar model, extracts citations as results.
// ---------------------------------------------------------------------------

type perplexityProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func (p *perplexityProvider) Name() string { return "perplexity" }

func (p *perplexityProvider) Search(ctx context.Context, query string, count int) ([]SearchResult, error) {
	body := map[string]any{
		"model": "sonar",
		"messages": []map[string]any{
			{"role": "user", "content": query},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("perplexity: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL,
		bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("perplexity: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perplexity: do request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("perplexity: HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Citations []string `json:"citations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("perplexity: decode: %w", err)
	}

	snippet := ""
	if len(payload.Choices) > 0 {
		snippet = payload.Choices[0].Message.Content
	}

	results := make([]SearchResult, 0, len(payload.Citations))
	for i, u := range payload.Citations {
		if i >= count {
			break
		}
		results = append(results, SearchResult{
			Title:   fmt.Sprintf("Citation %d", i+1),
			URL:     u,
			Snippet: snippet,
		})
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// OpenRouter backend — OpenAI-compatible API.
// ---------------------------------------------------------------------------

type openrouterProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func (o *openrouterProvider) Name() string { return "openrouter" }

func (o *openrouterProvider) Search(ctx context.Context, query string, count int) ([]SearchResult, error) {
	body := map[string]any{
		"model": "openai/gpt-4o-mini",
		"messages": []map[string]any{
			{"role": "user", "content": fmt.Sprintf("Search the web for: %s\nReturn up to %d results as JSON array: [{\"title\":\"...\",\"url\":\"...\",\"snippet\":\"...\"}]", query, count)},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openrouter: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL,
		bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("openrouter: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter: do request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter: HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("openrouter: decode: %w", err)
	}

	// TODO: parse structured search results; returning synthesised text as single result for now.
	if len(payload.Choices) == 0 {
		return nil, nil
	}
	return []SearchResult{{Title: query, URL: "", Snippet: payload.Choices[0].Message.Content}}, nil
}
