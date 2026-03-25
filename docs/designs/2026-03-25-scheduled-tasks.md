# Scheduled Tasks Design

**Status:** Proposed
**Date:** 2026-03-25

## Summary

This document intentionally rewrites the first scheduler slice to a narrower V1.

GistClaw should ship **scheduled local tasks**, not a broad automation platform.
The scheduler should be a small, durable producer that starts normal runtime runs
on a schedule. It should not introduce a second execution model beside the
existing journal-backed runtime.

## Why This V1 Is Smaller

The larger automation shape is attractive, but it is the wrong first move for
this repo.

OpenClaw proves that cron systems become complex quickly once they support:

- persistent session binding
- channel delivery targets
- isolated vs main-session semantics
- retry alerts and delivery routing
- user-facing automation breadth

GistClaw is not trying to rebuild that system right now. Its current kernel is:

- local-first
- runtime-mediated
- journal-backed for runs
- operator-facing around runs, approvals, sessions, routes, deliveries, and memory

The scheduler should fit that kernel instead of expanding product scope sideways.

## Goal

Allow an operator to say:

- run this task every morning
- run this task every hour
- run this task once at a future time

and have GistClaw start a normal local run at the right time, with the resulting
run visible in the existing run, approval, replay, and session surfaces.

## V1 Scope

V1 includes:

- durable schedules stored in SQLite
- three schedule kinds: `at`, `every`, `cron`
- one scheduled payload shape: a normal local task objective
- stateless execution per scheduled fire
- a single concurrency policy: `skip`
- manual `run now`
- enable/disable
- recent occurrence history
- startup catch-up for missed runs
- operator visibility for:
  - schedule definition
  - next due time
  - last outcome
  - recent scheduled occurrences
  - linked runtime run ID when dispatch succeeded

V1 does **not** include:

- persistent session-bound automation
- “run in the same conversation every day” semantics
- connector delivery targets
- webhooks
- per-schedule model overrides
- separate automation agents
- multi-step workflow pipelines
- reminder text injection into an existing conversation
- replayable scheduler event journaling beyond the existing run journal

## Product Behavior

Each schedule fire creates a **fresh scheduled run context**.

That means:

- no conversation carry-over between scheduled fires
- no session memory from the previous scheduled fire
- no “ongoing automation chat” semantics

This keeps V1 predictable and avoids context drift.

If an operator wants long-lived automation context later, that can be added as a
separate second slice with explicit semantics.

## Chosen Approach

### Recommended

Build a new `internal/scheduler/` package that:

1. stores schedule definitions and occurrence history in SQLite
2. wakes on the next due slot
3. claims due schedules transactionally
4. starts work through the existing runtime async inbound path
5. listens for run lifecycle events and reconciles occurrence state

### Rejected Alternative: Full OpenClaw-Style Cron Surface

This would include delivery modes, session targets, automation-specific
conversations, and a larger operator surface.

Reject for now because it creates a second product line before the current local
runtime has earned a narrow scheduler foundation.

### Rejected Alternative: Schedule Direct `runtime.Start` Calls

This bypasses the normal inbound/session resolution path and would create a new
special-case execution edge.

Reject because the scheduler should behave like a producer of normal work, not a
parallel executor with privileged shortcuts.

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
        |
        v
conversation journal -> runs/sessions/approvals/replay
        |
        v
scheduler run-event sink updates occurrence status
```

### Boundary Rule

The scheduler owns:

- schedule definitions
- next due computation
- occurrence claiming
- occurrence history

The runtime still owns:

- conversation resolution
- session creation
- run execution
- approvals
- replay
- receipts
- connector delivery

That split is the core design decision.

## Runtime Integration

The scheduler should start work through `ReceiveInboundMessageAsync` in
[internal/runtime/collaboration.go](/Users/canh/Projects/OSS/gistclaw/internal/runtime/collaboration.go).

For each claimed occurrence, it should construct a synthetic conversation key:

- `connector_id = "schedule"`
- `account_id = "local"`
- `external_id = "job:<schedule_id>"`
- `thread_id = "<scheduled_slot_rfc3339nano>"`

Then it sends the schedule objective as the inbound body.

### Why This Shape

- It reuses the canonical inbound runtime path.
- Each scheduled slot gets a fresh conversation because `thread_id` is unique per slot.
- Resulting runs flow through the normal journal-backed runtime machinery.
- Approval and replay behavior remain unchanged.

## Scheduler Package Shape

Proposed package:

```text
internal/scheduler/
  service.go          lifecycle and wake loop
  types.go            schedule and occurrence models
  store.go            sqlite reads/writes for schedules and occurrences
  schedule.go         next-run calculation
  dispatch.go         runtime handoff
  reconcile.go        run-event sink updates
```

The scheduler service should implement two roles:

1. long-running scheduler loop started by `gistclaw serve`
2. `model.RunEventSink` companion that updates schedule occurrence state from runtime events

## Data Model

Schema changes belong in
[001_init.sql](/Users/canh/Projects/OSS/gistclaw/internal/store/migrations/001_init.sql).

### `schedules`

Purpose: current definition + current scheduling state.

Suggested columns:

- `id TEXT PRIMARY KEY`
- `name TEXT NOT NULL`
- `objective TEXT NOT NULL`
- `workspace_root TEXT NOT NULL`
- `front_agent_id TEXT NOT NULL DEFAULT 'assistant'`
- `schedule_kind TEXT NOT NULL`
- `schedule_at TEXT NOT NULL DEFAULT ''`
- `schedule_every_seconds INTEGER NOT NULL DEFAULT 0`
- `schedule_cron_expr TEXT NOT NULL DEFAULT ''`
- `timezone TEXT NOT NULL DEFAULT ''`
- `enabled INTEGER NOT NULL DEFAULT 1`
- `concurrency_policy TEXT NOT NULL DEFAULT 'skip'`
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
- index on `(schedule_id, created_at DESC)`
- index on `(run_id)`

### Why Separate Definitions And Occurrences

This keeps:

- one row per schedule for current state
- many rows per actual fire for operator history

It also gives a clean place to link runtime run IDs back to a schedule fire.

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

### Local Time Rule

At the API boundary:

- `at` should require a fully qualified RFC3339 timestamp with offset
- `cron` may include timezone separately
- UI may present local wall-clock time, but storage should stay UTC

This keeps DST behavior explicit and avoids ambiguous write paths in V1.

## Concurrency And Catch-Up

### Concurrency Policy

V1 supports exactly one policy: `skip`.

If a schedule becomes due while one of its prior occurrences is still in:

- `dispatching`
- `active`
- `needs_approval`

then the new slot is recorded as:

- occurrence row with `status = 'skipped'`
- `skip_reason = 'previous_occurrence_active'`

and the schedule advances to the next due slot.

### Startup Catch-Up

On startup:

- one-shot `at` schedules that are still enabled and overdue should fire once
- recurring schedules should claim at most one overdue slot immediately
- do **not** replay every missed historical slot

This is the OpenClaw lesson worth copying: catch up enough to be useful, but
not enough to create restart storms.

## Occurrence State Machine

```text
dispatching -> active -> completed
                    \-> failed
                    \-> interrupted
                    \-> needs_approval -> active -> completed|failed|interrupted

due slot -> skipped
dispatching -> failed    (if runtime handoff fails before run start)
```

### Status Meaning

- `dispatching`: claimed by scheduler, runtime handoff in progress
- `active`: runtime accepted and started the run
- `needs_approval`: run is paused on an approval boundary
- `completed`: run finished successfully
- `failed`: runtime or dispatch failed
- `interrupted`: run stopped early
- `skipped`: due slot was intentionally not started

## Claiming Algorithm

The scheduler wake loop should:

1. load due schedules ordered by `next_run_at`
2. for each schedule, inside one transaction:
   - re-read the row
   - confirm it is still due and enabled
   - check for an active occurrence
   - insert a `schedule_occurrences` row for the current slot
   - advance `schedules.next_run_at` to the next future slot
3. commit
4. outside the transaction, dispatch runtime work

The `UNIQUE(schedule_id, slot_at)` constraint is the anti-duplication guard.

## Reconciliation From Runtime Events

The scheduler should implement a run-event sink and be added to the existing
event fanout in app bootstrap.

It should listen for at least:

- `run_started`
- `approval_requested`
- `run_completed`
- `run_failed`
- `run_interrupted`

Reconciliation rules:

- `run_started` -> occurrence becomes `active`
- `approval_requested` -> occurrence becomes `needs_approval`
- `run_completed` -> occurrence becomes `completed`
- `run_failed` -> occurrence becomes `failed`
- `run_interrupted` -> occurrence becomes `interrupted`

The sink should also update `schedules.last_*` fields.

## Startup Repair

On scheduler startup, repair inconsistent rows before the loop starts:

- `dispatching` rows older than a short grace period with empty `run_id`
  become `failed`
- `active` or `needs_approval` rows with a linked terminal run in `runs`
  are reconciled immediately
- schedules with `next_run_at` missing but still enabled recompute it

This keeps the scheduler robust against daemon crashes between claim and dispatch.

## Manual Operations

V1 should support:

- create
- update
- enable
- disable
- delete
- run now

`run now` should:

- create an occurrence with `slot_at = now`
- use the same stateless dispatch path
- respect the same concurrency policy

## Operator Surface

### CLI

Add a focused command group:

```text
gistclaw schedule add
gistclaw schedule list
gistclaw schedule show <id>
gistclaw schedule run <id>
gistclaw schedule enable <id>
gistclaw schedule disable <id>
gistclaw schedule delete <id>
```

### Web

Add `Configure > Schedules`.

The page should show:

- name
- objective summary
- next due
- enabled/disabled
- last outcome
- run-now action

Schedule detail should show:

- schedule definition
- next due
- recent occurrences
- linked run IDs

This belongs under **Configure**, because schedules shape future runs.

## Error Handling

### Schedule Computation Errors

If a schedule definition cannot compute its next slot:

- increment `schedule_error_count`
- clear `next_run_at`
- set `last_error`

After repeated failures, auto-disable the schedule.

### Dispatch Errors

If runtime handoff fails after the slot is claimed:

- mark occurrence `failed`
- set occurrence error text
- increment schedule failure counters

### Approval Pauses

Approval is not a scheduler failure.

If a scheduled run pauses for approval:

- occurrence becomes `needs_approval`
- operator resolves it through the normal approvals surface
- later runtime events complete reconciliation

## Observability

At minimum, log:

- scheduler start/stop
- next wake time
- due schedule claims
- skipped slots
- dispatch start/failure
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
- dispatch through runtime async inbound path
- occurrence reconciliation from runtime events
- approval pause transitions
- crash repair for stale `dispatching` rows

## Rollout

Rollout should be staged:

1. schema + scheduler package + CLI
2. runtime event sink reconciliation
3. web list/detail pages

This keeps the first shipping slice narrow while still making scheduled runs
visible through existing run pages from day one.

## Future Expansion

These are explicit follow-ups, not V1 creep:

- persistent session-bound schedules
- scheduled inbound reminders into an existing conversation
- per-schedule model/team overrides
- failure alerts
- external delivery targets
- workflow-style multi-step automation

## Decision

Ship V1 as **stateless scheduled local tasks** backed by SQLite and dispatched
through the existing async inbound runtime path.

That gives GistClaw the useful part of scheduling now, without dragging in the
full OpenClaw cron product before this repo is ready for it.
