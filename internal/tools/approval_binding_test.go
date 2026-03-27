package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/canhta/gistclaw/internal/authority"
)

func TestBuildApprovalBindingJSON_WriteNewFileResolvesConcreteOperand(t *testing.T) {
	root := t.TempDir()

	got, err := BuildApprovalBindingJSON(
		"write_new_file",
		root,
		NewWriteNewFileTool().Spec(),
		[]byte(`{"path":"docs/new.txt","content":"hello"}`),
		authority.Envelope{},
	)
	if err != nil {
		t.Fatalf("BuildApprovalBindingJSON: %v", err)
	}

	var binding authority.Binding
	if err := json.Unmarshal(got, &binding); err != nil {
		t.Fatalf("unmarshal binding: %v", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	wantOperand := filepath.Join(resolvedRoot, "docs", "new.txt")
	if binding.ToolName != "write_new_file" || !binding.Mutating {
		t.Fatalf("unexpected binding %+v", binding)
	}
	if binding.CWD != root {
		t.Fatalf("expected cwd %q, got %+v", root, binding)
	}
	if len(binding.Operands) != 1 || binding.Operands[0] != wantOperand {
		t.Fatalf("expected operand %q, got %+v", wantOperand, binding)
	}
	if len(binding.WriteRoots) != 1 || binding.WriteRoots[0] != root {
		t.Fatalf("expected write root %q, got %+v", root, binding)
	}
}

func TestBuildApprovalBindingJSON_ShellExecUsesResolvedCWDAndApproxArgv(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "repo")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	got, err := BuildApprovalBindingJSON(
		"shell_exec",
		root,
		NewShellExecTool(30, 1024).Spec(),
		[]byte(`{"command":"go test ./...","cwd":"repo"}`),
		authority.Envelope{},
	)
	if err != nil {
		t.Fatalf("BuildApprovalBindingJSON: %v", err)
	}

	var binding authority.Binding
	if err := json.Unmarshal(got, &binding); err != nil {
		t.Fatalf("unmarshal binding: %v", err)
	}
	wantCWD, _, err := resolveToolPath(root, "repo", authority.Envelope{})
	if err != nil {
		t.Fatalf("resolveToolPath: %v", err)
	}
	if binding.CWD != wantCWD {
		t.Fatalf("expected cwd %q, got %+v", wantCWD, binding)
	}
	if binding.Mutating {
		t.Fatalf("expected read-only shell binding, got %+v", binding)
	}
	if len(binding.ReadRoots) != 1 || binding.ReadRoots[0] != wantCWD {
		t.Fatalf("expected read root %q, got %+v", wantCWD, binding)
	}
	wantArgv := []string{"go", "test", "./..."}
	if len(binding.Argv) != len(wantArgv) {
		t.Fatalf("expected argv %v, got %+v", wantArgv, binding)
	}
	for i, want := range wantArgv {
		if binding.Argv[i] != want {
			t.Fatalf("expected argv %v, got %+v", wantArgv, binding)
		}
	}
}
