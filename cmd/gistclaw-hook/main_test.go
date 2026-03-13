// cmd/gistclaw-hook/main_test.go
package main_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// buildHook compiles the gistclaw-hook binary into a temp directory.
func buildHook(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell integration test: skip on Windows")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "gistclaw-hook")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/canhta/gistclaw/cmd/gistclaw-hook")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build gistclaw-hook: %v", err)
	}
	return bin
}

func TestHookBinary_Allow_ExitsZero(t *testing.T) {
	bin := buildHook(t)

	// Mock server that returns allow.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"hookSpecificOutput": map[string]string{
				"permissionDecision": "allow",
			},
		})
	}))
	defer srv.Close()

	// Extract addr from srv.URL (strip "http://").
	addr := strings.TrimPrefix(srv.URL, "http://")

	input := `{"tool_name":"Edit","tool_input":{"file_path":"/tmp/foo.go"}}`
	cmd := exec.Command(bin, "--addr", addr)
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		t.Fatalf("gistclaw-hook exited with error (expected 0): %v\nstderr: %s", err, stderr.String())
	}
	if stdout.String() == "" {
		t.Error("expected allow JSON on stdout, got nothing")
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(stdout.String()), &out); err != nil {
		t.Errorf("stdout is not valid JSON: %v\nstdout: %s", err, stdout.String())
	}
}

func TestHookBinary_Deny_ExitsTwo(t *testing.T) {
	bin := buildHook(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"hookSpecificOutput": map[string]string{
				"permissionDecision": "deny",
			},
			"systemMessage": "Rejected by operator",
		})
	}))
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "http://")
	cmd := exec.Command(bin, "--addr", addr)
	cmd.Stdin = strings.NewReader(`{"tool_name":"Bash"}`)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Error("expected non-zero exit code for deny, got 0")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error type: %T", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("exit code: got %d, want 2", exitErr.ExitCode())
	}
	if stderr.String() == "" {
		t.Error("expected deny JSON on stderr, got nothing")
	}
}

func TestHookBinary_NetworkError_ExitsTwo(t *testing.T) {
	bin := buildHook(t)

	// Use an address where nothing is listening.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--addr", "127.0.0.1:19998")
	cmd.Stdin = strings.NewReader(`{"tool_name":"Bash"}`)

	err := cmd.Run()
	if err == nil {
		t.Error("expected non-zero exit for network error, got 0")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error type: %T — %v", err, err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("exit code: got %d, want 2", exitErr.ExitCode())
	}
}
