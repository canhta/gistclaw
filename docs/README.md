# GistClaw Docs

## What this folder is

This folder is a code-grounded redesign dossier for GistClaw, a clean-slate, OpenClaw-inspired runtime.

It is not:

- a port plan
- a "rewrite OpenClaw feature-for-feature" checklist
- a generic agent platform pitch

The target product is a local-first agent team runtime that is easy to inspect, calm when idle, and safe to trust with real repo work.

## Read this first

**Start here for orientation:**

1. [`00-system-diagrams.md`](00-system-diagrams.md) — system shape at a glance

**Build path (read in order):**

1. [`implementation-plan.md`](implementation-plan.md) — canonical file-by-file plan for all 4 milestones; locked tool set, team constraints, decision rule
2. [`dependencies.md`](dependencies.md) — Go 1.25+, direct dependencies, stdlib table, exclusion list
3. [`11-architecture-redesign.md`](11-architecture-redesign.md) — full architecture rationale
4. [`12-go-package-structure.md`](12-go-package-structure.md) — day-one packages, dependency directions, ownership rules
5. [`13-core-interfaces.md`](13-core-interfaces.md) — locked interfaces: RunEventSink, ConversationStore, BudgetGuard, MemoryStore, WorkspaceApplier
6. [`16-roadmap-and-kill-list.md`](16-roadmap-and-kill-list.md) — what is deferred and why

**Per-milestone execution plans (no-code, step-by-step):**

| Milestone | Plan |
|-----------|------|
| 1 — Kernel Proof | [`superpowers/plans/2026-03-24-m1-kernel-proof.md`](superpowers/plans/2026-03-24-m1-kernel-proof.md) |
| 2 — Local Beta | [`superpowers/plans/2026-03-24-m2-local-beta.md`](superpowers/plans/2026-03-24-m2-local-beta.md) |
| 3 — Public Beta | [`superpowers/plans/2026-03-24-m3-public-beta.md`](superpowers/plans/2026-03-24-m3-public-beta.md) |
| 4 — Stable 1.0 | [`superpowers/plans/2026-03-24-m4-stable-1-0.md`](superpowers/plans/2026-03-24-m4-stable-1-0.md) |

**Product and scope reviews:**

- [`17-ceo-review.md`](17-ceo-review.md) — product ambition and scope decisions
- [`18-eng-review.md`](18-eng-review.md) — locked engineering decisions
- [`designs/v1-implementation-plan.md`](designs/v1-implementation-plan.md) — CEO-level scope decisions summary

**Evidence base (background reading):**

- [`01-docs-vs-code-map.md`](01-docs-vs-code-map.md) through [`10-openclaw-weaknesses.md`](10-openclaw-weaknesses.md) — analysis behind the redesign
- [`14-memory-model.md`](14-memory-model.md) — memory scope, promotion, and durable model
- [`15-security-model.md`](15-security-model.md) — workspace enforcement, approval gates, audit trail

## Proposal in one paragraph

Ship a single-binary daemon that can run a small, operator-defined agent team against one workspace root, keep one durable event journal in SQLite, show replay and receipts locally, ask for approval before side effects, and stay quiet when idle.

Start local-first.

Earn Telegram DM after the local loop works.

Do not rebuild OpenClaw's broad gateway, plugin host, or node mesh.

## What to keep from OpenClaw

- explicit conversation and session identity
- agent-scoped state
- access-control-before-intelligence
- visible cost and usage
- operator-authored behavior files
- serious operational thinking

## What to drop from OpenClaw

- the WebSocket control plane as the product center
- split truth across JSON, JSONL, markdown, and DB
- multi-surface connector ambition in v1
- in-process third-party plugins
- session-shaped delegation as the main collaboration model
- policy stacks that require archaeology to explain

## Hard scope for the first useful release

- single binary
- SQLite journal plus projections
- one provider adapter
- one editable starter team
- one workspace root per run
- local web UI
- repo-task workflow with preview, apply, and verify
- approvals
- receipts and simple replay
- per-run budget controls

## Explicitly not in the first useful release

- Telegram
- replay sharing or export
- compare views that need their own product surface
- Telegram groups
- GitHub publish-back
- vector memory
- plugin runtime or marketplace
- drag-and-drop team composer
- autonomous background agent loops

## Decision rule

Whenever there is a choice between:

- a feature that makes the first repo-task loop clearer
- and a feature that makes the platform broader

choose the first.
