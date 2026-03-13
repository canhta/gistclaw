// internal/config/config.go
package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds all runtime configuration for GistClaw.
type Config struct {
	TelegramToken  string  `env:"TELEGRAM_TOKEN"`
	AllowedUserIDs []int64 `env:"ALLOWED_USER_IDS" envSeparator:","`
	OpenCodeDir    string  `env:"OPENCODE_DIR"`
	ClaudeDir      string  `env:"CLAUDE_DIR"`
	DailyLimitUSD  float64 `env:"DAILY_LIMIT_USD"  envDefault:"5.0"`
	LLMProvider    string  `env:"LLM_PROVIDER"    envDefault:"openai-key"`
	// LLMProviders is an ordered fallback list (e.g. "copilot,openai-key").
	// If non-empty, takes priority over LLMProvider.
	LLMProviders      []string      `env:"LLM_PROVIDERS"       envSeparator:","`
	LLMCooldownWindow time.Duration `env:"LLM_COOLDOWN_WINDOW" envDefault:"5m"`
	// MemoryNotesDir is the directory for date-partitioned notes files.
	// Defaults to filepath.Join(filepath.Dir(MemoryPath), "notes") at runtime if empty.
	MemoryNotesDir string `env:"MEMORY_NOTES_DIR"`

	// LLM provider credentials
	OpenAIAPIKey    string `env:"OPENAI_API_KEY"`
	OpenAIModel     string `env:"OPENAI_MODEL"      envDefault:"gpt-4o"`
	CopilotGRPCAddr string `env:"COPILOT_GRPC_ADDR" envDefault:"localhost:4321"`

	// Infrastructure
	OpenCodePort   int    `env:"OPENCODE_PORT"    envDefault:"8766"`
	HookServerAddr string `env:"HOOK_SERVER_ADDR" envDefault:"127.0.0.1:8765"`
	SoulPath       string `env:"SOUL_PATH"        envDefault:"./SOUL.md"`
	MemoryPath     string `env:"MEMORY_PATH"      envDefault:"./MEMORY.md"`
	SQLitePath     string `env:"SQLITE_PATH"      envDefault:"./gistclaw.db"`
	LogLevel       string `env:"LOG_LEVEL"        envDefault:"info"`
	MCPConfigPath  string `env:"MCP_CONFIG_PATH"  envDefault:"./gistclaw.yaml"`

	// Search provider API keys (at least one needed for web_search tool)
	BraveAPIKey      string `env:"GISTCLAW_BRAVE_API_KEY"`
	GeminiAPIKey     string `env:"GISTCLAW_GEMINI_API_KEY"`
	XAIAPIKey        string `env:"GISTCLAW_XAI_API_KEY"`
	PerplexityAPIKey string `env:"GISTCLAW_PERPLEXITY_API_KEY"`
	OpenRouterAPIKey string `env:"GISTCLAW_OPENROUTER_API_KEY"`

	Tuning Tuning
}

// Tuning holds all timeouts and TTLs in one place. No magic constants elsewhere.
type Tuning struct {
	HITLTimeout             time.Duration `env:"TUNING_HITL_TIMEOUT"           envDefault:"5m"`
	HITLReminderBefore      time.Duration `env:"TUNING_HITL_REMINDER_BEFORE"   envDefault:"2m"`
	WebSearchTimeout        time.Duration `env:"TUNING_WEB_SEARCH_TIMEOUT"     envDefault:"10s"`
	WebFetchTimeout         time.Duration `env:"TUNING_WEB_FETCH_TIMEOUT"      envDefault:"30s"`
	SessionTTL              time.Duration `env:"TUNING_SESSION_TTL"            envDefault:"24h"`
	CostHistoryTTL          time.Duration `env:"TUNING_COST_HISTORY_TTL"       envDefault:"720h"` // 30d
	HeartbeatTier1Every     time.Duration `env:"TUNING_HB_TIER1_EVERY"         envDefault:"30s"`
	HeartbeatTier2Every     time.Duration `env:"TUNING_HB_TIER2_EVERY"         envDefault:"5m"`
	SchedulerTick           time.Duration `env:"TUNING_SCHEDULER_TICK"         envDefault:"1s"`
	MissedJobsFireLimit     int           `env:"TUNING_MISSED_JOBS_FIRE_LIMIT" envDefault:"5"`
	MaxIterations           int           `env:"TUNING_MAX_ITERATIONS"              envDefault:"20"`
	LLMRetryDelay           time.Duration `env:"TUNING_LLM_RETRY_DELAY"             envDefault:"1s"`
	ConversationWindowTurns int           `env:"TUNING_CONVERSATION_WINDOW_TURNS"   envDefault:"20"`
	SummarizeAtTurns        int           `env:"TUNING_SUMMARIZE_AT_TURNS"          envDefault:"0"`
	MCPConnectTimeout       time.Duration `env:"TUNING_MCP_CONNECT_TIMEOUT"    envDefault:"15s"`
	MCPCallTimeout          time.Duration `env:"TUNING_MCP_CALL_TIMEOUT"       envDefault:"10s"`
}

// Load parses all environment variables and validates required fields.
// Returns an error if any required field is missing or invalid.
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: parse env: %w", err)
	}
	return cfg, validate(cfg)
}

// OperatorChatID returns the Telegram chat ID to use for supervisor notifications.
// This is always AllowedUserIDs[0], which is guaranteed non-empty by Load().
// Returns 0 if AllowedUserIDs is empty (safe: Telegram IDs are never zero).
func (c Config) OperatorChatID() int64 {
	if len(c.AllowedUserIDs) == 0 {
		return 0
	}
	return c.AllowedUserIDs[0]
}

// HasSearchProvider returns true if at least one search API key is configured.
func (c Config) HasSearchProvider() bool {
	return c.BraveAPIKey != "" || c.GeminiAPIKey != "" ||
		c.XAIAPIKey != "" || c.PerplexityAPIKey != "" || c.OpenRouterAPIKey != ""
}

// EffectiveLLMProviders returns the ordered provider list.
// If LLMProviders is set, it is returned directly.
// Otherwise, LLMProvider is returned as a single-element slice.
func (c Config) EffectiveLLMProviders() []string {
	if len(c.LLMProviders) > 0 {
		return append([]string(nil), c.LLMProviders...)
	}
	return []string{c.LLMProvider}
}

// EffectiveMemoryNotesDir returns the notes directory, deriving it from
// MemoryPath if MemoryNotesDir is empty.
func (c Config) EffectiveMemoryNotesDir() string {
	if c.MemoryNotesDir != "" {
		return c.MemoryNotesDir
	}
	return filepath.Join(filepath.Dir(c.MemoryPath), "notes")
}

func validate(cfg Config) error {
	var errs []string
	if cfg.TelegramToken == "" {
		errs = append(errs, "TELEGRAM_TOKEN is required")
	}
	if len(cfg.AllowedUserIDs) == 0 {
		errs = append(errs, "ALLOWED_USER_IDS is required (comma-separated Telegram user IDs)")
	}
	if cfg.OpenCodeDir == "" {
		errs = append(errs, "OPENCODE_DIR is required")
	}
	if cfg.ClaudeDir == "" {
		errs = append(errs, "CLAUDE_DIR is required")
	}
	validProviders := map[string]bool{"openai-key": true, "copilot": true, "codex-oauth": true}
	if len(cfg.LLMProviders) == 0 {
		// Legacy single-provider mode.
		if !validProviders[cfg.LLMProvider] {
			errs = append(errs, fmt.Sprintf("LLM_PROVIDER must be one of: openai-key, copilot, codex-oauth (got %q)", cfg.LLMProvider))
		}
		if cfg.LLMProvider == "openai-key" && cfg.OpenAIAPIKey == "" {
			errs = append(errs, "OPENAI_API_KEY is required when LLM_PROVIDER=openai-key")
		}
	} else {
		// Multi-provider mode: validate each entry.
		for _, p := range cfg.LLMProviders {
			if !validProviders[p] {
				errs = append(errs, fmt.Sprintf("LLM_PROVIDERS: unknown provider %q (valid: openai-key, copilot, codex-oauth)", p))
			}
		}
		for _, p := range cfg.LLMProviders {
			if p == "openai-key" && cfg.OpenAIAPIKey == "" {
				errs = append(errs, "OPENAI_API_KEY is required when openai-key is in LLM_PROVIDERS")
				break
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}
