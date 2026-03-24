package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/sessions"
)

type sessionListResponse struct {
	Sessions []model.Session `json:"sessions"`
}

type sessionMailboxResponse struct {
	Session  model.Session          `json:"session"`
	Messages []model.SessionMessage `json:"messages"`
}

func (s *Server) handleSessionsIndex(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	list, err := s.rt.ListAllSessions(r.Context(), requestLimit(r, 50))
	if err != nil {
		http.Error(w, "failed to load sessions", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, sessionListResponse{Sessions: list})
}

func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	sessionID := r.PathValue("id")
	session, messages, err := s.rt.SessionHistory(r.Context(), sessionID, requestLimit(r, 100))
	if err != nil {
		if errors.Is(err, sessions.ErrSessionNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to load session", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, sessionMailboxResponse{
		Session:  session,
		Messages: messages,
	})
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
