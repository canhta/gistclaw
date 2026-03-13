package claudecode_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/claudecode"
	"github.com/canhta/gistclaw/internal/hitl"
)

// --- fakes shared within this test file ---
// (fakeChannelForHook, fakeApproverForHook, neverApprover, pickFreePort, itoa are defined in hooksrv_test.go)

type claudecodeFakeCostGuard struct{ tracked float64 }

func (g *claudecodeFakeCostGuard) Track(usd float64) { g.tracked += usd }

type claudecodeFakeSOULLoader struct{}

func (s *claudecodeFakeSOULLoader) Load() (string, error) { return "you are a helpful agent", nil }

// ---

// newEchoScript writes a shell script to a temp dir that mimics claude -p
// by printing stream-json lines to stdout and exiting cleanly.
func newEchoScript(t *testing.T, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-claude")
	content := "#!/bin/sh\n"
	for _, l := range lines {
		// Use printf to avoid shell interpretation issues with JSON.
		content += "printf '%s\\n' " + "'" + strings.ReplaceAll(l, "'", "'\\''") + "'\n"
	}
	if err := os.WriteFile(script, []byte(content), 0755); err != nil {
		t.Fatalf("write fake-claude: %v", err)
	}
	return script
}

// checkFakeClaudeAvailable skips the test if the system cannot run shell scripts.
func checkFakeClaudeAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
}

func TestClaudeCodeService_SubmitTask_StreamsText(t *testing.T) {
	checkFakeClaudeAvailable(t)

	lines := []string{
		`{"type":"text","text":"Hello "}`,
		`{"type":"text","text":"world!"}`,
		`{"type":"result","total_cost_usd":0.005,"is_error":false,"result":"done"}`,
	}
	fakeClaude := newEchoScript(t, lines)

	ch := &fakeChannelForHook{}
	guard := &claudecodeFakeCostGuard{}
	soul := &claudecodeFakeSOULLoader{}
	approver := &fakeApproverForHook{decision: hitl.HITLDecision{Allow: true}, delay: 0}

	cfg := claudecode.Config{
		Dir:            t.TempDir(),
		HookServerAddr: "127.0.0.1:" + itoa(pickFreePort(t)),
	}

	svc := claudecode.New(cfg, fakeClaude, ch, approver, guard, soul)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := svc.SubmitTask(ctx, 123, "build the auth module"); err != nil {
		t.Fatalf("SubmitTask: unexpected error: %v", err)
	}

	full := strings.Join(ch.messages, "")
	if !strings.Contains(full, "Hello world!") {
		t.Errorf("expected streamed text, got: %v", ch.messages)
	}
	if guard.tracked != 0.005 {
		t.Errorf("CostGuard.Track: got %v, want 0.005", guard.tracked)
	}
	found := false
	for _, m := range ch.messages {
		if strings.Contains(m, "✅ Done") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ✅ Done message, got: %v", ch.messages)
	}
}

func TestClaudeCodeService_SubmitTask_ZeroOutput(t *testing.T) {
	checkFakeClaudeAvailable(t)

	lines := []string{
		`{"type":"result","total_cost_usd":0,"is_error":false,"result":""}`,
	}
	fakeClaude := newEchoScript(t, lines)

	ch := &fakeChannelForHook{}
	svc := claudecode.New(
		claudecode.Config{Dir: t.TempDir(), HookServerAddr: "127.0.0.1:" + itoa(pickFreePort(t))},
		fakeClaude, ch, &fakeApproverForHook{}, &claudecodeFakeCostGuard{}, &claudecodeFakeSOULLoader{},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = svc.SubmitTask(ctx, 123, "test")

	found := false
	for _, m := range ch.messages {
		if strings.Contains(m, "no output") || strings.Contains(m, "⚠️") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected zero-output warning, got: %v", ch.messages)
	}
}

func TestClaudeCodeService_SubmitTask_ErrorResult(t *testing.T) {
	checkFakeClaudeAvailable(t)

	lines := []string{
		`{"type":"result","total_cost_usd":0,"is_error":true,"result":"claude exploded"}`,
	}
	fakeClaude := newEchoScript(t, lines)

	ch := &fakeChannelForHook{}
	svc := claudecode.New(
		claudecode.Config{Dir: t.TempDir(), HookServerAddr: "127.0.0.1:" + itoa(pickFreePort(t))},
		fakeClaude, ch, &fakeApproverForHook{}, &claudecodeFakeCostGuard{}, &claudecodeFakeSOULLoader{},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = svc.SubmitTask(ctx, 123, "test")

	found := false
	for _, m := range ch.messages {
		if strings.Contains(m, "claude exploded") || strings.Contains(m, "⚠️") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error message, got: %v", ch.messages)
	}
}

func TestClaudeCodeService_Busy_RejectsSecondTask(t *testing.T) {
	// Uses a script that blocks for 500ms.
	checkFakeClaudeAvailable(t)

	dir := t.TempDir()
	script := filepath.Join(dir, "slow-claude")
	slowScript := "#!/bin/sh\nsleep 0.5\nprintf '{\"type\":\"result\",\"total_cost_usd\":0,\"is_error\":false,\"result\":\"ok\"}\\n'\n"
	if err := os.WriteFile(script, []byte(slowScript), 0755); err != nil {
		t.Fatalf("write slow-claude: %v", err)
	}

	ch := &fakeChannelForHook{}
	svc := claudecode.New(
		claudecode.Config{Dir: t.TempDir(), HookServerAddr: "127.0.0.1:" + itoa(pickFreePort(t))},
		script, ch, &fakeApproverForHook{}, &claudecodeFakeCostGuard{}, &claudecodeFakeSOULLoader{},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start first task in background.
	go svc.SubmitTask(ctx, 123, "first")
	time.Sleep(100 * time.Millisecond) // give first task time to start

	// Second task while first is running.
	_ = svc.SubmitTask(ctx, 123, "second")

	found := false
	for _, m := range ch.messages {
		if strings.Contains(m, "busy") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected busy message for second task, got: %v", ch.messages)
	}
}

func TestClaudeCodeService_Name(t *testing.T) {
	svc := claudecode.New(
		claudecode.Config{Dir: t.TempDir(), HookServerAddr: "127.0.0.1:8765"},
		"claude", &fakeChannelForHook{}, &fakeApproverForHook{}, &claudecodeFakeCostGuard{}, &claudecodeFakeSOULLoader{},
	)
	if svc.Name() != "claudecode" {
		t.Errorf("Name: got %q, want claudecode", svc.Name())
	}
}

func TestClaudeCodeService_IsAlive_IdleIsAlive(t *testing.T) {
	svc := claudecode.New(
		claudecode.Config{Dir: t.TempDir(), HookServerAddr: "127.0.0.1:8765"},
		"claude", &fakeChannelForHook{}, &fakeApproverForHook{}, &claudecodeFakeCostGuard{}, &claudecodeFakeSOULLoader{},
	)
	// Idle state with no subprocess = alive (service is ready to accept work).
	ctx := context.Background()
	if !svc.IsAlive(ctx) {
		t.Error("IsAlive: expected true in Idle state")
	}
}
