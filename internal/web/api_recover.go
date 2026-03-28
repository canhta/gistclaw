package web

import "net/http"

type pageLinksResponse struct {
	NextURL string `json:"next_url,omitempty"`
	PrevURL string `json:"prev_url,omitempty"`
	HasNext bool   `json:"has_next"`
	HasPrev bool   `json:"has_prev"`
}

type recoverIndexResponse struct {
	Summary        recoverSummaryResponse    `json:"summary"`
	Approvals      []recoverApprovalResponse `json:"approvals"`
	ApprovalPaging pageLinksResponse         `json:"approval_paging"`
	Repair         recoverRepairResponse     `json:"repair"`
}

type recoverSummaryResponse struct {
	OpenApprovals      int `json:"open_approvals"`
	PendingApprovals   int `json:"pending_approvals"`
	ConnectorCount     int `json:"connector_count"`
	ActiveRoutes       int `json:"active_routes"`
	TerminalDeliveries int `json:"terminal_deliveries"`
}

type recoverApprovalResponse struct {
	ID              string `json:"id"`
	RunID           string `json:"run_id"`
	ToolName        string `json:"tool_name"`
	BindingSummary  string `json:"binding_summary"`
	Status          string `json:"status"`
	StatusLabel     string `json:"status_label"`
	StatusClass     string `json:"status_class"`
	ResolvedBy      string `json:"resolved_by,omitempty"`
	ResolvedAtLabel string `json:"resolved_at_label,omitempty"`
}

type recoverRepairResponse struct {
	ConnectorCount    int                             `json:"connector_count"`
	Filters           recoverRepairFiltersResponse    `json:"filters"`
	Health            []recoverDeliveryHealthResponse `json:"health"`
	RuntimeConnectors []recoverRuntimeHealthResponse  `json:"runtime_connectors"`
	ActiveRoutes      []recoverRouteResponse          `json:"active_routes"`
	ActivePaging      pageLinksResponse               `json:"active_paging"`
	RouteHistory      []recoverRouteResponse          `json:"route_history"`
	HistoryPaging     pageLinksResponse               `json:"history_paging"`
	Deliveries        []recoverDeliveryResponse       `json:"deliveries"`
	DeliveryPaging    pageLinksResponse               `json:"delivery_paging"`
}

type recoverRepairFiltersResponse struct {
	Query          string `json:"query"`
	ConnectorID    string `json:"connector_id"`
	RouteStatus    string `json:"route_status"`
	DeliveryStatus string `json:"delivery_status"`
	ActiveLimit    int    `json:"active_limit"`
	HistoryLimit   int    `json:"history_limit"`
	DeliveryLimit  int    `json:"delivery_limit"`
}

type recoverDeliveryHealthResponse struct {
	ConnectorID   string `json:"connector_id"`
	PendingCount  int    `json:"pending_count"`
	RetryingCount int    `json:"retrying_count"`
	TerminalCount int    `json:"terminal_count"`
	StateClass    string `json:"state_class"`
}

type recoverRuntimeHealthResponse struct {
	ConnectorID      string `json:"connector_id"`
	State            string `json:"state"`
	StateLabel       string `json:"state_label"`
	StateClass       string `json:"state_class"`
	Summary          string `json:"summary"`
	CheckedAtLabel   string `json:"checked_at_label,omitempty"`
	RestartSuggested bool   `json:"restart_suggested"`
}

type recoverRouteResponse struct {
	ID                string `json:"id"`
	ConnectorID       string `json:"connector_id"`
	ExternalID        string `json:"external_id"`
	ThreadID          string `json:"thread_id"`
	SessionID         string `json:"session_id"`
	ConversationID    string `json:"conversation_id"`
	AgentID           string `json:"agent_id"`
	RoleLabel         string `json:"role_label"`
	StatusLabel       string `json:"status_label"`
	DeactivatedLabel  string `json:"deactivated_label,omitempty"`
	DeactivationNote  string `json:"deactivation_note,omitempty"`
	ReplacedByRouteID string `json:"replaced_by_route_id,omitempty"`
}

type recoverDeliveryResponse struct {
	ID            string                `json:"id"`
	RunID         string                `json:"run_id"`
	SessionID     string                `json:"session_id"`
	ConnectorID   string                `json:"connector_id"`
	ChatID        string                `json:"chat_id"`
	Message       runStructuredTextView `json:"message"`
	Status        string                `json:"status"`
	StatusLabel   string                `json:"status_label"`
	AttemptsLabel string                `json:"attempts_label"`
}

func (s *Server) handleRecoverIndex(w http.ResponseWriter, r *http.Request) {
	approvals, err := s.loadApprovalsPageData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	repair, err := s.loadRoutesDeliveriesPageData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := recoverIndexResponse{
		Summary:        buildRecoverSummary(approvals, repair),
		Approvals:      buildRecoverApprovalResponses(approvals.Approvals),
		ApprovalPaging: pageLinksResponseFrom(approvals.Paging),
		Repair: recoverRepairResponse{
			ConnectorCount:    repair.ConnectorCount,
			Filters:           buildRecoverRepairFiltersResponse(repair.Filters),
			Health:            buildRecoverDeliveryHealthResponses(repair.Health),
			RuntimeConnectors: buildRecoverRuntimeHealthResponses(repair.RuntimeHealth),
			ActiveRoutes:      buildRecoverRouteResponses(repair.ActiveRoutes),
			ActivePaging:      pageLinksResponseFrom(repair.ActivePaging),
			RouteHistory:      buildRecoverRouteResponses(repair.RouteHistory),
			HistoryPaging:     pageLinksResponseFrom(repair.HistoryPaging),
			Deliveries:        buildRecoverDeliveryResponses(repair.Deliveries),
			DeliveryPaging:    pageLinksResponseFrom(repair.DeliveryPaging),
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func buildRecoverSummary(approvals approvalsPageData, repair routesDeliveriesPageData) recoverSummaryResponse {
	summary := recoverSummaryResponse{
		ConnectorCount: repair.ConnectorCount,
		ActiveRoutes:   len(repair.ActiveRoutes),
	}
	for _, item := range approvals.Approvals {
		switch item.Status {
		case "pending":
			summary.PendingApprovals++
			summary.OpenApprovals++
		case "expired":
			summary.OpenApprovals++
		}
	}
	for _, item := range repair.Health {
		summary.TerminalDeliveries += item.TerminalCount
	}
	return summary
}

func buildRecoverApprovalResponses(items []approvalItem) []recoverApprovalResponse {
	resp := make([]recoverApprovalResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, recoverApprovalResponse{
			ID:              item.ID,
			RunID:           item.RunID,
			ToolName:        item.ToolName,
			BindingSummary:  item.BindingSummary,
			Status:          item.Status,
			StatusLabel:     item.StatusLabel,
			StatusClass:     item.StatusClass,
			ResolvedBy:      item.ResolvedBy,
			ResolvedAtLabel: item.ResolvedAtLabel,
		})
	}
	return resp
}

func buildRecoverRepairFiltersResponse(filters routesDeliveriesPageFilters) recoverRepairFiltersResponse {
	return recoverRepairFiltersResponse(filters)
}

func buildRecoverDeliveryHealthResponses(items []routesDeliveriesDeliveryHealthView) []recoverDeliveryHealthResponse {
	resp := make([]recoverDeliveryHealthResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, recoverDeliveryHealthResponse(item))
	}
	return resp
}

func buildRecoverRuntimeHealthResponses(items []routesDeliveriesRuntimeHealthView) []recoverRuntimeHealthResponse {
	resp := make([]recoverRuntimeHealthResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, recoverRuntimeHealthResponse(item))
	}
	return resp
}

func buildRecoverRouteResponses(items []routesDeliveriesRouteView) []recoverRouteResponse {
	resp := make([]recoverRouteResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, recoverRouteResponse(item))
	}
	return resp
}

func buildRecoverDeliveryResponses(items []routesDeliveriesDeliveryView) []recoverDeliveryResponse {
	resp := make([]recoverDeliveryResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, recoverDeliveryResponse(item))
	}
	return resp
}

func pageLinksResponseFrom(links pageLinks) pageLinksResponse {
	return pageLinksResponse(links)
}
