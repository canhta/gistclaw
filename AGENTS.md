# AGENTS.md

This file provides guidance to AI agents when working with code in this repository.

## Project

GistClaw is a local-first multi-agent runtime for software repo tasks. A single Go binary (`gistclaw`) coordinates multiple AI agents around one repo task, with approvals before side effects, durable event journaling, and full audit trails. It is the clean-slate successor to OpenClaw.

**Module:** `github.com/canhta/gistclaw`
**Status:** Design complete, implementation begins at Milestone 1.

## Build & Test

```bash
go build -o bin/gistclaw ./cmd/gistclaw
go test ./...
go test ./internal/store/...     # single package
go test -run TestFoo ./...       # single test
go vet ./...
```

**Tech stack:** Go 1.24+, `modernc.org/sqlite` (pure-Go, no CGO), stdlib `net/http`, Go `testing` package.

## Problem-Solving Policy

**Think broadly, fix correctly.** Always identify the root cause before touching code. No hotfixes, no workarounds, no hacks. If a proper fix requires touching multiple files or refactoring a boundary, do it fully. A narrow patch that papers over the real issue is worse than doing nothing.

## Branch Policy

**`main` only.** No feature branches, detached HEADs, or git worktrees — ever.

## Testing Policy

**TDD always.** Write tests before implementation. Every change must maintain ≥70% coverage (`go test -cover ./...`). Do not merge code that drops below this threshold.

## Migration Policy

**Single migration file only.** All schema changes go into `001_init.sql` — edit it in place. No new numbered migration files until the project reaches stable. Drop and recreate your local DB when the schema changes.

## Code Style

**Go idioms and DRY always.** Follow standard Go conventions (`gofmt`, `go vet`, effective Go). Extract shared logic once — no duplicated code paths. Use the stdlib before reaching for helpers. Keep functions small and names self-documenting.

- **Errors:** return `error` as the last return value; wrap with `fmt.Errorf("context: %w", err)`; never discard errors silently.
- **Interfaces:** define interfaces in the consuming package, not the implementing package. Keep them small (1–3 methods).
- **Goroutines:** every goroutine must have a clear owner and exit path. No fire-and-forget goroutines without a `context.Context`.
- **Context:** `context.Context` is always the first parameter, named `ctx`. Never store it in a struct.
- **Zero values:** design types so the zero value is usable. Avoid `New*` constructors that only set defaults.
- **Table-driven tests:** use `t.Run` subtests with a `[]struct{name, input, want}` table. No duplicated test logic.
- **Naming:** acronyms are all-caps (`ID`, `URL`, `HTTP`). Receivers are short (1–2 chars). Package names are singular, lowercase, no underscores.

## Extensibility Policy

**New tools, providers, and connectors must plug into the declared interface seams — never bypass them.** No hardcoding of provider names, tool names, or connector IDs in the run engine or conversations packages.

## Refactoring Policy

**No backward compatibility. No legacy support.** When refactoring, do it completely — no shims, no deprecated wrappers, no compatibility aliases. Delete the old code.

## Architecture

### Runtime Model

- **Single-writer daemon** owns all mutable state (`runtime.db`). Local web UI, CLI, and Telegram DM send commands through the daemon.
- **Append-only events journal** is the persistence spine. All state changes go through `ConversationStore.AppendEvent` — this is the single canonical write path. The run engine never writes to the journal directly.
- **Current-state tables** (`runs`, `approvals`, `tool_calls`, `receipts`, etc.) are projections maintained transactionally from journal events.
- **One active root run per canonical conversation**. Parallel child delegations allowed under the root; new inbound asks queue or attach explicitly.
- **Bounded child concurrency** — each root run has a fixed active-child budget; extra delegations queue with visible backpressure.

### Execution Snapshots & Approvals

- Each root run records a frozen execution snapshot (team spec, soul, tool policy, config) at start. Child delegations inherit it. Live config edits apply to the next run only.
- **Snapshot-bound approvals**: each approval ticket binds to one concrete action bundle. If the proposed action changes materially, the ticket expires and the runtime must ask again.
- **Approval fingerprint**: `sha256(tool_name + normalized_args_json + target_path)`.

### Day-One Package Layout (Milestones 1–2)

```
cmd/gistclaw/        — binary entry point
internal/app/        — config, bootstrap, lifecycle, migration checks
internal/store/      — SQLite journal, migrations, tx helpers
internal/model/      — shared domain types, RunEventSink interface (zero project imports)
internal/conversations/ — ConversationKey normalization, event-history APIs, active-run arbitration
internal/tools/      — tool registry, policy evaluation, approval tickets, WorkspaceApplier
internal/runtime/    — run lifecycle (Start, Continue, Delegate, Resume), provider calls only
internal/replay/     — read-only replay, delegation graph, timeline, receipt projections
internal/memory/     — durable memory store, retrieval, promotion
internal/web/        — HTTP server, SSE broadcaster (implements RunEventSink), Go templates
```

`internal/telegram/` is added in Milestone 3. Do not create the deferred packages (`internal/agents/`, `internal/soul/`, `internal/providers/`, etc.) before the split trigger rule fires.

### Dependency Direction

```
app -> runtime, conversations, tools, replay, memory, web
web -> runtime (write path), store (read path), replay (read path)
runtime -> conversations, tools, memory, providers
conversations -> store
tools -> store
replay -> store, memory
memory -> store
```

**Forbidden (day-one):**
- `runtime` → `web` (runtime publishes to `RunEventSink` in `internal/model` instead)
- `web` writing to `store` directly for any write-path handler (must route through runtime)
- `internal/model` importing anything from this project (stdlib only)

Verify with `go list -deps`.

### Ownership Rules

| Package | Owns | Does NOT own |
|---------|------|-------------|
| `internal/runtime` | Run lifecycle, provider calls, delegation, handoff validation, streaming to `RunEventSink` | Conversation normalization, tool registry, approval tickets, workspace apply, memory |
| `internal/conversations` | `ConversationKey` normalization, `AppendEvent` (single journal write path), active-run arbitration | Connector payload normalization, run lifecycle |
| `internal/tools` | Tool registry, policy evaluation, approval ticket creation, `WorkspaceApplier`, tool execution | Connector routing, prompt assembly, run lifecycle |
| `internal/replay` | Read-only replay queries, delegation graph, timeline, receipt projection | Any writes to journal or durable state |
| `internal/memory` | Durable memory items, retrieval, promotion, inspection | Team-scope publish authorization (that's the run engine), transport |
| `internal/store` | All durable writes, SQLite transactions | Connector logic, provider logic |
| `internal/model` | Shared domain types, `RunEventSink` interface | Everything else (no project imports) |

### Global Guardrails

1. No WebSocket control plane (SSE is wired in Milestone 2)
2. No transcript files or JSON side stores — SQLite only
3. No plugin runtime
4. No vector memory
5. No autonomous background loops — only explicit run starts
6. No journal writes outside `ConversationStore.AppendEvent`
7. `schedules` table is NOT in 001_init.sql — deferred to Milestone 3
8. `BudgetGuard` does NOT handle active-child concurrency — that is `delegations.go`

## Design System

Always read `DESIGN.md` before any visual or UI decision. Do not deviate without explicit user approval.

## Documentation

Read in this order to onboard:
1. `docs/00-system-diagrams.md` — runtime shape, trust model, workflow
2. `docs/11-architecture-redesign.md` — design stance and targets
3. `docs/12-go-package-structure.md` — package ownership and dependency rules
4. `docs/13-core-interfaces.md` — minimal abstraction seams
5. `docs/superpowers/plans/2026-03-24-m*.md` — milestone task breakdowns
6. `docs/18-eng-review.md` — 12 locked architecture decisions
