package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/sessions"
)

type sessionListResponse struct {
	Sessions   []model.Session `json:"sessions"`
	NextCursor string          `json:"next_cursor,omitempty"`
	PrevCursor string          `json:"prev_cursor,omitempty"`
	HasNext    bool            `json:"has_next"`
	HasPrev    bool            `json:"has_prev"`
}

type connectorDeliveryHealthResponse struct {
	Connectors        []model.ConnectorDeliveryHealth `json:"connectors"`
	RuntimeConnectors []model.ConnectorHealthSnapshot `json:"runtime_connectors,omitempty"`
}

type routeDirectoryResponse struct {
	Routes     []model.RouteDirectoryItem `json:"routes"`
	NextCursor string                     `json:"next_cursor,omitempty"`
	PrevCursor string                     `json:"prev_cursor,omitempty"`
	HasNext    bool                       `json:"has_next"`
	HasPrev    bool                       `json:"has_prev"`
}

type routeCreateRequest struct {
	SessionID   string `json:"session_id"`
	ThreadID    string `json:"thread_id"`
	ConnectorID string `json:"connector_id"`
	AccountID   string `json:"account_id"`
	ExternalID  string `json:"external_id"`
}

type routeCreateResponse struct {
	Route model.RouteDirectoryItem `json:"route"`
}

type routeDeactivateResponse struct {
	Route model.RouteDirectoryItem `json:"route"`
}

type deliveryQueueResponse struct {
	Deliveries []model.DeliveryQueueItem `json:"deliveries"`
	NextCursor string                    `json:"next_cursor,omitempty"`
	PrevCursor string                    `json:"prev_cursor,omitempty"`
	HasNext    bool                      `json:"has_next"`
	HasPrev    bool                      `json:"has_prev"`
}

type sessionMailboxResponse struct {
	Session          model.Session           `json:"session"`
	Messages         []model.SessionMessage  `json:"messages"`
	Route            *model.SessionRoute     `json:"route,omitempty"`
	Deliveries       []model.OutboundIntent  `json:"deliveries,omitempty"`
	DeliveryFailures []model.DeliveryFailure `json:"delivery_failures,omitempty"`
}

type sessionSendRequest struct {
	Body          string `json:"body"`
	FromSessionID string `json:"from_session_id"`
}

type sessionSendResponse struct {
	Run model.Run `json:"run"`
}

type sessionRetryDeliveryResponse struct {
	Delivery model.OutboundIntent `json:"delivery"`
}

func (s *Server) handleSessionsIndex(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	activeProject, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load active project", http.StatusInternalServerError)
		return
	}
	filter := sessionListFilterFromRequest(r, 50)
	filter.ProjectID = activeProject.ID
	filter.WorkspaceRoot = activeProject.PrimaryPath
	page, err := s.rt.ListAllSessionsPage(r.Context(), filter)
	if err != nil {
		http.Error(w, "failed to load sessions", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, sessionListResponse{
		Sessions:   page.Items,
		NextCursor: page.NextCursor,
		PrevCursor: page.PrevCursor,
		HasNext:    page.HasNext,
		HasPrev:    page.HasPrev,
	})
}

func (s *Server) handleDeliveryHealth(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	list, err := s.rt.ConnectorDeliveryHealth(r.Context())
	if err != nil {
		http.Error(w, "failed to load delivery health", http.StatusInternalServerError)
		return
	}

	resp := connectorDeliveryHealthResponse{Connectors: list}
	if s.connectorHealth != nil {
		resp.RuntimeConnectors, err = s.connectorHealth.ConnectorHealth(r.Context())
		if err != nil {
			http.Error(w, "failed to load runtime connector health", http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRoutesIndex(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	activeProject, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load active project", http.StatusInternalServerError)
		return
	}
	filter := routeListFilterFromRequest(r, 50)
	filter.ProjectID = activeProject.ID
	filter.WorkspaceRoot = activeProject.PrimaryPath
	page, err := s.rt.ListRoutesPage(r.Context(), filter)
	if err != nil {
		http.Error(w, "failed to load routes", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, routeDirectoryResponse{
		Routes:     page.Items,
		NextCursor: page.NextCursor,
		PrevCursor: page.PrevCursor,
		HasNext:    page.HasNext,
		HasPrev:    page.HasPrev,
	})
}

func (s *Server) handleRouteCreate(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	var req routeCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" || req.ConnectorID == "" || req.ExternalID == "" {
		http.Error(w, "session_id, connector_id, and external_id are required", http.StatusBadRequest)
		return
	}

	route, err := s.rt.BindRoute(r.Context(), runtime.BindRouteCommand{
		SessionID:   req.SessionID,
		ThreadID:    req.ThreadID,
		ConnectorID: req.ConnectorID,
		AccountID:   req.AccountID,
		ExternalID:  req.ExternalID,
	})
	if err != nil {
		switch {
		case errors.Is(err, sessions.ErrSessionNotFound):
			http.NotFound(w, r)
			return
		default:
			http.Error(w, "failed to create route", http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, http.StatusOK, routeCreateResponse{Route: route})
}

func (s *Server) handleDeliveryIndex(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	activeProject, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load active project", http.StatusInternalServerError)
		return
	}
	filter := deliveryQueueFilterFromRequest(r, 50)
	filter.ProjectID = activeProject.ID
	filter.WorkspaceRoot = activeProject.PrimaryPath
	page, err := s.rt.ListDeliveriesPage(r.Context(), filter)
	if err != nil {
		http.Error(w, "failed to load deliveries", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, deliveryQueueResponse{
		Deliveries: page.Items,
		NextCursor: page.NextCursor,
		PrevCursor: page.PrevCursor,
		HasNext:    page.HasNext,
		HasPrev:    page.HasPrev,
	})
}

func (s *Server) handleRouteDeactivate(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}
	visible, err := s.routeVisibleInActiveProject(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "failed to load route", http.StatusInternalServerError)
		return
	}
	if !visible {
		http.NotFound(w, r)
		return
	}

	route, err := s.rt.DeactivateRoute(r.Context(), r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, runtime.ErrRouteNotFound):
			http.NotFound(w, r)
			return
		case errors.Is(err, runtime.ErrRouteNotActive):
			writeJSON(w, http.StatusConflict, map[string]string{
				"message": "Only active routes can be deactivated.",
			})
			return
		default:
			http.Error(w, "failed to deactivate route", http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, http.StatusOK, routeDeactivateResponse{Route: route})
}

func (s *Server) handleRouteSend(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}
	visible, err := s.routeVisibleInActiveProject(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "failed to load route", http.StatusInternalServerError)
		return
	}
	if !visible {
		http.NotFound(w, r)
		return
	}

	var req sessionSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Body == "" {
		http.Error(w, "body is required", http.StatusBadRequest)
		return
	}

	run, err := s.rt.SendRoute(r.Context(), r.PathValue("id"), req.FromSessionID, req.Body)
	if err != nil {
		switch {
		case errors.Is(err, runtime.ErrRouteNotFound):
			http.NotFound(w, r)
			return
		case errors.Is(err, runtime.ErrRouteNotActive):
			writeJSON(w, http.StatusConflict, map[string]string{
				"message": "Only active routes can receive messages.",
			})
			return
		case errors.Is(err, conversations.ErrConversationBusy):
			writeJSON(w, http.StatusConflict, map[string]string{
				"message": "The target session is busy with another active root run.",
			})
			return
		default:
			http.Error(w, "failed to send route message", http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, http.StatusOK, sessionSendResponse{Run: run})
}

func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	sessionID := r.PathValue("id")
	visible, err := s.sessionVisibleInActiveProject(r.Context(), sessionID)
	if err != nil {
		http.Error(w, "failed to load session", http.StatusInternalServerError)
		return
	}
	if !visible {
		http.NotFound(w, r)
		return
	}
	session, messages, err := s.rt.SessionHistory(r.Context(), sessionID, requestLimit(r, 100))
	if err != nil {
		if errors.Is(err, sessions.ErrSessionNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to load session", http.StatusInternalServerError)
		return
	}

	var route *model.SessionRoute
	loadedRoute, err := sessions.NewService(s.db, nil).LoadRouteBySession(r.Context(), sessionID)
	if err == nil {
		route = &loadedRoute
	} else if !errors.Is(err, sessions.ErrSessionRouteNotFound) {
		http.Error(w, "failed to load session route", http.StatusInternalServerError)
		return
	}

	deliveries, failures, err := s.rt.SessionDeliveryState(r.Context(), sessionID, requestLimit(r, 25))
	if err != nil {
		http.Error(w, "failed to load session delivery state", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, sessionMailboxResponse{
		Session:          session,
		Messages:         messages,
		Route:            route,
		Deliveries:       deliveries,
		DeliveryFailures: failures,
	})
}

func (s *Server) handleSessionSend(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}
	visible, err := s.sessionVisibleInActiveProject(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "failed to load session", http.StatusInternalServerError)
		return
	}
	if !visible {
		http.NotFound(w, r)
		return
	}

	var req sessionSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Body == "" {
		http.Error(w, "body is required", http.StatusBadRequest)
		return
	}

	run, err := s.rt.SendSession(r.Context(), runtime.SendSessionCommand{
		FromSessionID: req.FromSessionID,
		ToSessionID:   r.PathValue("id"),
		Body:          req.Body,
	})
	if err != nil {
		switch {
		case errors.Is(err, sessions.ErrSessionNotFound):
			http.NotFound(w, r)
			return
		case errors.Is(err, conversations.ErrConversationBusy):
			writeJSON(w, http.StatusConflict, map[string]string{
				"message": "The target session is busy with another active root run.",
			})
			return
		default:
			http.Error(w, "failed to send session message", http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, http.StatusOK, sessionSendResponse{Run: run})
}

func (s *Server) handleSessionRetryDelivery(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}
	visible, err := s.sessionVisibleInActiveProject(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "failed to load session", http.StatusInternalServerError)
		return
	}
	if !visible {
		http.NotFound(w, r)
		return
	}

	delivery, err := s.rt.RetrySessionDelivery(r.Context(), r.PathValue("id"), r.PathValue("delivery_id"))
	if err != nil {
		switch {
		case errors.Is(err, sessions.ErrSessionNotFound), errors.Is(err, runtime.ErrDeliveryNotFound):
			http.NotFound(w, r)
			return
		case errors.Is(err, runtime.ErrDeliveryNotRetryable):
			writeJSON(w, http.StatusConflict, map[string]string{
				"message": "Only terminal deliveries can be retried.",
			})
			return
		default:
			http.Error(w, "failed to retry delivery", http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, http.StatusOK, sessionRetryDeliveryResponse{Delivery: delivery})
}

func (s *Server) handleDeliveryRetry(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}
	visible, err := s.deliveryVisibleInActiveProject(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "failed to load delivery", http.StatusInternalServerError)
		return
	}
	if !visible {
		http.NotFound(w, r)
		return
	}

	delivery, err := s.rt.RetryDelivery(r.Context(), r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, runtime.ErrDeliveryNotFound):
			http.NotFound(w, r)
			return
		case errors.Is(err, runtime.ErrDeliveryNotRetryable):
			writeJSON(w, http.StatusConflict, map[string]string{
				"message": "Only terminal deliveries can be retried.",
			})
			return
		default:
			http.Error(w, "failed to retry delivery", http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, http.StatusOK, sessionRetryDeliveryResponse{Delivery: delivery})
}

func requestLimit(r *http.Request, fallback int) int {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func sessionListFilterFromRequest(r *http.Request, fallbackLimit int) sessions.SessionListFilter {
	return sessions.SessionListFilter{
		ConversationID: strings.TrimSpace(r.URL.Query().Get("conversation_id")),
		AgentID:        strings.TrimSpace(r.URL.Query().Get("agent_id")),
		Role:           strings.TrimSpace(r.URL.Query().Get("role")),
		Status:         strings.TrimSpace(r.URL.Query().Get("status")),
		ConnectorID:    strings.TrimSpace(r.URL.Query().Get("connector_id")),
		Query:          strings.TrimSpace(r.URL.Query().Get("q")),
		Binding:        strings.TrimSpace(r.URL.Query().Get("binding")),
		Cursor:         strings.TrimSpace(r.URL.Query().Get("cursor")),
		Direction:      strings.TrimSpace(r.URL.Query().Get("direction")),
		Limit:          requestLimit(r, fallbackLimit),
	}
}

func routeListFilterFromRequest(r *http.Request, fallbackLimit int) sessions.RouteListFilter {
	return sessions.RouteListFilter{
		ConnectorID: strings.TrimSpace(r.URL.Query().Get("connector_id")),
		Status:      strings.TrimSpace(r.URL.Query().Get("status")),
		Query:       strings.TrimSpace(r.URL.Query().Get("q")),
		Cursor:      strings.TrimSpace(r.URL.Query().Get("cursor")),
		Direction:   strings.TrimSpace(r.URL.Query().Get("direction")),
		Limit:       requestLimit(r, fallbackLimit),
	}
}

func deliveryQueueFilterFromRequest(r *http.Request, fallbackLimit int) sessions.DeliveryQueueFilter {
	return sessions.DeliveryQueueFilter{
		ConnectorID: strings.TrimSpace(r.URL.Query().Get("connector_id")),
		SessionID:   strings.TrimSpace(r.URL.Query().Get("session_id")),
		Status:      strings.TrimSpace(r.URL.Query().Get("status")),
		Query:       strings.TrimSpace(r.URL.Query().Get("q")),
		Cursor:      strings.TrimSpace(r.URL.Query().Get("cursor")),
		Direction:   strings.TrimSpace(r.URL.Query().Get("direction")),
		Limit:       requestLimit(r, fallbackLimit),
	}
}

func requestBool(r *http.Request, key string) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
