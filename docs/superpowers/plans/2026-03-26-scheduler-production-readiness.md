# Scheduler Production Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `gistclaw`'s first scheduler and harden it so scheduled runs survive restarts, avoid duplicate dispatch, stay observable to operators, and meet the repo's production bar.

**Architecture:** Add a small `internal/scheduler` package that owns schedule definitions, occurrence claiming, next-run computation, bounded startup catch-up, and reconciliation. The scheduler must never become a second run engine: it dispatches all work through `runtime.ReceiveInboundMessageAsync`, stores only scheduler-local state in SQLite, and mirrors authoritative runtime status back into occurrences. Wire it into `App.Start`, expose CLI-first management, and defer web UI and session-bound automation semantics.

**Tech Stack:** Go 1.25+, SQLite via `internal/store`, stdlib `context`/`time`/`database/sql`, `github.com/robfig/cron/v3`, Go `testing` package

---

## Current Assumption

As of `2026-03-26`, this `main` checkout still shows the scheduler as design-only. This plan therefore covers both:

1. shipping the scheduler backend slice
2. hardening it to production readiness in the same effort

If scheduler code exists elsewhere, keep the rollout order but collapse tasks whose files already exist.

## Guardrails

- Stay on `main`. Do not create branches or worktrees.
- Edit only `internal/store/migrations/001_init.sql` for schema changes.
- TDD always: each new behavior starts with a failing test.
- Keep the scheduler boring: SQLite + runtime seams only.
- No web schedule pages in this slice.
- No heartbeat-style prompts, `HEARTBEAT_OK`, or session-bound automation semantics.
- After schema changes, recreate the local database before manual verification.

## File Map

**Create**

- `internal/scheduler/types.go` — scheduler-local domain types, status constants, small runtime seam interfaces defined in the consuming package
- `internal/scheduler/schedule.go` — `at` / `every` / `cron` next-run calculation and validation helpers
- `internal/scheduler/store.go` — SQLite CRUD, due-claim, occurrence writes, reconciliation writes, startup repair reads
- `internal/scheduler/service.go` — timer loop, bounded catch-up, dispatch, reconciliation, lifecycle start/stop
- `internal/scheduler/schedule_test.go` — schedule math and validation coverage
- `internal/scheduler/store_test.go` — schema-backed CRUD, claim, repair, and reconciliation tests
- `internal/scheduler/service_test.go` — dispatch, catch-up, overlap skip, restart repair, and lifecycle tests
- `internal/app/schedules.go` — `App` wrapper methods for CLI schedule operations
- `internal/app/schedules_test.go` — app-layer scheduler wrapper tests
- `cmd/gistclaw/schedule.go` — `gistclaw schedule ...` CLI parser and printers
- `cmd/gistclaw/schedule_test.go` — CLI command tests

**Modify**

- `internal/store/migrations/001_init.sql` — add `schedules` and `schedule_occurrences` tables and indexes
- `internal/app/bootstrap.go` — construct scheduler service and store it on `App`
- `internal/app/lifecycle.go` — start/stop scheduler under `App.Start` ownership and include scheduler startup in prepare/start logs
- `internal/app/bootstrap_test.go` — replace the "no scheduler wired" expectation with positive wiring coverage
- `cmd/gistclaw/main.go` — add `schedule` subcommand to usage and dispatch
- `cmd/gistclaw/main_test.go` — usage and subcommand routing coverage
- `cmd/gistclaw/doctor.go` — add scheduler health checks only if the implementation cost stays low
- `cmd/gistclaw/doctor_test.go` — scheduler health output coverage if doctor is touched
- `README.md` — document CLI-first scheduled tasks once shipped
- `docs/system.md` — move scheduled tasks from gap to shipped runtime surface
- `docs/designs/2026-03-25-scheduled-tasks.md` — update only if the shipped design differs from this spec

## Not In Scope

- Dedicated web schedule pages
- Per-schedule front-agent selection
- Per-schedule model overrides
- Session-bound automation or main-session reminder semantics
- SSE schedule event fanout
- Connector-thread delivery policies for schedules

## Execution Order

Ship in this order:

1. Schema and scheduler package foundations
2. Deterministic schedule math
3. Store CRUD + due-claim + reconciliation
4. Service dispatch + bounded catch-up + startup repair
5. App lifecycle wiring
6. CLI management surface
7. Operator visibility and low-cost doctor checks
8. Docs + end-to-end verification

### Task 1: Add Scheduler Schema And Domain Types

**Files:**
- Create: `internal/scheduler/types.go`
- Test: `internal/scheduler/store_test.go`
- Modify: `internal/store/migrations/001_init.sql`

- [ ] **Step 1: Write the failing schema test**

```go
func TestSchedulerSchema_CreatesSchedulesAndOccurrencesTables(t *testing.T) {
	db := openTestDB(t)
	names := loadTableNames(t, db, "schedules", "schedule_occurrences")
	if !names["schedules"] || !names["schedule_occurrences"] {
		t.Fatalf("missing scheduler tables: %#v", names)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/scheduler -run TestSchedulerSchema_CreatesSchedulesAndOccurrencesTables -v`
Expected: FAIL because the scheduler tables do not exist yet

- [ ] **Step 3: Add schema and types**

Add `schedules` and `schedule_occurrences` to `internal/store/migrations/001_init.sql` using the shape from `docs/designs/2026-03-25-scheduled-tasks.md`.

In `internal/scheduler/types.go`, define:

```go
type ScheduleKind string
const (
	ScheduleKindAt    ScheduleKind = "at"
	ScheduleKindEvery ScheduleKind = "every"
	ScheduleKindCron  ScheduleKind = "cron"
)

type OccurrenceStatus string
const (
	OccurrenceDispatching   OccurrenceStatus = "dispatching"
	OccurrenceActive        OccurrenceStatus = "active"
	OccurrenceNeedsApproval OccurrenceStatus = "needs_approval"
	OccurrenceCompleted     OccurrenceStatus = "completed"
	OccurrenceFailed        OccurrenceStatus = "failed"
	OccurrenceInterrupted   OccurrenceStatus = "interrupted"
	OccurrenceSkipped       OccurrenceStatus = "skipped"
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/scheduler -run TestSchedulerSchema_CreatesSchedulesAndOccurrencesTables -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/migrations/001_init.sql internal/scheduler/types.go internal/scheduler/store_test.go
git commit -m "feat: add scheduler schema foundations"
```

### Task 2: Implement Deterministic Schedule Math

**Files:**
- Create: `internal/scheduler/schedule.go`
- Test: `internal/scheduler/schedule_test.go`

- [ ] **Step 1: Write the failing schedule tests**

```go
func TestComputeNextRun_AtScheduleFiresOnce(t *testing.T) {}
func TestComputeNextRun_EveryScheduleAnchorsToScheduledSlot(t *testing.T) {}
func TestComputeNextRun_CronScheduleUsesTimezone(t *testing.T) {}
func TestComputeNextRun_CronScheduleRejectsInvalidExpression(t *testing.T) {}
```

Include explicit cases for:

- future `at`
- expired `at`
- `every` anchored from scheduled slot, not completion time
- cron with IANA timezone
- invalid cron expression

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/scheduler -run TestComputeNextRun -v`
Expected: FAIL with missing scheduler math implementation

- [ ] **Step 3: Write minimal implementation**

Implement:

- `ValidateSpec(spec ScheduleSpec) error`
- `ComputeNextRun(spec ScheduleSpec, now time.Time) (time.Time, error)`
- pure-Go cron parsing with `robfig/cron/v3`
- no in-memory cron runner; parser/calculator only

Keep rules aligned with the design:

- `at` returns one future slot only
- `every` advances from scheduled slot
- `cron` returns the next future slot in the requested timezone

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/scheduler -run TestComputeNextRun -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/schedule.go internal/scheduler/schedule_test.go
git commit -m "feat: add scheduler next-run calculation"
```

### Task 3: Implement Store CRUD And Due-Claim Primitives

**Files:**
- Create: `internal/scheduler/store.go`
- Modify: `internal/store/migrations/001_init.sql`
- Test: `internal/scheduler/store_test.go`

- [ ] **Step 1: Write the failing store tests**

```go
func TestStore_CreateScheduleNormalizesSummaryFields(t *testing.T) {}
func TestStore_ListDueSchedulesOrdersByNextRunAt(t *testing.T) {}
func TestStore_ClaimDueOccurrenceCreatesSingleDispatchingRow(t *testing.T) {}
func TestStore_ClaimDueOccurrenceSkipsWhenPreviousOccurrenceActive(t *testing.T) {}
func TestStore_ClaimDueOccurrenceIsIdempotentPerSlot(t *testing.T) {}
```

Use the real SQLite schema and assert:

- `UNIQUE(schedule_id, slot_at)` prevents duplicate occurrences
- `next_run_at` advances inside the claim transaction
- overlap policy records `skip_reason = 'previous_occurrence_active'`

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/scheduler -run 'TestStore_' -v`
Expected: FAIL because the store APIs do not exist yet

- [ ] **Step 3: Write minimal store implementation**

Implement focused methods such as:

```go
func (s *Store) CreateSchedule(ctx context.Context, in CreateScheduleInput) (Schedule, error)
func (s *Store) UpdateSchedule(ctx context.Context, id string, patch UpdateScheduleInput) (Schedule, error)
func (s *Store) ListSchedules(ctx context.Context) ([]Schedule, error)
func (s *Store) LoadSchedule(ctx context.Context, id string) (Schedule, error)
func (s *Store) ClaimDueOccurrence(ctx context.Context, now time.Time) (*ClaimedOccurrence, error)
```

Do not write ad hoc SQL from service or CLI code. All scheduler writes stay here.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/scheduler -run 'TestStore_' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/store.go internal/scheduler/store_test.go internal/store/migrations/001_init.sql
git commit -m "feat: add scheduler store and due-claim logic"
```

### Task 4: Implement Reconciliation And Startup Repair Paths

**Files:**
- Modify: `internal/scheduler/store.go`
- Test: `internal/scheduler/store_test.go`

- [ ] **Step 1: Write the failing repair/reconcile tests**

```go
func TestStore_ReconcileOccurrenceMirrorsRunStatus(t *testing.T) {}
func TestStore_RepairDispatchingOccurrenceBackfillsRunFromInboundReceipt(t *testing.T) {}
func TestStore_RepairDispatchingOccurrenceMarksFailedAfterGracePeriod(t *testing.T) {}
func TestStore_RecomputeMissingNextRunAtForEnabledSchedule(t *testing.T) {}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/scheduler -run 'TestStore_(Reconcile|Repair)' -v`
Expected: FAIL because repair and reconciliation helpers are incomplete

- [ ] **Step 3: Extend the store with repair/reconcile helpers**

Implement methods for:

- loading non-terminal occurrences
- backfilling `run_id` / `conversation_id` via `inbound_receipts`
- mirroring `runs.status` into occurrence status
- updating parent schedule summary fields: `last_run_at`, `last_status`, `last_error`, `consecutive_failures`

Use runtime status as the only authoritative execution source.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/scheduler -run 'TestStore_(Reconcile|Repair)' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/store.go internal/scheduler/store_test.go
git commit -m "feat: add scheduler repair and reconciliation"
```

### Task 5: Implement Scheduler Service And Runtime Dispatch

**Files:**
- Create: `internal/scheduler/service.go`
- Test: `internal/scheduler/service_test.go`
- Modify: `internal/runtime/collaboration.go` only if a tiny helper seam is needed

- [ ] **Step 1: Write the failing service tests**

```go
func TestService_RunOnceClaimsAndDispatchesOccurrence(t *testing.T) {}
func TestService_RunOnceUsesReceiveInboundMessageAsyncWithOccurrenceID(t *testing.T) {}
func TestService_RunOncePersistsRunIDAndConversationIDAfterDispatch(t *testing.T) {}
func TestService_StartupCatchupClaimsAtMostOneRecurringOverdueSlot(t *testing.T) {}
func TestService_RunNowCoalescesDuplicateManualWakeRequests(t *testing.T) {}
```

Fake the runtime seam with a tiny consuming-package interface:

```go
type Dispatcher interface {
	ReceiveInboundMessageAsync(ctx context.Context, cmd runtime.InboundMessageCommand) (model.Run, error)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/scheduler -run 'TestService_' -v`
Expected: FAIL because the service loop and runtime handoff do not exist

- [ ] **Step 3: Write minimal service implementation**

Implement:

- `Start(ctx context.Context) error`
- `RunOnce(ctx context.Context) error`
- `RunNow(ctx context.Context, id string) error`
- bounded startup catch-up
- manual wake coalescing
- lifecycle-safe timer loop under caller-owned context

Dispatch each claimed occurrence through `runtime.ReceiveInboundMessageAsync` with:

```go
ConversationKey{
	ConnectorID: "schedule",
	AccountID:   "local",
	ExternalID:  "job:" + schedule.ID,
	ThreadID:    occurrence.ThreadID,
}
SourceMessageID = occurrence.ID
FrontAgentID    = "assistant"
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/scheduler -run 'TestService_' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/service.go internal/scheduler/service_test.go
git commit -m "feat: add scheduler service and runtime dispatch"
```

### Task 6: Wire Scheduler Into App Bootstrap And Lifecycle

**Files:**
- Create: `internal/app/schedules.go`
- Test: `internal/app/schedules_test.go`
- Modify: `internal/app/bootstrap.go`
- Modify: `internal/app/lifecycle.go`
- Modify: `internal/app/bootstrap_test.go`

- [ ] **Step 1: Write the failing app wiring tests**

```go
func TestBootstrap_WiresSchedulerService(t *testing.T) {}
func TestAppStart_StartsAndStopsSchedulerWithRunContext(t *testing.T) {}
func TestAppPrepare_RunsSchedulerRepairBeforeServing(t *testing.T) {}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app -run 'Test(Bootstrap_WiresSchedulerService|AppStart_StartsAndStopsSchedulerWithRunContext|AppPrepare_RunsSchedulerRepairBeforeServing)' -v`
Expected: FAIL because `App` does not own a scheduler yet

- [ ] **Step 3: Write minimal wiring**

Add a scheduler field to `App`, construct it in `Bootstrap`, and start it in `App.Start` under the same owned context as connectors and web. Do not spawn an unowned goroutine.

Expose app-layer wrappers in `internal/app/schedules.go`:

```go
func (a *App) ScheduleAdd(ctx context.Context, in scheduler.CreateScheduleInput) (scheduler.Schedule, error)
func (a *App) ScheduleList(ctx context.Context) ([]scheduler.Schedule, error)
func (a *App) ScheduleShow(ctx context.Context, id string) (scheduler.Schedule, error)
func (a *App) ScheduleRunNow(ctx context.Context, id string) error
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/app -run 'Test(Bootstrap_WiresSchedulerService|AppStart_StartsAndStopsSchedulerWithRunContext|AppPrepare_RunsSchedulerRepairBeforeServing)' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/bootstrap.go internal/app/lifecycle.go internal/app/bootstrap_test.go internal/app/schedules.go internal/app/schedules_test.go
git commit -m "feat: wire scheduler into app lifecycle"
```

### Task 7: Add CLI-First Schedule Management

**Files:**
- Create: `cmd/gistclaw/schedule.go`
- Test: `cmd/gistclaw/schedule_test.go`
- Modify: `cmd/gistclaw/main.go`
- Modify: `cmd/gistclaw/main_test.go`

- [ ] **Step 1: Write the failing CLI tests**

```go
func TestRun_ScheduleAdd(t *testing.T) {}
func TestRun_ScheduleList(t *testing.T) {}
func TestRun_ScheduleShow(t *testing.T) {}
func TestRun_ScheduleEnableDisableDelete(t *testing.T) {}
func TestRun_ScheduleRunNow(t *testing.T) {}
func TestRun_ScheduleStatus(t *testing.T) {}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/gistclaw -run 'TestRun_Schedule' -v`
Expected: FAIL because the `schedule` subcommand does not exist

- [ ] **Step 3: Implement the CLI**

Add:

- `gistclaw schedule add`
- `gistclaw schedule list`
- `gistclaw schedule show <id>`
- `gistclaw schedule run <id>`
- `gistclaw schedule enable <id>`
- `gistclaw schedule disable <id>`
- `gistclaw schedule delete <id>`
- `gistclaw schedule status`

Prefer explicit flags over clever parsing. Validate inputs in CLI, then pass normalized inputs to `App`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/gistclaw -run 'TestRun_Schedule' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/gistclaw/main.go cmd/gistclaw/main_test.go cmd/gistclaw/schedule.go cmd/gistclaw/schedule_test.go
git commit -m "feat: add scheduler CLI management"
```

### Task 8: Add Production Visibility And Low-Cost Health Checks

**Files:**
- Modify: `internal/app/schedules.go`
- Modify: `internal/app/schedules_test.go`
- Modify: `cmd/gistclaw/doctor.go`
- Modify: `cmd/gistclaw/doctor_test.go`

- [ ] **Step 1: Write the failing visibility tests**

```go
func TestScheduleStatus_ReturnsNextWakeAndCounts(t *testing.T) {}
func TestDoctor_WarnsOnBrokenSchedulerState(t *testing.T) {}
```

Only add doctor coverage if the implementation remains small and obvious.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app ./cmd/gistclaw -run 'Test(ScheduleStatus|Doctor_WarnsOnBrokenSchedulerState)' -v`
Expected: FAIL because scheduler status and health reporting are incomplete

- [ ] **Step 3: Implement status and doctor checks**

Add status outputs for:

- scheduler enabled/disabled
- total schedules
- due schedules
- active occurrences
- next wake time
- last failure summary if present

Doctor should flag:

- invalid stored schedule definitions
- stuck `dispatching` rows older than the grace period
- enabled schedules with missing `next_run_at`

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/app ./cmd/gistclaw -run 'Test(ScheduleStatus|Doctor_WarnsOnBrokenSchedulerState)' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/schedules.go internal/app/schedules_test.go cmd/gistclaw/doctor.go cmd/gistclaw/doctor_test.go
git commit -m "feat: add scheduler status and health checks"
```

### Task 9: Finalize Docs, Recreate Local DB, And Verify End To End

**Files:**
- Modify: `README.md`
- Modify: `docs/system.md`
- Modify: `docs/designs/2026-03-25-scheduled-tasks.md` only if code diverged

- [ ] **Step 1: Write the failing docs checklist**

Create a local checklist in the PR description or commit notes covering:

- README mentions schedule CLI
- system doc lists scheduler as shipped runtime surface
- no docs claim web schedule pages exist yet

- [ ] **Step 2: Recreate local DB after schema change**

Run:

```bash
rm -f runtime.db
go build -o bin/gistclaw ./cmd/gistclaw
```

Expected: clean build against the updated schema

- [ ] **Step 3: Run focused package tests**

Run:

```bash
go test ./internal/scheduler/...
go test ./internal/app/...
go test ./cmd/gistclaw/...
```

Expected: PASS

- [ ] **Step 4: Run full verification**

Run:

```bash
go test ./...
go test -cover ./...
go vet ./...
go build -o bin/gistclaw ./cmd/gistclaw
```

Expected:

- all tests pass
- total coverage stays at or above `70%`
- `go vet` is clean
- binary builds successfully

- [ ] **Step 5: Manual smoke test**

Run:

```bash
./bin/gistclaw schedule add --name "hourly check" --every 1h --workspace-root "$PWD" --objective "print repo status"
./bin/gistclaw schedule list
./bin/gistclaw serve
```

Verify:

- schedule is listed with a concrete next run
- serving process starts without scheduler startup errors
- a due schedule creates exactly one occurrence and one runtime run
- restart does not duplicate the same scheduled slot

- [ ] **Step 6: Commit**

```bash
git add README.md docs/system.md docs/designs/2026-03-25-scheduled-tasks.md
git commit -m "docs: document scheduler runtime surface"
```

## Acceptance Checklist

- [ ] Scheduler tables exist in `001_init.sql`
- [ ] `at`, `every`, and `cron` next-run calculation are covered by tests
- [ ] Due-claim is duplicate-safe and overlap=`skip` is enforced
- [ ] Dispatch always goes through `runtime.ReceiveInboundMessageAsync`
- [ ] Startup repair backfills `run_id` and `conversation_id` via `inbound_receipts`
- [ ] Restart catch-up is bounded
- [ ] Timer loop cannot hot-loop when state is stale
- [ ] `gistclaw schedule ...` CLI works end to end
- [ ] Operator can inspect scheduler health without web UI
- [ ] Full test suite, coverage gate, vet, and build all pass

## Suggested Commit Sequence

1. `feat: add scheduler schema foundations`
2. `feat: add scheduler next-run calculation`
3. `feat: add scheduler store and due-claim logic`
4. `feat: add scheduler repair and reconciliation`
5. `feat: add scheduler service and runtime dispatch`
6. `feat: wire scheduler into app lifecycle`
7. `feat: add scheduler CLI management`
8. `feat: add scheduler status and health checks`
9. `docs: document scheduler runtime surface`

## Execution Handoff

This plan intentionally keeps the first production slice CLI-first and backend-heavy. If implementation pressure rises, cut only the doctor enhancement before cutting any recovery or observability work.
