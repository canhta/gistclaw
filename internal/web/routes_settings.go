package web

import (
	"net/http"
	"strings"
)

type settingsPageData struct {
	TeamName      string
	WorkspaceRoot string
	AdminToken    string // masked for display only
	Error         string
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	data := settingsPageData{
		TeamName:      lookupSetting(s.db, "team_name"),
		WorkspaceRoot: lookupSetting(s.db, "workspace_root"),
	}

	// Mask admin token: show first 8 chars then asterisks
	raw := lookupSetting(s.db, "admin_token")
	if len(raw) > 8 {
		data.AdminToken = raw[:8] + strings.Repeat("*", len(raw)-8)
	} else if raw != "" {
		data.AdminToken = strings.Repeat("*", len(raw))
	}

	s.renderTemplate(w, "Settings", "settings_body", data)
}

func (s *Server) handleSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	updates := make(map[string]string)
	for _, key := range []string{"team_name", "workspace_root"} {
		if v := strings.TrimSpace(r.FormValue(key)); v != "" {
			updates[key] = v
		}
	}

	if err := s.rt.UpdateSettings(r.Context(), updates); err != nil {
		s.renderTemplate(w, "Settings", "settings_body", settingsPageData{Error: err.Error()})
		return
	}

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}
