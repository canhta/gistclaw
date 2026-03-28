package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type historyResponse struct {
	Summary    historySummaryResponse    `json:"summary"`
	Filters    historyFiltersResponse    `json:"filters"`
	Paging     pageLinksResponse         `json:"paging"`
	Runs       []workClusterResponse     `json:"runs"`
	Approvals  []historyApprovalResponse `json:"approvals"`
	Deliveries []historyDeliveryResponse `json:"deliveries"`
}

type historySummaryResponse struct {
	RunCount         int `json:"run_count"`
	CompletedRuns    int `json:"completed_runs"`
	RecoveryRuns     int `json:"recovery_runs"`
	ApprovalEvents   int `json:"approval_events"`
	DeliveryOutcomes int `json:"delivery_outcomes"`
}

type historyFiltersResponse struct {
	Query  string `json:"query"`
	Status string `json:"status"`
	Scope  string `json:"scope"`
	Limit  int    `json:"limit"`
}

type historyApprovalResponse struct {
	ID              string `json:"id"`
	RunID           string `json:"run_id"`
	ToolName        string `json:"tool_name"`
	Status          string `json:"status"`
	StatusLabel     string `json:"status_label"`
	ResolvedBy      string `json:"resolved_by"`
	ResolvedAtLabel string `json:"resolved_at_label"`
}

type historyDeliveryResponse struct {
	ID                 string `json:"id"`
	RunID              string `json:"run_id"`
	ConnectorID        string `json:"connector_id"`
	ChatID             string `json:"chat_id"`
	Status             string `json:"status"`
	StatusLabel        string `json:"status_label"`
	AttemptsLabel      string `json:"attempts_label"`
	LastAttemptAtLabel string `json:"last_attempt_at_label"`
	MessagePreview     string `json:"message_preview"`
}

func (s *Server) handleHistoryIndex(w http.ResponseWriter, r *http.Request) {
	pageData, err := s.loadRunsPageData(r.Context(), historyRunQuery(r.URL.Query()))
	if err != nil {
		http.Error(w, "failed to load run history", http.StatusInternalServerError)
		return
	}

	approvals, err := s.loadHistoryApprovals(r.Context(), 12)
	if err != nil {
		http.Error(w, "failed to load approval history", http.StatusInternalServerError)
		return
	}
	deliveries, err := s.loadHistoryDeliveries(r.Context(), 12)
	if err != nil {
		http.Error(w, "failed to load delivery history", http.StatusInternalServerError)
		return
	}

	runs := make([]workClusterResponse, 0, len(pageData.Clusters))
	for _, cluster := range pageData.Clusters {
		runs = append(runs, buildWorkClusterResponse(cluster))
	}

	writeJSON(w, http.StatusOK, historyResponse{
		Summary: buildHistorySummary(runs, approvals, deliveries),
		Filters: historyFiltersResponse{
			Query:  pageData.Filters.Query,
			Status: pageData.Filters.Status,
			Scope:  pageData.Filters.Scope,
			Limit:  pageData.Filters.Limit,
		},
		Paging:     pageLinksResponse{HasNext: pageData.Paging.HasNext, HasPrev: pageData.Paging.HasPrev},
		Runs:       runs,
		Approvals:  approvals,
		Deliveries: deliveries,
	})
}

func historyRunQuery(in url.Values) url.Values {
	query := cloneQuery(in)
	if strings.TrimSpace(query.Get("scope")) == "" {
		query.Set("scope", "all")
	}
	return query
}

func buildHistorySummary(
	runs []workClusterResponse,
	approvals []historyApprovalResponse,
	deliveries []historyDeliveryResponse,
) historySummaryResponse {
	resp := historySummaryResponse{
		RunCount:         len(runs),
		ApprovalEvents:   len(approvals),
		DeliveryOutcomes: len(deliveries),
	}
	for _, run := range runs {
		switch run.Root.Status {
		case "completed":
			resp.CompletedRuns++
		case "failed", "interrupted", "needs_approval":
			resp.RecoveryRuns++
		}
	}
	return resp
}

func (s *Server) loadHistoryApprovals(ctx context.Context, limit int) ([]historyApprovalResponse, error) {
	if limit <= 0 {
		limit = 12
	}

	rows, err := s.db.RawDB().QueryContext(
		ctx,
		`SELECT id, run_id, tool_name, status, COALESCE(resolved_by, ''), COALESCE(resolved_at, created_at)
		   FROM approvals
		  WHERE status != 'pending'
		  ORDER BY COALESCE(resolved_at, created_at) DESC, id DESC
		  LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query history approvals: %w", err)
	}
	defer rows.Close()

	resp := make([]historyApprovalResponse, 0, limit)
	for rows.Next() {
		var item historyApprovalResponse
		var resolvedAt string
		if err := rows.Scan(
			&item.ID,
			&item.RunID,
			&item.ToolName,
			&item.Status,
			&item.ResolvedBy,
			&resolvedAt,
		); err != nil {
			return nil, fmt.Errorf("scan history approval: %w", err)
		}
		item.StatusLabel = humanizeWebLabel(item.Status)
		item.ResolvedAtLabel = formatRunTimestamp(parseRunListTimestamp(resolvedAt))
		resp = append(resp, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history approvals: %w", err)
	}
	return resp, nil
}

func (s *Server) loadHistoryDeliveries(ctx context.Context, limit int) ([]historyDeliveryResponse, error) {
	if limit <= 0 {
		limit = 12
	}

	rows, err := s.db.RawDB().QueryContext(
		ctx,
		`SELECT id, COALESCE(run_id, ''), connector_id, chat_id, status, attempts,
		        COALESCE(last_attempt_at, created_at), COALESCE(message_text, '')
		   FROM outbound_intents
		  WHERE status NOT IN ('pending', 'retrying')
		  ORDER BY COALESCE(last_attempt_at, created_at) DESC, id DESC
		  LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query history deliveries: %w", err)
	}
	defer rows.Close()

	resp := make([]historyDeliveryResponse, 0, limit)
	for rows.Next() {
		var item historyDeliveryResponse
		var attempts int
		var lastAttemptAt string
		if err := rows.Scan(
			&item.ID,
			&item.RunID,
			&item.ConnectorID,
			&item.ChatID,
			&item.Status,
			&attempts,
			&lastAttemptAt,
			&item.MessagePreview,
		); err != nil {
			return nil, fmt.Errorf("scan history delivery: %w", err)
		}
		item.StatusLabel = humanizeWebLabel(item.Status)
		item.AttemptsLabel = attemptLabel(attempts)
		item.LastAttemptAtLabel = formatRunTimestamp(parseRunListTimestamp(lastAttemptAt))
		resp = append(resp, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history deliveries: %w", err)
	}
	return resp, nil
}
