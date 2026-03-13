# GistClaw Complete Refactor — Design Spec

> **Date:** 2026-03-13
> **Status:** Approved
> **Scope:** Internal architecture refactor — reliability, intelligence, multi-agent orchestration.
> Infrastructure (service topology, SQLite, Telegram, HITL, cost tracking) is preserved unchanged.

---

## 1. Goals

| Area | Problem | Solution |
|------|---------|----------|
| Reliability | Single provider — rate limits / downtime stall everything | `ProviderRouter` with fallback chains + per-provider cooldown |
| Reliability | Context overload — old turns silently dropped | `ConversationManager` with proactive LLM summarization |
| Intelligence | Passive memory — LLM only remembers if it explicitly calls a tool | `MemoryEngine` with LLM-directed auto-curation + daily notes |
| Intelligence | Context compression is manual | Auto-trigger summarization at configurable turn threshold |
| Multi-agent | LLM cannot delegate to OpenCode/ClaudeCode | `spawn_agent`, `run_parallel`, `chain_agents` tools |
| Multi-agent | No parallel agent dispatch | Fan-out via goroutines in `run_parallel` |
| Multi-agent | No agent chaining | Sequential pipeline with `{{previous_output}}` injection |
| Extensibility | Tool loop is a switch statement — hard to extend | Formal `Tool` interface + `ToolEngine` registry |

---

## 2. What Stays Unchanged

- Service topology: `errgroup` + 5 services (`gateway`, `opencode`, `claudecode`, `hitl`, `scheduler`)
- Supervisor (`WithRestart`) and restart budgets
- SQLite store and all existing table schemas
- Telegram channel implementation
- HITL system
- Cost tracking (`CostGuard`, `TrackingProvider`)
- MCP integration
- Scheduler
- All existing provider implementations (`copilot`, `openai`, `codex`)
- `/oc` and `/cc` command shortcuts (fast path, bypass LLM)

---

## 3. Package Layout Changes

```
internal/
  providers/
    router.go         ← NEW: ProviderRouter (fallback chains + cooldown)
    router_test.go
    llm.go            ← unchanged
    errors.go         ← unchanged (ClassifyError reused by router)
    copilot/          ← unchanged
    openai/           ← unchanged
    codex/            ← unchanged
    factory/
      factory.go      ← MODIFIED: builds ProviderRouter instead of single provider

  tools/              ← REDESIGNED: formal Tool interface
    tool.go           ← NEW: Tool interface + ToolResult + ToolEngine
    web_search.go     ← MOVED from gateway (wraps existing SearchProvider)
    web_fetch.go      ← MOVED from gateway (wraps existing WebFetcher)
    memory.go         ← NEW: remember, note, curate_memory tools
    scheduler.go      ← MOVED from gateway (wraps sched.Tools())
    mcp.go            ← MOVED from gateway (wraps mcp.Manager)
    agents.go         ← NEW: spawn_agent, run_parallel, chain_agents

  memory/             ← NEW PACKAGE
    engine.go         ← MemoryEngine interface + implementation
    engine_test.go

  conversation/       ← NEW PACKAGE
    manager.go        ← ConversationManager interface + implementation
    manager_test.go

  opencode/
    service.go        ← MODIFIED: add SubmitTaskWithResult to Service interface + serviceImpl

  claudecode/
    service.go        ← MODIFIED: add SubmitTaskWithResult to Service interface + serviceImpl

  gateway/
    service.go        ← MODIFIED: new fields, updated NewService signature
    router.go         ← EXTRACTED: command routing (was inline in handle())
    loop.go           ← EXTRACTED: tool loop + doom-loop guard + iter cap
    loop_test.go
    service_test.go   ← updated
```

---

## 4. Provider Router

### Interface

`ProviderRouter` implements `providers.LLMProvider` — fully transparent to all callers.

```go
// internal/providers/router.go

type ProviderRouter struct {
    providers []LLMProvider
    cooldowns sync.Map      // key: provider name → expiry time.Time
    window    time.Duration // cooldown window on rate limit (default: 5m)
}

func NewProviderRouter(providers []LLMProvider, cooldownWindow time.Duration) *ProviderRouter

// Chat implements LLMProvider. Tries providers in order, skipping cooled-down ones.
func (r *ProviderRouter) Chat(ctx context.Context, msgs []Message, tools []Tool) (*LLMResponse, error)
func (r *ProviderRouter) Name() string // "router(copilot→openai-key→codex-oauth)"
```

### Routing Logic

```
For each provider in order:
  1. If provider is on cooldown → skip
  2. Call provider.Chat()
  3. On success → return response
  4. If errors.Is(err, context.Canceled) OR errors.Is(err, context.DeadlineExceeded) →
       return error immediately (do NOT fall through to ClassifyError; propagate cancellation)
  5. On ErrKindTerminal → return error immediately (bad request, billing)
  6. On ErrKindRateLimit → mark provider on cooldown for window; try next
  7. On ErrKindRetryable → try next provider immediately
If all providers exhausted (all cooled down or all failed) → return last error
```

Step 4 must precede `ClassifyError` because `ClassifyError` maps `context.Canceled` to `ErrKindRetryable`, which would cause the router to iterate through all providers during shutdown rather than propagating cancellation immediately.

### Configuration

The project uses `caarlos0/env` struct tags. Two new env vars are added to `config.Config`:

```go
// New fields added to Config struct:
LLMProviders      []string      `env:"LLM_PROVIDERS"      envSeparator:","`
LLMCooldownWindow time.Duration `env:"LLM_COOLDOWN_WINDOW" envDefault:"5m"`
```

Existing `LLMProvider string` field is kept for backward compatibility.

**Precedence rule:** If `LLM_PROVIDERS` is non-empty, it defines the ordered fallback list and `LLM_PROVIDER` is ignored. If `LLM_PROVIDERS` is empty, the router wraps `LLM_PROVIDER` as a single-entry list (no fallback, identical to current behavior).

**Updated `validate()` rule:** The existing validation block:
```go
if !validProviders[cfg.LLMProvider] { ... }
```
becomes:
```go
// If LLM_PROVIDERS is set, skip the single-provider validation.
if len(cfg.LLMProviders) == 0 {
    if !validProviders[cfg.LLMProvider] {
        errs = append(errs, ...)
    }
    // openai-key check also only applies in single-provider mode
    if cfg.LLMProvider == "openai-key" && cfg.OpenAIAPIKey == "" {
        errs = append(errs, ...)
    }
} else {
    // Validate each entry in LLM_PROVIDERS is a known provider kind.
    for _, p := range cfg.LLMProviders {
        if !validProviders[p] {
            errs = append(errs, fmt.Sprintf("LLM_PROVIDERS: unknown provider %q", p))
        }
    }
    // openai-key check applies if openai-key appears anywhere in the list.
    for _, p := range cfg.LLMProviders {
        if p == "openai-key" && cfg.OpenAIAPIKey == "" {
            errs = append(errs, "OPENAI_API_KEY is required when openai-key is in LLM_PROVIDERS")
            break
        }
    }
}
```

### Cooldown State

In-memory only (`sync.Map` + `time.Time` expiry). Not persisted. No new SQLite tables.

---

## 5. ToolEngine

### Tool Interface

```go
// internal/tools/tool.go

type Tool interface {
    Definition() providers.Tool                                   // name, description, JSON schema
    Execute(ctx context.Context, input map[string]any) ToolResult
}

type ToolResult struct {
    ForLLM  string // content returned to the message loop; never empty
    ForUser string // if non-empty, sent to user; "" means send nothing to user separately
    // Note: "" does NOT fall back to ForLLM for user display.
    // If a tool wants the user to see ForLLM content, copy it to ForUser explicitly.
}
```

### ToolEngine

```go
type ToolEngine struct {
    tools map[string]Tool
}

func NewToolEngine() *ToolEngine
func (e *ToolEngine) Register(t Tool)
func (e *ToolEngine) Definitions() []providers.Tool

// Execute looks up name in the registry and calls Tool.Execute(ctx, input).
// name is the tool name string (tc.Name at the call site).
// input is the caller's pre-unmarshaled map[string]any from tc.InputJSON.
// The caller retains tc for doom-loop tracking (tc.ID, tc.Name); ToolEngine does not need tc.
// Returns ForLLM="unknown tool: <name>" if name is not registered.
func (e *ToolEngine) Execute(ctx context.Context, name string, input map[string]any) ToolResult
```

Gateway call site:
```go
result := engine.Execute(ctx, tc.Name, inputMap)
```

### Tool Implementations

| File | Tools | Notes |
|------|-------|-------|
| `web_search.go` | `web_search` | Wraps `tools.SearchProvider` (existing) |
| `web_fetch.go` | `web_fetch` | Wraps `tools.WebFetcher` (existing) |
| `memory.go` | `remember`, `note`, `curate_memory` | Delegates to `memory.Engine` |
| `scheduler.go` | `schedule_job`, `list_jobs`, `update_job`, `delete_job` | Wraps `scheduler.Service` |
| `mcp.go` | `{server}__{tool}` (dynamic) | Wraps `mcp.Manager.CallTool()` |
| `agents.go` | `spawn_agent`, `run_parallel`, `chain_agents` | Uses `agentOrchestrator` interface |

### Gateway Change

`handlePlainChat` becomes:
```go
engine := s.buildToolEngine()                    // replaces buildToolRegistry()
// ... tool loop ...
result := engine.Execute(ctx, tc.Name, inputMap) // replaces executeToolWithInput switch
```

---

## 6. Multi-Agent Tools

### Service Interface Extensions

`SubmitTaskWithResult` is added to both exported service interfaces and their concrete implementations:

```go
// internal/opencode/service.go — Service interface gains:
SubmitTaskWithResult(ctx context.Context, chatID int64, prompt string) (string, error)

// internal/claudecode/service.go — Service interface gains:
SubmitTaskWithResult(ctx context.Context, chatID int64, prompt string) (string, error)
```

Implementation behavior:

**`opencode.serviceImpl.SubmitTaskWithResult`**: identical to `SubmitTask` (same SSE loop, same Telegram streaming) with one addition: a second `strings.Builder` named `accumulator` that runs **parallel to** the existing Telegram-flush `buf`. Every `ev.Part.Text` value is appended to `accumulator` in addition to `buf`. The existing `buf`-flush-to-Telegram logic is untouched. When `session.status{type:"idle"}` is received, return `accumulator.String()`. Do NOT try to read from `buf` at that point — `buf` has already been flushed and reset.

**`claudecode.serviceImpl.SubmitTaskWithResult`**: identical to `SubmitTask` (same stream-json loop, same Telegram streaming) with one addition: a `strings.Builder` named `accumulator` that appends `ev.Text` for every event where `ev.Type == "text"`, parallel to the existing Telegram-flush `buf`. When `ev.Type == "result"` is received, return `accumulator.String()`. Do NOT try to read from `buf` — it has already been flushed. (`StreamEvent.Type` values per `claudecode/stream.go`: `"text"` carries `ev.Text`; `"result"` is the completion signal; all others are ignored.)

Both methods stream output to Telegram as normal (same as `SubmitTask`) AND return the final output to the caller.

The `gateway.ocService` and `gateway.ccService` interfaces are also extended to include `SubmitTaskWithResult`:

```go
// gateway/service.go
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

### agentOrchestrator Interface

Defined in `internal/tools/agents.go`. Uses only the methods needed by the three tools:

```go
// internal/tools/agents.go

type agentOrchestrator interface {
    SubmitTask(ctx context.Context, chatID int64, prompt string) error
    SubmitTaskWithResult(ctx context.Context, chatID int64, prompt string) (string, error)
}
```

Both `opencode.Service` and `claudecode.Service` (and by extension `gateway.ocService` / `gateway.ccService`) satisfy this interface after the extensions above.

### Tool Construction — Context Injection

The agent tools are constructed in `gateway.buildToolEngine()` with the service lifetime context injected:

```go
// in gateway/service.go
func (s *Service) buildToolEngine() *tools.ToolEngine {
    e := tools.NewToolEngine()
    // ...other tools...
    e.Register(tools.NewSpawnAgentTool(s.opencode, s.claudecode, s.cfg.OperatorChatID(), s.lifetimeCtx))
    e.Register(tools.NewRunParallelTool(s.opencode, s.claudecode, s.cfg.OperatorChatID(), s.lifetimeCtx))
    e.Register(tools.NewChainAgentsTool(s.opencode, s.claudecode, s.cfg.OperatorChatID()))
    return e
}
```

Each tool struct stores the lifetime context at construction:

```go
type spawnAgentTool struct {
    oc          agentOrchestrator
    cc          agentOrchestrator
    chatID      int64
    lifetimeCtx context.Context // service lifetime; used for background goroutines
}
```

### `spawn_agent`

Async fire-and-forget. Returns immediately; the goroutine runs for the full agent duration.

```
Input:  { kind: "opencode"|"claudecode", prompt: string }

Execution:
  Launch goroutine using lifetimeCtx (not request ctx):
    call oc.SubmitTask(lifetimeCtx, chatID, prompt)
    // goroutine errors are logged but not surfaced to caller
  Return immediately:
    ForLLM:  {"task_id":"<uuid>","status":"dispatched","agent":"<kind>"}
    ForUser: "" (agent streams its own output; nothing extra sent to user)
```

### `run_parallel`

Fan-out. Launches one goroutine per task using `lifetimeCtx`. Waits for all `SubmitTask` calls to return (each `SubmitTask` is synchronous and returns when the agent subprocess completes, which can take minutes). The LLM receives the dispatched-count response BEFORE all agents finish, because the goroutines are not joined before returning.

Concretely: goroutines are launched with `go func() { oc.SubmitTask(...) }()` and the tool returns immediately with "dispatched N tasks". The goroutines complete independently in the background. This is identical to `spawn_agent` but accepts a batch.

```
Input:  { tasks: [{ kind: string, prompt: string }, ...] }

Execution:
  For each task, launch a goroutine with lifetimeCtx calling SubmitTask().
  Goroutines run to completion in the background; not waited on.
  Return immediately:
    ForLLM:  {"dispatched":N,"agents":[{"kind":"...","prompt":"..."},...]}
    ForUser: "Dispatched N tasks in parallel."
```

**Bound:** goroutines are bounded by the number of tasks in the input array. No semaphore needed for typical usage (N is small — the LLM won't generate 100 tasks in one call).

### `chain_agents`

Sequential pipeline. Each step blocks using `SubmitTaskWithResult` until the agent finishes.

```
Input:  { steps: [{ kind: string, prompt_template: string }, ...] }
  "{{previous_output}}" in prompt_template is replaced with the prior step's output.
  First step: "{{previous_output}}" is replaced with "" (empty string).

Execution:
  Uses the request context (ctx from Execute call) — if ctx is cancelled mid-chain, the
  current SubmitTaskWithResult call is cancelled and the chain aborts.
  previousOutput := ""
  For each step i:
    prompt := strings.ReplaceAll(step.PromptTemplate, "{{previous_output}}", previousOutput)
    output, err := agent.SubmitTaskWithResult(ctx, chatID, prompt)
    If err: return ToolResult{ForLLM: fmt.Sprintf("chain aborted at step %d: %s", i, err)}
    previousOutput = output
  Return:
    ForLLM:  final step's output text
    ForUser: "" (each agent streams its own output per-step)
```

---

## 7. Memory Engine

### Package

```go
// internal/memory/engine.go

type Engine interface {
    // LoadContext returns the full system prompt injection using the same joining strategy
    // as the existing buildSystemPrompt: non-empty parts are collected and joined with "\n\n"
    // (matching strings.Join(parts, "\n\n") — consistent two-newline separator regardless
    // of trailing whitespace in individual parts).
    //
    // Parts (each omitted if empty/missing):
    //   1. SOUL file content
    //   2. "# Memory\n\n" + MEMORY.md content
    //   3. "# Today's Notes\n\n" + today's notes content (capped at 8000 bytes; tail kept)
    LoadContext() string

    AppendFact(content string) error  // append to MEMORY.md with timestamp prefix
    AppendNote(content string) error  // append to notes/YYYY-MM-DD.md with timestamp prefix
    Rewrite(content string) error     // replace full MEMORY.md
    TodayNotes() (string, error)
    MemoryPath() string               // path to MEMORY.md (used by curate_memory tool)
}

// NewEngine constructs the memory engine.
//   soulPath:   path to SOUL.md; "" disables SOUL loading.
//   memoryPath: path to MEMORY.md (e.g. ~/.gistclaw/memory/MEMORY.md).
//   notesDir:   directory for date-partitioned notes (e.g. ~/.gistclaw/memory/notes).
//               If empty, defaults to filepath.Join(filepath.Dir(memoryPath), "notes").
func NewEngine(soulPath, memoryPath, notesDir string) Engine
```

`memory.Engine` owns both SOUL and MEMORY.md loading, replacing the two `*infra.SOULLoader` fields (`soul` and `memory`) currently in `gateway.Service`. The `infra.SOULLoader` type is still used internally by the engine for mtime-cached reads; it is not removed from `internal/infra`.

### Storage Layout

```
filepath.Dir(memoryPath)/
  MEMORY.md           ← curated facts
  notes/
    2026-03-13.md     ← today's notes (append-only)
    2026-03-12.md
    ...
```

New env vars in `config.Config`:
```go
MemoryNotesDir string `env:"MEMORY_NOTES_DIR"` // default: derived as filepath.Join(filepath.Dir(cfg.MemoryPath), "notes")
```

### LLM-Directed Auto-Curation

After every plain-chat response, gateway fires a non-blocking background goroutine.

`s.lifetimeCtx` is the context passed to `gateway.Service.Run()`, stored as a field at the top of `Run`. It is cancelled only when the service shuts down, so background goroutines outlive a single request but not the process.

```go
go func() {
    // 10-second timeout derived from service lifetime context.
    bgCtx, cancel := context.WithTimeout(s.lifetimeCtx, 10*time.Second)
    defer cancel()

    prompt := buildCurationPrompt(userMsg, assistantReply)
    resp, err := s.llm.Chat(bgCtx, []providers.Message{{Role: "user", Content: prompt}}, nil)
    if err != nil {
        return // silent failure — memory is best-effort, no retry
    }
    var result struct {
        Remember bool   `json:"remember"`
        Kind     string `json:"kind"`    // "fact" or "note"
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
}()
```

Curation prompt (kept under 200 tokens):
```
Given this exchange, should anything be remembered long-term?
User: <msg>
Assistant: <reply>
Reply ONLY with JSON: {"remember":false} or {"remember":true,"kind":"fact|note","content":"..."}
```

### Tools

| Tool | Replaces | Behavior |
|------|---------|---------|
| `remember(content)` | `update_memory` | Calls `memory.AppendFact(content)` |
| `note(content)` | — | Calls `memory.AppendNote(content)` |
| `curate_memory()` | `clear_memory` | See below |

**`curate_memory()` tool — `Definition()` schema:**

```go
providers.Tool{
    Name:        "curate_memory",
    Description: "Review and rewrite MEMORY.md to remove stale or redundant entries.",
    InputSchema: map[string]any{
        "type":       "object",
        "properties": map[string]any{},
    },
}
```

**`curate_memory()` tool execution:**

Makes a synchronous LLM call inside the tool loop using `s.llm` (same provider as the outer chat loop). This nested call counts against cost tracking. It does NOT count against `MaxIterations`. No retry is applied — transient failures surface as a tool error string returned in `ForLLM`; the outer LLM can decide whether to retry.

```go
func (t *curateMemoryTool) Execute(ctx context.Context, _ map[string]any) tools.ToolResult {
    current, err := os.ReadFile(t.engine.MemoryPath())
    if err != nil || len(current) == 0 {
        return tools.ToolResult{ForLLM: "memory is empty; nothing to curate"}
    }
    prompt := fmt.Sprintf(
        "You are a memory curator. Review the following memory entries and rewrite them "+
        "as a concise, deduplicated list of facts. Remove stale or redundant entries. "+
        "Return only the rewritten memory content, no commentary.\n\n%s", current)
    resp, err := t.llm.Chat(ctx, []providers.Message{{Role: "user", Content: prompt}}, nil)
    if err != nil {
        return tools.ToolResult{ForLLM: "curate_memory failed: " + err.Error()}
    }
    if err := t.engine.Rewrite(resp.Content); err != nil {
        return tools.ToolResult{ForLLM: "curate_memory write error: " + err.Error()}
    }
    return tools.ToolResult{
        ForLLM:  `{"status":"ok","message":"Memory curated and rewritten."}`,
        ForUser: "Memory curated.",
    }
}
```

### System Prompt Injection

`buildSystemPrompt()` in gateway is deleted. Its call sites use `s.memory.LoadContext()` directly. The two `*infra.SOULLoader` fields in `gateway.Service` are removed.

---

## 8. Conversation Manager

### Package

```go
// internal/conversation/manager.go

type Manager interface {
    // Load returns history for chatID capped at windowTurns*2 rows
    // (windowTurns is the turn count; each turn = 2 rows; the *2 multiplier is applied
    // internally, consistent with the current store.GetHistory(chatID, windowTurns*2) call).
    // When summarize_at_turns = 0: MaybeSummarize is always a no-op; Load behaves
    // identically to store.GetHistory(chatID, windowTurns*2).
    Load(chatID int64) ([]providers.Message, error)

    // Save stores one message row. Replaces direct store.SaveMessage calls.
    // Delegates to store.Store.SaveMessage — no direct SQL.
    Save(chatID int64, role, content string) error

    // MaybeSummarize is synchronous.
    // Fast path (below threshold or disabled): returns nil in microseconds, no I/O.
    // Slow path (at or above threshold): makes one LLM call (~1-3s) then rewrites history.
    // Callers should expect and accept the latency on threshold crossings.
    // If summarize_at_turns = 0, always returns nil immediately.
    MaybeSummarize(ctx context.Context, chatID int64, llm providers.LLMProvider) error
}

// NewManager constructs the manager.
// windowTurns: the ConversationWindowTurns tuning value (rows fetched = windowTurns*2).
// summarizeAtTurns: trigger threshold in rows; 0 = disabled.
func NewManager(store *store.Store, windowTurns, summarizeAtTurns int) Manager
```

### Summarization Logic

```
MaybeSummarize(ctx, chatID, llm):
  1. SELECT COUNT(*) FROM messages WHERE chat_id = chatID
  2. If count < summarizeAtTurns OR summarizeAtTurns == 0 → return nil (fast path)
  3. Load ALL rows for chatID (no cap)
  4. Partition: olderRows = rows[0..len-4], recentRows = rows[len-4..len]
     (if total rows < 4, olderRows is empty; nothing to summarize — return nil)
  5. Format olderRows as "Role: Content\n" lines for the prompt
  6. Call llm.Chat(ctx, [{role:"user", content: summarizationPrompt}], nil)
  7. Call store.Store.ReplaceHistory(chatID, rows) where rows = [syntheticSummaryRow] + recentRows.
     ReplaceHistory is a new method on store.Store (see §8 Store Extension below).
     It executes in a single transaction: DELETE + INSERT. On error, transaction rolls back and
     history is unchanged. Timestamps on reconstituted rows are new (original timestamps lost —
     known and acceptable behavioral change).
  8. Return nil
```

On summarization error: return error to caller. Caller (gateway) logs warn and continues with full history — summarization failure is non-fatal.

### Store Extension

`store.Store` gains two new methods. Both keep all SQL in the store layer. This keeps all SQL in the store layer and avoids exposing `db *sql.DB` to the conversation package:

```go
// CountMessages returns the number of message rows for chatID.
func (s *Store) CountMessages(chatID int64) (int, error) {
    var count int
    err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE chat_id = ?`, chatID).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("store: count messages: %w", err)
    }
    return count, nil
}

// ReplaceHistory deletes all message rows for chatID and inserts the provided rows
// in a single transaction. rows is ordered oldest-first.
func (s *Store) ReplaceHistory(chatID int64, rows []HistoryMessage) error {
    tx, err := s.db.Begin()
    if err != nil { return fmt.Errorf("store: replace history: begin: %w", err) }
    defer tx.Rollback() // no-op if committed

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

`conversation.NewManager` receives `*store.Store` and calls exactly four store methods — no direct SQL in the conversation package:
- `store.GetHistory(chatID, limit)` — existing
- `store.SaveMessage(chatID, role, content)` — existing
- `store.CountMessages(chatID)` — **new** (see Store Extension)
- `store.ReplaceHistory(chatID, rows)` — **new** (see Store Extension)

Summarization prompt:
```
Summarize the following conversation history concisely, preserving all key facts,
decisions, preferences, and context. Return only the summary, no commentary.

<formatted older rows>
```

### Configuration

New field in `config.Tuning`:
```go
SummarizeAtTurns int `env:"TUNING_SUMMARIZE_AT_TURNS" envDefault:"0"` // 0 = disabled
```

### Gateway Change

Three `store` call sites are replaced with `conv` equivalents:

```go
// 1. History fetch — in handlePlainChat (was: store.GetHistory)
//    before:
history, err := s.store.GetHistory(chatID, s.cfg.Tuning.ConversationWindowTurns*2)
//    after:
if err := s.conv.MaybeSummarize(ctx, chatID, s.llm); err != nil {
    log.Warn().Err(err).Msg("gateway: summarization failed, using full history")
}
msgs, err := s.conv.Load(chatID)

// 2. User message persist — in handlePlainChat (was: store.SaveMessage line 234)
//    before:
if err := s.store.SaveMessage(chatID, "user", text); err != nil { ... }
//    after:
if err := s.conv.Save(chatID, "user", text); err != nil { ... }

// 3. Assistant message persist — in sendFinal (was: store.SaveMessage line 358)
//    before:
if err := s.store.SaveMessage(chatID, "assistant", content); err != nil { ... }
//    after:
if err := s.conv.Save(chatID, "assistant", content); err != nil { ... }
```

The `s.store` field is still present on `Service` (used by other methods like `ListPendingHITL`). Only the three conversation-history call sites move to `s.conv`.

---

## 9. Gateway Refactor

### File Split

`gateway/service.go` is split into three files. All functions remain methods on `*Service` — the split is organizational only.

**`gateway/router.go`** — `handle()`, `handleCallback()`, `isAllowed()`, `buildStatus()`, `formatDuration()`. No LLM calls, no tool logic.

**`gateway/loop.go`** — `handlePlainChat()`, `chatWithRetry()`, `buildToolEngine()`. All methods on `*Service`, retaining full access to service fields (`s.ch`, `s.cfg`, `s.llm`, etc.). `chatWithRetry` is unchanged.

**`gateway/service.go`** — `Service` struct, `NewService`, `Run`. The `buildSystemPrompt` method is deleted; its callers use `s.memory.LoadContext()` directly.

### Updated `Service` Struct

```go
type Service struct {
    ch         channel.Channel
    hitl       hitlService
    opencode   ocService
    claudecode ccService
    llm        providers.LLMProvider
    search     tools.SearchProvider
    fetcher    tools.WebFetcher
    mcp        mcp.Manager
    sched      *scheduler.Service
    st         *store.Store
    guard      *infra.CostGuard
    memory     memory.Engine          // REPLACES: soul *infra.SOULLoader + memory *infra.SOULLoader
    conv       conversation.Manager   // NEW
    startTime  time.Time
    cfg        config.Config
    lifetimeCtx context.Context       // initialized to context.Background() in NewService;
                                      // overwritten with the real ctx at top of Run()
}
```

**Nil safety:** `NewService` initializes `lifetimeCtx` to `context.Background()`:
```go
func NewService(...) *Service {
    return &Service{
        // ... other fields ...
        lifetimeCtx: context.Background(), // safe default for tests that don't call Run()
    }
}
```
`Run()` then overwrites it with the real lifetime context before the message loop starts. Tests that call `buildToolEngine` or `handlePlainChat` without calling `Run()` will get `context.Background()` goroutines rather than a nil-context panic.

### Updated `NewService` Signature

```go
func NewService(
    ch         channel.Channel,
    h          hitlService,
    oc         ocService,
    cc         ccService,
    llm        providers.LLMProvider,
    search     tools.SearchProvider,
    fetcher    tools.WebFetcher,
    m          mcp.Manager,
    sched      *scheduler.Service,
    st         *store.Store,
    guard      *infra.CostGuard,
    memory     memory.Engine,         // REPLACES: soul *infra.SOULLoader + memory *infra.SOULLoader
    conv       conversation.Manager,  // NEW
    startTime  time.Time,
    cfg        config.Config,
) *Service
```

Net: 15 → 15 parameters. Two removed (`soul *infra.SOULLoader`, `memory *infra.SOULLoader`), two added (`memory.Engine`, `conversation.Manager`).

`app.Run` constructs both before calling `NewService`:
```go
mem := memory.NewEngine(cfg.SoulPath, cfg.MemoryPath, cfg.MemoryNotesDir)
conv := conversation.NewManager(st, cfg.Tuning.ConversationWindowTurns, cfg.Tuning.SummarizeAtTurns)
gw := gateway.NewService(..., mem, conv, ...)
```

`lifetimeCtx` is stored at the top of `Run()`:
```go
func (s *Service) Run(ctx context.Context) error {
    s.lifetimeCtx = ctx
    // ...
}
```

**Memory model safety:** `lifetimeCtx` is written once at the top of `Run()` before the message-handling loop starts. All goroutines that read `lifetimeCtx` (auto-curation, `spawn_agent`, `run_parallel`) are spawned from within the loop body, which executes after the write. This satisfies the Go memory model's happens-before guarantee: the write to `s.lifetimeCtx` happens before any goroutine that reads it is created. No mutex is needed.

---

## 10. Error Handling

| Scenario | Behavior |
|----------|---------|
| ProviderRouter: all providers exhausted or cooled down | Return last error to `chatWithRetry`; existing retry/backoff applies |
| Summarization failure | Log warn; continue with full history (non-fatal) |
| Memory auto-curation failure | Goroutine discards error silently; best-effort, no retry |
| `curate_memory` nested LLM call failure | Return error string in `ForLLM`; MEMORY.md unchanged; no retry applied |
| `spawn_agent` dispatch failure | `ForLLM` = error string; LLM can report to user or retry |
| `run_parallel` goroutine failure | Goroutine logs error; tool already returned "dispatched" — failure is visible in agent's Telegram output |
| `chain_agents` step failure | Abort chain; return `ForLLM = "chain aborted at step N: <err>"` |

---

## 11. Testing

| Package | Test approach |
|---------|--------------|
| `providers/router.go` | Mock `LLMProvider`s; test fallback order, cooldown expiry, terminal error bypass, all-cooled-down exhaustion |
| `tools/tool.go` | Unit test `ToolEngine.Register`, `Definitions`, `Execute(name, input)` routing |
| `tools/agents.go` | Mock `agentOrchestrator`; verify goroutine launch for `spawn_agent`/`run_parallel`; verify output threading in `chain_agents` |
| `memory/engine.go` | Temp dir; verify MEMORY.md and notes files written; LoadContext composition; 8000-byte notes truncation |
| `conversation/manager.go` | In-memory SQLite; verify Load uses windowTurns*2; summarization triggers at threshold; no-op when disabled |
| `gateway/loop.go` | Mock `ToolEngine`; verify doom-loop and MaxIterations guards |
| `config/config.go` | Verify LLM_PROVIDERS precedence; validate() skips single-provider check when LLM_PROVIDERS set |

Existing tests for `opencode`, `claudecode`, `hitl`, `scheduler`, `store` — untouched except for adding `SubmitTaskWithResult` to mocks.

---

## 12. Migration Path

**Dependency ordering:** Step 6 (adding `SubmitTaskWithResult` to service interfaces) must be completed before `chain_agents` in step 2 is fully wired up. Either complete step 6 first, or stub `chain_agents.Execute` with `return ToolResult{ForLLM: "not yet implemented"}` until step 6 is done.

Steps 1, 3, and 4 have no inter-dependencies and can be done in parallel sessions. Step 2 has a soft dependency on Step 6 (resolved via stub as above).

1. **`internal/providers/router.go`** — additive; `factory.go` updated last
2. **`internal/tools/` refactor** — gateway switch replaced; `chain_agents` depends on step 6
3. **`internal/memory/`** — new package; gateway updated (removes two SOULLoader fields)
4. **`internal/conversation/`** — new package; gateway updated
5. **`internal/gateway/` split + `NewService` update** — behavior-preserving; update `app.Run`
6. **Service interface extension** — add `SubmitTaskWithResult` to `opencode.Service`, `claudecode.Service`, `gateway.ocService`, `gateway.ccService` + implement on `serviceImpl` in both packages. `app.go` requires **no changes**: it passes `*opencode.serviceImpl` and `*claudecode.claudecodeServiceImpl` as the concrete types, which automatically satisfy the expanded `gateway.ocService`/`gateway.ccService` interfaces once the `serviceImpl` types implement the new method.
7. **`config.go` update** — add new fields, update `validate()`
