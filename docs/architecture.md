# Architecture

GistClaw is a single Go binary. Five services run as goroutines under `errgroup`. No actor framework, no protobuf.

---

## Service topology

```
systemd (Restart=always)
    └── gistclaw binary
            │
      errgroup (root context)  ← internal/app
            │
    ┌───────┴──────────────────────────────────────────┐
    │                                                  │
WithRestart("gateway", unlimited)     WithRestart("hitl", 10, 10s)
- Telegram long-poll                  - inline keyboard approvals
- update_id dedup (SQLite)            - sequential question flows
- /oc /cc /status /stop routing       - auto-reject on timeout
- plain chat → LLM + tools
        │ injects interfaces
        └───────────────────────────────────────────────┐
                                                        │
                      ┌─────────────────────────────────┤
                      │                                 │
       WithRestart("opencode", 5/30s)   WithRestart("claudecode", 5/30s)
       - spawns opencode serve          - runs claude -p subprocess
       - HTTP + SSE client              - FSM: Idle ↔ Running
       - CostGuard.Track() on           - hook server :8765
         step-finish events             - CostGuard.Track() on
                                          total_cost_usd events
                      │
           WithRestart("scheduler", 5/30s)
           - 1s ticker, cron/at/every jobs
           - targets agent.Kind enum
           - SQLite jobs table
                      │
          ┌───────────┴─────────────────┐
      SQLiteStore                  internal/infra
      - sessions                   - CostGuard (atomic.Int64)
      - hitl_pending               - SOULLoader (mtime cache)
      - cost_daily                 - Heartbeat (Tier 1+2)
      - channel_state              internal/providers
      - jobs                       - LLMProvider + trackingProvider decorator
                                   internal/tools
                                   - web_search, web_fetch
                                   internal/mcp
                                   - MCPManager (stdio + SSE/HTTP)
```

All service boundaries are Go interfaces. Concrete types are unexported.

---

## Key flows

**`/oc` task:**
1. Gateway receives Telegram update → dedup check → `opencode.SubmitTask`
2. OpenCode POSTs to `opencode serve`, opens SSE stream → chunks forwarded to Telegram
3. `permission.asked` SSE event → HITL inline keyboard → user taps → POST reply to OpenCode
4. `session.status {type:"idle"}` → "Done"

**`/cc` task:** Same flow but via `claude -p` subprocess + `gistclaw-hook`. The hook binary POSTs to `:8765`, blocks waiting for the same HITL flow, then exits 0 (allow) or 2 (deny).

**Plain chat:** Gateway builds tool registry → `LLM.Chat` in a loop → may call `web_search`/`web_fetch` → doom-loop guard fires after 3 identical calls → final answer sent.

---

## Interface contracts

### `channel.Channel`
```go
type Channel interface {
    Receive(ctx context.Context) (<-chan InboundMessage, error)
    SendMessage(ctx context.Context, chatID int64, text string) error
    SendKeyboard(ctx context.Context, chatID int64, payload KeyboardPayload) error
    SendTyping(ctx context.Context, chatID int64) error
    Name() string
}
```
Gateway is the sole Telegram consumer — no 409 Conflict from two concurrent `getUpdates`.

### `providers.LLMProvider`
```go
type LLMProvider interface {
    Chat(ctx context.Context, messages []Message, tools []Tool) (*LLMResponse, error)
    Name() string
}
// Usage.TotalCostUSD: exact value, or 0 if billing is opaque
```
`NewTrackingProvider` wraps any provider and calls `CostGuard.Track` on every response.

### `hitl.Approver`
```go
type Approver interface {
    RequestPermission(ctx context.Context, req PermissionRequest) error
    RequestQuestion(ctx context.Context, req QuestionRequest) error
}
```
HITL internals (SQLite, timers, keyboard) hidden behind this interface.

### `opencode.Service` / `claudecode.Service`
```go
type Service interface {
    Run(ctx context.Context) error
    SubmitTask(ctx context.Context, chatID int64, prompt string) error
    Stop(ctx context.Context) error
    IsAlive(ctx context.Context) bool
    Name() string
}
```
`opencode.IsAlive` does a live HTTP health check. `claudecode.IsAlive` returns true as long as the service hasn't crashed.

### `scheduler.JobTarget`
```go
type JobTarget interface {
    RunAgentTask(ctx context.Context, kind agent.Kind, prompt string) error
    SendChat(ctx context.Context, chatID int64, text string) error
}
```
Scheduler dispatches via this interface — no direct dependency on any agent service.

---

## Supervision

`WithRestart` wraps a `func(context.Context) error` with a restart loop. Only gateway is critical (unlimited retries; failure cancels root context → systemd restart). All other services are non-critical: permanent failure is logged and the operator is notified, but the rest of the system keeps running.

| Service | Max attempts | Window |
|---|---|---|
| gateway | unlimited | — |
| opencode | 5 | 30s |
| claudecode | 5 | 30s |
| hitl | 10 | 10s |
| scheduler | 5 | 30s |

---

## Extending

### New channel (e.g. Discord)

1. Create `internal/channel/<name>/<name>.go` implementing `channel.Channel` (5 methods).
2. Add env vars to `internal/config/config.go` and `sample.env`.
3. Add a case to the channel factory in `internal/app/app.go`.

Nothing else changes — gateway and HITL only use the `channel.Channel` interface.

### New LLM provider

1. Create `internal/providers/<name>/<name>.go` implementing `providers.LLMProvider`.
   Set `Usage.TotalCostUSD` to the exact cost, or `0` if billing is opaque.
2. Add a case to the factory in `internal/providers/llm.go`.
3. Document `LLM_PROVIDER=<name>` in `sample.env`.

`NewTrackingProvider` wraps it automatically — no cost wiring needed.

### New agent kind

1. Add a constant to `internal/agent/kind.go` and cases to `String()` / `KindFromString()`.
2. Add a case to `appJobTarget.RunAgentTask` in `internal/app/app.go`.

Scheduler dispatches via `JobTarget` — no scheduler changes needed.
