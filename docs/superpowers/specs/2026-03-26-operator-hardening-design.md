# Operator Hardening Design

Date: 2026-03-26

## Summary

This design hardens `gistclaw`'s current local-first runtime without reopening OpenClaw-style platform sprawl.

The work is split into four productized slices:

1. security audit
2. connector supervision
3. team profiles
4. storage hygiene

The intent is to make the current shipped system safer and more operator-complete, not broader.

## Problem

`gistclaw` already ships the right kernel: journal-backed runtime state, approvals before risky writes, replay, sessions, route recovery, and a local web control plane. What is still missing is the operator hardening layer around that kernel.

Today:

- `gistclaw doctor` is mostly config and liveness oriented instead of a true risk audit.
- connector lifecycle is start-or-fail, with warnings in connector loops but no bounded runtime-owned supervision.
- team editing exists, but team selection is still effectively one workspace-owned default.
- storage is durable but not yet operator-friendly to inspect, budget, or maintain.

These gaps limit trust in the shipped system more than the lack of additional connectors.

## Goals

- Give operators a first-class risk audit surface for the currently shipped system.
- Add bounded connector health monitoring and recovery without bypassing runtime boundaries.
- Move team composition from a single implicit workspace copy toward explicit named profiles.
- Make database and replay storage health inspectable before size, backup age, or maintenance becomes an emergency.

## Non-Goals

- No WebSocket control plane.
- No OpenClaw channel matrix expansion.
- No plugin marketplace or installation UX.
- No file-backed session store or transcript-first architecture.
- No weakening of the append-only journal as the canonical write path.
- No onboarding redesign in this slice.

## Design Principles

### Preserve the Current Kernel

The append-only journal remains canonical. New health, audit, and maintenance surfaces may inspect state and trigger runtime-owned actions, but they do not create alternate write paths.

### Keep Boundaries Real

- `runtime` stays independent from `web`.
- connector-specific behavior stays in connector packages.
- app lifecycle owns supervision policy.
- interfaces are defined in the consuming package.

### Prefer Operator Completeness Over Platform Breadth

The next gain is not "more surfaces." The next gain is "the shipped surfaces are safer, clearer, and easier to recover."

## Slice 1: Security Audit

### Outcome

Operators can run a real audit that explains whether their current deployment is safe enough for the system they are actually running.

### Scope

- add a dedicated `internal/security/` package for audit findings, severities, and checks
- expose `gistclaw security audit` as a new CLI surface
- optionally allow `gistclaw doctor` to include or link to the same findings

### Checks

- web listen address and admin-token posture
- provider and research config completeness
- MCP command transport safety and binary existence
- workspace-root existence and writability
- connector config sanity for currently shipped Telegram and WhatsApp surfaces
- approval and tool-policy posture where the current config exposes it

### Output

The audit should return structured findings with:

- check ID
- severity
- title
- detail
- remediation

This starts CLI-first. The web UI can consume the same finding structure later.

## Slice 2: Connector Supervision

### Outcome

Connectors stop being "start a loop and hope." The app owns liveness policy and bounded restart behavior.

### Scope

- add app-owned connector supervision interfaces
- add health snapshots for connectors that can report them
- supervise restarts with startup grace, stale thresholds, cooldown, and bounded retry budgets

### Telegram

Telegram is the best candidate for full supervision because it owns a long-polling loop. The supervisor should detect:

- repeated start failures
- stalled receive loops
- outbound drain failures that indicate degraded state

### WhatsApp

WhatsApp should not pretend to have socket health it does not actually own. Instead, health should reflect:

- webhook freshness
- outbound delivery queue pressure
- repeated outbound failures

### Product Surface

- `doctor` reports connector health summary
- Recover pages can show degraded vs healthy connector state
- restart policy remains app-owned, not ad hoc connector self-restart logic

## Slice 3: Team Profiles

### Outcome

Each project can have multiple named team profiles instead of one hidden default directory.

### Scope

- move from `.gistclaw/teams/default/` as the only editable target to `.gistclaw/teams/<profile>/`
- persist the active profile per project in SQLite settings or project state
- let operators select, clone, create, delete, import, and export profiles from the Team page

### Important Constraint

This slice should not couple itself to onboarding in the first pass.

There are already staged onboarding/runtime changes in the repo, and team-profile work does not need onboarding integration to deliver value. The initial product surface is `Configure > Team`.

### Runtime Rule

Runs continue to bind an execution snapshot at start time. Changing the active team profile only affects future runs.

## Slice 4: Storage Hygiene

### Outcome

Operators can understand whether local state is healthy before the database becomes large, backups drift stale, or retention becomes unclear.

### Scope

- add store-health reporting from SQLite and filesystem metadata
- report database size, WAL size, free disk, and backup recency
- support safe maintenance recommendations and commands

### Deliberate Limit

Do not introduce destructive pruning of the append-only journal in this slice.

The first storage pass is inspection and operator-guided maintenance:

- backup status
- WAL checkpoint / vacuum guidance
- table-size visibility
- optional retention settings for derived or recoverable views only

## Cross-Cutting Architecture

```text
CLI / Web
    |
    v
app lifecycle + operator commands
    |
    +--> runtime
    +--> security audit package
    +--> connector supervision
    +--> store health package
    |
    v
SQLite + journal-backed projections
```

### Data Ownership

- security audit reads config, store metadata, and connector/runtime summaries
- connector supervision reads connector health and lifecycle state, then starts/stops connectors through app-owned policy
- team profile selection writes project-owned profile choice and team files
- storage hygiene reads SQLite/filesystem state and triggers explicit maintenance actions only

## Sequencing

Recommended order:

1. security audit
2. connector supervision
3. team profiles
4. storage hygiene

This order hardens the shipped system first and delays the only slice with likely overlap against the staged onboarding work.

## Risks

### Scope Creep Into Platform Breadth

Risk: the connector and team slices drift into "more connectors" or "full assistant-platform builder."

Mitigation: keep all work anchored to the current roadmap and shipped surfaces.

### Hidden Coupling In Connector Lifecycle

Risk: supervision logic leaks connector-specific assumptions into `app`.

Mitigation: define tiny optional health and control interfaces in `internal/app`, then use type assertions so non-reporting connectors still satisfy `model.Connector`.

### Team Profile Drift

Risk: team-profile selection silently changes the semantics of old runs.

Mitigation: keep current run snapshot binding unchanged; profile selection only affects future runs.

### Storage Maintenance Too Aggressive

Risk: the first maintenance pass deletes state that replay or recovery still depends on.

Mitigation: first ship inspection and explicit maintenance, not automatic journal pruning.

## Success Criteria

- operators can run `gistclaw security audit` and get actionable findings
- degraded connectors are visible and bounded instead of silently looping warnings forever
- projects can switch among named team profiles from the Team page
- `doctor` or `inspect status` shows storage health with actionable maintenance guidance
- all slices preserve the current journal-first runtime contract

## Accepted Direction

Implement all four slices, but ship them as serial, reviewable changes on `main` rather than one broad refactor.
