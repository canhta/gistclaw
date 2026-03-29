package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDebugEventsReturnsSelectedRunEventBoard(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	h.insertRunAt(
		t,
		"run-debug-root",
		"conv-debug-root",
		"Repair connector backlog",
		"active",
		"2026-03-29 09:00:00",
	)
	h.insertRunAt(
		t,
		"run-debug-worker",
		"conv-debug-worker",
		"Collect connector evidence",
		"needs_approval",
		"2026-03-29 09:05:00",
	)
	h.insertEventAt(
		t,
		"evt-root-start",
		"conv-debug-root",
		"run-debug-root",
		"run_started",
		"2026-03-29 09:00:00",
	)
	h.insertEventAtWithPayload(
		t,
		"evt-worker-tool",
		"conv-debug-worker",
		"run-debug-worker",
		"tool_started",
		[]byte(`{"tool_name":"connector_send","target":"telegram"}`),
		"2026-03-29 09:06:00",
	)
	h.insertEventAtWithPayload(
		t,
		"evt-worker-approval",
		"conv-debug-worker",
		"run-debug-worker",
		"approval_requested",
		[]byte(`{"tool_name":"system.run","reason":"needs exec approval"}`),
		"2026-03-29 09:07:00",
	)

	otherRoot := t.TempDir()
	h.insertRunInWorkspace(
		t,
		"run-other-project-events",
		"conv-other-project-events",
		"Other project run",
		"active",
		otherRoot,
	)
	h.insertEventAt(
		t,
		"evt-other-project",
		"conv-other-project-events",
		"run-other-project-events",
		"run_started",
		"2026-03-29 09:08:00",
	)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/debug/events?run_id=run-debug-worker", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Summary struct {
			SourceCount        int    `json:"source_count"`
			EventCount         int    `json:"event_count"`
			SelectedRunID      string `json:"selected_run_id"`
			LatestEventLabel   string `json:"latest_event_label"`
			LatestEventAtLabel string `json:"latest_event_at_label"`
		} `json:"summary"`
		Filters struct {
			RunID string `json:"run_id"`
			Limit int    `json:"limit"`
		} `json:"filters"`
		Sources []struct {
			RunID              string `json:"run_id"`
			Objective          string `json:"objective"`
			StatusLabel        string `json:"status_label"`
			EventCount         int    `json:"event_count"`
			LatestEventAtLabel string `json:"latest_event_at_label"`
			StreamURL          string `json:"stream_url"`
		} `json:"sources"`
		Events []struct {
			ID              string `json:"id"`
			RunID           string `json:"run_id"`
			RunShortID      string `json:"run_short_id"`
			Objective       string `json:"objective"`
			Kind            string `json:"kind"`
			KindLabel       string `json:"kind_label"`
			PayloadPreview  string `json:"payload_preview"`
			OccurredAtLabel string `json:"occurred_at_label"`
		} `json:"events"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode debug events response: %v", err)
	}

	if resp.Summary.SourceCount != 2 || resp.Summary.EventCount != 2 {
		t.Fatalf("unexpected summary counts: %+v", resp.Summary)
	}
	if resp.Summary.SelectedRunID != "run-debug-worker" ||
		resp.Summary.LatestEventLabel != "Approval Requested" {
		t.Fatalf("unexpected summary selection: %+v", resp.Summary)
	}
	if resp.Filters.RunID != "run-debug-worker" || resp.Filters.Limit != 20 {
		t.Fatalf("unexpected filters: %+v", resp.Filters)
	}
	if len(resp.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(resp.Sources))
	}
	if resp.Sources[0].RunID != "run-debug-worker" ||
		resp.Sources[0].StreamURL != "/api/work/run-debug-worker/events" {
		t.Fatalf("unexpected first source: %+v", resp.Sources[0])
	}
	if len(resp.Events) != 2 {
		t.Fatalf("expected 2 selected events, got %d", len(resp.Events))
	}
	if resp.Events[0].ID != "evt-worker-approval" ||
		resp.Events[0].KindLabel != "Approval Requested" {
		t.Fatalf("unexpected first event: %+v", resp.Events[0])
	}
	if resp.Events[0].PayloadPreview == "" || resp.Events[1].PayloadPreview == "" {
		t.Fatalf("expected payload previews, got %+v", resp.Events)
	}
	for _, item := range resp.Events {
		if item.RunID == "run-other-project-events" || item.Objective == "Other project run" {
			t.Fatalf("expected other project event to be hidden, got %+v", item)
		}
	}
}
