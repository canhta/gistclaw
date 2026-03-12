// internal/infra/soul.go
package infra

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// SOULLoader loads SOUL.md from disk with mtime-based caching.
// It reloads the file only when the modification time changes.
// Safe for concurrent use.
type SOULLoader struct {
	mu      sync.Mutex
	path    string
	content string
	mtime   time.Time
}

// NewSOULLoader creates a SOULLoader for the given file path.
func NewSOULLoader(path string) *SOULLoader {
	return &SOULLoader{path: path}
}

// Load returns the current content of SOUL.md.
// On first call, or when the file has been modified, it reads from disk.
// Returns an error if the file cannot be read.
func (l *SOULLoader) Load() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	info, err := os.Stat(l.path)
	if err != nil {
		return "", fmt.Errorf("soul: stat %q: %w", l.path, err)
	}

	// Use !l.mtime.IsZero() (not l.content != "") so an empty SOUL.md is cached correctly.
	if !l.mtime.IsZero() && info.ModTime().Equal(l.mtime) {
		return l.content, nil
	}

	data, err := os.ReadFile(l.path)
	if err != nil {
		return "", fmt.Errorf("soul: read %q: %w", l.path, err)
	}

	l.mtime = info.ModTime()
	l.content = string(data)
	return l.content, nil
}
