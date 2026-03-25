package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

type WebFetchTool struct {
	client   *http.Client
	maxBytes int64
}

func NewWebFetchTool(client *http.Client, maxBytes int64) *WebFetchTool {
	if client == nil {
		client = newBoundedHTTPClient(10 * time.Second)
	}
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	return &WebFetchTool{client: client, maxBytes: maxBytes}
}

func (t *WebFetchTool) Name() string { return "web_fetch" }

func (t *WebFetchTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Fetch one HTTP or HTTPS URL and return status, content type, readable text, and truncation metadata.",
		InputSchemaJSON: `{"type":"object","properties":{"url":{"type":"string","format":"uri"}},"required":["url"]}`,
		Risk:            model.RiskLow,
		SideEffect:      "none",
		Approval:        "never",
	}
}

func (t *WebFetchTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	var input struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("web_fetch: decode input: %w", err)
	}
	target, err := url.Parse(strings.TrimSpace(input.URL))
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("web_fetch: parse url: %w", err)
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return model.ToolResult{}, fmt.Errorf("web_fetch: unsupported scheme %q", target.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("web_fetch: build request: %w", err)
	}
	req.Header.Set("User-Agent", defaultResearchUserAgent)

	resp, err := t.client.Do(req)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("web_fetch: http: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, t.maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("web_fetch: read body: %w", err)
	}
	truncated := int64(len(body)) > t.maxBytes
	if truncated {
		body = body[:t.maxBytes]
	}

	contentType := resp.Header.Get("Content-Type")
	text := string(body)
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		text = extractReadableHTMLText(text)
	}
	if len(text) > int(t.maxBytes) {
		text = text[:t.maxBytes]
		truncated = true
	}

	output, err := json.Marshal(struct {
		URL         string `json:"url"`
		StatusCode  int    `json:"status_code"`
		ContentType string `json:"content_type"`
		Truncated   bool   `json:"truncated"`
		Text        string `json:"text"`
	}{
		URL:         target.String(),
		StatusCode:  resp.StatusCode,
		ContentType: contentType,
		Truncated:   truncated,
		Text:        text,
	})
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("web_fetch: encode output: %w", err)
	}
	return model.ToolResult{Output: string(output)}, nil
}

var (
	scriptStylePattern = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	tagPattern         = regexp.MustCompile(`(?s)<[^>]+>`)
	spacePattern       = regexp.MustCompile(`\s+`)
)

func extractReadableHTMLText(raw string) string {
	withoutScripts := scriptStylePattern.ReplaceAllString(raw, " ")
	withLineHints := strings.NewReplacer("</p>", "\n", "</div>", "\n", "</h1>", "\n", "</h2>", "\n", "</h3>", "\n", "<br>", "\n", "<br/>", "\n", "<br />", "\n").Replace(withoutScripts)
	withoutTags := tagPattern.ReplaceAllString(withLineHints, " ")
	unescaped := html.UnescapeString(withoutTags)
	normalized := spacePattern.ReplaceAllString(unescaped, " ")
	return strings.TrimSpace(normalized)
}
