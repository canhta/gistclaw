package web

import "net/http"

func (s *Server) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if s.maintenance == nil {
		http.Error(w, "update status unavailable", http.StatusServiceUnavailable)
		return
	}

	status, err := s.maintenance.MaintenanceStatus(r.Context())
	if err != nil {
		http.Error(w, "failed to load update status", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, status)
}
