package tools_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	mempkg "github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/tools"
)

// --- mock memory.Engine ---

type mockMemEngine struct {
	memPath    string
	appendErr  error
	rewriteErr error
	noted      []string
	facts      []string
}

func (m *mockMemEngine) AppendFact(content string) error {
	if m.appendErr != nil {
		return m.appendErr
	}
	m.facts = append(m.facts, content)
	return nil
}
func (m *mockMemEngine) AppendNote(content string) error {
	if m.appendErr != nil {
		return m.appendErr
	}
	m.noted = append(m.noted, content)
	return nil
}
func (m *mockMemEngine) Rewrite(content string) error { return m.rewriteErr }
func (m *mockMemEngine) MemoryPath() string           { return m.memPath }
func (m *mockMemEngine) LoadContext() string          { return "" }
func (m *mockMemEngine) TodayNotes() (string, error)  { return "", nil }

// Compile-time interface check.
var _ mempkg.Engine = (*mockMemEngine)(nil)

// --- mock LLMProvider ---

type mockLLM struct {
	resp *providers.LLMResponse
	err  error
}

func (m *mockLLM) Chat(_ context.Context, _ []providers.Message, _ []providers.Tool) (*providers.LLMResponse, error) {
	return m.resp, m.err
}
func (m *mockLLM) Name() string { return "mock" }

// Compile-time interface check.
var _ providers.LLMProvider = (*mockLLM)(nil)

// --- remember tests ---

func TestRememberTool_EmptyContent(t *testing.T) {
	eng := &mockMemEngine{}
	tool := tools.NewRememberTool(eng)
	result := tool.Execute(context.Background(), map[string]any{})
	if result.ForLLM == "" {
		t.Error("ForLLM must not be empty for empty content")
	}
}

func TestRememberTool_AppendError(t *testing.T) {
	eng := &mockMemEngine{appendErr: errors.New("disk full")}
	tool := tools.NewRememberTool(eng)
	result := tool.Execute(context.Background(), map[string]any{"content": "fact"})
	if result.ForLLM == "" {
		t.Error("ForLLM must not be empty on AppendFact error")
	}
}

func TestRememberTool_Success(t *testing.T) {
	eng := &mockMemEngine{}
	tool := tools.NewRememberTool(eng)
	result := tool.Execute(context.Background(), map[string]any{"content": "Go was created in 2007"})
	if result.ForUser != "Remembered." {
		t.Errorf("ForUser: want %q, got %q", "Remembered.", result.ForUser)
	}
}

// --- note tests ---

func TestNoteTool_EmptyContent(t *testing.T) {
	eng := &mockMemEngine{}
	tool := tools.NewNoteTool(eng)
	result := tool.Execute(context.Background(), map[string]any{})
	if result.ForLLM == "" {
		t.Error("ForLLM must not be empty for empty content")
	}
}

func TestNoteTool_AppendError(t *testing.T) {
	eng := &mockMemEngine{appendErr: errors.New("disk full")}
	tool := tools.NewNoteTool(eng)
	result := tool.Execute(context.Background(), map[string]any{"content": "note"})
	if result.ForLLM == "" {
		t.Error("ForLLM must not be empty on AppendNote error")
	}
}

func TestNoteTool_Success(t *testing.T) {
	eng := &mockMemEngine{}
	tool := tools.NewNoteTool(eng)
	result := tool.Execute(context.Background(), map[string]any{"content": "daily standup done"})
	if result.ForUser != "Noted." {
		t.Errorf("ForUser: want %q, got %q", "Noted.", result.ForUser)
	}
}

// --- curate_memory tests ---

func TestCurateMemoryTool_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	eng := &mockMemEngine{memPath: filepath.Join(dir, "MEMORY.md")}
	// File doesn't exist — os.ReadFile will error.
	tool := tools.NewCurateMemoryTool(eng, &mockLLM{})
	result := tool.Execute(context.Background(), nil)
	if result.ForLLM == "" {
		t.Error("ForLLM must not be empty when memory file missing")
	}
}

func TestCurateMemoryTool_LLMError(t *testing.T) {
	dir := t.TempDir()
	memFile := filepath.Join(dir, "MEMORY.md")
	if err := os.WriteFile(memFile, []byte("some facts"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	eng := &mockMemEngine{memPath: memFile}
	llm := &mockLLM{err: errors.New("llm unavailable")}
	tool := tools.NewCurateMemoryTool(eng, llm)
	result := tool.Execute(context.Background(), nil)
	if result.ForLLM == "" {
		t.Error("ForLLM must not be empty on LLM error")
	}
}

func TestCurateMemoryTool_LLMReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	memFile := filepath.Join(dir, "MEMORY.md")
	if err := os.WriteFile(memFile, []byte("some facts"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	eng := &mockMemEngine{memPath: memFile}
	llm := &mockLLM{resp: &providers.LLMResponse{Content: ""}}
	tool := tools.NewCurateMemoryTool(eng, llm)
	result := tool.Execute(context.Background(), nil)
	if result.ForLLM == "" {
		t.Error("ForLLM must not be empty when LLM returns empty content")
	}
}

func TestCurateMemoryTool_RewriteError(t *testing.T) {
	dir := t.TempDir()
	memFile := filepath.Join(dir, "MEMORY.md")
	if err := os.WriteFile(memFile, []byte("some facts"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	eng := &mockMemEngine{memPath: memFile, rewriteErr: errors.New("rewrite failed")}
	llm := &mockLLM{resp: &providers.LLMResponse{Content: "curated facts"}}
	tool := tools.NewCurateMemoryTool(eng, llm)
	result := tool.Execute(context.Background(), nil)
	if result.ForLLM == "" {
		t.Error("ForLLM must not be empty on Rewrite error")
	}
}

func TestCurateMemoryTool_Success(t *testing.T) {
	dir := t.TempDir()
	memFile := filepath.Join(dir, "MEMORY.md")
	if err := os.WriteFile(memFile, []byte("fact 1\nfact 2"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	eng := &mockMemEngine{memPath: memFile}
	llm := &mockLLM{resp: &providers.LLMResponse{Content: "curated content"}}
	tool := tools.NewCurateMemoryTool(eng, llm)
	result := tool.Execute(context.Background(), nil)
	if result.ForUser != "Memory curated." {
		t.Errorf("ForUser: want %q, got %q", "Memory curated.", result.ForUser)
	}
}
