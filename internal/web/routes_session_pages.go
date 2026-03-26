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

type sessionPageIndexData struct {
	Sessions []sessionPageIndexItemView
	Filters  sessionPageIndexFilters
	Paging   pageLinks
	Error    string
}

type sessionPageDetailData struct {
	Session          sessionPageDetailSessionView
	Messages         []sessionPageDetailMessageView
	Route            *sessionPageDetailRouteView
	ActiveRunID      string
	Deliveries       []sessionPageDetailDeliveryView
	DeliveryFailures []sessionPageDetailFailureView
	Error            string
}

type sessionPageIndexItemView struct {
	ID             string
	ConversationID string
	AgentID        string
	RoleLabel      string
	StatusLabel    string
	UpdatedAtLabel string
}

type sessionPageDetailSessionView struct {
	ID          string
	AgentID     string
	RoleLabel   string
	StatusLabel string
}

type sessionPageDetailMessageView struct {
	Kind         string
	KindLabel    string
	Body         runStructuredTextView
	SenderLabel  string
	SenderIsMono bool
	SourceRunID  string
}

type sessionPageDetailRouteView struct {
	ID               string
	ConnectorID      string
	ExternalID       string
	ThreadID         string
	StatusLabel      string
	CreatedAtLabel   string
	DeactivatedLabel string
}

type sessionPageDetailDeliveryView struct {
	ID            string
	ConnectorID   string
	ChatID        string
	Message       runStructuredTextView
	Status        string
	StatusLabel   string
	AttemptsLabel string
}

type sessionPageDetailFailureView struct {
	ID             string
	ConnectorID    string
	ChatID         string
	EventKindLabel string
	Error          string
	CreatedAtLabel string
}

type sessionPageIndexFilters struct {
	Query       string
	AgentID     string
	Role        string
	Status      string
	ConnectorID string
	Binding     string
}

func (s *Server) handleSessionPageIndex(w http.ResponseWriter, r *http.Request) {
	data, err := s.loadSessionPageIndexData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, r, "Sessions", "sessions_body", data)
}

func (s *Server) handleSessionPageDetail(w http.ResponseWriter, r *http.Request) {
	data, status, err := s.loadSessionPageDetailData(r)
	if err != nil {
		if status == http.StatusNotFound {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), status)
		return
	}
	s.renderTemplate(w, r, "Session Detail", "session_detail_body", data)
}

func (s *Server) handleSessionPageSend(w http.ResponseWriter, r *http.Request) {
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
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		s.renderSessionPageError(w, r, http.StatusUnprocessableEntity, "Session message body is required.")
		return
	}

	run, err := s.rt.SendSession(r.Context(), runtime.SendSessionCommand{
		FromSessionID: strings.TrimSpace(r.FormValue("from_session_id")),
		ToSessionID:   r.PathValue("id"),
		Body:          body,
	})
	if err != nil {
		switch {
		case errors.Is(err, sessions.ErrSessionNotFound):
			http.NotFound(w, r)
		case errors.Is(err, conversations.ErrConversationBusy):
			s.renderSessionPageError(w, r, http.StatusConflict, "The target session already has an active root run.")
		default:
			http.Error(w, "failed to send session message", http.StatusInternalServerError)
		}
		return
	}

	http.Redirect(w, r, runDetailPath(run.ID), http.StatusSeeOther)
}

func (s *Server) handleSessionPageRetryDelivery(w http.ResponseWriter, r *http.Request) {
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

	_, err = s.rt.RetrySessionDelivery(r.Context(), r.PathValue("id"), r.PathValue("delivery_id"))
	if err != nil {
		switch {
		case errors.Is(err, sessions.ErrSessionNotFound), errors.Is(err, runtime.ErrDeliveryNotFound):
			http.NotFound(w, r)
		case errors.Is(err, runtime.ErrDeliveryNotRetryable):
			s.renderSessionPageError(w, r, http.StatusConflict, "Only terminal deliveries can be retried.")
		default:
			http.Error(w, "failed to retry session delivery", http.StatusInternalServerError)
		}
		return
	}

	http.Redirect(w, r, sessionDetailPath(r.PathValue("id")), http.StatusSeeOther)
}

func (s *Server) renderSessionPageError(w http.ResponseWriter, r *http.Request, status int, message string) {
	data, loadStatus, err := s.loadSessionPageDetailData(r)
	if err != nil {
		http.Error(w, err.Error(), loadStatus)
		return
	}
	data.Error = message
	s.renderTemplateStatus(w, r, status, "Session Detail", "session_detail_body", data)
}

func (s *Server) loadSessionPageIndexData(r *http.Request) (sessionPageIndexData, error) {
	if s.rt == nil {
		return sessionPageIndexData{}, errors.New("runtime not configured")
	}

	filter := sessionListFilterFromRequest(r, 100)
	activeProject, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		return sessionPageIndexData{}, errors.New("failed to load active project")
	}
	filter.ProjectID = activeProject.ID
	filter.WorkspaceRoot = activeProject.WorkspaceRoot
	page, err := s.rt.ListAllSessionsPage(r.Context(), filter)
	if err != nil {
		return sessionPageIndexData{}, errors.New("failed to load sessions")
	}

	return sessionPageIndexData{
		Sessions: buildSessionPageIndexViews(page.Items),
		Filters: sessionPageIndexFilters{
			Query:       filter.Query,
			AgentID:     filter.AgentID,
			Role:        filter.Role,
			Status:      filter.Status,
			ConnectorID: filter.ConnectorID,
			Binding:     filter.Binding,
		},
		Paging: buildPageLinks(
			pageOperateSessions,
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

func (s *Server) loadSessionPageDetailData(r *http.Request) (sessionPageDetailData, int, error) {
	if s.rt == nil {
		return sessionPageDetailData{}, http.StatusInternalServerError, errors.New("runtime not configured")
	}

	sessionID := r.PathValue("id")
	visible, err := s.sessionVisibleInActiveProject(r.Context(), sessionID)
	if err != nil {
		return sessionPageDetailData{}, http.StatusInternalServerError, errors.New("failed to load session")
	}
	if !visible {
		return sessionPageDetailData{}, http.StatusNotFound, sessions.ErrSessionNotFound
	}
	session, messages, err := s.rt.SessionHistory(r.Context(), sessionID, 100)
	if err != nil {
		if errors.Is(err, sessions.ErrSessionNotFound) {
			return sessionPageDetailData{}, http.StatusNotFound, err
		}
		return sessionPageDetailData{}, http.StatusInternalServerError, errors.New("failed to load session")
	}

	var route *model.SessionRoute
	sessionSvc := sessions.NewService(s.db, nil)
	loadedRoute, err := sessionSvc.LoadRouteBySession(r.Context(), sessionID)
	if err == nil {
		route = &loadedRoute
	} else if !errors.Is(err, sessions.ErrSessionRouteNotFound) {
		return sessionPageDetailData{}, http.StatusInternalServerError, errors.New("failed to load session route")
	}

	activeRun, err := sessionSvc.LoadActiveRunRef(r.Context(), sessionID)
	if err != nil {
		return sessionPageDetailData{}, http.StatusInternalServerError, errors.New("failed to load active session run")
	}

	deliveries, failures, err := s.rt.SessionDeliveryState(r.Context(), sessionID, 25)
	if err != nil {
		return sessionPageDetailData{}, http.StatusInternalServerError, errors.New("failed to load session delivery state")
	}

	return sessionPageDetailData{
		Session:          buildSessionPageDetailSessionView(session),
		Messages:         buildSessionPageDetailMessageViews(messages),
		Route:            buildSessionPageDetailRouteView(route),
		ActiveRunID:      activeRun.ID,
		Deliveries:       buildSessionPageDetailDeliveryViews(deliveries),
		DeliveryFailures: buildSessionPageDetailFailureViews(failures),
	}, http.StatusOK, nil
}

func buildSessionPageIndexViews(items []model.Session) []sessionPageIndexItemView {
	views := make([]sessionPageIndexItemView, 0, len(items))
	for _, item := range items {
		views = append(views, sessionPageIndexItemView{
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

func buildSessionPageDetailSessionView(item model.Session) sessionPageDetailSessionView {
	return sessionPageDetailSessionView{
		ID:          item.ID,
		AgentID:     item.AgentID,
		RoleLabel:   sessionRoleSummaryLabel(item.Role),
		StatusLabel: humanizeWebLabel(item.Status),
	}
}

func buildSessionPageDetailMessageViews(items []model.SessionMessage) []sessionPageDetailMessageView {
	views := make([]sessionPageDetailMessageView, 0, len(items))
	for _, item := range items {
		views = append(views, sessionPageDetailMessageView{
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

func buildSessionPageDetailRouteView(item *model.SessionRoute) *sessionPageDetailRouteView {
	if item == nil {
		return nil
	}
	return &sessionPageDetailRouteView{
		ID:               item.ID,
		ConnectorID:      item.ConnectorID,
		ExternalID:       item.ExternalID,
		ThreadID:         item.ThreadID,
		StatusLabel:      humanizeWebLabel(item.Status),
		CreatedAtLabel:   formatWebTimestamp(item.CreatedAt),
		DeactivatedLabel: formatOptionalWebTimestamp(item.DeactivatedAt),
	}
}

func buildSessionPageDetailDeliveryViews(items []model.OutboundIntent) []sessionPageDetailDeliveryView {
	views := make([]sessionPageDetailDeliveryView, 0, len(items))
	for _, item := range items {
		views = append(views, sessionPageDetailDeliveryView{
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

func buildSessionPageDetailFailureViews(items []model.DeliveryFailure) []sessionPageDetailFailureView {
	views := make([]sessionPageDetailFailureView, 0, len(items))
	for _, item := range items {
		views = append(views, sessionPageDetailFailureView{
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
