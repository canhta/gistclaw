// internal/providers/factory/factory_test.go
package factory_test

import (
	"path/filepath"
	"testing"

	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/providers/factory"
	"github.com/canhta/gistclaw/internal/store"
)

func newFactoryStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "factory.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewOpenAIProvider(t *testing.T) {
	cfg := config.Config{
		LLMProvider:  "openai-key",
		OpenAIAPIKey: "test-key",
		OpenAIModel:  "gpt-4o",
	}
	p, err := factory.New(cfg, newFactoryStore(t))
	if err != nil {
		t.Fatalf("New openai-key: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("Name() = %q, want %q", p.Name(), "openai")
	}
}

func TestNewCopilotProvider(t *testing.T) {
	cfg := config.Config{
		LLMProvider:     "copilot",
		CopilotGRPCAddr: "localhost:4321",
	}
	p, err := factory.New(cfg, newFactoryStore(t))
	if err != nil {
		t.Fatalf("New copilot: %v", err)
	}
	if p.Name() != "copilot" {
		t.Errorf("Name() = %q, want %q", p.Name(), "copilot")
	}
}

func TestNewCodexProvider(t *testing.T) {
	cfg := config.Config{
		LLMProvider: "codex-oauth",
	}
	p, err := factory.New(cfg, newFactoryStore(t))
	if err != nil {
		t.Fatalf("New codex-oauth: %v", err)
	}
	if p.Name() != "codex" {
		t.Errorf("Name() = %q, want %q", p.Name(), "codex")
	}
}

func TestNewUnknownProviderReturnsError(t *testing.T) {
	cfg := config.Config{
		LLMProvider: "unknown-provider",
	}
	_, err := factory.New(cfg, newFactoryStore(t))
	if err == nil {
		t.Fatal("expected error for unknown LLM provider")
	}
}

func TestNewOpenAIMissingKeyReturnsError(t *testing.T) {
	cfg := config.Config{
		LLMProvider:  "openai-key",
		OpenAIAPIKey: "", // missing
		OpenAIModel:  "gpt-4o",
	}
	_, err := factory.New(cfg, newFactoryStore(t))
	if err == nil {
		t.Fatal("expected error when OPENAI_API_KEY is missing for openai-key provider")
	}
}
