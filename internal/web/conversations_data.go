package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/sessions"
)

type conversationIndexData struct {
	Sessions []conversationIndexItemView
	Filters  conversationIndexFiltersData
	Paging   pageLinks
}

type conversationDetailData struct {
	Session          conversationDetailSessionView
	Messages         []conversationDetailMessageView
	Route            *conversationDetailRouteView
	ActiveRunID      string
	Deliveries       []conversationDetailDeliveryView
	DeliveryFailures []conversationDetailFailureView
}

type conversationIndexItemView struct {
	ID             string
	ConversationID string
	AgentID        string
	RoleLabel      string
	StatusLabel    string
	UpdatedAtLabel string
}

type conversationDetailSessionView struct {
	ID          string
	AgentID     string
	RoleLabel   string
	StatusLabel string
}

type conversationDetailMessageView struct {
	Kind         string
	KindLabel    string
	Body         runStructuredTextView
	SenderLabel  string
	SenderIsMono bool
	SourceRunID  string
}

type conversationDetailRouteView struct {
	ID               string
	ConnectorID      string
	ExternalID       string
	ThreadID         string
	StatusLabel      string
	CreatedAtLabel   string
	DeactivatedLabel string
}

type conversationDetailDeliveryView struct {
	ID            string
	ConnectorID   string
	ChatID        string
	Message       runStructuredTextView
	Status        string
	StatusLabel   string
	AttemptsLabel string
}

type conversationDetailFailureView struct {
	ID             string
	ConnectorID    string
	ChatID         string
	EventKindLabel string
	Error          string
	CreatedAtLabel string
}

type conversationIndexFiltersData struct {
	Query       string
	AgentID     string
	Role        string
	Status      string
	ConnectorID string
	Binding     string
}

func (s *Server) loadConversationIndexData(r *http.Request) (conversationIndexData, error) {
	if s.rt == nil {
		return conversationIndexData{}, errors.New("runtime not configured")
	}

	filter := sessionListFilterFromRequest(r, 100)
	activeProject, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		return conversationIndexData{}, errors.New("failed to load active project")
	}
	filter.ProjectID = activeProject.ID
	page, err := s.rt.ListAllSessionsPage(r.Context(), filter)
	if err != nil {
		return conversationIndexData{}, errors.New("failed to load sessions")
	}

	return conversationIndexData{
		Sessions: buildConversationIndexViews(page.Items),
		Filters: conversationIndexFiltersData{
			Query:       filter.Query,
			AgentID:     filter.AgentID,
			Role:        filter.Role,
			Status:      filter.Status,
			ConnectorID: filter.ConnectorID,
			Binding:     filter.Binding,
		},
		Paging: buildPageLinks(
			"/api/conversations",
			cloneQuery(r.URL.Query()),
			"cursor",
			"direction",
			page.NextCursor,
			page.PrevCursor,
			page.HasNext,
			page.HasPrev,
		),
	}, nil
}

func (s *Server) loadConversationDetailData(r *http.Request) (conversationDetailData, int, error) {
	if s.rt == nil {
		return conversationDetailData{}, http.StatusInternalServerError, errors.New("runtime not configured")
	}

	sessionID := r.PathValue("id")
	visible, err := s.sessionVisibleInActiveProject(r.Context(), sessionID)
	if err != nil {
		return conversationDetailData{}, http.StatusInternalServerError, errors.New("failed to load session")
	}
	if !visible {
		return conversationDetailData{}, http.StatusNotFound, sessions.ErrSessionNotFound
	}
	session, messages, err := s.rt.SessionHistory(r.Context(), sessionID, 100)
	if err != nil {
		if errors.Is(err, sessions.ErrSessionNotFound) {
			return conversationDetailData{}, http.StatusNotFound, err
		}
		return conversationDetailData{}, http.StatusInternalServerError, errors.New("failed to load session")
	}

	var route *model.SessionRoute
	sessionSvc := sessions.NewService(s.db, nil)
	loadedRoute, err := sessionSvc.LoadRouteBySession(r.Context(), sessionID)
	if err == nil {
		route = &loadedRoute
	} else if !errors.Is(err, sessions.ErrSessionRouteNotFound) {
		return conversationDetailData{}, http.StatusInternalServerError, errors.New("failed to load session route")
	}

	activeRun, err := sessionSvc.LoadActiveRunRef(r.Context(), sessionID)
	if err != nil {
		return conversationDetailData{}, http.StatusInternalServerError, errors.New("failed to load active session run")
	}

	deliveries, failures, err := s.rt.SessionDeliveryState(r.Context(), sessionID, 25)
	if err != nil {
		return conversationDetailData{}, http.StatusInternalServerError, errors.New("failed to load session delivery state")
	}

	return conversationDetailData{
		Session:          buildConversationDetailSessionView(session),
		Messages:         buildConversationDetailMessageViews(messages),
		Route:            buildConversationDetailRouteView(route),
		ActiveRunID:      activeRun.ID,
		Deliveries:       buildConversationDetailDeliveryViews(deliveries),
		DeliveryFailures: buildConversationDetailFailureViews(failures),
	}, http.StatusOK, nil
}

func buildConversationIndexViews(items []model.Session) []conversationIndexItemView {
	views := make([]conversationIndexItemView, 0, len(items))
	for _, item := range items {
		views = append(views, conversationIndexItemView{
			ID:             item.ID,
			ConversationID: item.ConversationID,
			AgentID:        item.AgentID,
			RoleLabel:      sessionRoleLabel(item.Role),
			StatusLabel:    humanizeWebLabel(item.Status),
			UpdatedAtLabel: formatWebTimestamp(item.UpdatedAt),
		})
	}
	return views
}

func buildConversationDetailSessionView(item model.Session) conversationDetailSessionView {
	return conversationDetailSessionView{
		ID:          item.ID,
		AgentID:     item.AgentID,
		RoleLabel:   sessionRoleSummaryLabel(item.Role),
		StatusLabel: humanizeWebLabel(item.Status),
	}
}

func buildConversationDetailMessageViews(items []model.SessionMessage) []conversationDetailMessageView {
	views := make([]conversationDetailMessageView, 0, len(items))
	for _, item := range items {
		views = append(views, conversationDetailMessageView{
			Kind:         string(item.Kind),
			KindLabel:    sessionMessageKindLabel(item.Kind),
			Body:         buildStructuredTextView(item.Body, 6),
			SenderLabel:  sessionSenderLabel(item.SenderSessionID),
			SenderIsMono: strings.TrimSpace(item.SenderSessionID) != "",
			SourceRunID:  item.Provenance.SourceRunID,
		})
	}
	return views
}

func buildConversationDetailRouteView(item *model.SessionRoute) *conversationDetailRouteView {
	if item == nil {
		return nil
	}
	return &conversationDetailRouteView{
		ID:               item.ID,
		ConnectorID:      item.ConnectorID,
		ExternalID:       item.ExternalID,
		ThreadID:         item.ThreadID,
		StatusLabel:      humanizeWebLabel(item.Status),
		CreatedAtLabel:   formatWebTimestamp(item.CreatedAt),
		DeactivatedLabel: formatOptionalWebTimestamp(item.DeactivatedAt),
	}
}

func buildConversationDetailDeliveryViews(items []model.OutboundIntent) []conversationDetailDeliveryView {
	views := make([]conversationDetailDeliveryView, 0, len(items))
	for _, item := range items {
		views = append(views, conversationDetailDeliveryView{
			ID:            item.ID,
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

func buildConversationDetailFailureViews(items []model.DeliveryFailure) []conversationDetailFailureView {
	views := make([]conversationDetailFailureView, 0, len(items))
	for _, item := range items {
		views = append(views, conversationDetailFailureView{
			ID:             item.ID,
			ConnectorID:    item.ConnectorID,
			ChatID:         item.ChatID,
			EventKindLabel: humanizeWebLabel(item.EventKind),
			Error:          item.Error,
			CreatedAtLabel: formatWebTimestamp(item.CreatedAt),
		})
	}
	return views
}
