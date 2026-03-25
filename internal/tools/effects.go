package tools

import (
	"encoding/json"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
)

const (
	effectNone      = "none"
	effectRead      = "read"
	effectPatch     = "patch"
	effectCreate    = "create"
	effectDelete    = "delete"
	effectMove      = "move"
	effectExecRead  = "exec_read"
	effectExecWrite = "exec_write"
)

func classifyToolSpec(spec model.ToolSpec) string {
	if spec.SideEffect != "" {
		return spec.SideEffect
	}
	if spec.Risk == model.RiskLow {
		return effectRead
	}
	return effectExecWrite
}

func classifyToolCall(spec model.ToolSpec, inputJSON []byte) string {
	switch spec.Name {
	case "shell_exec":
		var input struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(inputJSON, &input); err != nil {
			return effectExecWrite
		}
		return classifyShellCommand(input.Command)
	default:
		return classifyToolSpec(spec)
	}
}

func classifyShellCommand(command string) string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return effectExecWrite
	}

	switch fields[0] {
	case "cat", "pwd", "ls", "find", "grep", "rg", "head", "tail", "sed", "awk", "git", "go":
		if len(fields) > 1 && fields[0] == "git" {
			switch fields[1] {
			case "status", "diff", "show", "log", "branch", "rev-parse":
				return effectExecRead
			}
		}
		if len(fields) > 1 && fields[0] == "go" {
			switch fields[1] {
			case "test", "build", "list", "vet", "fmt":
				return effectExecRead
			}
		}
		return effectExecRead
	case "touch", "rm", "mv", "cp", "mkdir", "rmdir", "chmod", "chown":
		return effectExecWrite
	default:
		return effectExecWrite
	}
}

func isReadEffect(effect string) bool {
	return effect == effectNone || effect == effectRead || effect == effectExecRead
}
