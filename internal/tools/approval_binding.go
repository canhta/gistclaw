package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/authority"
	"github.com/canhta/gistclaw/internal/model"
)

func BuildApprovalBindingJSON(toolName, root string, spec model.ToolSpec, inputJSON []byte, env authority.Envelope) ([]byte, error) {
	binding, err := buildApprovalBinding(toolName, root, spec, inputJSON, env)
	if err != nil {
		return nil, err
	}
	return json.Marshal(binding)
}

func buildApprovalBinding(toolName, root string, spec model.ToolSpec, inputJSON []byte, env authority.Envelope) (authority.Binding, error) {
	effect := classifyToolCall(spec, inputJSON)
	binding := authority.Binding{
		ToolName: toolName,
		CWD:      root,
		Mutating: !isReadEffect(effect),
	}

	switch toolName {
	case "shell_exec":
		var input struct {
			Command string `json:"command"`
			CWD     string `json:"cwd"`
		}
		if err := json.Unmarshal(inputJSON, &input); err != nil {
			return authority.Binding{}, fmt.Errorf("decode shell_exec binding input: %w", err)
		}
		cwd, err := resolveToolCWD(root, input.CWD, env)
		if err != nil {
			return authority.Binding{}, fmt.Errorf("resolve shell_exec cwd: %w", err)
		}
		binding.CWD = cwd
		binding.Argv = shellFields(input.Command)
	case "coder_exec":
		var input coderExecInput
		if err := json.Unmarshal(inputJSON, &input); err != nil {
			return authority.Binding{}, fmt.Errorf("decode coder_exec binding input: %w", err)
		}
		input.Authority = env
		cwd, err := resolveToolCWD(root, input.CWD, env)
		if err != nil {
			return authority.Binding{}, fmt.Errorf("resolve coder_exec cwd: %w", err)
		}
		binding.CWD = cwd
		argv, err := coderApprovalArgv(input, cwd)
		if err != nil {
			return authority.Binding{}, fmt.Errorf("build coder_exec approval argv: %w", err)
		}
		if len(argv) > 0 {
			binding.Argv = argv
		}
	case "write_new_file", "delete_path":
		var input struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(inputJSON, &input); err != nil {
			return authority.Binding{}, fmt.Errorf("decode %s binding input: %w", toolName, err)
		}
		absPath, _, err := resolveToolPath(root, input.Path, env)
		if err != nil {
			return authority.Binding{}, fmt.Errorf("resolve %s path: %w", toolName, err)
		}
		binding.Operands = []string{absPath}
	case "move_path":
		var input struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := json.Unmarshal(inputJSON, &input); err != nil {
			return authority.Binding{}, fmt.Errorf("decode move_path binding input: %w", err)
		}
		fromAbs, _, err := resolveToolPath(root, input.From, env)
		if err != nil {
			return authority.Binding{}, fmt.Errorf("resolve move_path source: %w", err)
		}
		toAbs, _, err := resolveToolPath(root, input.To, env)
		if err != nil {
			return authority.Binding{}, fmt.Errorf("resolve move_path destination: %w", err)
		}
		binding.Operands = []string{fromAbs, toAbs}
	case "apply_patch", "run_tests", "run_build", "git_status", "git_diff", "git_show", "git_log":
		var input struct {
			CWD string `json:"cwd"`
		}
		if err := json.Unmarshal(inputJSON, &input); err != nil {
			return authority.Binding{}, fmt.Errorf("decode %s binding input: %w", toolName, err)
		}
		cwd, err := resolveToolCWD(root, input.CWD, env)
		if err != nil {
			return authority.Binding{}, fmt.Errorf("resolve %s cwd: %w", toolName, err)
		}
		binding.CWD = cwd
	}

	if isReadEffect(effect) {
		binding.ReadRoots = compactBindingRoots(binding.CWD)
	} else {
		binding.WriteRoots = compactBindingRoots(binding.CWD)
	}
	return binding, nil
}

func coderApprovalArgv(input coderExecInput, cwd string) ([]string, error) {
	switch strings.TrimSpace(input.Backend) {
	case "codex":
		return newCodexCoderBackend("codex").ApprovalArgv(input, cwd)
	case "claude_code":
		return newClaudeCodeBackend("claude").ApprovalArgv(input, cwd)
	case "":
		return nil, nil
	default:
		return nil, fmt.Errorf("backend %q is not available", input.Backend)
	}
}

func compactBindingRoots(values ...string) []string {
	roots := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		roots = append(roots, value)
	}
	if len(roots) == 0 {
		return nil
	}
	return roots
}

func shellFields(command string) []string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return nil
	}
	return fields
}
