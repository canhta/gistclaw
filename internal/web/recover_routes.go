package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/sessions"
)

type routesDeliveriesPageData struct {
	ConnectorCount int
	Health         []routesDeliveriesDeliveryHealthView
	RuntimeHealth  []routesDeliveriesRuntimeHealthView
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

type routesDeliveriesDeliveryHealthView struct {
	ConnectorID   string
	PendingCount  int
	RetryingCount int
	TerminalCount int
	StateClass    string
}

type routesDeliveriesRuntimeHealthView struct {
	ConnectorID      string
	State            string
	StateLabel       string
	StateClass       string
	Summary          string
	CheckedAtLabel   string
	RestartSuggested bool
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
		ProjectID:   activeProject.ID,
		ConnectorID: filters.ConnectorID,
		Query:       filters.Query,
	}
	baseDeliveryFilter := sessions.DeliveryQueueFilter{
		ProjectID:   activeProject.ID,
		ConnectorID: filters.ConnectorID,
		Status:      filters.DeliveryStatus,
		Query:       filters.Query,
	}

	health, err := s.rt.ConnectorDeliveryHealth(r.Context())
	if err != nil {
		return routesDeliveriesPageData{}, errors.New("failed to load connector delivery health")
	}
	health = filterConnectorHealth(health, filters)

	runtimeHealth, err := s.loadRuntimeConnectorHealth(r.Context(), filters)
	if err != nil {
		return routesDeliveriesPageData{}, err
	}

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
		historyPaging = buildPageLinks("/api/recover", cloneQuery(r.URL.Query()), "history_cursor", "history_direction", historyPage.NextCursor, historyPage.PrevCursor, historyPage.HasNext, historyPage.HasPrev)
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
		activePaging = buildPageLinks("/api/recover", cloneQuery(r.URL.Query()), "active_cursor", "active_direction", activePage.NextCursor, activePage.PrevCursor, activePage.HasNext, activePage.HasPrev)
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
			historyPaging = buildPageLinks("/api/recover", cloneQuery(r.URL.Query()), "history_cursor", "history_direction", historyPage.NextCursor, historyPage.PrevCursor, historyPage.HasNext, historyPage.HasPrev)
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
		ConnectorCount: connectorHealthCount(health, runtimeHealth),
		Health:         buildRoutesDeliveriesDeliveryHealthViews(health),
		RuntimeHealth:  buildRoutesDeliveriesRuntimeHealthViews(runtimeHealth),
		ActiveRoutes:   buildRoutesDeliveriesRouteViews(activeRoutes),
		ActivePaging:   activePaging,
		RouteHistory:   buildRoutesDeliveriesRouteViews(routeHistory),
		HistoryPaging:  historyPaging,
		Deliveries:     buildRoutesDeliveriesDeliveryViews(deliveryPage.Items),
		DeliveryPaging: buildPageLinks("/api/recover", cloneQuery(r.URL.Query()), "delivery_cursor", "delivery_direction", deliveryPage.NextCursor, deliveryPage.PrevCursor, deliveryPage.HasNext, deliveryPage.HasPrev),
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

func (s *Server) loadRuntimeConnectorHealth(ctx context.Context, filters routesDeliveriesPageFilters) ([]model.ConnectorHealthSnapshot, error) {
	if s.connectorHealth == nil {
		return nil, nil
	}

	list, err := s.connectorHealth.ConnectorHealth(ctx)
	if err != nil {
		return nil, errors.New("failed to load runtime connector health")
	}
	return filterRuntimeConnectorHealth(list, filters), nil
}

func filterRuntimeConnectorHealth(list []model.ConnectorHealthSnapshot, filters routesDeliveriesPageFilters) []model.ConnectorHealthSnapshot {
	if filters.ConnectorID == "" && filters.Query == "" {
		return list
	}

	filtered := make([]model.ConnectorHealthSnapshot, 0, len(list))
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

func connectorHealthCount(queue []model.ConnectorDeliveryHealth, runtime []model.ConnectorHealthSnapshot) int {
	seen := make(map[string]struct{}, len(queue)+len(runtime))
	for _, item := range queue {
		seen[item.ConnectorID] = struct{}{}
	}
	for _, item := range runtime {
		seen[item.ConnectorID] = struct{}{}
	}
	return len(seen)
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

func buildRoutesDeliveriesDeliveryHealthViews(items []model.ConnectorDeliveryHealth) []routesDeliveriesDeliveryHealthView {
	views := make([]routesDeliveriesDeliveryHealthView, 0, len(items))
	for _, item := range items {
		stateClass := "is-active"
		if item.TerminalCount > 0 {
			stateClass = "is-error"
		} else if item.RetryingCount > 0 {
			stateClass = "is-approval"
		}
		views = append(views, routesDeliveriesDeliveryHealthView{
			ConnectorID:   item.ConnectorID,
			PendingCount:  item.PendingCount,
			RetryingCount: item.RetryingCount,
			TerminalCount: item.TerminalCount,
			StateClass:    stateClass,
		})
	}
	return views
}

func buildRoutesDeliveriesRuntimeHealthViews(items []model.ConnectorHealthSnapshot) []routesDeliveriesRuntimeHealthView {
	views := make([]routesDeliveriesRuntimeHealthView, 0, len(items))
	for _, item := range items {
		checkedAtLabel := ""
		if !item.CheckedAt.IsZero() {
			checkedAt := item.CheckedAt
			checkedAtLabel = formatOptionalWebTimestamp(&checkedAt)
		}
		stateClass := "is-muted"
		switch item.State {
		case model.ConnectorHealthHealthy:
			stateClass = "is-active"
		case model.ConnectorHealthDegraded:
			stateClass = "is-approval"
		}
		views = append(views, routesDeliveriesRuntimeHealthView{
			ConnectorID:      item.ConnectorID,
			State:            string(item.State),
			StateLabel:       humanizeWebLabel(string(item.State)),
			StateClass:       stateClass,
			Summary:          item.Summary,
			CheckedAtLabel:   checkedAtLabel,
			RestartSuggested: item.RestartSuggested,
		})
	}
	return views
}
