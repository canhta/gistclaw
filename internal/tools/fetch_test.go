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
