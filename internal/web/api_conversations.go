package web

import (
	"net/http"

	"github.com/canhta/gistclaw/internal/model"
)

type conversationsIndexResponse struct {
	Summary           conversationsSummaryResponse    `json:"summary"`
	Filters           conversationsFiltersResponse    `json:"filters"`
	Sessions          []conversationIndexItemResponse `json:"sessions"`
	Paging            pageLinksResponse               `json:"paging"`
	Health            []recoverDeliveryHealthResponse `json:"health"`
	RuntimeConnectors []recoverRuntimeHealthResponse  `json:"runtime_connectors"`
}

type conversationsSummaryResponse struct {
	SessionCount       int `json:"session_count"`
	ConnectorCount     int `json:"connector_count"`
	TerminalDeliveries int `json:"terminal_deliveries"`
}

type conversationsFiltersResponse struct {
	Query       string `json:"query"`
	AgentID     string `json:"agent_id"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	ConnectorID string `json:"connector_id"`
	Binding     string `json:"binding"`
}

type conversationIndexItemResponse struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	AgentID        string `json:"agent_id"`
	RoleLabel      string `json:"role_label"`
	StatusLabel    string `json:"status_label"`
	UpdatedAtLabel string `json:"updated_at_label"`
}

type conversationDetailResponse struct {
	Session          conversationDetailSessionResponse    `json:"session"`
	Messages         []conversationDetailMessageResponse  `json:"messages"`
	Route            *conversationDetailRouteResponse     `json:"route,omitempty"`
	ActiveRunID      string                               `json:"active_run_id,omitempty"`
	Deliveries       []conversationDetailDeliveryResponse `json:"deliveries"`
	DeliveryFailures []conversationDetailFailureResponse  `json:"delivery_failures"`
}

type conversationDetailSessionResponse struct {
	ID          string `json:"id"`
	AgentID     string `json:"agent_id"`
	RoleLabel   string `json:"role_label"`
	StatusLabel string `json:"status_label"`
}

type conversationDetailMessageResponse struct {
	Kind         string                `json:"kind"`
	KindLabel    string                `json:"kind_label"`
	Body         runStructuredTextView `json:"body"`
	SenderLabel  string                `json:"sender_label"`
	SenderIsMono bool                  `json:"sender_is_mono"`
	SourceRunID  string                `json:"source_run_id,omitempty"`
}

type conversationDetailRouteResponse struct {
	ID               string `json:"id"`
	ConnectorID      string `json:"connector_id"`
	ExternalID       string `json:"external_id"`
	ThreadID         string `json:"thread_id"`
	StatusLabel      string `json:"status_label"`
	CreatedAtLabel   string `json:"created_at_label"`
	DeactivatedLabel string `json:"deactivated_label,omitempty"`
}

type conversationDetailDeliveryResponse struct {
	ID            string                `json:"id"`
	ConnectorID   string                `json:"connector_id"`
	ChatID        string                `json:"chat_id"`
	Message       runStructuredTextView `json:"message"`
	Status        string                `json:"status"`
	StatusLabel   string                `json:"status_label"`
	AttemptsLabel string                `json:"attempts_label"`
}

type conversationDetailFailureResponse struct {
	ID             string `json:"id"`
	ConnectorID    string `json:"connector_id"`
	ChatID         string `json:"chat_id"`
	EventKindLabel string `json:"event_kind_label"`
	Error          string `json:"error"`
	CreatedAtLabel string `json:"created_at_label"`
}

func (s *Server) handleConversationsIndex(w http.ResponseWriter, r *http.Request) {
	data, err := s.loadConversationIndexData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	health, err := s.rt.ConnectorDeliveryHealth(r.Context())
	if err != nil {
		http.Error(w, "failed to load connector delivery health", http.StatusInternalServerError)
		return
	}
	repairFilters := routesDeliveriesPageFilters{
		Query:       data.Filters.Query,
		ConnectorID: data.Filters.ConnectorID,
	}
	health = filterConnectorHealth(health, repairFilters)
	runtimeHealth, err := s.loadRuntimeConnectorHealth(r.Context(), repairFilters)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := conversationsIndexResponse{
		Summary: conversationsSummaryResponse{
			SessionCount:       len(data.Sessions),
			ConnectorCount:     connectorHealthCount(health, runtimeHealth),
			TerminalDeliveries: countTerminalDeliveries(health),
		},
		Filters: conversationsFiltersResponse{
			Query:       data.Filters.Query,
			AgentID:     data.Filters.AgentID,
			Role:        data.Filters.Role,
			Status:      data.Filters.Status,
			ConnectorID: data.Filters.ConnectorID,
			Binding:     data.Filters.Binding,
		},
		Sessions:          buildConversationIndexItemResponses(data.Sessions),
		Paging:            pageLinksResponseFrom(data.Paging),
		Health:            buildRecoverDeliveryHealthResponses(buildRoutesDeliveriesDeliveryHealthViews(health)),
		RuntimeConnectors: buildRecoverRuntimeHealthResponses(buildRoutesDeliveriesRuntimeHealthViews(runtimeHealth)),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleConversationDetail(w http.ResponseWriter, r *http.Request) {
	data, status, err := s.loadConversationDetailData(r)
	if err != nil {
		if status == http.StatusNotFound {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), status)
		return
	}

	writeJSON(w, http.StatusOK, conversationDetailResponse{
		Session:          buildConversationDetailSessionResponse(data.Session),
		Messages:         buildConversationDetailMessageResponses(data.Messages),
		Route:            buildConversationDetailRouteResponse(data.Route),
		ActiveRunID:      data.ActiveRunID,
		Deliveries:       buildConversationDetailDeliveryResponses(data.Deliveries),
		DeliveryFailures: buildConversationDetailFailureResponses(data.DeliveryFailures),
	})
}

func buildConversationIndexItemResponses(items []conversationIndexItemView) []conversationIndexItemResponse {
	resp := make([]conversationIndexItemResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, conversationIndexItemResponse(item))
	}
	return resp
}

func buildConversationDetailSessionResponse(item conversationDetailSessionView) conversationDetailSessionResponse {
	return conversationDetailSessionResponse(item)
}

func buildConversationDetailMessageResponses(items []conversationDetailMessageView) []conversationDetailMessageResponse {
	resp := make([]conversationDetailMessageResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, conversationDetailMessageResponse(item))
	}
	return resp
}

func buildConversationDetailRouteResponse(item *conversationDetailRouteView) *conversationDetailRouteResponse {
	if item == nil {
		return nil
	}
	resp := conversationDetailRouteResponse(*item)
	return &resp
}

func buildConversationDetailDeliveryResponses(items []conversationDetailDeliveryView) []conversationDetailDeliveryResponse {
	resp := make([]conversationDetailDeliveryResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, conversationDetailDeliveryResponse(item))
	}
	return resp
}

func buildConversationDetailFailureResponses(items []conversationDetailFailureView) []conversationDetailFailureResponse {
	resp := make([]conversationDetailFailureResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, conversationDetailFailureResponse(item))
	}
	return resp
}

func countTerminalDeliveries(items []model.ConnectorDeliveryHealth) int {
	total := 0
	for _, item := range items {
		total += item.TerminalCount
	}
	return total
}
