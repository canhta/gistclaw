# Scheduled Tasks Design

**Status:** Implemented
**Date:** 2026-03-26

## Summary

GistClaw should ship **scheduled local tasks**, not a broad automation system.
The scheduler should be a small SQLite-backed service that claims due slots and
starts normal runtime work through the existing async inbound path.

This revision changes the earlier draft in one important way:

- V1 should **not** depend on a new runtime event-fanout contract
- V1 should use the runtime's existing inbound idempotency seam
- V1 should reconcile occurrence state from the authoritative `runs` projection
- V1 should ship **CLI first**, with web pages deferred

That shape matches the code that exists today in:

- [internal/runtime/collaboration.go](/Users/canh/Projects/OSS/gistclaw/internal/runtime/collaboration.go)
- [internal/runtime/runs.go](/Users/canh/Projects/OSS/gistclaw/internal/runtime/runs.go)
- [internal/runtime/projects.go](/Users/canh/Projects/OSS/gistclaw/internal/runtime/projects.go)
- [internal/conversations/service.go](/Users/canh/Projects/OSS/gistclaw/internal/conversations/service.go)
- [internal/app/lifecycle.go](/Users/canh/Projects/OSS/gistclaw/internal/app/lifecycle.go)
- [cmd/gistclaw/main.go](/Users/canh/Projects/OSS/gistclaw/cmd/gistclaw/main.go)

## What Already Exists

The current codebase already solves most of the risky parts of scheduled task
execution:

- **Async inbound dispatch**: `ReceiveInboundMessageAsync` already creates normal
  runtime-owned work from an inbound command.
- **Idempotent inbound acceptance**: `SourceMessageID` plus
  `inbound_receipts` already deduplicate retried inbound dispatch.
- **Conversation/session projection**: the runtime already owns conversation,
  session, run, approval, and replay state through the journal and projections.
- **Project/workspace normalization**: `runtime.RegisterProject`,
  `runtime.ActiveProject`, and run preparation already normalize and validate
  workspace roots.
- **App lifecycle ownership**: `App.Start` already owns long-lived services and
  cancellation.

V1 should reuse those seams directly rather than introducing a second scheduler
execution model.

## Why This V1 Fits The Current Code

The current runtime already gives the scheduler the hard parts it needs:

- a canonical async inbound entrypoint via `ReceiveInboundMessageAsync`
- durable run/session/conversation projection through the journal
- inbound idempotency via `SourceMessageID` and `inbound_receipts`
- authoritative run status in the `runs` table
- app-owned service lifecycle under `gistclaw serve`

The current runtime does **not** yet give the scheduler a complete real-time
lifecycle event stream. Today the fanout emits `run_started`, `turn_delta`,
`turn_completed`, and `run_completed`, but not the full set the earlier draft
wanted for scheduler reconciliation.

That means V1 should use the seams that already exist instead of inventing a
new runtime event contract before the scheduler has shipped at all.

## OpenClaw Heartbeat: What To Borrow

OpenClaw's heartbeat system is useful as a **future session-bound automation**
reference, but it should not define GistClaw scheduled-task V1.

Heartbeat in OpenClaw means:

- periodic awareness inside an existing main session
- optional delivery suppression when nothing needs attention
- active-hours gating
- batching many low-priority checks into one turn

That is a different product shape from GistClaw V1, which is:

- stateless per-fire scheduled tasks
- fresh conversation per slot
- explicit schedule definitions and occurrence history
- no ambient "ongoing automation chat"

The useful parts to borrow now are operational, not conversational:

- **Wake coalescing and retry cooldowns**: OpenClaw's heartbeat wake layer keeps
  one pending wake per target, preserves higher-priority reasons, and retries
  shortly when work is already in flight. GistClaw can reuse that idea for
  manual `run now` requests and startup repair dispatch retries.
- **Active-hours gating**: OpenClaw's active-hours logic is a good model for a
  later GistClaw follow-up slice once schedules need "only run during these
  hours" semantics.
- **Busy-lane skip semantics**: OpenClaw explicitly skips heartbeat work when
  the main lane is busy and retries soon instead of forcing overlap. That is
  conceptually aligned with GistClaw's `skip` concurrency policy.

The parts GistClaw should **not** copy into V1 are:

- heartbeat prompts and `HEARTBEAT_OK` acknowledgements
- transcript pruning of heartbeat-only turns
- delivery-target policy tied to direct-message chats
- main-session continuity as the default scheduling model

Those belong, if at all, in a later session-bound automation slice rather than
the first scheduler implementation.

## Goal

Allow an operator to say:

- run this task once at a future time
- run this task every hour
- run this task every morning

and have GistClaw start a normal local run at the right time, with the resulting
run visible through the existing run, approval, replay, and session surfaces.

## Shipping Slice

The first shipping slice includes:

- durable schedules stored in SQLite
- three schedule kinds: `at`, `every`, `cron`
- one scheduled payload shape: a normal local task objective
- stateless execution per scheduled fire
- one fixed front agent: `assistant`
- one fixed overlap behavior: `skip`
- enable/disable
- manual `run now`
- durable occurrence history
- bounded startup catch-up
- CLI management commands
- linkage from each schedule occurrence to the created runtime run

The first shipping slice does **not** include:

- a dedicated web schedules UI
- runtime event-fanout integration for schedules
- persistent session-bound automation
- delivery targets or reminders into existing connector threads
- separate automation conversations beyond the fresh per-slot runtime conversation
- per-schedule front-agent selection
- per-schedule model overrides
- workflow pipelines

## Product Behavior

Each schedule fire creates a **fresh scheduled run context**.

That means:

- no conversation carry-over between scheduled fires
- no session memory carried between scheduled fires
- no "ongoing automation chat" semantics

This keeps V1 predictable and aligned with the current runtime.

## Architecture

```text
SQLite schedules/occurrences
        |
        v
  scheduler loop
        |
        v
claim due slot in tx
        |
        v
runtime.ReceiveInboundMessageAsync(...)
  with SourceMessageID = occurrence_id
  with FrontAgentID = "assistant"
        |
        v
conversation journal -> runs/sessions/approvals/replay
        |
        v
scheduler reconciliation reads runs.status
        |
        v
occurrence status updated in SQLite
```

### Boundary Rule

The scheduler owns:

- schedule definitions
- next-due computation
- occurrence claiming
- occurrence history
- dispatch idempotency keys
- occurrence status mirroring

The runtime still owns:

- conversation resolution
- session creation
- run execution
- approvals
- replay
- receipts
- connector delivery

That split keeps the scheduler as a producer of normal work, not a second run
engine.

## Lifecycle Integration

The scheduler should be a managed service started by `gistclaw serve`.

It should be wired in bootstrap and started from the same owned lifecycle as the
existing connectors and web server:

- it receives the shared run context from `App.Start`
- it exits when that context is canceled
- it reports fatal startup/runtime errors through the same error channel pattern
- it does not spawn an unowned background goroutine

This matches the goroutine ownership policy in the repo.

## Runtime Integration

The scheduler should dispatch through `ReceiveInboundMessageAsync` in
[internal/runtime/collaboration.go](/Users/canh/Projects/OSS/gistclaw/internal/runtime/collaboration.go).

For each claimed occurrence, it should construct a synthetic conversation key:

- `connector_id = "schedule"`
- `account_id = "local"`
- `external_id = "job:<schedule_id>"`
- `thread_id = occurrence.thread_id`

It should also set:

- `SourceMessageID = <occurrence_id>`
- `FrontAgentID = "assistant"`

### Why This Shape

- It reuses the canonical inbound runtime path.
- `thread_id` is part of the normalized conversation key, so each slot gets a
  fresh conversation.
- Persisting the exact `thread_id` avoids trying to reconstruct a runtime key
  from SQLite datetime text later during crash repair.
- `SourceMessageID` makes dispatch idempotent on retries.
- `inbound_receipts` gives startup repair a durable way to recover `run_id`
  after a crash.

### Dispatch Rule

After `ReceiveInboundMessageAsync` returns successfully, the scheduler should
immediately persist:

- `occurrence.run_id`
- `occurrence.conversation_id`
- `occurrence.status = 'active'`
- `occurrence.started_at`

V1 should not wait for a separate `run_started` event to do this.

## Validation And Normalization

Schedule create and update should reuse the runtime's existing project seam
instead of inventing scheduler-local workspace rules.

Rules:

- if the operator provides `workspace_root`, the scheduler service should call
  `runtime.RegisterProject` to normalize it and fail fast if it is not a usable
  repo workspace.
- if `workspace_root` is omitted, the scheduler service should resolve
  `runtime.ActiveProject` and use that workspace.
- schedules should store the normalized `workspace_root` returned by the runtime
  seam.
- dispatch should pass that normalized workspace root back into
  `ReceiveInboundMessageAsync`.

This keeps workspace validation and project discovery in one place.

## Scheduler Package Shape

Proposed package:

```text
internal/scheduler/
  service.go          lifecycle, wake loop, dispatch, reconciliation, repair
  types.go            schedule and occurrence models
  store.go            sqlite reads/writes for schedules and occurrences
  schedule.go         next-run calculation
```

This package should be small, boring, SQLite-first, and minimal-diff. If
dispatch or reconciliation later become large enough to justify a split, that
can happen after V1 ships.

## Data Model

Schema changes belong in
[001_init.sql](/Users/canh/Projects/OSS/gistclaw/internal/store/migrations/001_init.sql).

### `schedules`

Purpose: current definition plus current scheduling state.

Suggested columns:

- `id TEXT PRIMARY KEY`
- `name TEXT NOT NULL`
- `objective TEXT NOT NULL`
- `workspace_root TEXT NOT NULL`
- `schedule_kind TEXT NOT NULL`
- `schedule_at TEXT NOT NULL DEFAULT ''`
- `schedule_every_seconds INTEGER NOT NULL DEFAULT 0`
- `schedule_cron_expr TEXT NOT NULL DEFAULT ''`
- `timezone TEXT NOT NULL DEFAULT ''`
- `enabled INTEGER NOT NULL DEFAULT 1`
- `next_run_at DATETIME`
- `last_run_at DATETIME`
- `last_status TEXT NOT NULL DEFAULT ''`
- `last_error TEXT NOT NULL DEFAULT ''`
- `consecutive_failures INTEGER NOT NULL DEFAULT 0`
- `schedule_error_count INTEGER NOT NULL DEFAULT 0`
- `created_at DATETIME NOT NULL DEFAULT (datetime('now'))`
- `updated_at DATETIME NOT NULL DEFAULT (datetime('now'))`

Indexes:

- `(enabled, next_run_at)`

### `schedule_occurrences`

Purpose: durable execution history for each claimed slot.

Suggested columns:

- `id TEXT PRIMARY KEY`
- `schedule_id TEXT NOT NULL`
- `slot_at DATETIME NOT NULL`
- `thread_id TEXT NOT NULL`
- `status TEXT NOT NULL`
- `skip_reason TEXT NOT NULL DEFAULT ''`
- `run_id TEXT NOT NULL DEFAULT ''`
- `conversation_id TEXT NOT NULL DEFAULT ''`
- `error TEXT NOT NULL DEFAULT ''`
- `started_at DATETIME`
- `finished_at DATETIME`
- `created_at DATETIME NOT NULL DEFAULT (datetime('now'))`
- `updated_at DATETIME NOT NULL DEFAULT (datetime('now'))`

Indexes and constraints:

- `UNIQUE(schedule_id, slot_at)`
- partial index on `(schedule_id, created_at DESC)` where `status IN
  ('dispatching', 'active', 'needs_approval')`
- index on `(status, updated_at)`
- index on `(run_id)`

### Important Detail

The occurrence `id` is also the dispatch `SourceMessageID`, and
`schedule_occurrences.thread_id` stores the exact runtime thread key used for
that slot.

That avoids adding a second synthetic identifier just for runtime handoff and
avoids fragile repair logic that tries to rebuild thread identity from datetime
formatting.

## Schedule Semantics

### `at`

- one-shot absolute timestamp
- stored in UTC
- after successful claim, `next_run_at` becomes `NULL`

### `every`

- fixed interval in seconds
- next slot is based on the scheduled slot, not on completion time
- this avoids drift

### `cron`

- cron expression plus optional IANA timezone
- stored next due time in UTC
- use a pure-Go parser such as `github.com/robfig/cron/v3`
- use it as a parser and next-time calculator only, not as an in-memory cron
  runner

### Local Time Rule

At the API boundary:

- `at` requires a fully qualified RFC3339 timestamp with offset
- `cron` may include timezone separately
- UI may present local wall-clock time, but storage stays UTC

This keeps DST behavior explicit.

## Concurrency And Catch-Up

### Concurrency Policy

V1 supports exactly one fixed behavior: `skip`.

If a schedule becomes due while one of its prior occurrences is still in:

- `dispatching`
- `active`
- `needs_approval`

then the new slot is recorded as:

- occurrence row with `status = 'skipped'`
- `skip_reason = 'previous_occurrence_active'`

and the schedule advances to the next future slot.

### Startup Catch-Up

On startup:

- one-shot `at` schedules that are enabled and overdue should fire once
- recurring schedules should claim at most one overdue slot immediately
- recurring schedules should then advance to the next future slot
- do **not** replay every missed historical slot

## Occurrence State Machine

```text
dispatching -> active -> completed
                    \-> failed
                    \-> interrupted
                    \-> needs_approval -> active -> completed|failed|interrupted

due slot -> skipped
dispatching -> failed
```

### Status Meaning

- `dispatching`: claimed by scheduler, runtime handoff not yet fully persisted
- `active`: runtime accepted the run
- `needs_approval`: linked run is paused on approval
- `completed`: linked run completed
- `failed`: dispatch failed or linked run failed
- `interrupted`: linked run stopped early
- `skipped`: due slot was intentionally not started

## Claiming Algorithm

The scheduler wake loop should:

1. load due schedules ordered by `next_run_at`
2. for each schedule, inside one transaction:
   - re-read the row
   - confirm it is still due and enabled
   - check for an active occurrence
   - compute and persist `thread_id = <scheduled_slot_rfc3339nano>`
   - insert a `schedule_occurrences` row for the current slot with
     `status = 'dispatching'`
   - advance `schedules.next_run_at` to the next future slot
3. commit
4. outside the transaction, dispatch runtime work

The `UNIQUE(schedule_id, slot_at)` constraint is the anti-duplication guard.

The wake loop should sleep until the nearest enabled `next_run_at`, clamped by:

- a short minimum delay to avoid hot-looping on stale overdue state
- a bounded maximum fallback poll interval when no schedule is due soon

## Dispatch Algorithm

For each claimed occurrence:

1. call `ReceiveInboundMessageAsync`
2. pass the synthetic schedule conversation key
3. pass `FrontAgentID = "assistant"`
4. pass `Body = schedule.objective`
5. pass the normalized `workspace_root`
6. pass `SourceMessageID = occurrence.id`
7. if the call succeeds:
   - persist `run_id`
   - persist `conversation_id`
   - set status to `active`
8. if the call fails before the run is accepted:
   - try one receipt-based recovery lookup using the persisted
     `occurrence.thread_id`
   - if recovery succeeds, backfill `run_id` and `conversation_id` and
     continue reconciliation
   - otherwise mark the occurrence `failed`
   - record the error text

If dispatch is retried after a crash, the same `SourceMessageID` should resolve
to the already-created run instead of launching duplicate work.

## Reconciliation From The Runs Projection

V1 should reconcile occurrences from the `runs` table, not from the runtime
event fanout.

On each scheduler wake and during startup repair, load non-terminal occurrences
with a `run_id` and map the linked run status:

- `active` -> occurrence `active`
- `needs_approval` -> occurrence `needs_approval`
- `completed` -> occurrence `completed`
- `failed` -> occurrence `failed`
- `interrupted` -> occurrence `interrupted`

This uses the runtime's authoritative projection instead of introducing a second
status channel for V1.

Whenever an occurrence reaches a new terminal or approval state, reconciliation
should also update the parent schedule summary fields:

- `last_run_at`
- `last_status`
- `last_error`
- `consecutive_failures`

## Startup Repair

On scheduler startup, repair inconsistent rows before entering the main loop:

- for `dispatching` rows with empty `run_id`, first try to recover the run via
  `inbound_receipts` using:
  - connector `schedule`
  - account `local`
  - thread `occurrence.thread_id`
  - source message ID `<occurrence_id>`
- if a receipt is found:
  - backfill `run_id`
  - backfill `conversation_id`
  - reconcile status from `runs`
- if no receipt is found and the row is older than a short grace period:
  - mark it `failed`
- for `active` or `needs_approval` rows with a `run_id`:
  - reconcile from `runs`
- for enabled schedules with missing `next_run_at`:
  - recompute it

This makes crash recovery concrete instead of aspirational.

## Manual Operations

V1 should support:

- create
- update
- enable
- disable
- delete
- run now

All of those write paths should go through scheduler service methods. CLI and
future web handlers should not issue ad hoc scheduler SQL directly.

`run now` should:

- create an occurrence with `slot_at = now`
- use the same dispatch path
- use the same `SourceMessageID = occurrence.id` rule
- respect the same concurrency policy

## Operator Surface

### CLI

The first slice should be CLI-first.

The current CLI is a hand-rolled top-level subcommand switch in
[cmd/gistclaw/main.go](/Users/canh/Projects/OSS/gistclaw/cmd/gistclaw/main.go),
so the scheduler should follow that pattern:

```text
gistclaw schedule add
gistclaw schedule update <id>
gistclaw schedule status
gistclaw schedule list
gistclaw schedule show <id>
gistclaw schedule run <id>
gistclaw schedule enable <id>
gistclaw schedule disable <id>
gistclaw schedule delete <id>
```

### Existing Surfaces Reused In V1

Scheduled runs should already be visible through existing:

- run detail pages
- approvals pages
- replay
- `inspect runs`
- `inspect replay`

### Web

A dedicated `Configure > Schedules` page is a follow-up slice, not part of the
first shipping cut.

## Error Handling

### Schedule Computation Errors

If a schedule definition cannot compute its next slot:

- increment `schedule_error_count`
- clear `next_run_at`
- set `last_error`

After repeated failures, auto-disable the schedule.

### Dispatch Errors

If runtime handoff fails after the slot is claimed and no receipt can be found:

- mark the occurrence `failed`
- set occurrence error text
- increment schedule failure counters

### Approval Pauses

Approval is not a scheduler failure.

If a linked run pauses for approval:

- the occurrence becomes `needs_approval` during reconciliation
- the operator resolves it through the normal approvals surface
- later reconciliation observes the resumed terminal or active state

## Failure Modes

| Flow | Realistic failure | Test required | Error handling | User-visible outcome |
|------|-------------------|---------------|----------------|----------------------|
| claim due slot | two wakes race on the same due row | yes | yes, via transaction + unique constraint | silent success, one claim wins |
| dispatch accepted but `run_id` not yet persisted | process crashes after runtime acceptance | yes | yes, via receipt-based recovery | operator still sees occurrence recover |
| workspace normalization | configured path is not a repo or no longer exists | yes | yes, fail create/update or mark dispatch failed | clear CLI error / failed occurrence |
| overlap handling | prior occurrence stays active when next slot becomes due | yes | yes, record skipped occurrence | visible skipped history entry |
| repair lookup | stale `dispatching` row has wrong or missing thread identity | yes | yes, by persisting exact `thread_id` | visible failed occurrence, not duplicate run |
| lifecycle shutdown | process stops mid-wake or mid-dispatch | yes | yes, owned context cancellation + startup repair | delayed execution, not orphaned work |

Any future implementation path that lacks a test, lacks error handling, and
would fail silently should be treated as a critical gap before coding starts.

## Observability

At minimum, log:

- scheduler start/stop
- next wake time
- due schedule claims
- skipped slots
- dispatch start/success/failure
- receipt-based recovery actions
- reconciliation updates
- startup repair actions

## Testing

Required test groups:

- next-run computation for `at`, `every`, `cron`
- timezone handling
- one-shot disable behavior
- unique occurrence claim on restart races
- skip-on-overlap policy
- startup catch-up behavior
- workspace normalization through runtime project registration
- dispatch through `ReceiveInboundMessageAsync`
- idempotent dispatch via `SourceMessageID`
- crash repair through `inbound_receipts`
- occurrence reconciliation from `runs.status`
- approval pause transitions through `runs.status = needs_approval`
- schedule summary field updates during reconciliation
- lifecycle cancellation for the scheduler service

### Coverage Diagram

```text
SCHEDULED TASK V1 TEST COVERAGE
===============================

[+] internal/scheduler/schedule.go
    |
    ├── nextAt()                 -> one-shot due / future / overdue startup catch-up
    ├── nextEvery()              -> fixed interval without drift
    └── nextCron()               -> timezone + DST boundary + invalid expression

[+] internal/scheduler/store.go
    |
    ├── create/update schedule   -> normalize workspace root through runtime seam
    ├── claim due occurrence     -> one winner under race
    ├── overlap query            -> active row forces skipped occurrence
    ├── repair lookup            -> receipt backfill by occurrence.id + thread_id
    └── reconcile writeback      -> occurrence status + schedule summary fields

[+] internal/scheduler/service.go
    |
    ├── startup repair
    │   ├── dispatching with receipt     -> recover to active/completed
    │   └── dispatching without receipt  -> fail after grace period
    ├── wake loop
    │   ├── due enabled schedule         -> claim + dispatch
    │   ├── disabled schedule            -> no claim
    │   └── context canceled             -> clean exit
    └── dispatch
        ├── runtime accepts run          -> persist run_id + conversation_id
        ├── runtime duplicate receipt    -> reload existing run
        └── runtime error                -> receipt recovery once, then fail

[+] cmd/gistclaw/main.go
    |
    ├── schedule add             -> validates required fields and writes via service
    ├── schedule run             -> manual occurrence through same dispatch path
    └── schedule enable/disable  -> service-owned write path only

Suggested test files:

- `internal/scheduler/schedule_test.go`
- `internal/scheduler/store_test.go`
- `internal/scheduler/service_test.go`
- `internal/app/bootstrap_test.go`
- `cmd/gistclaw/main_test.go`
```

The implementation should also keep inline ASCII comments in the scheduler
service around the occurrence state machine and startup repair flow.

## Rollout

Rollout should be staged:

1. schema + store + scheduler service + CLI
2. startup repair + status reconciliation polish
3. dedicated web list/detail pages if the backend proves useful
4. optional active-hours and wake-coalescing refinements inspired by heartbeat
5. optional real-time event fanout later, only if the web UI needs it

## NOT In Scope

- dedicated web schedule pages: CLI is enough to prove the backend slice first
- session-bound recurring conversations: V1 is fresh per-slot work only
- heartbeat prompt/ack semantics: that is a different product shape
- per-schedule agent or model overrides: fixed front agent keeps V1 minimal
- custom overlap policies beyond `skip`: one behavior is enough for first ship
- real-time scheduler event fanout: reconciliation from `runs` is sufficient now

## Decision

Ship V1 as **stateless scheduled local tasks** backed by SQLite, dispatched
through the existing async inbound runtime path, made idempotent with
`SourceMessageID = occurrence_id`, and reconciled from the authoritative `runs`
projection.

That gives GistClaw the useful part of scheduling now, while matching the code
that already exists in the repo instead of forcing a new runtime event contract
before the first scheduler slice ships.
