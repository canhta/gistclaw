package web

import (
	"net/http"
	"strings"
)

type settingsPageData struct {
	TeamName          string
	WorkspaceRoot     string
	AdminToken        string  // masked for display only
	PerRunTokenBudget string
	DailyCostCapUSD   string
	RollingCostUSD    float64
	Error             string
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	data := settingsPageData{
		TeamName:          lookupSetting(s.db, "team_name"),
		WorkspaceRoot:     lookupSetting(s.db, "workspace_root"),
		PerRunTokenBudget: lookupSetting(s.db, "per_run_token_budget"),
		DailyCostCapUSD:   lookupSetting(s.db, "daily_cost_cap_usd"),
	}

	// Read rolling 24-hour cost from receipts.
	var rolling float64
	_ = s.db.RawDB().QueryRow(
		`SELECT COALESCE(SUM(cost_usd), 0) FROM receipts WHERE created_at >= datetime('now', '-24 hours')`,
	).Scan(&rolling)
	data.RollingCostUSD = rolling

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
	for _, key := range []string{"team_name", "workspace_root", "per_run_token_budget", "daily_cost_cap_usd"} {
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
