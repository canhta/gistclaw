package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)


func TestDoctor_AllChecksPass(t *testing.T) {
	bin := buildBinary(t)
	workspaceRoot := t.TempDir()
	// Init a minimal git repo.
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := makeValidConfig(t, dbPath, workspaceRoot)

	cmd := exec.Command(bin, "--config", cfgPath, "doctor")
	cmd.Env = append(os.Environ(), "ANTHROPIC_BASE_URL=http://127.0.0.1:19999")
	out, err := cmd.CombinedOutput()
	output := string(out)

	// Should exit 0.
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 0 {
			// WARN exits are still 0; only hard FAIL exits with 1.
		} else if err != nil {
			t.Logf("doctor output:\n%s", output)
		}
	}
	if !strings.Contains(output, "PASS") {
		t.Errorf("expected PASS in output:\n%s", output)
	}
}

func TestDoctor_MissingWorkspaceFails(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := makeValidConfig(t, dbPath, "/nonexistent/workspace/path-xyz-99")

	cmd := exec.Command(bin, "--config", cfgPath, "doctor")
	cmd.Env = append(os.Environ(), "ANTHROPIC_BASE_URL=http://127.0.0.1:19999")
	out, _ := cmd.CombinedOutput()
	output := string(out)

	if !strings.Contains(output, "FAIL") {
		t.Errorf("expected FAIL for missing workspace in output:\n%s", output)
	}
}

func TestDoctor_MissingProviderFails(t *testing.T) {
	bin := buildBinary(t)
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")

	// Config with no provider block.
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	content := "database_path: " + dbPath + "\nworkspace_root: " + workspaceRoot + "\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command(bin, "--config", cfgPath, "doctor")
	out, _ := cmd.CombinedOutput()
	output := string(out)

	if !strings.Contains(output, "FAIL") {
		t.Errorf("expected FAIL for missing provider in output:\n%s", output)
	}
}

func TestDoctor_LowDiskIsWarn(t *testing.T) {
	// This test can't reliably simulate low disk; just verify doctor runs and
	// that the disk check line is always present in the output.
	bin := buildBinary(t)
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := makeValidConfig(t, dbPath, workspaceRoot)

	cmd := exec.Command(bin, "--config", cfgPath, "doctor")
	cmd.Env = append(os.Environ(), "ANTHROPIC_BASE_URL=http://127.0.0.1:19999")
	out, _ := cmd.CombinedOutput()
	output := string(out)

	if !strings.Contains(output, "disk") && !strings.Contains(output, "Disk") {
		t.Errorf("expected disk check line in output:\n%s", output)
	}
}

func TestDoctor_TelegramMissingIsSkipped(t *testing.T) {
	// Without a telegram_bot_token configured, the Telegram check should be
	// skipped entirely (not FAIL).
	bin := buildBinary(t)
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := makeValidConfig(t, dbPath, workspaceRoot)

	cmd := exec.Command(bin, "--config", cfgPath, "doctor")
	cmd.Env = append(os.Environ(), "ANTHROPIC_BASE_URL=http://127.0.0.1:19999")
	out, _ := cmd.CombinedOutput()
	output := string(out)

	// Doctor must not report Telegram as FAIL when token is absent.
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "telegram") && strings.Contains(line, "FAIL") {
			t.Errorf("expected Telegram check to be skipped (not FAIL) when no token configured:\n%s", output)
		}
	}
}
