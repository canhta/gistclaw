package tools

import (
	"fmt"
	"strings"
)

var ErrToolDenied = fmt.Errorf("tools: tool denied by policy")
var ErrUnsafeArgs = fmt.Errorf("tools: unsafe shell arguments")

func validateShellArgs(args string) error {
	switch {
	case strings.ContainsRune(args, 0):
		return fmt.Errorf("%w: null byte", ErrUnsafeArgs)
	case strings.Contains(args, ";"):
		return fmt.Errorf("%w: semicolon", ErrUnsafeArgs)
	case strings.Contains(args, "|"):
		return fmt.Errorf("%w: pipe", ErrUnsafeArgs)
	case strings.Contains(args, "../"):
		return fmt.Errorf("%w: path traversal", ErrUnsafeArgs)
	default:
		return nil
	}
}
