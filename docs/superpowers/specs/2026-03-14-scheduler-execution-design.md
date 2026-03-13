# Scheduler Execution Design

**Date:** 2026-03-14
**Status:** Approved

## Problem

Users can create scheduled jobs via chat, but jobs do not execute as expected. Three root causes:

1. **LLM has no clock.** `schedule_job` with `kind="at"` requires an RFC3339 timestamp. The LLM cannot compute `now + 5 minutes` correctly, producing wrong or past timestamps. Jobs either fire immediately on startup or never fire.

2. **No "run the chat loop" target.** `target="chat"` calls `SendMessage(prompt)` — it sends the raw prompt text as a Telegram message. There is no target that runs the gateway's LLM+tools loop (web_search, web_fetch, etc.) and delivers the computed result to the user.

3. **No job visibility in `/status`.** The command shows only a count (`"Scheduled jobs: 2 active"`), not individual jobs. Users cannot verify that a job was created or inspect its schedule.

## Solution Overview

Three additive fixes, all backwards-compatible:

- **Fix A:** Add `kind="in"` — one-shot delay job where schedule = seconds from now as a string.
- **Fix B:** Inject current UTC time as a system message at the start of every `handlePlainChat` call.
- **Fix C:** Add `target="gateway"` — when fired, runs the full gateway LLM+tools chat loop and sends the result to the operator via Telegram.
- **Fix D:** Enhance `/status` to list all individual jobs with details.

## Architecture

### Fix A — `kind="in"` (relative delay)

**File:** `internal/scheduler/service.go`

- Add `"in"` to `validateJob` alongside `"at"`, `"every"`, `"cron"`.
- In `computeInitialNextRun`: parse schedule as integer seconds, return `now + Duration(secs)`.
- In `CreateJob`: `kind="in"` sets `delete_after_run=true` (one-shot, same as `kind="at"`).
- `advanceNextRun`: `kind="in"` case is never reached (job is deleted after run).
- Update `scheduleJobSchema` description to document `kind="in"`: `"delay in seconds from now e.g. '300' (5 minutes), one-shot"`.

No schema changes. The `kind` column already stores arbitrary strings.

### Fix B — Current UTC time injection

**File:** `internal/gateway/loop.go`

At the start of `handlePlainChat`, after loading the soul/memory system prompt and before loading conversation history, append one system message:

```
"Current UTC time: 2026-03-14T15:04:05Z"
```

This is injected on every plain-chat call. The LLM uses it for:
- Computing correct RFC3339 timestamps for `kind="at"` jobs.
- Any other time-aware reasoning (cron expressions, "next Monday", etc.).

### Fix C — `target="gateway"` (run chat loop)

**Files:** `internal/agent/kind.go`, `internal/scheduler/service.go`, `internal/gateway/service.go`, `internal/app/app.go`

#### `internal/agent/kind.go`

Add `KindGateway Kind = 3`. Update `String()` and `KindFromString()` to handle `"gateway"`.

#### `internal/scheduler/service.go`

Update `scheduleJobSchema` to include `"gateway"` in the `target` enum and description.

No change to `fireJob` — `KindGateway` falls through to `RunAgentTask(ctx, KindGateway, prompt)`, which is handled by `appJobTarget`.

#### `internal/gateway/service.go`

Add a public method:

```go
func (s *Service) RunScheduledChat(ctx context.Context, chatID int64, prompt string) error {
    s.handlePlainChat(ctx, chatID, prompt)
    return nil
}
```

This delegates directly to the existing `handlePlainChat` — full LLM+tools loop, result sent to `chatID` via `s.ch.SendMessage`. No new state or concurrency concerns; `handlePlainChat` is already safe to call from any goroutine.

#### `internal/app/app.go`

`appJobTarget` gains a `gwRun func(ctx context.Context, chatID int64, prompt string) error` field and a package-private setter `setGatewayRunner`.

`RunAgentTask` handles the new case:

```go
case agent.KindGateway:
    if t.gwRun == nil {
        return fmt.Errorf("app: gateway runner not configured")
    }
    return t.gwRun(ctx, operatorChatID, prompt)
```

In `app.Run()`, after both `schedSvc` and `gatewaySvc` are constructed:

```go
jobTarget.setGatewayRunner(gatewaySvc.RunScheduledChat)
```

This deferred wire-up breaks the construction cycle without introducing a new interface method in the `scheduler` package. No circular package imports: `app` already imports both `gateway` and `scheduler`; `gateway` imports `scheduler`; `scheduler` imports neither.

### Fix D — Enhanced `/status` job listing

**File:** `internal/gateway/router.go`

Replace the current single-line count with a summary line followed by one line per job:

```
Scheduled jobs: 2 active, 1 disabled
  [abc12345] every 3600s → gateway: "get gold price" ✓ next in 45m
  [def34567] cron 0 9 * * 1-5 → opencode: "run tests" ✓ next in 3h 20m
  [ghi56789] at 2026-03-14T10:30:00Z → chat: "meeting reminder" ✗ disabled
```

Format per row:
- `[ID[:8]]` — first 8 chars of UUID
- `kind schedule` — kind and raw schedule string
- `→ target:` — target name
- `"prompt"` — prompt truncated to 40 chars with `…` if longer
- `✓`/`✗` — enabled/disabled
- `next in Xh Ym` (for enabled, future) or `overdue` (for enabled, past) or `disabled` (for disabled)

Jobs sorted: enabled first (by next_run_at ascending), disabled last.

## Data Flow: Scheduled Gold Price Lookup

1. User: `"get me gold prices in 5 minutes"`
2. LLM calls `schedule_job` with `kind="in"`, `target="gateway"`, `prompt="search for current gold price and send a summary"`, `schedule="300"`.
3. Gateway tool engine executes: `sched.CreateJob(...)` → job stored in SQLite with `next_run_at = now+5min`, `delete_after_run=true`, `target="gateway"`.
4. LLM responds: `"Scheduled! I'll get you gold prices in 5 minutes."`
5. 5 minutes later: scheduler tick fires, `ListEnabledJobsDueBefore(now)` returns the job.
6. `fireJob` → `RunAgentTask(ctx, KindGateway, "search for current gold price...")`.
7. `appJobTarget.RunAgentTask` → `gatewaySvc.RunScheduledChat(ctx, operatorChatID, prompt)`.
8. `handlePlainChat` runs full LLM+web_search loop → result sent to operator via Telegram.
9. Job deleted from store (`delete_after_run=true`).

## Error Handling

- `kind="in"` with non-integer schedule: `validateJob` returns error; LLM receives error string from tool result.
- `KindGateway` with `gwRun == nil`: `RunAgentTask` returns error; scheduler logs warning and sends "Scheduled job skipped: agent busy." to operator.
- Concurrent `handlePlainChat` calls (scheduler + live chat): safe. SQLite `_busy_timeout=5000ms` handles write contention. LLM calls are stateless per invocation.

## Tests

All tests in `package foo_test` (black-box), stdlib only.

| Package | Test | Assertion |
|---|---|---|
| `agent` | `TestKindGateway_RoundTrip` | `KindGateway.String() == "gateway"` and `KindFromString("gateway") == KindGateway` |
| `scheduler` | `TestSchedulerKindIn_NextRunAt` | `kind="in"`, `schedule="300"` → `next_run_at` within ±2s of `now+300s`, `delete_after_run=true` |
| `scheduler` | `TestSchedulerKindIn_Fires` | job fires once, is deleted from store, job target called exactly once |
| `gateway` | `TestStatus_ListsAllJobs` | `/status` response contains each job's 8-char ID prefix, kind, target, and truncated prompt |
| `gateway` | `TestPlainChat_CurrentTimeInjected` | LLM receives a system message containing `"Current UTC time:"` on every plain-chat call |
| `gateway` | `TestScheduleJob_GatewayTarget` | LLM calls `schedule_job` with `target="gateway"`; job in store has `target="gateway"` |
| `app` | `TestAppJobTarget_GatewayRunner` | `RunAgentTask(ctx, KindGateway, prompt)` invokes the registered runner; returns error when runner is nil |

## Files Changed

| File | Change |
|---|---|
| `internal/agent/kind.go` | Add `KindGateway = 3`, update `String()` and `KindFromString()` |
| `internal/scheduler/service.go` | Add `kind="in"` support; update tool schema for `kind="in"` and `target="gateway"` |
| `internal/gateway/loop.go` | Inject current UTC time system message in `handlePlainChat` |
| `internal/gateway/service.go` | Add `RunScheduledChat` method |
| `internal/gateway/router.go` | Enhance `buildStatus` to list individual jobs |
| `internal/app/app.go` | Add `gwRun` field + `setGatewayRunner`; handle `KindGateway` in `RunAgentTask`; call `setGatewayRunner` after gateway construction |
| `internal/agent/kind_test.go` | `TestKindGateway_RoundTrip` |
| `internal/scheduler/service_test.go` | `TestSchedulerKindIn_NextRunAt`, `TestSchedulerKindIn_Fires` |
| `internal/gateway/service_test.go` | `TestStatus_ListsAllJobs`, `TestPlainChat_CurrentTimeInjected`, `TestScheduleJob_GatewayTarget` |
| `internal/app/app_test.go` | `TestAppJobTarget_GatewayRunner` |
