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
}
