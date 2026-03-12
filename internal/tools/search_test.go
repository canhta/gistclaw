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

func TestNewSearchProvider_FallsToOpenRouter(t *testing.T) {
	cfg := config.Config{OpenRouterAPIKey: "or-key"}
	p := tools.NewSearchProvider(cfg)
	if p == nil || p.Name() != "openrouter" {
		t.Errorf("expected openrouter provider, got %v", p)
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

// ---------------------------------------------------------------------------
// Gemini backend: returns synthesised text snippet.
// ---------------------------------------------------------------------------

func TestGeminiSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{
							{"text": "Golang is an open-source language."},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := config.Config{GeminiAPIKey: "gemini-key"}
	p := tools.NewSearchProviderWithBaseURL(cfg, map[string]string{"gemini": srv.URL})
	if p == nil {
		t.Fatal("expected non-nil provider")
	}

	results, err := p.Search(context.Background(), "golang", 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Snippet == "" {
		t.Errorf("expected non-empty snippet, got empty")
	}
	if results[0].Snippet != "Golang is an open-source language." {
		t.Errorf("results[0].Snippet = %q", results[0].Snippet)
	}
}

// ---------------------------------------------------------------------------
// xAI backend: returns synthesised text snippet.
// ---------------------------------------------------------------------------

func TestXAISearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "xAI search result about golang"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := config.Config{XAIAPIKey: "xai-key"}
	p := tools.NewSearchProviderWithBaseURL(cfg, map[string]string{"xai": srv.URL})
	if p == nil {
		t.Fatal("expected non-nil provider")
	}

	results, err := p.Search(context.Background(), "golang", 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Snippet != "xAI search result about golang" {
		t.Errorf("results[0].Snippet = %q", results[0].Snippet)
	}
}
