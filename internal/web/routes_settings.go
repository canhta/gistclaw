package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	authpkg "github.com/canhta/gistclaw/internal/auth"
)

type settingsPageData struct {
	WorkspaceRoot      string
	AdminToken         string // masked for display only
	PerRunTokenBudget  string
	DailyCostCapUSD    string
	RollingCostUSD     float64
	TelegramToken      string // masked: first 8 chars then asterisks
	Error              string
	AccessError        string
	AccessNotice       string
	CurrentDevice      *settingsDeviceRow
	OtherActiveDevices []settingsDeviceRow
	BlockedDevices     []settingsDeviceRow
}

type settingsDeviceRow struct {
	ID               string
	PrimaryLabel     string
	SecondaryLine    string
	Current          bool
	Blocked          bool
	ActiveSessions   int
	DetailsIP        string
	DetailsUserAgent string
	RevokePath       string
	BlockPath        string
	UnblockPath      string
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	data := settingsPageData{
		WorkspaceRoot:     lookupSetting(s.db, "workspace_root"),
		PerRunTokenBudget: lookupSetting(s.db, "per_run_token_budget"),
		DailyCostCapUSD:   lookupSetting(s.db, "daily_cost_cap_usd"),
		AccessError:       strings.TrimSpace(r.URL.Query().Get("access_error")),
		AccessNotice:      strings.TrimSpace(r.URL.Query().Get("access_notice")),
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

	// Mask telegram bot token similarly.
	tgRaw := lookupSetting(s.db, "telegram_bot_token")
	if len(tgRaw) > 8 {
		data.TelegramToken = tgRaw[:8] + strings.Repeat("*", len(tgRaw)-8)
	} else if tgRaw != "" {
		data.TelegramToken = strings.Repeat("*", len(tgRaw))
	}

	if err := s.populateAccessDevices(r, &data); err != nil && data.AccessError == "" {
		data.AccessError = "Unable to load device access right now."
	}

	s.renderTemplate(w, r, "Settings", "settings_body", data)
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
	for _, key := range []string{"workspace_root", "per_run_token_budget", "daily_cost_cap_usd", "telegram_bot_token"} {
		if v := strings.TrimSpace(r.FormValue(key)); v != "" {
			updates[key] = v
		}
	}

	if err := s.rt.UpdateSettings(r.Context(), updates); err != nil {
		s.renderTemplate(w, r, "Settings", "settings_body", settingsPageData{Error: err.Error()})
		return
	}

	http.Redirect(w, r, pageConfigureSettings, http.StatusSeeOther)
}

func (s *Server) populateAccessDevices(r *http.Request, data *settingsPageData) error {
	devices, err := authpkg.ListDevices(r.Context(), s.db, time.Now().UTC())
	if err != nil {
		return err
	}
	currentSession, hasSession := requestSessionFromContext(r.Context())
	for _, device := range devices {
		row := settingsDeviceRow{
			ID:               device.ID,
			PrimaryLabel:     device.DisplayName,
			SecondaryLine:    settingsDeviceSecondaryLine(device),
			Current:          hasSession && device.ID == currentSession.DeviceID,
			Blocked:          device.Blocked,
			ActiveSessions:   device.ActiveSessions,
			DetailsIP:        device.LastIP,
			DetailsUserAgent: device.LastUserAgent,
			RevokePath:       settingsDeviceRevokePath(device.ID),
			BlockPath:        settingsDeviceBlockPath(device.ID),
			UnblockPath:      settingsDeviceUnblockPath(device.ID),
		}
		switch {
		case row.Current:
			data.CurrentDevice = &row
		case row.Blocked:
			data.BlockedDevices = append(data.BlockedDevices, row)
		case device.ActiveSessions > 0:
			data.OtherActiveDevices = append(data.OtherActiveDevices, row)
		}
	}
	return nil
}

func settingsDeviceSecondaryLine(device authpkg.DeviceAccess) string {
	parts := make([]string, 0, 3)
	if !device.LastSeenAt.IsZero() {
		parts = append(parts, "Last seen "+device.LastSeenAt.UTC().Format("2006-01-02 15:04 UTC"))
	}
	if device.LastIP != "" {
		parts = append(parts, "Network "+device.LastIP)
	}
	if device.ActiveSessions > 0 {
		label := "session"
		if device.ActiveSessions != 1 {
			label = "sessions"
		}
		parts = append(parts, strconv.Itoa(device.ActiveSessions)+" "+label+" active")
	}
	return strings.Join(parts, " · ")
}
