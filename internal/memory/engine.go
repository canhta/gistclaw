// internal/memory/engine.go
package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// notesCapBytes is the maximum number of bytes retained from today's notes in LoadContext.
// Newer entries are preserved (tail kept) when the cap is exceeded.
const notesCapBytes = 8000

// Engine manages the persistent memory context for the gateway.
type Engine interface {
	// LoadContext returns the full system prompt injection.
	// Parts (each omitted if empty/missing):
	//   1. SOUL file content
	//   2. "# Memory\n\n" + MEMORY.md content
	//   3. "# Today's Notes\n\n" + today's notes (capped at 8000 bytes, tail kept)
	// Parts are joined with "\n\n" (same as the existing buildSystemPrompt).
	LoadContext() string

	// AppendFact appends a timestamped entry to MEMORY.md.
	AppendFact(content string) error

	// AppendNote appends a timestamped entry to notes/YYYY-MM-DD.md.
	AppendNote(content string) error

	// Rewrite replaces the full MEMORY.md content.
	Rewrite(content string) error

	// TodayNotes returns the content of today's notes file.
	TodayNotes() (string, error)

	// MemoryPath returns the path to MEMORY.md.
	MemoryPath() string
}

type engine struct {
	soulPath   string // may be empty
	memoryPath string
	notesDir   string
}

// NewEngine constructs an Engine.
//
//	soulPath:   path to SOUL.md; empty string disables SOUL loading.
//	memoryPath: path to MEMORY.md.
//	notesDir:   directory for date-partitioned notes; created on first AppendNote.
func NewEngine(soulPath, memoryPath, notesDir string) Engine {
	return &engine{
		soulPath:   soulPath,
		memoryPath: memoryPath,
		notesDir:   notesDir,
	}
}

func (e *engine) MemoryPath() string { return e.memoryPath }

func (e *engine) LoadContext() string {
	var parts []string

	// 1. SOUL content.
	if e.soulPath != "" {
		if content, err := os.ReadFile(e.soulPath); err == nil && len(content) > 0 {
			parts = append(parts, strings.TrimRight(string(content), "\n"))
		}
	}

	// 2. MEMORY.md facts.
	if content, err := os.ReadFile(e.memoryPath); err == nil && len(content) > 0 {
		parts = append(parts, "# Memory\n\n"+strings.TrimRight(string(content), "\n"))
	}

	// 3. Today's notes, capped at notesCapBytes (tail kept — newest entries preserved).
	if notes, err := e.TodayNotes(); err == nil && notes != "" {
		capped := tailBytes(notes, notesCapBytes)
		parts = append(parts, "# Today's Notes\n\n"+strings.TrimRight(capped, "\n"))
	}

	return strings.Join(parts, "\n\n")
}

func (e *engine) AppendFact(content string) error {
	line := fmt.Sprintf("[%s] %s\n", time.Now().Format("2006-01-02 15:04"), content)
	return appendToFile(e.memoryPath, line)
}

func (e *engine) AppendNote(content string) error {
	if err := os.MkdirAll(e.notesDir, 0o755); err != nil {
		return fmt.Errorf("memory: create notes dir: %w", err)
	}
	file := filepath.Join(e.notesDir, time.Now().Format("2006-01-02")+".md")
	line := fmt.Sprintf("[%s] %s\n", time.Now().Format("15:04"), content)
	return appendToFile(file, line)
}

func (e *engine) Rewrite(content string) error {
	if err := os.MkdirAll(filepath.Dir(e.memoryPath), 0o755); err != nil {
		return fmt.Errorf("memory: create dir: %w", err)
	}
	if err := os.WriteFile(e.memoryPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("memory: write %s: %w", e.memoryPath, err)
	}
	return nil
}

func (e *engine) TodayNotes() (string, error) {
	file := filepath.Join(e.notesDir, time.Now().Format("2006-01-02")+".md")
	content, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("memory: read today notes: %w", err)
	}
	return string(content), nil
}

// appendToFile appends data to path, creating the file and parent dirs if needed.
func appendToFile(path, data string) (retErr error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("memory: create dir for %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("memory: open %s: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && retErr == nil {
			retErr = fmt.Errorf("memory: close %s: %w", path, cerr)
		}
	}()
	if _, err := f.WriteString(data); err != nil {
		return fmt.Errorf("memory: write %s: %w", path, err)
	}
	return nil
}

// tailBytes returns the last n bytes of s, aligned to the start of a line if possible.
func tailBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	tail := s[len(s)-n:]
	// Align to next newline so we don't cut mid-line.
	if idx := strings.Index(tail, "\n"); idx >= 0 {
		return tail[idx+1:]
	}
	return tail
}
