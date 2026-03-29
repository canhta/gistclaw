package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/runtime"
)

type debugEventsResponse struct {
	Summary debugEventsSummaryResponse `json:"summary"`
	Filters debugEventsFiltersResponse `json:"filters"`
	Sources []debugEventSourceResponse `json:"sources"`
	Events  []debugEventEntryResponse  `json:"events"`
}

type debugEventsSummaryResponse struct {
	SourceCount        int    `json:"source_count"`
	EventCount         int    `json:"event_count"`
	SelectedRunID      string `json:"selected_run_id,omitempty"`
	LatestEventLabel   string `json:"latest_event_label,omitempty"`
	LatestEventAtLabel string `json:"latest_event_at_label,omitempty"`
}

type debugEventsFiltersResponse struct {
	RunID string `json:"run_id,omitempty"`
	Limit int    `json:"limit"`
}

type debugEventSourceResponse struct {
	RunID              string `json:"run_id"`
	Objective          string `json:"objective"`
	AgentID            string `json:"agent_id"`
	Status             string `json:"status"`
	StatusLabel        string `json:"status_label"`
	EventCount         int    `json:"event_count"`
	LatestEventAtLabel string `json:"latest_event_at_label,omitempty"`
	StreamURL          string `json:"stream_url"`
}

type debugEventEntryResponse struct {
	ID              string `json:"id"`
	RunID           string `json:"run_id"`
	RunShortID      string `json:"run_short_id"`
	Objective       string `json:"objective"`
	AgentID         string `json:"agent_id"`
	Kind            string `json:"kind"`
	KindLabel       string `json:"kind_label"`
	PayloadPreview  string `json:"payload_preview"`
	OccurredAt      string `json:"occurred_at"`
	OccurredAtLabel string `json:"occurred_at_label"`
}

type debugEventsFilter struct {
	RunID string
	Limit int
}

type debugEventSourceRow struct {
	RunID         string
	Objective     string
	AgentID       string
	Status        string
	EventCount    int
	LatestEventAt string
}

type debugEventEntryRow struct {
	ID         string
	RunID      string
	Objective  string
	AgentID    string
	Kind       string
	PayloadRaw []byte
	CreatedAt  string
}

func (s *Server) handleDebugEventsIndex(w http.ResponseWriter, r *http.Request) {
	project, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load active project", http.StatusInternalServerError)
		return
	}

	filter := parseDebugEventsFilter(r.URL.Query())
	sources, err := s.loadDebugEventSources(r.Context(), project.ID, 8)
	if err != nil {
		http.Error(w, "failed to load debug event sources", http.StatusInternalServerError)
		return
	}

	selectedRunID := selectDebugEventsRun(filter.RunID, sources)
	events, err := s.loadDebugEventEntries(r.Context(), project.ID, selectedRunID, filter.Limit)
	if err != nil {
		http.Error(w, "failed to load debug events", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, debugEventsResponse{
		Summary: buildDebugEventsSummary(sources, events, selectedRunID),
		Filters: debugEventsFiltersResponse{
			RunID: selectedRunID,
			Limit: filter.Limit,
		},
		Sources: sources,
		Events:  events,
	})
}

func parseDebugEventsFilter(values url.Values) debugEventsFilter {
	limit := 20
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	return debugEventsFilter{
		RunID: strings.TrimSpace(values.Get("run_id")),
		Limit: limit,
	}
}

func (s *Server) loadDebugEventSources(
	ctx context.Context,
	projectID string,
	limit int,
) ([]debugEventSourceResponse, error) {
	if limit <= 0 {
		limit = 8
	}

	rows, err := s.db.RawDB().QueryContext(
		ctx,
		`SELECT runs.id,
		        COALESCE(runs.objective, ''),
		        COALESCE(runs.agent_id, ''),
		        runs.status,
		        COUNT(events.id) AS event_count,
		        COALESCE(MAX(events.created_at), '')
		   FROM runs
		   JOIN events ON events.run_id = runs.id
		  WHERE runs.project_id = ?
		  GROUP BY runs.id, runs.objective, runs.agent_id, runs.status
		  ORDER BY MAX(events.created_at) DESC, runs.id DESC
		  LIMIT ?`,
		projectID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query debug event sources: %w", err)
	}
	defer rows.Close()

	resp := make([]debugEventSourceResponse, 0, limit)
	for rows.Next() {
		var item debugEventSourceRow
		if err := rows.Scan(
			&item.RunID,
			&item.Objective,
			&item.AgentID,
			&item.Status,
			&item.EventCount,
			&item.LatestEventAt,
		); err != nil {
			return nil, fmt.Errorf("scan debug event source: %w", err)
		}
		resp = append(resp, debugEventSourceResponse{
			RunID:              item.RunID,
			Objective:          item.Objective,
			AgentID:            item.AgentID,
			Status:             item.Status,
			StatusLabel:        humanizeWebLabel(item.Status),
			EventCount:         item.EventCount,
			LatestEventAtLabel: formatRunTimestamp(parseRunListTimestamp(item.LatestEventAt)),
			StreamURL:          runEventsPath(item.RunID),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate debug event sources: %w", err)
	}
	return resp, nil
}

func selectDebugEventsRun(requested string, sources []debugEventSourceResponse) string {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		for _, source := range sources {
			if source.RunID == requested {
				return requested
			}
		}
	}
	if len(sources) == 0 {
		return ""
	}
	return sources[0].RunID
}

func (s *Server) loadDebugEventEntries(
	ctx context.Context,
	projectID string,
	runID string,
	limit int,
) ([]debugEventEntryResponse, error) {
	if strings.TrimSpace(runID) == "" {
		return []debugEventEntryResponse{}, nil
	}
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.RawDB().QueryContext(
		ctx,
		`SELECT events.id,
		        events.run_id,
		        COALESCE(runs.objective, ''),
		        COALESCE(runs.agent_id, ''),
		        events.kind,
		        COALESCE(events.payload_json, x''),
		        events.created_at
		   FROM events
		   JOIN runs ON runs.id = events.run_id
		  WHERE runs.project_id = ?
		    AND events.run_id = ?
		  ORDER BY events.created_at DESC, events.id DESC
		  LIMIT ?`,
		projectID,
		runID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query debug events: %w", err)
	}
	defer rows.Close()

	resp := make([]debugEventEntryResponse, 0, limit)
	for rows.Next() {
		var item debugEventEntryRow
		if err := rows.Scan(
			&item.ID,
			&item.RunID,
			&item.Objective,
			&item.AgentID,
			&item.Kind,
			&item.PayloadRaw,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan debug event: %w", err)
		}
		occurredAt := parseRunListTimestamp(item.CreatedAt)
		resp = append(resp, debugEventEntryResponse{
			ID:              item.ID,
			RunID:           item.RunID,
			RunShortID:      compactIdentifier(item.RunID),
			Objective:       item.Objective,
			AgentID:         item.AgentID,
			Kind:            item.Kind,
			KindLabel:       humanizeWebLabel(item.Kind),
			PayloadPreview:  debugEventPayloadPreview(item.PayloadRaw),
			OccurredAt:      occurredAt.UTC().Format(time.RFC3339),
			OccurredAtLabel: formatRunTimestamp(occurredAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate debug events: %w", err)
	}
	return resp, nil
}

func buildDebugEventsSummary(
	sources []debugEventSourceResponse,
	events []debugEventEntryResponse,
	selectedRunID string,
) debugEventsSummaryResponse {
	resp := debugEventsSummaryResponse{
		SourceCount:   len(sources),
		EventCount:    len(events),
		SelectedRunID: selectedRunID,
	}
	if len(events) > 0 {
		resp.LatestEventLabel = events[0].KindLabel
		resp.LatestEventAtLabel = events[0].OccurredAtLabel
	}
	return resp
}

func debugEventPayloadPreview(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return "No payload"
	}

	if json.Valid([]byte(trimmed)) {
		var compact bytes.Buffer
		if err := json.Compact(&compact, []byte(trimmed)); err == nil {
			trimmed = compact.String()
		}
	}

	const limit = 160
	if len(trimmed) <= limit {
		return trimmed
	}
	return trimmed[:limit-1] + "…"
}
