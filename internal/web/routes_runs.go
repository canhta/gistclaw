package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/replay"
)

type runListItem struct {
	ID          string
	Objective   string
	Summary     string
	Status      string
	StatusClass string
}

type runsPageData struct {
	Runs    []runListItem
	Filters runListFilters
	Paging  pageLinks
}

type runDetailPageData struct {
	RunID             string
	Status            string
	StatusClass       string
	StreamURL         string
	GraphURL          string
	Turns             []runTurnView
	Events            []runEventView
	Graph             runGraphView
	ExecutionSnapshot runExecutionSnapshotView
}

type runEventView struct {
	Kind      string
	CreatedAt time.Time
}

type runTurnView struct {
	Content   string
	CreatedAt time.Time
}

type runExecutionSnapshotView struct {
	TeamID string
	Agents []runExecutionAgentView
}

type runExecutionAgentView struct {
	ID           string
	ToolProfile  string
	Capabilities []string
}

type runGraphView struct {
	RootRunID string               `json:"root_run_id"`
	Headline  string               `json:"headline"`
	Summary   runGraphSummaryView  `json:"summary"`
	Nodes     []runGraphNodeView   `json:"nodes"`
	Edges     []runGraphEdgeView   `json:"edges"`
	Columns   []runGraphColumnView `json:"columns"`
}

type runGraphEdgeView struct {
	From string `json:"from"`
	To   string `json:"to"`
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

type runListFilters struct {
	Query  string
	Status string
	Limit  int
}

type runListRow struct {
	Item      runListItem
	CreatedAt string
}

func (s *Server) handleRunsIndex(w http.ResponseWriter, r *http.Request) {
	filter := runListFilterFromRequest(r)
	querySQL, args, err := buildRunListQuery(filter)
	if err != nil {
		http.Error(w, "failed to build runs query", http.StatusInternalServerError)
		return
	}
	rows, err := s.db.RawDB().QueryContext(r.Context(), querySQL, args...)
	if err != nil {
		http.Error(w, "failed to load runs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	runRows := make([]runListRow, 0, filter.Limit+1)
	for rows.Next() {
		var item runListItem
		var parentRunID string
		var createdAt string
		var workerCount int
		if err := rows.Scan(&item.ID, &item.Objective, &parentRunID, &item.Status, &createdAt, &workerCount); err != nil {
			http.Error(w, "failed to load runs", http.StatusInternalServerError)
			return
		}
		item.Summary = formatRunSummary(parentRunID, item.ID, workerCount)
		item.StatusClass = runStatusClass(item.Status)
		runRows = append(runRows, runListRow{Item: item, CreatedAt: createdAt})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to load runs", http.StatusInternalServerError)
		return
	}

	items, paging := finalizeRunListPage(r.URL.Query(), filter, runRows)
	s.renderTemplate(w, r, "Runs", "runs_body", runsPageData{
		Runs:    items,
		Filters: runListFilters{Query: filter.Query, Status: filter.Status, Limit: filter.Limit},
		Paging:  paging,
	})
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

	s.renderTemplate(w, r, "Run Detail", "run_detail_body", runDetailPageData{
		RunID:             replayRun.RunID,
		Status:            string(replayRun.Status),
		StatusClass:       runStatusClass(string(replayRun.Status)),
		StreamURL:         runEventsPath(replayRun.RunID),
		GraphURL:          runGraphPath(replayRun.RunID),
		Turns:             turns,
		Events:            events,
		Graph:             buildRunGraphView(graphSnapshot),
		ExecutionSnapshot: buildExecutionSnapshotView(replayRun.TeamID, replayRun.ExecutionSnapshotJSON),
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

func buildExecutionSnapshotView(teamID string, raw []byte) runExecutionSnapshotView {
	view := runExecutionSnapshotView{TeamID: teamID}
	if len(raw) == 0 {
		return view
	}

	var snapshot model.ExecutionSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return view
	}
	if snapshot.TeamID != "" {
		view.TeamID = snapshot.TeamID
	}

	agentIDs := make([]string, 0, len(snapshot.Agents))
	for agentID := range snapshot.Agents {
		agentIDs = append(agentIDs, agentID)
	}
	sort.Strings(agentIDs)

	view.Agents = make([]runExecutionAgentView, 0, len(agentIDs))
	for _, agentID := range agentIDs {
		profile := snapshot.Agents[agentID]
		capabilities := make([]string, 0, len(profile.Capabilities))
		for _, capability := range profile.Capabilities {
			capabilities = append(capabilities, string(capability))
		}
		sort.Strings(capabilities)
		view.Agents = append(view.Agents, runExecutionAgentView{
			ID:           agentID,
			ToolProfile:  profile.ToolProfile,
			Capabilities: capabilities,
		})
	}

	return view
}

type runListRequest struct {
	Query     string
	Status    string
	Limit     int
	Cursor    runListCursor
	HasCursor bool
	Direction string
}

type runListCursor struct {
	CreatedAt string
	ID        string
}

func runListFilterFromRequest(r *http.Request) runListRequest {
	cursor, ok := parseRunListCursor(strings.TrimSpace(r.URL.Query().Get("cursor")))
	direction := strings.TrimSpace(r.URL.Query().Get("direction"))
	if direction != "prev" {
		direction = "next"
	}

	return runListRequest{
		Query:     strings.TrimSpace(r.URL.Query().Get("q")),
		Status:    strings.TrimSpace(r.URL.Query().Get("status")),
		Limit:     requestNamedLimit(r, "limit", 20),
		Cursor:    cursor,
		HasCursor: ok,
		Direction: direction,
	}
}

func buildRunListQuery(filter runListRequest) (string, []any, error) {
	var query strings.Builder
	query.WriteString(`SELECT id, COALESCE(objective, ''), COALESCE(parent_run_id, ''), status, created_at,
	        (SELECT count(*) FROM runs child WHERE child.parent_run_id = runs.id) AS worker_count
	 FROM runs`)

	clauses := []string{"1=1"}
	args := make([]any, 0, 8)
	if filter.Query != "" {
		like := "%" + filter.Query + "%"
		clauses = append(clauses, "(id LIKE ? OR COALESCE(objective, '') LIKE ?)")
		args = append(args, like, like)
	}
	if filter.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.HasCursor {
		switch filter.Direction {
		case "prev":
			clauses = append(clauses, "(created_at > ? OR (created_at = ? AND id > ?))")
		default:
			clauses = append(clauses, "(created_at < ? OR (created_at = ? AND id < ?))")
		}
		args = append(args, filter.Cursor.CreatedAt, filter.Cursor.CreatedAt, filter.Cursor.ID)
	}

	query.WriteString(" WHERE ")
	query.WriteString(strings.Join(clauses, " AND "))
	if filter.Direction == "prev" {
		query.WriteString(" ORDER BY created_at ASC, id ASC")
	} else {
		query.WriteString(" ORDER BY created_at DESC, id DESC")
	}
	query.WriteString(" LIMIT ?")
	args = append(args, filter.Limit+1)
	return query.String(), args, nil
}

func finalizeRunListPage(query url.Values, filter runListRequest, rows []runListRow) ([]runListItem, pageLinks) {
	hasExtra := len(rows) > filter.Limit
	if hasExtra {
		rows = rows[:filter.Limit]
	}
	if filter.Direction == "prev" {
		for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
			rows[left], rows[right] = rows[right], rows[left]
		}
	}

	items := make([]runListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.Item)
	}

	var nextCursor string
	var prevCursor string
	hasNext := false
	hasPrev := false
	if len(rows) > 0 {
		first := rows[0]
		last := rows[len(rows)-1]
		prevCursor = encodeRunListCursor(first.CreatedAt, first.Item.ID)
		nextCursor = encodeRunListCursor(last.CreatedAt, last.Item.ID)
	}

	switch filter.Direction {
	case "prev":
		hasPrev = hasExtra
		hasNext = filter.HasCursor
	default:
		hasPrev = filter.HasCursor
		hasNext = hasExtra
	}

	return items, buildPageLinks(pageOperateRuns, cloneQuery(query), "cursor", "direction", nextCursor, prevCursor, hasNext, hasPrev)
}

func parseRunListCursor(raw string) (runListCursor, bool) {
	if raw == "" {
		return runListCursor{}, false
	}
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return runListCursor{}, false
	}
	return runListCursor{CreatedAt: parts[0], ID: parts[1]}, true
}

func encodeRunListCursor(createdAt, id string) string {
	if createdAt == "" || id == "" {
		return ""
	}
	return createdAt + "|" + id
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
	http.Redirect(w, r, pageOperateRuns, http.StatusSeeOther)
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
		Nodes:     make([]runGraphNodeView, 0, len(snapshot.Nodes)),
		Edges:     make([]runGraphEdgeView, 0, len(snapshot.Edges)),
		Columns:   make([]runGraphColumnView, 0, len(snapshot.Nodes)),
	}
	for _, edge := range snapshot.Edges {
		view.Edges = append(view.Edges, runGraphEdgeView{
			From: edge.From,
			To:   edge.To,
		})
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
		view.Nodes = append(view.Nodes, graphNode)

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
