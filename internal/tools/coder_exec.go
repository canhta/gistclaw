package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
)

type commandToolRunner interface {
	run(context.Context, commandRequest) (model.ToolResult, error)
}

type coderBackend interface {
	Name() string
	Build(coderExecInput, string) (commandRequest, error)
}

type coderExecInput struct {
	Backend          string `json:"backend"`
	Prompt           string `json:"prompt"`
	CWD              string `json:"cwd"`
	Sandbox          string `json:"sandbox"`
	SkipGitRepoCheck *bool  `json:"skip_git_repo_check"`
}

type CoderExecTool struct {
	runner   commandToolRunner
	backends map[string]coderBackend
}

func NewCoderExecTool(timeoutSec int, maxOutputBytes int) *CoderExecTool {
	return newCoderExecTool([]coderBackend{
		newCodexCoderBackend("codex"),
		newClaudeCodeBackend("claude"),
	}, newCommandRunner(timeoutSec, maxOutputBytes))
}

func newCoderExecTool(backends []coderBackend, runner commandToolRunner) *CoderExecTool {
	tool := &CoderExecTool{
		runner:   runner,
		backends: make(map[string]coderBackend, len(backends)),
	}
	for _, backend := range backends {
		if backend == nil {
			continue
		}
		tool.backends[backend.Name()] = backend
	}
	return tool
}

func (t *CoderExecTool) Name() string { return "coder_exec" }

func (t *CoderExecTool) Spec() model.ToolSpec {
	names := make([]string, 0, len(t.backends))
	for name := range t.backends {
		names = append(names, name)
	}
	sort.Strings(names)
	desc := "Run a registered coding CLI with a runtime-owned command contract."
	if len(names) > 0 {
		desc += " Available backends: " + strings.Join(names, ", ") + "."
	}
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     desc,
		InputSchemaJSON: `{"type":"object","properties":{"backend":{"type":"string"},"prompt":{"type":"string"},"cwd":{"type":"string"},"sandbox":{"type":"string"},"skip_git_repo_check":{"type":"boolean"}},"required":["prompt"],"additionalProperties":false}`,
		Risk:            model.RiskHigh,
		SideEffect:      effectExecWrite,
		Approval:        "maybe",
	}
}

func (t *CoderExecTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := workspaceRootFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	if t == nil || t.runner == nil {
		return model.ToolResult{}, fmt.Errorf("coder_exec: runner is required")
	}

	var input coderExecInput
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("coder_exec: decode input: %w", err)
	}
	input.Prompt = strings.TrimSpace(input.Prompt)
	if input.Prompt == "" {
		return model.ToolResult{}, fmt.Errorf("coder_exec: prompt is required")
	}
	input.Backend = strings.TrimSpace(input.Backend)
	if input.Backend == "" {
		if len(t.backends) == 1 {
			for name := range t.backends {
				input.Backend = name
			}
		}
	}
	backend, ok := t.backends[input.Backend]
	if !ok {
		return model.ToolResult{}, fmt.Errorf("coder_exec: backend %q is not available", input.Backend)
	}

	cwd := root
	if strings.TrimSpace(input.CWD) != "" {
		var resolveErr error
		cwd, _, resolveErr = resolveWorkspacePath(root, input.CWD)
		if resolveErr != nil {
			return model.ToolResult{}, fmt.Errorf("coder_exec: cwd: %w", resolveErr)
		}
	}

	req, err := backend.Build(input, cwd)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("coder_exec: build %s command: %w", input.Backend, err)
	}
	result, err := t.runner.run(ctx, req)
	wrapped, wrapErr := wrapCoderExecResult(input.Backend, result)
	if wrapErr != nil {
		return model.ToolResult{}, wrapErr
	}
	return wrapped, err
}

func wrapCoderExecResult(backend string, result model.ToolResult) (model.ToolResult, error) {
	if strings.TrimSpace(result.Output) == "" {
		payload, err := json.Marshal(map[string]any{"backend": backend})
		if err != nil {
			return model.ToolResult{}, fmt.Errorf("coder_exec: encode output: %w", err)
		}
		return model.ToolResult{Output: string(payload), Error: result.Error}, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Output), &payload); err != nil {
		return model.ToolResult{}, fmt.Errorf("coder_exec: decode runner output: %w", err)
	}
	payload["backend"] = backend
	output, err := json.Marshal(payload)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("coder_exec: encode output: %w", err)
	}
	return model.ToolResult{Output: string(output), Error: result.Error}, nil
}

type codexCoderBackend struct {
	command string
}

func newCodexCoderBackend(command string) coderBackend {
	command = strings.TrimSpace(command)
	if command == "" {
		command = "codex"
	}
	return codexCoderBackend{command: command}
}

func (b codexCoderBackend) Name() string { return "codex" }

func (b codexCoderBackend) Build(input coderExecInput, cwd string) (commandRequest, error) {
	sandbox := strings.TrimSpace(input.Sandbox)
	if sandbox == "" {
		sandbox = "workspace-write"
	}
	if sandbox != "read-only" && sandbox != "workspace-write" && sandbox != "danger-full-access" {
		return commandRequest{}, fmt.Errorf("unsupported sandbox %q", sandbox)
	}

	skipGitRepoCheck := true
	if input.SkipGitRepoCheck != nil {
		skipGitRepoCheck = *input.SkipGitRepoCheck
	}

	args := []string{"exec", "--sandbox", sandbox}
	if skipGitRepoCheck {
		args = append(args, "--skip-git-repo-check")
	}
	args = append(args, "-C", cwd, input.Prompt)
	return commandRequest{
		command: b.command,
		args:    args,
		cwd:     cwd,
		effect:  effectExecWrite,
	}, nil
}

type claudeCodeBackend struct {
	command string
}

func newClaudeCodeBackend(command string) coderBackend {
	command = strings.TrimSpace(command)
	if command == "" {
		command = "claude"
	}
	return claudeCodeBackend{command: command}
}

func (b claudeCodeBackend) Name() string { return "claude_code" }

func (b claudeCodeBackend) Build(input coderExecInput, cwd string) (commandRequest, error) {
	args := []string{
		"--print",
		"--output-format", "json",
		"--permission-mode", "acceptEdits",
		input.Prompt,
	}
	return commandRequest{
		command: b.command,
		args:    args,
		cwd:     cwd,
		effect:  effectExecWrite,
	}, nil
}
