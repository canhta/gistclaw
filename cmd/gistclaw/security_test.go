package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/store"
)

func TestRunSecurityAudit_PrintsFindingsAndFailsOnUnsafeConfig(t *testing.T) {
	workspaceRoot := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	content := strings.Join([]string{
		"database_path: " + dbPath,
		"workspace_root: " + workspaceRoot,
		"web:",
		"  listen_addr: 0.0.0.0:8080",
		"provider:",
		"  name: anthropic",
		"  api_key: sk-test",
		"",
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runSecurity(cfgPath, []string{"audit"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit for unsafe config")
	}
	if !strings.Contains(stdout.String(), "admin_token.missing") {
		t.Fatalf("expected missing admin token finding, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "web.listen_addr.exposed") {
		t.Fatalf("expected exposed web bind finding, got:\n%s", stdout.String())
	}
}

func TestRunSecurityAudit_PassesWhenConfigIsSafe(t *testing.T) {
	workspaceRoot := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := makeValidConfig(t, dbPath, workspaceRoot)

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	if _, err := db.RawDB().Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES ('admin_token', 'token-test', datetime('now'))`,
	); err != nil {
		t.Fatalf("insert admin token: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runSecurity(cfgPath, []string{"audit"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "no findings") {
		t.Fatalf("expected no findings output, got:\n%s", stdout.String())
	}
}
