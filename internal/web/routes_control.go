package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

type controlPageData struct {
	Health       []model.ConnectorDeliveryHealth
	ActiveRoutes []model.RouteDirectoryItem
	RouteHistory []model.RouteDirectoryItem
	Deliveries   []model.DeliveryQueueItem
	Error        string
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

	health, err := s.rt.ConnectorDeliveryHealth(r.Context())
	if err != nil {
		return controlPageData{}, errors.New("failed to load connector delivery health")
	}
	activeRoutes, err := s.rt.ListRoutes(r.Context(), "", "active", 50)
	if err != nil {
		return controlPageData{}, errors.New("failed to load active routes")
	}
	routeHistory, err := s.rt.ListRoutes(r.Context(), "", "inactive", 25)
	if err != nil {
		return controlPageData{}, errors.New("failed to load route history")
	}
	deliveries, err := s.rt.ListDeliveries(r.Context(), "", "all", 50)
	if err != nil {
		return controlPageData{}, errors.New("failed to load delivery queue")
	}

	return controlPageData{
		Health:       health,
		ActiveRoutes: activeRoutes,
		RouteHistory: routeHistory,
		Deliveries:   deliveries,
	}, nil
}
