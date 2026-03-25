package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

type runListItem struct {
	ID        string
	Objective string
	Summary   string
	Status    string
}

type runsPageData struct {
	Runs []runListItem
}

type runDetailPageData struct {
	RunID     string
	Status    string
	StreamURL string
	Turns     []runTurnView
	Events    []runEventView
}

type runEventView struct {
	Kind      string
	CreatedAt time.Time
}

type runTurnView struct {
	Content   string
	CreatedAt time.Time
}

func (s *Server) handleRunsIndex(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.RawDB().QueryContext(r.Context(),
		`SELECT id, COALESCE(objective, ''), COALESCE(parent_run_id, ''), status,
		        (SELECT count(*) FROM runs child WHERE child.parent_run_id = runs.id) AS worker_count
		 FROM runs
		 ORDER BY created_at DESC`,
	)
	if err != nil {
		http.Error(w, "failed to load runs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	runs := make([]runListItem, 0)
	for rows.Next() {
		var item runListItem
		var parentRunID string
		var workerCount int
		if err := rows.Scan(&item.ID, &item.Objective, &parentRunID, &item.Status, &workerCount); err != nil {
			http.Error(w, "failed to load runs", http.StatusInternalServerError)
			return
		}
		item.Summary = formatRunSummary(parentRunID, item.ID, workerCount)
		runs = append(runs, item)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to load runs", http.StatusInternalServerError)
		return
	}

	s.renderTemplate(w, "Runs", "runs_body", runsPageData{Runs: runs})
}

func formatRunSummary(parentRunID, runID string, workerCount int) string {
	if parentRunID == "" {
		return "front session with " + formatWorkerCount(workerCount)
	}
	return "worker session under " + parentRunID
}

func formatWorkerCount(count int) string {
	if count == 1 {
		return "1 worker run"
	}
	return fmt.Sprintf("%d worker runs", count)
}

func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")

	replayRun, err := s.replay.LoadRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to load run", http.StatusInternalServerError)
		return
	}

	events := make([]runEventView, 0, len(replayRun.Events))
	turns := make([]runTurnView, 0, len(replayRun.Events))
	for _, evt := range replayRun.Events {
		events = append(events, runEventView{
			Kind:      evt.Kind,
			CreatedAt: evt.CreatedAt,
		})
		if evt.Kind != "turn_completed" {
			continue
		}
		content, ok := turnContent(evt.PayloadJSON)
		if !ok {
			continue
		}
		turns = append(turns, runTurnView{
			Content:   content,
			CreatedAt: evt.CreatedAt,
		})
	}

	s.renderTemplate(w, "Run Detail", "run_detail_body", runDetailPageData{
		RunID:     replayRun.RunID,
		Status:    string(replayRun.Status),
		StreamURL: "/runs/" + url.PathEscape(replayRun.RunID) + "/events",
		Turns:     turns,
		Events:    events,
	})
}

func turnContent(payload []byte) (string, bool) {
	if len(payload) == 0 {
		return "", false
	}
	var decoded struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return "", false
	}
	if decoded.Content == "" {
		return "", false
	}
	return decoded.Content, true
}

// handleRunDismiss marks an interrupted run as dismissed so it no longer
// appears in the active run list. The run record is retained in the journal.
func (s *Server) handleRunDismiss(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	_, err := s.db.RawDB().ExecContext(r.Context(),
		`UPDATE runs SET status = 'dismissed', updated_at = datetime('now') WHERE id = ? AND status = 'interrupted'`,
		runID,
	)
	if err != nil {
		http.Error(w, "failed to dismiss run", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/runs", http.StatusSeeOther)
}

func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sub := s.broadcaster.Subscribe(runID)
	defer s.broadcaster.Unsubscribe(runID, sub)

	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt := <-sub:
			payload, err := marshalReplayDelta(evt)
			if err != nil {
				continue
			}
			if _, err := w.Write([]byte("data: ")); err != nil {
				return
			}
			if _, err := w.Write(payload); err != nil {
				return
			}
			if _, err := w.Write([]byte("\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func marshalReplayDelta(evt model.ReplayDelta) ([]byte, error) {
	type replayDeltaEnvelope struct {
		RunID      string          `json:"run_id"`
		Kind       string          `json:"kind"`
		Payload    json.RawMessage `json:"payload,omitempty"`
		OccurredAt time.Time       `json:"occurred_at"`
	}

	envelope := replayDeltaEnvelope{
		RunID:      evt.RunID,
		Kind:       evt.Kind,
		OccurredAt: evt.OccurredAt,
	}
	if len(evt.PayloadJSON) > 0 {
		envelope.Payload = json.RawMessage(evt.PayloadJSON)
	}
	return json.Marshal(envelope)
}
