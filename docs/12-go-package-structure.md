# Go Package Structure

## Implementation warning

Treat the full layout below as the stable-release target, not the day-one package split.

### Day-one packages (exist from Milestone 1)

```text
internal/app/           — config, bootstrap, lifecycle
internal/store/         — SQLite journal, migrations, tx helpers
internal/model/         — shared domain types, RunEventSink interface
internal/conversations/ — conversation key normalization, event-history APIs, active-run arbitration
internal/tools/         — tool registry, policy, approval tickets, WorkspaceApplier
internal/runtime/       — run engine, delegations, provider calls only
internal/replay/        — read-only replay and receipt projections (no mutation logic); concrete preview package builder
internal/memory/        — memory store, auto-promotion gate (separated from runtime to prevent accidental scope escalation)
internal/web/           — HTTP server, SSE broadcaster (implements RunEventSink), templates
```

Notes:

- internal/runtime owns run lifecycle, delegation, and provider calls only — it does NOT own conversation normalization, tool dispatch, or approval tickets
- internal/conversations replaces the conversation-key and event-history responsibilities previously collapsed into internal/runtime
- internal/tools replaces the tools and approvals responsibilities previously collapsed into internal/runtime
- internal/model is a day-one package because RunEventSink must be importable by both internal/runtime and internal/web without a cycle

`internal/telegram/` is added in Milestone 3 only.

### Deferred splits (stable-release targets, not day-one)

Do not create these before the split trigger rule fires (see `docs/20-v1-implementation-backlog.md`):

```text
internal/agents/     internal/soul/       internal/identity/
                                          internal/runs/
                     internal/providers/  internal/connectors/
internal/scheduler/  internal/auth/       internal/api/
internal/telemetry/
```

### Dependency direction (v1)

```text
app -> runtime
app -> conversations
app -> tools
app -> replay
app -> memory
app -> web
web -> runtime          (write path: approval resolution and run start via runtime method calls)
web -> store            (read path: direct store reads for run list, replay — intentional bypass)
web -> replay           (read path: receipt and replay queries)
runtime -> conversations (AppendEvent for all lifecycle events)
runtime -> tools         (tool policy checks and approval ticket creation)
runtime -> memory
runtime -> providers
conversations -> store
tools -> store
replay -> store
replay -> memory
memory -> store
```

Forbidden for day-one:

- runtime -> web (dependency inversion; runtime publishes to RunEventSink in internal/model instead)
- web writing to store directly for any write-path handler (must route through runtime)

Only split into the fuller layout below after the seam is proven by pain.

## Top-level layout

```text
cmd/
  gistclaw/
    main.go

internal/
  app/
    bootstrap.go
    config.go
    lifecycle.go

  store/
    db.go
    journal.go
    projections.go
    tx.go
    migrations/

  model/
    entities.go
    enums.go

  agents/
    registry.go
    profiles.go
    teams.go
    team_specs.go
    handoff_edges.go
    starter_templates.go
    operator_surface.go

  soul/
    load.go
    validate.go
    compile.go

  identity/
    load.go
    validate.go

  memory/
    store.go
    retrieve.go
    summarize.go
    promote.go
    publish.go
    inspect.go
    edit.go

  conversations/
    keys.go
    service.go

  runs/
    engine.go
    assignment.go
    snapshot.go
    streaming.go
    delegation.go
    queue.go
    recovery.go
    guardrails.go

  replay/
    service.go
    graph.go
    timeline.go
    live.go
    preview_package.go
    receipt.go
    compare.go
    explain.go

  tools/
    registry.go
    policy.go
    approvals.go
    runner.go
    file/
    shell/
    http/
    search/
    messaging/
    email/
    calendar/

  providers/
    types.go
    registry.go
    lanes.go
    selection.go
    openai/
    anthropic/
    google/

  connectors/
    telegram/
      connector.go
      inbound.go
      outbound.go
      queue.go
      dm_progress.go

  scheduler/
    cron.go
    service.go

  auth/
    admin.go
    secrets.go

  api/
    server.go
    handlers.go
    ui_embed.go
    ui_live_replay.go
    ui_first_run.go
    ui_team_composer.go
    ui_memory_inspector.go

  telemetry/
    log.go
    trace.go
    metrics.go
    activity.go
```

## Ownership rules

### `internal/app`

Owns only:

- config loading
- dependency wiring
- startup and shutdown
- migration checks

It must not own runtime behavior.

It may coordinate first-run bootstrap state only.

### `internal/store`

Owns all durable writes.

Rules:

- no JSON side stores for core entities
- the append-only `events` journal is the persistence spine
- current-state tables are projections maintained transactionally from journaled state changes
- all state changes happen through transactions
- no connector or provider logic here

### `internal/model`

Owns shared domain types only.

This avoids cyclic imports between runtime, tools, memory, and connectors.

### `internal/agents`

Owns:

- agent registry
- team spec loading and validation
- allowed handoff-edge definitions between agents
- capability flag validation (workspace-write, operator-facing, read-heavy, propose-only)
- tool profile assignment per agent
- memory-scope assignment per agent
- starter-team loading and bundled default generation
- soul and identity file location resolution

Does NOT own:

- operator-surface mapping for conversational surfaces — this is a connector-local concern owned by internal/connectors
- team-scope memory publish authorization — this is authorized by the run engine
- soul parsing and compilation — owned by internal/soul (deferred split)
- identity display data — owned by internal/identity (deferred split)

Note: soul loading and starter-team validation are concrete services inside internal/agents for the stable release — not the package's primary public interface. The primary interface is agent registry, handoff validation, and capability resolution.

### `internal/soul`

Owns:

- `soul.yaml` parsing
- schema validation
- structured soul-card editing rules
- compile-to-runtime-instructions

It must not inspect transcripts or live runs.

### `internal/identity`

Owns only display-facing identity data.

It must stay separate from behavior and policy.

### `internal/memory`

Owns:

- durable memory items
- working summaries
- retrieval
- promotion rules
- memory inspection and edit services

It must not own transport or tool execution.

- internal/memory does not own team-scope publish authorization — scope escalation from local to team scope is authorized by the run engine before calling WriteFact
- internal/memory is a storage service; it stores facts with the scope value it receives but does not interpret team topology

### `internal/conversations`

Owns:

- canonical conversation-key normalization and string codec (ConnectorID + AccountID + ExternalID + ThreadID — no TeamID)
- conversation keys and event-history APIs
- conversation lookup by connector ids
- conversation ownership binding to team scope at the Run level, not the Conversation level
- active-root-run arbitration per canonical conversation
- ConversationStore.AppendEvent — the single canonical journal append path for ALL event types

It must not own:

- connector-specific payload normalization (internal/connectors)
- run lifecycle logic (internal/runtime)

### `internal/runtime`

Owns only:

- one run lifecycle (Start, Continue, Delegate, Resume, ReconcileInterrupted)
- provider calls
- delegation creation and child-slot allocation
- handoff validation against team edges in frozen execution snapshot
- streaming to observers via RunEventSink
- interrupted-run reconciliation

It must NOT own:

- conversation key normalization (internal/conversations)
- event-history APIs (internal/conversations)
- tool registry or policy evaluation (internal/tools)
- approval ticket creation or resolution (internal/tools)
- workspace apply logic (internal/tools)
- memory retrieval or promotion (internal/memory)

### `internal/replay`

Owns:

- run replay queries
- delegation graph assembly
- timeline projection
- live replay projection for the local web UI
- concrete preview-package builder (reads journal and projections; makes no model calls)
- memory, tool, and approval event stitching

Charter limit:

- internal/replay is read-only with respect to the journal and durable state — it appends nothing
- if explain.go requires model calls to generate grounded explanations, it must move to a separate presentation service with provider access before Task 7 starts
- replay reads handoff edges from the execution snapshot stored in the DB, not from the live teams/ directory

It does not own execution or scope escalation authorization.

### `internal/tools`

Owns:

- tool registry
- tool policy evaluation (allow / ask / deny)
- approval ticket creation and binding
- WorkspaceApplier (concrete apply-checkpoint service, not a tool in the callable catalog)
- tool execution (file, shell, HTTP, search, messaging, email, calendar)

It must not own:

- connector routing
- prompt assembly
- run lifecycle coordination

WorkspaceApplier rule: the run engine calls WorkspaceApplier.Apply after receiving a valid approval ticket from ApprovalGate. WorkspaceApplier receives workspace root as a parameter — it does not read the execution snapshot itself.

### `internal/providers`

Owns:

- provider adapters
- capability flags
- request/response normalization
- agent- and phase-based model lane resolution
- explicit escalation from cheap lanes to stronger lanes

Do not build a generic provider platform. Keep adapters concrete.

### `internal/connectors`

Each connector package owns:

- inbound auth
- inbound normalization
- connector-local routing into conversations and runs
- payload normalization
- outbound delivery
- durable outbound intent draining
- delivery retries and receipts for that connector

The rest of the runtime sees only normalized envelopes.

### `internal/scheduler`

Owns explicit schedule execution only in the initial phases.

Rules:

- schedule-triggered runs only
- no free-roaming proactive agent loop here
- no parallel automation frameworks here

### `internal/api`

Owns the local admin HTTP API and embedded local web UI.

No separate WS control plane package before the first stable release.

It may expose team-spec read and write handlers for the local web UI.

Stable-release collapse rule:

- first-run helpers stay as concrete services inside `internal/agents`, `internal/replay`, and `internal/api`
- replay presentation stays inside `internal/replay` and `internal/api`
- later-phase sharing helpers do not get their own packages before the stable release

Naming rule (internal/web vs internal/api):

- internal/web is the canonical day-one package name for the HTTP server, SSE broadcaster, and templates
- internal/api is NOT a separate package — it is the eventual rename of internal/web at a later phase if the package split is needed
- do not create both internal/web and internal/api as separate packages; the stable-release layout shows the target shape after a rename, not a coexisting split
- during Milestones 1-4, all HTTP serving, SSE, and templates live in internal/web
- if a split is needed later (e.g., separating API handlers from template rendering), that split happens as a defined migration step, not a spontaneous new package

## Dependency direction (stable-release target)

The arrows below describe the stable-release package graph, not the day-one layout.
For the day-one dependency graph see the "Day-one packages" section above.

Allowed:

```text
app -> api
app -> runs
api -> runs
api -> conversations
api -> approvals
runs -> providers
runs -> tools
runs -> memory
runs -> conversations
runs -> agents
replay -> store
replay -> runs
replay -> memory
replay -> conversations
connectors -> conversations
connectors -> runs
all durable state -> store
```

Forbidden:

- connectors importing providers
- providers importing connectors
- tools importing connectors directly
- memory importing runs
- soul importing store transactions
- admin UI importing adapters directly
- replay guessing truth from prompt text

## Why this wins

This layout is intentionally boring:

- small number of top-level concepts
- clear ownership
- no giant gateway package
- no plugin-runtime package at the center

That is exactly the point.
