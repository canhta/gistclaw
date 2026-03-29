package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppMaintenanceStatusReportsBuildAndInstallContext(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := Config{
		StorageRoot:  filepath.Join(root, "storage"),
		StateDir:     filepath.Join(root, "state"),
		DatabasePath: filepath.Join(root, "state", "gistclaw.db"),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "test-key",
		},
	}

	application, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("bootstrap app: %v", err)
	}
	defer func() { _ = application.Stop() }()

	application.SetConfigPath("/etc/gistclaw/custom.yaml")
	application.SetBinaryPath("/opt/gistclaw/bin/gistclaw")
	application.SetBuildInfo(BuildInfo{
		Version:   "v1.2.3",
		Commit:    "abcdef1234567890",
		BuildDate: "2026-03-29T09:15:00Z",
	})

	status, err := application.MaintenanceStatus(context.Background())
	if err != nil {
		t.Fatalf("maintenance status: %v", err)
	}

	if status.Release.Version != "v1.2.3" {
		t.Fatalf("version = %q, want v1.2.3", status.Release.Version)
	}
	if status.Release.Commit != "abcdef1234567890" {
		t.Fatalf("commit = %q, want abcdef1234567890", status.Release.Commit)
	}
	if status.Install.ConfigPath != "/etc/gistclaw/custom.yaml" {
		t.Fatalf("config_path = %q", status.Install.ConfigPath)
	}
	if status.Install.BinaryPath != "/opt/gistclaw/bin/gistclaw" {
		t.Fatalf("binary_path = %q", status.Install.BinaryPath)
	}
	if status.Install.ServiceUnitPath != SystemdServiceUnitPath {
		t.Fatalf("service_unit_path = %q", status.Install.ServiceUnitPath)
	}
	if !strings.Contains(
		status.Service.UnitPreview,
		"/opt/gistclaw/bin/gistclaw --config /etc/gistclaw/custom.yaml serve",
	) {
		t.Fatalf("expected rendered service unit to include custom binary and config path:\n%s", status.Service.UnitPreview)
	}
	if status.Runtime.ActiveRuns != 0 || status.Runtime.PendingApprovals != 0 || status.Runtime.InterruptedRuns != 0 {
		t.Fatalf("unexpected runtime counts: %+v", status.Runtime)
	}
	if status.Runtime.StartedAtLabel == "" || status.Runtime.UptimeLabel == "" {
		t.Fatalf("expected runtime timestamps, got %+v", status.Runtime)
	}
	if status.Storage.BackupStatus == "" {
		t.Fatalf("expected storage backup status in %+v", status.Storage)
	}
	if status.Guides.ReleaseNotesURL != "https://github.com/canhta/gistclaw/releases" {
		t.Fatalf("release notes url = %q", status.Guides.ReleaseNotesURL)
	}
	if len(status.Commands.RunUpdate) != 3 {
		t.Fatalf("expected 3 run update commands, got %+v", status.Commands.RunUpdate)
	}
	if status.Commands.RunUpdate[0].Label != "Installed binary" {
		t.Fatalf("unexpected first run update command: %+v", status.Commands.RunUpdate[0])
	}
	if status.Commands.RunUpdate[0].Command != "/opt/gistclaw/bin/gistclaw version" {
		t.Fatalf("unexpected binary version command: %q", status.Commands.RunUpdate[0].Command)
	}
	if status.Commands.RunUpdate[1].Command != "systemctl cat gistclaw --no-pager" {
		t.Fatalf("unexpected service unit command: %q", status.Commands.RunUpdate[1].Command)
	}
	if status.Commands.RunUpdate[2].Command != "sudo systemctl restart gistclaw" {
		t.Fatalf("unexpected restart command: %q", status.Commands.RunUpdate[2].Command)
	}
	if len(status.Commands.RestartReport) != 3 {
		t.Fatalf("expected 3 restart report commands, got %+v", status.Commands.RestartReport)
	}
	if status.Commands.RestartReport[0].Command != "systemctl status gistclaw --no-pager" {
		t.Fatalf("unexpected status command: %q", status.Commands.RestartReport[0].Command)
	}
	if status.Commands.RestartReport[1].Command != "journalctl -u gistclaw -n 100 --no-pager" {
		t.Fatalf("unexpected journal command: %q", status.Commands.RestartReport[1].Command)
	}
	if status.Commands.RestartReport[2].Command != "du -sh "+cfg.StorageRoot {
		t.Fatalf("expected storage command to use the storage root, got %q", status.Commands.RestartReport[2].Command)
	}
}
