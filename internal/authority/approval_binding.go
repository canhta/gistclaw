package authority

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
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

func sortedCopy(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
