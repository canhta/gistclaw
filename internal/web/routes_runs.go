package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type runListItem struct {
	ID        string
	Objective string
	Status    string
}

type runsPageData struct {
	Runs []runListItem
}

type runDetailPageData struct {
	RunID  string
	Status string
	Events []runEventView
}

type runEventView struct {
	Kind      string
	CreatedAt time.Time
}

func (s *Server) handleRunsIndex(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.RawDB().QueryContext(r.Context(),
		`SELECT id, COALESCE(objective, ''), status
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
		if err := rows.Scan(&item.ID, &item.Objective, &item.Status); err != nil {
			http.Error(w, "failed to load runs", http.StatusInternalServerError)
			return
		}
		runs = append(runs, item)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to load runs", http.StatusInternalServerError)
		return
	}

	s.renderTemplate(w, "Runs", "runs_body", runsPageData{Runs: runs})
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
	for _, evt := range replayRun.Events {
		events = append(events, runEventView{
			Kind:      evt.Kind,
			CreatedAt: evt.CreatedAt,
		})
	}

	s.renderTemplate(w, "Run Detail", "run_detail_body", runDetailPageData{
		RunID:  replayRun.RunID,
		Status: string(replayRun.Status),
		Events: events,
	})
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
			payload, err := json.Marshal(evt)
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
