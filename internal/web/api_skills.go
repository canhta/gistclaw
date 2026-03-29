package web

import (
	"net/http"

	"github.com/canhta/gistclaw/internal/extensionstatus"
)

func (s *Server) handleSkillsStatus(w http.ResponseWriter, r *http.Request) {
	if s.extensions == nil {
		writeJSON(w, http.StatusOK, extensionstatus.FallbackStatus("Extension status source is not wired into this daemon."))
		return
	}

	status, err := s.extensions.ExtensionStatus(r.Context())
	if err != nil {
		http.Error(w, "failed to load extension status", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, status)
}
