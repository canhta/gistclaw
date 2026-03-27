package web

import (
	"encoding/json"
	"strings"

	"github.com/canhta/gistclaw/internal/authority"
)

func approvalDisplayTarget(bindingJSON []byte) string {
	var binding authority.Binding
	if err := json.Unmarshal(bindingJSON, &binding); err != nil {
		return ""
	}
	for _, operand := range binding.Operands {
		if strings.TrimSpace(operand) != "" {
			return strings.TrimSpace(operand)
		}
	}
	if strings.TrimSpace(binding.CWD) != "" {
		return strings.TrimSpace(binding.CWD)
	}
	for _, root := range binding.WriteRoots {
		if strings.TrimSpace(root) != "" {
			return strings.TrimSpace(root)
		}
	}
	for _, root := range binding.ReadRoots {
		if strings.TrimSpace(root) != "" {
			return strings.TrimSpace(root)
		}
	}
	return ""
}
