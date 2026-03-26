package web

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/projectscope"
	"github.com/canhta/gistclaw/internal/runtime"
)

type approvalItem struct {
	ID              string
	RunID           string
	ToolName        string
	TargetPath      string
	Status          string
	StatusLabel     string
	StatusClass     string
	ResolvedBy      string
	CreatedAt       time.Time
	ResolvedAt      *time.Time
	ResolvedAtLabel string
}

type approvalsPageData struct {
	Approvals []approvalItem
	Filters   approvalListFilters
	Paging    pageLinks
	Error     string
}

func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	filter := approvalListFilterFromRequest(r)
	activeProject, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load active project", http.StatusInternalServerError)
		return
	}
	querySQL, args, err := buildApprovalListQuery(filter, activeProject)
	if err != nil {
		http.Error(w, "failed to build approvals query", http.StatusInternalServerError)
		return
	}
	rows, err := s.db.RawDB().QueryContext(r.Context(), querySQL, args...)
	if err != nil {
		http.Error(w, "failed to load approvals", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	approvalRows := make([]approvalListRow, 0, filter.Limit+1)
	for rows.Next() {
		var item approvalItem
		var createdAt string
		var resolvedAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.RunID, &item.ToolName, &item.TargetPath, &item.Status, &item.ResolvedBy, &item.CreatedAt, &resolvedAt, &createdAt); err != nil {
			http.Error(w, "failed to load approvals", http.StatusInternalServerError)
			return
		}
		item.StatusLabel = humanizeWebLabel(item.Status)
		item.StatusClass = approvalStatusClass(item.Status)
		if resolvedAt.Valid {
			item.ResolvedAt = &resolvedAt.Time
			item.ResolvedAtLabel = formatWebTimestamp(resolvedAt.Time)
		}
		approvalRows = append(approvalRows, approvalListRow{Item: item, CreatedAt: createdAt})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to load approvals", http.StatusInternalServerError)
		return
	}

	items, paging := finalizeApprovalListPage(r.URL.Query(), filter, approvalRows)
	s.renderTemplate(w, r, "Approvals", "approvals_body", approvalsPageData{
		Approvals: items,
		Filters: approvalListFilters{
			Query:  filter.Query,
			Status: filter.Status,
			Limit:  filter.Limit,
		},
		Paging: paging,
	})
}

func (s *Server) handleApprovalResolve(w http.ResponseWriter, r *http.Request) {
	ticketID := r.PathValue("id")
	wantsJSON := strings.Contains(strings.ToLower(r.Header.Get("Accept")), "application/json")
	visible, err := s.approvalVisibleInActiveProject(r.Context(), ticketID)
	if err != nil {
		http.Error(w, "failed to load approval", http.StatusInternalServerError)
		return
	}
	if !visible {
		http.NotFound(w, r)
		return
	}

	// Guard: check if approval is expired before attempting to resolve.
	var currentStatus string
	if err := s.db.RawDB().QueryRowContext(r.Context(),
		"SELECT status FROM approvals WHERE id = ?", ticketID,
	).Scan(&currentStatus); err == nil && currentStatus == "expired" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "This approval has expired and can no longer be resolved.",
		})
		return
	}

	if err := r.ParseForm(); err != nil {
		writeApprovalResolveError(w, wantsJSON, http.StatusBadRequest, "invalid form")
		return
	}

	decision := r.FormValue("decision")
	if decision != "approved" && decision != "denied" {
		writeApprovalResolveError(w, wantsJSON, http.StatusBadRequest, "decision must be approved or denied")
		return
	}

	if s.rt == nil {
		writeApprovalResolveError(w, wantsJSON, http.StatusInternalServerError, "runtime not configured")
		return
	}

	if err := s.rt.ResolveApprovalAsync(r.Context(), ticketID, decision); err != nil {
		if wantsJSON {
			writeApprovalResolveError(w, true, http.StatusInternalServerError, err.Error())
			return
		}
		s.renderTemplate(w, r, "Approvals", "approvals_body", approvalsPageData{
			Error: err.Error(),
		})
		return
	}

	if wantsJSON {
		writeJSON(w, http.StatusOK, map[string]string{
			"approval_id": ticketID,
			"decision":    decision,
			"status":      decision,
		})
		return
	}
	http.Redirect(w, r, pageRecoverApprovals, http.StatusSeeOther)
}

type approvalListFilters struct {
	Query  string
	Status string
	Limit  int
}

type approvalListRequest struct {
	Query     string
	Status    string
	Limit     int
	Cursor    approvalListCursor
	HasCursor bool
	Direction string
}

type approvalListCursor struct {
	CreatedAt string
	ID        string
}

type approvalListRow struct {
	Item      approvalItem
	CreatedAt string
}

func approvalListFilterFromRequest(r *http.Request) approvalListRequest {
	cursor, ok := parseApprovalListCursor(strings.TrimSpace(r.URL.Query().Get("cursor")))
	direction := strings.TrimSpace(r.URL.Query().Get("direction"))
	if direction != "prev" {
		direction = "next"
	}

	return approvalListRequest{
		Query:     strings.TrimSpace(r.URL.Query().Get("q")),
		Status:    strings.TrimSpace(r.URL.Query().Get("status")),
		Limit:     requestNamedLimit(r, "limit", 20),
		Cursor:    cursor,
		HasCursor: ok,
		Direction: direction,
	}
}

func buildApprovalListQuery(filter approvalListRequest, activeProject model.Project) (string, []any, error) {
	var query strings.Builder
	query.WriteString(`SELECT approvals.id, approvals.run_id, approvals.tool_name, COALESCE(approvals.target_path, ''), approvals.status, COALESCE(approvals.resolved_by, ''), approvals.created_at, approvals.resolved_at, approvals.created_at
	 FROM approvals
	 JOIN runs ON runs.id = approvals.run_id`)

	clauses := []string{"1=1"}
	args := make([]any, 0, 8)
	condition, scopeArgs := projectscope.RunCondition(activeProject, "runs")
	clauses = append(clauses, condition)
	args = append(args, scopeArgs...)
	if filter.Query != "" {
		like := "%" + filter.Query + "%"
		clauses = append(clauses, "(approvals.id LIKE ? OR approvals.run_id LIKE ? OR approvals.tool_name LIKE ? OR COALESCE(approvals.target_path, '') LIKE ?)")
		args = append(args, like, like, like, like)
	}
	switch filter.Status {
	case "", "all":
	case "pending", "expired", "approved", "denied":
		clauses = append(clauses, "approvals.status = ?")
		args = append(args, filter.Status)
	case "open":
		clauses = append(clauses, "approvals.status IN ('pending', 'expired')")
	default:
		return "", nil, fmt.Errorf("unknown approval status filter %q", filter.Status)
	}
	if filter.HasCursor {
		switch filter.Direction {
		case "prev":
			clauses = append(clauses, "(approvals.created_at > ? OR (approvals.created_at = ? AND approvals.id > ?))")
		default:
			clauses = append(clauses, "(approvals.created_at < ? OR (approvals.created_at = ? AND approvals.id < ?))")
		}
		args = append(args, filter.Cursor.CreatedAt, filter.Cursor.CreatedAt, filter.Cursor.ID)
	}

	query.WriteString(" WHERE ")
	query.WriteString(strings.Join(clauses, " AND "))
	if filter.Direction == "prev" {
		query.WriteString(" ORDER BY approvals.created_at ASC, approvals.id ASC")
	} else {
		query.WriteString(" ORDER BY approvals.created_at DESC, approvals.id DESC")
	}
	query.WriteString(" LIMIT ?")
	args = append(args, filter.Limit+1)
	return query.String(), args, nil
}

func finalizeApprovalListPage(query url.Values, filter approvalListRequest, rows []approvalListRow) ([]approvalItem, pageLinks) {
	hasExtra := len(rows) > filter.Limit
	if hasExtra {
		rows = rows[:filter.Limit]
	}
	if filter.Direction == "prev" {
		for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
			rows[left], rows[right] = rows[right], rows[left]
		}
	}

	items := make([]approvalItem, 0, len(rows))
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
		prevCursor = encodeApprovalListCursor(first.CreatedAt, first.Item.ID)
		nextCursor = encodeApprovalListCursor(last.CreatedAt, last.Item.ID)
	}

	switch filter.Direction {
	case "prev":
		hasPrev = hasExtra
		hasNext = filter.HasCursor
	default:
		hasPrev = filter.HasCursor
		hasNext = hasExtra
	}

	return items, buildPageLinks(pageRecoverApprovals, cloneQuery(query), "cursor", "direction", nextCursor, prevCursor, hasNext, hasPrev)
}

func parseApprovalListCursor(raw string) (approvalListCursor, bool) {
	if raw == "" {
		return approvalListCursor{}, false
	}
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return approvalListCursor{}, false
	}
	return approvalListCursor{CreatedAt: parts[0], ID: parts[1]}, true
}

func encodeApprovalListCursor(createdAt, id string) string {
	if createdAt == "" || id == "" {
		return ""
	}
	return createdAt + "|" + id
}

func approvalStatusClass(status string) string {
	switch status {
	case "pending":
		return "is-approval"
	case "approved":
		return "is-success"
	case "denied":
		return "is-error"
	case "expired":
		return "is-muted"
	default:
		return ""
	}
}

func writeApprovalResolveError(w http.ResponseWriter, wantsJSON bool, status int, message string) {
	if wantsJSON {
		writeJSON(w, status, map[string]string{"message": message})
		return
	}
	http.Error(w, message, status)
}
