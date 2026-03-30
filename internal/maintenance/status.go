package maintenance

import (
	"path/filepath"
	"strings"
)

type Status struct {
	Notice   string        `json:"notice,omitempty"`
	Release  ReleaseStatus `json:"release"`
	Runtime  RuntimeStatus `json:"runtime"`
	Install  InstallStatus `json:"install"`
	Service  ServiceStatus `json:"service"`
	Commands CommandStatus `json:"commands"`
	Storage  StorageStatus `json:"storage"`
	Guides   GuideStatus   `json:"guides"`
}

func FallbackStatus(notice string) Status {
	return Status{
		Notice: notice,
		Release: ReleaseStatus{
			Version:        "unknown",
			Commit:         "unknown",
			BuildDate:      "unknown",
			BuildDateLabel: "unknown",
		},
		Runtime: RuntimeStatus{
			StartedAtLabel: "Unavailable",
			UptimeLabel:    "Unavailable",
		},
		Install: InstallStatus{
			ConfigPath:       "Unavailable",
			StateDir:         "Unavailable",
			DatabaseDir:      "Unavailable",
			StorageRoot:      "Unavailable",
			BinaryPath:       "Unavailable",
			WorkingDirectory: "Unavailable",
			ServiceUnitPath:  "Unavailable",
		},
		Service: ServiceStatus{
			RestartPolicy: "unknown",
			UnitPreview:   "Unavailable",
		},
		Commands: fallbackCommands(),
		Storage: StorageStatus{
			BackupStatus: "unknown",
			Warnings:     []string{},
		},
		Guides: GuideStatus{
			ReleaseNotesURL: "https://github.com/canhta/gistclaw/releases",
			UbuntuDocPath:   "docs/install-ubuntu.md",
			MacOSDocPath:    "docs/install-macos.md",
			RecoveryDocPath: "docs/recovery.md",
			ChangelogPath:   "CHANGELOG.md",
		},
	}
}

type ReleaseStatus struct {
	Version        string `json:"version"`
	Commit         string `json:"commit"`
	BuildDate      string `json:"build_date"`
	BuildDateLabel string `json:"build_date_label"`
}

type RuntimeStatus struct {
	StartedAt        string `json:"started_at"`
	StartedAtLabel   string `json:"started_at_label"`
	UptimeLabel      string `json:"uptime_label"`
	ActiveRuns       int    `json:"active_runs"`
	InterruptedRuns  int    `json:"interrupted_runs"`
	PendingApprovals int    `json:"pending_approvals"`
}

type InstallStatus struct {
	ConfigPath       string `json:"config_path"`
	StateDir         string `json:"state_dir"`
	DatabaseDir      string `json:"database_dir"`
	StorageRoot      string `json:"storage_root"`
	BinaryPath       string `json:"binary_path"`
	WorkingDirectory string `json:"working_directory"`
	ServiceUnitPath  string `json:"service_unit_path"`
}

type ServiceStatus struct {
	RestartPolicy string `json:"restart_policy"`
	UnitPreview   string `json:"unit_preview"`
}

type OperatorCommand struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Detail  string `json:"detail"`
	Command string `json:"command"`
}

type CommandStatus struct {
	RunUpdate     []OperatorCommand `json:"run_update"`
	RestartReport []OperatorCommand `json:"restart_report"`
}

type StorageStatus struct {
	DatabaseBytes       int64    `json:"database_bytes"`
	WALBytes            int64    `json:"wal_bytes"`
	FreeDiskBytes       int64    `json:"free_disk_bytes"`
	BackupStatus        string   `json:"backup_status"`
	LatestBackupAtLabel string   `json:"latest_backup_at_label"`
	LatestBackupPath    string   `json:"latest_backup_path"`
	Warnings            []string `json:"warnings"`
}

type GuideStatus struct {
	ReleaseNotesURL string `json:"release_notes_url"`
	UbuntuDocPath   string `json:"ubuntu_doc_path"`
	MacOSDocPath    string `json:"macos_doc_path"`
	RecoveryDocPath string `json:"recovery_doc_path"`
	ChangelogPath   string `json:"changelog_path"`
}

func BuildCommands(install InstallStatus) CommandStatus {
	commands := CommandStatus{
		RunUpdate:     []OperatorCommand{},
		RestartReport: []OperatorCommand{},
	}

	serviceName := serviceName(install.ServiceUnitPath)
	if availableCommandValue(install.BinaryPath) {
		commands.RunUpdate = append(commands.RunUpdate, OperatorCommand{
			ID:      "binary-version",
			Label:   "Installed binary",
			Detail:  "Confirm the installed binary reports the expected release metadata before restarting the daemon.",
			Command: shellJoin(install.BinaryPath, "version"),
		})
	}
	if serviceName != "" {
		commands.RunUpdate = append(commands.RunUpdate, OperatorCommand{
			ID:      "service-unit",
			Label:   "Inspect service unit",
			Detail:  "Confirm the active systemd unit before replacing the binary or restarting the daemon.",
			Command: shellJoin("systemctl", "cat", serviceName, "--no-pager"),
		})
		commands.RunUpdate = append(commands.RunUpdate, OperatorCommand{
			ID:      "restart-daemon",
			Label:   "Restart daemon",
			Detail:  "Restart the shipped service after replacing the binary or config.",
			Command: shellJoin("sudo", "systemctl", "restart", serviceName),
		})
		commands.RestartReport = append(commands.RestartReport, OperatorCommand{
			ID:      "service-status",
			Label:   "Service status",
			Detail:  "Verify the service came back cleanly after the restart.",
			Command: shellJoin("systemctl", "status", serviceName, "--no-pager"),
		})
		commands.RestartReport = append(commands.RestartReport, OperatorCommand{
			ID:      "recent-journal",
			Label:   "Recent journal",
			Detail:  "Review the most recent daemon boot logs.",
			Command: shellJoin("journalctl", "-u", serviceName, "-n", "100", "--no-pager"),
		})
	}
	if availableCommandValue(install.StorageRoot) {
		commands.RestartReport = append(commands.RestartReport, OperatorCommand{
			ID:      "storage-footprint",
			Label:   "Storage footprint",
			Detail:  "Review storage usage after the restart.",
			Command: shellJoin("du", "-sh", install.StorageRoot),
		})
	}

	return commands
}

func fallbackCommands() CommandStatus {
	return CommandStatus{
		RunUpdate: []OperatorCommand{
			{
				ID:      "path-version",
				Label:   "Binary on PATH",
				Detail:  "Confirm a gistclaw binary is available before applying an update.",
				Command: "gistclaw version",
			},
			{
				ID:      "ubuntu-service-unit",
				Label:   "Ubuntu service unit",
				Detail:  "Inspect the shipped systemd unit when this machine uses the Ubuntu install path.",
				Command: "systemctl cat gistclaw --no-pager",
			},
			{
				ID:      "homebrew-service-info",
				Label:   "Homebrew service info",
				Detail:  "Inspect the managed Homebrew service when this machine uses the macOS install path.",
				Command: "brew services info gistclaw",
			},
			{
				ID:      "ubuntu-restart",
				Label:   "Ubuntu restart",
				Detail:  "Restart the shipped systemd service after replacing the binary or config.",
				Command: "sudo systemctl restart gistclaw",
			},
			{
				ID:      "homebrew-restart",
				Label:   "Homebrew restart",
				Detail:  "Restart the managed Homebrew service after replacing the binary or config.",
				Command: "brew services restart gistclaw",
			},
		},
		RestartReport: []OperatorCommand{
			{
				ID:      "ubuntu-status",
				Label:   "Ubuntu service status",
				Detail:  "Verify the systemd service came back cleanly after the restart.",
				Command: "systemctl status gistclaw --no-pager",
			},
			{
				ID:      "homebrew-status",
				Label:   "Homebrew service info",
				Detail:  "Verify the Homebrew service state after the restart.",
				Command: "brew services info gistclaw",
			},
			{
				ID:      "ubuntu-journal",
				Label:   "Ubuntu journal",
				Detail:  "Review the most recent systemd boot logs from the Ubuntu install path.",
				Command: "journalctl -u gistclaw -n 100 --no-pager",
			},
			{
				ID:      "ubuntu-inspect",
				Label:   "Ubuntu runtime inspect",
				Detail:  "Inspect the shipped Ubuntu config path after the restart.",
				Command: "gistclaw inspect status --config /etc/gistclaw/config.yaml",
			},
			{
				ID:      "homebrew-inspect",
				Label:   "Homebrew runtime inspect",
				Detail:  "Inspect the shipped Homebrew config path after the restart.",
				Command: "gistclaw inspect status --config /opt/homebrew/etc/gistclaw/config.yaml",
			},
		},
	}
}

func serviceName(serviceUnitPath string) string {
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
