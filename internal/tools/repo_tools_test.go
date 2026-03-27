package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/authority"
	"github.com/canhta/gistclaw/internal/model"
)

func TestResolveScopedPath_RejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, _, err := resolveScopedPath(root, "link/secret.txt")
	if err == nil {
		t.Fatal("expected symlink escape to fail")
	}
	if err != ErrEscapeAttempt {
		t.Fatalf("expected ErrEscapeAttempt, got %v", err)
	}
}

func TestResolveToolPath_AllowsAbsolutePathInElevatedMode(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "notes.txt")

	absPath, displayPath, err := resolveToolPath(root, outside, authority.Envelope{
		HostAccessMode: authority.HostAccessModeElevated,
	})
	if err != nil {
		t.Fatalf("resolveToolPath: %v", err)
	}
	if absPath != outside {
		t.Fatalf("expected abs path %q, got %q", outside, absPath)
	}
	if displayPath != filepath.ToSlash(outside) {
		t.Fatalf("expected display path %q, got %q", filepath.ToSlash(outside), displayPath)
	}
}

func TestBuildRegistry_RegistersRepoPowerTools(t *testing.T) {
	reg, closer, err := BuildRegistry(context.Background(), BuildOptions{})
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	if closer != nil {
		defer closer.Close()
	}

	for _, name := range []string{
		"list_dir",
		"read_file",
		"grep_search",
		"apply_patch",
		"write_new_file",
		"delete_path",
		"move_path",
		"git_status",
		"git_diff",
		"git_show",
		"git_log",
		"shell_exec",
		"coder_exec",
		"run_tests",
		"run_build",
	} {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("expected %q to be registered", name)
		}
	}
}

func TestListDir_ReturnsWorkspaceRelativeEntries(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "README.md", "# repo\n")
	writeWorkspaceFile(t, root, "cmd/app.go", "package main\n")

	tool := NewListDirTool()
	got, err := tool.Invoke(withToolContext(context.Background(), root), model.ToolCall{
		ID:        "call-list",
		ToolName:  tool.Name(),
		InputJSON: []byte(`{"path":"."}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var payload struct {
		Path    string `json:"path"`
		Entries []struct {
			Path string `json:"path"`
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.Path != "." {
		t.Fatalf("expected path '.', got %q", payload.Path)
	}
	if len(payload.Entries) == 0 {
		t.Fatal("expected entries")
	}
	if !containsEntry(payload.Entries, "README.md", "file") {
		t.Fatalf("expected README.md file entry, got %+v", payload.Entries)
	}
	if !containsEntry(payload.Entries, "cmd", "dir") {
		t.Fatalf("expected cmd dir entry, got %+v", payload.Entries)
	}
}

func TestReadFile_ReturnsRequestedLineRange(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "notes.txt", "one\ntwo\nthree\nfour\n")

	tool := NewReadFileTool(1024)
	got, err := tool.Invoke(withToolContext(context.Background(), root), model.ToolCall{
		ID:        "call-read",
		ToolName:  tool.Name(),
		InputJSON: []byte(`{"path":"notes.txt","start_line":2,"end_line":3}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var payload struct {
		Path      string `json:"path"`
		Content   string `json:"content"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.Path != "notes.txt" {
		t.Fatalf("unexpected path %q", payload.Path)
	}
	if payload.Content != "two\nthree\n" {
		t.Fatalf("unexpected content %q", payload.Content)
	}
	if payload.StartLine != 2 || payload.EndLine != 3 {
		t.Fatalf("unexpected line range %+v", payload)
	}
}

func TestReadFile_AllowsElevatedAbsolutePath(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tool := NewReadFileTool(1024)
	got, err := tool.Invoke(WithInvocationContext(context.Background(), InvocationContext{
		CWD: root,
		Authority: authority.Envelope{
			HostAccessMode: authority.HostAccessModeElevated,
		},
	}), model.ToolCall{
		ID:        "call-read-absolute",
		ToolName:  tool.Name(),
		InputJSON: []byte(`{"path":` + quoteJSONString(outside) + `}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var payload struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.Path != filepath.ToSlash(outside) {
		t.Fatalf("unexpected path %q", payload.Path)
	}
	if payload.Content != "outside\n" {
		t.Fatalf("unexpected content %q", payload.Content)
	}
}

func TestGrepSearch_FindsMatchesUnderWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "README.md", "alpha\nneedle here\nomega\n")
	writeWorkspaceFile(t, root, "docs/guide.md", "needle there too\n")

	tool := NewGrepSearchTool(64 << 10)
	got, err := tool.Invoke(withToolContext(context.Background(), root), model.ToolCall{
		ID:        "call-grep",
		ToolName:  tool.Name(),
		InputJSON: []byte(`{"query":"needle","path":"."}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var payload struct {
		Query   string `json:"query"`
		Matches []struct {
			Path       string `json:"path"`
			LineNumber int    `json:"line_number"`
			Line       string `json:"line"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.Query != "needle" {
		t.Fatalf("unexpected query %q", payload.Query)
	}
	if len(payload.Matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(payload.Matches))
	}
}

func TestWriteNewFile_CreatesMissingFile(t *testing.T) {
	root := t.TempDir()
	tool := NewWriteNewFileTool()

	got, err := tool.Invoke(withToolContext(context.Background(), root), model.ToolCall{
		ID:        "call-create",
		ToolName:  tool.Name(),
		InputJSON: []byte(`{"path":"docs/new.txt","content":"hello\n"}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if content := string(mustReadFile(t, filepath.Join(root, "docs/new.txt"))); content != "hello\n" {
		t.Fatalf("unexpected file content %q", content)
	}

	var payload struct {
		Path    string `json:"path"`
		Created bool   `json:"created"`
	}
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !payload.Created || payload.Path != "docs/new.txt" {
		t.Fatalf("unexpected payload %+v", payload)
	}
}

func TestMoveAndDeletePath_UpdateWorkspace(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "tmp/a.txt", "hello\n")

	moveTool := NewMovePathTool()
	if _, err := moveTool.Invoke(withToolContext(context.Background(), root), model.ToolCall{
		ID:        "call-move",
		ToolName:  moveTool.Name(),
		InputJSON: []byte(`{"from":"tmp/a.txt","to":"tmp/b.txt"}`),
	}); err != nil {
		t.Fatalf("move Invoke: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "tmp/a.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected source removed, stat err=%v", err)
	}

	deleteTool := NewDeletePathTool()
	if _, err := deleteTool.Invoke(withToolContext(context.Background(), root), model.ToolCall{
		ID:        "call-delete",
		ToolName:  deleteTool.Name(),
		InputJSON: []byte(`{"path":"tmp/b.txt"}`),
	}); err != nil {
		t.Fatalf("delete Invoke: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "tmp/b.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected destination removed, stat err=%v", err)
	}
}

func TestApplyPatch_UpdatesExistingFile(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "README.md", "hello\n")

	tool := NewApplyPatchTool(30)
	patch := strings.Join([]string{
		"diff --git a/README.md b/README.md",
		"index ce01362..2e09960 100644",
		"--- a/README.md",
		"+++ b/README.md",
		"@@ -1 +1 @@",
		"-hello",
		"+world",
		"",
	}, "\n")
	if _, err := tool.Invoke(withToolContext(context.Background(), root), model.ToolCall{
		ID:        "call-patch",
		ToolName:  tool.Name(),
		InputJSON: []byte(`{"patch":` + quoteJSONString(patch) + `}`),
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if content := string(mustReadFile(t, filepath.Join(root, "README.md"))); content != "world\n" {
		t.Fatalf("unexpected patched content %q", content)
	}
}

func TestShellExec_RunsInsideWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	tool := NewShellExecTool(30, 64<<10)

	got, err := tool.Invoke(withToolContext(context.Background(), root), model.ToolCall{
		ID:        "call-shell",
		ToolName:  tool.Name(),
		InputJSON: []byte(`{"command":"pwd"}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var payload struct {
		Command  string `json:"command"`
		CWD      string `json:"cwd"`
		Stdout   string `json:"stdout"`
		ExitCode int    `json:"exit_code"`
		Effect   string `json:"effect"`
	}
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.CWD != root {
		t.Fatalf("expected cwd %q, got %q", root, payload.CWD)
	}
	if strings.TrimSpace(payload.Stdout) != root {
		t.Fatalf("expected stdout to report workspace root, got %q", payload.Stdout)
	}
	if payload.ExitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", payload.ExitCode)
	}
	if payload.Effect != effectExecRead {
		t.Fatalf("expected read effect, got %q", payload.Effect)
	}
}

func TestShellExec_AllowsElevatedAbsoluteCWD(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	tool := NewShellExecTool(30, 64<<10)

	got, err := tool.Invoke(WithInvocationContext(context.Background(), InvocationContext{
		CWD: root,
		Authority: authority.Envelope{
			HostAccessMode: authority.HostAccessModeElevated,
		},
	}), model.ToolCall{
		ID:        "call-shell-absolute",
		ToolName:  tool.Name(),
		InputJSON: []byte(`{"command":"pwd","cwd":` + quoteJSONString(outside) + `}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var payload struct {
		CWD    string `json:"cwd"`
		Stdout string `json:"stdout"`
	}
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.CWD != outside {
		t.Fatalf("expected cwd %q, got %q", outside, payload.CWD)
	}
	if strings.TrimSpace(payload.Stdout) != outside {
		t.Fatalf("expected stdout %q, got %q", outside, payload.Stdout)
	}
}

func TestRunTestsAndBuild_DefaultToGoTooling(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "go.mod", "module example.com/repo\n\ngo 1.24\n")
	writeWorkspaceFile(t, root, "main.go", "package main\n\nfunc main() {}\n")
	writeWorkspaceFile(t, root, "main_test.go", "package main\n\nimport \"testing\"\n\nfunc TestOK(t *testing.T) {}\n")

	testsTool := NewRunTestsTool(60, 64<<10)
	testResult, err := testsTool.Invoke(withToolContext(context.Background(), root), model.ToolCall{
		ID:        "call-tests",
		ToolName:  testsTool.Name(),
		InputJSON: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("run_tests Invoke: %v", err)
	}

	buildTool := NewRunBuildTool(60, 64<<10)
	buildResult, err := buildTool.Invoke(withToolContext(context.Background(), root), model.ToolCall{
		ID:        "call-build",
		ToolName:  buildTool.Name(),
		InputJSON: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("run_build Invoke: %v", err)
	}

	for name, raw := range map[string]string{
		"run_tests": testResult.Output,
		"run_build": buildResult.Output,
	} {
		var payload struct {
			Command  string `json:"command"`
			ExitCode int    `json:"exit_code"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatalf("%s unmarshal output: %v", name, err)
		}
		if payload.ExitCode != 0 {
			t.Fatalf("%s expected zero exit code, got %d", name, payload.ExitCode)
		}
		if payload.Command == "" {
			t.Fatalf("%s expected command to be recorded", name)
		}
	}
}

func TestGitTools_InspectRepository(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "README.md", "hello\n")
	runGit(t, root, "init")
	runGit(t, root, "config", "user.name", "Gist Claw")
	runGit(t, root, "config", "user.email", "gist@example.com")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-m", "init")
	writeWorkspaceFile(t, root, "README.md", "updated\n")

	tests := []struct {
		name  string
		tool  Tool
		want  string
		input string
	}{
		{name: "status", tool: NewGitStatusTool(30, 64<<10), want: "README.md", input: `{}`},
		{name: "diff", tool: NewGitDiffTool(30, 64<<10), want: "-hello", input: `{}`},
		{name: "show", tool: NewGitShowTool(30, 64<<10), want: "init", input: `{"target":"HEAD"}`},
		{name: "log", tool: NewGitLogTool(30, 64<<10), want: "init", input: `{"limit":1}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.tool.Invoke(withToolContext(context.Background(), root), model.ToolCall{
				ID:        "call-" + tc.name,
				ToolName:  tc.tool.Name(),
				InputJSON: []byte(tc.input),
			})
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			var payload struct {
				Stdout string `json:"stdout"`
			}
			if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
				t.Fatalf("unmarshal output: %v", err)
			}
			if !strings.Contains(payload.Stdout, tc.want) {
				t.Fatalf("expected output to contain %q, got %q", tc.want, payload.Stdout)
			}
		})
	}
}

func TestPolicy_DecideCall_HandlesShellEffects(t *testing.T) {
	p := &Policy{Profile: "scoped_write"}
	agent := model.AgentProfile{
		Capabilities: []model.AgentCapability{model.CapScopedWrite},
		ToolProfile:  "scoped_write",
	}

	readDecision := p.DecideCall(agent, model.RunProfile{}, NewShellExecTool(30, 1024).Spec(), []byte(`{"command":"git status"}`))
	if readDecision.Mode != model.DecisionAllow {
		t.Fatalf("expected read-only shell to be allowed, got %s", readDecision.Mode)
	}

	writeDecision := p.DecideCall(agent, model.RunProfile{}, NewShellExecTool(30, 1024).Spec(), []byte(`{"command":"touch created.txt"}`))
	if writeDecision.Mode != model.DecisionAsk {
		t.Fatalf("expected mutating shell to ask, got %s", writeDecision.Mode)
	}
}

func withToolContext(ctx context.Context, root string) context.Context {
	return WithInvocationContext(ctx, InvocationContext{CWD: root})
}

func writeWorkspaceFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	abs := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", abs, err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", abs, err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func containsEntry(entries []struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}, name, kind string) bool {
	for _, entry := range entries {
		if entry.Name == name && entry.Kind == kind {
			return true
		}
	}
	return false
}

func quoteJSONString(s string) string {
	raw, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
