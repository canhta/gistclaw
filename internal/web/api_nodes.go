package web

import (
	"net/http"

	"github.com/canhta/gistclaw/internal/nodeinventory"
)

func (s *Server) handleNodesStatus(w http.ResponseWriter, r *http.Request) {
	if s.nodes == nil {
		writeJSON(w, http.StatusOK, nodeinventory.FallbackStatus("Node inventory source is not wired into this daemon."))
		return
	}

	status, err := s.nodes.NodeInventoryStatus(r.Context())
	if err != nil {
		http.Error(w, "failed to load node inventory", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, status)
}
