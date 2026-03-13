# GistClaw Architecture

This document describes the internal structure of GistClaw: how services are arranged,
how they communicate, and the contracts that keep them decoupled.

---

## 1. Service Topology

GistClaw is a single Go binary. Five services run as goroutines under `golang.org/x/sync/errgroup`.
Everything else is plain Go called synchronously. No actor framework, no protobuf.

```
systemd (Restart=always, StartLimitBurst=5)
    └── gistclaw binary
            │
      errgroup (root context)   ← owned by internal/app.App.Run()
            │
      ┌─────┴──────────────────────────────────────────┐
      │                                                 │
  withRestart("gateway", unlimited)        withRestart("hitl", 10, 10s)
  - channel.Channel interface              - permission + question HITL
  - v1: Telegram long-poll impl            - per-request channel pattern
  - update_id dedup (SQLite)               - 5-min timer + 3-min reminder
  - /oc /cc /status /stop routing          - sequential per-question
  - plain chat → trackedLLM               - auto-reject stale on startup
    (web_search + web_fetch
     + MCP tools [Agent category]
     + scheduler tools [System category])
        │ injected interfaces
        │ (channel.Channel, hitl.Approver,
        │  opencode.Service, claudecode.Service,
        │  providers.LLMProvider [decorated])
        └────────────────────────────────────────────────┐
                                                         │
                              ┌──────────────────────────┤
                              │                          │
               withRestart("opencode", 5, 30s) withRestart("claudecode", 5, 30s)
               - start opencode serve           - start claude -p subprocess
               - HTTP client                    - stream-json parser
               - SSE consumer                   - FSM: Idle→Running
               - session lifecycle                →WaitingInput→Idle
               - no FSM needed                  - hook helper: gistclaw-hook
               - CostGuard.Track() on           - HTTP server :8765
                 step-finish events             - CostGuard.Track() on
                                                  total_cost_usd events
                              │
                  withRestart("scheduler", 5, 30s)
                  - 1s ticker
                  - job kinds: at/every/cron
                  - targets: agent.Kind enum
                  - SQLite jobs table
                  - fires via scheduler.JobTarget interface
                              │
               ┌──────────────┴─────────────────────┐
           SQLiteStore                          internal/infra
           (not a service loop)                 - CostGuard (atomic.Int64)
           - sessions                           - SOULLoader (mtime cache)
           - hitl_pending                       - Heartbeat (Tier 1+2)
           - cost_daily                         internal/providers
           - channel_state                      - LLMProvider interface
           - jobs                               - trackingProvider decorator
                                                - copilot / codex / openai impls
                                                internal/tools
                                                - SearchProvider (Brave/Gemini/
                                                  Grok/Perplexity — auto-detect)
                                                - WebFetcher (go-readability)
                                                internal/mcp
                                                - MCPManager (stdio + SSE/HTTP)
```

---

## 2. Main Flows

### Flow A — `/oc build the auth module`

1. Telegram update arrives at `gateway.Service`
2. `update_id` dedup check against `channel_state` in SQLite
3. Command prefix `/oc` → `opencode.Service.SubmitTask(ctx, chatID, prompt)`
4. OpenCode service POSTs to `opencode serve` REST API (reuses or creates session)
5. SSE consumer receives `message.part.updated` events → `channel.SendMessage` in chunks
6. `CostGuard.Track()` called on each `step-finish` event
7. `session.status {type:"idle"}` → `channel.SendMessage(chatID, "✅ Done")`

### Flow B — OpenCode permission request mid-task

1. SSE fires `permission.asked` event
2. `opencode.Service` calls `hitl.Approver.RequestPermission(ctx, req)`
3. `hitl.Service` writes `hitl_pending` to SQLite; sends inline keyboard via `channel.SendKeyboard`
4. Timer: reminder at `HITLReminderBefore` before timeout; auto-reject at `HITLTimeout`
5. User taps button → gateway routes callback → `hitl.Service` resolves → POST reply to OpenCode

### Flow C — Claude Code tool approval via hook helper

1. `claude` subprocess spawns `gistclaw-hook`; hook writes `PreToolUse` JSON to its stdin
2. `gistclaw-hook` POSTs to `http://127.0.0.1:8765/hook/pretool`
3. `claudecode.Service` hook handler calls `hitl.Approver.RequestPermission` and blocks
4. Same HITL flow as Flow B
5. Handler writes allow/deny JSON; `gistclaw-hook` exits with code 0 or 2

### Flow D — Zero output from agent

1. OpenCode session goes idle with empty output buffer
2. `opencode.Service` sends: `"⚠️ Agent finished but produced no output. Check the session or resend your prompt."`
3. No auto-retry; no crash; user decides next action

### Flow E — Plain chat with tool loop

1. No command prefix → `gateway.Service` builds tool registry (Core + Agent + System tools)
2. Calls `trackedLLM.Chat(ctx, messages, tools)` in a loop
3. LLM may call `web_search` or `web_fetch` (doom-loop guard: 3 identical calls → forced final answer)
4. Final text answer → `channel.SendMessage`; cost tracked automatically by decorator

### Flow F — Plain chat without tool use

1. No command prefix → `gateway.Service` builds tool registry
2. Calls `trackedLLM.Chat(ctx, messages, tools)`
3. LLM decides NOT to call any tool — returns final text directly
4. `channel.SendMessage(chatID, answer)`; cost tracked automatically by decorator

### Flow G — `question.asked` with multiple questions

1. SSE fires `question.asked` with `questions` array (e.g. two questions)
2. `opencode.Service` calls `hitl.Approver.RequestQuestion(ctx, QuestionRequest{ChatID, ID, Questions})`
3. `hitl.Service` processes questions **sequentially**:
   - Sends question[0] keyboard; waits for user reply
   - Records answer (e.g. `["testify"]`)
   - Sends question[1] keyboard; waits for user reply
   - Records answer (e.g. `["yes"]`)
4. `hitl.Service` sends a **single** POST reply: `POST /question/:id/reply {"answers":[["testify"],["yes"]]}`
5. Agent continues with all answers at once

---

## 3. Interface Contracts

### `channel.Channel` (`internal/channel/channel.go`)

```go
type Channel interface {
    Receive(ctx context.Context) (<-chan InboundMessage, error)
    SendMessage(ctx context.Context, chatID int64, text string) error
    SendKeyboard(ctx context.Context, chatID int64, payload KeyboardPayload) error
    SendTyping(ctx context.Context, chatID int64) error
    Name() string // "telegram", "whatsapp", etc.
}
```

`gateway.Service` only imports `channel.Channel`. Platform-specific types (e.g., `telego`) are
confined to `internal/channel/telegram`. `hitl/keyboard.go` imports only `internal/channel` —
no Telegram dependency in HITL.

### `providers.LLMProvider` (`internal/providers/llm.go`)

```go
type LLMProvider interface {
    Chat(ctx context.Context, messages []Message, tools []Tool) (*LLMResponse, error)
    Name() string
}

type Usage struct {
    PromptTokens     int
    CompletionTokens int
    TotalCostUSD     float64 // required; 0 if provider cannot determine cost
}
```

All three providers (`openai-key`, `copilot`, `codex-oauth`) implement this interface.
`NewTrackingProvider` wraps any `LLMProvider` and calls `CostGuard.Track` on every
successful response. Gateway and scheduler only see the decorated interface.

### `hitl.Approver` (`internal/hitl/service.go`)

```go
type Approver interface {
    RequestPermission(ctx context.Context, req PermissionRequest) error
    RequestQuestion(ctx context.Context, req QuestionRequest) error
}
```

OpenCode and ClaudeCode services call this interface. HITL internals (SQLite, keyboard,
timers) are hidden behind it.

### `opencode.Service` (`internal/opencode/service.go`)

```go
type Service interface {
    Run(ctx context.Context) error
    SubmitTask(ctx context.Context, chatID int64, prompt string) error
    Stop(ctx context.Context) error
    IsAlive(ctx context.Context) bool
}
```

### `claudecode.Service` (`internal/claudecode/service.go`)

```go
type Service interface {
    Run(ctx context.Context) error
    SubmitTask(ctx context.Context, chatID int64, prompt string) error
    Stop(ctx context.Context) error
    IsAlive(ctx context.Context) bool
}
```

`claudecode.Service` additionally owns the HTTP server at `:8765` for `gistclaw-hook`
communication. The `IsAlive` implementation checks FSM state (`Idle`, `Running`, or
`WaitingInput`) — not `os.FindProcess`.

### `scheduler.JobTarget` (`internal/scheduler/service.go`)

```go
type JobTarget interface {
    RunAgentTask(ctx context.Context, kind agent.Kind, prompt string) error
    SendChat(ctx context.Context, chatID int64, text string) error
}
```

`JobTarget` is wired in `app.NewApp` — `scheduler.Service` does not import `gateway` or
any concrete agent service. `agent.Kind` is a typed enum; no stringly-typed agent identifiers.

### `AgentHealthChecker` (`internal/infra/heartbeat.go`)

```go
type AgentHealthChecker interface {
    Name() string
    IsAlive(ctx context.Context) bool
}
```

Both `opencode.Service` and `claudecode.Service` implement this. `infra.Heartbeat` Tier 2
calls it every 5 minutes and triggers a restart if the agent is dead.

---

## 4. Supervision Model

`withRestart` (`internal/app/supervisor.go`) wraps any `func(context.Context) error` in a
restart loop. `PermanentFailure` is returned when the service exhausts its restart budget.
`app.Run` owns the policy: which services are critical (cancel the root context on failure)
and which are non-critical (log + notify operator, keep running).

### Supervision strategies

| Service | Max attempts | Window | On permanent stop |
|---|---|---|---|
| `gateway.Service` | unlimited | — | Returns `PermanentFailure` → errgroup cancels root context → systemd restarts all |
| `opencode.Service` | 5 | 30s | Log + notify operator; other services continue |
| `claudecode.Service` | 5 | 30s | Log + notify operator; other services continue |
| `hitl.Service` | 10 | 10s | `DrainPending()` first (unblocks hook handlers); then log + notify |
| `scheduler.Service` | 5 | 30s | Log + notify; SQLite jobs resume on next restart |

### Critical vs non-critical

Only `gateway.Service` is critical. If it permanently fails (impossible in practice — unlimited
retries), the root context is cancelled and systemd restarts the whole process.

All other services are non-critical: a permanently failed `opencode.Service` leaves
`claudecode`, `hitl`, `scheduler`, and `gateway` fully operational. Users are notified via
Telegram and can continue using other features.

### `withRestart` signature

```go
// maxAttempts=0 means unlimited restarts.
func withRestart(
    name        string,
    maxAttempts int,
    window      time.Duration,
    fn          func(context.Context) error,
) func(context.Context) error
```

Returns `nil` on clean shutdown (ctx cancelled) or clean service exit. Returns
`PermanentFailure` when the restart budget is exhausted within `window`.
