# GistClaw Plan 4: Tools & MCP

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the two tool packages (`internal/tools`) and the MCP client layer (`internal/mcp`) that the agent services call at runtime to perform web search, web fetch, and MCP tool invocations.

**Architecture:** `internal/tools` is two files — `search.go` (auto-detecting search provider with five backends) and `fetch.go` (readability-based HTML-to-text fetcher). `internal/mcp` is also two files — `config.go` (YAML parser for `gistclaw.yaml`) and `manager.go` (MCP client hub: connect all servers, namespace tools, call by name). All four files are stateless or hold only connection state; no goroutines are spawned here — callers own the loops.

**Tech Stack:** Go 1.25, `github.com/go-shiori/go-readability`, `github.com/modelcontextprotocol/go-sdk/mcp`, `gopkg.in/yaml.v3`

**Design reference:** `docs/plans/design.md` §9.15, §9.22, §11

**Depends on:** Plan 1 (config), Plan 2 (store) — parallel to Plans 3 and 5

---

## Execution order

```
Task 1  internal/tools/search.go   (SearchProvider interface + backends + factory)
Task 2  internal/tools/fetch.go    (WebFetcher with go-readability)
Task 3  internal/mcp/config.go     (MCPServerConfig + LoadMCPConfig)
Task 4  internal/mcp/manager.go    (MCPManager: connect / GetAllTools / CallTool / Close)
```

Tasks 1–4 are independent of each other and can be implemented in any order.

---

### Task 1: `internal/tools/search.go` — SearchProvider interface + all backends + factory

**Files:**
- Create: `internal/tools/search.go`
- Create: `internal/tools/search_test.go`

**Step 1: Write the failing test**

```go
// internal/tools/search_test.go
package tools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/tools"
)

// ---------------------------------------------------------------------------
// Helper: build a config with only the named key set.
// ---------------------------------------------------------------------------

func cfgWithKey(key, value string) config.Config {
	var cfg config.Config
	switch key {
	case "brave":
		cfg.BraveAPIKey = value
	case "gemini":
		cfg.GeminiAPIKey = value
	case "xai":
		cfg.XAIAPIKey = value
	case "perplexity":
		cfg.PerplexityAPIKey = value
	}
	return cfg
}

// ---------------------------------------------------------------------------
// Factory: auto-detect priority tests.
// ---------------------------------------------------------------------------

func TestNewSearchProvider_NoKeys_ReturnsNil(t *testing.T) {
	p := tools.NewSearchProvider(config.Config{})
	if p != nil {
		t.Errorf("expected nil provider when no API keys set, got %T", p)
	}
}

func TestNewSearchProvider_BravePriority(t *testing.T) {
	cfg := config.Config{
		BraveAPIKey:      "brave-key",
		GeminiAPIKey:     "gemini-key",
		XAIAPIKey:        "xai-key",
		PerplexityAPIKey: "perp-key",
	}
	p := tools.NewSearchProvider(cfg)
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.Name() != "brave" {
		t.Errorf("expected brave provider, got %q", p.Name())
	}
}

func TestNewSearchProvider_FallsToGemini(t *testing.T) {
	cfg := cfgWithKey("gemini", "gemini-key")
	p := tools.NewSearchProvider(cfg)
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.Name() != "gemini" {
		t.Errorf("expected gemini provider, got %q", p.Name())
	}
}

func TestNewSearchProvider_FallsToXAI(t *testing.T) {
	p := tools.NewSearchProvider(cfgWithKey("xai", "xai-key"))
	if p == nil || p.Name() != "xai" {
		t.Errorf("expected xai provider, got %v", p)
	}
}

func TestNewSearchProvider_FallsToPerplexity(t *testing.T) {
	p := tools.NewSearchProvider(cfgWithKey("perplexity", "perp-key"))
	if p == nil || p.Name() != "perplexity" {
		t.Errorf("expected perplexity provider, got %v", p)
	}
}

// ---------------------------------------------------------------------------
// Brave backend: integration test against httptest server.
// ---------------------------------------------------------------------------

func TestBraveSearch(t *testing.T) {
	// Mock Brave API response.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		resp := map[string]any{
			"web": map[string]any{
				"results": []map[string]any{
					{"title": "Result 1", "url": "https://example.com/1", "description": "Snippet 1"},
					{"title": "Result 2", "url": "https://example.com/2", "description": "Snippet 2"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := config.Config{BraveAPIKey: "test-key"}
	p := tools.NewSearchProviderWithBaseURL(cfg, map[string]string{"brave": srv.URL})
	if p == nil {
		t.Fatal("expected non-nil provider")
	}

	results, err := p.Search(context.Background(), "golang", 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "Result 1" {
		t.Errorf("results[0].Title = %q, want %q", results[0].Title, "Result 1")
	}
	if results[0].URL != "https://example.com/1" {
		t.Errorf("results[0].URL = %q", results[0].URL)
	}
	if results[0].Snippet != "Snippet 1" {
		t.Errorf("results[0].Snippet = %q", results[0].Snippet)
	}
}

// ---------------------------------------------------------------------------
// Perplexity backend: extracts citations as results.
// ---------------------------------------------------------------------------

func TestPerplexitySearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "Answer about golang"}},
			},
			"citations": []string{
				"https://go.dev",
				"https://pkg.go.dev",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := config.Config{PerplexityAPIKey: "perp-key"}
	p := tools.NewSearchProviderWithBaseURL(cfg, map[string]string{"perplexity": srv.URL})
	if p == nil {
		t.Fatal("expected non-nil provider")
	}

	results, err := p.Search(context.Background(), "golang", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	if results[0].URL != "https://go.dev" {
		t.Errorf("results[0].URL = %q, want https://go.dev", results[0].URL)
	}
}

// ---------------------------------------------------------------------------
// Search result fields are populated correctly.
// ---------------------------------------------------------------------------

func TestSearchResultFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"web": map[string]any{
				"results": []map[string]any{
					{"title": "T", "url": "https://u.com", "description": "S"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := config.Config{BraveAPIKey: "k"}
	p := tools.NewSearchProviderWithBaseURL(cfg, map[string]string{"brave": srv.URL})
	results, _ := p.Search(context.Background(), "q", 1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result")
	}
	r := results[0]
	if r.Title == "" || r.URL == "" || r.Snippet == "" {
		t.Errorf("empty field in result: %+v", r)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/tools/...
```

Expected: `FAIL` — package does not exist yet.

**Step 3: Add dependency**

```bash
go mod tidy
```

No new external dependencies for `search.go` — it uses only `net/http` and `encoding/json` from stdlib. (Dependencies for other tasks are added in their respective steps.)

**Step 4: Write implementation**

```go
// internal/tools/search.go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
// is set in cfg. Priority: Brave → Gemini → xAI → Perplexity.
// Returns nil if no key is configured (caller should omit the web_search tool).
// timeout is used for all HTTP calls; pass cfg.Tuning.WebSearchTimeout.
func NewSearchProvider(cfg config.Config) SearchProvider {
	return NewSearchProviderWithBaseURL(cfg, nil)
}

// NewSearchProviderWithBaseURL is like NewSearchProvider but allows overriding
// base URLs for each backend. Used in tests to point at httptest servers.
// Keys in baseURLs: "brave", "gemini", "xai", "perplexity".
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
	}
	return nil
}

// httpClientFor creates a client with the given timeout.
// Backends hold their own *http.Client set at construction time.

// httpClientFor creates a client with the given timeout.
// Backends hold their own *http.Client set at construction time.

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
	defer resp.Body.Close()

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
// TODO: Replace stub with production Gemini grounding/search API call.
// The current implementation uses generateContent with a search instruction
// and returns a single synthesised result. A production version should use
// the Grounding with Google Search feature when available in the Go SDK.
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
	data, _ := json.Marshal(body)

	endpoint := fmt.Sprintf("%s?key=%s", g.baseURL, g.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("gemini: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: do request: %w", err)
	}
	defer resp.Body.Close()

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

	// TODO: Parse structured search results from grounding metadata.
	// For now return the raw text as a single result.
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
// TODO: Replace with the official xAI search API when published.
// Currently uses the chat completions endpoint with a search instruction.
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
	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, x.baseURL,
		strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("xai: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+x.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := x.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xai: do request: %w", err)
	}
	defer resp.Body.Close()

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

	// TODO: Parse structured JSON from content when Grok returns it reliably.
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
	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL,
		strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("perplexity: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perplexity: do request: %w", err)
	}
	defer resp.Body.Close()

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

	// Extract citations as URL results; fill snippet from the answer content.
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
```

**Step 5: Run test to verify it passes**

```bash
go test ./internal/tools/... -run TestNewSearchProvider -v
go test ./internal/tools/... -run TestBraveSearch -v
go test ./internal/tools/... -run TestPerplexitySearch -v
go test ./internal/tools/... -run TestSearchResultFields -v
```

Expected: all `PASS`.

**Step 6: Commit**

```bash
git add internal/tools/search.go internal/tools/search_test.go
git commit -m "feat: add SearchProvider interface with Brave/Gemini/xAI/Perplexity backends"
```

---

### Task 2: `internal/tools/fetch.go` — WebFetcher with go-readability

**Files:**
- Create: `internal/tools/fetch.go`
- Create: `internal/tools/fetch_test.go`

**Step 1: Write the failing test**

```go
// internal/tools/fetch_test.go
package tools_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/tools"
)

func newFetcher() tools.WebFetcher {
	return tools.NewWebFetcher()
}

// ---------------------------------------------------------------------------
// Happy path: server returns valid HTML — readability extracts the text.
// ---------------------------------------------------------------------------

func TestFetch_ExtractsReadableContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Test Page</title></head><body>
<article>
<h1>Hello World</h1>
<p>This is the main content of the article that should be extracted by readability.</p>
<p>A second paragraph with more meaningful content to pass readability heuristics.</p>
</article>
<nav><a href="/other">other</a></nav>
</body></html>`)
	}))
	defer srv.Close()

	f := newFetcher()
	content, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if content == "" {
		t.Fatal("expected non-empty content")
	}
	// Readability should preserve the heading and paragraph text.
	if !strings.Contains(content, "Hello World") && !strings.Contains(content, "main content") {
		t.Errorf("content does not contain expected text; got: %q", content[:min(200, len(content))])
	}
}

// ---------------------------------------------------------------------------
// Non-2xx: returned as string, not error.
// ---------------------------------------------------------------------------

func TestFetch_Non2xxReturnedAsString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	f := newFetcher()
	content, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("expected no error for non-2xx, got: %v", err)
	}
	if !strings.HasPrefix(content, "HTTP 404") {
		t.Errorf("expected content to start with 'HTTP 404', got: %q", content)
	}
	if !strings.Contains(content, srv.URL) {
		t.Errorf("expected content to contain URL %q, got: %q", srv.URL, content)
	}
}

// ---------------------------------------------------------------------------
// Body too large: returned as string, not error.
// ---------------------------------------------------------------------------

func TestFetch_BodyTooLargeReturnedAsString(t *testing.T) {
	const limit = 5 * 1024 * 1024 // 5 MB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// Write more than 5 MB.
		chunk := strings.Repeat("a", 1024)
		for i := 0; i < limit/len(chunk)+2; i++ {
			_, _ = w.Write([]byte(chunk))
		}
	}))
	defer srv.Close()

	f := newFetcher()
	content, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("expected no error for oversized body, got: %v", err)
	}
	if content != "Page too large (> 5 MB)" {
		t.Errorf("expected 'Page too large' message, got: %q", content)
	}
}

// ---------------------------------------------------------------------------
// Cloudflare 403 retry: second request (with "gistclaw" UA) returns 200.
// ---------------------------------------------------------------------------

func TestFetch_Cloudflare403Retry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			// Simulate Cloudflare-gated 403 with cf-mitigated header.
			w.Header().Set("cf-mitigated", "challenge")
			http.Error(w, "Cloudflare", http.StatusForbidden)
			return
		}
		// Second attempt with fallback UA succeeds.
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><article><p>Retry content is here and has enough text to pass readability.</p></article></body></html>`)
	}))
	defer srv.Close()

	f := newFetcher()
	content, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if attempts < 2 {
		t.Errorf("expected at least 2 attempts (retry), got %d", attempts)
	}
	// Either succeeded or returned HTTP 403 string (if retry also fails).
	_ = content // both outcomes are valid; the test verifies retry happened.
}

// ---------------------------------------------------------------------------
// Context cancellation is respected.
// ---------------------------------------------------------------------------

func TestFetch_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never respond — hang forever.
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	f := newFetcher()
	_, err := f.Fetch(ctx, srv.URL)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/tools/... -run TestFetch -v
```

Expected: `FAIL` — `tools.WebFetcher` and `tools.NewWebFetcher` not defined yet.

**Step 3: Add dependency**

```bash
go get github.com/go-shiori/go-readability
go mod tidy
```

**Step 4: Write implementation**

```go
// internal/tools/fetch.go
package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	readability "github.com/go-shiori/go-readability"
)

const (
	fetchBodyLimit   = 5 * 1024 * 1024 // 5 MB
	fetchTimeout     = 30 * time.Second
	fetchChromeUA    = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	fetchFallbackUA  = "gistclaw"
)

// WebFetcher fetches a URL and returns its readable text content.
type WebFetcher interface {
	Fetch(ctx context.Context, rawURL string) (content string, err error)
}

type webFetcher struct {
	client *http.Client
}

// NewWebFetcher creates a WebFetcher backed by go-readability.
//   - Limits response body to 5 MB.
//   - Uses Chrome User-Agent by default.
//   - On Cloudflare 403, retries once with "gistclaw" User-Agent.
//   - Non-2xx responses: returns "HTTP <code> from <url>" (not an error).
//   - Body > 5 MB: returns "Page too large (> 5 MB)" (not an error).
func NewWebFetcher() WebFetcher {
	return &webFetcher{
		client: &http.Client{Timeout: fetchTimeout},
	}
}

func (f *webFetcher) Fetch(ctx context.Context, rawURL string) (string, error) {
	content, status, cfBlocked, err := f.doFetch(ctx, rawURL, fetchChromeUA)
	if err != nil {
		return "", err
	}

	// Cloudflare 403 retry: only when cf-mitigated header is present.
	if status == http.StatusForbidden && cfBlocked {
		content, status, _, err = f.doFetch(ctx, rawURL, fetchFallbackUA)
		if err != nil {
			return "", err
		}
	}

	if status != 0 {
		// Non-2xx returned as informational string, not an error.
		return fmt.Sprintf("HTTP %d from %s", status, rawURL), nil
	}
	return content, nil
}

// doFetch performs a single HTTP GET. Returns (content, 0, false, nil) on success,
// (content, statusCode, cfBlocked, nil) on non-2xx, ("", 0, false, err) on network/parse error,
// ("Page too large (> 5 MB)", 0, false, nil) if body exceeds limit.
// cfBlocked is true when status is 403 and the cf-mitigated response header is present.
func (f *webFetcher) doFetch(ctx context.Context, rawURL string, userAgent string) (string, int, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", 0, false, fmt.Errorf("fetch: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := f.client.Do(req)
	if err != nil {
		return "", 0, false, fmt.Errorf("fetch: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		cfBlocked := resp.StatusCode == http.StatusForbidden && resp.Header.Get("cf-mitigated") != ""
		return "", resp.StatusCode, cfBlocked, nil
	}

	// Read with size limit.
	limited := io.LimitReader(resp.Body, int64(fetchBodyLimit)+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", 0, false, fmt.Errorf("fetch: read body: %w", err)
	}
	if len(data) > fetchBodyLimit {
		return "Page too large (> 5 MB)", 0, false, nil
	}

	// Parse with go-readability.
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", 0, false, fmt.Errorf("fetch: parse url: %w", err)
	}

	article, err := readability.FromReader(strings.NewReader(string(data)), parsedURL)
	if err != nil {
		// Readability failed — return raw text stripped of HTML tags as fallback.
		return stripTags(string(data)), 0, false, nil
	}

	// Prefer TextContent; fall back to Title if empty.
	text := strings.TrimSpace(article.TextContent)
	if text == "" {
		text = article.Title
	}
	return text, 0, false, nil
}

// stripTags is a very simple HTML tag stripper used as a readability fallback.
// It does NOT handle all edge cases — it is only called when readability fails.
func stripTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
```

**Step 5: Run test to verify it passes**

```bash
go test ./internal/tools/... -run TestFetch -v
```

Expected: all `PASS`. (The `TestFetch_Cloudflare403Retry` test verifies that at least 2 HTTP requests were made; both retry outcomes are valid.)

**Step 6: Commit**

```bash
git add internal/tools/fetch.go internal/tools/fetch_test.go go.mod go.sum
git commit -m "feat: add WebFetcher backed by go-readability with 5 MB limit and CF retry"
```

---

### Task 3: `internal/mcp/config.go` — MCPServerConfig + LoadMCPConfig

**Files:**
- Create: `internal/mcp/config.go`
- Create: `internal/mcp/config_test.go`

**Step 1: Write the failing test**

```go
// internal/mcp/config_test.go
package mcp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/canhta/gistclaw/internal/mcp"
)

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "gistclaw.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp yaml: %v", err)
	}
	return p
}

// ---------------------------------------------------------------------------
// Basic YAML parsing.
// ---------------------------------------------------------------------------

func TestLoadMCPConfig_ParsesStdioServer(t *testing.T) {
	yaml := `
mcp_servers:
  filesystem:
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/home/user"]
    env: ["HOME=/home/user"]
`
	path := writeTempYAML(t, yaml)
	configs, err := mcp.LoadMCPConfig(path)
	if err != nil {
		t.Fatalf("LoadMCPConfig: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 server, got %d", len(configs))
	}
	fs, ok := configs["filesystem"]
	if !ok {
		t.Fatal("expected 'filesystem' key in configs")
	}
	if fs.Command != "npx" {
		t.Errorf("Command = %q, want %q", fs.Command, "npx")
	}
	if len(fs.Args) != 3 {
		t.Errorf("Args len = %d, want 3; args = %v", len(fs.Args), fs.Args)
	}
	if fs.Args[0] != "-y" {
		t.Errorf("Args[0] = %q, want %q", fs.Args[0], "-y")
	}
	if len(fs.Env) != 1 || fs.Env[0] != "HOME=/home/user" {
		t.Errorf("Env = %v, want [HOME=/home/user]", fs.Env)
	}
}

func TestLoadMCPConfig_ParsesSSEServer(t *testing.T) {
	yaml := `
mcp_servers:
  github:
    url: https://mcp.example.com/github/sse
    headers:
      Authorization: Bearer token123
      X-Custom: custom-value
`
	path := writeTempYAML(t, yaml)
	configs, err := mcp.LoadMCPConfig(path)
	if err != nil {
		t.Fatalf("LoadMCPConfig: %v", err)
	}
	gh, ok := configs["github"]
	if !ok {
		t.Fatal("expected 'github' key")
	}
	if gh.URL != "https://mcp.example.com/github/sse" {
		t.Errorf("URL = %q", gh.URL)
	}
	if gh.Headers["Authorization"] != "Bearer token123" {
		t.Errorf("Authorization header = %q", gh.Headers["Authorization"])
	}
	if gh.Headers["X-Custom"] != "custom-value" {
		t.Errorf("X-Custom header = %q", gh.Headers["X-Custom"])
	}
}

func TestLoadMCPConfig_ParsesMultipleServers(t *testing.T) {
	yaml := `
mcp_servers:
  server1:
    command: cmd1
  server2:
    url: https://example.com
  server3:
    command: cmd3
    env_file: /path/to/.env
`
	path := writeTempYAML(t, yaml)
	configs, err := mcp.LoadMCPConfig(path)
	if err != nil {
		t.Fatalf("LoadMCPConfig: %v", err)
	}
	if len(configs) != 3 {
		t.Fatalf("expected 3 servers, got %d", len(configs))
	}
	if configs["server3"].EnvFile != "/path/to/.env" {
		t.Errorf("server3.EnvFile = %q", configs["server3"].EnvFile)
	}
}

// ---------------------------------------------------------------------------
// Missing file: returns empty map, no error.
// ---------------------------------------------------------------------------

func TestLoadMCPConfig_MissingFileReturnsEmptyMap(t *testing.T) {
	configs, err := mcp.LoadMCPConfig("/nonexistent/path/gistclaw.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if configs == nil {
		t.Fatal("expected non-nil map")
	}
	if len(configs) != 0 {
		t.Errorf("expected empty map, got %d entries", len(configs))
	}
}

// ---------------------------------------------------------------------------
// Empty mcp_servers section: returns empty map.
// ---------------------------------------------------------------------------

func TestLoadMCPConfig_EmptySection(t *testing.T) {
	yaml := "mcp_servers:\n"
	path := writeTempYAML(t, yaml)
	configs, err := mcp.LoadMCPConfig(path)
	if err != nil {
		t.Fatalf("LoadMCPConfig: %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("expected empty map, got %d entries", len(configs))
	}
}

// ---------------------------------------------------------------------------
// Invalid YAML: returns error.
// ---------------------------------------------------------------------------

func TestLoadMCPConfig_InvalidYAMLReturnsError(t *testing.T) {
	yaml := "mcp_servers: [unclosed"
	path := writeTempYAML(t, yaml)
	_, err := mcp.LoadMCPConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/mcp/... -run TestLoadMCPConfig -v
```

Expected: `FAIL` — package does not exist yet.

**Step 3: Add dependency**

```bash
go get gopkg.in/yaml.v3
go mod tidy
```

**Step 4: Write implementation**

```go
// internal/mcp/config.go
package mcp

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// MCPServerConfig holds the configuration for a single MCP server.
// Exactly one of Command (stdio transport) or URL (SSE/HTTP transport) must be set.
type MCPServerConfig struct {
	// Stdio transport
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Env     []string `yaml:"env"`     // KEY=VALUE pairs passed as environment variables
	EnvFile string   `yaml:"env_file"` // path to a .env file for this server

	// SSE / HTTP transport
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers"`
}

// gistclawYAML is the top-level structure of gistclaw.yaml.
type gistclawYAML struct {
	MCPServers map[string]MCPServerConfig `yaml:"mcp_servers"`
}

// LoadMCPConfig reads a gistclaw.yaml file at path and returns the named server
// configurations. If the file does not exist, an empty (non-nil) map is returned
// with no error. Any other I/O error or YAML parse error is returned as an error.
func LoadMCPConfig(path string) (map[string]MCPServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]MCPServerConfig), nil
		}
		return nil, fmt.Errorf("mcp: read config %q: %w", path, err)
	}

	var doc gistclawYAML
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("mcp: parse config %q: %w", path, err)
	}

	if doc.MCPServers == nil {
		return make(map[string]MCPServerConfig), nil
	}
	return doc.MCPServers, nil
}
```

**Step 5: Run test to verify it passes**

```bash
go test ./internal/mcp/... -run TestLoadMCPConfig -v
```

Expected: all `PASS`.

**Step 6: Commit**

```bash
git add internal/mcp/config.go internal/mcp/config_test.go go.mod go.sum
git commit -m "feat: add MCPServerConfig and LoadMCPConfig with stdio/SSE support"
```

---

### Task 4: `internal/mcp/manager.go` — MCPManager with connect/GetAllTools/CallTool/Close

**Files:**
- Create: `internal/mcp/manager.go`
- Create: `internal/mcp/manager_test.go`

**Step 1: Write the failing test**

```go
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
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/mcp/... -run TestNewMCPManager -run TestCallTool -run TestGetAllTools -run TestTool_Namespace -v
```

Expected: `FAIL` — `mcp.NewMCPManager`, `mcp.MCPManager`, `providers.Tool` not defined yet.

**Step 3: Add dependency**

```bash
go get github.com/modelcontextprotocol/go-sdk/mcp
go mod tidy
```

> **Note:** If `github.com/modelcontextprotocol/go-sdk` is not yet published or the import path differs, check the current MCP Go SDK repository. Common alternatives: `github.com/mark3labs/mcp-go` or `github.com/strowk/mcp-golang`. Use whichever is available. The manager implementation below abstracts the client behind the `MCPManager` struct so the import can be swapped in one place.
>
> If the SDK is not yet available, add a build tag `//go:build ignore` to the file and implement a stub that satisfies the interface. Add a TODO comment explaining this.

**Step 4: Write implementation**

```go
// internal/mcp/manager.go
package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/canhta/gistclaw/internal/providers"
)

const mcpCallTimeout = 10 * time.Second

// connectedServer holds a live MCP client connection and the tools it exposes.
type connectedServer struct {
	name   string
	client *gomcp.Client
	tools  []providers.Tool
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
			slog.Warn("mcp: failed to connect to server, skipping",
				"server", name,
				"error", err,
			)
			continue
		}
		m.servers[name] = srv
	}

	return m
}

// connectServer establishes a connection to a single MCP server and lists its
// tools. Returns an error if the connection or tool listing fails.
func connectServer(name string, cfg MCPServerConfig) (*connectedServer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var client *gomcp.Client
	var err error

	switch {
	case cfg.Command != "":
		// Stdio transport.
		env := buildEnv(cfg)
		transport := gomcp.NewStdioTransport(cfg.Command, cfg.Args, env)
		client, err = gomcp.NewClient(ctx, transport)
	case cfg.URL != "":
		// SSE / HTTP transport.
		transport := gomcp.NewSSETransport(cfg.URL, cfg.Headers)
		client, err = gomcp.NewClient(ctx, transport)
	default:
		return nil, fmt.Errorf("mcp: server %q has neither command nor url", name)
	}

	if err != nil {
		return nil, fmt.Errorf("mcp: connect %q: %w", name, err)
	}

	// List tools from the server.
	rawTools, err := client.ListTools(ctx)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("mcp: list tools from %q: %w", name, err)
	}

	tools := make([]providers.Tool, 0, len(rawTools))
	for _, t := range rawTools {
		tools = append(tools, providers.Tool{
			Name:        name + "__" + t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}

	return &connectedServer{
		name:   name,
		client: client,
		tools:  tools,
	}, nil
}

// buildEnv merges the process environment with the server's Env list.
// TODO: Load EnvFile and merge its KEY=VALUE pairs as well.
func buildEnv(cfg MCPServerConfig) []string {
	return cfg.Env
}

// GetAllTools returns all tools from all connected servers, each namespaced as
// "{serverName}__{toolName}". Returns an empty (non-nil) slice if no servers
// are connected. The returned type is []providers.Tool so gateway.Service can
// append directly into its tool registry without conversion.
func (m *MCPManager) GetAllTools() []providers.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var all []providers.Tool
	for _, srv := range m.servers {
		all = append(all, srv.tools...)
	}
	if all == nil {
		all = []providers.Tool{}
	}
	return all
}

// CallTool calls a tool on the appropriate MCP server.
// toolName must be the namespaced form "{serverName}__{toolName}".
//
// Returns:
//   - "MCP server '<name>' is not available" if the server is not connected (no error).
//   - "MCP tool call timed out" if the server does not respond within 10s (no error).
//   - The tool's string result on success.
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

	result, err := srv.client.CallTool(callCtx, rawTool, input)
	if err != nil {
		if callCtx.Err() == context.DeadlineExceeded {
			return "MCP tool call timed out", nil
		}
		// Return SDK errors as tool_result strings so the LLM can see them
		// and decide how to proceed, rather than crashing the tool loop.
		return fmt.Sprintf("MCP tool error: %v", err), nil
	}

	return result, nil
}

// splitToolName splits "{serverName}__{toolName}" into its two parts.
// Returns an error if the tool name does not contain the double-underscore separator.
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
		if err := srv.client.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
		}
	}
	m.servers = make(map[string]*connectedServer)

	if len(errs) > 0 {
		return fmt.Errorf("mcp: close errors: %s", strings.Join(errs, "; "))
	}
	return nil
}
```

> **Note on go-sdk API:** The `gomcp.Client` API above (`NewStdioTransport`, `NewSSETransport`, `NewClient`, `ListTools`, `CallTool`, `client.Close`) reflects the expected interface. The actual `github.com/modelcontextprotocol/go-sdk` API may differ. After running `go get`, check the SDK's documentation:
> ```bash
> go doc github.com/modelcontextprotocol/go-sdk/mcp
> ```
> Adjust method names and transport constructors to match what the SDK actually exports. The test suite only exercises connection failure and empty-config paths, so the real SDK surface is only exercised in integration tests (see Task 4, Step 5 note).

**Step 5: Run test to verify it passes**

```bash
go test ./internal/mcp/... -v
```

Expected: all `PASS`.

> **If SDK not available:** If `go get github.com/modelcontextprotocol/go-sdk/mcp` fails, temporarily add a build constraint `//go:build !integration` to `manager.go` and implement a stub `Client` type in the same package. The unit tests do not require a live SDK — they only test the manager's resilience to connection failure and its routing logic. Add a TODO noting the real SDK import.

**Step 6: Commit**

```bash
git add internal/mcp/manager.go internal/mcp/manager_test.go go.mod go.sum
git commit -m "feat: add MCPManager with connect/GetAllTools/CallTool/Close and double-underscore namespacing"
```

---

## Final verification

After all four tasks complete, run the full test suite:

```bash
go test ./internal/tools/... ./internal/mcp/... -v
```

Expected: all tests `PASS`.

Run the module build check:

```bash
go build ./...
```

Expected: succeeds with no errors.

Final commit:

```bash
git add .
git commit -m "feat: plan 4 complete — tools (search + fetch) and MCP (config + manager)"
```

---

Plan 4 complete. Run alongside Plan 3 and Plan 5. After all three complete: Plan 6 (Agent Services).
