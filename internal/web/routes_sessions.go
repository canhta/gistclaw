package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/sessions"
)

type sessionListResponse struct {
	Sessions []model.Session `json:"sessions"`
}

type sessionMailboxResponse struct {
	Session  model.Session          `json:"session"`
	Messages []model.SessionMessage `json:"messages"`
}

type sessionSendRequest struct {
	Body          string `json:"body"`
	FromSessionID string `json:"from_session_id"`
}

type sessionSendResponse struct {
	Run model.Run `json:"run"`
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

func (s *Server) handleSessionSend(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
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
