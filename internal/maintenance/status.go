package maintenance

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
		Commands: CommandStatus{
			RunUpdate:     []OperatorCommand{},
			RestartReport: []OperatorCommand{},
		},
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
