package web

import (
	"fmt"
	"net/http"
)

func serveSPAAssets() http.Handler {
	return http.StripPrefix("/", http.FileServer(http.FS(spaAssetsFS())))
}

func (s *Server) handleSPADocument(w http.ResponseWriter, r *http.Request) {
	body, err := readSPAAsset("index.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read spa index: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}
