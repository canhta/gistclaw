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
}
