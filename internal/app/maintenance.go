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
	installStatus := maintenance.InstallStatus{
		ConfigPath:       configPath,
		StateDir:         a.cfg.StateDir,
		DatabaseDir:      filepath.Dir(a.cfg.DatabasePath),
		StorageRoot:      a.cfg.StorageRoot,
		BinaryPath:       binaryPath,
		WorkingDirectory: SystemdWorkingDirectory,
		ServiceUnitPath:  SystemdServiceUnitPath,
	}
	serviceStatus := maintenance.ServiceStatus{
		RestartPolicy: "on-failure",
		UnitPreview:   RenderSystemdUnit(binaryPath, configPath),
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
		Install:  installStatus,
		Service:  serviceStatus,
		Commands: buildMaintenanceCommands(installStatus),
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

func buildMaintenanceCommands(install maintenance.InstallStatus) maintenance.CommandStatus {
	commands := maintenance.CommandStatus{
		RunUpdate:     []maintenance.OperatorCommand{},
		RestartReport: []maintenance.OperatorCommand{},
	}

	serviceName := maintenanceServiceName(install.ServiceUnitPath)
	if availableCommandValue(install.BinaryPath) {
		commands.RunUpdate = append(commands.RunUpdate, maintenance.OperatorCommand{
			ID:      "binary-version",
			Label:   "Installed binary",
			Detail:  "Confirm the installed binary reports the expected release metadata before restarting the daemon.",
			Command: shellJoin(install.BinaryPath, "version"),
		})
	}
	if serviceName != "" {
		commands.RunUpdate = append(commands.RunUpdate, maintenance.OperatorCommand{
			ID:      "service-unit",
			Label:   "Inspect service unit",
			Detail:  "Confirm the active systemd unit before replacing the binary or restarting the daemon.",
			Command: shellJoin("systemctl", "cat", serviceName, "--no-pager"),
		})
		commands.RunUpdate = append(commands.RunUpdate, maintenance.OperatorCommand{
			ID:      "restart-daemon",
			Label:   "Restart daemon",
			Detail:  "Restart the shipped service after replacing the binary or config.",
			Command: shellJoin("sudo", "systemctl", "restart", serviceName),
		})
		commands.RestartReport = append(commands.RestartReport, maintenance.OperatorCommand{
			ID:      "service-status",
			Label:   "Service status",
			Detail:  "Verify the service came back cleanly after the restart.",
			Command: shellJoin("systemctl", "status", serviceName, "--no-pager"),
		})
		commands.RestartReport = append(commands.RestartReport, maintenance.OperatorCommand{
			ID:      "recent-journal",
			Label:   "Recent journal",
			Detail:  "Review the most recent daemon boot logs.",
			Command: shellJoin("journalctl", "-u", serviceName, "-n", "100", "--no-pager"),
		})
	}
	if availableCommandValue(install.StorageRoot) {
		commands.RestartReport = append(commands.RestartReport, maintenance.OperatorCommand{
			ID:      "storage-footprint",
			Label:   "Storage footprint",
			Detail:  "Review storage usage after the restart.",
			Command: shellJoin("du", "-sh", install.StorageRoot),
		})
	}

	return commands
}

func maintenanceServiceName(serviceUnitPath string) string {
	if !availableCommandValue(serviceUnitPath) {
		return ""
	}
	base := filepath.Base(serviceUnitPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	return strings.TrimSpace(name)
}

func availableCommandValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" &&
		!strings.EqualFold(trimmed, "unavailable") &&
		!strings.EqualFold(trimmed, "unknown")
}

func shellJoin(parts ...string) string {
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		quoted = append(quoted, shellQuote(part))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n'\"`$\\") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
