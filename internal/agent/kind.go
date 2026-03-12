package agent

import "fmt"

// Kind identifies which agent a scheduler job or task targets.
type Kind int

const (
	KindOpenCode Kind = iota
	KindClaudeCode
	KindChat
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
	default:
		return fmt.Sprintf("unknown(%d)", int(k))
	}
}

// KindFromString parses the string representation stored in SQLite.
// Returns an error for any unrecognised value.
func KindFromString(s string) (Kind, error) {
	switch s {
	case "opencode":
		return KindOpenCode, nil
	case "claudecode":
		return KindClaudeCode, nil
	case "chat":
		return KindChat, nil
	default:
		return 0, fmt.Errorf("agent: unknown kind %q", s)
	}
}
