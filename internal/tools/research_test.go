package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

type fakeSearchBackend struct {
	results   []SearchResult
	err       error
	lastQuery string
	lastLimit int
	callCount int
}

func (f *fakeSearchBackend) Search(_ context.Context, query string, maxResults int) ([]SearchResult, error) {
	f.lastQuery = query
	f.lastLimit = maxResults
	f.callCount++
	return f.results, f.err
}

func TestWebSearch_RejectsEmptyQuery(t *testing.T) {
	tool := NewWebSearchTool(&fakeSearchBackend{}, ResearchConfig{
		Provider:   "tavily",
		APIKey:     "tvly-test",
		MaxResults: 5,
	})

	_, err := tool.Invoke(context.Background(), model.ToolCall{
		ID:        "call-1",
		ToolName:  "web_search",
		InputJSON: []byte(`{"query":"   "}`),
	})
	if err == nil {
		t.Fatal("expected empty query to fail")
	}
}

func TestWebSearch_ReturnsNormalizedResultObjects(t *testing.T) {
	backend := &fakeSearchBackend{
		results: []SearchResult{
			{Title: "Result One", URL: "https://example.com/one", Snippet: "first"},
			{Title: "Result Two", URL: "https://example.com/two", Snippet: "second"},
		},
	}
	tool := NewWebSearchTool(backend, ResearchConfig{
		Provider:   "tavily",
		APIKey:     "tvly-test",
		MaxResults: 3,
	})

	got, err := tool.Invoke(context.Background(), model.ToolCall{
		ID:        "call-2",
		ToolName:  "web_search",
		InputJSON: []byte(`{"query":"gistclaw","max_results":2}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if backend.lastQuery != "gistclaw" {
		t.Fatalf("expected query to round-trip, got %q", backend.lastQuery)
	}
	if backend.lastLimit != 2 {
		t.Fatalf("expected max_results=2, got %d", backend.lastLimit)
	}

	var payload struct {
		Query   string         `json:"query"`
		Results []SearchResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.Query != "gistclaw" {
		t.Fatalf("unexpected query %q", payload.Query)
	}
	if len(payload.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(payload.Results))
	}
	if payload.Results[0].Title != "Result One" || payload.Results[1].URL != "https://example.com/two" {
		t.Fatalf("unexpected normalized results: %+v", payload.Results)
	}
}

func TestWebFetch_RejectsUnsupportedScheme(t *testing.T) {
	tool := NewWebFetchTool(newBoundedHTTPClient(2*time.Second), 1024)

	_, err := tool.Invoke(context.Background(), model.ToolCall{
		ID:        "call-fetch-1",
		ToolName:  "web_fetch",
		InputJSON: []byte(`{"url":"ftp://example.com/file.txt"}`),
	})
	if err == nil {
		t.Fatal("expected unsupported scheme to fail")
	}
}

func TestWebFetch_TruncatesOversizedBodies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(strings.Repeat("a", 4096)))
	}))
	defer srv.Close()

	tool := NewWebFetchTool(newBoundedHTTPClient(2*time.Second), 256)
	got, err := tool.Invoke(context.Background(), model.ToolCall{
		ID:        "call-fetch-2",
		ToolName:  "web_fetch",
		InputJSON: []byte(`{"url":"` + srv.URL + `"}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var payload struct {
		Truncated bool   `json:"truncated"`
		Text      string `json:"text"`
	}
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !payload.Truncated {
		t.Fatal("expected truncated=true")
	}
	if len(payload.Text) > 256 {
		t.Fatalf("expected text to be truncated to limit, got %d bytes", len(payload.Text))
	}
}

func TestWebFetch_ExtractsReadableTextFromHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><h1>Hello</h1><p>world</p><script>ignore()</script></body></html>`))
	}))
	defer srv.Close()

	tool := NewWebFetchTool(newBoundedHTTPClient(2*time.Second), 4096)
	got, err := tool.Invoke(context.Background(), model.ToolCall{
		ID:        "call-fetch-3",
		ToolName:  "web_fetch",
		InputJSON: []byte(`{"url":"` + srv.URL + `"}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !strings.Contains(payload.Text, "Hello") || !strings.Contains(payload.Text, "world") {
		t.Fatalf("expected readable text, got %q", payload.Text)
	}
	if strings.Contains(payload.Text, "<h1>") || strings.Contains(payload.Text, "ignore()") {
		t.Fatalf("expected markup/scripts stripped, got %q", payload.Text)
	}
}

func TestTavilySearchBackend_Search(t *testing.T) {
	var captured struct {
		APIKey     string `json:"api_key"`
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"Repo","url":"https://example.com","content":"snippet"}]}`))
	}))
	defer srv.Close()

	backend := &tavilySearchBackend{
		apiKey:   "tvly-test",
		endpoint: srv.URL,
		client:   newBoundedHTTPClient(2 * time.Second),
	}

	results, err := backend.Search(context.Background(), "gistclaw", 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if captured.APIKey != "tvly-test" || captured.Query != "gistclaw" || captured.MaxResults != 3 {
		t.Fatalf("unexpected request payload: %+v", captured)
	}
	if len(results) != 1 || results[0].Snippet != "snippet" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestBuildRegistry_RegistersDefaultWebFetchTool(t *testing.T) {
	reg, closer, err := BuildRegistry(context.Background(), BuildOptions{})
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	if closer != nil {
		defer closer.Close()
	}
	if _, ok := reg.Get("web_fetch"); !ok {
		t.Fatal("expected web_fetch to always be registered")
	}
	if _, ok := reg.Get("web_search"); ok {
		t.Fatal("expected web_search to stay hidden without research config")
	}
}

func TestBuildRegistry_RejectsUnknownResearchProvider(t *testing.T) {
	_, _, err := BuildRegistry(context.Background(), BuildOptions{
		Research: ResearchConfig{Provider: "bing", APIKey: "key"},
	})
	if err == nil {
		t.Fatal("expected unknown research provider to fail")
	}
}
