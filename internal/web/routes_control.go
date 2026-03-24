package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/sessions"
)

type controlPageData struct {
	Health         []model.ConnectorDeliveryHealth
	ActiveRoutes   []model.RouteDirectoryItem
	ActivePaging   pageLinks
	RouteHistory   []model.RouteDirectoryItem
	HistoryPaging  pageLinks
	Deliveries     []model.DeliveryQueueItem
	DeliveryPaging pageLinks
	Filters        controlPageFilters
	Error          string
}

type controlPageFilters struct {
	Query          string
	ConnectorID    string
	RouteStatus    string
	DeliveryStatus string
	ActiveLimit    int
	HistoryLimit   int
	DeliveryLimit  int
}

func (s *Server) handleControlPage(w http.ResponseWriter, r *http.Request) {
	data, err := s.loadControlPageData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "Control", "control_body", data)
}

func (s *Server) handleControlRouteSend(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		s.renderControlError(w, r, http.StatusUnprocessableEntity, "Route send body is required.")
		return
	}

	run, err := s.rt.SendRoute(r.Context(), r.PathValue("id"), strings.TrimSpace(r.FormValue("from_session_id")), body)
	if err != nil {
		switch {
		case errors.Is(err, runtime.ErrRouteNotFound):
			http.NotFound(w, r)
		case errors.Is(err, runtime.ErrRouteNotActive):
			s.renderControlError(w, r, http.StatusConflict, "Only active routes can receive messages.")
		case errors.Is(err, conversations.ErrConversationBusy):
			s.renderControlError(w, r, http.StatusConflict, "The target session already has an active root run.")
		default:
			http.Error(w, "failed to send route message", http.StatusInternalServerError)
		}
		return
	}

	http.Redirect(w, r, "/runs/"+run.ID, http.StatusSeeOther)
}

func (s *Server) handleControlRouteDeactivate(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	_, err := s.rt.DeactivateRoute(r.Context(), r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, runtime.ErrRouteNotFound):
			http.NotFound(w, r)
		case errors.Is(err, runtime.ErrRouteNotActive):
			s.renderControlError(w, r, http.StatusConflict, "Only active routes can be deactivated.")
		default:
			http.Error(w, "failed to deactivate route", http.StatusInternalServerError)
		}
		return
	}

	http.Redirect(w, r, "/control", http.StatusSeeOther)
}

func (s *Server) handleControlDeliveryRetry(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	_, err := s.rt.RetryDelivery(r.Context(), r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, runtime.ErrDeliveryNotFound):
			http.NotFound(w, r)
		case errors.Is(err, runtime.ErrDeliveryNotRetryable):
			s.renderControlError(w, r, http.StatusConflict, "Only terminal deliveries can be retried.")
		default:
			http.Error(w, "failed to retry delivery", http.StatusInternalServerError)
		}
		return
	}

	http.Redirect(w, r, "/control", http.StatusSeeOther)
}

func (s *Server) renderControlError(w http.ResponseWriter, r *http.Request, status int, message string) {
	data, err := s.loadControlPageData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.Error = message
	s.renderTemplateStatus(w, status, "Control", "control_body", data)
}

func (s *Server) loadControlPageData(r *http.Request) (controlPageData, error) {
	if s.rt == nil {
		return controlPageData{}, errors.New("runtime not configured")
	}

	filters := controlPageFilters{
		Query:          strings.TrimSpace(r.URL.Query().Get("q")),
		ConnectorID:    strings.TrimSpace(r.URL.Query().Get("connector_id")),
		RouteStatus:    normalizeControlStatus(r.URL.Query().Get("route_status")),
		DeliveryStatus: normalizeControlStatus(r.URL.Query().Get("delivery_status")),
		ActiveLimit:    requestNamedLimit(r, "active_limit", 50),
		HistoryLimit:   requestNamedLimit(r, "history_limit", 25),
		DeliveryLimit:  requestNamedLimit(r, "delivery_limit", 50),
	}
	baseRouteFilter := sessions.RouteListFilter{
		ConnectorID: filters.ConnectorID,
		Query:       filters.Query,
	}
	baseDeliveryFilter := sessions.DeliveryQueueFilter{
		ConnectorID: filters.ConnectorID,
		Status:      filters.DeliveryStatus,
		Query:       filters.Query,
	}

	health, err := s.rt.ConnectorDeliveryHealth(r.Context())
	if err != nil {
		return controlPageData{}, errors.New("failed to load connector delivery health")
	}
	health = filterConnectorHealth(health, filters)

	var (
		activeRoutes  []model.RouteDirectoryItem
		activePaging  pageLinks
		routeHistory  []model.RouteDirectoryItem
		historyPaging pageLinks
	)
	switch filters.RouteStatus {
	case "inactive":
		historyFilter := baseRouteFilter
		historyFilter.Status = "inactive"
		historyFilter.Limit = filters.HistoryLimit
		historyFilter.Cursor = strings.TrimSpace(r.URL.Query().Get("history_cursor"))
		historyFilter.Direction = strings.TrimSpace(r.URL.Query().Get("history_direction"))
		historyPage, err := s.rt.ListRoutesPage(r.Context(), historyFilter)
		if err != nil {
			return controlPageData{}, errors.New("failed to load route history")
		}
		routeHistory = historyPage.Items
		historyPaging = buildPageLinks("/control", cloneQuery(r.URL.Query()), "history_cursor", "history_direction", historyPage.NextCursor, historyPage.PrevCursor, historyPage.HasNext, historyPage.HasPrev)
	default:
		activeFilter := baseRouteFilter
		activeFilter.Status = "active"
		activeFilter.Limit = filters.ActiveLimit
		activeFilter.Cursor = strings.TrimSpace(r.URL.Query().Get("active_cursor"))
		activeFilter.Direction = strings.TrimSpace(r.URL.Query().Get("active_direction"))
		activePage, err := s.rt.ListRoutesPage(r.Context(), activeFilter)
		if err != nil {
			return controlPageData{}, errors.New("failed to load active routes")
		}
		activeRoutes = activePage.Items
		activePaging = buildPageLinks("/control", cloneQuery(r.URL.Query()), "active_cursor", "active_direction", activePage.NextCursor, activePage.PrevCursor, activePage.HasNext, activePage.HasPrev)
		if filters.RouteStatus == "all" {
			historyFilter := baseRouteFilter
			historyFilter.Status = "inactive"
			historyFilter.Limit = filters.HistoryLimit
			historyFilter.Cursor = strings.TrimSpace(r.URL.Query().Get("history_cursor"))
			historyFilter.Direction = strings.TrimSpace(r.URL.Query().Get("history_direction"))
			historyPage, err := s.rt.ListRoutesPage(r.Context(), historyFilter)
			if err != nil {
				return controlPageData{}, errors.New("failed to load route history")
			}
			routeHistory = historyPage.Items
			historyPaging = buildPageLinks("/control", cloneQuery(r.URL.Query()), "history_cursor", "history_direction", historyPage.NextCursor, historyPage.PrevCursor, historyPage.HasNext, historyPage.HasPrev)
		}
	}

	deliveryFilter := baseDeliveryFilter
	deliveryFilter.Limit = filters.DeliveryLimit
	deliveryFilter.Cursor = strings.TrimSpace(r.URL.Query().Get("delivery_cursor"))
	deliveryFilter.Direction = strings.TrimSpace(r.URL.Query().Get("delivery_direction"))
	deliveryPage, err := s.rt.ListDeliveriesPage(r.Context(), deliveryFilter)
	if err != nil {
		return controlPageData{}, errors.New("failed to load delivery queue")
	}

	return controlPageData{
		Health:         health,
		ActiveRoutes:   activeRoutes,
		ActivePaging:   activePaging,
		RouteHistory:   routeHistory,
		HistoryPaging:  historyPaging,
		Deliveries:     deliveryPage.Items,
		DeliveryPaging: buildPageLinks("/control", cloneQuery(r.URL.Query()), "delivery_cursor", "delivery_direction", deliveryPage.NextCursor, deliveryPage.PrevCursor, deliveryPage.HasNext, deliveryPage.HasPrev),
		Filters:        filters,
	}, nil
}

func normalizeControlStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "all":
		return "all"
	case "active":
		return "active"
	case "inactive":
		return "inactive"
	case "pending":
		return "pending"
	case "retrying":
		return "retrying"
	case "terminal":
		return "terminal"
	default:
		return "all"
	}
}

func filterConnectorHealth(list []model.ConnectorDeliveryHealth, filters controlPageFilters) []model.ConnectorDeliveryHealth {
	if filters.ConnectorID == "" && filters.Query == "" {
		return list
	}

	filtered := make([]model.ConnectorDeliveryHealth, 0, len(list))
	query := strings.ToLower(filters.Query)
	for _, item := range list {
		if filters.ConnectorID != "" && item.ConnectorID != filters.ConnectorID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(item.ConnectorID), query) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}
