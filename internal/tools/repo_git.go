package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
)

type gitInput struct {
	Target string `json:"target"`
	Limit  int    `json:"limit"`
}

type GitStatusTool struct{ runner commandRunner }

func NewGitStatusTool(timeoutSec int, maxOutputBytes int) *GitStatusTool {
	return &GitStatusTool{runner: newCommandRunner(timeoutSec, maxOutputBytes)}
}

func (t *GitStatusTool) Name() string { return "git_status" }

func (t *GitStatusTool) Spec() model.ToolSpec {
	return gitSpec(t.Name(), "Show git status for the repository in the current working directory.")
}

func (t *GitStatusTool) Invoke(ctx context.Context, _ model.ToolCall) (model.ToolResult, error) {
	root, err := cwdFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	return t.runner.run(ctx, commandRequest{
		command: "git",
		args:    []string{"status", "--short", "--branch"},
		cwd:     root,
		effect:  effectRead,
	})
}

type GitDiffTool struct{ runner commandRunner }

func NewGitDiffTool(timeoutSec int, maxOutputBytes int) *GitDiffTool {
	return &GitDiffTool{runner: newCommandRunner(timeoutSec, maxOutputBytes)}
}

func (t *GitDiffTool) Name() string { return "git_diff" }

func (t *GitDiffTool) Spec() model.ToolSpec {
	return gitSpec(t.Name(), "Show git diff output for the repository in the current working directory.")
}

func (t *GitDiffTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := cwdFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	var input gitInput
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("git_diff: decode input: %w", err)
	}
	args := []string{"diff"}
	if target := strings.TrimSpace(input.Target); target != "" {
		args = append(args, target)
	}
	return t.runner.run(ctx, commandRequest{
		command: "git",
		args:    args,
		cwd:     root,
		effect:  effectRead,
	})
}

type GitShowTool struct{ runner commandRunner }

func NewGitShowTool(timeoutSec int, maxOutputBytes int) *GitShowTool {
	return &GitShowTool{runner: newCommandRunner(timeoutSec, maxOutputBytes)}
}

func (t *GitShowTool) Name() string { return "git_show" }

func (t *GitShowTool) Spec() model.ToolSpec {
	return gitSpec(t.Name(), "Show one git object or revision from the repository in the current working directory.")
}

func (t *GitShowTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := cwdFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	var input gitInput
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("git_show: decode input: %w", err)
	}
	target := strings.TrimSpace(input.Target)
	if target == "" {
		target = "HEAD"
	}
	return t.runner.run(ctx, commandRequest{
		command: "git",
		args:    []string{"show", target, "--stat", "--oneline"},
		cwd:     root,
		effect:  effectRead,
	})
}

type GitLogTool struct{ runner commandRunner }

func NewGitLogTool(timeoutSec int, maxOutputBytes int) *GitLogTool {
	return &GitLogTool{runner: newCommandRunner(timeoutSec, maxOutputBytes)}
}

func (t *GitLogTool) Name() string { return "git_log" }

func (t *GitLogTool) Spec() model.ToolSpec {
	return gitSpec(t.Name(), "Show recent commit history from the repository in the current working directory.")
}

func (t *GitLogTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	root, err := cwdFromContext(ctx)
	if err != nil {
		return model.ToolResult{}, err
	}
	var input gitInput
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("git_log: decode input: %w", err)
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}
	return t.runner.run(ctx, commandRequest{
		command: "git",
		args:    []string{"log", "--oneline", fmt.Sprintf("-%d", limit)},
		cwd:     root,
		effect:  effectRead,
	})
}

func gitSpec(name, description string) model.ToolSpec {
	return model.ToolSpec{
		Name:            name,
		Description:     description,
		InputSchemaJSON: `{"type":"object","properties":{"target":{"type":"string"},"limit":{"type":"integer","minimum":1}}}`,
		Risk:            model.RiskLow,
		SideEffect:      effectRead,
		Approval:        "never",
	}
}
