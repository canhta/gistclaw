# Roadmap and Kill List

## Build order

The replacement should be built in stages that preserve simplicity. Do not start with the most generic architecture.

## Release ladder

`11-architecture-redesign.md` captures the target shape.

`19-buildable-v1-plan.md` captures the practical build order.

This roadmap is the short version:

- earn the kernel first
- earn the local product loop second
- add Telegram only after the local loop works
- use stable `1.0` for hardening, not for sneaking in new primitives

## Phase 0: lock the thesis

Deliver:

- final architecture doc
- schema sketch
- package layout
- CLI surface

Exit criteria:

- no unresolved debate about persistence, delegation, soul, or tool security

## Phase 1: kernel proof

This is an internal milestone only.

Build:

- `gistclaw serve`
- SQLite store and migrations
- one provider adapter
- one CLI-driven run surface
- one root run plus one child run
- soul loading
- append-only conversation event log
- working-summary compaction
- interrupted-run recovery
- read-only inspect commands

Do not build yet:

- web UI
- Telegram
- approvals UI
- memory editor

Why first:

- prove the run engine and data model before adding transport and collaboration

Exit criteria:

- one repo task can run end to end
- the run is replayable from durable events
- the system records a usable receipt
- restart moves unfinished runs into explicit `interrupted`

## Phase 2: local beta

This is the first milestone that should feel like a product instead of a runtime demo.

Build:

- first-class local web UI
- one default file-backed team
- bounded agent capabilities plus bounded handoff edges
- explicit child-run delegation
- one polished starter workflow
  Workflow choice: `Repo Task Team`
- preview, apply, and verify loop inside one workspace root
- replay tree or graph and run timeline
- completion receipts
- approval queue in the local web UI
- per-run budgets
- quiet-state trust surface
- grounded why-explanations for the highest-signal events
- approval-gated apply path inside allowed workspace roots
- verification loop that runs the most relevant checks and records the result

Keep intentionally simple:

- team specs stay file-backed
- no drag-and-drop or graph composer
- no memory editor
- no Telegram
- no replay sharing or export
- no compare view unless it falls out cheaply from receipt data

Exit criteria:

- a user can bind one repo and run one task without CLI archaeology
- the starter workflow can preview, apply, and verify a concrete change
- the replay explains what happened without transcript scraping
- the receipt shows model, cost, verification, and approval outcome
- the system stays quiet when idle

## Phase 3: public beta

Build:

- custom team definitions
- structured team editor backed by the same files
- structured soul editor
- basic memory inspector
- exactly one real external connector
  Connector choice: Telegram
- Telegram scope: direct messages only
- rare Telegram milestones only: started, blocked, approval needed, finished
- daily budget caps
- grounded why-cards for delegation, approval, memory load, and budget stop

Still defer:

- Telegram groups
- replay sharing or export
- GitHub publish-back
- inline medium-risk or high-risk Telegram approvals
- compare view if it needs bespoke UI work

Exit criteria:

- an operator can edit a team without inventing new runtime primitives
- Telegram DM feels like one clear teammate, not a role chorus
- active work is inspectable locally even when the trigger came from Telegram
- dangerous approvals still force the higher-trust local surface

## Phase 4: stable 1.0

Build:

- onboarding polish
- backup and export
- doctor command
- low-risk Telegram approvals
- retention and recovery hardening
- docs that match the product

Do not add new primitives here.

Exit criteria:

- restarts are boring
- approvals are auditable
- idle burn is effectively zero
- the default repo-task workflow feels polished instead of merely functional

## Phase 5: only then widen connector coverage

Possible later additions:

- Telegram groups
- replay sharing and redacted export
- explicit GitHub publish-back for repo-task runs
- Slack
- one mail connector
- node-level intervention and targeted reruns
- team blueprints and starter packs
- WhatsApp
- calendar connector
- embedding-backed retrieval
- Postgres
- out-of-process extensions

These should be earned, not assumed.

## Kill list

These ideas should be deleted from the clean-slate design from day one.

### 1. General-purpose WS control plane

Reason:

- too much surface
- not needed for first useful product

### 2. ACP-style second runtime

Reason:

- duplicates the delegation model

### 3. Transcript files plus metadata files

Reason:

- split truth

### 4. In-process third-party code plugins

Reason:

- bad trust boundary

### 5. Daily markdown memory as runtime memory

Reason:

- prompt bloat
- weak queryability

### 6. Many-layer tool policy composition

Reason:

- hard to explain
- hard to audit

### 7. Thread-bound delegation as a core primitive

Reason:

- blurs tasks and conversations

### 8. Startup config mutation

Reason:

- surprising
- hard to debug

### 9. Many restart modes

Reason:

- hidden operational complexity

### 10. Broad automation framework before stable release

Reason:

- cron is enough initially
- free-roaming proactive agents would undermine the low-burn trust story

## Non-goals before stable release

Do not spend time on:

- plugin marketplace
- multi-node clustering
- mobile-app-managed gateway lifecycles
- vector memory
- complex provider fallback graphs
- end-user configurable policy engines

## What absolutely must survive from OpenClaw

Keep these lessons:

- agent identity must be explicit
- memory must be durable and scoped
- tool safety must be structural
- operator-authored behavior files are valuable
- logs and doctor surfaces matter

## First internal implementation milestone

A good first milestone is:

- one binary
- SQLite state
- one provider
- one CLI chat surface
- one delegated child run
- one structured log file
- one durable replay event stream

If that milestone is not dramatically simpler than OpenClaw, the redesign failed.

## Stable Release Bar

Before calling the product stable, it must also have:

- custom operator-defined teams
- structured team editor backed by the same files
- explicit agent-to-agent handoff edges in team definitions
- memory inspector and editor for durable memory
- bounded auto-promotion for typed durable facts
- local-by-default durable memory with explicit publish to team scope
- fixed agent- and phase-based model lanes with explicit escalation
- event-driven and explicit-schedule automation only
- Telegram direct messages only
- one operator-facing agent alias in Telegram DM
- compact completion cards with replay or receipt access in DM
- first-class local web UI
- replay graph inspection
- live replay updates in the local web UI only
- first-class run receipts
- simple comparison only if it fell out cheaply from the same receipt data
- one iconic end-to-end starter workflow
- one preloaded editable default team on first open
- approval-gated apply mode for `Repo Task Team`
- verification loop with recorded evidence for `Repo Task Team`
- one workspace root per root run
- one active root run per canonical conversation
- grounded inline why-explanations
- quiet-by-default trust surface
- scoped persistent memory
- explicit per-run budgets
- low or zero idle burn by default

## Review stop line

At this point the core architecture is locked enough to start implementation.

Do not spend more design time before code on:

- Telegram group semantics
- replay sharing packages or redacted export flows
- non-Git or multi-root apply expansion
- richer delivery guarantees than durable intent plus retry and dedupe
- wider runtime capability surface than the bounded starter model
- extra approval, routing, or scheduler edge mechanics

The next work should be implementation planning against the locked core loop, not deeper architecture branching.
