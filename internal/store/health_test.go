package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHealth_ReportIncludesDatabaseWALAndBackupMetadata(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gistclaw.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := os.WriteFile(dbPath+"-wal", []byte("wal"), 0o644); err != nil {
		t.Fatalf("write wal: %v", err)
	}

	backupPath := filepath.Join(dir, "gistclaw.20260326-010203.db.bak")
	if err := os.WriteFile(backupPath, []byte("backup"), 0o644); err != nil {
		t.Fatalf("write backup: %v", err)
	}
	backupAt := time.Date(2026, time.March, 26, 1, 2, 3, 0, time.UTC)
	if err := os.Chtimes(backupPath, backupAt, backupAt); err != nil {
		t.Fatalf("chtimes backup: %v", err)
	}

	report, err := LoadHealth(dbPath, backupAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("LoadHealth: %v", err)
	}
	if report.DatabaseBytes == 0 {
		t.Fatal("expected non-zero database size")
	}
	if report.WALBytes != 3 {
		t.Fatalf("WALBytes = %d, want 3", report.WALBytes)
	}
	if report.FreeDiskBytes == 0 {
		t.Fatal("expected non-zero free disk bytes")
	}
	if report.LatestBackupPath != backupPath {
		t.Fatalf("LatestBackupPath = %q, want %q", report.LatestBackupPath, backupPath)
	}
	if report.LatestBackupAt == nil || !report.LatestBackupAt.Equal(backupAt) {
		t.Fatalf("LatestBackupAt = %v, want %v", report.LatestBackupAt, backupAt)
	}
	if report.BackupStatus != BackupStatusFresh {
		t.Fatalf("BackupStatus = %q, want %q", report.BackupStatus, BackupStatusFresh)
	}
}

func TestHealth_WarnsWhenBackupIsMissingOrStale(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gistclaw.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	now := time.Date(2026, time.March, 26, 12, 0, 0, 0, time.UTC)
	report, err := LoadHealth(dbPath, now)
	if err != nil {
		t.Fatalf("LoadHealth missing backup: %v", err)
	}
	if !containsHealthWarning(report.Warnings, WarningMissingBackup) {
		t.Fatalf("expected missing-backup warning, got %#v", report.Warnings)
	}
	if report.BackupStatus != BackupStatusMissing {
		t.Fatalf("BackupStatus = %q, want %q", report.BackupStatus, BackupStatusMissing)
	}

	backupPath := filepath.Join(dir, "gistclaw.20260318-010203.db.bak")
	if err := os.WriteFile(backupPath, []byte("backup"), 0o644); err != nil {
		t.Fatalf("write backup: %v", err)
	}
	staleAt := now.Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(backupPath, staleAt, staleAt); err != nil {
		t.Fatalf("chtimes stale backup: %v", err)
	}

	report, err = LoadHealth(dbPath, now)
	if err != nil {
		t.Fatalf("LoadHealth stale backup: %v", err)
	}
	if !containsHealthWarning(report.Warnings, WarningStaleBackup) {
		t.Fatalf("expected stale-backup warning, got %#v", report.Warnings)
	}
	if report.BackupStatus != BackupStatusStale {
		t.Fatalf("BackupStatus = %q, want %q", report.BackupStatus, BackupStatusStale)
	}
}

func TestHealth_WarnsWhenDiskHeadroomIsLow(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gistclaw.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	originalStatfs := statfs
	statfs = func(path string, stat *statfsStat) error {
		stat.Bavail = 1
		stat.Bsize = 1
		return nil
	}
	defer func() {
		statfs = originalStatfs
	}()

	report, err := LoadHealth(dbPath, time.Now().UTC())
	if err != nil {
		t.Fatalf("LoadHealth: %v", err)
	}
	if !containsHealthWarning(report.Warnings, WarningLowDisk) {
		t.Fatalf("expected low-disk warning, got %#v", report.Warnings)
	}
}

func containsHealthWarning(warnings []HealthWarning, want HealthWarning) bool {
	for _, warning := range warnings {
		if warning == want {
			return true
		}
	}
	return false
}
