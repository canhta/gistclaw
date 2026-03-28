# Current System

This document is the source of truth for what the repository ships today: package ownership, runtime shape, and operator surface. Use [docs/vision.md](vision.md) and [docs/roadmap.md](roadmap.md) for direction, and [docs/kernel.md](kernel.md) for invariants.

## What Ships Today

- A single self-contained Go binary, `gistclaw`, with daemon and operator commands.
- GitHub Releases with a blessed Ubuntu 24 installer path and Apple Silicon download path.
- A local web host with a built-in browser login, starter-project onboarding, and operator-job pages grouped under Operate, Configure, and Recover.
- A journal-backed runtime that records runs, session collaboration, approvals, receipts, route bindings, and outbound delivery state in SQLite.
- A SQLite-backed scheduler service for local scheduled tasks, occurrence history, restart repair, and CLI-first schedule management.
- Provider adapters for Anthropic and OpenAI-compatible endpoints.
- A tool registry with built-in web fetch, optional Tavily search, and optional MCP stdio tools.
- Live external surfaces for Telegram DM, WhatsApp, and optional unofficial Zalo Personal messaging.
- A default team definition under [teams/default/team.yaml](../teams/default/team.yaml).
- Project-specific team profiles stored under `storage_root/projects/<project-id>/teams/`, with the machine fallback under `storage_root/teams/default/`.

## Runtime Model

- The daemon owns mutable state in `runtime.db`.
- The append-only conversation journal is still the canonical write path.
- Current-state tables project that journal into runs, approvals, conversational gates, tool calls, receipts, sessions, bindings, deliveries, and memory items.
- A conversation can have one active root run at a time.
- Collaboration happens through runtime-managed sessions and session messages.
- The front assistant is direct-execution by default and receives a runtime execution recommendation (`direct`, `delegate`, or `parallelize`) before each provider turn.
- Raw specialist spawning is now guarded by that recommendation, so tasks classified as `direct` must use local capabilities instead of spawning by default.
- Structured delegation is available for specialist work, so the front assistant can request `research`, `write`, `review`, or `verify` work without choosing the worker topology itself.
- Risky tool calls still require explicit approval before mutating writes are applied.
- Connector-bound front sessions can surface blocked approvals as conversational gates, letting the same chat collect approval or denial and resume the run.
- Outbound delivery can carry transport-agnostic action buttons so chat connectors may offer deterministic approval controls without leaving the active conversation.

## Operator Surfaces

### CLI

- `gistclaw serve` starts the daemon and local web host.
- `gistclaw version` prints the running release/build metadata.
- `gistclaw auth set-password` bootstraps or resets the built-in browser password.
- `gistclaw auth zalo-personal login`, `logout`, `contacts`, `groups`, `send-text`, `send-image`, and `send-file` manage optional Zalo Personal credentials, CLI QR auth, target lookup, and operator sends.
- `gistclaw run` submits a task directly from the CLI.
- `gistclaw inspect` reports status, runs, replay, the canonical `systemd` unit, the admin token through `inspect token`, and storage health for the current database.
- `gistclaw security audit` reports deployment-risk findings for the current config and runtime posture.
- `gistclaw schedule` adds, updates, reports scheduler status, lists, shows, runs, enables, disables, and deletes scheduled tasks.
- `gistclaw doctor` checks config, database, provider, project paths, research, MCP binaries, Telegram reachability, connector health, storage health, and scheduler state.
- `gistclaw backup` creates a timestamped SQLite backup.
- `gistclaw export` writes runs, receipts, and approvals to JSON.

### Web

- `/login` is the browser gate for operator access and shows a locked setup-required page until a password has been bootstrapped.
- `/onboarding` starts with a starter project, then lets the operator keep it, bind an existing repo, or create a new project elsewhere before the first run.
- Operator UI pages and browser read APIs require the built-in login. Machine automation may still use the admin token path.
- The shell includes a project switcher that updates the active project context without turning project selection into a primary Settings job.
- `/operate/runs` and `/operate/runs/{id}` show run state and live replay, with the orchestration graph kept on runs and run detail. The runs queue defaults to the active project with an explicit all-projects filter.
- `/operate/sessions` and `/operate/sessions/{id}` expose session mailbox history, route state, and delivery failures.
- `/operate/start-task` starts a new operator task from the web surface.
- `/configure/team` selects, creates, clones, deletes, edits, imports, and exports named team profiles for the active project, including base profile, tool family, delegation, and specialist-visibility controls, stored under the operator storage root instead of the repo path.
- `/configure/memory` lists, edits, and forgets stored facts.
- `/configure/settings` updates machine-level operator settings, rotates the operator password, and manages current, active, and blocked browser devices.
- `/recover/approvals` resolves risky tool actions.
- `/recover/routes-deliveries` exposes connector health, route bindings, route history, and delivery retry actions.

### Connectors

- Telegram is wired through long polling, DM-focused control commands, chat-native approval/blocked-state replies, inline approval buttons, and command fallback that resume pending runs from the same DM.
- WhatsApp is wired through a webhook handler plus outbound delivery.
- Zalo Personal is wired through CLI-driven QR authentication, stored SQLite credentials, listener-level retry and duplicate-session recovery, DM plus safe-by-default group handling, friend/group lookup, operator send commands, connector health reporting, and an unofficial reverse-engineered protocol implementation.
- Connector helper packages also exist for control-plane delivery plumbing and SMTP email, while bootstrap currently wires Telegram, WhatsApp, and optional Zalo Personal for live runtime use.

## Extension Seams

- Providers live in `internal/providers/`.
- Tools live in `internal/tools/`.
- Connectors live in `internal/connectors/`.
- Team definitions live in `teams/`.

These seams are real in the codebase today. The runtime should depend on their interfaces, not on hardcoded shortcuts.

## Package Map

```text
cmd/gistclaw/                     CLI entry point and operator commands
internal/app/                     config, bootstrap, lifecycle, command wiring
internal/conversations/           conversation resolution and journal-backed write path
internal/connectors/control/      transport-native commands like /help and /status
internal/connectors/delivery/     shared delivery helpers
internal/connectors/email/        SMTP outbound connector helpers
internal/connectors/telegram/     Telegram inbound and outbound runtime integration
internal/connectors/whatsapp/     WhatsApp webhook and outbound runtime integration
internal/connectors/zalopersonal/ Zalo Personal auth state, connector loop, inbound and outbound runtime integration
internal/connectors/zalopersonal/protocol/ Reverse-engineered Zalo Personal HTTP, crypto, and WebSocket transport
internal/memory/                  memory storage, search, summarize, promote, edit, forget
internal/model/                   shared runtime types and interfaces
internal/providers/anthropic/     Anthropic provider adapter
internal/providers/openai/        OpenAI-compatible provider adapter
internal/providers/providerutil/  shared provider helpers and error translation
internal/replay/                  replay loading and receipt/preview projections
internal/runtime/                 run loop, collaboration, approvals, routing, delivery recovery
internal/runtime/recommendation/  execution recommendation engine for direct, delegated, and parallel work
internal/scheduler/               schedule definitions, claiming, repair, reconciliation, CLI-facing service
internal/sessions/                session directory, routes, pagination, delivery listings
internal/store/                   SQLite open/migrate helpers and schema
internal/teams/                   team.yaml validation
internal/tools/                   tool registry, policy, approvals, MCP, research, scoped apply
internal/web/                     HTTP server, SSE, pages, JSON APIs
teams/default/                    shipped team and soul files
```

## Current Gaps

The codebase is past the original reset, but a few surfaces are still intentionally incomplete:

- broader connector and gateway coverage
- official Zalo OA or group support
- dedicated web schedule pages
- explicit storage-maintenance commands beyond health reporting
- extra packaging channels beyond GitHub Releases and the blessed installer
- more polished extension workflows beyond the current tool and provider seams
