package web

import "net/http"

func (s *Server) handleNodesStatus(w http.ResponseWriter, r *http.Request) {
	if s.nodes == nil {
		http.Error(w, "node inventory unavailable", http.StatusServiceUnavailable)
		return
	}

	status, err := s.nodes.NodeInventoryStatus(r.Context())
	if err != nil {
		http.Error(w, "failed to load node inventory", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, status)
}
