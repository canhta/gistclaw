# Agent Runtime and Sessions

## What this subsystem is trying to solve

OpenClaw needs:

- conversation identity
- durable transcripts
- per-agent isolation
- queueing for overlapping turns
- steering and follow-up behavior
- context compaction

The concepts are right. The implementation is overloaded.

## Main runtime path

The hot path for a normal inbound turn is:

`src/routing/resolve-route.ts` `resolveAgentRoute()` -> `src/auto-reply/reply/session.ts` `initSessionState()` -> `src/agents/pi-embedded-runner/run.ts` `runEmbeddedPiAgent()` -> `src/agents/pi-embedded-runner/run/attempt.ts`

This matters because the "session subsystem" is not isolated. Route selection, session initialization, transcript writes, queueing, tool runs, compaction, and delivery all sit on one path.

## Session identity model

This is one of the strongest parts of OpenClaw.

Important file:

- `src/routing/session-key.ts`

Key properties:

- keys are namespaced by `agentId`
- channel, peer, group, and thread dimensions are explicit
- DM scoping supports multiple strategies
- identity links can canonicalize DM peers

This is a real abstraction with clear value.

## Durable state ownership

Session state is split across multiple layers:

### 1. Session metadata store

Files:

- `src/config/sessions/store.ts`
- `src/config/sessions/paths.ts`
- `src/config/sessions/types.ts`

Holds:

- `sessionId`
- timestamps
- delivery context
- model/provider overrides
- queue settings
- usage
- compaction counters
- memory-flush metadata
- spawned session lineage
- ACP metadata

### 2. Transcript files

Files:

- `src/config/sessions/transcript.ts`
- `src/config/sessions/paths.ts`

Transcript persistence is delegated into `@mariozechner/pi-coding-agent` session machinery.

### 3. In-memory active-run registries and lanes

Files:

- `src/auto-reply/reply/queue/*`
- `src/agents/subagent-registry.ts`

These own live coordination state that is not the same thing as durable transcript state.

This is the second major design smell. One conceptual entity, "a session," has too many storage forms.

## Queueing, streaming, and steering

Core files:

- `src/auto-reply/reply/agent-runner.ts`
- `src/auto-reply/reply/queue/*`

The session runner handles:

- active-run coordination
- queue modes
- typing behavior
- streaming/chunking
- follow-up runs
- steering interrupts
- tool-visibility rules

This is workflow-engine behavior hidden inside a reply runner.

## Compaction and transcript mutation

Core files:

- `src/agents/compaction.ts`
- `src/agents/pi-embedded-runner/compact.ts`
- `src/agents/pi-embedded-runner/tool-result-truncation.ts`
- `src/agents/pi-embedded-runner/session-truncation.ts`

Important judgment:

- compaction is real and thoughtful
- append-only transcript mental models are false

The code rewrites or sanitizes state around:

- oversized tool results
- summary checkpoints
- session truncation
- repair flows

That is practical, but it means the docs oversell audit simplicity.

## DM scoping and leakage risk

Core files:

- `src/routing/session-key.ts`
- `src/security/audit-channel.ts`

OpenClaw supports multiple DM scope modes. That is flexible, but it creates real leakage risk.

The strongest evidence is `collectChannelSecurityFindings()` in `src/security/audit-channel.ts`, which warns when `dmScope="main"` is used in multi-user DM setups.

Judgment:

- per-agent main-session collapse is convenient
- it is not a safe default for a system that can represent many users and channels

## Subagent and ACP lineage inside sessions

Core files:

- `src/config/sessions/types.ts`
- `src/agents/subagent-spawn.ts`
- `src/acp/session.ts`

`SessionEntry` stores:

- `spawnedBy`
- `parentSessionKey`
- `spawnDepth`
- `subagentRole`
- `subagentControlScope`
- `acp`

That is real lineage tracking. It is also another sign that sessions are being used to carry task-runtime state that should probably live in a separate task model.

## What is genuinely strong

### 1. Session keys are excellent

The namespacing model is one of the cleanest parts of the repo.

### 2. Per-agent separation is real

State directories and store resolution are not fake isolation.

### 3. Compaction is treated seriously

Identifier-preservation and tool-result sanitization show good instincts.

### 4. Queueing is more honest than most assistant runtimes

OpenClaw does not pretend turns never overlap. It has real concurrency logic.

## What is over-engineered

### 1. Dual durable state

JSON session store plus transcript files plus in-memory live registries is too much.

### 2. Too much orchestration on the reply path

Delivery, usage, queueing, steering, compaction, and follow-up behavior should not all live together.

### 3. DM scope flexibility is too expensive

`main`, `per-peer`, `per-channel-peer`, and `per-account-channel-peer` are useful, but the default surface is too broad.

## Useful abstractions vs fake abstractions

Useful:

- session keys
- explicit lineage fields
- compaction checkpoints
- queue lanes

Fake or too expensive:

- "append-only transcript" as the operator story
- sessions as the place to store both conversation state and task/delegation runtime state
- a single reply runner as the home for all orchestration

## Docs vs code

Strong match:

- `https://docs.openclaw.ai/reference/session-management-compaction`
- `https://docs.openclaw.ai/concepts/compaction`

Material mismatch:

- docs underplay dual persistence
- docs say subagents cannot call `sessions_spawn`; code allows it under policy
- docs imply stronger append-only guarantees than the code delivers

## Judgments

- Single-binary feasibility: high
- Runtime simplicity: low-medium
- Multi-agent quality: medium-high structurally, lower behaviorally
- Persistence design quality: medium
- Security clarity for DMs: low-medium

## What should be simplified

- make every conversation key explicit by default
- move task/delegation runtime out of session metadata
- store transcript events, metadata, and compaction checkpoints in one database
- reduce queue/steer/follow-up policy to a smaller set of modes

## What should be deleted

- default main-DM collapse
- JSON store plus transcript-file duplication
- append-only marketing around a mutable transcript pipeline

## What the lightweight redesign should do instead

Use SQLite only:

- `conversations`
- `events`
- `runs`
- `summaries`
- `delegations`

Then keep one invariant:

conversation history is an ordered event log; everything else is derived or indexed from that.
