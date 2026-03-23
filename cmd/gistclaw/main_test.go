package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain_UnknownSubcommand(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "gistclaw")

	build := exec.Command("go", "build", "-o", bin, "./cmd/gistclaw")
	build.Dir = findModuleRoot(t)
	build.Env = append(os.Environ(), "GOFLAGS=")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	cmd := exec.Command(bin, "nonsense")
	err = cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit code for unknown subcommand")
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %T", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.ExitCode())
	}
}

func TestMain_HelpFlag(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "gistclaw")

	build := exec.Command("go", "build", "-o", bin, "./cmd/gistclaw")
	build.Dir = findModuleRoot(t)
	build.Env = append(os.Environ(), "GOFLAGS=")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	cmd := exec.Command(bin, "-h")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 2 {
			t.Fatalf("expected exit code 0 or 2 for -h, got %d", exitErr.ExitCode())
		}
	}

	if len(output) == 0 {
		t.Fatal("expected usage output, got empty")
	}

	usage := string(output)
	if !strings.Contains(usage, "serve") || !strings.Contains(usage, "run") || !strings.Contains(usage, "inspect") {
		t.Fatalf("usage output missing expected subcommands:\n%s", usage)
	}
}

func findModuleRoot(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("go", "env", "GOMOD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("cannot find module root: %v", err)
	}

	modPath := strings.TrimSpace(string(out))
	if modPath == "" {
		t.Fatal("go env GOMOD returned empty path")
	}

	return filepath.Dir(modPath)
}
