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
	"github.com/canhta/gistclaw/internal/replay"
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
	RunID       string
	Status      string
	StatusClass string
	StreamURL   string
	GraphURL    string
	Turns       []runTurnView
	Events      []runEventView
	Graph       runGraphView
}

type runEventView struct {
	Kind      string
	CreatedAt time.Time
}

type runTurnView struct {
	Content   string
	CreatedAt time.Time
}

type runGraphView struct {
	RootRunID string               `json:"root_run_id"`
	Headline  string               `json:"headline"`
	Summary   runGraphSummaryView  `json:"summary"`
	Columns   []runGraphColumnView `json:"columns"`
}

type runGraphSummaryView struct {
	Total         int    `json:"total"`
	Pending       int    `json:"pending"`
	Active        int    `json:"active"`
	NeedsApproval int    `json:"needs_approval"`
	Completed     int    `json:"completed"`
	Failed        int    `json:"failed"`
	Interrupted   int    `json:"interrupted"`
	RootStatus    string `json:"root_status"`
}

type runGraphColumnView struct {
	Depth int                `json:"depth"`
	Label string             `json:"label"`
	Nodes []runGraphNodeView `json:"nodes"`
}

type runGraphNodeView struct {
	ID          string `json:"id"`
	ParentRunID string `json:"parent_run_id"`
	AgentID     string `json:"agent_id"`
	SessionID   string `json:"session_id,omitempty"`
	Objective   string `json:"objective"`
	Status      string `json:"status"`
	StatusClass string `json:"status_class"`
	Depth       int    `json:"depth"`
	IsRoot      bool   `json:"is_root"`
	ParentLabel string `json:"parent_label,omitempty"`
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
	graphSnapshot, err := s.replay.LoadGraphSnapshot(r.Context(), runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to load run graph", http.StatusInternalServerError)
		return
	}

	s.renderTemplate(w, "Run Detail", "run_detail_body", runDetailPageData{
		RunID:       replayRun.RunID,
		Status:      string(replayRun.Status),
		StatusClass: runStatusClass(string(replayRun.Status)),
		StreamURL:   "/runs/" + url.PathEscape(replayRun.RunID) + "/events",
		GraphURL:    "/runs/" + url.PathEscape(replayRun.RunID) + "/graph",
		Turns:       turns,
		Events:      events,
		Graph:       buildRunGraphView(graphSnapshot),
	})
}

func (s *Server) handleRunGraph(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")

	graphSnapshot, err := s.replay.LoadGraphSnapshot(r.Context(), runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to load run graph", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, buildRunGraphView(graphSnapshot))
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

func buildRunGraphView(snapshot replay.RunGraphSnapshot) runGraphView {
	view := runGraphView{
		RootRunID: snapshot.RootRunID,
		Columns:   make([]runGraphColumnView, 0, len(snapshot.Nodes)),
	}

	columnIndex := make(map[int]int, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		graphNode := runGraphNodeView{
			ID:          node.ID,
			ParentRunID: node.ParentRunID,
			AgentID:     node.AgentID,
			SessionID:   node.SessionID,
			Objective:   node.Objective,
			Status:      string(node.Status),
			StatusClass: runStatusClass(string(node.Status)),
			Depth:       node.Depth,
			IsRoot:      node.ID == snapshot.RootRunID,
		}
		if graphNode.ParentRunID != "" {
			graphNode.ParentLabel = "from " + graphNode.ParentRunID
		}

		switch node.Status {
		case model.RunStatusPending:
			view.Summary.Pending++
		case model.RunStatusActive:
			view.Summary.Active++
		case model.RunStatusNeedsApproval:
			view.Summary.NeedsApproval++
		case model.RunStatusCompleted:
			view.Summary.Completed++
		case model.RunStatusFailed:
			view.Summary.Failed++
		case model.RunStatusInterrupted:
			view.Summary.Interrupted++
		}
		view.Summary.Total++
		if graphNode.IsRoot {
			view.Summary.RootStatus = graphNode.Status
		}

		idx, ok := columnIndex[node.Depth]
		if !ok {
			view.Columns = append(view.Columns, runGraphColumnView{
				Depth: node.Depth,
				Label: runGraphColumnLabel(node.Depth),
				Nodes: []runGraphNodeView{},
			})
			idx = len(view.Columns) - 1
			columnIndex[node.Depth] = idx
		}
		view.Columns[idx].Nodes = append(view.Columns[idx].Nodes, graphNode)
	}

	view.Headline = runGraphHeadline(view.Summary)
	return view
}

func runGraphColumnLabel(depth int) string {
	switch depth {
	case 0:
		return "Front Session"
	case 1:
		return "Delegated Workers"
	default:
		return fmt.Sprintf("Depth %d", depth)
	}
}

func runGraphHeadline(summary runGraphSummaryView) string {
	switch {
	case summary.NeedsApproval > 0:
		return fmt.Sprintf("%d run(s) waiting on approval.", summary.NeedsApproval)
	case summary.Active > 0:
		return fmt.Sprintf("%d run(s) actively working.", summary.Active)
	case summary.Pending > 0:
		return fmt.Sprintf("%d run(s) queued to start.", summary.Pending)
	case summary.Failed > 0:
		return fmt.Sprintf("%d run(s) failed in this execution graph.", summary.Failed)
	case summary.Interrupted > 0:
		return fmt.Sprintf("%d run(s) were interrupted.", summary.Interrupted)
	case summary.Completed == summary.Total && summary.Total > 0:
		return "All visible runs completed."
	default:
		return "Graph status is available."
	}
}

func runStatusClass(status string) string {
	switch status {
	case string(model.RunStatusPending):
		return "is-pending"
	case string(model.RunStatusActive):
		return "is-active"
	case string(model.RunStatusNeedsApproval):
		return "is-approval"
	case string(model.RunStatusCompleted):
		return "is-success"
	case string(model.RunStatusFailed), "error":
		return "is-error"
	case string(model.RunStatusInterrupted):
		return "is-muted"
	default:
		return ""
	}
}
