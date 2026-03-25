package web

import (
	"net/http"
	"strings"

	"github.com/canhta/gistclaw/internal/runtime"
)

func (s *Server) handleProjectActivate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	projectID := strings.TrimSpace(r.FormValue("project_id"))
	if projectID == "" {
		http.Error(w, "project_id is required", http.StatusBadRequest)
		return
	}
	if err := runtime.SetActiveProject(r.Context(), s.db, projectID); err != nil {
		http.Error(w, "failed to switch project", http.StatusInternalServerError)
		return
	}

	redirectTo := strings.TrimSpace(r.FormValue("redirect_to"))
	if !strings.HasPrefix(redirectTo, "/") {
		redirectTo = pageOperateRuns
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}
