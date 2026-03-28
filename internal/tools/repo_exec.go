package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/creack/pty/v2"
)

type commandResult struct {
	Command   string `json:"command"`
	CWD       string `json:"cwd"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exit_code"`
	TimedOut  bool   `json:"timed_out"`
	Truncated bool   `json:"truncated"`
	Effect    string `json:"effect"`
}

type commandRunner struct {
	timeout   time.Duration
	maxOutput int
}

type commandRequest struct {
	command           string
	args              []string
	cwd               string
	stdin             string
	effect            string
	outputCapturePath string
	usePTY            bool
}

type shellCommand struct {
	command string
	args    []string
}

func newCommandRunner(timeoutSec int, maxOutputBytes int) commandRunner {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	if maxOutputBytes <= 0 {
		maxOutputBytes = 64 << 10
	}
	return commandRunner{
		timeout:   time.Duration(timeoutSec) * time.Second,
		maxOutput: maxOutputBytes,
	}
}

func (r commandRunner) run(ctx context.Context, req commandRequest) (model.ToolResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	if req.usePTY && strings.TrimSpace(req.stdin) == "" {
		return r.runPTY(runCtx, req)
	}
	return r.runPiped(runCtx, req)
}

func (r commandRunner) runPiped(ctx context.Context, req commandRequest) (model.ToolResult, error) {
	cmd := exec.CommandContext(ctx, req.command, req.args...)
	cmd.Dir = req.cwd
	if req.stdin != "" {
		cmd.Stdin = strings.NewReader(req.stdin)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("tools: stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("tools: stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return model.ToolResult{}, fmt.Errorf("tools: start command: %w", err)
	}

	sink := toolLogSinkFromContext(ctx)
	stdoutDone := make(chan error, 1)
	stderrDone := make(chan error, 1)
	go func() {
		stdoutDone <- streamCommandOutput(ctx, stdoutPipe, &stdout, sink, "stdout")
	}()
	go func() {
		stderrDone <- streamCommandOutput(ctx, stderrPipe, &stderr, sink, "stderr")
	}()

	err = cmd.Wait()
	stdoutErr := <-stdoutDone
	stderrErr := <-stderrDone
	if err == nil {
		if stdoutErr != nil && !isClosedCommandPipe(stdoutErr) {
			err = stdoutErr
		} else if stderrErr != nil && !isClosedCommandPipe(stderrErr) {
			err = stderrErr
		}
	}
	result := commandResult{
		Command: strings.TrimSpace(strings.Join(append([]string{req.command}, req.args...), " ")),
		CWD:     req.cwd,
		Stdout:  stdout.String(),
		Stderr:  stderr.String(),
		Effect:  req.effect,
	}
	if strings.TrimSpace(req.outputCapturePath) != "" {
		if captured, readErr := os.ReadFile(req.outputCapturePath); readErr == nil {
			text := strings.TrimSpace(string(captured))
			if text != "" {
				result.Stdout = text
			}
		}
		_ = os.Remove(req.outputCapturePath)
	}
	if len(result.Stdout) > r.maxOutput {
		result.Stdout = result.Stdout[:r.maxOutput]
		result.Truncated = true
	}
	if len(result.Stderr) > r.maxOutput {
		result.Stderr = result.Stderr[:r.maxOutput]
		result.Truncated = true
	}

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		result.ExitCode = 0
	case errors.As(err, &exitErr):
		result.ExitCode = exitErr.ExitCode()
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		result.ExitCode = -1
		result.TimedOut = true
	default:
		result.ExitCode = -1
	}

	output, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return model.ToolResult{}, fmt.Errorf("tools: encode command result: %w", marshalErr)
	}
	toolResult := model.ToolResult{Output: string(output)}
	if err != nil {
		if result.TimedOut {
			return toolResult, fmt.Errorf("tools: command timed out")
		}
		return toolResult, fmt.Errorf("tools: command failed: %w", err)
	}
	return toolResult, nil
}

func (r commandRunner) runPTY(ctx context.Context, req commandRequest) (model.ToolResult, error) {
	cmd := exec.CommandContext(ctx, req.command, req.args...)
	cmd.Dir = req.cwd

	tty, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 120, Rows: 40})
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("tools: start PTY command: %w", err)
	}

	var terminal bytes.Buffer
	sink := toolLogSinkFromContext(ctx)
	readDone := make(chan error, 1)
	go func() {
		readDone <- streamTerminalOutput(ctx, tty, &terminal, sink)
	}()

	err = cmd.Wait()
	_ = tty.Close()
	readErr := <-readDone
	if err == nil && readErr != nil && !errors.Is(readErr, os.ErrClosed) {
		err = readErr
	}

	result := commandResult{
		Command: strings.TrimSpace(strings.Join(append([]string{req.command}, req.args...), " ")),
		CWD:     req.cwd,
		Stdout:  terminal.String(),
		Effect:  req.effect,
	}
	if strings.TrimSpace(req.outputCapturePath) != "" {
		if captured, readErr := os.ReadFile(req.outputCapturePath); readErr == nil {
			text := strings.TrimSpace(string(captured))
			if text != "" {
				result.Stdout = text
			}
		}
		_ = os.Remove(req.outputCapturePath)
	}
	if len(result.Stdout) > r.maxOutput {
		result.Stdout = result.Stdout[:r.maxOutput]
		result.Truncated = true
	}

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		result.ExitCode = 0
	case errors.As(err, &exitErr):
		result.ExitCode = exitErr.ExitCode()
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		result.ExitCode = -1
		result.TimedOut = true
	default:
		result.ExitCode = -1
	}

	output, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return model.ToolResult{}, fmt.Errorf("tools: encode command result: %w", marshalErr)
	}
	toolResult := model.ToolResult{Output: string(output)}
	if err != nil {
		if result.TimedOut {
			return toolResult, fmt.Errorf("tools: command timed out")
		}
		return toolResult, fmt.Errorf("tools: command failed: %w", err)
	}
	return toolResult, nil
}

func streamCommandOutput(ctx context.Context, reader io.Reader, buffer *bytes.Buffer, sink ToolLogSink, stream string) error {
	buf := bufio.NewReader(reader)
	for {
		chunk, err := buf.ReadString('\n')
		if chunk != "" {
			buffer.WriteString(chunk)
			if sink != nil {
				if err := sink.Record(ctx, ToolLogRecord{
					Stream:     stream,
					Text:       chunk,
					OccurredAt: time.Now().UTC(),
				}); err != nil {
					return err
				}
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

func streamTerminalOutput(ctx context.Context, reader io.Reader, buffer *bytes.Buffer, sink ToolLogSink) error {
	chunk := make([]byte, 4096)
	for {
		n, err := reader.Read(chunk)
		if n > 0 {
			text := string(chunk[:n])
			buffer.WriteString(text)
			if sink != nil {
				if err := sink.Record(ctx, ToolLogRecord{
					Stream:     "terminal",
					Text:       text,
					OccurredAt: time.Now().UTC(),
				}); err != nil {
					return err
				}
			}
		}
		if err == nil {
			continue
		}
		if isExpectedTerminalReadClose(err) {
			return nil
		}
		return err
	}
}

func isExpectedTerminalReadClose(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
		return true
	}
	return strings.Contains(err.Error(), "input/output error")
}

func isClosedCommandPipe(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, os.ErrClosed) ||
		errors.Is(err, io.ErrClosedPipe) ||
		strings.Contains(err.Error(), "file already closed")
}

type ApplyPatchTool struct {
	runner commandRunner
}

func NewApplyPatchTool(timeoutSec int) *ApplyPatchTool {
	return &ApplyPatchTool{runner: newCommandRunner(timeoutSec, 64<<10)}
}

func (t *ApplyPatchTool) Name() string { return "apply_patch" }

func (t *ApplyPatchTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Apply one unified diff patch inside the run directory or an explicit host directory when authority allows.",
		InputSchemaJSON: `{"type":"object","properties":{"patch":{"type":"string"},"cwd":{"type":"string"}},"required":["patch"]}`,
		Family:          model.ToolFamilyRepoWrite,
		Risk:            model.RiskMedium,
		SideEffect:      effectPatch,
		Approval:        "required",
	}
}

func (t *ApplyPatchTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := cwdFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	var input struct {
		Patch string `json:"patch"`
		CWD   string `json:"cwd"`
	}
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("apply_patch: decode input: %w", err)
	}
	if strings.TrimSpace(input.Patch) == "" {
		return model.ToolResult{}, fmt.Errorf("apply_patch: patch is required")
	}
	cwd, err := resolveToolCWD(root, input.CWD, authorityFromContext(ctx))
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("apply_patch: cwd: %w", err)
	}
	return t.runner.run(ctx, commandRequest{
		command: "git",
		args:    []string{"apply", "--recount", "--whitespace=nowarn", "-"},
		cwd:     cwd,
		stdin:   input.Patch,
		effect:  effectPatch,
	})
}

type ShellExecTool struct {
	runner commandRunner
}

func NewShellExecTool(timeoutSec int, maxOutputBytes int) *ShellExecTool {
	return &ShellExecTool{runner: newCommandRunner(timeoutSec, maxOutputBytes)}
}

func (t *ShellExecTool) Name() string { return "shell_exec" }

func (t *ShellExecTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Run one shell command inside the run directory or an explicit host directory when authority allows.",
		InputSchemaJSON: `{"type":"object","properties":{"command":{"type":"string"},"cwd":{"type":"string"},"timeout_sec":{"type":"integer","minimum":1}},"required":["command"]}`,
		Family:          model.ToolFamilyRepoWrite,
		Risk:            model.RiskHigh,
		SideEffect:      effectExecWrite,
		Approval:        "maybe",
	}
}

func (t *ShellExecTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := cwdFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	env := authorityFromContext(ctx)
	var input struct {
		Command string `json:"command"`
		CWD     string `json:"cwd"`
	}
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("shell_exec: decode input: %w", err)
	}
	if err := validateShellArgs(input.Command); err != nil {
		meta, ok := InvocationContextFrom(ctx)
		if !ok || strings.TrimSpace(meta.ApprovalID) == "" {
			return model.ToolResult{}, err
		}
	}
	cwd := root
	if strings.TrimSpace(input.CWD) != "" {
		cwd, err = resolveToolCWD(root, input.CWD, env)
		if err != nil {
			return model.ToolResult{}, fmt.Errorf("shell_exec: cwd: %w", err)
		}
	}
	effect := classifyShellCommand(input.Command)
	shell, err := resolveShellCommand()
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("shell_exec: resolve shell: %w", err)
	}
	return t.runner.run(ctx, commandRequest{
		command: shell.command,
		args:    append(shell.args, input.Command),
		cwd:     cwd,
		effect:  effect,
	})
}

func resolveShellCommand() (shellCommand, error) {
	candidates := candidateShells()
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		path, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		return shellCommand{
			command: path,
			args:    shellArgsForPath(path),
		}, nil
	}
	return shellCommand{}, fmt.Errorf("no supported shell found in PATH")
}

func candidateShells() []string {
	return []string{
		strings.TrimSpace(os.Getenv("SHELL")),
		"zsh",
		"bash",
		"sh",
	}
}

func shellArgsForPath(path string) []string {
	switch filepath.Base(path) {
	case "bash", "zsh":
		return []string{"-lc"}
	default:
		return []string{"-c"}
	}
}

type RunTestsTool struct {
	runner commandRunner
}

type repoCommandInput struct {
	Target string `json:"target"`
	CWD    string `json:"cwd"`
}

func NewRunTestsTool(timeoutSec int, maxOutputBytes int) *RunTestsTool {
	return &RunTestsTool{runner: newCommandRunner(timeoutSec, maxOutputBytes)}
}

func (t *RunTestsTool) Name() string { return "run_tests" }

func (t *RunTestsTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Run the repository's default test command from the run directory or an explicit host directory when authority allows.",
		InputSchemaJSON: `{"type":"object","properties":{"target":{"type":"string"},"cwd":{"type":"string"}}}`,
		Family:          model.ToolFamilyRepoRead,
		Risk:            model.RiskLow,
		SideEffect:      effectExecRead,
		Approval:        "never",
	}
}

func (t *RunTestsTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := cwdFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	var input repoCommandInput
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("run_tests: decode input: %w", err)
	}
	cwd, err := resolveToolCWD(root, input.CWD, authorityFromContext(ctx))
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("run_tests: cwd: %w", err)
	}
	command, args, err := detectTestCommand(cwd, strings.TrimSpace(input.Target))
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("run_tests: %w", err)
	}
	return t.runner.run(ctx, commandRequest{
		command: command,
		args:    args,
		cwd:     cwd,
		effect:  effectExecRead,
	})
}

type RunBuildTool struct {
	runner commandRunner
}

func NewRunBuildTool(timeoutSec int, maxOutputBytes int) *RunBuildTool {
	return &RunBuildTool{runner: newCommandRunner(timeoutSec, maxOutputBytes)}
}

func (t *RunBuildTool) Name() string { return "run_build" }

func (t *RunBuildTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Run the repository's default build command from the run directory or an explicit host directory when authority allows.",
		InputSchemaJSON: `{"type":"object","properties":{"target":{"type":"string"},"cwd":{"type":"string"}}}`,
		Family:          model.ToolFamilyRepoRead,
		Risk:            model.RiskLow,
		SideEffect:      effectExecRead,
		Approval:        "never",
	}
}

func (t *RunBuildTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := cwdFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	var input repoCommandInput
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("run_build: decode input: %w", err)
	}
	cwd, err := resolveToolCWD(root, input.CWD, authorityFromContext(ctx))
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("run_build: cwd: %w", err)
	}
	command, args, err := detectBuildCommand(cwd, strings.TrimSpace(input.Target))
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("run_build: %w", err)
	}
	return t.runner.run(ctx, commandRequest{
		command: command,
		args:    args,
		cwd:     cwd,
		effect:  effectExecRead,
	})
}

func detectTestCommand(root, target string) (string, []string, error) {
	switch {
	case fileExists(filepath.Join(root, "go.mod")):
		if target == "" {
			target = "./..."
		}
		return "go", []string{"test", target}, nil
	case fileExists(filepath.Join(root, "Cargo.toml")):
		args := []string{"test"}
		if target != "" {
			args = append(args, target)
		}
		return "cargo", args, nil
	case fileExists(filepath.Join(root, "package.json")):
		args := []string{"test"}
		if target != "" {
			args = append(args, "--", target)
		}
		return "npm", args, nil
	case fileExists(filepath.Join(root, "pyproject.toml")) || fileExists(filepath.Join(root, "requirements.txt")):
		args := []string{"-m", "pytest"}
		if target != "" {
			args = append(args, target)
		}
		return "python3", args, nil
	default:
		return "", nil, fmt.Errorf("could not determine test command")
	}
}

func detectBuildCommand(root, target string) (string, []string, error) {
	switch {
	case fileExists(filepath.Join(root, "go.mod")):
		if target == "" {
			target = "./..."
		}
		return "go", []string{"build", target}, nil
	case fileExists(filepath.Join(root, "Cargo.toml")):
		args := []string{"build"}
		if target != "" {
			args = append(args, target)
		}
		return "cargo", args, nil
	case fileExists(filepath.Join(root, "package.json")):
		args := []string{"run", "build"}
		if target != "" {
			args = append(args, "--", target)
		}
		return "npm", args, nil
	default:
		return "", nil, fmt.Errorf("could not determine build command")
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
