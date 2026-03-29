package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/maintenance"
	"github.com/canhta/gistclaw/internal/store"
)

const releaseNotesURL = "https://github.com/canhta/gistclaw/releases"

type BuildInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

func (a *App) SetConfigPath(path string) {
	a.configPath = strings.TrimSpace(path)
}

func (a *App) SetBinaryPath(path string) {
	a.binaryPath = strings.TrimSpace(path)
}

func (a *App) SetBuildInfo(info BuildInfo) {
	a.buildInfo = info
}

func (a *App) MaintenanceStatus(ctx context.Context) (maintenance.Status, error) {
	status, err := a.InspectStatus(ctx)
	if err != nil {
		return maintenance.Status{}, fmt.Errorf("inspect status: %w", err)
	}

	build := normalizedBuildInfo(a.buildInfo)
	startedAt := a.startedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}

	configPath := a.configPath
	if configPath == "" {
		configPath = SystemdConfigPath
	}
	binaryPath := a.binaryPath
	if binaryPath == "" {
		binaryPath = SystemdBinaryPath
	}

	return maintenance.Status{
		Release: maintenance.ReleaseStatus{
			Version:        build.Version,
			Commit:         build.Commit,
			BuildDate:      build.BuildDate,
			BuildDateLabel: formatMaintenanceTimestamp(build.BuildDate),
		},
		Runtime: maintenance.RuntimeStatus{
			StartedAt:        startedAt.Format(time.RFC3339),
			StartedAtLabel:   startedAt.Format("2006-01-02 15:04:05 MST"),
			UptimeLabel:      formatUptime(time.Since(startedAt)),
			ActiveRuns:       status.ActiveRuns,
			InterruptedRuns:  status.InterruptedRuns,
			PendingApprovals: status.PendingApprovals,
		},
		Install: maintenance.InstallStatus{
			ConfigPath:       configPath,
			StateDir:         a.cfg.StateDir,
			DatabaseDir:      filepath.Dir(a.cfg.DatabasePath),
			StorageRoot:      a.cfg.StorageRoot,
			BinaryPath:       binaryPath,
			WorkingDirectory: SystemdWorkingDirectory,
			ServiceUnitPath:  SystemdServiceUnitPath,
		},
		Service: maintenance.ServiceStatus{
			RestartPolicy: "on-failure",
			UnitPreview:   RenderSystemdUnit(binaryPath, configPath),
		},
		Storage: maintenance.StorageStatus{
			DatabaseBytes:       status.Storage.DatabaseBytes,
			WALBytes:            status.Storage.WALBytes,
			FreeDiskBytes:       int64(status.Storage.FreeDiskBytes),
			BackupStatus:        string(status.Storage.BackupStatus),
			LatestBackupAtLabel: formatMaintenanceTime(status.Storage.LatestBackupAt),
			LatestBackupPath:    status.Storage.LatestBackupPath,
			Warnings:            stringifyStorageWarnings(status.Storage.Warnings),
		},
		Guides: maintenance.GuideStatus{
			ReleaseNotesURL: releaseNotesURL,
			UbuntuDocPath:   "docs/install-ubuntu.md",
			MacOSDocPath:    "docs/install-macos.md",
			RecoveryDocPath: "docs/recovery.md",
			ChangelogPath:   "CHANGELOG.md",
		},
	}, nil
}

func normalizedBuildInfo(info BuildInfo) BuildInfo {
	if strings.TrimSpace(info.Version) == "" {
		info.Version = "dev"
	}
	if strings.TrimSpace(info.Commit) == "" {
		info.Commit = "unknown"
	}
	if strings.TrimSpace(info.BuildDate) == "" {
		info.BuildDate = "unknown"
	}
	return info
}

func formatMaintenanceTimestamp(value string) string {
	if strings.TrimSpace(value) == "" || value == "unknown" {
		return "unknown"
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return parsed.UTC().Format("2006-01-02 15:04:05 MST")
}

func formatMaintenanceTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02 15:04:05 MST")
}

func formatUptime(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	if d < 24*time.Hour {
		hours := int(d / time.Hour)
		minutes := int((d % time.Hour) / time.Minute)
		if minutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	days := int(d / (24 * time.Hour))
	hours := int((d % (24 * time.Hour)) / time.Hour)
	if hours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, hours)
}

func stringifyStorageWarnings(warnings []store.HealthWarning) []string {
	if len(warnings) == 0 {
		return nil
	}
	resp := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		resp = append(resp, string(warning))
	}
	return resp
}
