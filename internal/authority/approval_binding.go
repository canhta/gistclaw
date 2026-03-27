package authority

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

type Binding struct {
	ToolName   string   `json:"tool_name"`
	Argv       []string `json:"argv,omitempty"`
	CWD        string   `json:"cwd,omitempty"`
	ReadRoots  []string `json:"read_roots,omitempty"`
	WriteRoots []string `json:"write_roots,omitempty"`
	Mutating   bool     `json:"mutating,omitempty"`
	Network    bool     `json:"network,omitempty"`
	Operands   []string `json:"operands,omitempty"`
}

func (b Binding) Fingerprint() string {
	normalized := b
	normalized.ReadRoots = sortedCopy(normalized.ReadRoots)
	normalized.WriteRoots = sortedCopy(normalized.WriteRoots)
	normalized.Operands = sortedCopy(normalized.Operands)

	payload, _ := json.Marshal(normalized)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func (b Binding) Summary() string {
	for _, operand := range b.Operands {
		if strings.TrimSpace(operand) != "" {
			return strings.TrimSpace(operand)
		}
	}
	if preview := bindingArgvPreview(b.Argv); preview != "" {
		if cwd := strings.TrimSpace(b.CWD); cwd != "" {
			return preview + " @ " + cwd
		}
		return preview
	}
	if strings.TrimSpace(b.CWD) != "" {
		return strings.TrimSpace(b.CWD)
	}
	for _, root := range b.WriteRoots {
		if strings.TrimSpace(root) != "" {
			return strings.TrimSpace(root)
		}
	}
	for _, root := range b.ReadRoots {
		if strings.TrimSpace(root) != "" {
			return strings.TrimSpace(root)
		}
	}
	return ""
}

func BindingSummaryJSON(raw []byte) string {
	var binding Binding
	if err := json.Unmarshal(raw, &binding); err != nil {
		return ""
	}
	return binding.Summary()
}

func sortedCopy(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func bindingArgvPreview(argv []string) string {
	trimmed := make([]string, 0, len(argv))
	for _, value := range argv {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		trimmed = append(trimmed, value)
		if len(trimmed) == 2 {
			break
		}
	}
	if len(trimmed) == 0 {
		return ""
	}
	for i, value := range trimmed {
		trimmed[i] = sanitizeBindingPreviewToken(value)
	}
	return strings.Join(trimmed, " ")
}

func sanitizeBindingPreviewToken(value string) string {
	if strings.ContainsAny(value, " \t\r\n") {
		return strconv.Quote(value)
	}
	return value
}
