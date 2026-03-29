package web

import (
	"errors"
	"net/http"

	"github.com/canhta/gistclaw/internal/debugrpc"
)

func (s *Server) handleDebugRPCStatus(w http.ResponseWriter, r *http.Request) {
	if s.debugRPC == nil {
		if _, ok := debugrpc.ResolveProbe(r.URL.Query().Get("probe")); !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"message": "Unknown debug probe.",
			})
			return
		}
		writeJSON(w, http.StatusOK, debugrpc.FallbackStatus(
			r.URL.Query().Get("probe"),
			"RPC probe source is not wired into this daemon.",
		))
		return
	}

	status, err := s.debugRPC.DebugRPCStatus(r.Context(), r.URL.Query().Get("probe"))
	if err != nil {
		if errors.Is(err, debugrpc.ErrUnknownProbe) {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"message": "Unknown debug probe.",
			})
			return
		}
		http.Error(w, "failed to load debug rpc status", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, status)
}
