# Design: LLM-Generated /start and /help Commands

**Date:** 2026-03-14  
**Status:** Approved

---

## Problem

Tèo has no way to tell users what it can do. A new user sending their first message gets no orientation — they don't know about `/oc`, `/cc`, web search, memory tools, or anything else.

---

## Solution

Add `/start` and `/help` commands that trigger a **one-time LLM-generated capability summary**, cached in memory for the lifetime of the process. The LLM generates the response in Tèo's own voice, grounded in the live SOUL.md content and an injected list of available commands and tools.

---

## Architecture

All changes are confined to `internal/gateway/`.

### New fields on `Service`

```go
helpOnce   sync.Once
cachedHelp string
```

`helpOnce` ensures the LLM is called at most once across concurrent `/start`/`/help` calls.

### New command cases in `router.go`

```
case text == "/start":
    s.handleHelp(ctx, msg.ChatID)
case text == "/help":
    s.handleHelp(ctx, msg.ChatID)
```

### `handleHelp(ctx, chatID)` method

```
func (s *Service) handleHelp(ctx context.Context, chatID int64)
```

1. If `s.llm` is nil, send the hardcoded fallback directly and return.
2. Call `s.helpOnce.Do(func() { ... })` to generate `s.cachedHelp` exactly once:
   a. Build a prompt:
      - System message: `s.memory.LoadContext()` if `s.memory != nil` and content is non-empty
      - User message: injected tool/command list + instruction (see Prompt Design)
   b. Call `s.llm.Chat(ctx, msgs, nil)` with no tools.
   c. On success: assign response content to `s.cachedHelp`.
   d. On LLM error: log with `log.Warn().Err(err).Msg("gateway: handleHelp LLM error; using fallback")`. **Do not** assign to `s.cachedHelp` — leave it empty so the next call retries.
3. If `s.cachedHelp` is empty (LLM failed), send the hardcoded fallback string.
4. Otherwise send `s.cachedHelp`.

**Key invariant:** `s.cachedHelp` is only written inside `helpOnce.Do`. It is read after `Do` returns (which guarantees the write happened-before the read). No additional mutex is needed.

**On LLM failure:** `helpOnce` is already "spent" — subsequent calls will skip the `Do` body and go straight to step 3 (fallback). This is acceptable: a transient error means the user gets the fallback for this process lifetime, and a bot restart resets `helpOnce`. The behaviour is logged at Warn level so operators can see it.

### Hardcoded fallback

Short command list used when the LLM call fails or `s.llm` is nil:

```
Tèo đây! Tao có thể làm:
/oc <task>  — chạy OpenCode
/cc <task>  — chạy Claude Code
/status     — xem trạng thái
/stop       — dừng agent
Chat thường: hỏi gì cũng được, tao có web search, memory, scheduler.
```

Defined as a package-level `const helpFallback` in `router.go`.

---

## Prompt Design

The prompt sent to the LLM:

```
[system: <SOUL.md content>]   ← omitted if s.memory is nil or LoadContext() is empty

[user:
Available commands and tools:
- /oc <task>: submit a coding task to OpenCode agent
- /cc <task>: submit a coding task to Claude Code agent
- /stop: stop the currently running agent
- /status: show bot uptime, agent status, cost, scheduled jobs
- web_search: search the web (Brave Search)
- web_fetch: fetch and read a URL
- remember: save a long-term fact to memory
- note: save a short-term note
- schedule_job / list_jobs / delete_job: manage cron-based scheduled tasks
- spawn_agent / run_parallel / chain_agents: orchestrate multiple AI agents

Describe what you can do for the user. Use your own voice and personality. Be concise.
]
```

---

## Caching Strategy

- **Scope:** in-memory, per process lifetime
- **Generation:** `sync.Once` — attempted on the first `/start` or `/help` call
- **On success:** cached for all future calls this process lifetime
- **On failure:** `cachedHelp` stays empty; fallback sent; `helpOnce` is spent (no retry this lifetime); bot restart resets everything
- **Invalidation:** bot restart (which also reloads SOUL.md)
- **Rationale:** The capability list is stable within a running process. Token cost is paid once. No DB complexity. The failure trade-off (permanent fallback until restart) is acceptable and logged.

---

## Error Handling

| Scenario | Behaviour |
|----------|-----------|
| LLM call succeeds | Cache and send LLM response |
| LLM call errors (network, cost-guard, etc.) | Log Warn; send hardcoded fallback; no cache written |
| `s.memory` is nil | Skip system message; still call LLM with tool list only |
| `s.llm` is nil | Skip `helpOnce.Do` entirely; send hardcoded fallback directly |
| ctx cancelled during LLM call (shutdown race) | LLM returns error → same as LLM error path above |

---

## Logging

```go
log.Warn().Err(err).Msg("gateway: handleHelp LLM error; using fallback")
```

Logged at `Warn` level (not `Error`) because the user still gets a useful response via fallback.

---

## Testing

Tests go in `internal/gateway/service_test.go` (package `gateway_test`), alongside existing tests, to reuse the existing `mockLLM`, `mockChannel`, and `newTestService` helpers.

### `TestHandleHelpLLMSuccess`

- Construct service with mock LLM that returns `"mocked help text"`.
- Inject `/start` via `svc.handle(ctx, msg)` (synchronous — no goroutine needed).
- Assert `mockChannel.lastSent == "mocked help text"`.
- Inject `/help` via `svc.handle`.
- Assert `mockChannel.lastSent == "mocked help text"` again.
- Assert mock LLM was called exactly once (cache hit on second call).

### `TestHandleHelpLLMFailure`

- Construct service with mock LLM that returns an error.
- Inject `/help`.
- Assert `mockChannel.lastSent` contains `/oc` (hardcoded fallback).
- Assert mock LLM was called once (not retried).

### `TestHandleHelpNilLLM`

- Construct service with `llm: nil`.
- Inject `/help`.
- Assert `mockChannel.lastSent` contains `/oc` (hardcoded fallback; no panic).

---

## Files Changed

| File | Change |
|------|--------|
| `internal/gateway/service.go` | Add `helpOnce sync.Once`, `cachedHelp string` fields |
| `internal/gateway/router.go` | Add `/start`, `/help` cases; add `handleHelp` method; add `helpFallback` const |
| `internal/gateway/service_test.go` | Add `TestHandleHelpLLMSuccess`, `TestHandleHelpLLMFailure`, `TestHandleHelpNilLLM` |

No new files. No new packages.
