package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctor_AllChecksPass(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := makeValidConfig(t, dbPath, workspaceRoot)

	var stdout, stderr bytes.Buffer
	runDoctor(cfgPath, &stdout, &stderr)
	output := stdout.String()
	if !strings.Contains(output, "PASS") {
		t.Errorf("expected PASS in output:\n%s", output)
	}
}

func TestDoctor_MissingWorkspaceFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := makeValidConfig(t, dbPath, "/nonexistent/workspace/path-xyz-99")

	var stdout, stderr bytes.Buffer
	code := runDoctor(cfgPath, &stdout, &stderr)
	if code == 0 {
		t.Error("expected non-zero exit for missing workspace")
	}
	if !strings.Contains(stdout.String(), "FAIL") {
		t.Errorf("expected FAIL in output:\n%s", stdout.String())
	}
}

func TestDoctor_MissingProviderFails(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")

	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	content := "database_path: " + dbPath + "\nworkspace_root: " + workspaceRoot + "\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runDoctor(cfgPath, &stdout, &stderr)
	if code == 0 {
		t.Error("expected non-zero exit for missing provider")
	}
	if !strings.Contains(stdout.String(), "FAIL") {
		t.Errorf("expected FAIL in output:\n%s", stdout.String())
	}
}

func TestDoctor_LowDiskIsWarn(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := makeValidConfig(t, dbPath, workspaceRoot)

	var stdout, stderr bytes.Buffer
	runDoctor(cfgPath, &stdout, &stderr)
	output := stdout.String()
	if !strings.Contains(output, "disk") {
		t.Errorf("expected disk check line in output:\n%s", output)
	}
}

func TestDoctor_TelegramMissingIsSkipped(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := makeValidConfig(t, dbPath, workspaceRoot)

	var stdout, stderr bytes.Buffer
	runDoctor(cfgPath, &stdout, &stderr)
	output := stdout.String()
	// When no telegram token is set, the telegram check should not appear.
	if strings.Contains(output, "telegram") {
		t.Errorf("telegram check should be skipped when no token set, got:\n%s", output)
	}
}

func TestDoctor_TelegramConfiguredInYAMLIsChecked(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")

	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	content := strings.Join([]string{
		"database_path: " + dbPath,
		"workspace_root: " + workspaceRoot,
		"provider:",
		"  name: openai",
		"  api_key: sk-test",
		"telegram:",
		"  bot_token: telegram-test-token",
		"",
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	runDoctor(cfgPath, &stdout, &stderr)
	output := stdout.String()
	if !strings.Contains(output, "telegram") {
		t.Errorf("expected telegram check line when token is configured in YAML, got:\n%s", output)
	}
}

func TestDoctor_BadConfigFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runDoctor("/nonexistent/config.yaml", &stdout, &stderr)
	if code == 0 {
		t.Error("expected non-zero exit for missing config file")
	}
	if !strings.Contains(stdout.String(), "FAIL") {
		t.Errorf("expected FAIL in output:\n%s", stdout.String())
	}
}
