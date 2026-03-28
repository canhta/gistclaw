# AGENTS.md

## Project

GistClaw is a local-first multi-agent runtime for software repo tasks. A single Go binary (`gistclaw`) coordinates agents around a shared workspace, with approvals before risky side effects, durable event journaling, replayable runs, and operator-facing web controls.

**Module:** `github.com/canhta/gistclaw`
**Status:** Active implementation on `main`. The current tree already includes the daemon, local web host, sessions, memory, tools, providers, replay, and live Telegram and WhatsApp paths.

## Build & Test

```bash
go build -o bin/gistclaw ./cmd/gistclaw
go test ./...
go test ./internal/store/...     # single package
go test -run TestFoo ./...       # single test
go vet ./...
```

**Tech stack:** Go 1.25+, `modernc.org/sqlite` (pure-Go, no CGO), stdlib `net/http`, Go `testing` package.

## Problem-Solving Policy

**Think broadly, fix correctly.** Always identify the root cause before touching code. No hotfixes, no workarounds, no hacks. If a proper fix requires touching multiple files or refactoring a boundary, do it fully.

## Branch Policy

**`main` only.** No feature branches, detached HEADs, or git worktrees — ever.

## Testing Policy

**TDD always.** Write tests before implementation. Every change must maintain ≥70% coverage (`go test -cover ./...`). Do not merge code that drops below this threshold.

## Migration Policy

**Single migration file only.** All schema changes go into `001_init.sql` — edit it in place. No new numbered migration files until the project reaches stable. Drop and recreate your local DB when the schema changes.

## Code Style

**Go idioms and DRY always.** Follow standard Go conventions (`gofmt`, `go vet`, effective Go). Extract shared logic once — no duplicated code paths. Use the stdlib before reaching for helpers. Keep functions small and names self-documenting.

**Design must follow SOLID and DRY.** Every new boundary, capability seam, or runtime flow must have a single clear responsibility, depend on abstractions rather than concrete connector-specific details, and avoid duplicated logic or parallel ad hoc paths.

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

**Real refactors only — no transport shims.** Every refactor must move logic to its correct home. Never satisfy a refactor by wrapping or delegating to the old location from the new one. If the call site changes but the logic does not move, it is not a refactor — undo it and do it properly.

## Naming Policy

**No phase or version names in code.** Never embed milestone, phase, or version labels in identifiers, comments, or constants (e.g. no `phase1`, `m2Handler`, `v2Route`). Names must describe what something *is*, not when it was added. Exception: `// TODO(m3): ...` notes are allowed.

## Scope Policy

**Out-of-scope changes get a TODO, not an implementation.** If a correct fix or feature belongs to a future milestone, add `// TODO(mX): <description>` at the relevant call site and stop. Do not implement deferred work inline without explicit instruction.

## Current System

Use [docs/system.md](docs/system.md) as the source of truth for the shipped package map and operator surface.

The important current boundaries are:

- `gistclaw serve` starts the daemon and local web host.
- SQLite is the only state store. No transcript files or JSON side stores.
- All journal-backed state changes still flow through `ConversationStore.AppendEvent`.
- `runtime` must not depend on `web`.
- Web write paths go through `runtime`; they do not mutate runtime state directly through ad hoc SQL.
- Live run updates use SSE. No WebSocket control plane.
- Provider adapters belong under `internal/providers/`, tools under `internal/tools/`, connectors under `internal/connectors/`, and team files under `teams/`.
- The shipped live connector surfaces are Telegram and WhatsApp. Do not imply broader connector coverage than the code actually wires today.

## Design System

Always read `DESIGN.md` before making any visual or UI decision.
All navigation, page hierarchy, graph placement, typography, colors, spacing, motion, and aesthetic direction for the SvelteKit rewrite are defined there.
Do not deviate without explicit user approval.
In QA mode, flag any code that does not match `DESIGN.md`.

## UI Language & Concern Separation Policy

- UI text must use the user's vocabulary — never internal field names, enum values, or DB identifiers.
- Navigation and page naming must default to user tasks and outcomes before system nouns or implementation concepts.
- Each page/panel serves one user task. Workflows that differ must live in separate surfaces.

## Documentation

Read in this order to onboard:
1. `README.md` — product and quick-start overview
2. `docs/system.md` — current shipped system and package ownership
3. `docs/vision.md` — long-term product direction
4. `docs/kernel.md` — runtime invariants
5. `docs/roadmap.md` — current priorities and non-goals
6. `docs/extensions.md` — extension seam rules
7. `DESIGN.md` — visual system for the local web UI
