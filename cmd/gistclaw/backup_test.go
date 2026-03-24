package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/store"
)

func seedDB(t *testing.T, dbPath string) {
	t.Helper()
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_ = db.Close()
}

func TestBackup_CreatesFile(t *testing.T) {
	bin := buildBinary(t)
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "gistclaw.db")
	seedDB(t, dbPath)

	cmd := exec.Command(bin, "backup", "--db", dbPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("backup failed: %v\n%s", err, out)
	}

	// Backup path is printed to stdout.
	bakPath := strings.TrimSpace(string(out))
	if _, err := os.Stat(bakPath); err != nil {
		t.Errorf("backup file not found at %q: %v", bakPath, err)
	}
}

func TestBackup_FilenameHasTimestamp(t *testing.T) {
	bin := buildBinary(t)
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "gistclaw.db")
	seedDB(t, dbPath)

	cmd := exec.Command(bin, "backup", "--db", dbPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("backup failed: %v\n%s", err, out)
	}

	bakPath := strings.TrimSpace(string(out))
	base := filepath.Base(bakPath)
	// Expect a timestamp component like 20260324-153000 in the filename.
	if !strings.Contains(base, "-") || !strings.HasSuffix(base, ".db.bak") {
		t.Errorf("expected backup filename with timestamp and .db.bak suffix, got %q", base)
	}
}

func TestBackup_SucceedsWithConcurrentReader(t *testing.T) {
	bin := buildBinary(t)
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "gistclaw.db")
	seedDB(t, dbPath)

	// Hold a read connection open while backup runs.
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open read connection: %v", err)
	}
	defer db.Close()
	_ = db.RawDB().QueryRow("SELECT 1")

	cmd := exec.Command(bin, "backup", "--db", dbPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("backup with concurrent reader failed: %v\n%s", err, out)
	}
}
