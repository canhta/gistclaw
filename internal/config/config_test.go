// internal/config/config_test.go
package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/config"
)

func TestLoadRequiresAllowedUserIDs(t *testing.T) {
	os.Clearenv()
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when ALLOWED_USER_IDS is empty")
	}
}

func TestLoadRequiresTelegramToken(t *testing.T) {
	os.Clearenv()
	t.Setenv("ALLOWED_USER_IDS", "123")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when TELEGRAM_TOKEN is empty")
	}
}

func TestLoadMinimalValid(t *testing.T) {
	os.Clearenv()
	t.Setenv("TELEGRAM_TOKEN", "tok")
	t.Setenv("ALLOWED_USER_IDS", "111,222")
	t.Setenv("OPENCODE_DIR", "/tmp/oc")
	t.Setenv("CLAUDE_DIR", "/tmp/cc")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.AllowedUserIDs) != 2 {
		t.Errorf("AllowedUserIDs: got %d, want 2", len(cfg.AllowedUserIDs))
	}
	if cfg.OperatorChatID() != 111 {
		t.Errorf("OperatorChatID: got %d, want 111", cfg.OperatorChatID())
	}
	if cfg.Tuning.HITLTimeout != 5*time.Minute {
		t.Errorf("HITLTimeout default: got %v, want 5m", cfg.Tuning.HITLTimeout)
	}
}

func TestLoadDefaults(t *testing.T) {
	os.Clearenv()
	t.Setenv("TELEGRAM_TOKEN", "tok")
	t.Setenv("ALLOWED_USER_IDS", "1")
	t.Setenv("OPENCODE_DIR", "/tmp/oc")
	t.Setenv("CLAUDE_DIR", "/tmp/cc")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMProvider != "openai-key" {
		t.Errorf("LLMProvider default: got %q, want %q", cfg.LLMProvider, "openai-key")
	}
	if cfg.DailyLimitUSD != 5.0 {
		t.Errorf("DailyLimitUSD default: got %v, want 5.0", cfg.DailyLimitUSD)
	}
	if cfg.OpenCodePort != 8766 {
		t.Errorf("OpenCodePort default: got %d, want 8766", cfg.OpenCodePort)
	}
	if cfg.HookServerAddr != "127.0.0.1:8765" {
		t.Errorf("HookServerAddr default: got %q", cfg.HookServerAddr)
	}
	if cfg.OpenAIModel != "gpt-4o" {
		t.Errorf("OpenAIModel default: got %q, want %q", cfg.OpenAIModel, "gpt-4o")
	}
}

func TestOperatorChatIDEmptySlice(t *testing.T) {
	// Config with empty AllowedUserIDs — not reachable via Load() (validation rejects it),
	// but the method must not panic and must return 0 as the safe default.
	cfg := config.Config{}
	if got := cfg.OperatorChatID(); got != 0 {
		t.Errorf("OperatorChatID() with empty slice = %d, want 0", got)
	}
}

func TestHasSearchProviderFalseWhenNoKeys(t *testing.T) {
	cfg := config.Config{} // all search API key fields empty
	if cfg.HasSearchProvider() {
		t.Error("HasSearchProvider() = true with no keys configured, want false")
	}
}

func TestHasSearchProviderTrueForEachKey(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
	}{
		{"brave", config.Config{BraveAPIKey: "k"}},
		{"gemini", config.Config{GeminiAPIKey: "k"}},
		{"xai", config.Config{XAIAPIKey: "k"}},
		{"perplexity", config.Config{PerplexityAPIKey: "k"}},
		{"openrouter", config.Config{OpenRouterAPIKey: "k"}},
	}
	for _, c := range cases {
		if !c.cfg.HasSearchProvider() {
			t.Errorf("HasSearchProvider() = false with %s key set, want true", c.name)
		}
	}
}

func TestLLMProviders_OverridesLLMProvider(t *testing.T) {
	os.Clearenv()
	t.Setenv("TELEGRAM_TOKEN", "tok")
	t.Setenv("ALLOWED_USER_IDS", "1")
	t.Setenv("OPENCODE_DIR", "/tmp/oc")
	t.Setenv("CLAUDE_DIR", "/tmp/cc")
	t.Setenv("LLM_PROVIDERS", "copilot,openai-key")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.LLMProviders) != 2 {
		t.Errorf("LLMProviders: got %d entries, want 2", len(cfg.LLMProviders))
	}
	if cfg.LLMProviders[0] != "copilot" {
		t.Errorf("LLMProviders[0]: got %q, want %q", cfg.LLMProviders[0], "copilot")
	}
}

func TestLLMProviders_LegacySingleProvider_StillWorks(t *testing.T) {
	os.Clearenv()
	t.Setenv("TELEGRAM_TOKEN", "tok")
	t.Setenv("ALLOWED_USER_IDS", "1")
	t.Setenv("OPENCODE_DIR", "/tmp/oc")
	t.Setenv("CLAUDE_DIR", "/tmp/cc")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	// LLM_PROVIDERS not set; LLM_PROVIDER defaults to "openai-key"
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMProvider != "openai-key" {
		t.Errorf("LLMProvider: got %q, want openai-key", cfg.LLMProvider)
	}
}

func TestLLMProviders_UnknownProvider_ReturnsError(t *testing.T) {
	os.Clearenv()
	t.Setenv("TELEGRAM_TOKEN", "tok")
	t.Setenv("ALLOWED_USER_IDS", "1")
	t.Setenv("OPENCODE_DIR", "/tmp/oc")
	t.Setenv("CLAUDE_DIR", "/tmp/cc")
	t.Setenv("LLM_PROVIDERS", "copilot,unknown-provider")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for unknown provider in LLM_PROVIDERS")
	}
}

func TestLLMProviders_MissingOpenAIKey_ReturnsError(t *testing.T) {
	os.Clearenv()
	t.Setenv("TELEGRAM_TOKEN", "tok")
	t.Setenv("ALLOWED_USER_IDS", "1")
	t.Setenv("OPENCODE_DIR", "/tmp/oc")
	t.Setenv("CLAUDE_DIR", "/tmp/cc")
	t.Setenv("LLM_PROVIDERS", "copilot,openai-key")
	// OPENAI_API_KEY not set
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when openai-key in LLM_PROVIDERS but OPENAI_API_KEY missing")
	}
}

func TestSummarizeAtTurns_Default(t *testing.T) {
	os.Clearenv()
	t.Setenv("TELEGRAM_TOKEN", "tok")
	t.Setenv("ALLOWED_USER_IDS", "1")
	t.Setenv("OPENCODE_DIR", "/tmp/oc")
	t.Setenv("CLAUDE_DIR", "/tmp/cc")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Tuning.SummarizeAtTurns != 0 {
		t.Errorf("SummarizeAtTurns default: got %d, want 0", cfg.Tuning.SummarizeAtTurns)
	}
}
