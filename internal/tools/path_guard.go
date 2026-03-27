package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/canhta/gistclaw/internal/authority"
)

func resolveScopedPath(root, rawPath string) (string, string, error) {
	if root == "" {
		return "", "", ErrCWDRequired
	}
	if strings.ContainsRune(rawPath, 0) {
		return "", "", ErrEscapeAttempt
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", fmt.Errorf("tools: abs cwd: %w", err)
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", "", fmt.Errorf("tools: eval cwd: %w", err)
	}
	realRoot = filepath.Clean(realRoot)

	candidate := strings.TrimSpace(rawPath)
	if candidate == "" || candidate == "." {
		return realRoot, ".", nil
	}

	var joined string
	if filepath.IsAbs(candidate) {
		joined = filepath.Clean(candidate)
	} else {
		joined = filepath.Clean(filepath.Join(realRoot, candidate))
	}
	if joined != realRoot && !strings.HasPrefix(joined, realRoot+string(filepath.Separator)) {
		return "", "", ErrEscapeAttempt
	}
	if err := ensureNoSymlinkEscape(realRoot, joined); err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(realRoot, joined)
	if err != nil {
		return "", "", fmt.Errorf("tools: relative path: %w", err)
	}
	return joined, rel, nil
}

func resolveToolPath(root, rawPath string, env authority.Envelope) (string, string, error) {
	env = authority.NormalizeEnvelope(env)
	candidate := strings.TrimSpace(rawPath)
	if filepath.IsAbs(candidate) && env.HostAccessMode == authority.HostAccessModeElevated {
		if root == "" {
			return "", "", ErrCWDRequired
		}
		if strings.ContainsRune(candidate, 0) {
			return "", "", ErrEscapeAttempt
		}
		cleaned := filepath.Clean(candidate)
		return cleaned, filepath.ToSlash(cleaned), nil
	}
	return resolveScopedPath(root, rawPath)
}

func ensureNoSymlinkEscape(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("tools: relative target path: %w", err)
	}
	if rel == "." {
		return nil
	}

	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("tools: stat %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}

		resolved, err := filepath.EvalSymlinks(current)
		if err != nil {
			return fmt.Errorf("tools: eval symlink %s: %w", current, err)
		}
		resolved = filepath.Clean(resolved)
		if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
			return ErrEscapeAttempt
		}
		current = resolved
	}

	return nil
}
