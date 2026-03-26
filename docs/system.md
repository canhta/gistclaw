# Current System

This document is the source of truth for what the repository currently ships.

Use it for package ownership, runtime shape, and operator surface. Use the other docs for product direction, kernel rules, future roadmap, and extension philosophy.

## User Value First

If you are evaluating GistClaw as a tool, the current value is:

- you can run it locally and keep control of state, approvals, and replay on your own machine
- you can treat it like one assistant at the surface while still getting multi-agent coordination underneath
- you can inspect what happened after a run through replay, sessions, routes, deliveries, and memory instead of trusting a black box

## What Ships Today

- A single Go binary, `gistclaw`, with daemon and operator commands.
- A local web host with starter-project onboarding plus operator-job pages grouped under Operate, Configure, and Recover.
- A journal-backed runtime that records runs, session collaboration, approvals, receipts, route bindings, and outbound delivery state in SQLite.
- A SQLite-backed scheduler service for local scheduled tasks, occurrence history, restart repair, and CLI-first schedule management.
- Provider adapters for Anthropic and OpenAI-compatible endpoints.
- A tool registry with built-in web fetch, optional Tavily search, and optional MCP stdio tools.
- Live external surfaces for Telegram DM and WhatsApp.
- A default team definition under [teams/default/team.yaml](/Users/canh/Projects/OSS/gistclaw/teams/default/team.yaml).

## Runtime Model

- The daemon owns mutable state in `runtime.db`.
- The append-only conversation journal is still the canonical write path.
- Current-state tables project that journal into runs, approvals, tool calls, receipts, sessions, bindings, deliveries, and memory items.
- A conversation can have one active root run at a time.
- Collaboration happens through runtime-managed sessions and session messages.
- Risky tool calls still require explicit approval before workspace writes are applied.

## Operator Surfaces

### CLI

- `gistclaw serve` starts the daemon and local web host.
- `gistclaw run` submits a task directly from the CLI.
- `gistclaw inspect` reports status, runs, replay, and the admin token.
- `gistclaw schedule` adds, updates, lists, shows, runs, enables, disables, and deletes scheduled tasks.
- `gistclaw doctor` checks config, database, provider, workspace, research, MCP binaries, Telegram reachability, and disk headroom.
- `gistclaw backup` creates a timestamped SQLite backup.
- `gistclaw export` writes runs, receipts, and approvals to JSON.

### Web

- `/onboarding` starts with a starter project, then lets the operator keep it, bind an existing repo, or create a new project elsewhere before the first run.
- The shell includes a project switcher that updates the active workspace context without turning project selection into a primary Settings job.
- `/operate/runs` and `/operate/runs/{id}` show run state and live replay, with the orchestration graph kept on runs and run detail. The runs queue defaults to the active project with an explicit all-projects filter.
- `/operate/sessions` and `/operate/sessions/{id}` expose session mailbox history, route state, and delivery failures.
- `/operate/start-task` starts a new operator task from the web surface.
- `/configure/team` edits and exports the runtime team definition.
- `/configure/memory` lists, edits, and forgets stored facts.
- `/configure/settings` updates machine-level operator settings such as budgets and tokens, and keeps raw workspace editing only as an advanced override.
- `/recover/approvals` resolves risky tool actions.
- `/recover/routes-deliveries` exposes connector health, route bindings, route history, and delivery retry actions.

### Connectors

- Telegram is wired through long polling and DM-focused control commands.
- WhatsApp is wired through a webhook handler plus outbound delivery.
- Connector helper packages also exist for control-plane delivery plumbing and SMTP email, but bootstrap currently wires Telegram and WhatsApp for live runtime use.

## Extension Seams

- Providers live in `internal/providers/`.
- Tools live in `internal/tools/`.
- Connectors live in `internal/connectors/`.
- Team definitions live in `teams/`.

These seams are real in the codebase today. The runtime should depend on their interfaces, not on hardcoded product-specific shortcuts.

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
internal/memory/                  memory storage, search, summarize, promote, edit, forget
internal/model/                   shared runtime types and interfaces
internal/providers/anthropic/     Anthropic provider adapter
internal/providers/openai/        OpenAI-compatible provider adapter
internal/providers/providerutil/  shared provider helpers and error translation
internal/replay/                  replay loading and receipt/preview projections
internal/runtime/                 run loop, collaboration, approvals, routing, delivery recovery
internal/scheduler/               schedule definitions, claiming, repair, reconciliation, CLI-facing service
internal/sessions/                session directory, routes, pagination, delivery listings
internal/store/                   SQLite open/migrate helpers and schema
internal/teams/                   team.yaml validation
internal/tools/                   tool registry, policy, approvals, MCP, research, workspace apply
internal/web/                     HTTP server, SSE, pages, JSON APIs
teams/default/                    shipped team and soul files
```

## Current Gaps

The codebase is past the original reset, but a few surfaces are still intentionally incomplete:

- broader connector and gateway coverage
- dedicated web schedule pages
- operator-friendly team selection and editing
- richer packaging and deployment guidance
- more polished extension workflows beyond the current tool and provider seams
