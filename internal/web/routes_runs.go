package web

import (
	"context"
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
	"github.com/canhta/gistclaw/internal/projectscope"
	"github.com/canhta/gistclaw/internal/replay"
	"github.com/canhta/gistclaw/internal/runtime"
)

type runListItem struct {
	ID                string
	DetailURL         string
	Objective         string
	AgentID           string
	Status            string
	StatusLabel       string
	StatusClass       string
	ModelLane         string
	ModelID           string
	ModelDisplay      string
	InputTokens       int
	OutputTokens      int
	TokenSummary      string
	StartedAtShort    string
	StartedAtExact    string
	StartedAtISO      string
	LastActivityShort string
	LastActivityExact string
	LastActivityISO   string
	Depth             int
}

type runsPageData struct {
	Clusters            []runListClusterView
	Filters             runListFilters
	Paging              pageLinks
	QueueStrip          runQueueStripView
	ActiveProjectName   string
	ActiveWorkspaceRoot string
}

type runQueueStripView struct {
	Headline     string
	RootRuns     int
	WorkerRuns   int
	RecoveryRuns int
	Summary      runGraphSummaryView
}

type runListClusterView struct {
	Root            runListItem
	Children        []runListItem
	ChildCount      int
	ChildCountLabel string
	BlockerLabel    string
	HasChildren     bool
}

type runDetailPageData struct {
	RunID             string
	RunShortID        string
	Status            string
	StatusLabel       string
	StatusClass       string
	StartedAtLabel    string
	LastActivityLabel string
	EventCount        int
	TurnCount         int
	StreamURL         string
	GraphURL          string
	Turns             []runTurnView
	Events            []runEventView
	Graph             runGraphView
	ExecutionSnapshot runExecutionSnapshotView
}

type runEventView struct {
	Kind           string
	Label          string
	CreatedAt      time.Time
	CreatedAtLabel string
	ToneClass      string
}

type runTurnView struct {
	Content        string
	CreatedAt      time.Time
	CreatedAtLabel string
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
	ID             string `json:"id"`
	ShortID        string `json:"short_id"`
	ParentRunID    string `json:"parent_run_id"`
	AgentID        string `json:"agent_id"`
	SessionID      string `json:"session_id,omitempty"`
	SessionShortID string `json:"session_short_id,omitempty"`
	Objective      string `json:"objective"`
	Status         string `json:"status"`
	StatusLabel    string `json:"status_label"`
	StatusClass    string `json:"status_class"`
	UpdatedAtLabel string `json:"updated_at_label"`
	Depth          int    `json:"depth"`
	IsRoot         bool   `json:"is_root"`
	ParentLabel    string `json:"parent_label,omitempty"`
}

type runListFilters struct {
	Query  string
	Status string
	Limit  int
	Scope  string
}

type runListRow struct {
	ID           string
	Objective    string
	AgentID      string
	Status       string
	QueueStatus  string
	ModelLane    string
	ModelID      string
	InputTokens  int
	OutputTokens int
	CreatedAt    string
	UpdatedAt    string
	WorkerCount  int
}

type runChildRow struct {
	RootID       string
	ID           string
	ParentRunID  string
	Objective    string
	AgentID      string
	Status       string
	ModelLane    string
	ModelID      string
	InputTokens  int
	OutputTokens int
	CreatedAt    string
	UpdatedAt    string
	Depth        int
}

func (s *Server) handleRunsIndex(w http.ResponseWriter, r *http.Request) {
	filter := runListFilterFromRequest(r)
	activeProject, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load active project", http.StatusInternalServerError)
		return
	}
	querySQL, args, err := buildRunListQuery(filter, activeProject)
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
		var row runListRow
		if err := rows.Scan(
			&row.ID,
			&row.Objective,
			&row.AgentID,
			&row.Status,
			&row.QueueStatus,
			&row.ModelLane,
			&row.ModelID,
			&row.InputTokens,
			&row.OutputTokens,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.WorkerCount,
		); err != nil {
			http.Error(w, "failed to load runs", http.StatusInternalServerError)
			return
		}
		runRows = append(runRows, row)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to load runs", http.StatusInternalServerError)
		return
	}

	rootRows, paging := finalizeRunListPage(r.URL.Query(), filter, runRows)
	descendants, err := loadRunDescendants(r.Context(), s.db.RawDB(), rootRows)
	if err != nil {
		http.Error(w, "failed to load worker runs", http.StatusInternalServerError)
		return
	}
	clusters := buildRunListClusters(rootRows, descendants)
	s.renderTemplate(w, r, "Runs", "runs_body", runsPageData{
		Clusters:            clusters,
		Filters:             runListFilters{Query: filter.Query, Status: filter.Status, Limit: filter.Limit, Scope: filter.Scope},
		Paging:              paging,
		QueueStrip:          buildRunQueueStrip(clusters),
		ActiveProjectName:   activeProject.Name,
		ActiveWorkspaceRoot: activeProject.WorkspaceRoot,
	})
}

func formatWorkerCount(count int) string {
	if count == 1 {
		return "1 worker"
	}
	return fmt.Sprintf("%d workers", count)
}

func humanizeRunStatus(status string) string {
	switch status {
	case "needs_approval":
		return "needs approval"
	default:
		return strings.ReplaceAll(status, "_", " ")
	}
}

func buildRunQueueStrip(clusters []runListClusterView) runQueueStripView {
	view := runQueueStripView{}
	for _, cluster := range clusters {
		view.RootRuns++
		view.Summary.Total++
		incrementRunQueueSummary(&view, cluster.Root.Status)
		for _, child := range cluster.Children {
			view.WorkerRuns++
			view.Summary.Total++
			incrementRunQueueSummary(&view, child.Status)
		}
	}

	switch {
	case view.Summary.Total == 0:
		view.Headline = "No live collaboration yet. Start a task to spin up the queue, graph, and recovery surface."
	case view.RecoveryRuns > 0:
		view.Headline = "Recovery work is visible in the queue. Review blocked or interrupted runs before starting new work."
	case view.Summary.Active > 0 || view.Summary.Pending > 0:
		view.Headline = "The queue is active. Root runs, delegated workers, and completion states are visible in one strip."
	default:
		view.Headline = "Recent work is settled. Use this strip to scan collaboration shape before opening a run detail."
	}

	return view
}

func incrementRunQueueSummary(view *runQueueStripView, status string) {
	switch status {
	case "pending":
		view.Summary.Pending++
	case "active":
		view.Summary.Active++
	case "needs_approval":
		view.Summary.NeedsApproval++
		view.RecoveryRuns++
	case "completed":
		view.Summary.Completed++
	case "failed":
		view.Summary.Failed++
		view.RecoveryRuns++
	case "interrupted":
		view.Summary.Interrupted++
		view.RecoveryRuns++
	}
}

func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	visible, err := s.runVisibleInActiveProject(r.Context(), runID)
	if err != nil {
		http.Error(w, "failed to load run", http.StatusInternalServerError)
		return
	}
	if !visible {
		http.NotFound(w, r)
		return
	}

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
			Kind:           evt.Kind,
			Label:          humanizeEventKind(evt.Kind),
			CreatedAt:      evt.CreatedAt,
			CreatedAtLabel: formatRunTimestamp(evt.CreatedAt),
			ToneClass:      eventToneClass(evt.Kind),
		})
		if evt.Kind != "turn_completed" {
			continue
		}
		content, ok := turnContent(evt.PayloadJSON)
		if !ok {
			continue
		}
		turns = append(turns, runTurnView{
			Content:        content,
			CreatedAt:      evt.CreatedAt,
			CreatedAtLabel: formatRunTimestamp(evt.CreatedAt),
		})
	}
	startedAt, lastActivity := runEventWindow(replayRun.Events)
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
		RunShortID:        compactIdentifier(replayRun.RunID),
		Status:            string(replayRun.Status),
		StatusLabel:       humanizeRunStatus(string(replayRun.Status)),
		StatusClass:       runStatusClass(string(replayRun.Status)),
		StartedAtLabel:    formatRunTimestamp(startedAt),
		LastActivityLabel: formatRunTimestamp(lastActivity),
		EventCount:        len(events),
		TurnCount:         len(turns),
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
	visible, err := s.runVisibleInActiveProject(r.Context(), runID)
	if err != nil {
		http.Error(w, "failed to load run graph", http.StatusInternalServerError)
		return
	}
	if !visible {
		http.NotFound(w, r)
		return
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
	Scope     string
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
		Scope:     runScopeFromRequest(r),
		Cursor:    cursor,
		HasCursor: ok,
		Direction: direction,
	}
}

func runScopeFromRequest(r *http.Request) string {
	if strings.TrimSpace(r.URL.Query().Get("scope")) == "all" {
		return "all"
	}
	return "active"
}

func buildRunListQuery(filter runListRequest, activeProject model.Project) (string, []any, error) {
	var query strings.Builder
	query.WriteString(`WITH RECURSIVE run_tree(root_id, id, status) AS (
		SELECT root.id, root.id, root.status
		FROM runs root
		WHERE COALESCE(root.parent_run_id, '') = ''
		UNION ALL
		SELECT run_tree.root_id, child.id, child.status
		FROM run_tree
		JOIN runs child ON child.parent_run_id = run_tree.id
	),
	queue_summary AS (
		SELECT root_id,
			SUM(CASE WHEN id != root_id THEN 1 ELSE 0 END) AS worker_count,
			MAX(CASE WHEN id != root_id AND status = 'needs_approval' THEN 1 ELSE 0 END) AS has_needs_approval,
			MAX(CASE WHEN id != root_id AND status = 'failed' THEN 1 ELSE 0 END) AS has_failed,
			MAX(CASE WHEN id != root_id AND status = 'interrupted' THEN 1 ELSE 0 END) AS has_interrupted,
			MAX(CASE WHEN id != root_id AND status = 'active' THEN 1 ELSE 0 END) AS has_active_workers,
			MAX(CASE WHEN id != root_id AND status = 'pending' THEN 1 ELSE 0 END) AS has_pending_workers
		FROM run_tree
		GROUP BY root_id
	),
	root_queue AS (
		SELECT root.id,
			COALESCE(root.objective, '') AS objective,
			COALESCE(root.agent_id, '') AS agent_id,
			root.status,
			` + runQueueStatusExpression("root", "queue_summary") + ` AS queue_status,
			COALESCE(root.model_lane, '') AS model_lane,
			COALESCE(root.model_id, '') AS model_id,
			COALESCE(root.input_tokens, 0) AS input_tokens,
			COALESCE(root.output_tokens, 0) AS output_tokens,
			root.created_at,
			root.updated_at,
			COALESCE(queue_summary.worker_count, 0) AS worker_count
		FROM runs root
		LEFT JOIN queue_summary ON queue_summary.root_id = root.id
		WHERE COALESCE(root.parent_run_id, '') = ''`)

	clauses := []string{"1=1"}
	args := make([]any, 0, 10)
	rootScopeArgs := make([]any, 0, 2)
	if filter.Query != "" {
		like := "%" + filter.Query + "%"
		clauses = append(clauses, "(id LIKE ? OR objective LIKE ? OR agent_id LIKE ?)")
		args = append(args, like, like, like)
	}
	if filter.Status != "" {
		clauses = append(clauses, "queue_status = ?")
		args = append(args, filter.Status)
	}
	if filter.Scope != "all" {
		condition, scopeValues := projectscope.RunCondition(activeProject, "root")
		query.WriteString(" AND ")
		query.WriteString(condition)
		rootScopeArgs = append(rootScopeArgs, scopeValues...)
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

	query.WriteString(`
	)
	SELECT id, objective, agent_id, status, queue_status, model_lane, model_id, input_tokens, output_tokens, created_at, updated_at, worker_count
	FROM root_queue
	WHERE `)
	query.WriteString(strings.Join(clauses, " AND "))
	if filter.Direction == "prev" {
		query.WriteString(" ORDER BY created_at ASC, id ASC")
	} else {
		query.WriteString(" ORDER BY created_at DESC, id DESC")
	}
	query.WriteString(" LIMIT ?")
	args = append(args, filter.Limit+1)
	args = append(rootScopeArgs, args...)
	return query.String(), args, nil
}

func finalizeRunListPage(query url.Values, filter runListRequest, rows []runListRow) ([]runListRow, pageLinks) {
	hasExtra := len(rows) > filter.Limit
	if hasExtra {
		rows = rows[:filter.Limit]
	}
	if filter.Direction == "prev" {
		for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
			rows[left], rows[right] = rows[right], rows[left]
		}
	}

	var nextCursor string
	var prevCursor string
	hasNext := false
	hasPrev := false
	if len(rows) > 0 {
		first := rows[0]
		last := rows[len(rows)-1]
		prevCursor = encodeRunListCursor(first.CreatedAt, first.ID)
		nextCursor = encodeRunListCursor(last.CreatedAt, last.ID)
	}

	switch filter.Direction {
	case "prev":
		hasPrev = hasExtra
		hasNext = filter.HasCursor
	default:
		hasPrev = filter.HasCursor
		hasNext = hasExtra
	}

	return rows, buildPageLinks(pageOperateRuns, cloneQuery(query), "cursor", "direction", nextCursor, prevCursor, hasNext, hasPrev)
}

func runQueueStatusExpression(rootAlias, queueAlias string) string {
	rootPrefix := ""
	if rootAlias != "" {
		rootPrefix = rootAlias + "."
	}
	queuePrefix := ""
	if queueAlias != "" {
		queuePrefix = queueAlias + "."
	}
	return `CASE
		WHEN COALESCE(` + queuePrefix + `has_needs_approval, 0) = 1 THEN 'needs_approval'
		WHEN COALESCE(` + queuePrefix + `has_failed, 0) = 1 THEN 'failed'
		WHEN COALESCE(` + queuePrefix + `has_interrupted, 0) = 1 THEN 'interrupted'
		WHEN COALESCE(` + queuePrefix + `has_active_workers, 0) = 1 THEN 'active'
		WHEN COALESCE(` + queuePrefix + `has_pending_workers, 0) = 1 THEN 'pending'
		ELSE ` + rootPrefix + `status
	END`
}

func loadRunDescendants(ctx context.Context, db *sql.DB, roots []runListRow) (map[string][]runChildRow, error) {
	descendants := make(map[string][]runChildRow, len(roots))
	if len(roots) == 0 {
		return descendants, nil
	}

	args := make([]any, 0, len(roots))
	placeholders := make([]string, 0, len(roots))
	for _, root := range roots {
		placeholders = append(placeholders, "?")
		args = append(args, root.ID)
	}

	query := `WITH RECURSIVE descendants(root_id, id, parent_run_id, objective, agent_id, status, model_lane, model_id, input_tokens, output_tokens, created_at, updated_at, depth) AS (
		SELECT root.id, child.id, child.parent_run_id, COALESCE(child.objective, ''), COALESCE(child.agent_id, ''), child.status, COALESCE(child.model_lane, ''), COALESCE(child.model_id, ''), COALESCE(child.input_tokens, 0), COALESCE(child.output_tokens, 0), child.created_at, child.updated_at, 1
		FROM runs root
		JOIN runs child ON child.parent_run_id = root.id
		WHERE root.id IN (` + strings.Join(placeholders, ", ") + `)
		UNION ALL
		SELECT descendants.root_id, child.id, child.parent_run_id, COALESCE(child.objective, ''), COALESCE(child.agent_id, ''), child.status, COALESCE(child.model_lane, ''), COALESCE(child.model_id, ''), COALESCE(child.input_tokens, 0), COALESCE(child.output_tokens, 0), child.created_at, child.updated_at, descendants.depth + 1
		FROM descendants
		JOIN runs child ON child.parent_run_id = descendants.id
	)
	SELECT root_id, id, parent_run_id, objective, agent_id, status, model_lane, model_id, input_tokens, output_tokens, created_at, updated_at, depth
	FROM descendants
	ORDER BY root_id ASC, created_at ASC, id ASC`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var row runChildRow
		if err := rows.Scan(
			&row.RootID,
			&row.ID,
			&row.ParentRunID,
			&row.Objective,
			&row.AgentID,
			&row.Status,
			&row.ModelLane,
			&row.ModelID,
			&row.InputTokens,
			&row.OutputTokens,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.Depth,
		); err != nil {
			return nil, err
		}
		descendants[row.RootID] = append(descendants[row.RootID], row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return descendants, nil
}

func buildRunListClusters(roots []runListRow, descendants map[string][]runChildRow) []runListClusterView {
	clusters := make([]runListClusterView, 0, len(roots))
	for _, root := range roots {
		childRows := descendants[root.ID]
		cluster := runListClusterView{
			Root:            buildRunListItem(root.ID, root.Objective, root.AgentID, root.QueueStatus, root.ModelLane, root.ModelID, root.InputTokens, root.OutputTokens, root.CreatedAt, root.UpdatedAt, 0),
			Children:        make([]runListItem, 0, len(childRows)),
			ChildCount:      len(childRows),
			ChildCountLabel: formatWorkerCount(len(childRows)),
			BlockerLabel:    summarizeRunBlocker(root.QueueStatus, childRows, root.Status, len(childRows)),
			HasChildren:     len(childRows) > 0,
		}
		for _, child := range childRows {
			cluster.Children = append(cluster.Children, buildRunListItem(child.ID, child.Objective, child.AgentID, child.Status, child.ModelLane, child.ModelID, child.InputTokens, child.OutputTokens, child.CreatedAt, child.UpdatedAt, child.Depth))
		}
		clusters = append(clusters, cluster)
	}
	return clusters
}

func buildRunListItem(id, objective, agentID, status, modelLane, modelID string, inputTokens, outputTokens int, createdAt, updatedAt string, depth int) runListItem {
	startedAt := buildRunTimestampView(parseRunListTimestamp(createdAt))
	lastActivity := buildRunTimestampView(parseRunListTimestamp(updatedAt))
	modelDisplay := formatRunModelDisplay(modelID, modelLane)
	return runListItem{
		ID:                id,
		DetailURL:         pageOperateRuns + "/" + id,
		Objective:         objective,
		AgentID:           agentID,
		Status:            status,
		StatusLabel:       humanizeRunStatus(status),
		StatusClass:       runStatusClass(status),
		ModelLane:         modelLane,
		ModelID:           modelID,
		ModelDisplay:      modelDisplay,
		InputTokens:       inputTokens,
		OutputTokens:      outputTokens,
		TokenSummary:      formatRunTokenSummary(inputTokens, outputTokens),
		StartedAtShort:    startedAt.Relative,
		StartedAtExact:    startedAt.Exact,
		StartedAtISO:      startedAt.ISO,
		LastActivityShort: lastActivity.Relative,
		LastActivityExact: lastActivity.Exact,
		LastActivityISO:   lastActivity.ISO,
		Depth:             depth,
	}
}

func formatRunModelDisplay(modelID, modelLane string) string {
	if value := strings.TrimSpace(modelID); value != "" {
		return value
	}
	if value := strings.TrimSpace(modelLane); value != "" {
		return value
	}
	return "not recorded"
}

func formatRunTokenSummary(inputTokens, outputTokens int) string {
	return fmt.Sprintf("%d in / %d out", inputTokens, outputTokens)
}

func summarizeRunBlocker(queueStatus string, descendants []runChildRow, rootStatus string, childCount int) string {
	switch queueStatus {
	case "needs_approval":
		for _, child := range descendants {
			if child.Status == "needs_approval" {
				if child.AgentID != "" {
					return child.AgentID + " waiting on approval"
				}
				return "Worker waiting on approval"
			}
		}
		return "Worker waiting on approval"
	case "failed":
		for _, child := range descendants {
			if child.Status == "failed" {
				if child.AgentID != "" {
					return child.AgentID + " failed"
				}
				return "Worker failed"
			}
		}
		return "Run failed"
	case "interrupted":
		for _, child := range descendants {
			if child.Status == "interrupted" {
				if child.AgentID != "" {
					return child.AgentID + " interrupted"
				}
				return "Worker interrupted"
			}
		}
		return "Run interrupted"
	case "active":
		if count := countRunStatus(descendants, "active"); count > 0 {
			return formatWorkerCount(count) + " active"
		}
		if rootStatus == "active" {
			return "Coordinator active"
		}
	case "pending":
		if count := countRunStatus(descendants, "pending"); count > 0 {
			return formatWorkerCount(count) + " queued"
		}
		return "Queued to start"
	case "completed":
		if childCount > 0 {
			return formatWorkerCount(childCount) + " settled"
		}
		return "No delegated workers"
	}
	if childCount > 0 {
		return formatWorkerCount(childCount) + " visible"
	}
	return "No delegated workers"
}

func countRunStatus(rows []runChildRow, want string) int {
	count := 0
	for _, row := range rows {
		if row.Status == want {
			count++
		}
	}
	return count
}

type runTimestampView struct {
	Relative string
	Exact    string
	ISO      string
}

var runListNow = func() time.Time {
	return time.Now().UTC()
}

func parseRunListTimestamp(raw string) time.Time {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
	} {
		var (
			ts  time.Time
			err error
		)
		if layout == "2006-01-02 15:04:05" {
			ts, err = time.ParseInLocation(layout, value, time.UTC)
		} else {
			ts, err = time.Parse(layout, value)
		}
		if err == nil {
			return ts.UTC()
		}
	}
	return time.Time{}
}

func buildRunTimestampView(ts time.Time) runTimestampView {
	if ts.IsZero() {
		return runTimestampView{
			Relative: "Not recorded",
			Exact:    "Not recorded yet",
		}
	}
	return runTimestampView{
		Relative: humanizeRunRelativeTime(runListNow(), ts),
		Exact:    formatRunTimestamp(ts),
		ISO:      ts.UTC().Format(time.RFC3339),
	}
}

func humanizeRunRelativeTime(now, ts time.Time) string {
	if ts.IsZero() {
		return "Not recorded"
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if ts.After(now) {
		ts = now
	}
	delta := now.Sub(ts)
	switch {
	case delta < 30*time.Second:
		return "just now"
	case delta < time.Hour:
		return fmt.Sprintf("%dm ago", int(delta/time.Minute))
	case delta < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(delta/time.Hour))
	default:
		return fmt.Sprintf("%dd ago", int(delta/(24*time.Hour)))
	}
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
	visible, err := s.runVisibleInActiveProject(r.Context(), runID)
	if err != nil {
		http.Error(w, "failed to load run events", http.StatusInternalServerError)
		return
	}
	if !visible {
		http.NotFound(w, r)
		return
	}

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
			ID:             node.ID,
			ShortID:        compactIdentifier(node.ID),
			ParentRunID:    node.ParentRunID,
			AgentID:        node.AgentID,
			SessionID:      node.SessionID,
			SessionShortID: compactIdentifier(node.SessionID),
			Objective:      node.Objective,
			Status:         string(node.Status),
			StatusLabel:    humanizeRunStatus(string(node.Status)),
			StatusClass:    runStatusClass(string(node.Status)),
			UpdatedAtLabel: formatRunTimestamp(node.UpdatedAt),
			Depth:          node.Depth,
			IsRoot:         node.ID == snapshot.RootRunID,
		}
		if graphNode.ParentRunID != "" {
			graphNode.ParentLabel = "from " + compactIdentifier(graphNode.ParentRunID)
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

func humanizeEventKind(kind string) string {
	label := strings.TrimSpace(strings.ReplaceAll(kind, "_", " "))
	if label == "" {
		return "Unknown event"
	}
	return strings.ToUpper(label[:1]) + label[1:]
}

func eventToneClass(kind string) string {
	switch kind {
	case "run_started", "run_resumed", "tool_started":
		return "is-active"
	case "approval_requested":
		return "is-approval"
	case "turn_completed", "tool_completed", "run_completed":
		return "is-success"
	case "run_failed", "tool_failed":
		return "is-error"
	case "run_interrupted":
		return "is-muted"
	default:
		return ""
	}
}

func compactIdentifier(id string) string {
	if id == "" {
		return ""
	}
	if len(id) <= 14 {
		return id
	}
	return id[:8] + "…" + id[len(id)-4:]
}

func formatRunTimestamp(ts time.Time) string {
	if ts.IsZero() {
		return "Not recorded yet"
	}
	return ts.UTC().Format("2006-01-02 15:04:05 UTC")
}

func runEventWindow(events []model.Event) (time.Time, time.Time) {
	if len(events) == 0 {
		return time.Time{}, time.Time{}
	}
	return events[0].CreatedAt, events[len(events)-1].CreatedAt
}
