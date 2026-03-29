package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLogsIndexReturnsSummaryAndFilteredEntries(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	h.logs.Append("web", "info", "request method=GET path=/api/work")
	h.logs.Append("scheduler", "warn", "missed occurrence schedule_id=daily-digest")
	h.logs.Append("runtime", "error", "provider call failed")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/logs?q=failed&level=error&limit=50", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Summary struct {
			BufferedEntries int `json:"buffered_entries"`
			VisibleEntries  int `json:"visible_entries"`
			ErrorEntries    int `json:"error_entries"`
			WarningEntries  int `json:"warning_entries"`
		} `json:"summary"`
		Filters struct {
			Query  string `json:"query"`
			Level  string `json:"level"`
			Source string `json:"source"`
			Limit  int    `json:"limit"`
		} `json:"filters"`
		Sources   []string `json:"sources"`
		StreamURL string   `json:"stream_url"`
		Entries   []struct {
			ID             int64  `json:"id"`
			Source         string `json:"source"`
			Level          string `json:"level"`
			LevelLabel     string `json:"level_label"`
			Message        string `json:"message"`
			Raw            string `json:"raw"`
			CreatedAtLabel string `json:"created_at_label"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode logs response: %v", err)
	}

	if resp.Summary.BufferedEntries != 3 {
		t.Fatalf("buffered_entries = %d, want 3", resp.Summary.BufferedEntries)
	}
	if resp.Summary.VisibleEntries != 1 {
		t.Fatalf("visible_entries = %d, want 1", resp.Summary.VisibleEntries)
	}
	if resp.Summary.ErrorEntries != 1 || resp.Summary.WarningEntries != 0 {
		t.Fatalf("unexpected summary counts: %+v", resp.Summary)
	}
	if resp.Filters.Query != "failed" || resp.Filters.Level != "error" || resp.Filters.Source != "all" {
		t.Fatalf("unexpected filters: %+v", resp.Filters)
	}
	if resp.Filters.Limit != 50 {
		t.Fatalf("limit = %d, want 50", resp.Filters.Limit)
	}
	if resp.StreamURL != "/api/logs/stream?q=failed&level=error&limit=50" {
		t.Fatalf("stream_url = %q", resp.StreamURL)
	}
	if len(resp.Sources) != 3 {
		t.Fatalf("expected 3 sources, got %d (%v)", len(resp.Sources), resp.Sources)
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(resp.Entries))
	}
	if resp.Entries[0].Source != "runtime" || resp.Entries[0].Level != "error" {
		t.Fatalf("unexpected entry metadata: %+v", resp.Entries[0])
	}
	if resp.Entries[0].LevelLabel != "Error" {
		t.Fatalf("level_label = %q, want Error", resp.Entries[0].LevelLabel)
	}
	if resp.Entries[0].Message != "provider call failed" {
		t.Fatalf("message = %q, want %q", resp.Entries[0].Message, "provider call failed")
	}
	if resp.Entries[0].CreatedAtLabel == "" {
		t.Fatalf("expected created_at_label to be populated")
	}
}

func TestLogsStreamEmitsMatchingEntries(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)

	ts := httptest.NewServer(h.server)
	defer ts.Close()

	resp, reader := subscribeSSE(t, ts.URL+"/api/logs/stream?source=web&level=warn")
	defer resp.Body.Close()

	waitForLogSubscribers(t, h.logs, 1)

	h.logs.Append("runtime", "warn", "provider backoff")
	h.logs.Append("web", "info", "request method=GET path=/api/work")
	h.logs.Append("web", "warn", "panic method=GET path=/api/debug err=boom")

	event := readSSEEventWithin(t, resp, reader, time.Second)
	if got := event["source"]; got != "web" {
		t.Fatalf("expected source web, got %#v", got)
	}
	if got := event["level"]; got != "warn" {
		t.Fatalf("expected level warn, got %#v", got)
	}
	if got := event["message"]; got != "panic method=GET path=/api/debug err=boom" {
		t.Fatalf("unexpected message %#v", got)
	}
}

func waitForLogSubscribers(t *testing.T, logs interface{ SubscriberCount() int }, want int) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if logs.SubscriberCount() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("subscriber count never reached %d", want)
}

func TestLogsStreamSubscriberDropsAfterDisconnect(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)

	ts := httptest.NewServer(h.server)
	defer ts.Close()

	resp, _ := subscribeSSE(t, ts.URL+"/api/logs/stream")
	waitForLogSubscribers(t, h.logs, 1)
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if h.logs.SubscriberCount() == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("expected log subscribers to drain after disconnect")
}

func TestLogsStreamUsesStructuredJSON(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)

	ts := httptest.NewServer(h.server)
	defer ts.Close()

	resp, reader := subscribeSSE(t, ts.URL+"/api/logs/stream")
	defer resp.Body.Close()

	waitForLogSubscribers(t, h.logs, 1)
	h.logs.Append("runtime", "error", "provider call failed")

	event := readSSEEventWithin(t, resp, reader, time.Second)
	if _, ok := event["id"].(float64); !ok {
		t.Fatalf("expected numeric id in payload, got %#v", event["id"])
	}
	if got := event["level_label"]; got != "Error" {
		t.Fatalf("expected level_label Error, got %#v", got)
	}
	if got := event["raw"]; got == nil || got == "" {
		t.Fatalf("expected raw line in payload, got %#v", got)
	}
}

func TestLogsStreamCanEmitAfterSubscribe(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)

	ts := httptest.NewServer(h.server)
	defer ts.Close()

	resp, reader := subscribeSSE(t, ts.URL+"/api/logs/stream?source=scheduler")
	defer resp.Body.Close()

	waitForLogSubscribers(t, h.logs, 1)
	if err := h.logs.Emit(context.Background(), "scheduler", "warn", "repair delayed"); err != nil {
		t.Fatalf("emit log entry: %v", err)
	}

	event := readSSEEventWithin(t, resp, reader, time.Second)
	if got := event["source"]; got != "scheduler" {
		t.Fatalf("expected source scheduler, got %#v", got)
	}
}
