package web

import "net/http"

func (s *Server) handleSkillsStatus(w http.ResponseWriter, r *http.Request) {
	if s.extensions == nil {
		http.Error(w, "extension status unavailable", http.StatusServiceUnavailable)
		return
	}

	status, err := s.extensions.ExtensionStatus(r.Context())
	if err != nil {
		http.Error(w, "failed to load extension status", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, status)
}
