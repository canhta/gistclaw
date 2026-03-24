package main

import (
	"bytes"
	"database/sql"
	"strings"
	"testing"

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
	cfgPath, dbPath := writeCLIConfig(t)

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

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(
		`INSERT INTO settings (key, value, updated_at)
		 VALUES ('admin_token', 'unit-token', datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
	)
	if err != nil {
		t.Fatalf("insert admin token: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"inspect", "--config", cfgPath, "token"}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect token failed with code %d:\n%s", code, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "unit-token" {
		t.Fatalf("unexpected token output: %q", stdout.String())
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

func TestLoadApp_PreparesInterruptedRuns(t *testing.T) {
	cfgPath, dbPath := writeCLIConfig(t)

	application, err := loadApp(cfgPath)
	if err != nil {
		t.Fatalf("initial loadApp failed: %v", err)
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

	application, err = loadApp(cfgPath)
	if err != nil {
		t.Fatalf("second loadApp failed: %v", err)
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
