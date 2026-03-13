package claudecode

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

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
	fsmIdle    fsmState = iota // No subprocess running; ready for new task.
	fsmRunning                 // Subprocess running (including HITL waits).
)

// Service is the interface satisfied by *serviceImpl.
// It satisfies infra.AgentHealthChecker via Name() + IsAlive().
type Service interface {
	Run(ctx context.Context) error
	SubmitTask(ctx context.Context, chatID int64, prompt string) error
	// SubmitTaskWithResult submits a prompt and blocks until the agent finishes,
	// returning the full concatenated text output. Streams output to Telegram normally.
	SubmitTaskWithResult(ctx context.Context, chatID int64, prompt string) (string, error)
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

	state     fsmState    // accessed via atomic
	runExited atomic.Bool // set to true when Run() returns; used by IsAlive
	mu        sync.Mutex
	cmd       *exec.Cmd   // current subprocess; nil when Idle
	hookSrv   *HookServer // long-lived hook server; started in Run()
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
// Sets runExited=true on return so IsAlive() can detect that Run has crashed.
func (s *claudecodeServiceImpl) Run(ctx context.Context) error {
	defer s.runExited.Store(true)
	srv := NewHookServer(s.cfg.HookServerAddr, 0, s.approver, s.ch)
	s.mu.Lock()
	s.hookSrv = srv
	s.mu.Unlock()
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe(ctx, s.cfg.HookServerAddr)
	}()
	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

// IsAlive returns true as long as Run() has not exited. The service is
// considered alive in Idle state (ready to accept work) and in Running state
// (subprocess active). It is only "dead" after Run() returns, which means the
// hook server has crashed and the supervisor has not yet restarted it.
func (s *claudecodeServiceImpl) IsAlive(_ context.Context) bool {
	return !s.runExited.Load()
}

// SubmitTask starts a `claude -p` subprocess and streams output to Telegram.
func (s *claudecodeServiceImpl) SubmitTask(ctx context.Context, chatID int64, prompt string) error {
	return s.submitAndStream(ctx, chatID, prompt, nil)
}

// SubmitTaskWithResult submits a prompt and blocks until the agent finishes,
// returning the full concatenated text output. Streams output to Telegram normally.
func (s *claudecodeServiceImpl) SubmitTaskWithResult(ctx context.Context, chatID int64, prompt string) (string, error) {
	var acc strings.Builder
	if err := s.submitAndStream(ctx, chatID, prompt, &acc); err != nil {
		return "", err
	}
	return acc.String(), nil
}

// submitAndStream is the shared implementation for SubmitTask and SubmitTaskWithResult.
// If acc is non-nil, all text event content is appended to it.
func (s *claudecodeServiceImpl) submitAndStream(ctx context.Context, chatID int64, prompt string, acc *strings.Builder) error {
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
	s.mu.Lock()
	hookSrv := s.hookSrv
	s.mu.Unlock()
	if hookSrv != nil {
		hookSrv.SetChatID(chatID)
	}

	// Parse stream-json output.
	var buf strings.Builder
	hadOutput := false
	gotResult := false
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
			if acc != nil {
				acc.WriteString(ev.Text)
			}
			// Flush at 4096-char boundary; step back to a valid UTF-8 boundary.
			for buf.Len() >= 4096 {
				str := buf.String()
				cut := 4096
				for cut > 0 && !utf8.ValidString(str[:cut]) {
					cut--
				}
				chunk := str[:cut]
				rest := str[cut:]
				buf.Reset()
				buf.WriteString(rest)
				_ = s.ch.SendMessage(ctx, chatID, chunk)
			}

		case "result":
			gotResult = true
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

	// Subprocess finished — submitAndStream owns Wait().
	_ = cmd.Wait()

	if !gotResult {
		_ = s.ch.SendMessage(ctx, chatID, "⚠️ Claude Code exited unexpectedly.")
	}

	s.mu.Lock()
	s.cmd = nil
	s.mu.Unlock()

	return scanner.Err()
}

// Stop sends SIGTERM to the subprocess; if it has not exited after 2s, sends SIGKILL.
// The kill-after-delay runs in a goroutine to avoid blocking the caller.
// SubmitTask owns cmd.Wait(); Stop only signals.
func (s *claudecodeServiceImpl) Stop(_ context.Context) error {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}

	// Signal SIGTERM. SubmitTask owns Wait(); we just signal.
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		log.Warn().Err(err).Msg("claudecode: SIGTERM")
		return nil
	}

	// Give process 2s to exit. If still alive, send SIGKILL.
	// Run in a goroutine so Stop() returns immediately.
	go func() {
		time.Sleep(2 * time.Second)
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			log.Warn().Err(err).Msg("claudecode: SIGKILL")
		}
	}()
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
		_ = os.WriteFile(settingsPath+".bak", existing, 0600)
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
	// Include --addr flag when using a non-default hook server address so that
	// gistclaw-hook connects to the correct server in multi-instance deployments.
	addrFlag := ""
	if s.cfg.HookServerAddr != "" && s.cfg.HookServerAddr != "127.0.0.1:8765" {
		addrFlag = " --addr " + s.cfg.HookServerAddr
	}
	settings["hooks"] = map[string]interface{}{
		"PreToolUse": []map[string]interface{}{
			{
				"matcher": ".*",
				"hooks": []map[string]string{
					{"type": "command", "command": "gistclaw-hook" + addrFlag},
				},
			},
		},
		"PostToolUse": []map[string]interface{}{
			{
				"matcher": ".*",
				"hooks": []map[string]string{
					{"type": "command", "command": "gistclaw-hook --type notification" + addrFlag},
				},
			},
		},
		"Stop": []map[string]interface{}{
			{
				"hooks": []map[string]string{
					{"type": "command", "command": "gistclaw-hook --type stop" + addrFlag},
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
