package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/canhta/gistclaw/internal/logstream"
)

type logsIndexResponse struct {
	Summary   logsSummaryResponse `json:"summary"`
	Filters   logsFilterResponse  `json:"filters"`
	Sources   []string            `json:"sources"`
	StreamURL string              `json:"stream_url"`
	Entries   []logEntryResponse  `json:"entries"`
}

type logsSummaryResponse struct {
	BufferedEntries int `json:"buffered_entries"`
	VisibleEntries  int `json:"visible_entries"`
	ErrorEntries    int `json:"error_entries"`
	WarningEntries  int `json:"warning_entries"`
}

type logsFilterResponse struct {
	Query  string `json:"query"`
	Level  string `json:"level"`
	Source string `json:"source"`
	Limit  int    `json:"limit"`
}

type logEntryResponse struct {
	ID             int64  `json:"id"`
	Source         string `json:"source"`
	Level          string `json:"level"`
	LevelLabel     string `json:"level_label"`
	Message        string `json:"message"`
	Raw            string `json:"raw"`
	CreatedAtLabel string `json:"created_at_label"`
}

func (s *Server) handleLogsIndex(w http.ResponseWriter, r *http.Request) {
	snapshot := logsSnapshot(s.logs, parseLogsQuery(r.URL.Query()))
	resp := buildLogsIndexResponse(snapshot)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleLogsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	query := parseLogsQuery(r.URL.Query())

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if s.logs == nil {
		<-r.Context().Done()
		return
	}

	sub := s.logs.Subscribe(query)
	defer s.logs.Unsubscribe(sub)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case entry := <-sub:
			if err := writeLogEntry(w, flusher, entry); err != nil {
				return
			}
		}
	}
}

func logsSnapshot(logs *logstream.Sink, query logstream.Query) logstream.Snapshot {
	if logs == nil {
		return logstream.Snapshot{
			BufferedEntries: 0,
			Sources:         []string{},
			Entries:         []logstream.Entry{},
			Query:           normalizeLogsQuery(query),
		}
	}
	return logs.Snapshot(query)
}

func buildLogsIndexResponse(snapshot logstream.Snapshot) logsIndexResponse {
	resp := logsIndexResponse{
		Summary: logsSummaryResponse{
			BufferedEntries: snapshot.BufferedEntries,
			VisibleEntries:  len(snapshot.Entries),
		},
		Filters: logsFilterResponse{
			Query:  snapshot.Query.Query,
			Level:  snapshot.Query.Level,
			Source: snapshot.Query.Source,
			Limit:  snapshot.Query.Limit,
		},
		Sources:   append([]string(nil), snapshot.Sources...),
		StreamURL: logsStreamPath(snapshot.Query),
		Entries:   make([]logEntryResponse, 0, len(snapshot.Entries)),
	}
	for _, entry := range snapshot.Entries {
		if entry.Level == logstream.LevelError {
			resp.Summary.ErrorEntries++
		}
		if entry.Level == logstream.LevelWarn {
			resp.Summary.WarningEntries++
		}
		resp.Entries = append(resp.Entries, buildLogEntryResponse(entry))
	}
	return resp
}

func buildLogEntryResponse(entry logstream.Entry) logEntryResponse {
	return logEntryResponse{
		ID:             entry.ID,
		Source:         entry.Source,
		Level:          entry.Level,
		LevelLabel:     humanizeWebLabel(entry.Level),
		Message:        entry.Message,
		Raw:            entry.Raw,
		CreatedAtLabel: formatWebTimestamp(entry.CreatedAt),
	}
}

func parseLogsQuery(values url.Values) logstream.Query {
	limit := 200
	if rawLimit := strings.TrimSpace(values.Get("limit")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil {
			limit = parsed
		}
	}
	return normalizeLogsQuery(logstream.Query{
		Query:  strings.TrimSpace(values.Get("q")),
		Level:  strings.TrimSpace(values.Get("level")),
		Source: strings.TrimSpace(values.Get("source")),
		Limit:  limit,
	})
}

func normalizeLogsQuery(query logstream.Query) logstream.Query {
	normalized := logstream.Query{
		Query:  strings.TrimSpace(query.Query),
		Level:  strings.TrimSpace(strings.ToLower(query.Level)),
		Source: strings.TrimSpace(strings.ToLower(query.Source)),
		Limit:  query.Limit,
	}
	if normalized.Level == "" {
		normalized.Level = logstream.LevelAll
	}
	switch normalized.Level {
	case logstream.LevelAll, logstream.LevelInfo, logstream.LevelWarn, logstream.LevelError:
	default:
		normalized.Level = logstream.LevelAll
	}
	if normalized.Source == "" {
		normalized.Source = "all"
	}
	if normalized.Limit <= 0 {
		normalized.Limit = 200
	}
	if normalized.Limit > 500 {
		normalized.Limit = 500
	}
	return normalized
}

func logsStreamPath(query logstream.Query) string {
	values := make([]string, 0, 4)
	if query.Query != "" {
		values = append(values, "q="+url.QueryEscape(query.Query))
	}
	if query.Level != "" && query.Level != logstream.LevelAll {
		values = append(values, "level="+url.QueryEscape(query.Level))
	}
	if query.Source != "" && query.Source != "all" {
		values = append(values, "source="+url.QueryEscape(query.Source))
	}
	if query.Limit > 0 {
		values = append(values, "limit="+url.QueryEscape(strconv.Itoa(query.Limit)))
	}
	if len(values) > 0 {
		return "/api/logs/stream?" + strings.Join(values, "&")
	}
	return "/api/logs/stream"
}

func writeLogEntry(w http.ResponseWriter, flusher http.Flusher, entry logstream.Entry) error {
	payload, err := json.Marshal(buildLogEntryResponse(entry))
	if err != nil {
		return nil
	}
	if _, err := w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
