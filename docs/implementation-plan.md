# GistClaw V1 Runtime — Implementation Plan

> **Related:** [dependencies.md](dependencies.md) — Go version + dependency rationale | [12-go-package-structure.md](12-go-package-structure.md) — package ownership | [13-core-interfaces.md](13-core-interfaces.md) — locked interfaces | [README.md](README.md) — doc index
>
> **Per-milestone execution plans:** [M1](superpowers/plans/2026-03-24-m1-kernel-proof.md) | [M2](superpowers/plans/2026-03-24-m2-local-beta.md) | [M3](superpowers/plans/2026-03-24-m3-public-beta.md) | [M4](superpowers/plans/2026-03-24-m4-stable-1-0.md)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the gistclaw repo-task runtime from kernel proof through stable 1.0, with one calm local-first loop (gistclaw serve) that can later grow into Telegram DM without re-architecting the core.

**Architecture:** One Go binary (gistclaw). Five day-one domain packages separated from the run engine: internal/conversations (journal append path), internal/tools (tool registry + WorkspaceApplier), internal/model (shared types + RunEventSink), internal/replay (read-only projections), internal/memory (fact store). internal/runtime owns only run lifecycle and provider calls. The web SSE broadcaster implements model.RunEventSink — runtime never imports internal/web.

**Module path:** `github.com/canhta/gistclaw`

**Tech Stack:** Go 1.25+, stdlib net/http, html/template, log/slog, SSE for live updates, SQLite via modernc.org/sqlite (pure-Go, no CGO), go.yaml.in/yaml/v4 for config/team/soul parsing, Go testing + httptest, minimal Telegram REST client in Milestone 3. See `docs/dependencies.md` for the full dependency rationale and exclusion list.

---

## Global Guardrails

1. No WebSocket control plane in v1.
2. No transcript files or JSON side stores for core entities.
3. No plugin runtime.
4. No vector memory.
5. No Telegram groups.
6. No GitHub publish-back.
7. No autonomous background loops.
8. No hidden dynamic model router.
9. No writes outside one declared workspace root per root run.
10. No internal/runtime importing internal/web (dependency inversion violation).
11. No journal writes outside ConversationStore.AppendEvent.
12. Split trigger rule: split only when (a) clear external consumers AND (b) exported functions called by 2+ packages that would not otherwise need internal/runtime.

## Milestone Map

| Milestone | Name | Summary |
|-----------|------|---------|
| **1** | Kernel Proof | One daemon, SQLite journal, one provider adapter, CLI-driven runs, delegation, interrupted-run recovery |
| **2** | Local Beta | Local web UI, SSE live replay, onboarding wizard, preview/apply/verify loop, receipts |
| **3** | Public Beta | Team editor, visual composer, memory inspector, Telegram DM |
| **4** | Stable 1.0 | Doctor command, backup/export, approval polish, receipt polish, onboarding polish |

Each milestone must feel complete on its own. Do not start the next milestone until the current one has passing tests and a manual proof run.

---

## Locked V1 Tool Set

**In scope (v1 only):**

- repo read and search
- patch apply and file write inside one workspace root
- shell exec inside that workspace root
- git status, diff, and show
- test-command execution
- approval tickets
- structured completion and receipt emission

**Deferred (not in v1):**

- email, calendar, browser automation
- generic outbound HTTP by default
- plugin-provided tools

## Locked V1 Team Constraints

- v1 ships one editable starter team — not a fixed role catalog as the product center
- operators may rename agents and rewire handoffs
- the runtime validates handoff edges, tool profiles, memory scopes, and capability flags
- at least one agent must own workspace mutation
- at least one agent may be operator-facing
- most agents should stay read-heavy or propose-only by default

## Decision Rule

Whenever there is a choice between a feature that makes the first repo-task loop clearer and a feature that makes the platform broader, choose the first.

---

## Milestone 1 — Kernel Proof

**Ships:** one daemon, SQLite journal plus projections, one provider adapter, CLI-driven run surface, one root run plus one delegated child run, interrupted-run recovery, BudgetGuard with per-run and daily caps.

**Does NOT ship:** web UI, Telegram, approvals UI, memory editor.

**Exit criteria:**
- One repo task can run end to end (CLI-driven, no web UI required)
- The run is replayable from durable events
- The system records a receipt with model, tokens, cost, and verification result
- Restart moves unfinished runs to `interrupted`
- The memory read path is exercised on every turn (verified by structured log inspection showing a memory retrieval attempt, even if the result is empty)
- Per-run budget cap stops a run that exceeds its token/cost limit
- Daily cap check blocks a second run when rolling 24h usage is exhausted

---

### File: go.mod

**Responsibility:** Declares the Go module identity and minimum Go version. This is the single source of truth for the module path that all import statements reference. It is NOT responsible for pinning transitive dependency versions beyond what `go mod tidy` produces.

**Must contain:**
- `module github.com/canhta/gistclaw`
- `go 1.25` directive
- `require modernc.org/sqlite` (added after first `go mod tidy`)

**Invariants:**
- The module path is always `github.com/canhta/gistclaw` — never `gistclaw` or any other form
- The Go version directive is always `1.24` or higher

- [ ] Step 1: Initialize the module.
  Run: `go mod init github.com/canhta/gistclaw`
  Expected: `go.mod` exists with correct module path and go directive.
- [ ] Step 2: Verify module resolves.
  Run: `go test ./...`
  Expected: exits cleanly (may report no test files, not an error).
- [ ] Commit: `init go module`

---

### File: cmd/gistclaw/main.go

**Responsibility:** CLI entrypoint for the `gistclaw` binary. Parses the top-level subcommand (`serve`, `run`, `inspect`, `doctor`, `backup`, `export`) and dispatches to the appropriate handler in internal/app. This file owns argument parsing and nothing else — no business logic, no config loading, no direct DB access.

**Key exports:**
- `func main()` — subcommand dispatch

**Invariants:**
- main.go contains no business logic; every subcommand delegates to internal/app or another internal package
- Unknown subcommands print usage and exit with code 1
- The binary name is `gistclaw` (not `gistclaw`)

- [ ] Step 1: Write failing test in `cmd/gistclaw/main_test.go`.
  Test `TestMain_UnknownSubcommand`: invoke with an unknown subcommand, verify it exits with code 1.
  Run: `go test ./cmd/gistclaw -run TestMain_UnknownSubcommand -v`
  Expected: FAIL (no implementation yet).
- [ ] Step 2: Implement main.go with subcommand dispatch.
  Parse os.Args for `serve`, `run`, `inspect`. Each dispatches to a function in internal/app. Unknown subcommand prints usage.
  Run: `go test ./cmd/gistclaw -run TestMain -v`
  Expected: PASS.
- [ ] Commit: `add gistclaw cli entrypoint`

---

### File: internal/app/config.go

**Responsibility:** Loads and validates the daemon configuration from a YAML file. Provides typed access to workspace root, provider settings, listen address, and database path. This file is NOT responsible for applying config at runtime or watching for changes.

**Key exports:**
- `type Config struct` — workspace_root, provider, listen_addr, db_path, budget fields
- `func LoadConfig(path string) (Config, error)` — parse YAML, validate required fields

**Invariants:**
- Missing workspace_root returns a validation error naming the field
- Missing provider config returns a validation error naming the field
- Default listen address is `127.0.0.1:8374`
- Default database path is `./gistclaw.db`

- [ ] Step 1: Write failing tests in `internal/app/config_test.go`.
  - `TestConfig_DefaultPath`: loading with no explicit path uses `./config.yaml`; returns error if file missing.
  - `TestConfig_ExplicitPath`: loading with `-config /tmp/test.yaml` reads that path.
  - `TestConfig_MissingWorkspaceRoot`: config file exists but `workspace_root` is empty; returns a validation error naming the missing field.
  - `TestConfig_MissingProviderConfig`: config file exists but no provider section; returns a validation error naming the missing field.
  Run: `go test ./internal/app -run TestConfig -v`
  Expected: all four tests FAIL.
- [ ] Step 2: Implement LoadConfig with YAML parsing and validation.
  Run: `go test ./internal/app -run TestConfig -v`
  Expected: all four tests PASS.
- [ ] Commit: `add config loading`

---

### File: internal/app/bootstrap.go

**Responsibility:** Pure wiring file that creates domain objects and hands them references. Calls domain-scoped wiring functions in order: storeWiring, runtimeWiring, replayWiring, webWiring. This file is NOT responsible for any business logic — it only connects pieces.

**Key exports:**
- `func Bootstrap(cfg Config) (*App, error)` — orchestrates all wiring
- `func storeWiring(cfg Config) (*store.DB, error)`
- `func runtimeWiring(cfg Config, db *store.DB, mem *memory.Store, sink model.RunEventSink) (*runtime.Runtime, error)`
- `func replayWiring(db *store.DB) (*replay.Service, error)`
- `func webWiring(cfg Config, rt *runtime.Runtime, rp *replay.Service, sse *web.SSEBroadcaster) (*web.Server, error)`

**Invariants:**
- No function in bootstrap.go exceeds 30 lines
- Bootstrap passes `model.RunEventSink` (the interface) to runtime, never `*web.SSEBroadcaster`
- Each wiring function is independently testable with stub arguments

- [ ] Step 1: Write failing tests in `internal/app/bootstrap_test.go`.
  - `TestBootstrap_WiringFunctionsExist`: call each wiring function signature with nil/stub args; verify they return typed results.
  - `TestBootstrap_NoFunctionOver30Lines`: static analysis or manual assertion that no function in bootstrap.go exceeds 30 lines.
  Run: `go test ./internal/app -run TestBootstrap -v`
  Expected: FAIL.
- [ ] Step 2: Implement bootstrap.go with four wiring functions. Each creates its dependencies and returns a typed result. Bootstrap calls them in sequence.
  Run: `go test ./internal/app -run TestBootstrap -v`
  Expected: PASS.
- [ ] Commit: `add bootstrap wiring`

---

### File: internal/app/lifecycle.go

**Responsibility:** Manages daemon startup sequence and graceful shutdown on SIGINT/SIGTERM. Runs migration check before serving. Coordinates shutdown of HTTP server, provider calls, and DB handle. This file is NOT responsible for config loading or dependency wiring.

**Key exports:**
- `func (a *App) Run(ctx context.Context) error` — start serving, block until signal
- `func (a *App) Shutdown(ctx context.Context) error` — graceful teardown

**Invariants:**
- Migration check runs before any HTTP handler accepts requests
- SIGINT triggers graceful shutdown with a 30-second timeout
- ReconcileInterrupted runs before accepting new work
- Shutdown closes DB handle last (after HTTP server and pending operations)

- [ ] Step 1: Write failing test in `internal/app/lifecycle_test.go`.
  - `TestLifecycle_GracefulShutdown`: start daemon, send SIGINT, verify HTTP server stops accepting connections and DB closes cleanly within 5 seconds.
  Run: `go test ./internal/app -run TestLifecycle -v`
  Expected: FAIL.
- [ ] Step 2: Implement Run and Shutdown. Run calls ReconcileInterrupted, then starts HTTP server. Shutdown stops server, waits for in-flight work, closes DB.
  Run: `go test ./internal/app -run TestLifecycle -v`
  Expected: PASS.
- [ ] Commit: `add daemon lifecycle`

---

### File: internal/store/db.go

**Responsibility:** Opens and configures the SQLite database connection. Applies WAL mode, busy_timeout, and foreign_keys pragmas on every connection open. Provides a transaction helper that commits or rolls back. Handles SQLITE_FULL gracefully. This file is NOT responsible for schema migrations or query logic.

**Key exports:**
- `type DB struct` — wraps the SQLite connection
- `func Open(path string) (*DB, error)` — open with pragmas
- `func (db *DB) Tx(ctx context.Context, fn func(tx *sql.Tx) error) error` — transaction helper
- `func (db *DB) Close() error`

**Invariants:**
- WAL mode is always enabled (PRAGMA journal_mode=WAL)
- busy_timeout is always 5000ms
- foreign_keys is always ON
- SQLITE_FULL during commit attempts to journal an error event, then marks run as interrupted; if even that fails, logs structured error and returns (no panic)
- The transaction helper either commits or rolls back — no dangling transactions

- [ ] Step 1: Write failing tests in `internal/store/db_test.go`.
  - `TestDB_WALMode`: open a database, query `PRAGMA journal_mode`; verify it returns "wal".
  - `TestDB_ForeignKeys`: open a database, query `PRAGMA foreign_keys`; verify it returns 1.
  - `TestDB_TxCommitsOnSuccess`: run a transaction that inserts a row; verify the row is visible after Tx returns.
  - `TestDB_TxRollbackOnError`: run a transaction that inserts a row then returns an error; verify the row is NOT visible.
  Run: `go test ./internal/store -run TestDB -v`
  Expected: all four tests FAIL.
- [ ] Step 2: Implement Open with pragma application and Tx with commit/rollback.
  Run: `go test ./internal/store -run TestDB -v`
  Expected: all four tests PASS.
- [ ] Commit: `add sqlite connection and tx helper`

---

### File: internal/store/migrate.go

**Responsibility:** Reads embedded SQL migration files and applies them sequentially against the database. Tracks the current schema version in a `schema_version` table. Supports idempotent re-runs and refuses to silently downgrade. This file is NOT responsible for writing the migration SQL itself or for any runtime query logic.

**Key exports:**
- `func Migrate(db *DB) error` — apply all pending migrations
- `func SchemaVersion(db *DB) (int, error)` — return current version

**Invariants:**
- Migrations are applied in filename order (001, 002, ...)
- Running Migrate twice on the same database does not error or duplicate tables
- A database with a future schema version returns a clear error (not a silent downgrade)
- Migration files are embedded via `//go:embed`

- [ ] Step 1: Write failing tests in `internal/store/migrate_test.go`.
  - `TestMigrate_FreshBoot`: new database applies all migrations, schema version matches expected.
  - `TestMigrate_IdempotentRerun`: calling migrate twice on the same database does not error or duplicate tables.
  - `TestMigrate_SchemaVersionMismatch`: a database with a future schema version returns a clear error.
  - `TestMigrate_ConcurrentReadWrite`: open a WAL-mode database, start a read transaction, write an event in a separate goroutine; assert neither blocks or errors within 5 seconds.
  Run: `go test ./internal/store -run TestMigrate -v`
  Expected: all four tests FAIL.
- [ ] Step 2: Implement Migrate with embedded SQL file reading, sequential application, and version tracking.
  Run: `go test ./internal/store -run TestMigrate -v`
  Expected: all four tests PASS.
- [ ] Commit: `add migration runner`

---

### File: internal/store/migrations/001_init.sql

**Responsibility:** Creates all day-one tables for the append-only journal, run state, delegations, tool calls, approvals, receipts, durable memory, outbound intents, and settings. This file is NOT responsible for indexes, projections, or the schedules table (deferred to later migrations).

**Must contain tables:**
- `events` — id, conversation_id, run_id, parent_run_id, kind, payload_json, created_at
- `runs` — run_id, agent_id, team_id, conversation_id, parent_run_id, objective, workspace_root, tool_profile, memory_scope, max_depth, max_active_children, status, execution_snapshot_id, input_tokens, output_tokens, created_at, updated_at
- `delegations` — id, parent_run_id, child_run_id, status, created_at
- `tool_calls` — id, run_id, tool_name, args_json, result_json, approval_id, created_at
- `approvals` — id, run_id, tool_name, args_fingerprint, status, created_at, resolved_at
- `receipts` — id, run_id, model_lane, input_tokens, output_tokens, cost, verification_status, created_at
- `memory_items` — id, agent_id, scope, content, provenance, source, confidence, dedupe_key, created_at, updated_at
- `outbound_intents` — id, run_id, connector_id, chat_id, message_text, dedupe_key, status, retries, created_at, updated_at
- `settings` — key, value, updated_at

**Invariants:**
- All tables use TEXT primary keys (UUIDs or ULIDs), not INTEGER AUTOINCREMENT
- `events` table has created_at NOT NULL for ordering guarantees
- No `schedules` table — that is `003_scheduler.sql` in Milestone 3

- [ ] Step 1: Write the SQL file with all nine tables and NOT NULL constraints.
- [ ] Step 2: Verify via migrate test that all tables are created.
  Run: `go test ./internal/store -run TestMigrate_FreshBoot -v`
  Expected: PASS.
- [ ] Commit: `add init migration`

---

### File: internal/store/migrations/002_projections.sql

**Responsibility:** Creates read-optimized projection tables and indexes that support replay queries, active-run enforcement, and memory retrieval. These projections are maintained transactionally from journal state changes. This file is NOT responsible for the base tables (001) or the scheduler (003).

**Must contain:**
- `run_summaries` projection table — working context per run, auto-generated during compaction
- Index: `events(run_id, created_at)` — replay and live-replay queries
- Index: `runs(conversation_id, status)` — one active root run per conversation enforcement
- Index: `delegations(parent_run_id, status)` — active/queued children for a root run
- Index: `approvals(run_id, status)` — pending approvals for a run
- Index: `memory_items(agent_id, scope)` — memory retrieval by scope

**Invariants:**
- All indexes are CREATE INDEX IF NOT EXISTS (idempotent)
- Projection tables are populated by application code during AppendEvent, not by SQL triggers

- [ ] Step 1: Write the SQL file with the projection table and five indexes.
- [ ] Step 2: Verify via migrate test.
  Run: `go test ./internal/store -run TestMigrate_FreshBoot -v`
  Expected: PASS (both 001 and 002 applied).
- [ ] Commit: `add projection indexes`

---

### File: internal/model/types.go

**Responsibility:** Defines all shared domain types that multiple packages need to import without creating cycles. This is the dependency-break package — both internal/runtime and internal/web can import it. Contains ProviderError, RunEventSink interface, and all shared value types. This file is NOT responsible for any behavior or persistence logic.

**Key exports:**
- `type ProviderErrorCode string` and five constants: `ErrRateLimit`, `ErrContextWindowExceeded`, `ErrModelRefusal`, `ErrProviderTimeout`, `ErrMalformedResponse`
- `type ProviderError struct` — Code, Message, Retryable; implements `error` interface
- `type RunEventSink interface` — `Emit(ctx, runID, evt ReplayDelta) error`
- `type ReplayDelta struct` — event payload pushed to SSE clients
- Shared domain types: `RunProfile`, `RunPhase`, `AgentProfile`, `Envelope`, `Event`, `EventKind`, `ToolSpec`, `ToolRisk`, `SideEffectClass`, `ApprovalMode`, `DecisionMode`, `ToolDecision`, `ToolCall`, `ToolResult`, `ApprovalRequest`, `ApprovalTicket`, `ApprovalDecision`, `MemoryItem`, `MemoryQuery`, `UsageRecord`, `RunReceipt`, `ChangePreview`, `ApplyResult`

**Invariants:**
- All five ProviderErrorCode constants have distinct non-empty string values
- `ProviderError.Error()` returns `string(e.Code) + ": " + e.Message`
- RunEventSink has exactly one method (Emit) — no transport details leak into the interface
- No types in this file depend on internal/store, internal/runtime, or internal/web

- [ ] Step 1: Write failing tests in `internal/model/types_test.go`.
  - `TestProviderError_SatisfiesErrorInterface`: construct a ProviderError, call `.Error()`, verify string contains code and message.
  - `TestProviderError_AllFiveCodesExist`: assert all five ProviderErrorCode constants are defined and distinct.
  - `TestRunEventSink_InterfaceShape`: define a mock that implements RunEventSink; assert it compiles and Emit can be called.
  Run: `go test ./internal/model -run 'TestProviderError|TestRunEventSink' -v`
  Expected: all three tests FAIL.
- [ ] Step 2: Implement types.go with all types listed above.
  Run: `go test ./internal/model -run 'TestProviderError|TestRunEventSink' -v`
  Expected: all three tests PASS.
- [ ] Commit: `add shared model types and provider error`

---

### File: internal/conversations/keys.go

**Responsibility:** Defines the ConversationKey struct and its normalization logic. Produces a stable, URL-safe, log-safe string form from four fields. This file is NOT responsible for persistence, event appending, or run arbitration.

**Key exports:**
- `type ConversationKey struct` — ConnectorID, AccountID, ExternalID, ThreadID (four fields, NO TeamID)
- `func NormalizeKey(env model.Envelope) ConversationKey`
- `func (k ConversationKey) String() string` — stable normalized form

**Invariants:**
- ConversationKey has exactly four fields — no TeamID
- Missing ThreadID normalizes to `"main"`
- ActorID from the Envelope does NOT change the key
- The same inbound event always resolves to the same canonical key string
- The normalized string is safe for logs, indexes, receipts, and URL paths

- [ ] Step 1: Write failing tests in `internal/conversations/conversations_test.go`.
  - `TestConversation_NormalizesKey`: same inbound event always resolves to the same canonical key string.
  - `TestConversation_MissingThreadDefaultsToMain`: Envelope with empty ThreadID produces key with ThreadID = "main".
  - `TestConversation_ActorDoesNotChangeKey`: two Envelopes differing only in ActorID produce the same key.
  - `TestConversation_TeamReassignmentDoesNotChangeKey`: ConversationKey has no TeamID field (compile-time check: struct literal with only 4 fields compiles).
  Run: `go test ./internal/conversations -run TestConversation -v`
  Expected: all four tests FAIL.
- [ ] Step 2: Implement NormalizeKey and String methods.
  Run: `go test ./internal/conversations -run TestConversation -v`
  Expected: all four tests PASS.
- [ ] Commit: `add canonical conversation keys`

---

### File: internal/conversations/service.go

**Responsibility:** Implements ConversationStore — the single canonical journal append path for ALL event types. Manages conversation resolution, event listing, and active-root-run arbitration. This file is NOT responsible for conversation key normalization (that is keys.go) or run lifecycle logic (that is internal/runtime).

**Key exports:**
- `type ConversationStore struct`
- `func (s *ConversationStore) Resolve(ctx, key) (Conversation, error)` — find or create
- `func (s *ConversationStore) AppendEvent(ctx, evt) error` — single canonical journal append
- `func (s *ConversationStore) ListEvents(ctx, conversationID, limit) ([]Event, error)`
- `func (s *ConversationStore) ActiveRootRun(ctx, conversationID) (RunRef, error)`

**Invariants:**
- AppendEvent appends the event and updates projections atomically in one store transaction
- No package other than ConversationStore may append to the events table directly
- Each canonical conversation may have at most one active root run at a time
- Resolve is idempotent — calling it twice with the same key returns the same conversation

- [ ] Step 1: Write failing test.
  - `TestConversation_ActiveRootRunArbitration`: starting a second root run for the same conversation returns an error while the first is active.
  Run: `go test ./internal/conversations -run TestConversation_ActiveRootRunArbitration -v`
  Expected: FAIL.
- [ ] Step 2: Implement ConversationStore with Resolve, AppendEvent, ListEvents, ActiveRootRun. AppendEvent uses store.DB.Tx for atomicity.
  Run: `go test ./internal/conversations -run TestConversation -v`
  Expected: all five tests PASS (including keys.go tests).
- [ ] Commit: `add conversation store`

---

### File: internal/runtime/types.go

**Responsibility:** Defines runtime-internal types used by the run engine, delegation logic, and provider adapters. These types are not shared outside internal/runtime — shared types live in internal/model. This file is NOT responsible for any behavior or persistence.

**Key exports:**
- `type Runtime struct` — the run engine; holds ConversationStore, RunEventSink, Provider, ToolRegistry, MemoryStore, BudgetGuard references
- `type StartRun struct`, `type ContinueRun struct`, `type DelegateRun struct`, `type ResumeRun struct` — command types
- `type ReconcileReport struct` — result of interrupted-run reconciliation

**Invariants:**
- Runtime constructor requires a `model.RunEventSink` (interface), never a concrete SSE type
- Runtime holds a ConversationStore reference for journal writes, never a `*store.DB`
- All command types carry enough context for the engine to execute without querying external state

- [ ] Step 1: Define the types file alongside runs.go implementation (no separate test needed — types are validated by runs_test.go).
- [ ] Commit: (included in runs.go commit)

---

### File: internal/runtime/runs.go

**Responsibility:** Implements the run engine: Start, Continue, Delegate, Resume, ReconcileInterrupted. Manages the core run loop including provider calls, context compilation with memory retrieval, proactive context-window compaction, and BudgetGuard enforcement. This file is NOT responsible for delegation queue management (delegations.go), tool dispatch (internal/tools), or conversation key normalization (internal/conversations).

**Key exports:**
- `func NewRuntime(cs *conversations.ConversationStore, sink model.RunEventSink, prov Provider, tools *tools.Registry, mem *memory.Store, bg BudgetGuard) *Runtime`
- `func (rt *Runtime) Start(ctx, cmd StartRun) (Run, error)`
- `func (rt *Runtime) Continue(ctx, cmd ContinueRun) (Run, error)`
- `func (rt *Runtime) Delegate(ctx, cmd DelegateRun) (Run, error)`
- `func (rt *Runtime) Resume(ctx, cmd ResumeRun) (Run, error)`
- `func (rt *Runtime) ReconcileInterrupted(ctx) (ReconcileReport, error)`

**Invariants:**
- Runtime calls `conversationStore.AppendEvent` for ALL lifecycle events — never writes to store.DB directly
- Runtime calls `sink.Emit(ctx, runID, event)` after each durable journal write
- All provider errors are `*model.ProviderError` — runtime switches on Code only, never type-asserts vendor errors
- Memory retrieval (Search) is called during context compilation for every turn, even when the store is empty
- Proactive context-window compaction fires when cumulative tokens exceed 75% of the model's declared context window
- BudgetGuard.BeforeTurn is called before every provider call; BudgetGuard.RecordUsage is called after every provider call
- Budget exhaustion fails closed — a run that exceeds its cap is stopped, not allowed to continue

**BudgetGuard integration (all 4 methods):**

BudgetGuard is constructed by bootstrap and injected into Runtime. The run engine calls BudgetGuard at these points:

1. **BeforeTurn(ctx, run RunProfile) error** — called before every provider call. If the per-run token/cost cap is exhausted, returns an error. The run engine journals a `budget_exhausted` event and transitions the run to `stopped`. The run is NOT marked as failed — it completed its budget.

2. **RecordUsage(ctx, runID, usage UsageRecord) error** — called after every provider call with the actual tokens consumed and cost. Persists the usage record durably so that CheckDailyCap and receipt building have accurate data.

3. **CheckDailyCap(ctx, accountID) error** — called before Start to verify the operator has remaining daily budget. Uses a rolling 24-hour window (NOT UTC midnight reset). If the cap is exhausted, Start returns an error and no run is created. Conservative default cap is enabled before the operator customizes it.

4. **RecordIdleBurn(ctx, runID, duration) error** — called when a run is in a waiting state (e.g., blocked on approval) with open model context. Defaults to zero burn. Only called if the provider charges for idle context retention. Most runs will never trigger this.

**BudgetGuard rules:**
- Active-child budget is NOT part of BudgetGuard — it is enforced inside delegations.go
- Budget exhaustion fails closed for all cap types (per-run, daily)
- Conservative default caps are enabled before operator customization
- Raising or disabling caps requires an explicit operator action
- Rolling 24h cap aggregates from durable UsageRecord history, not in-memory counters

- [ ] Step 1: Write failing run lifecycle tests in `internal/runtime/runs_test.go`.
  - `TestRun_CreateAndComplete`: create a root run, execute one turn with MockProvider, verify run transitions to "completed", verify a receipt row exists.
  - `TestRun_LifecycleEventsJournaled`: create and complete a run, list events from ConversationStore, verify lifecycle events (run_started, turn_completed, run_completed) appear in order.
  - `TestRun_SinkEmitCalled`: create a run with a mock RunEventSink, verify Emit was called for each lifecycle event after journal write.
  - `TestRun_NeverImportsWeb`: use `go list -deps ./internal/runtime` to verify internal/web does not appear.
  - `TestRun_MemoryRetrievalEveryTurn`: complete a multi-turn run, verify memory.Search was called for each turn (mock memory store records calls).
  Run: `go test ./internal/runtime -run 'TestRun_Create|TestRun_Lifecycle|TestRun_Sink|TestRun_NeverImports|TestRun_Memory' -v`
  Expected: all five tests FAIL.
- [ ] Step 2: Implement MockProvider in `internal/runtime/provider_test.go`. MockProvider implements Provider with deterministic responses and configurable error returns.
- [ ] Step 3: Write failing provider error handling tests in `internal/runtime/provider_test.go`.
  - `TestProvider_RateLimit`: MockProvider returns ErrRateLimit; engine retries with exponential backoff up to 3 times, then fails the run with error event.
  - `TestProvider_ContextWindowExceeded`: MockProvider returns ErrContextWindowExceeded; engine triggers compaction and retries once; if still exceeded, fails with named error.
  - `TestProvider_ModelRefusal`: MockProvider returns ErrModelRefusal; engine surfaces it as a run event, does NOT silently retry.
  - `TestProvider_Timeout`: MockProvider returns ErrProviderTimeout; engine retries once, then fails.
  - `TestProvider_MalformedResponse`: MockProvider returns ErrMalformedResponse; engine fails the step, logs full context.
  - `TestProvider_AdapterWrapsError`: MockProvider emits a raw error; adapter wraps it in `*model.ProviderError` before it reaches runs.go.
  Run: `go test ./internal/runtime -run TestProvider -v`
  Expected: all six tests FAIL.
- [ ] Step 4: Write failing BudgetGuard tests in `internal/runtime/runs_test.go`.
  - `TestBudgetGuard_PerRunCapExhaustion`: configure a BudgetGuard with a low per-run cap (e.g., 100 tokens). Run enough turns to exceed it. Verify the run is stopped with a `budget_exhausted` event in the journal, not marked as failed.
  - `TestBudgetGuard_DailyCapBlocksNewRun`: configure a BudgetGuard with a low daily cap. Complete one run that uses most of the cap. Attempt to Start a second run. Verify Start returns an error and no run is created.
  - `TestBudgetGuard_IdleBurnZeroWhenNotWaiting`: complete a run that never enters a waiting state. Verify RecordIdleBurn was never called (or called with zero duration).
  - `TestBudgetGuard_ActiveChildBudgetSeparate`: verify that active-child budget enforcement is in delegations.go, not in BudgetGuard. Start a run, exhaust BudgetGuard, verify delegation queue still functions (BudgetGuard does not coordinate concurrency).
  Run: `go test ./internal/runtime -run TestBudgetGuard -v`
  Expected: all four tests FAIL.
- [ ] Step 5: Write failing compaction test.
  - `TestRun_CompactionAt75Percent`: configure MockProvider with 1000-token context window. Accumulate > 750 tokens. Verify compaction fires before next call and the subsequent request is below the threshold.
  Run: `go test ./internal/runtime -run TestRun_Compaction -v`
  Expected: FAIL.
- [ ] Step 6: Implement runs.go with Start, Continue, Resume, ReconcileInterrupted, BudgetGuard integration, and compaction logic. Implement provider.go with Provider interface and adapter.
  Run: `go test ./internal/runtime -run 'TestRun|TestProvider|TestBudgetGuard' -v`
  Expected: all tests PASS.
- [ ] Commit: `implement run engine with budget guard`

---

### File: internal/runtime/delegations.go

**Responsibility:** Manages child-run creation, handoff-edge validation against the frozen execution snapshot, SQLite-backed delegation queue with slot allocation, and active-child budget enforcement. This file is NOT responsible for root-run lifecycle (runs.go), provider calls, or BudgetGuard (which handles per-run and daily caps, not concurrency).

**Key exports:**
- `func (rt *Runtime) createDelegation(ctx, cmd DelegateRun) (Run, error)` — called by Delegate in runs.go
- `func (rt *Runtime) promoteQueuedDelegation(ctx, parentRunID) error` — promote oldest queued child when a slot opens
- `func (rt *Runtime) freezeExecutionSnapshot(ctx, teamYAML []byte, runID string) error` — store snapshot at root-run start

**Invariants:**
- Handoff-edge validation reads from the frozen execution snapshot (DB row), NOT the live team.yaml
- Execution snapshot is stored as a DB row at root-run start; child runs inherit the same snapshot
- Active-child budget (max_active_children) is enforced here, NOT in BudgetGuard
- When all slots are full, new delegations become `status='queued'` rows in the delegations table (SQL-based, not in-memory channel)
- When a child completes, the oldest queued delegation is promoted to active
- Delegation to an undeclared handoff target fails with a named error event, but does NOT interrupt the root run (recoverable failure)
- One active root run per conversation, enforced via ConversationStore

- [ ] Step 1: Write failing delegation tests in `internal/runtime/delegations_test.go`.
  - `TestDelegation_ParentChildLinkage`: delegate from a root run; verify child has parent_run_id set, delegation edge exists.
  - `TestDelegation_BudgetLimit`: set max_active_children=2; delegate 3; verify only 2 active, 1 queued.
  - `TestDelegation_QueuedChildWhenSlotsFull`: all slots full, new delegation creates queued row; when a child completes, queued becomes active.
  - `TestDelegation_HandoffEdgeRejection`: agent A delegates to agent C where A->C is NOT in team spec; assert rejected, error event journaled, root run NOT interrupted.
  - `TestDelegation_SnapshotImmutability`: start root run, edit team.yaml on disk, assert active run's handoff edges unchanged (reads from DB snapshot).
  - `TestDelegation_QueuedVisibleAfterRestart`: create queued delegations, simulate restart, verify queued delegations are visible alongside interrupted runs.
  Run: `go test ./internal/runtime -run TestDelegation -v`
  Expected: all six tests FAIL.
- [ ] Step 2: Implement execution snapshot freezing at root-run start.
- [ ] Step 3: Implement delegation with handoff-edge validation from frozen snapshot.
- [ ] Step 4: Implement SQLite-backed delegation queue with slot allocation and promotion.
- [ ] Step 5: Implement one-active-root-run enforcement via ConversationStore.ActiveRootRun.
- [ ] Step 6: Implement interrupted reconciliation (ReconcileInterrupted in runs.go calls this for delegation cleanup).
  Run: `go test ./internal/runtime -run TestDelegation -v`
  Expected: all six tests PASS.
- [ ] Commit: `add delegation and recovery rules`

---

### File: internal/runtime/provider.go

**Responsibility:** Defines the Provider interface and implements concrete provider adapters. Translates all vendor-specific errors into `*model.ProviderError` before returning to the run engine. Includes MockProvider for testing. This file is NOT responsible for run lifecycle or context compilation.

**Key exports:**
- `type Provider interface` — `ID() string`, `Capabilities() ModelCapabilities`, `Generate(ctx, req GenerateRequest, stream StreamSink) (GenerateResult, error)`
- `type GenerateRequest struct` — compiled instructions, conversation context, tool specs, model id, temperature
- `type GenerateResult struct` — response text, tool calls, usage
- `type MockProvider struct` — deterministic test provider

**Invariants:**
- All five ProviderErrorCode cases are handled by every adapter
- Adapters may not return raw vendor errors to callers outside the provider package
- `Retryable` on ProviderError is advisory; the run engine applies its own retry budget
- MockProvider is in `provider_test.go` (test-only), not shipped in the binary

- [ ] Step 1: (Tests written as part of runs.go steps above.)
- [ ] Step 2: Implement Provider interface and one concrete adapter (or stub for Milestone 1).
  Run: `go test ./internal/runtime -run TestProvider -v`
  Expected: PASS.
- [ ] Commit: (included in runs.go commit)

---

### File: internal/tools/registry.go

**Responsibility:** Maintains the tool registry mapping tool names to Tool implementations. Evaluates tool policy (allow/ask/deny) based on agent capability flags and tool risk/side-effect class. This file is NOT responsible for tool execution (runner.go), approval tickets (approvals.go), or workspace enforcement (workspace.go).

**Key exports:**
- `type Registry struct`
- `func (r *Registry) Register(tool Tool)`
- `func (r *Registry) Lookup(name string) (Tool, bool)`
- `func (r *Registry) Decide(ctx, agent AgentProfile, run RunProfile, tool ToolSpec) ToolDecision`

**Invariants:**
- Every agent starts from one built-in tool profile (read-only, read-heavy, workspace-write, operator-facing, elevated)
- Operators may add per-tool overrides without redefining the entire policy
- A read-heavy agent requesting a write tool gets decision "deny"
- A workspace-write agent requesting a write tool gets decision "ask" (requires approval)
- A read-only tool always gets decision "allow" regardless of agent profile

- [ ] Step 1: Write failing tests in `internal/tools/tools_test.go`.
  - `TestToolPolicy_AllowReadOnly`: read-only tool invoked by read-heavy agent returns "allow".
  - `TestToolPolicy_AskWriteTool`: file-write tool invoked by workspace-write agent returns "ask".
  - `TestToolPolicy_DenyEscalation`: file-write tool invoked by read-heavy agent returns "deny".
  Run: `go test ./internal/tools -run TestToolPolicy -v`
  Expected: all three tests FAIL.
- [ ] Step 2: Implement Registry with Register, Lookup, and Decide.
  Run: `go test ./internal/tools -run TestToolPolicy -v`
  Expected: all three tests PASS.
- [ ] Commit: `add tool registry and policy`

---

### File: internal/tools/approvals.go

**Responsibility:** Creates and resolves approval tickets with snapshot binding. Each ticket is bound to a specific action via a fingerprint. Tickets expire when the action changes. This file is NOT responsible for the approval UI (internal/web), tool execution, or workspace enforcement.

**Key exports:**
- `type ApprovalGate struct`
- `func (g *ApprovalGate) Request(ctx, req ApprovalRequest) (ApprovalTicket, error)`
- `func (g *ApprovalGate) Resolve(ctx, ticketID string) (ApprovalDecision, error)`

**Invariants:**
- Fingerprint is `sha256(tool_name + normalized_args_json + target_path)`
- If any component of the fingerprint changes, the ticket expires and a new request is required
- Approval tickets are single-use
- Timeout defaults to deny
- Each ticket binds to one concrete action snapshot (tool name, args, target, risk summary, preview ref)

- [ ] Step 1: Write failing tests in `internal/tools/approvals_test.go`.
  - `TestApproval_FingerprintBinding`: create ticket; verify fingerprint is sha256(tool_name + normalized_args_json + target_path).
  - `TestApproval_ExpiryOnActionChange`: create ticket, change args, attempt resolve; verify expired.
  - `TestApproval_ResolveValid`: create and resolve ticket; verify status transitions to approved.
  Run: `go test ./internal/tools -run TestApproval -v`
  Expected: all three tests FAIL.
- [ ] Step 2: Implement ApprovalGate with Request and Resolve.
  Run: `go test ./internal/tools -run TestApproval -v`
  Expected: all three tests PASS.
- [ ] Commit: `add approval gate`

---

### File: internal/tools/workspace.go

**Responsibility:** Implements WorkspaceApplier — the apply-checkpoint service that enforces workspace path containment. Preview shows the proposed changes; Apply validates the workspace root and checks all paths resolve within it. This file is NOT a tool in the agent's callable catalog — it is an apply gate. It is NOT responsible for approval ticket creation (approvals.go) or tool execution (runner.go).

**Key exports:**
- `type WorkspaceApplier struct`
- `func (w *WorkspaceApplier) Preview(ctx, runID string) (ChangePreview, error)`
- `func (w *WorkspaceApplier) Apply(ctx, runID string, approval ApprovalTicket) (ApplyResult, error)`

**Invariants:**
- Apply is limited to the declared workspace root for that run
- WorkspaceApplier receives workspace root as a parameter — it does NOT read the execution snapshot itself
- Path traversal (`../`) that resolves outside workspace root is rejected
- Symlinks that resolve outside workspace root are rejected
- Apply requires a valid approval ticket; the ticket must bind to the exact preview snapshot being approved

- [ ] Step 1: Write failing tests in `internal/tools/workspace_test.go`.
  - `TestWorkspace_RootEnforcement`: Apply with path outside workspace root returns error.
  - `TestWorkspace_PathTraversal`: path containing `../` resolving outside root is rejected.
  Run: `go test ./internal/tools -run TestWorkspace -v`
  Expected: both tests FAIL.
- [ ] Step 2: Implement WorkspaceApplier with path validation.
  Run: `go test ./internal/tools -run TestWorkspace -v`
  Expected: both tests PASS.
- [ ] Commit: `add workspace applier`

---

### File: internal/tools/runner.go

**Responsibility:** Executes tools: shell commands with argument sanitization, file operations, and git operations. Records all tool-call journal entries (request, decision, result, approval linkage) via ConversationStore.AppendEvent. This file is NOT responsible for policy decisions (registry.go), approval tickets (approvals.go), or workspace path enforcement (workspace.go).

**Key exports:**
- `type ToolRunner struct`
- `func (r *ToolRunner) Execute(ctx, call ToolCall, decision ToolDecision) (ToolResult, error)`

**Invariants:**
- Shell exec sanitizes or rejects: semicolons (`;`), pipe chars (`|`), null bytes, multi-command expansion
- All tool-call journal entries record request, decision, result, and approval linkage
- Tool execution respects the decision from registry.Decide — deny means no execution
- Every tool call is journaled via ConversationStore.AppendEvent, even if the call fails

- [ ] Step 1: Write failing tests in `internal/tools/tools_test.go`.
  - `TestShellExec_Semicolons`: args containing `;` are rejected.
  - `TestShellExec_PipeChars`: args containing `|` are rejected.
  - `TestShellExec_NullBytes`: args containing null bytes are rejected.
  - `TestShellExec_MultiCommandExpansion`: args expanding to multiple commands are rejected.
  Run: `go test ./internal/tools -run TestShellExec -v`
  Expected: all four tests FAIL.
- [ ] Step 2: Implement ToolRunner with sanitization and journal recording.
  Run: `go test ./internal/tools -run TestShellExec -v`
  Expected: all four tests PASS.
- [ ] Commit: `add tool runner`

---

### File: internal/replay/service.go

**Responsibility:** Implements ReplayReader for loading run replays and delegation graphs from durable events and projections. Reads handoff edges from the execution snapshot stored in DB, not from the live team.yaml. This file is NOT responsible for receipt building (receipts.go), preview packaging (preview_package.go), or any mutation.

**Key exports:**
- `type Service struct`
- `func (s *Service) LoadRun(ctx, runID string) (RunReplay, error)`
- `func (s *Service) LoadGraph(ctx, rootRunID string) (ReplayGraph, error)`

**Invariants:**
- Replay is read-only — it appends nothing to the journal or any durable state
- Handoff edges in the graph come from the execution snapshot DB row, not the live team.yaml
- Events in the replay are ordered by created_at
- Tool calls in the replay have request, decision, and result linked

- [ ] Step 1: Write failing tests in `internal/replay/replay_test.go`.
  - `TestReplay_GraphAssembly`: complete a root run with one child delegation; LoadGraph returns a graph with root and child nodes linked.
  - `TestReplay_TimelineOrdering`: events in the replay are ordered by created_at.
  - `TestReplay_ToolCallStitching`: a tool call appears with request, decision, and result linked.
  - `TestReplay_HandoffEdgesFromSnapshot`: complete a run with team spec V1, change team.yaml to V2, LoadGraph shows V1 edges (from frozen snapshot).
  Run: `go test ./internal/replay -run TestReplay -v`
  Expected: all four tests FAIL.
- [ ] Step 2: Implement LoadRun and LoadGraph.
  Run: `go test ./internal/replay -run TestReplay -v`
  Expected: all four tests PASS.
- [ ] Commit: `add replay service`

---

### File: internal/replay/receipts.go

**Responsibility:** Builds completion receipts from the durable journal and projections. Each completed run gets a receipt with token counts, cost, verification status, and approval outcomes. This file is NOT responsible for replay graph assembly (service.go) or comparison logic.

**Key exports:**
- `type ReceiptReporter struct`
- `func (r *ReceiptReporter) Build(ctx, rootRunID string) (RunReceipt, error)`

**Invariants:**
- Every completed run gets a receipt
- Receipts include: model_lane, input_tokens, output_tokens, cost, verification_status, approval_outcomes, final_status
- Token counts match accumulated usage records from the journal
- Receipt is computed from the same durable journal used by replay — no separate data path

- [ ] Step 1: Write failing tests in `internal/replay/receipts_test.go`.
  - `TestReceipt_ContainsRequiredFields`: build a receipt for a completed run; verify all required fields present.
  - `TestReceipt_BuildFromJournal`: complete a run with known events, build receipt, verify token counts match.
  Run: `go test ./internal/replay -run TestReceipt -v`
  Expected: both tests FAIL.
- [ ] Step 2: Implement Build.
  Run: `go test ./internal/replay -run TestReceipt -v`
  Expected: both tests PASS.
- [ ] Commit: `add receipt builder`

---

### File: internal/replay/preview_package.go

**Responsibility:** Builds a preview package from the durable journal and projections. The preview package summarizes a run's proposed changes for operator review before approval. This is a concrete service — no interface needed. This file is NOT responsible for model calls, approval decisions, or replay graph assembly.

**Key exports:**
- `type PreviewPackageBuilder struct`
- `func (b *PreviewPackageBuilder) Build(ctx, runID string) (PreviewPackage, error)`

**Invariants:**
- Makes NO model calls — reads only from journal and projections
- Preview packaging does not imply apply readiness unless a later approval flow exists
- The preview package includes: summary, grounded reasons, proposed diff, verification plan, receipt reference, replay path

- [ ] Step 1: Write failing test in `internal/replay/replay_test.go`.
  - `TestPreview_NoModelCalls`: build a preview package; verify the mock provider received zero Generate calls during Build.
  Run: `go test ./internal/replay -run TestPreview -v`
  Expected: FAIL.
- [ ] Step 2: Implement Build.
  Run: `go test ./internal/replay -run TestPreview -v`
  Expected: PASS.
- [ ] Commit: `add preview package builder`

---

### File: internal/memory/store.go

**Responsibility:** Implements MemoryStore against the SQLite memory_items table. Provides CRUD and search operations for durable memory facts. This file is NOT responsible for promotion logic (promote.go), scope escalation authorization (that is the run engine), or tool execution.

**Key exports:**
- `type Store struct`
- `func (s *Store) WriteFact(ctx, item MemoryItem) error`
- `func (s *Store) UpdateFact(ctx, item MemoryItem) error`
- `func (s *Store) ForgetFact(ctx, memoryID string) error`
- `func (s *Store) Search(ctx, query MemoryQuery) ([]MemoryItem, error)`
- `func (s *Store) SummarizeConversation(ctx, conversationID string) (SummaryRef, error)`

**Invariants:**
- WriteFact does NOT trigger auto-promotion — promotion is a separate explicit call in promote.go
- Search on empty store returns an empty slice, not an error
- Memory items keep provenance and scope metadata visible
- The store accepts WriteFact with any scope value but does NOT validate scope policy itself
- Human UpdateFact (source=human) outranks model-promoted facts

- [ ] Step 1: Write failing tests in `internal/memory/memory_test.go`.
  - `TestMemory_WriteFactPersists`: WriteFact with provenance, scope, dedupe_key; Search returns it.
  - `TestMemory_WriteFactNoAutoPromotion`: WriteFact does NOT trigger auto-promotion; no promotion event after WriteFact.
  - `TestMemory_HumanEditOutranksModel`: write model-promoted fact, then UpdateFact with source=human; Search returns human version.
  - `TestMemory_SearchEmptyStoreReturnsEmpty`: Search on fresh store returns empty slice, not error.
  Run: `go test ./internal/memory -run TestMemory -v`
  Expected: all four tests FAIL.
- [ ] Step 2: Implement Store against SQLite.
  Run: `go test ./internal/memory -run TestMemory -v`
  Expected: all four tests PASS.
- [ ] Commit: `add memory store`

---

### File: internal/memory/promote.go

**Responsibility:** Implements the explicit promotion entrypoint for memory candidates. Promotion is never triggered automatically by WriteFact — it requires an explicit call. This file is NOT responsible for basic CRUD (store.go), scope authorization, or tool execution.

**Key exports:**
- `func (s *Store) Promote(ctx, candidate MemoryCandidate) error`

**Invariants:**
- Promotion emits a typed journal event via ConversationStore.AppendEvent
- Auto-promotion is allowed only for narrow typed candidates (not arbitrary model output)
- Ambiguous memory stays a proposal or is rejected
- Candidates must include provenance, confidence, dedupe key, and explicit scope
- Human edits outrank model-promoted facts

- [ ] Step 1: Write failing test in `internal/memory/memory_test.go`.
  - `TestMemory_PromoteEmitsEvent`: call Promote with a candidate; assert a typed promotion journal event appears.
  Run: `go test ./internal/memory -run TestMemory_Promote -v`
  Expected: FAIL.
- [ ] Step 2: Implement Promote.
  Run: `go test ./internal/memory -run TestMemory_Promote -v`
  Expected: PASS.
- [ ] Commit: `add memory promotion`

---

### Milestone 1 Acceptance Tests

These tests validate the full kernel proof. Run after all Milestone 1 files are implemented.

- [ ] Create acceptance test(s) in a top-level `acceptance_test.go` or under `internal/runtime/`:
  - **End-to-end run**: one repo task runs end to end (CLI-driven, no web UI). Verify run completes with status "completed".
  - **Replayable**: load the run via replay.Service.LoadRun; verify events are present and ordered.
  - **Receipt**: build receipt via replay.ReceiptReporter.Build; verify it has model_lane, tokens, cost, final_status.
  - **Interrupted recovery**: create active runs, call ReconcileInterrupted, verify all become "interrupted".
  - **Memory path**: verify memory.Search was called during context compilation on every turn (mock records calls).
  - **Budget guard**: verify per-run cap stops a run; verify daily cap blocks a second run.

Run: `go test ./... -run 'Milestone1|KernelProof' -v`
Expected: PASS.

- [ ] Commit: `milestone 1 complete — kernel proof`

---

## Milestone 2 — Local Beta

**Ships:** local web UI with Runs, Run detail, Approvals, and Settings pages; SSE live replay; admin token auth; first-run onboarding wizard; default file-backed team; preview/apply/verify loop; per-run budgets; completion receipts.

**Does NOT ship:** team editor, memory editor, Telegram, compare view.

**Exit criteria:**
- A user can bind one repo and run a task
- Starter workflow can preview, apply, and verify a concrete change
- Replay explains what happened without transcript scraping
- Receipt shows model, cost, verification, and approval outcome
- System stays quiet when idle (no model calls in `gistclaw inspect status`)

---

### File: internal/web/server.go

**Responsibility:** Creates and configures the HTTP server, registers all routes, and implements admin token authentication middleware. Generates a crypto-random admin token on first start and stores it in the settings table. This file is NOT responsible for SSE broadcasting (sse.go), individual route handlers, or template rendering.

**Key exports:**
- `type Server struct`
- `func NewServer(cfg Config, rt *runtime.Runtime, rp *replay.Service, sse *SSEBroadcaster, db *store.DB) *Server`
- `func (s *Server) ListenAndServe(ctx context.Context) error`

**Invariants:**
- Admin token is generated on first start (no token in settings), stored via settings table
- All write-path handlers (POST, PUT, DELETE, PATCH) require `Authorization: Bearer <token>`
- Read-only GET routes have no auth requirement
- Write-path handlers call `*runtime.Runtime` methods — never call store.DB directly for writes
- Read-path handlers may read from store/replay directly (intentional bypass for read contention)
- `gistclaw inspect token` retrieves the current admin token

- [ ] Step 1: Write failing tests in `internal/web/server_test.go` using httptest.
  - `TestWeb_WritePathRequiresToken`: POST /run without bearer token returns 401.
  - `TestWeb_WritePathRejectsInvalidToken`: POST /run with wrong token returns 401.
  - `TestWeb_ReadPathNoAuth`: GET /runs without token returns 200.
  - `TestWeb_NoWritePathCallsStoreDirectly`: inspect write-path handler registrations; assert none reference store.DB methods directly.
  Run: `go test ./internal/web -run TestWeb -v`
  Expected: all four tests FAIL.
- [ ] Step 2: Implement NewServer with route registration and admin token middleware.
  Run: `go test ./internal/web -run TestWeb -v`
  Expected: all four tests PASS.
- [ ] Commit: `add web server with auth`

---

### File: internal/web/sse.go

**Responsibility:** Implements the SSE broadcaster that fans out run events to connected web clients. Implements `model.RunEventSink` so the runtime can push events without importing internal/web. Manages per-run subscriber channels. This file is NOT responsible for HTTP routing, template rendering, or authentication.

**Key exports:**
- `type SSEBroadcaster struct`
- `func (b *SSEBroadcaster) Emit(ctx, runID string, evt model.ReplayDelta) error` — implements model.RunEventSink
- `func (b *SSEBroadcaster) Subscribe(ctx, runID string) (<-chan model.ReplayDelta, func())`

**Invariants:**
- Maintains `map[string][]chan model.ReplayDelta` keyed by RunID
- Emit sends non-blocking to all subscribers for a run — slow clients are dropped, not blocking
- On client disconnect, the channel is removed via defer+close
- Channels lost on restart are acceptable — reconnecting clients catch up from event log
- Bootstrap passes `model.RunEventSink` (the interface) to runtime, never `*SSEBroadcaster`

- [ ] Step 1: Write failing test in `internal/web/server_test.go`.
  - `TestWeb_SSEFanOut`: two httptest clients subscribe for the same run; Emit once; both receive it. One disconnects; subsequent Emit does not block.
  Run: `go test ./internal/web -run TestWeb_SSEFanOut -v`
  Expected: FAIL.
- [ ] Step 2: Implement SSEBroadcaster with subscriber map, non-blocking fan-out, and cleanup.
  Run: `go test ./internal/web -run TestWeb_SSEFanOut -v`
  Expected: PASS.
- [ ] Commit: `add sse broadcaster`

---

### File: internal/web/routes_runs.go

**Responsibility:** Handles HTTP routes for listing runs and viewing run details. Read-path handlers that read directly from store/replay. This file is NOT responsible for run creation (routes_run_submit.go), approvals, or SSE streaming.

**Key exports:**
- `func (s *Server) handleListRuns(w, r)` — GET /runs
- `func (s *Server) handleRunDetail(w, r)` — GET /runs/:id

**Invariants:**
- Runs list is grouped by state: ACTIVE, NEEDS ATTENTION, RECENT
- Run detail layout adapts by run state (ACTIVE / NEEDS APPROVAL / COMPLETED / INTERRUPTED)
- Read path reads from store/replay directly (no runtime method needed for reads)

- [ ] Step 1: Write failing tests.
  - `TestWeb_ListRuns`: GET /runs returns 200 with run list HTML.
  - `TestWeb_FetchRunDetail`: GET /runs/:id returns 200 with run detail HTML.
  Run: `go test ./internal/web -run 'TestWeb_ListRuns|TestWeb_FetchRunDetail' -v`
  Expected: FAIL.
- [ ] Step 2: Implement handlers.
  Run: `go test ./internal/web -run 'TestWeb_ListRuns|TestWeb_FetchRunDetail' -v`
  Expected: PASS.
- [ ] Commit: `add runs routes`

---

### File: internal/web/routes_run_submit.go

**Responsibility:** Handles task submission via POST /run. Creates a new run via runtime.Start and redirects to run detail. This is a write-path handler that requires admin token auth. This file is NOT responsible for listing runs or viewing details.

**Key exports:**
- `func (s *Server) handleRunSubmit(w, r)` — POST /run

**Invariants:**
- Requires admin token
- Calls `runtime.Start` — never writes to store directly
- On success, redirects to GET /runs/:id with the new run ID
- On validation error, re-renders the form with error messages

- [ ] Step 1: Write failing test.
  - `TestWeb_SubmitRun`: POST /run with valid token and body starts a run; verify redirect to run detail.
  Run: `go test ./internal/web -run TestWeb_SubmitRun -v`
  Expected: FAIL.
- [ ] Step 2: Implement handler.
  Run: `go test ./internal/web -run TestWeb_SubmitRun -v`
  Expected: PASS.
- [ ] Commit: `add run submit route`

---

### File: internal/web/routes_approvals.go

**Responsibility:** Handles HTTP routes for listing pending approvals and resolving them. List is read-path (no auth); resolve is write-path (requires admin token, routes through runtime). This file is NOT responsible for approval ticket creation (internal/tools/approvals.go) or run lifecycle.

**Key exports:**
- `func (s *Server) handleListApprovals(w, r)` — GET /approvals
- `func (s *Server) handleResolveApproval(w, r)` — POST /approvals/:id/resolve

**Invariants:**
- Pending approvals show: tool name, args, risk level, preview diff, snapshot context
- Resolved-today audit trail shown below pending items
- Resolve routes through runtime (not direct DB write)
- Calm empty state when nothing pending (not "No items found")

- [ ] Step 1: Write failing tests.
  - `TestWeb_ListApprovals`: GET /approvals returns 200.
  - `TestWeb_ResolveApproval`: POST /approvals/:id/resolve with valid token resolves the ticket.
  Run: `go test ./internal/web -run 'TestWeb_ListApprovals|TestWeb_ResolveApproval' -v`
  Expected: FAIL.
- [ ] Step 2: Implement handlers.
  Run: `go test ./internal/web -run 'TestWeb_ListApprovals|TestWeb_ResolveApproval' -v`
  Expected: PASS.
- [ ] Commit: `add approvals routes`

---

### File: internal/web/routes_settings.go

**Responsibility:** Handles HTTP routes for viewing and updating daemon settings. Provider, workspace, budget, and access sections with inline edit per field. This file is NOT responsible for config file loading (internal/app/config.go) or runtime behavior.

**Key exports:**
- `func (s *Server) handleSettings(w, r)` — GET /settings
- `func (s *Server) handleUpdateSettings(w, r)` — PUT /settings

**Invariants:**
- Settings displayed in four sections: Provider, Workspace, Budget, Access
- Inline edit per field, not full-page form
- Update requires admin token and routes through runtime
- No full-page form — each field is independently editable

- [ ] Step 1: Write failing test.
  - `TestWeb_UpdateSettings`: PUT /settings with valid admin token returns 200.
  Run: `go test ./internal/web -run TestWeb_UpdateSettings -v`
  Expected: FAIL.
- [ ] Step 2: Implement handlers.
  Run: `go test ./internal/web -run TestWeb_UpdateSettings -v`
  Expected: PASS.
- [ ] Commit: `add settings routes`

---

### File: internal/web/routes_onboarding.go

**Responsibility:** Implements the 4-step first-run onboarding wizard: workspace bind, repo scan (no model calls), balanced trio shortlist, preview-only first run. Redirects to onboarding when no workspace is bound. This file is NOT responsible for the run engine, team validation, or template rendering.

**Key exports:**
- `func (s *Server) handleOnboarding(w, r)` — GET /onboarding
- `func (s *Server) handleWorkspaceBind(w, r)` — POST /onboarding/workspace
- `func (s *Server) handleRepoScan(w, r)` — POST /onboarding/scan
- `func (s *Server) handleFirstRun(w, r)` — POST /onboarding/run

**Invariants:**
- First-open GET / redirects to /onboarding when no workspace is bound
- Workspace validation: path exists, is a git repo, is writable
- Repo scan uses only file and git signals — NO model calls
- Balanced trio: explain subsystem, review diff/branch, find next safe improvement
- User picks the task — do not auto-select
- First run is preview-only (no apply capability)
- After successful first preview: CTA to connect Telegram DM

- [ ] Step 1: Write failing tests in `internal/web/routes_onboarding_test.go`.
  - `TestOnboarding_RedirectWhenNoWorkspace`: GET / redirects to /onboarding when no workspace bound.
  - `TestOnboarding_WorkspaceValidation`: valid git repo path succeeds; non-existent or non-git path returns error.
  - `TestOnboarding_RepoScanNonEmpty`: scan a repo with recent commits; returns non-empty shortlist.
  - `TestOnboarding_BalancedTrioTypes`: shortlist contains all three types: explain, review, improve.
  - `TestOnboarding_PreviewDispatch`: selected task starts preview-only run, returns PreviewPackage.
  Run: `go test ./internal/web -run TestOnboarding -v`
  Expected: all five tests FAIL.
- [ ] Step 2: Implement workspace bind step with validation.
- [ ] Step 3: Implement repo-signal scan using git commands only.
- [ ] Step 4: Render balanced trio shortlist.
- [ ] Step 5: Dispatch preview-only first run.
  Run: `go test ./internal/web -run TestOnboarding -v`
  Expected: all five tests PASS.
- [ ] Commit: `add first-run onboarding wizard`

---

### File: internal/web/templates/layout.html

**Responsibility:** Base HTML layout for all web pages. Nav bar with Runs, Approvals (with pending badge), Settings. Idle indicator. Loads JetBrains Mono for metadata display. This template is NOT responsible for page-specific content.

**Must contain:**
- `<nav>` with links: Runs, Approvals (badge count), Settings
- JetBrains Mono font loading
- Idle/active indicator in nav
- `{{template "content" .}}` block for page content

**Invariants (DESIGN.md):**
- 0px border radius everywhere
- 1.5px solid #1c1917 borders on all UI chrome
- No shadows anywhere
- JetBrains Mono for run IDs, timestamps, cost figures, token counts
- Weight 400 (body) and 700 (headings/labels) — no 500/600
- No emoji anywhere

- [ ] Step 1: Create layout.html following DESIGN.md strictly.
- [ ] Step 2: Verify in server_test.go that GET /runs returns HTML containing the nav structure.
- [ ] Commit: (included in web server commit)

---

### File: internal/web/templates/runs.html

**Responsibility:** Renders the runs list page grouped by state (ACTIVE / NEEDS ATTENTION / RECENT). Live row updates via SSE. Warm empty state with primary CTA. This template is NOT responsible for run detail or submission.

**Must contain:**
- Three state groups with headers
- Per-run row: run ID (JetBrains Mono), agent, status, created_at, cost
- "+ Submit task" button navigating to /run
- Warm empty state (not "No items found")
- SSE connection for live row updates

**Invariants:**
- 4px left border for run state signaling — no background fills
- Run IDs in JetBrains Mono
- Timestamps in JetBrains Mono

- [ ] Step 1: Create runs.html template.
- [ ] Commit: (included in runs routes commit)

---

### File: internal/web/templates/run_detail.html

**Responsibility:** Renders a single run's detail page. Layout adapts by run state: ACTIVE shows live event stream, NEEDS APPROVAL shows pending approval card, COMPLETED shows receipt and replay, INTERRUPTED shows resume action.

**Must contain:**
- Run header: run ID, agent, status, duration, cost (all metadata in JetBrains Mono)
- State-adaptive content area
- Delegation graph for runs with children
- Timeline of events
- SSE connection for live updates when ACTIVE

**Invariants:**
- 4px left border color signals state (not background fills)
- Receipt data in JetBrains Mono
- Delegation graph reads from execution snapshot, not live team.yaml

- [ ] Step 1: Create run_detail.html template.
- [ ] Commit: (included in runs routes commit)

---

### File: internal/web/templates/run_submit.html

**Responsibility:** Renders the task submission form. Single-purpose page for submitting a new repo task. Post-submit redirects to run detail.

**Must contain:**
- Task objective text area
- Workspace root display (from settings)
- Submit button

**Invariants:**
- Form submits to POST /run
- Validation errors shown inline

- [ ] Step 1: Create run_submit.html template.
- [ ] Commit: (included in run submit route commit)

---

### File: internal/web/templates/approvals.html

**Responsibility:** Renders the approvals page with inline pending approvals and resolved-today audit trail. Calm empty state when nothing pending.

**Must contain:**
- Pending approvals list: tool name, args, risk level, preview diff, approve/deny buttons
- Resolved-today section below pending
- Calm empty state (not "No items found")

**Invariants:**
- Args displayed in JetBrains Mono
- Risk level has 4px left border color indicator
- Approve/deny are write-path actions requiring token

- [ ] Step 1: Create approvals.html template.
- [ ] Commit: (included in approvals routes commit)

---

### File: internal/web/templates/settings.html

**Responsibility:** Renders the settings page with four sections: Provider, Workspace, Budget, Access. Inline edit per field.

**Must contain:**
- Provider section: model, API key (masked), endpoint
- Workspace section: root path, status
- Budget section: per-run cap, daily cap (displayed in JetBrains Mono)
- Access section: admin token (masked with copy button)

**Invariants:**
- Inline edit per field, not full-page form
- Numeric values in JetBrains Mono
- Save routes through runtime (PUT /settings)

- [ ] Step 1: Create settings.html template.
- [ ] Commit: (included in settings routes commit)

---

### File: internal/web/templates/onboarding.html

**Responsibility:** Renders the 4-step onboarding wizard. Progressive disclosure — one step visible at a time. Post-preview shows Telegram DM CTA.

**Must contain:**
- Step 1: Workspace path input with validation feedback
- Step 2: Repo scan progress (no model calls, git signals only)
- Step 3: Balanced trio shortlist — user picks one
- Step 4: Preview-only run result with receipt summary
- Post-completion: Telegram DM CTA

**Invariants:**
- User must pick the task — no auto-select
- Preview-only mode — no apply capability in first run
- Shortlist includes all three types: explain, review, improve

- [ ] Step 1: Create onboarding.html template.
- [ ] Commit: (included in onboarding routes commit)

---

### File: teams/default/team.yaml

**Responsibility:** Defines the default starter team with 3-4 agents, explicit capability flags, handoff edges, and tool profiles. This is the hero workflow, not a template gallery. Operators can rename agents and rewire handoffs. This file is NOT responsible for runtime behavior — it is configuration only.

**Must contain:**
- 3-4 agent definitions with: name, capability_flags, soul_file, tool_profile, memory_scope
- Explicit handoff_edges between agents (forward and return paths)
- At least one agent with workspace-write capability
- At least one agent marked operator-facing
- Most agents are read-heavy or propose-only by default

**Invariants:**
- Schema passes the validation from internal/runtime startup
- All handoff edge targets reference declared agents
- No unknown fields (strict parsing)
- No empty soul role fields

- [ ] Step 1: Write team.yaml with 3-4 agents and handoff edges.
- [ ] Step 2: Verify via startup validation test.
  Run: `go test ./internal/runtime -run TestStarterWorkflow_SchemaValidation -v`
  Expected: PASS.
- [ ] Commit: `add default team`

---

### File: teams/default/agent-01.soul.yaml through agent-04.soul.yaml

**Responsibility:** Structured soul files for each agent in the default team. Define role, tone, posture, collaboration style, escalation rules, decision boundaries, tool posture, prohibitions, and notes. These files are NOT raw prompt templates — they are structured configuration compiled before the model sees them.

**Must contain (each file):**
- role: what this agent does
- tone: communication style
- posture: default behavioral stance
- collaboration_style: how it works with other agents
- escalation_rules: when to hand off or ask for help
- decision_boundaries: what it can and cannot decide
- tool_posture: read-only, read-heavy, workspace-write, etc.
- prohibitions: what it must never do
- notes: additional context

**Invariants:**
- No empty role fields
- Soul files are parsed and compiled before the model sees them
- Structured fields only — no raw prompt editing as default surface

- [ ] Step 1: Write soul files for all agents.
- [ ] Commit: `add agent soul files`

---

### Milestone 2 Acceptance Tests

- [ ] Validate exit criteria:
  - User binds one repo and runs a task via web UI
  - Starter workflow previews, applies (after approval), and verifies
  - Replay explains what happened
  - Receipt shows model, cost, verification, approval outcome
  - System stays quiet when idle
  - Onboarding wizard completes end-to-end
  - SSE live updates work in run detail

Run: `go test ./... -run 'Milestone2|LocalBeta|StarterWorkflow|TestOnboarding' -v`
Expected: PASS.

- [ ] Commit: `milestone 2 complete — local beta`

---

## Milestone 3 — Public Beta

**Ships:** team editor with structured soul field editing, visual team composer with button-based handoff wiring, memory inspector with scope/provenance/forget/edit/filter, Telegram DM connector with long polling and durable delivery.

**Does NOT ship:** Telegram groups, replay sharing, GitHub publish-back.

**Exit criteria:**
- Operator can edit a team without inventing new runtime primitives
- Memory inspector shows scope and provenance without raw SQL access
- One operator can run the same team locally or from Telegram DM
- The system still feels like one calm teammate
- Replay and receipts still explain the work without extra operator guesswork

---

### File: internal/web/routes_team.go

**Responsibility:** Handles HTTP routes for team editing: listing agents, editing soul fields, adding/removing agents, wiring handoff edges. All writes validate the team spec before saving to disk. This file is NOT responsible for template rendering, run lifecycle, or soul parsing.

**Key exports:**
- `func (s *Server) handleListAgents(w, r)` — GET /team
- `func (s *Server) handleEditSoulField(w, r)` — PUT /team/agents/:id/soul
- `func (s *Server) handleAddAgent(w, r)` — POST /team/agents
- `func (s *Server) handleRemoveAgent(w, r)` — DELETE /team/agents/:id
- `func (s *Server) handleWireEdge(w, r)` — POST /team/edges

**Invariants:**
- Saves validate agent capabilities, handoff targets, and soul fields before writing to disk
- Validation errors shown inline (not silent failure)
- Reads and writes canonical teams/ YAML — no second config format
- Soul editing is structured (role, tone, posture, etc.) — no raw prompt editor as default

- [ ] Step 1: Write failing tests in `internal/web/routes_team_test.go`.
  - `TestTeam_ListAgents`: GET /team returns 200 with agent list.
  - `TestTeam_EditSoulField`: PUT /team/agents/:id/soul updates the soul file.
  - `TestTeam_AddAgent`: POST /team/agents adds agent to team.yaml.
  - `TestTeam_RemoveAgent`: DELETE /team/agents/:id removes agent.
  - `TestTeam_WireHandoffEdge`: POST /team/edges adds edge between declared agents.
  - `TestTeam_ValidateBeforeSave`: save with invalid handoff target returns validation error inline.
  Run: `go test ./internal/web -run TestTeam -v`
  Expected: all six tests FAIL.
- [ ] Step 2: Implement team editing handlers with validation.
  Run: `go test ./internal/web -run TestTeam -v`
  Expected: all six tests PASS.
- [ ] Commit: `add team editor`

---

### File: internal/web/routes_memory.go

**Responsibility:** Handles HTTP routes for the memory inspector: listing facts with scope and provenance, filtering by scope and agent, inline forget with confirmation, and inline edit. This file is NOT responsible for memory storage logic (internal/memory), promotion, or scope authorization.

**Key exports:**
- `func (s *Server) handleListMemory(w, r)` — GET /memory
- `func (s *Server) handleForgetFact(w, r)` — DELETE /memory/:id
- `func (s *Server) handleEditFact(w, r)` — PUT /memory/:id

**Invariants:**
- Every fact shows scope, source, provenance, confidence, and last-updated
- Forget requires confirmation step
- Edit preserves provenance metadata
- Filter by scope (local, team, all) and agent

- [ ] Step 1: Write failing tests in `internal/web/routes_memory_test.go`.
  - `TestMemory_ListFacts`: GET /memory returns 200 with facts showing scope and provenance.
  - `TestMemory_FilterByScope`: GET /memory?scope=local returns only local-scoped facts.
  - `TestMemory_FilterByAgent`: GET /memory?agent=agent-01 returns only that agent's facts.
  - `TestMemory_ForgetFact`: DELETE /memory/:id with confirmation removes the fact.
  - `TestMemory_EditFact`: PUT /memory/:id updates content, preserves provenance.
  Run: `go test ./internal/web -run TestMemory -v`
  Expected: all five tests FAIL.
- [ ] Step 2: Implement memory inspector handlers.
  Run: `go test ./internal/web -run TestMemory -v`
  Expected: all five tests PASS.
- [ ] Commit: `add memory inspector`

---

### File: internal/web/templates/team_editor.html

**Responsibility:** Renders the team editor page with agent list, structured soul field editing, and visual composer with button-based handoff wiring. No drag-and-drop. Same node vocabulary as replay graph.

**Must contain:**
- Agent list with capability badges
- Per-agent soul field editor (structured fields, not raw prompt)
- Handoff edge list with add/remove buttons
- Visual team graph (same node vocabulary as replay graph)
- Inline validation error display

**Invariants:**
- No drag-and-drop node positioning — button-based wiring only
- Saves validate before writing to disk
- 0px border radius, 1.5px borders per DESIGN.md

- [ ] Step 1: Create team_editor.html template.
- [ ] Commit: (included in team editor commit)

---

### File: internal/web/templates/memory_inspector.html

**Responsibility:** Renders the memory inspector page with facts in plain prose, scope and provenance always visible, inline forget/edit, and filter controls.

**Must contain:**
- Fact list with scope, source, provenance, confidence, last-updated
- Filter dropdowns: scope (local, team, all), agent
- Inline forget button with confirmation modal
- Inline edit for fact content
- Calm empty state when no facts exist

**Invariants:**
- Provenance always visible — never hidden behind a click
- Metadata in JetBrains Mono
- 0px border radius per DESIGN.md

- [ ] Step 1: Create memory_inspector.html template.
- [ ] Commit: (included in memory inspector commit)

---

### File: internal/telegram/bot.go

**Responsibility:** Implements the Telegram Bot API client using long polling (getUpdates with 30s timeout). No webhooks. Starts polling on daemon startup when Telegram is configured. This file is NOT responsible for message normalization (inbound.go), outbound delivery (outbound.go), or approval resolution.

**Key exports:**
- `type Bot struct`
- `func NewBot(token string, sink InboundSink) *Bot`
- `func (b *Bot) Start(ctx context.Context) error` — begin polling
- `func (b *Bot) Stop() error`

**Invariants:**
- Long polling only (getUpdates, 30s timeout) — no webhooks
- DM only: group messages are silently rejected (logged and skipped)
- Bot does not start unless Telegram is configured in settings

- [ ] Step 1: Write failing tests in `internal/telegram/bot_test.go`.
  - `TestTelegram_InboundDMNormalization`: mock DM update normalizes to Envelope with ConnectorID="telegram", correct AccountID, ExternalID=chat_id, ThreadID="main".
  Run: `go test ./internal/telegram -run TestTelegram_InboundDMNormalization -v`
  Expected: FAIL.
- [ ] Step 2: Implement Bot with long polling.
  Run: `go test ./internal/telegram -run TestTelegram_InboundDMNormalization -v`
  Expected: PASS.
- [ ] Commit: `add telegram bot`

---

### File: internal/telegram/inbound.go

**Responsibility:** Normalizes incoming Telegram DM messages into the standard Envelope format. Rejects group messages. This file is NOT responsible for polling (bot.go), outbound delivery, or approval handling.

**Key exports:**
- `func NormalizeUpdate(update TelegramUpdate) (model.Envelope, error)`

**Invariants:**
- ConnectorID is always "telegram"
- ExternalID is the string form of chat_id
- ThreadID is always "main" (DM only)
- Group messages return an error (silently rejected at bot level)

- [ ] Step 1: (Tests included in bot_test.go above.)
- [ ] Step 2: Implement NormalizeUpdate.
- [ ] Commit: (included in telegram bot commit)

---

### File: internal/telegram/outbound.go

**Responsibility:** Handles durable outbound message delivery to Telegram. Records intent before delivery, implements exponential backoff retry, and journals terminal failures. Output restricted to four milestone message types. This file is NOT responsible for inbound normalization or polling.

**Key exports:**
- `type Outbound struct`
- `func (o *Outbound) Send(ctx, intent OutboundIntent) error`
- `func (o *Outbound) DrainPending(ctx) error` — retry pending intents on startup

**Invariants:**
- Record intent in outbound_intents table BEFORE delivery attempt
- Retry policy: exponential backoff (1s, 2s, 4s, 8s, 16s), max 5 retries
- Terminal failure (5 retries or bot token revoked) journals `delivery_failed` event
- Output restricted to four message types: started, blocked (approval_needed), finished
- Low-risk approvals may be resolved via inline Telegram buttons; medium/high-risk direct to local web UI
- Dedupe key prevents duplicate sends on restart

- [ ] Step 1: Write failing tests in `internal/telegram/bot_test.go`.
  - `TestTelegram_OutboundStatusCard`: build outbound card for "completed" run; verify concise text with status.
  - `TestTelegram_DedupeKeyPersistence`: send intent; verify dedupe_key stored in outbound_intents.
  - `TestTelegram_RetryThenSucceed`: mock API fails 3 times then succeeds; verify delivered with retry count 3.
  - `TestTelegram_TerminalFailure`: mock API fails 5 times; verify delivery_failed event journaled.
  Run: `go test ./internal/telegram -run TestTelegram -v`
  Expected: all four tests FAIL.
- [ ] Step 2: Implement Send with durable intent, retry, and failure journaling.
  Run: `go test ./internal/telegram -run TestTelegram -v`
  Expected: all tests PASS.
- [ ] Commit: `add telegram outbound delivery`

---

### File: internal/store/migrations/003_scheduler.sql

**Responsibility:** Adds the schedules table for the scheduler package. This migration is intentionally deferred from 001/002 to Milestone 3 because the scheduler is not active in Milestones 1 or 2.

**Must contain:**
- `schedules` table — id, name, cron_expression, team_id, objective, enabled, created_at, updated_at

**Invariants:**
- Only created in Milestone 3, not earlier
- Schema version increments correctly
- Does not modify any existing tables from 001/002

- [ ] Step 1: Write the migration SQL.
- [ ] Step 2: Verify via migrate test.
  Run: `go test ./internal/store -run TestMigrate_FreshBoot -v`
  Expected: PASS (001, 002, 003 all applied).
- [ ] Commit: `add scheduler migration`

---

### Milestone 3 Acceptance Tests

- [ ] Validate exit criteria:
  - Team editor: create agent, wire edge, edit soul, validate before save
  - Memory inspector: list, filter, forget, edit
  - Telegram DM: send task via DM, receive milestone messages, resolve low-risk approval
  - Replay and receipts explain DM-originated runs
  - System feels like one calm teammate

Run: `go test ./... -run 'Milestone3|PublicBeta|TestTeam|TestMemory|TestTelegram' -v`
Expected: PASS.

- [ ] Commit: `milestone 3 complete — public beta`

---

## Milestone 4 — Stable 1.0

**Ships:** doctor command, backup/export, approval polish, receipt polish, onboarding polish.

**No new primitives.**

**Exit criteria:**
- Restarts are boring: interrupted runs are visible and resumable, no silent loss
- Approvals are auditable: each ticket has a replay entry with action, snapshot, and outcome
- Idle burn is effectively zero: `gistclaw inspect status` confirms no background model calls
- Default repo-task workflow feels polished instead of merely functional

---

### File: cmd/gistclaw/doctor.go

**Responsibility:** Implements the `gistclaw doctor` subcommand that runs a series of health checks and reports results. This is a diagnostic-only command that makes no changes. This file is NOT responsible for fixing issues or runtime behavior.

**Key exports:**
- `func RunDoctor(cfg Config) error`

**Doctor checks:**
- Config validity (config.yaml parses and passes validation)
- DB reachability (SQLite file accessible, WAL mode confirmed)
- Provider config (API key present, provider endpoint reachable with timeout)
- Workspace root health (path exists, is git repo, is writable)
- Telegram config if enabled (bot token valid format, long polling connectable)
- Disk space (warn if < 500MB available on the partition containing the DB)

**Invariants:**
- Each check prints a status line: PASS, WARN, or FAIL with clear message
- Exit code 0 if all pass, 1 if any fail, 2 if any warn
- Doctor does not modify any state

- [ ] Step 1: Write failing tests in `cmd/gistclaw/doctor_test.go`.
  - `TestDoctor_ValidConfig`: valid config + reachable DB + valid workspace returns all checks passing.
  - `TestDoctor_MissingDB`: unreachable DB reports DB check as failed.
  - `TestDoctor_LowDiskSpace`: < 500MB warns about disk space.
  - `TestDoctor_InvalidProviderConfig`: missing API key reports provider check failed.
  Run: `go test ./cmd/gistclaw -run TestDoctor -v`
  Expected: all four tests FAIL.
- [ ] Step 2: Implement RunDoctor with all checks.
  Run: `go test ./cmd/gistclaw -run TestDoctor -v`
  Expected: all four tests PASS.
- [ ] Commit: `add doctor command`

---

### Files: hardening passes across existing packages

**Responsibility:** Final hardening of lifecycle, store recovery, approval flow, receipt surface, and backup/export. No new packages or primitives — polish and reliability work only.

**Hardening targets:**

1. **internal/app/lifecycle.go** — Fully graceful shutdown: finish in-flight provider calls (with timeout), flush pending outbound intents, close DB cleanly. Always run ReconcileInterrupted before accepting new work.

2. **internal/store/db.go** — Backup command using SQLite backup API (not raw file copy while WAL is active). `gistclaw backup` produces a timestamped backup file.

3. **internal/runtime/runs.go** — `gistclaw export runs <run_id>` exports a run's events as JSON for debugging. Verify no background model calls when idle.

4. **internal/web/routes_approvals.go** — Polish: each approval ticket shows tool name, args, risk level, preview diff, snapshot context. Each resolved approval has a replay entry.

5. **internal/web/routes_runs.go** — Polish: receipts show tokens, cost, wall-clock, verification, approval outcomes, model lane. Make legible before adding new features.

- [ ] Step 1: Write failing recovery and idle-behavior tests in `internal/runtime/recovery_test.go`.
  - `TestRecovery_RestartReconciliation`: create active runs, simulate restart, verify all become 'interrupted'.
  - `TestRecovery_InterruptedRunsResumable`: interrupted runs can be resumed.
  - `TestRecovery_NoBackgroundModelCalls`: start daemon with no active runs, wait 5 seconds, verify zero Generate calls.
  - `TestRecovery_StaleApprovalExpiry`: create ticket, change action, verify expired.
  Run: `go test ./internal/runtime -run TestRecovery -v`
  Expected: FAIL.
- [ ] Step 2: Write failing approval flow integration tests in `internal/web/approval_flow_test.go`.
  - `TestApprovalFlow_FullCycle`: submit task -> approval point -> visible in UI -> resolve -> run continues -> receipt generated.
  - `TestApprovalFlow_ReplayEntry`: each resolved approval has a replay entry with action, snapshot, outcome.
  Run: `go test ./internal/web -run TestApprovalFlow -v`
  Expected: FAIL.
- [ ] Step 3: Implement lifecycle hardening, backup, export, approval polish, receipt polish.
  Run: `go test ./... -run 'TestRecovery|TestApprovalFlow|TestDoctor' -v`
  Expected: all tests PASS.
- [ ] Commit: `harden runtime for stable 1.0`

---

### Milestone 4 Acceptance Tests

- [ ] Validate exit criteria:
  - Restarts are boring: interrupted runs visible and resumable
  - Approvals auditable: replay entry for each
  - Idle burn zero: no background model calls
  - Default workflow polished: onboarding through receipt

Run: `go test ./... -run 'Milestone4|Stable' -v`
Expected: PASS.

- [ ] Run one manual proof flow:
  1. Start daemon: `gistclaw serve`
  2. Run repo task via web UI or CLI
  3. Approve the apply action
  4. Verify the result in run detail view
  5. Inspect replay: `gistclaw inspect replay <run_id>`
  6. Confirm no background work after completion: `gistclaw inspect status`

- [ ] Commit: `milestone 4 complete — stable 1.0`

---

## Manual Proof Checklist

Before calling the backlog complete, verify all of these manually:

- [ ] One active root run per conversation
- [ ] Interrupted runs remain visible after restart
- [ ] No agent without workspace-write capability can escape workspace root
- [ ] Approval tickets expire when the action changes
- [ ] Receipts show tokens, cost, verification, and approval outcomes
- [ ] Replay is assembled from durable events, not transcript scraping
- [ ] The system stays quiet when idle
- [ ] Memory read path is exercised on every turn (observable in logs)
- [ ] Team spec edits do not affect active runs (snapshot immutability)
- [ ] Runtime never imports internal/web (verify with `go list -deps ./internal/runtime`)

## Stop Rules

Pause and re-plan if any task tries to introduce:

1. A second runtime
2. A plugin system
3. Multiple connectors before Telegram DM is done
4. A new storage format for core entities
5. A visual team composer that writes to a format other than canonical teams/ YAML
6. Replay export before the local replay UI is solid
7. Compare mode that requires a separate subsystem
8. internal/api as a new package alongside internal/web
