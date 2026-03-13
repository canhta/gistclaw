package memory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/memory"
)

// Note: This implementation uses direct os.ReadFile calls instead of
// infra.SOULLoader's mtime-caching. This is a deliberate simplification —
// the caching optimization is omitted. For a low-frequency Telegram bot this
// is acceptable; add SOULLoader wrapping later if file I/O becomes a bottleneck.

func newTestEngine(t *testing.T) (memory.Engine, string) {
	t.Helper()
	dir := t.TempDir()
	memPath := filepath.Join(dir, "MEMORY.md")
	notesDir := filepath.Join(dir, "notes")
	eng := memory.NewEngine("", memPath, notesDir)
	return eng, dir
}

func TestLoadContext_EmptyReturnsEmpty(t *testing.T) {
	eng, _ := newTestEngine(t)
	got := eng.LoadContext()
	if got != "" {
		t.Errorf("LoadContext on empty: got %q, want empty", got)
	}
}

func TestAppendFact_CreatesFile(t *testing.T) {
	eng, dir := newTestEngine(t)
	if err := eng.AppendFact("user prefers short answers"); err != nil {
		t.Fatalf("AppendFact: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read MEMORY.md: %v", err)
	}
	if !strings.Contains(string(content), "user prefers short answers") {
		t.Errorf("MEMORY.md does not contain appended fact: %s", content)
	}
}

func TestAppendNote_CreatesDailyFile(t *testing.T) {
	eng, dir := newTestEngine(t)
	if err := eng.AppendNote("discussed refactor"); err != nil {
		t.Fatalf("AppendNote: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "notes"))
	if err != nil {
		t.Fatalf("read notes dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no notes files created")
	}
	content, _ := os.ReadFile(filepath.Join(dir, "notes", entries[0].Name()))
	if !strings.Contains(string(content), "discussed refactor") {
		t.Errorf("notes file does not contain appended note: %s", content)
	}
}

func TestRewrite_ReplacesContent(t *testing.T) {
	eng, dir := newTestEngine(t)
	_ = eng.AppendFact("old fact")
	if err := eng.Rewrite("new curated content"); err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	if strings.Contains(string(content), "old fact") {
		t.Error("Rewrite should have removed old content")
	}
	if !strings.Contains(string(content), "new curated content") {
		t.Errorf("Rewrite: expected new content, got %s", content)
	}
}

func TestLoadContext_IncludesMemory(t *testing.T) {
	eng, _ := newTestEngine(t)
	_ = eng.AppendFact("key fact")
	ctx := eng.LoadContext()
	if !strings.Contains(ctx, "key fact") {
		t.Errorf("LoadContext should contain memory: %s", ctx)
	}
	if !strings.Contains(ctx, "# Memory") {
		t.Errorf("LoadContext should contain '# Memory' header: %s", ctx)
	}
}

func TestLoadContext_IncludesSOUL(t *testing.T) {
	dir := t.TempDir()
	soulPath := filepath.Join(dir, "SOUL.md")
	_ = os.WriteFile(soulPath, []byte("you are a helpful assistant"), 0o644)
	memPath := filepath.Join(dir, "MEMORY.md")
	eng := memory.NewEngine(soulPath, memPath, filepath.Join(dir, "notes"))
	ctx := eng.LoadContext()
	if !strings.Contains(ctx, "you are a helpful assistant") {
		t.Errorf("LoadContext should include SOUL content: %s", ctx)
	}
}

func TestLoadContext_NotesCappedAt8000Bytes(t *testing.T) {
	eng, _ := newTestEngine(t)
	// Write a note that exceeds 8000 bytes.
	bigNote := strings.Repeat("x", 9000)
	_ = eng.AppendNote(bigNote)
	ctx := eng.LoadContext()
	// Find the notes section and verify it is capped.
	notesStart := strings.Index(ctx, "# Today's Notes")
	if notesStart < 0 {
		t.Fatal("no notes section in LoadContext")
	}
	notesContent := ctx[notesStart:]
	if len(notesContent) > 8200 { // 8000 + small header overhead
		t.Errorf("notes section too large: %d bytes", len(notesContent))
	}
}

func TestMemoryPath_ReturnsPath(t *testing.T) {
	dir := t.TempDir()
	memPath := filepath.Join(dir, "MEMORY.md")
	eng := memory.NewEngine("", memPath, filepath.Join(dir, "notes"))
	if eng.MemoryPath() != memPath {
		t.Errorf("MemoryPath: got %q, want %q", eng.MemoryPath(), memPath)
	}
}
