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

type routesDeliveriesPageData struct {
	Health         []model.ConnectorDeliveryHealth
	ActiveRoutes   []routesDeliveriesRouteView
	ActivePaging   pageLinks
	RouteHistory   []routesDeliveriesRouteView
	HistoryPaging  pageLinks
	Deliveries     []routesDeliveriesDeliveryView
	DeliveryPaging pageLinks
	Filters        routesDeliveriesPageFilters
	Error          string
}

type routesDeliveriesPageFilters struct {
	Query          string
	ConnectorID    string
	RouteStatus    string
	DeliveryStatus string
	ActiveLimit    int
	HistoryLimit   int
	DeliveryLimit  int
}

type routesDeliveriesRouteView struct {
	ID                string
	ConnectorID       string
	ExternalID        string
	ThreadID          string
	SessionID         string
	ConversationID    string
	AgentID           string
	RoleLabel         string
	StatusLabel       string
	DeactivatedLabel  string
	DeactivationNote  string
	ReplacedByRouteID string
}

type routesDeliveriesDeliveryView struct {
	ID            string
	RunID         string
	SessionID     string
	ConnectorID   string
	ChatID        string
	Message       runStructuredTextView
	Status        string
	StatusLabel   string
	AttemptsLabel string
}

func (s *Server) handleRoutesDeliveriesPage(w http.ResponseWriter, r *http.Request) {
	data, err := s.loadRoutesDeliveriesPageData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, r, "Routes & Deliveries", "routes_deliveries_body", data)
}

func (s *Server) handleRoutesDeliveriesRouteSend(w http.ResponseWriter, r *http.Request) {
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
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		s.renderRoutesDeliveriesError(w, r, http.StatusUnprocessableEntity, "Route send body is required.")
		return
	}

	run, err := s.rt.SendRoute(r.Context(), r.PathValue("id"), strings.TrimSpace(r.FormValue("from_session_id")), body)
	if err != nil {
		switch {
		case errors.Is(err, runtime.ErrRouteNotFound):
			http.NotFound(w, r)
		case errors.Is(err, runtime.ErrRouteNotActive):
			s.renderRoutesDeliveriesError(w, r, http.StatusConflict, "Only active routes can receive messages.")
		case errors.Is(err, conversations.ErrConversationBusy):
			s.renderRoutesDeliveriesError(w, r, http.StatusConflict, "The target session already has an active root run.")
		default:
			http.Error(w, "failed to send route message", http.StatusInternalServerError)
		}
		return
	}

	http.Redirect(w, r, runDetailPath(run.ID), http.StatusSeeOther)
}

func (s *Server) handleRoutesDeliveriesRouteDeactivate(w http.ResponseWriter, r *http.Request) {
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

	_, err = s.rt.DeactivateRoute(r.Context(), r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, runtime.ErrRouteNotFound):
			http.NotFound(w, r)
		case errors.Is(err, runtime.ErrRouteNotActive):
			s.renderRoutesDeliveriesError(w, r, http.StatusConflict, "Only active routes can be deactivated.")
		default:
			http.Error(w, "failed to deactivate route", http.StatusInternalServerError)
		}
		return
	}

	http.Redirect(w, r, pageRecoverRoutesDeliveries, http.StatusSeeOther)
}

func (s *Server) handleRoutesDeliveriesDeliveryRetry(w http.ResponseWriter, r *http.Request) {
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

	_, err = s.rt.RetryDelivery(r.Context(), r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, runtime.ErrDeliveryNotFound):
			http.NotFound(w, r)
		case errors.Is(err, runtime.ErrDeliveryNotRetryable):
			s.renderRoutesDeliveriesError(w, r, http.StatusConflict, "Only terminal deliveries can be retried.")
		default:
			http.Error(w, "failed to retry delivery", http.StatusInternalServerError)
		}
		return
	}

	http.Redirect(w, r, pageRecoverRoutesDeliveries, http.StatusSeeOther)
}

func (s *Server) renderRoutesDeliveriesError(w http.ResponseWriter, r *http.Request, status int, message string) {
	data, err := s.loadRoutesDeliveriesPageData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.Error = message
	s.renderTemplateStatus(w, r, status, "Routes & Deliveries", "routes_deliveries_body", data)
}

func (s *Server) loadRoutesDeliveriesPageData(r *http.Request) (routesDeliveriesPageData, error) {
	if s.rt == nil {
		return routesDeliveriesPageData{}, errors.New("runtime not configured")
	}

	filters := routesDeliveriesPageFilters{
		Query:          strings.TrimSpace(r.URL.Query().Get("q")),
		ConnectorID:    strings.TrimSpace(r.URL.Query().Get("connector_id")),
		RouteStatus:    normalizeRoutesDeliveriesStatus(r.URL.Query().Get("route_status")),
		DeliveryStatus: normalizeRoutesDeliveriesStatus(r.URL.Query().Get("delivery_status")),
		ActiveLimit:    requestNamedLimit(r, "active_limit", 50),
		HistoryLimit:   requestNamedLimit(r, "history_limit", 25),
		DeliveryLimit:  requestNamedLimit(r, "delivery_limit", 50),
	}
	activeProject, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		return routesDeliveriesPageData{}, errors.New("failed to load active project")
	}
	baseRouteFilter := sessions.RouteListFilter{
		ProjectID:     activeProject.ID,
		WorkspaceRoot: activeProject.WorkspaceRoot,
		ConnectorID:   filters.ConnectorID,
		Query:         filters.Query,
	}
	baseDeliveryFilter := sessions.DeliveryQueueFilter{
		ProjectID:     activeProject.ID,
		WorkspaceRoot: activeProject.WorkspaceRoot,
		ConnectorID:   filters.ConnectorID,
		Status:        filters.DeliveryStatus,
		Query:         filters.Query,
	}

	health, err := s.rt.ConnectorDeliveryHealth(r.Context())
	if err != nil {
		return routesDeliveriesPageData{}, errors.New("failed to load connector delivery health")
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
			return routesDeliveriesPageData{}, errors.New("failed to load route history")
		}
		routeHistory = historyPage.Items
		historyPaging = buildPageLinks(pageRecoverRoutesDeliveries, cloneQuery(r.URL.Query()), "history_cursor", "history_direction", historyPage.NextCursor, historyPage.PrevCursor, historyPage.HasNext, historyPage.HasPrev)
	default:
		activeFilter := baseRouteFilter
		activeFilter.Status = "active"
		activeFilter.Limit = filters.ActiveLimit
		activeFilter.Cursor = strings.TrimSpace(r.URL.Query().Get("active_cursor"))
		activeFilter.Direction = strings.TrimSpace(r.URL.Query().Get("active_direction"))
		activePage, err := s.rt.ListRoutesPage(r.Context(), activeFilter)
		if err != nil {
			return routesDeliveriesPageData{}, errors.New("failed to load active routes")
		}
		activeRoutes = activePage.Items
		activePaging = buildPageLinks(pageRecoverRoutesDeliveries, cloneQuery(r.URL.Query()), "active_cursor", "active_direction", activePage.NextCursor, activePage.PrevCursor, activePage.HasNext, activePage.HasPrev)
		if filters.RouteStatus == "all" {
			historyFilter := baseRouteFilter
			historyFilter.Status = "inactive"
			historyFilter.Limit = filters.HistoryLimit
			historyFilter.Cursor = strings.TrimSpace(r.URL.Query().Get("history_cursor"))
			historyFilter.Direction = strings.TrimSpace(r.URL.Query().Get("history_direction"))
			historyPage, err := s.rt.ListRoutesPage(r.Context(), historyFilter)
			if err != nil {
				return routesDeliveriesPageData{}, errors.New("failed to load route history")
			}
			routeHistory = historyPage.Items
			historyPaging = buildPageLinks(pageRecoverRoutesDeliveries, cloneQuery(r.URL.Query()), "history_cursor", "history_direction", historyPage.NextCursor, historyPage.PrevCursor, historyPage.HasNext, historyPage.HasPrev)
		}
	}

	deliveryFilter := baseDeliveryFilter
	deliveryFilter.Limit = filters.DeliveryLimit
	deliveryFilter.Cursor = strings.TrimSpace(r.URL.Query().Get("delivery_cursor"))
	deliveryFilter.Direction = strings.TrimSpace(r.URL.Query().Get("delivery_direction"))
	deliveryPage, err := s.rt.ListDeliveriesPage(r.Context(), deliveryFilter)
	if err != nil {
		return routesDeliveriesPageData{}, errors.New("failed to load delivery queue")
	}

	return routesDeliveriesPageData{
		Health:         health,
		ActiveRoutes:   buildRoutesDeliveriesRouteViews(activeRoutes),
		ActivePaging:   activePaging,
		RouteHistory:   buildRoutesDeliveriesRouteViews(routeHistory),
		HistoryPaging:  historyPaging,
		Deliveries:     buildRoutesDeliveriesDeliveryViews(deliveryPage.Items),
		DeliveryPaging: buildPageLinks(pageRecoverRoutesDeliveries, cloneQuery(r.URL.Query()), "delivery_cursor", "delivery_direction", deliveryPage.NextCursor, deliveryPage.PrevCursor, deliveryPage.HasNext, deliveryPage.HasPrev),
		Filters:        filters,
	}, nil
}

func normalizeRoutesDeliveriesStatus(raw string) string {
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

func filterConnectorHealth(list []model.ConnectorDeliveryHealth, filters routesDeliveriesPageFilters) []model.ConnectorDeliveryHealth {
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

func buildRoutesDeliveriesRouteViews(items []model.RouteDirectoryItem) []routesDeliveriesRouteView {
	views := make([]routesDeliveriesRouteView, 0, len(items))
	for _, item := range items {
		views = append(views, routesDeliveriesRouteView{
			ID:                item.ID,
			ConnectorID:       item.ConnectorID,
			ExternalID:        item.ExternalID,
			ThreadID:          item.ThreadID,
			SessionID:         item.SessionID,
			ConversationID:    item.ConversationID,
			AgentID:           item.AgentID,
			RoleLabel:         sessionRoleLabel(item.Role),
			StatusLabel:       humanizeWebLabel(item.Status),
			DeactivatedLabel:  formatOptionalWebTimestamp(item.DeactivatedAt),
			DeactivationNote:  item.DeactivationReason,
			ReplacedByRouteID: item.ReplacedByRouteID,
		})
	}
	return views
}

func buildRoutesDeliveriesDeliveryViews(items []model.DeliveryQueueItem) []routesDeliveriesDeliveryView {
	views := make([]routesDeliveriesDeliveryView, 0, len(items))
	for _, item := range items {
		views = append(views, routesDeliveriesDeliveryView{
			ID:            item.ID,
			RunID:         item.RunID,
			SessionID:     item.SessionID,
			ConnectorID:   item.ConnectorID,
			ChatID:        item.ChatID,
			Message:       buildStructuredTextView(item.MessageText, 3),
			Status:        item.Status,
			StatusLabel:   humanizeWebLabel(item.Status),
			AttemptsLabel: attemptLabel(item.Attempts),
		})
	}
	return views
}
