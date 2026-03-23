# Multi-Agent and Delegation

## What this subsystem is trying to solve

OpenClaw wants many agents to coexist inside one gateway and collaborate without running separate products or separate databases.

That is a worthwhile goal. The problem is that the collaboration model sprawls across too many mechanisms.

## Structural isolation that really exists

Core files:

- `src/agents/agent-scope.ts`
- `src/agents/agent-paths.ts`
- `src/routing/session-key.ts`
- `src/config/sessions/paths.ts`
- `src/agents/auth-profiles/*`

What is real:

- each agent gets a normalized `agentId`
- workspace can differ per agent
- `agentDir` can differ per agent
- session stores are namespaced per agent
- auth profiles are per agent

This is not fake isolation. OpenClaw genuinely has useful agent boundaries.

## Routing and binding model

Core file:

- `src/routing/resolve-route.ts`

Binding precedence is deterministic and reasonably clear:

1. peer
2. parent peer
3. guild plus roles
4. guild
5. team
6. account
7. channel
8. default agent

This part is better than most assistant systems. The routing rules are concrete.

## Ordinary subagent spawning

Core files:

- `src/agents/subagent-spawn.ts`
- `src/agents/subagent-control.ts`
- `src/agents/subagent-registry.ts`
- `src/config/sessions/types.ts`

The normal subagent path already carries a lot of semantics:

- spawn mode `run` or `session`
- optional thread binding
- sandbox inheritance or requirement
- attachment materialization
- workspace inheritance
- explicit child capability resolution
- descendant tracking
- steer and abort behavior

`SessionEntry` stores lineage fields like:

- `spawnedBy`
- `parentSessionKey`
- `spawnDepth`
- `subagentRole`
- `subagentControlScope`

That is real multi-agent runtime state, not just routing metadata.

## ACP is a second delegation system

Core files:

- `src/agents/acp-spawn.ts`
- `src/acp/control-plane/manager.core.ts`
- `src/acp/runtime/*`

ACP adds another path with:

- separate runtime backends
- separate thread-binding logic
- separate session mode and runtime options
- separate parent stream relays
- separate lifecycle tooling

This is the fourth major design smell. The repo has two delegation runtimes in core.

## On-behalf-of and delegate architecture

Docs:

- `https://docs.openclaw.ai/concepts/delegate-architecture`

The docs present a more first-class delegate model than the code actually contains.

What the code really gives you is:

- isolated agents
- deterministic route bindings
- per-agent auth stores
- per-agent tool and sandbox policy
- subagent spawning

That is enough to build a delegate pattern. It is not the same as a dedicated delegate runtime with crisp on-behalf-of semantics.

## Collision and hidden-complexity risks

### 1. Subagent inheritance is broader than advertised

Minimal bootstrap still includes `SOUL.md`, `IDENTITY.md`, and `USER.md`, not only `AGENTS.md` and `TOOLS.md`.

### 2. Session lineage and task lineage are mixed

Sessions are carrying both conversation history and delegation runtime state.

### 3. Thread binding adds another axis of behavior

Thread-bound child sessions blur the line between a task and a continuing conversation.

### 4. ACP raises policy complexity

Sandbox, host access, runtime backend availability, and thread relays all branch separately for ACP.

## What is genuinely strong

### 1. Per-agent state separation

This is worth keeping.

### 2. Deterministic routing

Bindings are concrete instead of magic.

### 3. Child-session lineage fields

OpenClaw at least persists who spawned what.

### 4. Control of child runs exists

Listing, steering, and descendant tracking are real capabilities, not aspirational docs.

## What is over-engineered

### 1. Two delegation runtimes

Subagents and ACP should not both be core.

### 2. Too many control semantics

Control scope, thread binding, steering, descendant counting, and persistent child sessions are too much for the default model.

### 3. Delegate architecture is more concept than primitive

The docs sound cleaner than the runtime actually is.

## Useful abstractions vs fake abstractions

Useful:

- `agentId` namespacing
- routing precedence
- explicit spawn depth and child-role metadata

Fake or too expensive:

- ACP in the core runtime
- thread-bound child sessions as a primary delegation story
- "delegate architecture" as a top-level concept without one tight backing primitive

## Docs vs code

Strong match:

- `https://docs.openclaw.ai/concepts/multi-agent`

Material mismatch:

- `https://docs.openclaw.ai/tools/subagents` understates inherited context
- delegate docs imply a cleaner, more unified runtime than the code implements

## Judgments

- Multi-agent structural quality: high
- Multi-agent behavioral simplicity: low
- Isolation quality: medium-high with policy help
- Debuggability of delegation: low-medium

## What should be simplified

- one delegation mechanism
- one persisted run/task model
- explicit child permissions
- explicit memory scope
- explicit max depth
- inspectable agent-to-agent messages

## What should be deleted

- ACP from v1
- thread-binding complexity as a core delegation primitive
- using sessions as the primary container for delegation state

## What the lightweight redesign should do instead

Use one model:

- `agents`
- `runs`
- `delegations`
- `messages`

Team collaboration should use explicit handoff edges between agents, not free-form peer chat.

The stable release should use a bounded capability model rather than a hardcoded role catalog.

Operators should be able to build whatever team they want by naming agents freely and composing capability postures inside a validated team graph.

Each agent instance may then customize:

- display name
- soul
- tool profile
- memory scope
- budget posture

Stable-release side-effect ownership rule:

- only agents with workspace-write capability own workspace mutation
- only agents marked operator-facing own outbound operator messaging
- most agents are read-heavy or propose-only by default
- side effects should happen in a small number of explicitly capable agents, not everywhere in the team graph

Every child run should declare:

- parent run id
- child agent id
- objective
- workspace root
- tool profile
- memory scope
- time budget
- token budget
- max depth

Root-run worker budget rule:

- each root run gets a small fixed active-child budget
- stable release should default to something boring like 2 to 4 active child runs depending on team profile
- extra delegations queue behind that budget instead of spawning immediately
- replay should show queued, running, blocked, and completed child work distinctly
- coordination logic may choose what to queue next, but it may not mint unlimited parallelism

Workspace boundary rule:

- each root run owns one declared workspace root
- child runs inherit that same workspace root
- stable release does not let one root run mutate across multiple repos or filesystem roots
- if the operator wants work in a second repo, that becomes a separate root run or a later explicit handoff

Conversation concurrency rule:

- one canonical conversation may have only one active root run at a time
- delegation fan-out happens inside that root run, not beside it
- a new inbound ask for the same conversation must either queue behind the active root run or be folded into that run as operator-visible follow-up work
- this keeps approvals, receipts, replay, and memory writes attached to one primary task spine

Every team spec should also declare:

- which capabilities each agent instance holds
- which agents may delegate to which other agents
- which agents may critique or return work to which other agents
- which agents are terminal and may only report upward

Why this wins:

- real teamwork without chat-room chaos
- loop control is structural instead of heuristic
- replay can show allowed and actual handoff edges clearly

No second runtime. No hidden on-behalf-of magic.
