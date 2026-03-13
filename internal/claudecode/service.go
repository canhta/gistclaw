package claudecode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/hitl"
)

// claudecodeChannel is the narrow channel interface needed by the service.
// The full channel.Channel interface satisfies this; tests use fakeChannelForHook.
type claudecodeChannel interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}

// Narrow dependency interfaces — keeps claudecodeServiceImpl testable without
// importing infra directly. The concrete infra types satisfy these interfaces;
// tests provide lightweight fakes.

type claudecodeApprover interface {
	RequestPermission(ctx context.Context, req hitl.PermissionRequest) error
	RequestQuestion(ctx context.Context, req hitl.QuestionRequest) error
}

type claudecodeCostTracker interface {
	Track(usd float64)
}

type claudecodeSoulLoader interface {
	Load() (string, error)
}

// fsmState is the FSM state for claudecode.Service.
type fsmState int32

const (
	fsmIdle         fsmState = iota // No subprocess running; ready for new task.
	fsmRunning                      // Subprocess running; processing output.
	fsmWaitingInput                 // Subprocess blocked on hook/pretool approval.
)

// Service is the interface satisfied by *serviceImpl.
// It satisfies infra.AgentHealthChecker via Name() + IsAlive().
type Service interface {
	Run(ctx context.Context) error
	SubmitTask(ctx context.Context, chatID int64, prompt string) error
	Stop(ctx context.Context) error
	IsAlive(ctx context.Context) bool
	Name() string // returns "claudecode"
}

// claudecodeServiceImpl implements Service.
type claudecodeServiceImpl struct {
	cfg       Config
	claudeBin string // path/name of the claude binary (injected for testing)
	ch        claudecodeChannel
	approver  claudecodeApprover
	guard     claudecodeCostTracker
	soul      claudecodeSoulLoader

	state   fsmState // accessed via atomic
	mu      sync.Mutex
	cmd     *exec.Cmd   // current subprocess; nil when Idle
	hookSrv *HookServer // long-lived hook server; started in Run()
}

// New constructs a claudecode.Service. All dependencies are injected as interfaces.
// In production, pass *infra.CostGuard and *infra.SOULLoader (both satisfy the narrow
// interfaces above). In tests, pass lightweight fakes.
// claudeBin is the path or name of the claude binary. In tests, pass the path to a
// fake script. In production, pass "claude" (resolved via PATH).
func New(cfg Config, claudeBin string, ch claudecodeChannel, approver claudecodeApprover, guard claudecodeCostTracker, soul claudecodeSoulLoader) Service {
	if cfg.HookServerAddr == "" {
		cfg.HookServerAddr = "127.0.0.1:8765"
	}
	return &claudecodeServiceImpl{
		cfg:       cfg,
		claudeBin: claudeBin,
		ch:        ch,
		approver:  approver,
		guard:     guard,
		soul:      soul,
	}
}

func (s *claudecodeServiceImpl) Name() string { return "claudecode" }

// Run starts the long-lived hook HTTP server and blocks until ctx is cancelled.
// The hook server is started once here (not per-task) so Claude Code can call
// gistclaw-hook at any point after Run() begins without a race condition.
func (s *claudecodeServiceImpl) Run(ctx context.Context) error {
	s.hookSrv = NewHookServer(s.cfg.HookServerAddr, 0, s.approver, s.ch)
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.hookSrv.ListenAndServe(ctx, s.cfg.HookServerAddr)
	}()
	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

// IsAlive returns true if the FSM is in any active state (Idle, Running, or WaitingInput).
// The service is considered alive as long as it has not crashed. A nil subprocess in
// Idle state means the service is alive and ready to accept work.
func (s *claudecodeServiceImpl) IsAlive(_ context.Context) bool {
	return true // FSM is always "alive" — not crashed; subprocess may or may not be running
}

// SubmitTask starts a `claude -p` subprocess and streams output to Telegram.
func (s *claudecodeServiceImpl) SubmitTask(ctx context.Context, chatID int64, prompt string) error {
	// Check FSM: reject if already running.
	if !atomic.CompareAndSwapInt32((*int32)(&s.state), int32(fsmIdle), int32(fsmRunning)) {
		_ = s.ch.SendMessage(ctx, chatID, "⚠️ Claude Code is busy. Wait for the current task to finish.")
		return nil
	}
	defer atomic.StoreInt32((*int32)(&s.state), int32(fsmIdle))

	// Merge .claude/settings.local.json with hook configuration.
	if err := s.patchSettings(); err != nil {
		log.Warn().Err(err).Msg("claudecode: could not patch settings.local.json; hooks may not work")
	}

	// Load SOUL.md.
	soul, err := s.soul.Load()
	if err != nil {
		log.Warn().Err(err).Msg("claudecode: could not load SOUL.md; proceeding without system prompt")
		soul = ""
	}

	// Build args.
	args := []string{"-p", prompt, "--output-format", "stream-json"}
	if soul != "" {
		args = append(args, "--system-prompt", soul)
	}

	cmd := exec.CommandContext(ctx, s.claudeBin, args...)
	cmd.Dir = s.cfg.Dir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("claudecode: stdout pipe: %w", err)
	}

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	if err := cmd.Start(); err != nil {
		s.mu.Lock()
		s.cmd = nil
		s.mu.Unlock()
		return fmt.Errorf("claudecode: start subprocess: %w", err)
	}

	// Update the long-lived hook server's chatID for this task.
	// The hook server was started in Run() and is shared across tasks.
	if s.hookSrv != nil {
		s.hookSrv.SetChatID(chatID)
	}

	// Parse stream-json output.
	var buf strings.Builder
	hadOutput := false
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		ev, err := ParseStreamLine(line)
		if err != nil {
			log.Warn().Err(err).Msg("claudecode: skip malformed stream-json line")
			continue
		}
		if ev == nil {
			continue
		}

		switch ev.Type {
		case "text":
			hadOutput = true
			buf.WriteString(ev.Text)
			// Flush at 4096-char boundary.
			for buf.Len() >= 4096 {
				chunk := buf.String()[:4096]
				_ = s.ch.SendMessage(ctx, chatID, chunk)
				rest := buf.String()[4096:]
				buf.Reset()
				buf.WriteString(rest)
			}

		case "result":
			s.guard.Track(ev.TotalCostUSD)
			// Flush remaining buffer.
			if buf.Len() > 0 {
				_ = s.ch.SendMessage(ctx, chatID, buf.String())
				buf.Reset()
				hadOutput = true
			}
			if ev.IsError {
				msg := ev.Result
				if msg == "" {
					msg = "Claude Code encountered an error."
				}
				_ = s.ch.SendMessage(ctx, chatID, "⚠️ "+msg)
			} else if !hadOutput {
				_ = s.ch.SendMessage(ctx, chatID, "⚠️ Agent finished but produced no output.")
			} else {
				_ = s.ch.SendMessage(ctx, chatID, "✅ Done")
			}
		}
	}

	// Subprocess finished.
	_ = cmd.Wait()

	s.mu.Lock()
	s.cmd = nil
	s.mu.Unlock()

	return scanner.Err()
}

// Stop sends SIGTERM to the subprocess, waits 2s, then sends SIGKILL.
func (s *claudecodeServiceImpl) Stop(_ context.Context) error {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		log.Warn().Err(err).Msg("claudecode: SIGTERM")
	}

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Exited cleanly after SIGTERM.
	case <-time.After(2 * time.Second):
		log.Warn().Msg("claudecode: subprocess did not exit after SIGTERM; sending SIGKILL")
		_ = cmd.Process.Kill()
	}
	return nil
}

// patchSettings merges hook configuration into .claude/settings.local.json.
func (s *claudecodeServiceImpl) patchSettings() error {
	settingsPath := s.cfg.SettingsPath
	if settingsPath == "" {
		settingsPath = filepath.Join(s.cfg.Dir, ".claude", "settings.local.json")
	}

	// Backup.
	existing, err := os.ReadFile(settingsPath)
	if err == nil {
		_ = os.WriteFile("/tmp/gistclaw-claude-settings.bak", existing, 0600)
	}

	// Read existing (or start with {}).
	var settings map[string]interface{}
	if err == nil {
		if jsonErr := json.Unmarshal(existing, &settings); jsonErr != nil {
			settings = map[string]interface{}{}
		}
	} else {
		settings = map[string]interface{}{}
	}

	// Patch only the hooks key.
	settings["hooks"] = map[string]interface{}{
		"PreToolUse": []map[string]interface{}{
			{
				"matcher": ".*",
				"hooks": []map[string]string{
					{"type": "command", "command": "gistclaw-hook"},
				},
			},
		},
		"PostToolUse": []map[string]interface{}{
			{
				"matcher": ".*",
				"hooks": []map[string]string{
					{"type": "command", "command": "gistclaw-hook --type notification"},
				},
			},
		},
		"Stop": []map[string]interface{}{
			{
				"hooks": []map[string]string{
					{"type": "command", "command": "gistclaw-hook --type stop"},
				},
			},
		},
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(settingsPath), err)
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	return os.WriteFile(settingsPath, out, 0644)
}
