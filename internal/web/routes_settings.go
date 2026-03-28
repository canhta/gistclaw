package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	authpkg "github.com/canhta/gistclaw/internal/auth"
	"github.com/canhta/gistclaw/internal/authority"
)

type settingsPageData struct {
	StorageRoot        string
	ApprovalMode       string
	HostAccessMode     string
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

type settingsDeviceGroups struct {
	CurrentDevice      *settingsDeviceRow
	OtherActiveDevices []settingsDeviceRow
	BlockedDevices     []settingsDeviceRow
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	machine, err := s.loadSettingsMachineSnapshot(r.Context())
	if err != nil {
		http.Error(w, "failed to load settings", http.StatusInternalServerError)
		return
	}

	data := settingsPageData{
		StorageRoot:       machine.StorageRoot,
		ApprovalMode:      machine.ApprovalMode,
		HostAccessMode:    machine.HostAccessMode,
		AdminToken:        machine.AdminToken,
		PerRunTokenBudget: machine.PerRunTokenBudget,
		DailyCostCapUSD:   machine.DailyCostCapUSD,
		RollingCostUSD:    machine.RollingCostUSD,
		TelegramToken:     machine.TelegramToken,
		AccessError:       strings.TrimSpace(r.URL.Query().Get("access_error")),
		AccessNotice:      strings.TrimSpace(r.URL.Query().Get("access_notice")),
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
	if value := strings.TrimSpace(r.FormValue("approval_mode")); value != "" {
		switch value {
		case string(authority.ApprovalModePrompt), string(authority.ApprovalModeAutoApprove):
			updates["approval_mode"] = value
		default:
			s.renderTemplate(w, r, "Settings", "settings_body", settingsPageData{
				StorageRoot:    s.storageRoot,
				ApprovalMode:   lookupSetting(s.db, "approval_mode"),
				HostAccessMode: lookupSetting(s.db, "host_access_mode"),
				Error:          "approval mode is invalid",
			})
			return
		}
	}
	if value := strings.TrimSpace(r.FormValue("host_access_mode")); value != "" {
		switch value {
		case string(authority.HostAccessModeStandard), string(authority.HostAccessModeElevated):
			updates["host_access_mode"] = value
		default:
			s.renderTemplate(w, r, "Settings", "settings_body", settingsPageData{
				StorageRoot:    s.storageRoot,
				ApprovalMode:   lookupSetting(s.db, "approval_mode"),
				HostAccessMode: lookupSetting(s.db, "host_access_mode"),
				Error:          "host access mode is invalid",
			})
			return
		}
	}
	for _, key := range []string{"per_run_token_budget", "daily_cost_cap_usd", "telegram_bot_token"} {
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
	groups, err := s.loadSettingsDeviceGroups(r)
	if err != nil {
		return err
	}
	data.CurrentDevice = groups.CurrentDevice
	data.OtherActiveDevices = groups.OtherActiveDevices
	data.BlockedDevices = groups.BlockedDevices
	return nil
}

func (s *Server) loadSettingsDeviceGroups(r *http.Request) (settingsDeviceGroups, error) {
	devices, err := authpkg.ListDevices(r.Context(), s.db, time.Now().UTC())
	if err != nil {
		return settingsDeviceGroups{}, err
	}
	currentSession, hasSession := requestSessionFromContext(r.Context())
	groups := settingsDeviceGroups{}
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
			groups.CurrentDevice = &row
		case row.Blocked:
			groups.BlockedDevices = append(groups.BlockedDevices, row)
		case device.ActiveSessions > 0:
			groups.OtherActiveDevices = append(groups.OtherActiveDevices, row)
		}
	}
	return groups, nil
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
