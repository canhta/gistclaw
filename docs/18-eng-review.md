# Engineering Review Reflection

## Source of truth

This pass reviews the redesign as an implementation plan, not as a product-vision exercise.

The engineering posture is:

- keep the OpenClaw-like feel
- reduce stable-release scope to the realistic core loop
- prefer boring ownership boundaries
- cut integration surfaces that are not required to prove the core system

## Step 0: scope challenge

Accepted direction:

- keep the architecture and product thesis
- implement it through a release ladder:
  - kernel proof
  - local beta
  - public beta
  - stable `1.0`
- keep stable `1.0` centered on:
  - single binary
  - local web UI
  - replay and receipts
  - custom teams
  - disciplined memory
  - `Repo Task Team`
  - approvals
  - Telegram DM

Deferred from stable release:

- Telegram groups
- polished replay-page sharing
- GitHub publish-back
- compare view if it needs its own bespoke product surface

Reason:

- the earlier docs still mixed the eventual product with the first shippable milestones
- the ladder preserves the product ambition while making the implementation plan executable

## Codex side review

Attempted:

- independent Codex plan review against the design doc

Observed:

- the local Codex install emitted skill-loading and local state DB warnings
- the run did not return a clean final review artifact

Usable signal:

- the same hot spots still surfaced:
  - session and delegation boundaries
  - tool policy simplification
  - approval surfaces
  - cost and usage accounting

Judgment:

- treat the Codex pass as weak corroboration, not a gating review result

## Architecture review

### Accepted

#### 1. Daemon owns all mutable state

Decision:

- accepted

Reason:

- multiple writers would make approvals, replay, and live surfaces harder to reason about
- single-writer ownership is the boring and debuggable choice for a one-engineer-operated runtime

Implementation direction:

- only the daemon writes `runtime.db`
- local web UI, CLI, and Telegram DM send commands through the daemon
- worker subprocesses remain execution sandboxes, not state owners

### Accepted

#### 2. Collapse stable internal abstractions aggressively

Decision:

- accepted

Reason:

- the reduced stable-release scope no longer justifies interface-per-helper or package-per-helper design
- too many early abstractions would slow implementation and make the runtime harder to understand for one engineer

Implementation direction:

- keep interfaces only at real external seams such as store, provider, connector, tool, approval gate, and run engine
- keep first-run, presentation, and later-phase sharing logic as concrete services inside a few packages
- collapse separate ingest, delivery, and admin UI package ideas into concrete code under connectors and API

### Accepted

#### 3. Canonical conversation key spec

Decision:

- accepted

Reason:

- DM continuity, approvals, replay stitching, and memory scope all depend on naming the same conversation the same way every time
- this is the kind of identity primitive that is cheap now and painful later

Implementation direction:

- define one structured `ConversationKey` plus one normalized string form
- normalize connector id, account id, team id, external conversation id, and thread id through one conversations package
- keep actor identity out of the durable conversation key

### Accepted

#### 4. Append-only event journal plus projections

Decision:

- accepted

Reason:

- replay, receipts, activity, and debugging get much harder when each one invents its own history source
- one journal plus read-optimized projections is the boring shape that keeps failure analysis and product inspection aligned

Implementation direction:

- treat `events` as the append-only runtime journal
- persist run lifecycle, delegation, tool, approval, verification, memory, and delivery milestones into that journal
- keep `runs`, `approvals`, `tool_calls`, `summaries`, and other current-state tables as projections or indexes
- build replay, live replay, and receipts from the same journal plus projections

### Accepted

#### 5. Explicit interrupted state on restart

Decision:

- accepted

Reason:

- automatic recovery sounds elegant until it creates half-resumed runs, duplicate side effects, and impossible debugging sessions
- a clean interrupted state plus explicit resume or rerun action keeps restart behavior boring and trustworthy

Implementation direction:

- daemon startup reconciles unfinished runs into explicit `interrupted` state
- interrupted runs stay visible in replay and status surfaces until the operator chooses resume or rerun
- stable release does not auto-resume runs on boot or config restart

### Accepted

#### 6. One active root run per canonical conversation

Decision:

- accepted

Reason:

- multiple root runs writing into the same chat thread would blur approvals, memory, receipts, and user expectations fast
- keeping one root owner per conversation preserves clarity while still allowing rich delegation inside the active run

Implementation direction:

- enforce one active root run per canonical conversation
- allow parallel child delegations only under that root run
- queue or explicitly attach new inbound asks for that conversation instead of creating competing root runs

### Accepted

#### 7. Snapshot-bound approvals

Decision:

- accepted

Reason:

- broad approvals are convenient until they quietly turn into trust in future decisions the operator never actually reviewed
- single-use snapshot approvals keep the trust boundary concrete, replayable, and explainable

Implementation direction:

- each approval ticket binds to one concrete action bundle: tool, normalized arguments, target scope, risk summary, and preview when available
- if the proposed action changes materially, the ticket expires and the runtime must ask again
- stable release does not support run-wide elevated windows as the primary approval model

### Accepted

#### 8. Root-run execution snapshot

Decision:

- accepted

Reason:

- if team wiring, soul, or tool policy changes mid-run, replay and approvals stop describing a stable reality
- freezing the execution snapshot at root-run start keeps active work predictable while still letting operators edit the next run immediately

Implementation direction:

- each root run records a frozen execution snapshot including team spec, soul, tool policy, and relevant config
- child delegations inherit from that snapshot
- config and spec edits apply only to new root runs

### Accepted

#### 9. Durable outbound intent with retry and dedupe

Decision:

- accepted

Reason:

- outbound chat delivery sits in the messy real world where crashes and retries happen
- recording intent first and treating delivery as at-least-once is much more honest and operable than pretending the live path can guarantee exactly-once behavior

Implementation direction:

- record outbound intents durably before connector delivery
- use retry with connector-specific dedupe keys where available
- project intent, attempts, confirmations, and terminal failures into replay and receipts
- stable release does not chase exactly-once delivery for Telegram DM

### Accepted

#### 10. Small fixed child-worker budget per root run

Decision:

- accepted

Reason:

- multi-agent systems get expensive and chaotic fast when coordination logic can mint parallel work without a hard cap
- a small explicit worker budget keeps teamwork real while making cost, CPU, and replay shape predictable

Implementation direction:

- each root run gets a bounded active-child budget
- extra delegations queue behind that budget with visible backpressure
- coordination logic may choose ordering, but it may not bypass the concurrency cap

### Accepted

#### 11. Narrow side-effect ownership by capability

Decision:

- accepted

Reason:

- spreading mutation power across every agent makes approvals, replay, and incident analysis much harder to trust
- keeping side effects in a narrow executor set preserves clear authority lines without killing team flexibility

Implementation direction:

- agents with workspace-write capability own workspace mutation
- agents marked operator-facing own outbound operator messaging
- most other agents remain read-heavy or propose-only by default

### Accepted

#### 12. One workspace root per root run

Decision:

- accepted

Reason:

- multi-root runs would blur previews, approvals, verification, and incident analysis across too much filesystem territory
- one declared workspace root keeps the side-effect boundary obvious and makes replay easier to trust

Implementation direction:

- each root run records one declared workspace root
- child runs inherit that workspace root
- stable release does not support one root run mutating across multiple repos or roots

## Review stop line

The architecture review has gone deep enough on the stable-release core.

Locked core decisions now cover:

- single-writer daemon ownership
- append-only event journal plus projections
- one active root run per canonical conversation
- snapshot-bound approvals
- root-run execution snapshots
- durable outbound intent with retry and dedupe
- bounded child concurrency with backpressure
- narrow side-effect ownership by capability
- one workspace root per root run

Do not branch further into edge-policy design before implementation.

Defer remaining nuance until code proves a real gap, especially around:

- Telegram group behavior
- replay sharing and export packages
- broader apply eligibility rules
- deeper connector-specific delivery edge cases
- richer coordination heuristics or scheduler policy
