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
	Sessions []model.Session
	Filters  sessionPageIndexFilters
	Error    string
}

type sessionPageDetailData struct {
	Session          model.Session
	Messages         []model.SessionMessage
	Route            *model.SessionRoute
	Deliveries       []model.OutboundIntent
	DeliveryFailures []model.DeliveryFailure
	Error            string
}

type sessionPageIndexFilters struct {
	Query       string
	AgentID     string
	Role        string
	Status      string
	ConnectorID string
	BoundOnly   bool
}

func (s *Server) handleSessionPageIndex(w http.ResponseWriter, r *http.Request) {
	data, err := s.loadSessionPageIndexData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "Sessions", "sessions_body", data)
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
	s.renderTemplate(w, "Session Detail", "session_detail_body", data)
}

func (s *Server) handleSessionPageSend(w http.ResponseWriter, r *http.Request) {
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

	http.Redirect(w, r, "/runs/"+run.ID, http.StatusSeeOther)
}

func (s *Server) handleSessionPageRetryDelivery(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	_, err := s.rt.RetrySessionDelivery(r.Context(), r.PathValue("id"), r.PathValue("delivery_id"))
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

	http.Redirect(w, r, "/sessions/"+r.PathValue("id"), http.StatusSeeOther)
}

func (s *Server) renderSessionPageError(w http.ResponseWriter, r *http.Request, status int, message string) {
	data, loadStatus, err := s.loadSessionPageDetailData(r)
	if err != nil {
		http.Error(w, err.Error(), loadStatus)
		return
	}
	data.Error = message
	s.renderTemplateStatus(w, status, "Session Detail", "session_detail_body", data)
}

func (s *Server) loadSessionPageIndexData(r *http.Request) (sessionPageIndexData, error) {
	if s.rt == nil {
		return sessionPageIndexData{}, errors.New("runtime not configured")
	}

	filter := sessionListFilterFromRequest(r, 100)
	list, err := s.rt.ListAllSessions(r.Context(), filter)
	if err != nil {
		return sessionPageIndexData{}, errors.New("failed to load sessions")
	}

	return sessionPageIndexData{
		Sessions: list,
		Filters: sessionPageIndexFilters{
			Query:       filter.Query,
			AgentID:     filter.AgentID,
			Role:        filter.Role,
			Status:      filter.Status,
			ConnectorID: filter.ConnectorID,
			BoundOnly:   filter.BoundOnly,
		},
	}, nil
}

func (s *Server) loadSessionPageDetailData(r *http.Request) (sessionPageDetailData, int, error) {
	if s.rt == nil {
		return sessionPageDetailData{}, http.StatusInternalServerError, errors.New("runtime not configured")
	}

	sessionID := r.PathValue("id")
	session, messages, err := s.rt.SessionHistory(r.Context(), sessionID, 100)
	if err != nil {
		if errors.Is(err, sessions.ErrSessionNotFound) {
			return sessionPageDetailData{}, http.StatusNotFound, err
		}
		return sessionPageDetailData{}, http.StatusInternalServerError, errors.New("failed to load session")
	}

	var route *model.SessionRoute
	loadedRoute, err := sessions.NewService(s.db, nil).LoadRouteBySession(r.Context(), sessionID)
	if err == nil {
		route = &loadedRoute
	} else if !errors.Is(err, sessions.ErrSessionRouteNotFound) {
		return sessionPageDetailData{}, http.StatusInternalServerError, errors.New("failed to load session route")
	}

	deliveries, failures, err := s.rt.SessionDeliveryState(r.Context(), sessionID, 25)
	if err != nil {
		return sessionPageDetailData{}, http.StatusInternalServerError, errors.New("failed to load session delivery state")
	}

	return sessionPageDetailData{
		Session:          session,
		Messages:         messages,
		Route:            route,
		Deliveries:       deliveries,
		DeliveryFailures: failures,
	}, http.StatusOK, nil
}
