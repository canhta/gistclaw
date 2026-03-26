package main

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestRun_HelpAndUnknownCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := run([]string{"-h"}, &stdout, &stderr); code != 0 {
		t.Fatalf("expected help exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Usage: gistclaw") {
		t.Fatalf("help output missing usage:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"unknown"}, &stdout, &stderr); code != 1 {
		t.Fatalf("expected unknown command exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("stderr missing unknown command message:\n%s", stderr.String())
	}
}

func TestRun_RunAndInspectCommands(t *testing.T) {
	startMockAnthropicServer(t)
	cfgPath, _ := writeCLIConfig(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := run([]string{"run", "--config", cfgPath, "unit test task"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run command failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	runID := fieldValue(t, stdout.String(), "run_id")

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"inspect", "--config", cfgPath, "status"}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect status failed with code %d:\n%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "active_runs: 0") {
		t.Fatalf("inspect status missing counts:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"inspect", "--config", cfgPath, "runs"}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect runs failed with code %d:\n%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), runID) {
		t.Fatalf("inspect runs missing run id %q:\n%s", runID, stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"inspect", "--config", cfgPath, "replay", runID}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect replay failed with code %d:\n%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "run_completed") {
		t.Fatalf("inspect replay missing lifecycle events:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"inspect", "--config", cfgPath, "token"}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect token failed with code %d:\n%s", code, stderr.String())
	}
	token := strings.TrimSpace(stdout.String())
	if len(token) != 64 {
		t.Fatalf("expected generated 64-char token, got %q", token)
	}
	for _, r := range token {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			t.Fatalf("expected generated hex token, got %q", token)
		}
	}
}

func TestRun_InspectUnknownSubcommandStillFails(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := run([]string{"inspect", "systemd"}, &stdout, &stderr); code == 0 {
		t.Fatal("expected non-zero exit for unknown inspect subcommand")
	}
	if !strings.Contains(stderr.String(), "unknown inspect subcommand: systemd") {
		t.Fatalf("unexpected stderr:\n%s", stderr.String())
	}
}

func TestParseConfigPath_UsesEnvOverride(t *testing.T) {
	cfgPath, _ := writeCLIConfig(t)
	t.Setenv("GISTCLAW_CONFIG", cfgPath)

	got, args, err := parseConfigPath([]string{"inspect", "status"})
	if err != nil {
		t.Fatalf("parseConfigPath failed: %v", err)
	}
	if got != cfgPath {
		t.Fatalf("expected config path %q, got %q", cfgPath, got)
	}
	if len(args) != 2 || args[0] != "inspect" || args[1] != "status" {
		t.Fatalf("unexpected remaining args: %#v", args)
	}
}

func TestLoadApp_InvalidConfig(t *testing.T) {
	path := t.TempDir() + "/missing.yaml"
	if _, err := loadApp(path); err == nil {
		t.Fatal("expected loadApp to fail for missing config")
	}
}

func TestLoadPreparedApp_PreparesInterruptedRuns(t *testing.T) {
	cfgPath, dbPath := writeCLIConfig(t)

	application, err := loadPreparedApp(cfgPath)
	if err != nil {
		t.Fatalf("initial loadPreparedApp failed: %v", err)
	}
	if err := application.Stop(); err != nil {
		t.Fatalf("initial Stop failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
		 VALUES ('cli-stale-run', 'conv-cli-stale', 'agent-a', 'active', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert stale run: %v", err)
	}

	application, err = loadPreparedApp(cfgPath)
	if err != nil {
		t.Fatalf("second loadPreparedApp failed: %v", err)
	}
	defer func() { _ = application.Stop() }()

	var status string
	err = db.QueryRow("SELECT status FROM runs WHERE id = 'cli-stale-run'").Scan(&status)
	if err != nil {
		t.Fatalf("query stale run: %v", err)
	}
	if status != "interrupted" {
		t.Fatalf("expected interrupted status, got %q", status)
	}
}

func TestRun_InspectDoesNotInterruptActiveRuns(t *testing.T) {
	cfgPath, dbPath := writeCLIConfig(t)

	application, err := loadApp(cfgPath)
	if err != nil {
		t.Fatalf("loadApp failed: %v", err)
	}
	if err := application.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
		 VALUES ('cli-active-run', 'conv-cli-active', 'agent-a', 'active', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert active run: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"inspect", "--config", cfgPath, "status"}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect status failed with code %d:\n%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "active_runs: 1") {
		t.Fatalf("inspect status should report the active run:\n%s", stdout.String())
	}

	var status string
	err = db.QueryRow("SELECT status FROM runs WHERE id = 'cli-active-run'").Scan(&status)
	if err != nil {
		t.Fatalf("query active run: %v", err)
	}
	if status != "active" {
		t.Fatalf("inspect should not mutate active run status, got %q", status)
	}
}

func TestRun_InspectStatusIncludesStorageHealth(t *testing.T) {
	cfgPath, dbPath := writeCLIConfig(t)
	seedDB(t, dbPath)

	backupPath := filepath.Join(filepath.Dir(dbPath), "runtime.20260326-010203.db.bak")
	if err := os.WriteFile(backupPath, []byte("backup"), 0o644); err != nil {
		t.Fatalf("write backup: %v", err)
	}
	backupAt := time.Date(2026, time.March, 26, 1, 2, 3, 0, time.UTC)
	if err := os.Chtimes(backupPath, backupAt, backupAt); err != nil {
		t.Fatalf("chtimes backup: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"inspect", "--config", cfgPath, "status"}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect status failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	for _, want := range []string{"database_bytes:", "wal_bytes:", "free_disk_bytes:", "backup_status: fresh"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("inspect status missing %q:\n%s", want, stdout.String())
		}
	}
}
