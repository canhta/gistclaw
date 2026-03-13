package agent

import "fmt"

// Kind identifies which agent a scheduler job or task targets.
type Kind int

const (
	// KindUnknown is a sentinel for unrecognised or uninitialised values.
	// Explicit -1 ensures a zero-value Kind is not silently treated as KindOpenCode.
	KindUnknown    Kind = -1
	KindOpenCode   Kind = 0
	KindClaudeCode Kind = 1
	KindChat       Kind = 2
	KindGateway    Kind = 3
)

// String returns the SQLite-compatible string representation.
func (k Kind) String() string {
	switch k {
	case KindOpenCode:
		return "opencode"
	case KindClaudeCode:
		return "claudecode"
	case KindChat:
		return "chat"
	case KindGateway:
		return "gateway"
	default:
		return fmt.Sprintf("unknown(%d)", int(k))
	}
}

// KindFromString parses the string representation stored in SQLite.
// Returns KindUnknown and an error for any unrecognised value.
func KindFromString(s string) (Kind, error) {
	switch s {
	case "opencode":
		return KindOpenCode, nil
	case "claudecode":
		return KindClaudeCode, nil
	case "chat":
		return KindChat, nil
	case "gateway":
		return KindGateway, nil
	default:
		return KindUnknown, fmt.Errorf("agent: unknown kind %q", s)
	}
}
