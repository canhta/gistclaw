package web

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type approvalItem struct {
	ID          string
	RunID       string
	ToolName    string
	TargetPath  string
	Status      string
	StatusClass string
	ResolvedBy  string
	CreatedAt   time.Time
	ResolvedAt  *time.Time
}

type approvalsPageData struct {
	Approvals []approvalItem
	Filters   approvalListFilters
	Paging    pageLinks
	Error     string
}

func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	filter := approvalListFilterFromRequest(r)
	querySQL, args, err := buildApprovalListQuery(filter)
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
		item.StatusClass = approvalStatusClass(item.Status)
		if resolvedAt.Valid {
			item.ResolvedAt = &resolvedAt.Time
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
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	decision := r.FormValue("decision")
	if decision != "approved" && decision != "denied" {
		http.Error(w, "decision must be approved or denied", http.StatusBadRequest)
		return
	}

	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	if err := s.rt.ResolveApproval(r.Context(), ticketID, decision); err != nil {
		s.renderTemplate(w, r, "Approvals", "approvals_body", approvalsPageData{
			Error: err.Error(),
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

func buildApprovalListQuery(filter approvalListRequest) (string, []any, error) {
	var query strings.Builder
	query.WriteString(`SELECT id, run_id, tool_name, COALESCE(target_path, ''), status, COALESCE(resolved_by, ''), created_at, resolved_at, created_at
	 FROM approvals`)

	clauses := []string{"1=1"}
	args := make([]any, 0, 8)
	if filter.Query != "" {
		like := "%" + filter.Query + "%"
		clauses = append(clauses, "(id LIKE ? OR run_id LIKE ? OR tool_name LIKE ? OR COALESCE(target_path, '') LIKE ?)")
		args = append(args, like, like, like, like)
	}
	switch filter.Status {
	case "", "all":
	case "pending", "expired", "approved", "denied":
		clauses = append(clauses, "status = ?")
		args = append(args, filter.Status)
	case "open":
		clauses = append(clauses, "status IN ('pending', 'expired')")
	default:
		return "", nil, fmt.Errorf("unknown approval status filter %q", filter.Status)
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
