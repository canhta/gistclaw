package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	LowDiskWarningThreshold   = 500 * 1024 * 1024
	StaleBackupWarningMaximum = 7 * 24 * time.Hour
)

type BackupStatus string

const (
	BackupStatusMissing BackupStatus = "missing"
	BackupStatusFresh   BackupStatus = "fresh"
	BackupStatusStale   BackupStatus = "stale"
)

type HealthWarning string

const (
	WarningLowDisk       HealthWarning = "low_disk"
	WarningMissingBackup HealthWarning = "backup_missing"
	WarningStaleBackup   HealthWarning = "backup_stale"
)

type HealthReport struct {
	DatabasePath     string
	DatabaseBytes    int64
	WALPath          string
	WALBytes         int64
	FreeDiskBytes    uint64
	LatestBackupPath string
	LatestBackupAt   *time.Time
	BackupStatus     BackupStatus
	Warnings         []HealthWarning
}

type statfsStat = syscall.Statfs_t

var statfs = func(path string, stat *statfsStat) error {
	return syscall.Statfs(path, stat)
}

func LoadHealth(dbPath string, now time.Time) (HealthReport, error) {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" {
		return HealthReport{}, fmt.Errorf("store health: database path is required")
	}
	if dbPath == ":memory:" {
		return HealthReport{}, fmt.Errorf("store health: file-backed database is required")
	}

	info, err := os.Stat(dbPath)
	if err != nil {
		return HealthReport{}, fmt.Errorf("store health: stat database: %w", err)
	}

	report := HealthReport{
		DatabasePath:  dbPath,
		DatabaseBytes: info.Size(),
		WALPath:       dbPath + "-wal",
	}

	if walInfo, err := os.Stat(report.WALPath); err == nil {
		report.WALBytes = walInfo.Size()
	} else if !os.IsNotExist(err) {
		return HealthReport{}, fmt.Errorf("store health: stat wal: %w", err)
	}

	var stat statfsStat
	if err := statfs(filepath.Dir(dbPath), &stat); err != nil {
		return HealthReport{}, fmt.Errorf("store health: statfs: %w", err)
	}
	report.FreeDiskBytes = stat.Bavail * uint64(stat.Bsize)

	latestBackupPath, latestBackupAt, err := newestBackup(dbPath)
	if err != nil {
		return HealthReport{}, err
	}
	report.LatestBackupPath = latestBackupPath
	report.LatestBackupAt = latestBackupAt
	report.BackupStatus = classifyBackup(now.UTC(), latestBackupAt)
	report.Warnings = collectHealthWarnings(report)

	return report, nil
}

func BackupPathForTime(srcPath string, now time.Time) string {
	ts := now.UTC().Format("20060102-150405")
	base := filepath.Base(srcPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	dstName := fmt.Sprintf("%s.%s%s.bak", stem, ts, ext)
	return filepath.Join(filepath.Dir(srcPath), dstName)
}

func newestBackup(dbPath string) (string, *time.Time, error) {
	dir := filepath.Dir(dbPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, fmt.Errorf("store health: read backup dir: %w", err)
	}

	prefix, suffix := backupNameParts(dbPath)
	var latestBackupPath string
	var latestBackupAt *time.Time
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return "", nil, fmt.Errorf("store health: stat backup %q: %w", name, err)
		}
		modTime := info.ModTime().UTC()
		if latestBackupAt == nil || modTime.After(*latestBackupAt) {
			latestBackupPath = filepath.Join(dir, name)
			latestBackupAt = &modTime
		}
	}

	return latestBackupPath, latestBackupAt, nil
}

func backupNameParts(dbPath string) (string, string) {
	base := filepath.Base(dbPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return stem + ".", ext + ".bak"
}

func classifyBackup(now time.Time, latestBackupAt *time.Time) BackupStatus {
	if latestBackupAt == nil {
		return BackupStatusMissing
	}
	if latestBackupAt.After(now) {
		return BackupStatusFresh
	}
	if now.Sub(*latestBackupAt) > StaleBackupWarningMaximum {
		return BackupStatusStale
	}
	return BackupStatusFresh
}

func collectHealthWarnings(report HealthReport) []HealthWarning {
	warnings := make([]HealthWarning, 0, 2)
	if report.FreeDiskBytes < LowDiskWarningThreshold {
		warnings = append(warnings, WarningLowDisk)
	}

	switch report.BackupStatus {
	case BackupStatusMissing:
		warnings = append(warnings, WarningMissingBackup)
	case BackupStatusStale:
		warnings = append(warnings, WarningStaleBackup)
	}

	return warnings
}
