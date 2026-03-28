package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	authpkg "github.com/canhta/gistclaw/internal/auth"
)

type settingsDeviceRow struct {
	ID               string
	PrimaryLabel     string
	SecondaryLine    string
	Current          bool
	Blocked          bool
	ActiveSessions   int
	DetailsIP        string
	DetailsUserAgent string
}

type settingsDeviceGroups struct {
	CurrentDevice      *settingsDeviceRow
	OtherActiveDevices []settingsDeviceRow
	BlockedDevices     []settingsDeviceRow
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
