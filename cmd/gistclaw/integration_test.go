package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	goRuntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestMain_RunAndInspectFlow(t *testing.T) {
	startMockAnthropicServer(t)
	bin := buildBinary(t)
	cfgPath, _ := writeCLIConfig(t)

	runOutput := runCLI(t, bin, "run", "--config", cfgPath, "review the repo")
	if !strings.Contains(runOutput, "status: completed") {
		t.Fatalf("run output missing completed status:\n%s", runOutput)
	}

	runID := fieldValue(t, runOutput, "run_id")

	runsOutput := runCLI(t, bin, "inspect", "--config", cfgPath, "runs")
	if !strings.Contains(runsOutput, runID) || !strings.Contains(runsOutput, "completed") {
		t.Fatalf("inspect runs output missing run %q:\n%s", runID, runsOutput)
	}

	statusOutput := runCLI(t, bin, "inspect", "--config", cfgPath, "status")
	for _, want := range []string{
		"active_runs: 0",
		"interrupted_runs: 0",
		"pending_approvals: 0",
	} {
		if !strings.Contains(statusOutput, want) {
			t.Fatalf("inspect status output missing %q:\n%s", want, statusOutput)
		}
	}

	replayOutput := runCLI(t, bin, "inspect", "--config", cfgPath, "replay", runID)
	for _, want := range []string{"run_started", "turn_completed", "run_completed"} {
		if !strings.Contains(replayOutput, want) {
			t.Fatalf("inspect replay output missing %q:\n%s", want, replayOutput)
		}
	}
}

func TestMain_InspectTokenReadsSettings(t *testing.T) {
	startMockAnthropicServer(t)
	bin := buildBinary(t)
	cfgPath, dbPath := writeCLIConfig(t)

	_ = runCLI(t, bin, "run", "--config", cfgPath, "seed the database")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(
		`INSERT INTO settings (key, value, updated_at)
		 VALUES ('admin_token', 'token-123', datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
	)
	if err != nil {
		t.Fatalf("insert admin token: %v", err)
	}

	tokenOutput := runCLI(t, bin, "inspect", "--config", cfgPath, "token")
	if !strings.Contains(tokenOutput, "token-123") {
		t.Fatalf("inspect token output missing token:\n%s", tokenOutput)
	}
}

func TestMain_ServeStartsAndStopsOnInterrupt(t *testing.T) {
	if goRuntime.GOOS == "windows" {
		t.Skip("interrupt signaling is platform-specific")
	}

	bin := buildBinary(t)
	listenAddr := reserveListenAddr(t)
	cfgPath, _ := writeCLIConfigWithListenAddr(t, listenAddr)

	cmd := exec.Command(bin, "serve", "--config", cfgPath)
	var output lockedBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Start(); err != nil {
		t.Fatalf("starting serve command: %v", err)
	}

	waitForServeReady(t, listenAddr, &output)

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("signal interrupt: %v", err)
	}

	if err := cmd.Wait(); err != nil && !strings.Contains(err.Error(), "signal: interrupt") {
		t.Fatalf("serve command exited with error: %v\n%s", err, output.String())
	}
}

// startMockAnthropicServer starts an httptest.Server that returns minimal valid
// Anthropic Messages API responses, sets ANTHROPIC_BASE_URL so the provider
// uses it, and registers cleanup. Must be called before Bootstrap / loadApp.
func startMockAnthropicServer(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id":   "msg_test",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "mock response"},
			},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
}

func buildBinary(t *testing.T) string {
	t.Helper()
	return buildBinaryWithArgs(t)
}

func buildBinaryWithArgs(t *testing.T, extraArgs ...string) string {
	t.Helper()

	dir := t.TempDir()
	bin := filepath.Join(dir, "gistclaw")

	args := []string{"build"}
	args = append(args, extraArgs...)
	args = append(args, "-o", bin, "./cmd/gistclaw")
	build := exec.Command("go", args...)
	build.Dir = findModuleRoot(t)
	build.Env = append(os.Environ(), "GOFLAGS=")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	return bin
}

func writeCLIConfig(t *testing.T) (string, string) {
	t.Helper()
	return writeCLIConfigWithListenAddr(t, "127.0.0.1:0")
}

func writeCLIConfigWithListenAddr(t *testing.T, listenAddr string) (string, string) {
	t.Helper()

	dir := t.TempDir()
	workspaceRoot := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	dbPath := filepath.Join(dir, "state", "runtime.db")
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := fmt.Sprintf(
		"storage_root: %q\ndatabase_path: %q\nweb:\n  listen_addr: %q\nprovider:\n  name: anthropic\n  api_key: sk-test\n",
		workspaceRoot,
		dbPath,
		listenAddr,
	)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return cfgPath, dbPath
}

func runCLI(t *testing.T, bin string, args ...string) string {
	t.Helper()

	cmd := exec.Command(bin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %v failed: %v\n%s", args, err, output)
	}
	return string(output)
}

func fieldValue(t *testing.T, output, key string) string {
	t.Helper()

	prefix := key + ":"
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}

	t.Fatalf("field %q not found in output:\n%s", key, output)
	return ""
}

func waitForServeReady(t *testing.T, listenAddr string, output *lockedBuffer) {
	t.Helper()

	client := &http.Client{
		Timeout: 200 * time.Millisecond,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	deadline := time.Now().Add(20 * time.Second)
	url := "http://" + listenAddr + "/"
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("serve did not become reachable at %s:\n%s", listenAddr, output.String())
}

func reserveListenAddr(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve listen addr: %v", err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
