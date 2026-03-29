package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canhta/gistclaw/internal/maintenance"
)

type stubMaintenanceSource struct {
	status maintenance.Status
	err    error
}

func (s stubMaintenanceSource) MaintenanceStatus(context.Context) (maintenance.Status, error) {
	if s.err != nil {
		return maintenance.Status{}, s.err
	}
	return s.status, nil
}

func TestUpdateStatusReturnsMaintenanceSnapshot(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	h.rawServer.maintenance = stubMaintenanceSource{
		status: maintenance.Status{
			Release: maintenance.ReleaseStatus{
				Version:        "v1.2.3",
				Commit:         "abcdef1234567890",
				BuildDate:      "2026-03-29T09:15:00Z",
				BuildDateLabel: "2026-03-29 09:15:00 UTC",
			},
			Runtime: maintenance.RuntimeStatus{
				StartedAt:        "2026-03-29T09:30:00Z",
				StartedAtLabel:   "2026-03-29 09:30:00 UTC",
				UptimeLabel:      "47m",
				ActiveRuns:       2,
				InterruptedRuns:  1,
				PendingApprovals: 3,
			},
			Install: maintenance.InstallStatus{
				ConfigPath:       "/etc/gistclaw/config.yaml",
				StateDir:         "/var/lib/gistclaw",
				DatabaseDir:      "/var/lib/gistclaw",
				StorageRoot:      "/var/lib/gistclaw/storage",
				BinaryPath:       "/usr/local/bin/gistclaw",
				WorkingDirectory: "/var/lib/gistclaw",
				ServiceUnitPath:  "/etc/systemd/system/gistclaw.service",
			},
			Service: maintenance.ServiceStatus{
				RestartPolicy: "on-failure",
				UnitPreview:   "[Unit]\nDescription=GistClaw service\n",
			},
			Storage: maintenance.StorageStatus{
				DatabaseBytes:       4096,
				WALBytes:            256,
				FreeDiskBytes:       1048576,
				BackupStatus:        "healthy",
				LatestBackupAtLabel: "2026-03-29 09:10:00 UTC",
				LatestBackupPath:    "/var/lib/gistclaw/backups/backup-2026-03-29.db",
				Warnings:            []string{"low_disk_space"},
			},
			Guides: maintenance.GuideStatus{
				ReleaseNotesURL: "https://github.com/canhta/gistclaw/releases",
				UbuntuDocPath:   "docs/install-ubuntu.md",
				MacOSDocPath:    "docs/install-macos.md",
				RecoveryDocPath: "docs/recovery.md",
				ChangelogPath:   "CHANGELOG.md",
			},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/update", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Release struct {
			Version string `json:"version"`
			Commit  string `json:"commit"`
		} `json:"release"`
		Runtime struct {
			UptimeLabel      string `json:"uptime_label"`
			PendingApprovals int    `json:"pending_approvals"`
		} `json:"runtime"`
		Install struct {
			ConfigPath      string `json:"config_path"`
			ServiceUnitPath string `json:"service_unit_path"`
		} `json:"install"`
		Service struct {
			RestartPolicy string `json:"restart_policy"`
			UnitPreview   string `json:"unit_preview"`
		} `json:"service"`
		Storage struct {
			BackupStatus     string   `json:"backup_status"`
			LatestBackupPath string   `json:"latest_backup_path"`
			Warnings         []string `json:"warnings"`
		} `json:"storage"`
		Guides struct {
			ReleaseNotesURL string `json:"release_notes_url"`
			RecoveryDocPath string `json:"recovery_doc_path"`
		} `json:"guides"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode update response: %v", err)
	}

	if resp.Release.Version != "v1.2.3" || resp.Release.Commit != "abcdef1234567890" {
		t.Fatalf("unexpected release: %+v", resp.Release)
	}
	if resp.Runtime.UptimeLabel != "47m" || resp.Runtime.PendingApprovals != 3 {
		t.Fatalf("unexpected runtime: %+v", resp.Runtime)
	}
	if resp.Install.ConfigPath != "/etc/gistclaw/config.yaml" || resp.Install.ServiceUnitPath != "/etc/systemd/system/gistclaw.service" {
		t.Fatalf("unexpected install: %+v", resp.Install)
	}
	if resp.Service.RestartPolicy != "on-failure" || resp.Service.UnitPreview == "" {
		t.Fatalf("unexpected service: %+v", resp.Service)
	}
	if resp.Storage.BackupStatus != "healthy" || resp.Storage.LatestBackupPath == "" || len(resp.Storage.Warnings) != 1 {
		t.Fatalf("unexpected storage: %+v", resp.Storage)
	}
	if resp.Guides.ReleaseNotesURL != "https://github.com/canhta/gistclaw/releases" || resp.Guides.RecoveryDocPath != "docs/recovery.md" {
		t.Fatalf("unexpected guides: %+v", resp.Guides)
	}
}

func TestUpdateStatusReturnsFallbackWhenSourceMissing(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/update", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Notice string `json:"notice"`
		Release struct {
			Version string `json:"version"`
		} `json:"release"`
		Runtime struct {
			UptimeLabel string `json:"uptime_label"`
		} `json:"runtime"`
		Guides struct {
			ReleaseNotesURL string `json:"release_notes_url"`
		} `json:"guides"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode update fallback response: %v", err)
	}

	if resp.Notice != "Maintenance status source is not wired into this daemon." {
		t.Fatalf("unexpected notice: %q", resp.Notice)
	}
	if resp.Release.Version != "unknown" {
		t.Fatalf("unexpected fallback release version: %+v", resp.Release)
	}
	if resp.Runtime.UptimeLabel != "Unavailable" {
		t.Fatalf("unexpected fallback runtime: %+v", resp.Runtime)
	}
	if resp.Guides.ReleaseNotesURL != "https://github.com/canhta/gistclaw/releases" {
		t.Fatalf("unexpected fallback guides: %+v", resp.Guides)
	}
}
