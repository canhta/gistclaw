package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/authority"
	"github.com/canhta/gistclaw/internal/model"
)

type recordingCommandRunner struct {
	calls int
	req   commandRequest
	res   model.ToolResult
	err   error
}

func (r *recordingCommandRunner) run(_ context.Context, req commandRequest) (model.ToolResult, error) {
	r.calls++
	r.req = req
	return r.res, r.err
}

func TestCoderExecTool_InvokeBuildsCodexCommand(t *testing.T) {
	root := t.TempDir()
	runner := &recordingCommandRunner{
		res: model.ToolResult{Output: `{"command":"codex exec --sandbox workspace-write --skip-git-repo-check -C /tmp/project \"Build it\"","cwd":"/tmp/project","stdout":"ok","stderr":"","exit_code":0,"timed_out":false,"truncated":false,"effect":"exec_write"}`},
	}
	tool := newCoderExecTool([]coderBackend{
		newCodexCoderBackend("codex"),
	}, runner)

	got, err := tool.Invoke(withToolContext(context.Background(), root), model.ToolCall{
		ID:       "call-coder",
		ToolName: tool.Name(),
		InputJSON: []byte(`{
			"backend":"codex",
			"cwd":"project",
			"prompt":"Build it"
		}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("expected 1 runner call, got %d", runner.calls)
	}
	if runner.req.command != "codex" {
		t.Fatalf("expected codex command, got %q", runner.req.command)
	}
	wantCWD, _, err := resolveScopedPath(root, "project")
	if err != nil {
		t.Fatalf("resolveScopedPath: %v", err)
	}
	if runner.req.cwd != wantCWD {
		t.Fatalf("expected cwd %q, got %q", wantCWD, runner.req.cwd)
	}
	wantArgs := []string{
		"exec",
		"--sandbox", "workspace-write",
		"--color", "never",
		"-o",
		"<capture>",
		"--skip-git-repo-check",
		"-C", wantCWD,
	}
	if len(runner.req.args) != len(wantArgs)+1 {
		t.Fatalf("expected %d args, got %d (%v)", len(wantArgs), len(runner.req.args), runner.req.args)
	}
	for i := range wantArgs {
		if wantArgs[i] == "<capture>" {
			if strings.TrimSpace(runner.req.args[i]) == "" {
				t.Fatalf("expected non-empty output capture path, got %q", runner.req.args[i])
			}
			continue
		}
		if runner.req.args[i] != wantArgs[i] {
			t.Fatalf("arg %d: want %q, got %q", i, wantArgs[i], runner.req.args[i])
		}
	}
	wrappedPrompt := runner.req.args[len(runner.req.args)-1]
	for _, want := range []string{
		"You are a non-interactive coding subagent running inside GistClaw.",
		"You were dispatched as a subagent to execute a specific task.",
		"Do not ask the user questions.",
		"Build it",
	} {
		if !strings.Contains(wrappedPrompt, want) {
			t.Fatalf("expected wrapped codex prompt to contain %q, got %q", want, wrappedPrompt)
		}
	}

	var payload struct {
		Backend string `json:"backend"`
		Command string `json:"command"`
		CWD     string `json:"cwd"`
		Stdout  string `json:"stdout"`
	}
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.Backend != "codex" {
		t.Fatalf("expected codex backend, got %q", payload.Backend)
	}
	if payload.CWD != "/tmp/project" {
		t.Fatalf("expected wrapped cwd from runner output, got %q", payload.CWD)
	}
	if payload.Stdout != "ok" {
		t.Fatalf("expected stdout passthrough, got %q", payload.Stdout)
	}
}

func TestCommandRunner_PrefersCapturedOutputFile(t *testing.T) {
	runner := newCommandRunner(5, 64<<10)
	capturePath := filepath.Join(t.TempDir(), "last-message.txt")
	root := t.TempDir()
	shell := mustShellRequest(t, fmt.Sprintf("printf 'raw transcript'; printf 'final summary' > %q", capturePath))

	got, err := runner.run(context.Background(), commandRequest{
		command:           shell.command,
		args:              shell.args,
		cwd:               root,
		effect:            effectExecWrite,
		outputCapturePath: capturePath,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	var payload struct {
		Stdout string `json:"stdout"`
	}
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.Stdout != "final summary" {
		t.Fatalf("expected captured summary output, got %q", payload.Stdout)
	}
}

func TestCoderExecTool_InvokeHonorsExplicitSkipGitRepoCheckFalse(t *testing.T) {
	root := t.TempDir()
	runner := &recordingCommandRunner{
		res: model.ToolResult{Output: `{"command":"codex exec --sandbox workspace-write -C /tmp/root prompt","cwd":"/tmp/root","stdout":"","stderr":"","exit_code":0,"timed_out":false,"truncated":false,"effect":"exec_write"}`},
	}
	tool := newCoderExecTool([]coderBackend{
		newCodexCoderBackend("codex"),
	}, runner)

	if _, err := tool.Invoke(withToolContext(context.Background(), root), model.ToolCall{
		ID:       "call-coder",
		ToolName: tool.Name(),
		InputJSON: []byte(`{
			"backend":"codex",
			"prompt":"prompt",
			"skip_git_repo_check":false
		}`),
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	for _, arg := range runner.req.args {
		if arg == "--skip-git-repo-check" {
			t.Fatalf("did not expect --skip-git-repo-check in args: %v", runner.req.args)
		}
	}
}

func TestCoderExecTool_InvokeUsesElevatedAuthorityForHostAccess(t *testing.T) {
	root := t.TempDir()
	elevatedCWD := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(elevatedCWD, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	runner := &recordingCommandRunner{
		res: model.ToolResult{Output: `{"command":"codex exec --sandbox danger-full-access --skip-git-repo-check -C /tmp/outside \"Build it\"","cwd":"/tmp/outside","stdout":"ok","stderr":"","exit_code":0,"timed_out":false,"truncated":false,"effect":"exec_write"}`},
	}
	tool := newCoderExecTool([]coderBackend{
		newCodexCoderBackend("codex"),
	}, runner)

	got, err := tool.Invoke(WithInvocationContext(context.Background(), InvocationContext{
		CWD: root,
		Authority: authority.Envelope{
			HostAccessMode: authority.HostAccessModeElevated,
		},
	}), model.ToolCall{
		ID:       "call-coder",
		ToolName: tool.Name(),
		InputJSON: []byte(fmt.Sprintf(`{
			"backend":"codex",
			"cwd":%q,
			"prompt":"Build it"
		}`, elevatedCWD)),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if runner.req.cwd != elevatedCWD {
		t.Fatalf("expected elevated cwd %q, got %q", elevatedCWD, runner.req.cwd)
	}
	if len(runner.req.args) < 4 {
		t.Fatalf("expected sandbox args, got %v", runner.req.args)
	}
	for i, want := range []string{"exec", "--sandbox", "danger-full-access", "--color"} {
		if runner.req.args[i] != want {
			t.Fatalf("expected danger-full-access sandbox args, got %v", runner.req.args)
		}
	}
	var payload struct {
		Backend string `json:"backend"`
	}
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.Backend != "codex" {
		t.Fatalf("expected codex backend, got %q", payload.Backend)
	}
}

func TestCoderExecTool_InvokeBuildsClaudeCodeCommand(t *testing.T) {
	root := t.TempDir()
	runner := &recordingCommandRunner{
		res: model.ToolResult{Output: `{"command":"claude --print --output-format json --permission-mode acceptEdits \"Build it\"","cwd":"/tmp/project","stdout":"ok","stderr":"","exit_code":0,"timed_out":false,"truncated":false,"effect":"exec_write"}`},
	}
	tool := newCoderExecTool([]coderBackend{
		newClaudeCodeBackend("claude"),
	}, runner)

	got, err := tool.Invoke(withToolContext(context.Background(), root), model.ToolCall{
		ID:       "call-coder",
		ToolName: tool.Name(),
		InputJSON: []byte(`{
			"backend":"claude_code",
			"cwd":"project",
			"prompt":"Build it"
		}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	wantCWD, _, err := resolveScopedPath(root, "project")
	if err != nil {
		t.Fatalf("resolveScopedPath: %v", err)
	}
	if runner.req.command != "claude" {
		t.Fatalf("expected claude command, got %q", runner.req.command)
	}
	if runner.req.cwd != wantCWD {
		t.Fatalf("expected cwd %q, got %q", wantCWD, runner.req.cwd)
	}
	wantArgs := []string{
		"--print",
		"--output-format", "json",
		"--permission-mode", "acceptEdits",
	}
	if len(runner.req.args) != len(wantArgs)+1 {
		t.Fatalf("expected %d args, got %d (%v)", len(wantArgs), len(runner.req.args), runner.req.args)
	}
	for i := range wantArgs {
		if runner.req.args[i] != wantArgs[i] {
			t.Fatalf("arg %d: want %q, got %q", i, wantArgs[i], runner.req.args[i])
		}
	}
	wrappedPrompt := runner.req.args[len(runner.req.args)-1]
	for _, want := range []string{
		"You are a non-interactive coding subagent running inside GistClaw.",
		"You were dispatched as a subagent to execute a specific task.",
		"Do not ask the user questions.",
		"Build it",
	} {
		if !strings.Contains(wrappedPrompt, want) {
			t.Fatalf("expected wrapped claude prompt to contain %q, got %q", want, wrappedPrompt)
		}
	}

	var payload struct {
		Backend string `json:"backend"`
		Stdout  string `json:"stdout"`
	}
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.Backend != "claude_code" {
		t.Fatalf("expected claude_code backend, got %q", payload.Backend)
	}
	if payload.Stdout != "ok" {
		t.Fatalf("expected stdout passthrough, got %q", payload.Stdout)
	}
}

func TestCoderExecTool_InvokeRejectsUnknownBackend(t *testing.T) {
	tool := newCoderExecTool([]coderBackend{
		newCodexCoderBackend("codex"),
	}, &recordingCommandRunner{})

	_, err := tool.Invoke(withToolContext(context.Background(), t.TempDir()), model.ToolCall{
		ID:        "call-coder",
		ToolName:  tool.Name(),
		InputJSON: []byte(`{"backend":"claude_code","prompt":"Build it"}`),
	})
	if err == nil || err.Error() != "coder_exec: backend \"claude_code\" is not available" {
		t.Fatalf("expected unknown backend error, got %v", err)
	}
}
