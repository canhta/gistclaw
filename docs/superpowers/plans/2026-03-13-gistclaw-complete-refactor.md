# GistClaw Complete Refactor Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor gistclaw internals to add provider fallback chains, LLM-curated memory, proactive conversation summarization, a formal tool registry, and multi-agent orchestration tools — without changing the service topology or external interfaces.

**Architecture:** Five independent layers are built bottom-up: ProviderRouter wraps existing providers with ordered fallback + per-provider cooldown; MemoryEngine consolidates SOUL/MEMORY.md/daily-notes into one package; ConversationManager adds proactive LLM summarization; a formal ToolEngine replaces the gateway switch statement; three new agent tools (spawn_agent, run_parallel, chain_agents) enable multi-agent delegation. The gateway is then refactored from a 793-line god file into three focused files, and app.go is updated to wire it all together.

**Tech Stack:** Go 1.25.7, `github.com/canhta/gistclaw` module, standard `testing` package, `modernc.org/sqlite`, `github.com/rs/zerolog`

**Spec:** `docs/superpowers/specs/2026-03-13-gistclaw-complete-refactor-design.md`

---

## File Map

### New Files

| File | Responsibility |
|------|----------------|
| `internal/providers/router.go` | ProviderRouter: ordered fallback + per-provider cooldown |
| `internal/providers/router_test.go` | Router unit tests |
| `internal/memory/engine.go` | MemoryEngine: SOUL + MEMORY.md + daily notes |
| `internal/memory/engine_test.go` | Memory engine tests |
| `internal/conversation/manager.go` | ConversationManager: windowed history + proactive summarization |
| `internal/conversation/manager_test.go` | Conversation manager tests |
| `internal/tools/tool.go` | Tool interface + ToolResult + ToolEngine registry |
| `internal/tools/tool_test.go` | ToolEngine registration and dispatch tests |
| `internal/tools/web_search_tool.go` | `web_search` tool wrapping existing `tools.SearchProvider` |
| `internal/tools/web_fetch_tool.go` | `web_fetch` tool wrapping existing `tools.WebFetcher` |
| `internal/tools/memory_tool.go` | `remember`, `note`, `curate_memory` tools |
| `internal/tools/scheduler_tool.go` | `schedule_job`, `list_jobs`, `update_job`, `delete_job` tools |
| `internal/tools/mcp_tool.go` | MCP tool adapter (dynamic per-server tools) |
| `internal/tools/agents_tool.go` | `spawn_agent`, `run_parallel`, `chain_agents` tools |
| `internal/tools/agents_tool_test.go` | Multi-agent tool tests |
| `internal/gateway/router.go` | Extracted: `handle()`, `handleCallback()`, `isAllowed()`, `buildStatus()` |
| `internal/gateway/loop.go` | Extracted: `handlePlainChat()`, `chatWithRetry()`, `buildToolEngine()` |

### Modified Files

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `LLMProviders`, `LLMCooldownWindow`, `MemoryNotesDir`, `SummarizeAtTurns`; update `validate()` |
| `internal/store/sqlite.go` | Add `CountMessages()`, `ReplaceHistory()` |
| `internal/providers/factory/factory.go` | Build `ProviderRouter` instead of single provider |
| `internal/opencode/service.go` | Add `SubmitTaskWithResult` to `Service` interface + `serviceImpl` |
| `internal/claudecode/service.go` | Add `SubmitTaskWithResult` to `Service` interface + `serviceImpl` |
| `internal/gateway/service.go` | New fields (`memory`, `conv`, `lifetimeCtx`); remove `soul`/`memory` SOULLoader fields; update `NewService` signature; add `ocService.SubmitTaskWithResult` + `ccService.SubmitTaskWithResult` to local interfaces |
| `internal/app/app.go` | Remove `soul`/`memory` fields; construct `memory.Engine` + `conversation.Manager`; update `gateway.NewService` call |

### Task Dependency Order

Tasks 1–7 are independent and can be executed in parallel. Tasks 8–11 depend only on Task 8 (ToolEngine). Tasks 12–13 depend on all prior tasks.

```
Tasks 1–7 (parallel) → Tasks 8–11 (sequential, 9–11 after 8) → Task 12 → Task 13
```

---

## Chunk 1: Config + Store Foundations

### Task 1: Config — New Fields and Updated Validation

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/config/config_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/canh/Projects/Claw/gistclaw
go test ./internal/config/... -run "TestLLMProviders|TestSummarize" -v
```

Expected: FAIL — fields not found on Config struct.

- [ ] **Step 3: Implement the config changes**

In `internal/config/config.go`, add to the `Config` struct (after `LLMProvider`):

```go
// LLMProviders is an ordered fallback list (e.g. "copilot,openai-key").
// If non-empty, takes priority over LLMProvider.
LLMProviders      []string      `env:"LLM_PROVIDERS"       envSeparator:","`
LLMCooldownWindow time.Duration `env:"LLM_COOLDOWN_WINDOW" envDefault:"5m"`
// MemoryNotesDir is the directory for date-partitioned notes files.
// Defaults to filepath.Join(filepath.Dir(MemoryPath), "notes") at runtime if empty.
MemoryNotesDir string `env:"MEMORY_NOTES_DIR"`
```

Add to `Tuning` struct:

```go
SummarizeAtTurns int `env:"TUNING_SUMMARIZE_AT_TURNS" envDefault:"0"`
```

Add `"path/filepath"` to imports.

Replace the existing `validate()` LLM block:

```go
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
```

Add a helper used by factory (add as exported method on Config):

```go
// EffectiveLLMProviders returns the ordered provider list.
// If LLMProviders is set, it is returned directly.
// Otherwise, LLMProvider is returned as a single-element slice.
func (c Config) EffectiveLLMProviders() []string {
    if len(c.LLMProviders) > 0 {
        return c.LLMProviders
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/config/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add LLMProviders, MemoryNotesDir, SummarizeAtTurns fields"
```

---

### Task 2: Store — CountMessages and ReplaceHistory

**Files:**
- Modify: `internal/store/sqlite.go`
- Modify: `internal/store/sqlite_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/store/sqlite_test.go`:

```go
func TestCountMessages_Empty(t *testing.T) {
	s := newTestStore(t)
	count, err := s.CountMessages(42)
	if err != nil {
		t.Fatalf("CountMessages: %v", err)
	}
	if count != 0 {
		t.Errorf("CountMessages empty: got %d, want 0", count)
	}
}

func TestCountMessages_WithRows(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 3; i++ {
		if err := s.SaveMessage(42, "user", "hello"); err != nil {
			t.Fatalf("SaveMessage: %v", err)
		}
	}
	count, err := s.CountMessages(42)
	if err != nil {
		t.Fatalf("CountMessages: %v", err)
	}
	if count != 3 {
		t.Errorf("CountMessages: got %d, want 3", count)
	}
	// Different chatID must return 0.
	count2, err := s.CountMessages(99)
	if err != nil {
		t.Fatalf("CountMessages other chatID: %v", err)
	}
	if count2 != 0 {
		t.Errorf("CountMessages other chatID: got %d, want 0", count2)
	}
}

func TestReplaceHistory_ReplacesAllRows(t *testing.T) {
	s := newTestStore(t)
	// Seed some old rows.
	for i := 0; i < 5; i++ {
		_ = s.SaveMessage(1, "user", "old")
	}
	// Replace with 2 new rows.
	rows := []store.HistoryMessage{
		{Role: "assistant", Content: "[Summary]: old stuff"},
		{Role: "user", Content: "new message"},
	}
	// Note: all reconstituted rows will have the same created_at timestamp.
	// GetHistory orders by (created_at DESC, id DESC) then reverses — so ordering
	// is preserved via AUTOINCREMENT id within the same transaction. This is correct
	// under SQLite's single-connection sequential inserts.
	if err := s.ReplaceHistory(1, rows); err != nil {
		t.Fatalf("ReplaceHistory: %v", err)
	}
	got, err := s.GetHistory(1, 100)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetHistory len: got %d, want 2", len(got))
	}
	if got[0].Content != "[Summary]: old stuff" {
		t.Errorf("row 0 content: got %q, want summary", got[0].Content)
	}
}

func TestReplaceHistory_OtherChatIDUnaffected(t *testing.T) {
	s := newTestStore(t)
	_ = s.SaveMessage(1, "user", "chat1")
	_ = s.SaveMessage(2, "user", "chat2")
	_ = s.ReplaceHistory(1, []store.HistoryMessage{{Role: "assistant", Content: "new"}})
	got, _ := s.GetHistory(2, 100)
	if len(got) != 1 {
		t.Errorf("chat2 should be unaffected: got %d rows, want 1", len(got))
	}
}

func TestReplaceHistory_EmptyRows_DeletesAll(t *testing.T) {
	s := newTestStore(t)
	_ = s.SaveMessage(1, "user", "old")
	if err := s.ReplaceHistory(1, nil); err != nil {
		t.Fatalf("ReplaceHistory nil: %v", err)
	}
	count, _ := s.CountMessages(1)
	if count != 0 {
		t.Errorf("after ReplaceHistory nil: got %d rows, want 0", count)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/store/... -run "TestCountMessages|TestReplaceHistory" -v
```

Expected: FAIL — methods not found.

- [ ] **Step 3: Implement the new store methods**

Add to `internal/store/sqlite.go`:

```go
// HistoryMessage is a single row from the messages table.
// Note: this type may already be defined above — if so, skip this declaration.
// (Check if store.HistoryMessage exists; it is used by GetHistory.)

// CountMessages returns the number of message rows for chatID.
func (s *Store) CountMessages(chatID int64) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE chat_id = ?`, chatID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: count messages: %w", err)
	}
	return count, nil
}

// ReplaceHistory deletes all message rows for chatID and inserts rows in a single
// transaction. rows is ordered oldest-first. Passing nil or empty rows deletes all.
func (s *Store) ReplaceHistory(chatID int64, rows []HistoryMessage) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: replace history: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck — no-op after Commit

	if _, err := tx.Exec(`DELETE FROM messages WHERE chat_id = ?`, chatID); err != nil {
		return fmt.Errorf("store: replace history: delete: %w", err)
	}
	for _, row := range rows {
		if _, err := tx.Exec(
			`INSERT INTO messages (chat_id, role, content) VALUES (?, ?, ?)`,
			chatID, row.Role, row.Content,
		); err != nil {
			return fmt.Errorf("store: replace history: insert: %w", err)
		}
	}
	return tx.Commit()
}
```

> **Note on HistoryMessage:** Check whether `HistoryMessage` is already defined in `sqlite.go`. If it is, skip the type declaration above. If GetHistory uses an anonymous struct or a different name, align accordingly. Look for the `GetHistory` return type.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/store/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/sqlite.go internal/store/sqlite_test.go
git commit -m "feat(store): add CountMessages and ReplaceHistory"
```

---

## Chunk 2: ProviderRouter + MemoryEngine

### Task 3: ProviderRouter

**Files:**
- Create: `internal/providers/router.go`
- Create: `internal/providers/router_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/providers/router_test.go`:

```go
package providers_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/providers"
)

// mockProvider is a test double for LLMProvider.
type mockProvider struct {
	name   string
	resp   *providers.LLMResponse
	err    error
	calls  atomic.Int32
}

func (m *mockProvider) Chat(_ context.Context, _ []providers.Message, _ []providers.Tool) (*providers.LLMResponse, error) {
	m.calls.Add(1)
	return m.resp, m.err
}
func (m *mockProvider) Name() string { return m.name }

func TestRouter_SuccessOnFirst(t *testing.T) {
	p1 := &mockProvider{name: "p1", resp: &providers.LLMResponse{Content: "ok"}}
	p2 := &mockProvider{name: "p2", resp: &providers.LLMResponse{Content: "fallback"}}
	r := providers.NewProviderRouter([]providers.LLMProvider{p1, p2}, 5*time.Minute)
	resp, err := r.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("content: got %q, want %q", resp.Content, "ok")
	}
	if p2.calls.Load() != 0 {
		t.Error("p2 should not be called when p1 succeeds")
	}
}

func TestRouter_FallsBackOnRateLimit(t *testing.T) {
	p1 := &mockProvider{name: "p1", err: errors.New("429 rate limit exceeded")}
	p2 := &mockProvider{name: "p2", resp: &providers.LLMResponse{Content: "fallback"}}
	r := providers.NewProviderRouter([]providers.LLMProvider{p1, p2}, 5*time.Minute)
	resp, err := r.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "fallback" {
		t.Errorf("content: got %q, want fallback", resp.Content)
	}
}

func TestRouter_TerminalError_NoFallback(t *testing.T) {
	p1 := &mockProvider{name: "p1", err: errors.New("400 bad request")}
	p2 := &mockProvider{name: "p2", resp: &providers.LLMResponse{Content: "should not reach"}}
	r := providers.NewProviderRouter([]providers.LLMProvider{p1, p2}, 5*time.Minute)
	_, err := r.Chat(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error on terminal error")
	}
	if p2.calls.Load() != 0 {
		t.Error("p2 should not be called on terminal error")
	}
}

func TestRouter_CooldownSkipsProvider(t *testing.T) {
	p1 := &mockProvider{name: "p1", err: errors.New("429 rate limit")}
	p2 := &mockProvider{name: "p2", resp: &providers.LLMResponse{Content: "p2"}}
	r := providers.NewProviderRouter([]providers.LLMProvider{p1, p2}, 1*time.Hour)
	// First call: p1 rate-limited → falls back to p2, p1 goes on cooldown.
	_, _ = r.Chat(context.Background(), nil, nil)
	p1.calls.Store(0)
	p2.calls.Store(0)
	// Second call: p1 still on cooldown → goes straight to p2.
	resp, err := r.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p1.calls.Load() != 0 {
		t.Error("p1 should be skipped (on cooldown)")
	}
	if resp.Content != "p2" {
		t.Errorf("content: got %q, want p2", resp.Content)
	}
}

func TestRouter_ContextCanceled_ReturnsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	p1 := &mockProvider{name: "p1", err: context.Canceled}
	p2 := &mockProvider{name: "p2", resp: &providers.LLMResponse{Content: "should not reach"}}
	r := providers.NewProviderRouter([]providers.LLMProvider{p1, p2}, 5*time.Minute)
	_, err := r.Chat(ctx, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if p2.calls.Load() != 0 {
		t.Error("p2 should not be called when context is cancelled")
	}
}

func TestRouter_AllExhausted_ReturnsLastError(t *testing.T) {
	p1 := &mockProvider{name: "p1", err: errors.New("429 rate limit")}
	p2 := &mockProvider{name: "p2", err: errors.New("503 service unavailable")}
	r := providers.NewProviderRouter([]providers.LLMProvider{p1, p2}, 5*time.Minute)
	_, err := r.Chat(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestRouter_Name(t *testing.T) {
	p1 := &mockProvider{name: "copilot"}
	p2 := &mockProvider{name: "openai-key"}
	r := providers.NewProviderRouter([]providers.LLMProvider{p1, p2}, 5*time.Minute)
	name := r.Name()
	if name == "" {
		t.Error("Name() should not be empty")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/providers/... -run "TestRouter" -v
```

Expected: FAIL — `providers.NewProviderRouter` not found.

- [ ] **Step 3: Implement ProviderRouter**

Create `internal/providers/router.go`:

```go
// internal/providers/router.go
package providers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ProviderRouter implements LLMProvider with ordered fallback and per-provider cooldown.
// It is transparent to callers — use it wherever an LLMProvider is expected.
type ProviderRouter struct {
	providers []LLMProvider
	window    time.Duration // cooldown duration on rate limit

	mu        sync.Mutex
	cooldowns map[string]time.Time // provider name → cooldown expiry
}

// NewProviderRouter constructs a ProviderRouter.
// providers: ordered list, tried in sequence; must be non-empty.
// cooldownWindow: how long to pause a provider after a rate-limit error.
func NewProviderRouter(providers []LLMProvider, cooldownWindow time.Duration) *ProviderRouter {
	return &ProviderRouter{
		providers: providers,
		window:    cooldownWindow,
		cooldowns: make(map[string]time.Time),
	}
}

// Chat tries each provider in order, skipping providers on cooldown.
// Propagates context cancellation immediately without trying further providers.
// On rate-limit error, puts the provider on cooldown and tries the next.
// On terminal error, returns immediately without fallback.
// Returns the last error if all providers are exhausted.
func (r *ProviderRouter) Chat(ctx context.Context, msgs []Message, tools []Tool) (*LLMResponse, error) {
	var lastErr error
	for _, p := range r.providers {
		if r.isCooledDown(p.Name()) {
			continue
		}
		resp, err := p.Chat(ctx, msgs, tools)
		if err == nil {
			return resp, nil
		}
		// Propagate context cancellation immediately — do NOT classify or fallback.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		lastErr = err
		switch ClassifyError(err) {
		case ErrKindTerminal:
			return nil, err
		case ErrKindRateLimit:
			r.setCooldown(p.Name())
			// try next provider
		case ErrKindRetryable:
			// try next provider
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("router: all providers exhausted: %w", lastErr)
	}
	return nil, fmt.Errorf("router: no providers available")
}

// Name returns a human-readable description of the router's provider chain.
func (r *ProviderRouter) Name() string {
	names := make([]string, len(r.providers))
	for i, p := range r.providers {
		names[i] = p.Name()
	}
	return "router(" + strings.Join(names, "→") + ")"
}

func (r *ProviderRouter) isCooledDown(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	expiry, ok := r.cooldowns[name]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(r.cooldowns, name)
		return false
	}
	return true
}

func (r *ProviderRouter) setCooldown(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cooldowns[name] = time.Now().Add(r.window)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/providers/... -v
```

Expected: all PASS.

- [ ] **Step 5: Update factory to build ProviderRouter**

Replace `internal/providers/factory/factory.go`:

```go
// internal/providers/factory/factory.go
package factory

import (
	"fmt"
	"time"

	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/providers"
	codexprovider "github.com/canhta/gistclaw/internal/providers/codex"
	copilotprovider "github.com/canhta/gistclaw/internal/providers/copilot"
	oaiprovider "github.com/canhta/gistclaw/internal/providers/openai"
	"github.com/canhta/gistclaw/internal/store"
)

// New constructs an LLMProvider from cfg.
// If cfg.LLMProviders has multiple entries, returns a ProviderRouter.
// If only one provider is configured (legacy LLM_PROVIDER), returns it directly
// wrapped in a single-entry router for uniform behaviour.
func New(cfg config.Config, s *store.Store) (providers.LLMProvider, error) {
	providerNames := cfg.EffectiveLLMProviders()
	impls := make([]providers.LLMProvider, 0, len(providerNames))
	for _, name := range providerNames {
		p, err := buildOne(name, cfg, s)
		if err != nil {
			return nil, err
		}
		impls = append(impls, p)
	}
	if len(impls) == 1 {
		return impls[0], nil
	}
	window := cfg.LLMCooldownWindow
	if window <= 0 {
		window = 5 * time.Minute
	}
	return providers.NewProviderRouter(impls, window), nil
}

func buildOne(name string, cfg config.Config, s *store.Store) (providers.LLMProvider, error) {
	switch name {
	case "openai-key":
		model := cfg.OpenAIModel
		if model == "" {
			model = "gpt-4o"
		}
		return oaiprovider.New(cfg.OpenAIAPIKey, model), nil
	case "copilot":
		addr := cfg.CopilotGRPCAddr
		if addr == "" {
			addr = "localhost:4321"
		}
		return copilotprovider.New(addr), nil
	case "codex-oauth":
		return codexprovider.New(s), nil
	default:
		return nil, fmt.Errorf("factory: unknown provider %q", name)
	}
}
```

- [ ] **Step 6: Run all provider tests (including existing factory tests)**

```bash
go test ./internal/providers/... -v
```

Expected: all PASS. The refactored `factory.New` uses `cfg.EffectiveLLMProviders()` — existing factory tests that set `LLMProvider: "openai-key"` will work because `EffectiveLLMProviders()` falls back to `LLMProvider` when `LLMProviders` is empty.

- [ ] **Step 7: Commit**

```bash
git add internal/providers/router.go internal/providers/router_test.go \
        internal/providers/factory/factory.go
git commit -m "feat(providers): add ProviderRouter with fallback chains and cooldown"
```

---

### Task 4: MemoryEngine

**Files:**
- Create: `internal/memory/engine.go`
- Create: `internal/memory/engine_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/memory/engine_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/memory/... -v
```

Expected: FAIL — package not found.

- [ ] **Step 3: Implement MemoryEngine**

Create `internal/memory/engine.go`:

```go
// internal/memory/engine.go
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
//   soulPath:   path to SOUL.md; empty string disables SOUL loading.
//   memoryPath: path to MEMORY.md.
//   notesDir:   directory for date-partitioned notes; created on first AppendNote.
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

	// 3. Today's notes, capped at 8000 bytes (tail kept — newest entries preserved).
	if notes, err := e.TodayNotes(); err == nil && notes != "" {
		capped := tailBytes(notes, 8000)
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
	return os.WriteFile(e.memoryPath, []byte(content), 0o644)
}

func (e *engine) TodayNotes() (string, error) {
	file := filepath.Join(e.notesDir, time.Now().Format("2006-01-02")+".md")
	content, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("memory: read today notes: %w", err)
	}
	return string(content), nil
}

// appendToFile appends data to path, creating the file and parent dirs if needed.
func appendToFile(path, data string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("memory: create dir for %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("memory: open %s: %w", path, err)
	}
	defer f.Close()
	_, err = f.WriteString(data)
	return err
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/memory/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/engine.go internal/memory/engine_test.go
git commit -m "feat(memory): add MemoryEngine with SOUL/MEMORY.md/daily-notes support"
```

---

## Chunk 3: ConversationManager + Service Extensions

### Task 5: ConversationManager

**Files:**
- Create: `internal/conversation/manager.go`
- Create: `internal/conversation/manager_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/conversation/manager_test.go`:

```go
package conversation_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/conversation"
	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/store"
)

func newTestManager(t *testing.T, windowTurns, summarizeAtTurns int) (conversation.Manager, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() }) //nolint:errcheck
	m := conversation.NewManager(s, windowTurns, summarizeAtTurns)
	return m, s
}

func TestLoad_Empty(t *testing.T) {
	m, _ := newTestManager(t, 20, 0)
	msgs, err := m.Load(42)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("Load empty: got %d msgs, want 0", len(msgs))
	}
}

func TestSaveAndLoad(t *testing.T) {
	m, _ := newTestManager(t, 20, 0)
	_ = m.Save(1, "user", "hello")
	_ = m.Save(1, "assistant", "world")
	msgs, err := m.Load(1)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("Load: got %d msgs, want 2", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Errorf("msg[0]: got %+v", msgs[0])
	}
}

func TestLoad_RespectsWindowTurns(t *testing.T) {
	m, _ := newTestManager(t, 2, 0) // windowTurns=2 → max 4 rows
	for i := 0; i < 10; i++ {
		_ = m.Save(1, "user", "msg")
		_ = m.Save(1, "assistant", "reply")
	}
	msgs, err := m.Load(1)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(msgs) > 4 {
		t.Errorf("Load window: got %d msgs, want ≤4 (windowTurns*2)", len(msgs))
	}
}

func TestMaybeSummarize_DisabledByDefault(t *testing.T) {
	m, s := newTestManager(t, 20, 0) // summarizeAtTurns=0
	for i := 0; i < 30; i++ {
		_ = m.Save(1, "user", "msg")
	}
	// Mock LLM that must NOT be called.
	llm := &failIfCalledProvider{t: t}
	err := m.MaybeSummarize(context.Background(), 1, llm)
	if err != nil {
		t.Fatalf("MaybeSummarize: %v", err)
	}
	count, _ := s.CountMessages(1)
	if count != 30 {
		t.Errorf("rows should be unchanged: got %d, want 30", count)
	}
}

func TestMaybeSummarize_BelowThreshold_NoOp(t *testing.T) {
	m, s := newTestManager(t, 20, 16)
	for i := 0; i < 10; i++ { // 10 < 16 threshold
		_ = m.Save(1, "user", "msg")
	}
	llm := &failIfCalledProvider{t: t}
	err := m.MaybeSummarize(context.Background(), 1, llm)
	if err != nil {
		t.Fatalf("MaybeSummarize below threshold: %v", err)
	}
	count, _ := s.CountMessages(1)
	if count != 10 {
		t.Errorf("rows should be unchanged: got %d, want 10", count)
	}
}

func TestMaybeSummarize_AtThreshold_Summarizes(t *testing.T) {
	m, s := newTestManager(t, 20, 5)
	for i := 0; i < 5; i++ {
		_ = m.Save(1, "user", fmt.Sprintf("msg %d", i))
		_ = m.Save(1, "assistant", fmt.Sprintf("reply %d", i))
	} // 10 rows ≥ 5
	llm := &stubSummaryProvider{summary: "old stuff summarized"}
	err := m.MaybeSummarize(context.Background(), 1, llm)
	if err != nil {
		t.Fatalf("MaybeSummarize: %v", err)
	}
	msgs, _ := m.Load(1)
	// Should be: 1 summary row + 4 recent rows = 5
	if len(msgs) > 5 {
		t.Errorf("after summarization: got %d msgs, want ≤5", len(msgs))
	}
	// Summary row should exist.
	found := false
	for _, msg := range msgs {
		if strings.Contains(msg.Content, "Summary") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a summary row after summarization")
	}
	_ = s // suppress unused warning
}

// failIfCalledProvider panics if Chat is called (used to assert LLM is NOT called).
type failIfCalledProvider struct{ t *testing.T }

func (f *failIfCalledProvider) Chat(_ context.Context, _ []providers.Message, _ []providers.Tool) (*providers.LLMResponse, error) {
	f.t.Fatal("LLM should not be called")
	return nil, nil
}
func (f *failIfCalledProvider) Name() string { return "fail-if-called" }

// stubSummaryProvider returns a fixed summary string.
type stubSummaryProvider struct{ summary string }

func (s *stubSummaryProvider) Chat(_ context.Context, _ []providers.Message, _ []providers.Tool) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: s.summary}, nil
}
func (s *stubSummaryProvider) Name() string { return "stub-summary" }
```

> **Note:** Add `"fmt"` to the import block of the test file.

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/conversation/... -v
```

Expected: FAIL — package not found.

- [ ] **Step 3: Implement ConversationManager**

Create `internal/conversation/manager.go`:

```go
// internal/conversation/manager.go
package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/store"
)

// Manager handles conversation history with optional proactive summarization.
type Manager interface {
	// Load returns history for chatID capped at windowTurns*2 rows (chronological order).
	Load(chatID int64) ([]providers.Message, error)

	// Save persists a single message row.
	Save(chatID int64, role, content string) error

	// MaybeSummarize checks if history exceeds the summarization threshold.
	// Fast path (below threshold or disabled): returns nil immediately.
	// Slow path: makes one LLM call and rewrites history in SQLite.
	// If summarizeAtTurns == 0, always returns nil.
	MaybeSummarize(ctx context.Context, chatID int64, llm providers.LLMProvider) error
}

type manager struct {
	store            *store.Store
	windowTurns      int
	summarizeAtTurns int
}

// NewManager constructs a Manager.
//   windowTurns: history cap in turns (rows fetched = windowTurns*2).
//   summarizeAtTurns: row count threshold; 0 = disabled.
func NewManager(s *store.Store, windowTurns, summarizeAtTurns int) Manager {
	return &manager{
		store:            s,
		windowTurns:      windowTurns,
		summarizeAtTurns: summarizeAtTurns,
	}
}

func (m *manager) Load(chatID int64) ([]providers.Message, error) {
	rows, err := m.store.GetHistory(chatID, m.windowTurns*2)
	if err != nil {
		return nil, fmt.Errorf("conversation: load: %w", err)
	}
	msgs := make([]providers.Message, len(rows))
	for i, r := range rows {
		msgs[i] = providers.Message{Role: r.Role, Content: r.Content}
	}
	return msgs, nil
}

func (m *manager) Save(chatID int64, role, content string) error {
	if err := m.store.SaveMessage(chatID, role, content); err != nil {
		return fmt.Errorf("conversation: save: %w", err)
	}
	return nil
}

func (m *manager) MaybeSummarize(ctx context.Context, chatID int64, llm providers.LLMProvider) error {
	if m.summarizeAtTurns <= 0 {
		return nil // disabled
	}
	count, err := m.store.CountMessages(chatID)
	if err != nil {
		return fmt.Errorf("conversation: count: %w", err)
	}
	if count < m.summarizeAtTurns {
		return nil // below threshold — fast path
	}

	// Load all rows for summarization.
	rows, err := m.store.GetHistory(chatID, count)
	if err != nil {
		return fmt.Errorf("conversation: load for summarization: %w", err)
	}
	if len(rows) < 4 {
		return nil // too few rows to meaningfully summarize
	}

	// Partition: keep last 4 rows intact; summarize the rest.
	olderRows := rows[:len(rows)-4]
	recentRows := rows[len(rows)-4:]

	// Build summarization prompt.
	var sb strings.Builder
	for _, r := range olderRows {
		sb.WriteString(strings.ToUpper(r.Role[:1]) + r.Role[1:])
		sb.WriteString(": ")
		sb.WriteString(r.Content)
		sb.WriteString("\n")
	}
	prompt := "Summarize the following conversation history concisely, preserving all key facts, " +
		"decisions, preferences, and context. Return only the summary, no commentary.\n\n" +
		sb.String()

	resp, err := llm.Chat(ctx, []providers.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		return fmt.Errorf("conversation: summarize LLM call: %w", err)
	}

	summary := fmt.Sprintf("[Summary as of %s]: %s", time.Now().Format("2006-01-02"), resp.Content)
	newRows := make([]store.HistoryMessage, 0, 1+len(recentRows))
	newRows = append(newRows, store.HistoryMessage{Role: "assistant", Content: summary})
	for _, r := range recentRows {
		newRows = append(newRows, r)
	}

	if err := m.store.ReplaceHistory(chatID, newRows); err != nil {
		return fmt.Errorf("conversation: replace history: %w", err)
	}
	return nil
}
```


- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/conversation/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/conversation/manager.go internal/conversation/manager_test.go
git commit -m "feat(conversation): add ConversationManager with proactive summarization"
```

---

### Task 6: opencode.SubmitTaskWithResult

**Files:**
- Modify: `internal/opencode/service.go`
- Modify: `internal/opencode/service_test.go`

- [ ] **Step 1: Read the existing SubmitTask and consumeSSE implementations**

```bash
grep -n "consumeSSE\|SubmitTask\|Part.Text\|session.status\|idle" \
    /Users/canh/Projects/Claw/gistclaw/internal/opencode/service.go | head -40
```

Understand where `ev.Part.Text` is appended to the Telegram-flush `buf` and where `ev.Status.Type == "idle"` is handled. The accumulator must run **parallel to** the existing `buf` — never replace it.

- [ ] **Step 2: Add SubmitTaskWithResult to the Service interface**

In `internal/opencode/service.go`, add to the `Service` interface:

```go
// SubmitTaskWithResult submits a prompt and blocks until the agent finishes,
// returning the full concatenated output text. Streams output to Telegram normally.
SubmitTaskWithResult(ctx context.Context, chatID int64, prompt string) (string, error)
```

- [ ] **Step 3: Implement SubmitTaskWithResult on serviceImpl**

Locate the `consumeSSE` function (or inline SSE loop inside `SubmitTask`). Add a new method that calls the same logic but with a parallel `strings.Builder` accumulator. The key pattern:

```go
func (s *serviceImpl) SubmitTaskWithResult(ctx context.Context, chatID int64, prompt string) (string, error) {
    var accumulator strings.Builder
    err := s.consumeSSEWithAccumulator(ctx, chatID, prompt, &accumulator)
    return accumulator.String(), err
}
```

If `consumeSSE` is not a separate method but is inlined in `SubmitTask`, extract it first:

```go
func (s *serviceImpl) SubmitTask(ctx context.Context, chatID int64, prompt string) error {
    _, err := s.submitAndAccumulate(ctx, chatID, prompt, nil)
    return err
}

func (s *serviceImpl) SubmitTaskWithResult(ctx context.Context, chatID int64, prompt string) (string, error) {
    return s.submitAndAccumulate(ctx, chatID, prompt, &strings.Builder{})
}

// submitAndAccumulate is the shared implementation.
// acc may be nil (SubmitTask path) or a non-nil Builder (SubmitTaskWithResult path).
func (s *serviceImpl) submitAndAccumulate(ctx context.Context, chatID int64, prompt string, acc *strings.Builder) (string, error) {
    // ... existing SubmitTask logic ...
    // In the SSE loop, wherever ev.Part.Text is appended to buf (the Telegram flush buffer),
    // also append to acc if acc != nil:
    //   if acc != nil { acc.WriteString(ev.Part.Text) }
    // At the return point (after "idle"), if acc != nil: return acc.String(), nil
    // Otherwise: return "", nil
}
```

The exact implementation depends on the current structure of `SubmitTask`. Follow the pattern above — do NOT modify the existing Telegram-streaming logic; only add a parallel accumulator.

- [ ] **Step 4: Write a test for SubmitTaskWithResult**

In `internal/opencode/service_test.go`, add a test that:
1. Starts a mock HTTP server that returns a minimal SSE stream with a few `message.part.updated` events and a final `session.status{type:"idle"}`.
2. Calls `SubmitTaskWithResult`.
3. Asserts the returned string contains the expected text content.

Look at the existing `service_test.go` to understand the mock HTTP server pattern already in use — replicate it.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/opencode/... -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/opencode/service.go internal/opencode/service_test.go
git commit -m "feat(opencode): add SubmitTaskWithResult for agent chaining"
```

---

### Task 7: claudecode.SubmitTaskWithResult

**Files:**
- Modify: `internal/claudecode/service.go`
- Modify: `internal/claudecode/service_test.go`

- [ ] **Step 1: Read the existing SubmitTask stream loop**

```bash
grep -n "ev.Type\|ev.Text\|result\|buf\|accumulator\|Telegram\|SendMessage" \
    /Users/canh/Projects/Claw/gistclaw/internal/claudecode/service.go | head -40
```

Understand how `ev.Type == "text"` events are streamed and where `ev.Type == "result"` signals completion.

- [ ] **Step 2: Add SubmitTaskWithResult to the Service interface**

In `internal/claudecode/service.go`, add to the `Service` interface:

```go
// SubmitTaskWithResult submits a prompt and blocks until the agent finishes,
// returning the full concatenated text output. Streams output to Telegram normally.
SubmitTaskWithResult(ctx context.Context, chatID int64, prompt string) (string, error)
```

- [ ] **Step 3: Implement SubmitTaskWithResult**

Apply the same parallel-accumulator pattern as Task 6:
- Add `var accumulator strings.Builder` alongside the existing `buf`.
- On every `ev.Type == "text"` event: append `ev.Text` to both `buf` (existing) and `accumulator`.
- On `ev.Type == "result"`: return `accumulator.String(), nil`.

Refactor `SubmitTask` to call a shared `submitAndAccumulate(ctx, chatID, prompt, acc *strings.Builder)` if that makes the code cleaner, mirroring Task 6.

- [ ] **Step 4: Write a test for SubmitTaskWithResult**

In `internal/claudecode/service_test.go`, add a test that:
1. Writes a mock stream-json output (lines of JSON).
2. Calls `SubmitTaskWithResult`.
3. Asserts the returned string contains the text from `"text"` events.

Look at the existing test for `SubmitTask` to understand the mock subprocess or pipe pattern.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/claudecode/... -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/claudecode/service.go internal/claudecode/service_test.go
git commit -m "feat(claudecode): add SubmitTaskWithResult for agent chaining"
```

---

## Chunk 4: ToolEngine and All Tool Implementations

### Task 8: ToolEngine — Formal Tool Interface and Registry

**Files:**
- Create: `internal/tools/tool.go`
- Create: `internal/tools/tool_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/tools/tool_test.go`:

```go
package tools_test

import (
	"context"
	"testing"

	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/tools"
)

// echoTool is a minimal Tool implementation for testing.
type echoTool struct{ name string }

func (e *echoTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        e.name,
		Description: "echoes input",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}
}
func (e *echoTool) Execute(_ context.Context, input map[string]any) tools.ToolResult {
	return tools.ToolResult{ForLLM: "echo:" + e.name}
}

func TestToolEngine_RegisterAndDefinitions(t *testing.T) {
	engine := tools.NewToolEngine()
	engine.Register(&echoTool{name: "tool_a"})
	engine.Register(&echoTool{name: "tool_b"})
	defs := engine.Definitions()
	if len(defs) != 2 {
		t.Fatalf("Definitions: got %d, want 2", len(defs))
	}
}

func TestToolEngine_Execute_KnownTool(t *testing.T) {
	engine := tools.NewToolEngine()
	engine.Register(&echoTool{name: "my_tool"})
	result := engine.Execute(context.Background(), "my_tool", nil)
	if result.ForLLM != "echo:my_tool" {
		t.Errorf("Execute: got %q, want %q", result.ForLLM, "echo:my_tool")
	}
}

func TestToolEngine_Execute_UnknownTool(t *testing.T) {
	engine := tools.NewToolEngine()
	result := engine.Execute(context.Background(), "not_registered", nil)
	if result.ForLLM == "" {
		t.Error("Execute unknown tool: ForLLM should not be empty")
	}
}

func TestToolResult_ForUser_EmptyMeansNoUserOutput(t *testing.T) {
	// Document the contract: ForUser="" means send nothing to user (not fall back to ForLLM).
	result := tools.ToolResult{ForLLM: "internal", ForUser: ""}
	if result.ForUser != "" {
		t.Error("ForUser should default to empty")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/tools/... -run "TestToolEngine|TestToolResult" -v
```

Expected: FAIL — `tools.NewToolEngine`, `tools.ToolResult` not found.

- [ ] **Step 3: Implement the Tool interface and ToolEngine**

Create `internal/tools/tool.go`:

```go
// internal/tools/tool.go
package tools

import (
	"context"
	"fmt"

	"github.com/canhta/gistclaw/internal/providers"
)

// Tool is the interface all gateway tools must implement.
type Tool interface {
	// Definition returns the tool metadata and JSON schema for the LLM.
	Definition() providers.Tool

	// Execute runs the tool with the given input (pre-unmarshaled from tc.InputJSON).
	// Must never return an empty ForLLM — use an error message if execution fails.
	Execute(ctx context.Context, input map[string]any) ToolResult
}

// ToolResult carries the output of a tool execution through two channels.
type ToolResult struct {
	// ForLLM is the content returned to the message loop. Never empty.
	ForLLM string

	// ForUser is shown to the user separately if non-empty.
	// Empty string means: send nothing to the user separately (NOT a fallback to ForLLM).
	ForUser string
}

// ToolEngine is a registry of Tools.
// It replaces the switch statement in gateway/service.go.
type ToolEngine struct {
	tools map[string]Tool
}

// NewToolEngine constructs an empty ToolEngine.
func NewToolEngine() *ToolEngine {
	return &ToolEngine{tools: make(map[string]Tool)}
}

// Register adds a Tool to the engine. Panics if a tool with the same name is
// registered twice (programming error, caught at startup).
func (e *ToolEngine) Register(t Tool) {
	name := t.Definition().Name
	if _, exists := e.tools[name]; exists {
		panic(fmt.Sprintf("tools: duplicate registration for %q", name))
	}
	e.tools[name] = t
}

// Definitions returns tool metadata for all registered tools (for the LLM tool list).
func (e *ToolEngine) Definitions() []providers.Tool {
	defs := make([]providers.Tool, 0, len(e.tools))
	for _, t := range e.tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

// Execute dispatches to the registered tool by name.
// name is tc.Name at the call site; input is the pre-unmarshaled arguments.
func (e *ToolEngine) Execute(ctx context.Context, name string, input map[string]any) ToolResult {
	t, ok := e.tools[name]
	if !ok {
		return ToolResult{ForLLM: fmt.Sprintf("unknown tool: %q", name)}
	}
	return t.Execute(ctx, input)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/tools/... -run "TestToolEngine|TestToolResult" -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tools/tool.go internal/tools/tool_test.go
git commit -m "feat(tools): add Tool interface and ToolEngine registry"
```

---

### Task 9: Migrate Existing Tools to ToolEngine

**Files:**
- Create: `internal/tools/web_search_tool.go`
- Create: `internal/tools/web_fetch_tool.go`
- Create: `internal/tools/scheduler_tool.go`
- Create: `internal/tools/mcp_tool.go`

These files wrap existing implementations. No new logic — just adapt existing gateway tool code to the Tool interface. Test by verifying they register and their `Definition()` returns the correct name.

- [ ] **Step 1: Create web_search_tool.go**

```go
// internal/tools/web_search_tool.go
package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/providers"
)

// webSearchTool wraps the existing SearchProvider.
type webSearchTool struct {
	search SearchProvider // existing interface in tools package
}

// NewWebSearchTool constructs the web_search tool.
// search may be nil — the tool will return an error message if called without a provider.
func NewWebSearchTool(search SearchProvider) Tool {
	return &webSearchTool{search: search}
}

func (w *webSearchTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        "web_search",
		Description: "Search the web for current information.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query"},
			},
			"required": []string{"query"},
		},
	}
}

func (w *webSearchTool) Execute(ctx context.Context, input map[string]any) ToolResult {
	if w.search == nil {
		return ToolResult{ForLLM: "web_search is unavailable: no search API key configured"}
	}
	query, _ := input["query"].(string)
	if query == "" {
		return ToolResult{ForLLM: "web_search: query is required"}
	}
	// SearchProvider.Search(ctx, query, count) returns ([]SearchResult, error).
	results, err := w.search.Search(ctx, query, 5)
	if err != nil {
		return ToolResult{ForLLM: fmt.Sprintf("web_search error: %v", err)}
	}
	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s\n", i+1, r.Title, r.URL, r.Snippet))
	}
	return ToolResult{ForLLM: sb.String()}
}
```


- [ ] **Step 2: Create web_fetch_tool.go**

```go
// internal/tools/web_fetch_tool.go
package tools

import (
	"context"
	"fmt"

	"github.com/canhta/gistclaw/internal/providers"
)

type webFetchTool struct{ fetcher WebFetcher }

func NewWebFetchTool(fetcher WebFetcher) Tool {
	return &webFetchTool{fetcher: fetcher}
}

func (w *webFetchTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        "web_fetch",
		Description: "Fetch the content of a URL.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{"type": "string", "description": "URL to fetch"},
			},
			"required": []string{"url"},
		},
	}
}

func (w *webFetchTool) Execute(ctx context.Context, input map[string]any) ToolResult {
	url, _ := input["url"].(string)
	if url == "" {
		return ToolResult{ForLLM: "web_fetch: url is required"}
	}
	content, err := w.fetcher.Fetch(ctx, url)
	if err != nil {
		return ToolResult{ForLLM: fmt.Sprintf("web_fetch error: %v", err)}
	}
	return ToolResult{ForLLM: content}
}
```

> **Note:** Check the existing `WebFetcher` interface in `internal/tools/fetch.go` for the exact method signature.

- [ ] **Step 3: Create scheduler_tool.go**

```go
// internal/tools/scheduler_tool.go
package tools

import (
	"context"
	"fmt"

	"github.com/canhta/gistclaw/internal/agent"
	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/scheduler"
)

// schedulerTool wraps a single scheduler operation as a Tool.
type schedulerTool struct {
	sched  *scheduler.Service
	def    providers.Tool
	execFn func(ctx context.Context, input map[string]any) (string, error)
}

func (s *schedulerTool) Definition() providers.Tool { return s.def }
func (s *schedulerTool) Execute(ctx context.Context, input map[string]any) ToolResult {
	result, err := s.execFn(ctx, input)
	if err != nil {
		return ToolResult{ForLLM: fmt.Sprintf("%s error: %v", s.def.Name, err)}
	}
	return ToolResult{ForLLM: result}
}

// NewSchedulerTools returns Tool instances for all four scheduler operations.
// scheduler.Service has no ExecuteTool method — each tool is dispatched individually.
func NewSchedulerTools(sched *scheduler.Service) []Tool {
	schTools := sched.Tools() // returns []providers.Tool with 4 entries
	result := make([]Tool, 0, len(schTools))
	for _, def := range schTools {
		d := def // capture
		var execFn func(ctx context.Context, input map[string]any) (string, error)
		switch d.Name {
		case "schedule_job":
			execFn = func(_ context.Context, input map[string]any) (string, error) {
				kind, _ := input["kind"].(string)
				target, _ := input["target"].(string)
				prompt, _ := input["prompt"].(string)
				schedule, _ := input["schedule"].(string)
				agentKind, err := agent.KindFromString(target)
				if err != nil {
					return "", fmt.Errorf("invalid target %q: %w", target, err)
				}
				j := scheduler.Job{
					Kind:     kind,
					Target:   agentKind,
					Prompt:   prompt,
					Schedule: schedule,
				}
				if err := sched.CreateJob(j); err != nil {
					return "", err
				}
				return `{"status":"created"}`, nil
			}
		case "list_jobs":
			execFn = func(_ context.Context, _ map[string]any) (string, error) {
				jobs, err := sched.ListJobs()
				if err != nil {
					return "", err
				}
				return scheduler.JobsToJSON(jobs), nil
			}
		case "update_job":
			execFn = func(_ context.Context, input map[string]any) (string, error) {
				id, _ := input["id"].(string)
				if id == "" {
					return "", fmt.Errorf("id is required")
				}
				fields := make(map[string]any)
				for k, v := range input {
					if k != "id" {
						fields[k] = v
					}
				}
				if err := sched.UpdateJob(id, fields); err != nil {
					return "", err
				}
				return `{"status":"updated"}`, nil
			}
		case "delete_job":
			execFn = func(_ context.Context, input map[string]any) (string, error) {
				id, _ := input["id"].(string)
				if id == "" {
					return "", fmt.Errorf("id is required")
				}
				if err := sched.DeleteJob(id); err != nil {
					return "", err
				}
				return `{"status":"deleted"}`, nil
			}
		default:
			execFn = func(_ context.Context, _ map[string]any) (string, error) {
				return "", fmt.Errorf("unknown scheduler tool: %q", d.Name)
			}
		}
		result = append(result, &schedulerTool{sched: sched, def: d, execFn: execFn})
	}
	return result
}
```

- [ ] **Step 4: Create mcp_tool.go**

```go
// internal/tools/mcp_tool.go
package tools

import (
	"context"
	"fmt"

	"github.com/canhta/gistclaw/internal/mcp"
	"github.com/canhta/gistclaw/internal/providers"
)

// mcpTool wraps a single MCP tool discovered from mcp.Manager.
// Tool names are already namespaced as "{serverName}__{toolName}" by the manager.
type mcpTool struct {
	manager mcp.Manager
	def     providers.Tool
}

func (m *mcpTool) Definition() providers.Tool { return m.def }
func (m *mcpTool) Execute(ctx context.Context, input map[string]any) ToolResult {
	// mcp.Manager.CallTool takes (ctx, toolName, input) — 3 args.
	// toolName is the namespaced form already stored in m.def.Name.
	result, err := m.manager.CallTool(ctx, m.def.Name, input)
	if err != nil {
		return ToolResult{ForLLM: fmt.Sprintf("mcp %s error: %v", m.def.Name, err)}
	}
	return ToolResult{ForLLM: result}
}

// NewMCPTools returns Tool instances for all tools discovered from the MCP manager.
// mcp.Manager.GetAllTools() returns []providers.Tool (a flat slice, not a map).
// Tool names are pre-namespaced as "{serverName}__{toolName}".
func NewMCPTools(manager mcp.Manager) []Tool {
	allTools := manager.GetAllTools() // returns []providers.Tool
	result := make([]Tool, 0, len(allTools))
	for _, def := range allTools {
		d := def // capture
		result = append(result, &mcpTool{
			manager: manager,
			def:     d,
		})
	}
	return result
}
```

- [ ] **Step 5: Verify compilation**

```bash
go build ./internal/tools/...
```

Fix any type mismatches with the existing `SearchProvider`, `WebFetcher`, `scheduler.Service`, and `mcp.Manager` APIs.

- [ ] **Step 6: Commit**

```bash
git add internal/tools/web_search_tool.go internal/tools/web_fetch_tool.go \
        internal/tools/scheduler_tool.go internal/tools/mcp_tool.go
git commit -m "feat(tools): migrate web_search, web_fetch, scheduler, mcp to ToolEngine"
```

---

### Task 10: Memory Tools

**Files:**
- Create: `internal/tools/memory_tool.go`

- [ ] **Step 1: Implement remember, note, curate_memory tools**

Create `internal/tools/memory_tool.go`:

```go
// internal/tools/memory_tool.go
package tools

import (
	"context"
	"fmt"
	"os"

	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/providers"
)

// rememberTool appends a fact to MEMORY.md.
type rememberTool struct{ eng memory.Engine }

func NewRememberTool(eng memory.Engine) Tool { return &rememberTool{eng: eng} }
func (t *rememberTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        "remember",
		Description: "Save a fact to long-term memory (MEMORY.md).",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"content": map[string]any{"type": "string"}},
			"required":   []string{"content"},
		},
	}
}
func (t *rememberTool) Execute(_ context.Context, input map[string]any) ToolResult {
	content, _ := input["content"].(string)
	if content == "" {
		return ToolResult{ForLLM: "remember: content is required"}
	}
	if err := t.eng.AppendFact(content); err != nil {
		return ToolResult{ForLLM: fmt.Sprintf("remember error: %v", err)}
	}
	return ToolResult{ForLLM: `{"status":"ok"}`, ForUser: "Remembered."}
}

// noteTool appends an entry to today's notes file.
type noteTool struct{ eng memory.Engine }

func NewNoteTool(eng memory.Engine) Tool { return &noteTool{eng: eng} }
func (t *noteTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        "note",
		Description: "Add an entry to today's daily notes.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"content": map[string]any{"type": "string"}},
			"required":   []string{"content"},
		},
	}
}
func (t *noteTool) Execute(_ context.Context, input map[string]any) ToolResult {
	content, _ := input["content"].(string)
	if content == "" {
		return ToolResult{ForLLM: "note: content is required"}
	}
	if err := t.eng.AppendNote(content); err != nil {
		return ToolResult{ForLLM: fmt.Sprintf("note error: %v", err)}
	}
	return ToolResult{ForLLM: `{"status":"ok"}`, ForUser: "Noted."}
}

// curateMemoryTool reviews and rewrites MEMORY.md via an LLM call.
type curateMemoryTool struct {
	eng memory.Engine
	llm providers.LLMProvider
}

func NewCurateMemoryTool(eng memory.Engine, llm providers.LLMProvider) Tool {
	return &curateMemoryTool{eng: eng, llm: llm}
}
func (t *curateMemoryTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        "curate_memory",
		Description: "Review and rewrite MEMORY.md to remove stale or redundant entries.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}
func (t *curateMemoryTool) Execute(ctx context.Context, _ map[string]any) ToolResult {
	current, err := os.ReadFile(t.eng.MemoryPath())
	if err != nil || len(current) == 0 {
		return ToolResult{ForLLM: "memory is empty; nothing to curate"}
	}
	prompt := fmt.Sprintf(
		"You are a memory curator. Review the following memory entries and rewrite them "+
			"as a concise, deduplicated list of facts. Remove stale or redundant entries. "+
			"Return only the rewritten memory content, no commentary.\n\n%s", current)
	resp, err := t.llm.Chat(ctx, []providers.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		return ToolResult{ForLLM: fmt.Sprintf("curate_memory failed: %v", err)}
	}
	if err := t.eng.Rewrite(resp.Content); err != nil {
		return ToolResult{ForLLM: fmt.Sprintf("curate_memory write error: %v", err)}
	}
	return ToolResult{ForLLM: `{"status":"ok","message":"Memory curated."}`, ForUser: "Memory curated."}
}
```

- [ ] **Step 2: Build to verify**

```bash
go build ./internal/tools/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/tools/memory_tool.go
git commit -m "feat(tools): add remember, note, curate_memory tools"
```

---

### Task 11: Multi-Agent Tools

**Files:**
- Create: `internal/tools/agents_tool.go`
- Create: `internal/tools/agents_tool_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/tools/agents_tool_test.go`:

```go
package tools_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/tools"
)

// mockOrchestrator records calls and returns configured responses.
type mockOrchestrator struct {
	submitErr           error
	submitWithResultOut string
	submitWithResultErr error
	submitCalls         atomic.Int32
	withResultCalls     atomic.Int32
}

func (m *mockOrchestrator) SubmitTask(_ context.Context, _ int64, _ string) error {
	m.submitCalls.Add(1)
	return m.submitErr
}
func (m *mockOrchestrator) SubmitTaskWithResult(_ context.Context, _ int64, _ string) (string, error) {
	m.withResultCalls.Add(1)
	time.Sleep(1 * time.Millisecond) // simulate brief work
	return m.submitWithResultOut, m.submitWithResultErr
}

func TestSpawnAgent_ReturnsImmediately(t *testing.T) {
	oc := &mockOrchestrator{}
	tool := tools.NewSpawnAgentTool(oc, oc, 123, context.Background())
	start := time.Now()
	result := tool.Execute(context.Background(), map[string]any{
		"kind":   "opencode",
		"prompt": "do something",
	})
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("spawn_agent should return immediately, took %v", elapsed)
	}
	if result.ForLLM == "" {
		t.Error("ForLLM should not be empty")
	}
}

func TestRunParallel_DispatchesAll(t *testing.T) {
	oc := &mockOrchestrator{}
	cc := &mockOrchestrator{}
	tool := tools.NewRunParallelTool(oc, cc, 123, context.Background())
	result := tool.Execute(context.Background(), map[string]any{
		"tasks": []any{
			map[string]any{"kind": "opencode", "prompt": "task 1"},
			map[string]any{"kind": "claudecode", "prompt": "task 2"},
		},
	})
	if result.ForLLM == "" {
		t.Error("ForLLM should not be empty")
	}
	// Both goroutines should eventually call SubmitTask.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if oc.submitCalls.Load() >= 1 && cc.submitCalls.Load() >= 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("expected both agents dispatched; oc=%d cc=%d",
		oc.submitCalls.Load(), cc.submitCalls.Load())
}

func TestChainAgents_ThreadsOutput(t *testing.T) {
	oc := &mockOrchestrator{submitWithResultOut: "step1 output"}
	tool := tools.NewChainAgentsTool(oc, oc, 123)
	result := tool.Execute(context.Background(), map[string]any{
		"steps": []any{
			map[string]any{"kind": "opencode", "prompt_template": "start"},
			map[string]any{"kind": "opencode", "prompt_template": "continue with: {{previous_output}}"},
		},
	})
	if !strings.Contains(result.ForLLM, "step1 output") {
		t.Errorf("chain output should contain step1 output: %q", result.ForLLM)
	}
}

func TestChainAgents_AbortOnError(t *testing.T) {
	oc := &mockOrchestrator{submitWithResultErr: errors.New("agent failed")}
	tool := tools.NewChainAgentsTool(oc, oc, 123)
	result := tool.Execute(context.Background(), map[string]any{
		"steps": []any{
			map[string]any{"kind": "opencode", "prompt_template": "start"},
		},
	})
	if !strings.Contains(result.ForLLM, "aborted") && !strings.Contains(result.ForLLM, "error") {
		t.Errorf("chain should report error on step failure: %q", result.ForLLM)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/tools/... -run "TestSpawnAgent|TestRunParallel|TestChainAgents" -v
```

Expected: FAIL — `tools.NewSpawnAgentTool` etc. not found.

- [ ] **Step 3: Implement multi-agent tools**

Create `internal/tools/agents_tool.go`:

```go
// internal/tools/agents_tool.go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/providers"
)

// agentOrchestrator is the minimal interface required by the three agent tools.
// Both opencode.Service and claudecode.Service satisfy this after the Task 6/7 extensions.
type agentOrchestrator interface {
	SubmitTask(ctx context.Context, chatID int64, prompt string) error
	SubmitTaskWithResult(ctx context.Context, chatID int64, prompt string) (string, error)
}

// --- spawn_agent ---

type spawnAgentTool struct {
	oc          agentOrchestrator
	cc          agentOrchestrator
	chatID      int64
	lifetimeCtx context.Context
}

// NewSpawnAgentTool constructs the spawn_agent tool.
// lifetimeCtx should be the service's lifetime context (from gateway.Service.lifetimeCtx).
func NewSpawnAgentTool(oc, cc agentOrchestrator, chatID int64, lifetimeCtx context.Context) Tool {
	return &spawnAgentTool{oc: oc, cc: cc, chatID: chatID, lifetimeCtx: lifetimeCtx}
}

func (t *spawnAgentTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        "spawn_agent",
		Description: "Dispatch a task to an AI coding agent asynchronously. Returns immediately.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"kind":   map[string]any{"type": "string", "enum": []string{"opencode", "claudecode"}},
				"prompt": map[string]any{"type": "string"},
			},
			"required": []string{"kind", "prompt"},
		},
	}
}

func (t *spawnAgentTool) Execute(_ context.Context, input map[string]any) ToolResult {
	kind, _ := input["kind"].(string)
	prompt, _ := input["prompt"].(string)
	if kind == "" || prompt == "" {
		return ToolResult{ForLLM: "spawn_agent: kind and prompt are required"}
	}
	agent := t.agentFor(kind)
	if agent == nil {
		return ToolResult{ForLLM: fmt.Sprintf("spawn_agent: unknown kind %q", kind)}
	}
	go func() {
		_ = agent.SubmitTask(t.lifetimeCtx, t.chatID, prompt)
	}()
	out, _ := json.Marshal(map[string]string{"status": "dispatched", "agent": kind})
	return ToolResult{ForLLM: string(out)}
}

func (t *spawnAgentTool) agentFor(kind string) agentOrchestrator {
	switch kind {
	case "opencode":
		return t.oc
	case "claudecode":
		return t.cc
	}
	return nil
}

// --- run_parallel ---

type runParallelTool struct {
	oc          agentOrchestrator
	cc          agentOrchestrator
	chatID      int64
	lifetimeCtx context.Context
}

func NewRunParallelTool(oc, cc agentOrchestrator, chatID int64, lifetimeCtx context.Context) Tool {
	return &runParallelTool{oc: oc, cc: cc, chatID: chatID, lifetimeCtx: lifetimeCtx}
}

func (t *runParallelTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        "run_parallel",
		Description: "Dispatch multiple tasks to AI coding agents simultaneously.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tasks": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"kind":   map[string]any{"type": "string"},
							"prompt": map[string]any{"type": "string"},
						},
					},
				},
			},
			"required": []string{"tasks"},
		},
	}
}

func (t *runParallelTool) Execute(_ context.Context, input map[string]any) ToolResult {
	tasksRaw, _ := input["tasks"].([]any)
	if len(tasksRaw) == 0 {
		return ToolResult{ForLLM: "run_parallel: tasks array is required"}
	}
	dispatched := 0
	for _, taskRaw := range tasksRaw {
		task, _ := taskRaw.(map[string]any)
		kind, _ := task["kind"].(string)
		prompt, _ := task["prompt"].(string)
		agent := t.agentFor(kind)
		if agent == nil {
			continue
		}
		go func(a agentOrchestrator, p string) {
			_ = a.SubmitTask(t.lifetimeCtx, t.chatID, p)
		}(agent, prompt)
		dispatched++
	}
	out, _ := json.Marshal(map[string]any{"dispatched": dispatched})
	return ToolResult{
		ForLLM:  string(out),
		ForUser: fmt.Sprintf("Dispatched %d tasks in parallel.", dispatched),
	}
}

func (t *runParallelTool) agentFor(kind string) agentOrchestrator {
	switch kind {
	case "opencode":
		return t.oc
	case "claudecode":
		return t.cc
	}
	return nil
}

// --- chain_agents ---

type chainAgentsTool struct {
	oc     agentOrchestrator
	cc     agentOrchestrator
	chatID int64
}

func NewChainAgentsTool(oc, cc agentOrchestrator, chatID int64) Tool {
	return &chainAgentsTool{oc: oc, cc: cc, chatID: chatID}
}

func (t *chainAgentsTool) Definition() providers.Tool {
	return providers.Tool{
		Name:        "chain_agents",
		Description: "Run agents sequentially, passing each step's output to the next.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"steps": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"kind":            map[string]any{"type": "string"},
							"prompt_template": map[string]any{"type": "string"},
						},
					},
				},
			},
			"required": []string{"steps"},
		},
	}
}

func (t *chainAgentsTool) Execute(ctx context.Context, input map[string]any) ToolResult {
	stepsRaw, _ := input["steps"].([]any)
	if len(stepsRaw) == 0 {
		return ToolResult{ForLLM: "chain_agents: steps array is required"}
	}
	previousOutput := ""
	for i, stepRaw := range stepsRaw {
		step, _ := stepRaw.(map[string]any)
		kind, _ := step["kind"].(string)
		tmpl, _ := step["prompt_template"].(string)
		prompt := strings.ReplaceAll(tmpl, "{{previous_output}}", previousOutput)
		agent := t.agentFor(kind)
		if agent == nil {
			return ToolResult{ForLLM: fmt.Sprintf("chain_agents: unknown kind %q at step %d", kind, i)}
		}
		output, err := agent.SubmitTaskWithResult(ctx, t.chatID, prompt)
		if err != nil {
			return ToolResult{ForLLM: fmt.Sprintf("chain_agents: aborted at step %d: %v", i, err)}
		}
		previousOutput = output
	}
	return ToolResult{ForLLM: previousOutput}
}

func (t *chainAgentsTool) agentFor(kind string) agentOrchestrator {
	switch kind {
	case "opencode":
		return t.oc
	case "claudecode":
		return t.cc
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/tools/... -run "TestSpawnAgent|TestRunParallel|TestChainAgents" -v
```

Expected: all PASS.

- [ ] **Step 5: Run all tool tests**

```bash
go test ./internal/tools/... -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tools/agents_tool.go internal/tools/agents_tool_test.go
git commit -m "feat(tools): add spawn_agent, run_parallel, chain_agents multi-agent tools"
```

---

## Chunk 5: Gateway Refactor and App Wiring

### Task 12: Gateway Refactor — Split and Wire New Dependencies

**Files:**
- Create: `internal/gateway/router.go`
- Create: `internal/gateway/loop.go`
- Modify: `internal/gateway/service.go`

This is a behaviour-preserving refactor + new dependency wiring. All functions remain methods on `*Service`. The split is organizational only.

- [ ] **Step 1: Read the full gateway/service.go**

```bash
wc -l /Users/canh/Projects/Claw/gistclaw/internal/gateway/service.go
grep -n "^func " /Users/canh/Projects/Claw/gistclaw/internal/gateway/service.go
```

Identify which functions go to `router.go`, `loop.go`, and stay in `service.go`.

- [ ] **Step 2: Create gateway/router.go**

Move `handle()`, `handleCallback()`, `isAllowed()`, `buildStatus()`, `formatDuration()` to `router.go`:

```go
// internal/gateway/router.go
package gateway

// Move the following functions here verbatim from service.go:
//   func (s *Service) handle(ctx context.Context, msg channel.Message)
//   func (s *Service) handleCallback(ctx context.Context, msg channel.Message)
//   func (s *Service) isAllowed(userID int64) bool
//   func (s *Service) buildStatus() string
//   func formatDuration(d time.Duration) string
// No logic changes.
```

> **Critical — field rename:** The existing `service.go` uses `s.store` for the store field. The new `Service` struct (Step 4 below) renames it to `s.st`. After moving `buildStatus()` and any other function that references `s.store`, do a search-and-replace within the moved code: replace `s.store.` with `s.st.`.

- [ ] **Step 3: Create gateway/loop.go**

Move `handlePlainChat()`, `chatWithRetry()` to `loop.go`. Add `buildToolEngine()`:

```go
// internal/gateway/loop.go
package gateway

// Move here verbatim from service.go:
//   func (s *Service) handlePlainChat(ctx, chatID, text)
//   func (s *Service) chatWithRetry(...)

// Add new method:
func (s *Service) buildToolEngine() *toolpkg.ToolEngine {
    e := toolpkg.NewToolEngine()

    // Web tools.
    e.Register(toolpkg.NewWebSearchTool(s.search))
    e.Register(toolpkg.NewWebFetchTool(s.fetcher))

    // Memory tools.
    e.Register(toolpkg.NewRememberTool(s.memory))
    e.Register(toolpkg.NewNoteTool(s.memory))
    e.Register(toolpkg.NewCurateMemoryTool(s.memory, s.llm))

    // Scheduler tools.
    for _, t := range toolpkg.NewSchedulerTools(s.sched) {
        e.Register(t)
    }

    // MCP tools (dynamic, discovered at build time).
    for _, t := range toolpkg.NewMCPTools(s.mcp) {
        e.Register(t)
    }

    // Agent orchestration tools.
    e.Register(toolpkg.NewSpawnAgentTool(s.opencode, s.claudecode, s.cfg.OperatorChatID(), s.lifetimeCtx))
    e.Register(toolpkg.NewRunParallelTool(s.opencode, s.claudecode, s.cfg.OperatorChatID(), s.lifetimeCtx))
    e.Register(toolpkg.NewChainAgentsTool(s.opencode, s.claudecode, s.cfg.OperatorChatID()))

    return e
}
```

In `handlePlainChat`, replace:
- `buildToolRegistry()` → `s.buildToolEngine()`
- `executeToolWithInput(tc, ...)` → `engine.Execute(ctx, tc.Name, inputMap)`
- `s.store.GetHistory(chatID, s.cfg.Tuning.ConversationWindowTurns*2)` → the conv pattern below
- `s.store.SaveMessage(chatID, "user", text)` → `s.conv.Save(chatID, "user", text)`
- `buildSystemPrompt()` → `s.memory.LoadContext()`

Add the `MaybeSummarize` + `Load` block:

```go
// In handlePlainChat, before loading history:
if err := s.conv.MaybeSummarize(ctx, chatID, s.llm); err != nil {
    log.Warn().Err(err).Msg("gateway: summarization failed, using full history")
}
msgs, err := s.conv.Load(chatID)
```

Add the auto-curation goroutine at the end of `handlePlainChat` (after the assistant reply is sent). Pass `text` and `assistantReply` as goroutine parameters to avoid closure capture issues:

```go
go func(userText, reply string) {
    bgCtx, cancel := context.WithTimeout(s.lifetimeCtx, 10*time.Second)
    defer cancel()
    prompt := fmt.Sprintf(
        "Given this exchange, should anything be remembered long-term?\nUser: %s\nAssistant: %s\n"+
            "Reply ONLY with JSON: {\"remember\":false} or {\"remember\":true,\"kind\":\"fact|note\",\"content\":\"...\"}",
        userText, reply)
    resp, err := s.llm.Chat(bgCtx, []providers.Message{{Role: "user", Content: prompt}}, nil)
    if err != nil {
        return
    }
    var result struct {
        Remember bool   `json:"remember"`
        Kind     string `json:"kind"`
        Content  string `json:"content"`
    }
    if json.Unmarshal([]byte(resp.Content), &result) == nil && result.Remember {
        switch result.Kind {
        case "fact":
            _ = s.memory.AppendFact(result.Content)
        case "note":
            _ = s.memory.AppendNote(result.Content)
        }
    }
}(text, assistantReply)
```

> **Note:** `assistantReply` is the string variable that holds the final LLM response text (the same string sent to the user via Telegram). Ensure it is defined before this goroutine is launched. In the existing `handlePlainChat`, look for where the final assistant message is assembled — capture that string as `assistantReply` and pass it here.

In `sendFinal`, replace `s.store.SaveMessage(chatID, "assistant", content)` with `s.conv.Save(chatID, "assistant", content)`.

- [ ] **Step 4: Update gateway/service.go — Service struct and NewService**

Update the `Service` struct (remove `soul`, `memory *infra.SOULLoader`; add `memory memory.Engine`, `conv conversation.Manager`, `lifetimeCtx`):

```go
type Service struct {
    ch          channel.Channel
    hitl        hitlService
    opencode    ocService
    claudecode  ccService
    llm         providers.LLMProvider
    search      tools.SearchProvider
    fetcher     tools.WebFetcher
    mcp         mcp.Manager
    sched       *scheduler.Service
    st          *store.Store
    guard       *infra.CostGuard
    memory      mempkg.Engine          // NEW (import as mempkg)
    conv        convpkg.Manager        // NEW (import as convpkg)
    startTime   time.Time
    cfg         config.Config
    lifetimeCtx context.Context        // initialized to context.Background() in NewService
}
```

Update `NewService` signature (remove `soul`, `memory *infra.SOULLoader`; add `mem mempkg.Engine`, `conv convpkg.Manager`):

```go
func NewService(
    ch      channel.Channel,
    h       hitlService,
    oc      ocService,
    cc      ccService,
    llm     providers.LLMProvider,
    search  tools.SearchProvider,
    fetcher tools.WebFetcher,
    m       mcp.Manager,
    sched   *scheduler.Service,
    st      *store.Store,
    guard   *infra.CostGuard,
    mem     mempkg.Engine,
    conv    convpkg.Manager,
    startTime time.Time,
    cfg    config.Config,
) *Service {
    return &Service{
        ch: ch, hitl: h, opencode: oc, claudecode: cc, llm: llm,
        search: search, fetcher: fetcher, mcp: m, sched: sched,
        st: st, guard: guard, memory: mem, conv: conv,
        startTime: startTime, cfg: cfg,
        lifetimeCtx: context.Background(), // safe default; overwritten in Run()
    }
}
```

Update `Run()` to store `lifetimeCtx`:

```go
func (s *Service) Run(ctx context.Context) error {
    s.lifetimeCtx = ctx
    // ... rest unchanged ...
}
```

Update the `ocService` and `ccService` local interfaces to include `SubmitTaskWithResult`:

```go
type ocService interface {
    SubmitTask(ctx context.Context, chatID int64, prompt string) error
    SubmitTaskWithResult(ctx context.Context, chatID int64, prompt string) (string, error)
    Stop(ctx context.Context) error
    IsAlive(ctx context.Context) bool
}

type ccService interface {
    SubmitTask(ctx context.Context, chatID int64, prompt string) error
    SubmitTaskWithResult(ctx context.Context, chatID int64, prompt string) (string, error)
    Stop(ctx context.Context) error
    IsAlive(ctx context.Context) bool
}
```

Delete `buildSystemPrompt()` — it is replaced by `s.memory.LoadContext()`.

Delete `buildToolRegistry()` and `executeToolWithInput()` — replaced by `buildToolEngine()` and `engine.Execute()`.

- [ ] **Step 5: Build to verify**

```bash
go build ./internal/gateway/...
```

Fix any import errors. The key imports to add:
```go
import (
    mempkg  "github.com/canhta/gistclaw/internal/memory"
    convpkg "github.com/canhta/gistclaw/internal/conversation"
    toolpkg "github.com/canhta/gistclaw/internal/tools"
)
```

- [ ] **Step 6: Run gateway tests**

```bash
go test ./internal/gateway/... -v
```

Fix any test failures caused by the `NewService` signature change (update test helpers).

- [ ] **Step 7: Commit**

```bash
git add internal/gateway/service.go internal/gateway/router.go internal/gateway/loop.go
git commit -m "refactor(gateway): split into router/loop/service; wire ToolEngine, memory, conversation"
```

---

### Task 13: Update app.go

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Remove soul/memory SOULLoader fields; add memory.Engine + conversation.Manager**

In `App` struct, replace:
```go
// Remove:
soul   *infra.SOULLoader
memory *infra.SOULLoader
```

Add:
```go
mem  memory.Engine
```

In `NewApp`, replace the two `infra.NewSOULLoader` calls:
```go
// Remove:
soul := infra.NewSOULLoader(cfg.SoulPath)
memory := infra.NewSOULLoader(cfg.MemoryPath)

// Add (use mempkg alias since "memory" package is imported as mempkg):
mem := mempkg.NewEngine(cfg.SoulPath, cfg.MemoryPath, cfg.EffectiveMemoryNotesDir())
```

Update `App` struct initialization:
```go
return &App{
    cfg:    cfg,
    store:  s,
    mem:    mem,   // replaces soul + memory
    rawLLM: rawLLM,
    mcp:    mcpManager,
}, nil
```

In `Run()`, construct the `conversation.Manager` and update the `gateway.NewService` call:

```go
// Before gateway construction:
conv := conversation.NewManager(s, cfg.Tuning.ConversationWindowTurns, cfg.Tuning.SummarizeAtTurns)

// Update gateway.NewService call (remove a.soul, a.memory; add a.mem, conv):
gatewaySvc := gateway.NewService(
    ch,
    hitlSvc,
    ocSvc,
    ccSvc,
    trackedLLM,
    search,
    fetcher,
    a.mcp,
    schedSvc,
    s,
    costGuard,
    a.mem,   // memory.Engine (was: a.soul, a.memory)
    conv,    // conversation.Manager (new)
    time.Now(),
    cfg,
)
```

Also remove the `a.soul` pass-through to `opencode.New` and `claudecode.New` if they still use it directly. Check whether those services still need their own soul loader after this refactor — they load SOUL for prompt injection into the OpenCode REST API, which is separate from the gateway's system prompt. They should keep their own `soulLoader` dependency (unchanged).

- [ ] **Step 2: Add imports**

```go
import (
    "github.com/canhta/gistclaw/internal/conversation"
    mempkg "github.com/canhta/gistclaw/internal/memory"
)
```

- [ ] **Step 3: Build the whole project**

```bash
go build ./...
```

Fix any remaining compilation errors.

- [ ] **Step 4: Run all tests**

```bash
go test ./... -v 2>&1 | tail -50
```

Expected: all PASS. Fix any failures.

- [ ] **Step 5: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(app): wire memory.Engine and conversation.Manager into gateway"
```

---

## Final Verification

- [ ] **Build the binary**

```bash
go build -o /tmp/gistclaw ./cmd/gistclaw/
echo "Build: OK"
```

- [ ] **Run all tests**

```bash
go test ./... -count=1
```

Expected: PASS with no failures.

- [ ] **Run with race detector**

```bash
go test -race ./internal/providers/... ./internal/tools/... ./internal/conversation/... ./internal/memory/...
```

Expected: no data races.

- [ ] **Final commit**

```bash
git add -A
git commit -m "feat: complete gistclaw refactor — ProviderRouter, ToolEngine, memory, conversation, multi-agent"
```
