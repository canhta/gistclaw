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
	fetchBodyLimit  = 5 * 1024 * 1024 // 5 MB
	fetchTimeout    = 30 * time.Second
	fetchChromeUA   = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	fetchFallbackUA = "gistclaw"
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

// doFetch performs a single HTTP GET. Returns:
//   - (content, 0, false, nil) on success
//   - ("", statusCode, cfBlocked, nil) on non-2xx
//   - ("Page too large (> 5 MB)", 0, false, nil) if body exceeds limit
//   - ("", 0, false, err) on network/parse error
//
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
	defer resp.Body.Close() //nolint:errcheck

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
