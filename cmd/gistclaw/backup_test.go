package main

import (
	"bytes"
	"os"
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
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "gistclaw.db")
	seedDB(t, dbPath)

	var stdout, stderr bytes.Buffer
	code := runBackup([]string{"--db", dbPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("backup exited %d: %s", code, stderr.String())
	}
	bakPath := strings.TrimSpace(stdout.String())
	if _, err := os.Stat(bakPath); err != nil {
		t.Errorf("backup file not found at %q: %v", bakPath, err)
	}
}

func TestBackup_FilenameHasTimestamp(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "gistclaw.db")
	seedDB(t, dbPath)

	var stdout, stderr bytes.Buffer
	code := runBackup([]string{"--db", dbPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("backup exited %d: %s", code, stderr.String())
	}
	bakPath := strings.TrimSpace(stdout.String())
	base := filepath.Base(bakPath)
	if !strings.Contains(base, "-") || !strings.HasSuffix(base, ".db.bak") {
		t.Errorf("expected timestamped .db.bak filename, got %q", base)
	}
}

func TestBackup_SucceedsWithConcurrentReader(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "gistclaw.db")
	seedDB(t, dbPath)

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open read connection: %v", err)
	}
	defer db.Close()
	_ = db.RawDB().QueryRow("SELECT 1")

	var stdout, stderr bytes.Buffer
	code := runBackup([]string{"--db", dbPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("backup with concurrent reader exited %d: %s", code, stderr.String())
	}
}

func TestBackup_MissingDBFlagErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runBackup([]string{}, &stdout, &stderr)
	if code == 0 {
		t.Error("expected non-zero exit for missing --db flag")
	}
}

func TestParseFlag_EqualsForm(t *testing.T) {
	val, err := parseFlag([]string{"--db=/tmp/foo.db"}, "--db")
	if err != nil || val != "/tmp/foo.db" {
		t.Errorf("expected /tmp/foo.db, got %q err=%v", val, err)
	}
}

func TestParseFlag_SpaceForm(t *testing.T) {
	val, err := parseFlag([]string{"--db", "/tmp/foo.db"}, "--db")
	if err != nil || val != "/tmp/foo.db" {
		t.Errorf("expected /tmp/foo.db, got %q err=%v", val, err)
	}
}

func TestParseFlag_Missing(t *testing.T) {
	val, err := parseFlag([]string{"--other", "x"}, "--db")
	if err != nil || val != "" {
		t.Errorf("expected empty string, got %q err=%v", val, err)
	}
}
