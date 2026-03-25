package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/model"
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
	command string
	args    []string
	cwd     string
	stdin   string
	effect  string
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

	cmd := exec.CommandContext(runCtx, req.command, req.args...)
	cmd.Dir = req.cwd
	if req.stdin != "" {
		cmd.Stdin = strings.NewReader(req.stdin)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := commandResult{
		Command: strings.TrimSpace(strings.Join(append([]string{req.command}, req.args...), " ")),
		CWD:     req.cwd,
		Stdout:  stdout.String(),
		Stderr:  stderr.String(),
		Effect:  req.effect,
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
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
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
		Description:     "Apply one unified diff patch inside the workspace root.",
		InputSchemaJSON: `{"type":"object","properties":{"patch":{"type":"string"}},"required":["patch"]}`,
		Risk:            model.RiskMedium,
		SideEffect:      effectPatch,
		Approval:        "required",
	}
}

func (t *ApplyPatchTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := workspaceRootFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	var input struct {
		Patch string `json:"patch"`
	}
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("apply_patch: decode input: %w", err)
	}
	if strings.TrimSpace(input.Patch) == "" {
		return model.ToolResult{}, fmt.Errorf("apply_patch: patch is required")
	}
	return t.runner.run(ctx, commandRequest{
		command: "git",
		args:    []string{"apply", "--recount", "--whitespace=nowarn", "-"},
		cwd:     root,
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
		Description:     "Run one shell command inside the workspace root or a child directory.",
		InputSchemaJSON: `{"type":"object","properties":{"command":{"type":"string"},"cwd":{"type":"string"},"timeout_sec":{"type":"integer","minimum":1}},"required":["command"]}`,
		Risk:            model.RiskHigh,
		SideEffect:      effectExecWrite,
		Approval:        "maybe",
	}
}

func (t *ShellExecTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := workspaceRootFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	var input struct {
		Command string `json:"command"`
		CWD     string `json:"cwd"`
	}
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("shell_exec: decode input: %w", err)
	}
	if err := validateShellArgs(input.Command); err != nil {
		return model.ToolResult{}, err
	}
	cwd := root
	if strings.TrimSpace(input.CWD) != "" {
		cwd, _, err = resolveWorkspacePath(root, input.CWD)
		if err != nil {
			return model.ToolResult{}, fmt.Errorf("shell_exec: cwd: %w", err)
		}
		info, err := os.Stat(cwd)
		if err != nil {
			return model.ToolResult{}, fmt.Errorf("shell_exec: stat cwd: %w", err)
		}
		if !info.IsDir() {
			return model.ToolResult{}, fmt.Errorf("shell_exec: cwd must be a directory")
		}
	}
	effect := classifyShellCommand(input.Command)
	return t.runner.run(ctx, commandRequest{
		command: "zsh",
		args:    []string{"-lc", input.Command},
		cwd:     cwd,
		effect:  effect,
	})
}

type RunTestsTool struct {
	runner commandRunner
}

func NewRunTestsTool(timeoutSec int, maxOutputBytes int) *RunTestsTool {
	return &RunTestsTool{runner: newCommandRunner(timeoutSec, maxOutputBytes)}
}

func (t *RunTestsTool) Name() string { return "run_tests" }

func (t *RunTestsTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Run the repository's default test command from the workspace root.",
		InputSchemaJSON: `{"type":"object","properties":{"target":{"type":"string"}}}`,
		Risk:            model.RiskLow,
		SideEffect:      effectExecRead,
		Approval:        "never",
	}
}

func (t *RunTestsTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := workspaceRootFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	var input struct {
		Target string `json:"target"`
	}
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("run_tests: decode input: %w", err)
	}
	command, args, err := detectTestCommand(root, strings.TrimSpace(input.Target))
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("run_tests: %w", err)
	}
	return t.runner.run(ctx, commandRequest{
		command: command,
		args:    args,
		cwd:     root,
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
		Description:     "Run the repository's default build command from the workspace root.",
		InputSchemaJSON: `{"type":"object","properties":{"target":{"type":"string"}}}`,
		Risk:            model.RiskLow,
		SideEffect:      effectExecRead,
		Approval:        "never",
	}
}

func (t *RunBuildTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := workspaceRootFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	var input struct {
		Target string `json:"target"`
	}
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("run_build: decode input: %w", err)
	}
	command, args, err := detectBuildCommand(root, strings.TrimSpace(input.Target))
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("run_build: %w", err)
	}
	return t.runner.run(ctx, commandRequest{
		command: command,
		args:    args,
		cwd:     root,
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
