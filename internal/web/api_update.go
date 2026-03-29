package web

import (
	"net/http"

	"github.com/canhta/gistclaw/internal/maintenance"
)

func (s *Server) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if s.maintenance == nil {
		writeJSON(w, http.StatusOK, maintenance.FallbackStatus("Maintenance status source is not wired into this daemon."))
		return
	}

	status, err := s.maintenance.MaintenanceStatus(r.Context())
	if err != nil {
		http.Error(w, "failed to load update status", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, status)
}
