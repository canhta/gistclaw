# Front Assistant Runtime Refactor Design

## Goal

Refactor GistClaw so the user-facing assistant is direct-execution by default and autonomous delegation is an escalation path, not the primary control plane.

The target product behavior is:

- the front assistant answers simple questions directly
- the front assistant executes bounded local actions through first-class capabilities
- the front assistant delegates autonomously only when the work actually requires a specialist
- delegation remains visible in runs, replay, and graphs

This is a full refactor. No backward-compatibility layer is required for the current coordinator-first design.

## Problem

The current shipped system over-delegates because the front agent is structurally a coordinator.

Today:

- the front agent in `teams/default/coordinator.soul.yaml` is instructed to classify work and spawn specialists
- `session_spawn` is exposed as a normal tool
- the front agent is allowed to use it
- direct product capabilities are under-modeled compared to repo and research tools

That makes delegation the easiest path even for bounded local actions. The Zalo contact lookup failure exposed the issue, but the flaw is architectural and affects the whole runtime.

## Design Principles

1. One assistant at the surface.
2. Direct execution before delegation.
3. Delegation remains a first-class visible concept for operators.
4. Deterministic product actions should be explicit runtime capabilities, not prompt inventions.
5. Team topology should not leak into ordinary model reasoning more than necessary.
6. Kernel boundaries stay strict: runtime owns sessions, delegation, approvals, routing, replay, and authority.

## What We Learned From OpenClaw And GoClaw

Useful patterns:

- `openclaw` exposes channel capabilities such as directory lookup, target resolution, auth, and send as normal product surfaces.
- `goclaw` exposes deterministic operational methods through a gateway/router instead of treating those actions as research tasks.

These both point to the same lesson:

- real product actions should be callable capabilities
- specialist delegation should be reserved for specialist work

This refactor adopts that lesson without copying either project architecture.

## Scope

### In Scope

- front-agent contract rewrite
- typed delegation instead of free-form front-run spawning
- explicit capability tool families
- runtime package restructuring for capability execution and delegation orchestration
- team config and policy model rewrite
- documentation rewrite for the new architecture

### Out Of Scope

- connector breadth expansion
- provider expansion
- plugin marketplace or installation UX
- compatibility shims for old team contracts

## Target Architecture

```text
user message
  -> front assistant session
  -> direct capability tools when sufficient
  -> typed delegation request when specialist work is required
  -> runtime delegation orchestrator
  -> specialist worker session
  -> visible replay / graph / receipts / approvals
```

### Front Assistant

The front assistant is the only user-facing voice in a conversation.

Its job:

- understand the request
- choose direct capabilities first
- ask for clarification when ambiguity blocks safe action
- delegate only when the task requires research, write, review, or verification work
- synthesize specialist outputs back into the user-facing thread

It is not a coordinator in the current sense. It does not decompose by default.

### Specialist Workers

Workers remain real runtime actors with their own runs, sessions, replay, and visibility.

Initial worker set:

- `researcher`
- `patcher`
- `reviewer`
- `verifier`

These remain specialist-only. They do not become user-facing.

## Collaboration Model

The kernel collaboration primitives remain valid, but the default usage changes.

### Direct Capability Path

Use this when:

- one tool call can answer the request
- a short deterministic tool chain can answer the request
- the request is a local product action with clear inputs

Examples:

- list connector contacts
- resolve a target by name
- send a connector message
- show connector status
- inspect run status

### Delegation Path

Use this when:

- external research is required
- code changes are required
- explicit review is requested
- explicit verification is requested
- the task is large enough that specialist isolation materially improves correctness

## Runtime Refactor

### Remove Free-Form Front Spawning

The front assistant should no longer receive a generic `session_spawn(agent_id, prompt)` capability.

Instead, the model gets a typed delegation interface such as:

- `delegate_task(kind, objective, context)`

Where:

- `kind` is constrained to known specialist workflows
- `objective` is the user-level or specialist-level target
- `context` carries structured supporting detail

The runtime, not the model, chooses:

- which agent to run
- whether additional specialist stages are required
- whether a follow-up review or verification run is mandatory

This removes arbitrary team-topology reasoning from normal model behavior.

### Add Capability Execution Layer

Create a dedicated runtime-owned capability layer for deterministic actions.

Proposed package:

- `internal/runtime/capabilities/`

Responsibilities:

- register runtime-facing direct capabilities
- validate structured capability inputs
- dispatch to app, connector, or runtime handlers
- normalize results back to tool outputs

This is distinct from `internal/tools/`, which remains the generic tool execution seam exposed to models and providers.

### Add Delegation Orchestration Layer

Create:

- `internal/runtime/delegation/`

Responsibilities:

- map typed delegation requests to specialist workflows
- create worker runs and sessions
- enforce allowed specialist kinds per agent
- attach orchestration metadata for replay and graph rendering

This layer owns specialist fan-out. The front assistant requests delegation; it does not choose arbitrary child topology.

## Tool Model Refactor

The current `tool_posture` model is too coarse. It cannot cleanly express:

- front assistant can use bounded app and connector actions
- front assistant cannot mutate repo files directly
- front assistant can request specialist delegation
- worker agents have narrow tool access tied to their role

### Replace Tool Posture With Explicit Policy Inputs

Refactor the agent/tool contract to support:

- `tool_families`
- `delegation_kinds`
- `approval_profile`

Example shape:

```text
front assistant:
  tool_families:
    - repo_read
    - runtime_capability
    - connector_capability
    - web_read
    - delegate
  delegation_kinds:
    - research
    - write
    - review
    - verify
  approval_profile: operator_facing
```

Worker examples:

- researcher: `web_read`, `repo_read`
- patcher: `repo_read`, `repo_write`
- reviewer: `repo_read`, `diff_review`
- verifier: `repo_read`, `verification`

This replaces the current logic centered on `tool_posture` capability buckets.

### Tool Families

Refactor tools into explicit families instead of policy by historical name only.

Families to support first:

- `repo_read`
- `repo_write`
- `runtime_capability`
- `connector_capability`
- `web_read`
- `delegate`
- `verification`
- `diff_review`

Every tool spec should declare its family in addition to risk and approval behavior.

## Capability Surfaces

The first-class capability approach should be generic, not Zalo-specific.

### Initial Generic Capabilities

- `connector_directory_list`
- `connector_target_resolve`
- `connector_send`
- `connector_status`
- `app_action`

These are runtime-level capability tools that map to registered connector or app handlers.

### Connector Adapters

Connectors can implement capability interfaces for:

- directory lookup
- target resolution
- outbound send
- status

Zalo Personal is the first concrete adapter because it already has app and protocol plumbing, but the design is generic and should also fit future Telegram or WhatsApp operator actions when appropriate.

## Team Definition Refactor

Refactor the shipped default team.

### New Roles

- `assistant`: direct operator-facing front assistant
- `researcher`
- `patcher`
- `reviewer`
- `verifier`

The front assistant should no longer describe itself as a coordinator.

### Team Config Changes

Replace:

- raw `can_spawn`
- coarse `tool_posture`

With:

- `tool_families`
- `delegation_kinds`
- optional `message_targets` if inter-agent messaging remains necessary

If free-form inter-agent messaging is no longer needed for the initial production refactor, remove it from the front assistant contract as well.

## Package And File Structure

### Runtime

Add or refactor toward:

- `internal/runtime/capabilities/`
- `internal/runtime/delegation/`
- `internal/runtime/context/`

Keep:

- `internal/runtime/` as kernel owner

But move logic into these focused units instead of concentrating it inside the current run loop helpers.

### Tools

Refactor `internal/tools/` so files and registration match tool families.

Suggested direction:

- `repo_*.go` stays for repo tools
- `web_*.go` stays for web tools
- `capability_*.go` for direct capability tools
- `delegate_*.go` for typed delegation tools
- `policy.go` rewritten around tool families

### Teams

Refactor:

- `teams/default/team.yaml`
- replace `teams/default/coordinator.soul.yaml`

Likely result:

- `teams/default/assistant.soul.yaml`
- existing worker soul files updated to the new model

## Context Assembly

Provider instructions should reflect the new architecture.

The front assistant context must state:

- direct local capabilities are preferred
- delegation is reserved for specialist work
- the user should not be bounced into a child run for bounded local actions

Worker contexts must stay narrow and role-specific.

## Approval And Authority

Direct capability execution does not weaken current authority boundaries.

Rules:

- read-only capabilities: no approval
- outbound actions with side effects: approval when policy requires
- repo writes: still specialist-owned and approval-gated
- delegation itself: no approval, but visible in replay and run graph

The new architecture should make approvals clearer because direct product actions will no longer be confused with research or coordination.

## Replay And Operator Visibility

Delegation remains first-class visible, per approved product direction.

The run graph and replay should clearly show:

- front assistant direct tool executions
- delegation requests
- spawned specialist runs
- returned outputs
- approvals and blocked states

The UI should still feel like one assistant at the surface, but the operator must be able to inspect delegation without guesswork.

## Data Model And Event Implications

The journal remains the source of truth.

Likely event additions or refactors:

- explicit delegation-requested event
- delegation-resolved or worker-attached event
- capability-executed events with normalized structured metadata

The exact event names can be decided during implementation, but the important rule is:

- replay must distinguish direct capability execution from specialist delegation

## Migration Strategy

No compatibility layer.

Migration plan:

1. introduce the new capability and delegation model in code
2. switch the shipped default team to the new assistant-first design
3. remove old coordinator-first assumptions
4. update docs and operator surfaces to describe the new system
5. update tests to assert direct-first behavior

Any old config or snapshot shape used only for the previous architecture can be deleted or migrated in place without preserving compatibility behavior.

## Testing Strategy

This refactor must be TDD-driven.

### Kernel Tests

- front assistant uses direct capability tool when available
- front assistant does not create a child run for bounded local actions
- front assistant creates a delegation request for research/write/review/verify tasks
- runtime maps typed delegation to the correct specialist workflow

### Policy Tests

- tool visibility by tool family
- delegation kind access by agent
- front assistant denied repo-write tools
- patcher denied connector capability tools

### Team Snapshot Tests

- new team config parses correctly
- new agent profile exposes explicit tool families and delegation kinds
- old coordinator-only assumptions are removed

### Integration Tests

- direct local action from inbound connector message completes in one front run
- code-change task creates worker runs and remains visible in replay
- explicit review request routes through reviewer
- explicit verification request routes through verifier

### Regression Tests

- the original over-delegation case should be covered by a failing test first
- direct capability path must be preferred over delegation for any registered matching capability

## Documentation Changes

Update:

- `README.md`
- `docs/system.md`
- `docs/kernel.md`
- `docs/vision.md`
- `docs/extensions.md`
- `docs/roadmap.md` as needed

Docs must reflect:

- direct-execution front assistant
- typed delegation
- explicit capability surfaces
- visible specialist runs

Remove wording that still implies the front assistant is a coordinator-first agent.

## Risks

### Risk: Partial Refactor Leaves Mixed Semantics

If the new capability/delegation model lands while old `tool_posture` and free-form spawning rules still shape decisions, the runtime will become harder to reason about than it is today.

Mitigation:

- remove old semantics decisively
- refactor policy and team snapshot together

### Risk: Front Assistant Becomes Too Powerful

If the front assistant receives broad mutation tools to avoid delegation, authority boundaries will weaken.

Mitigation:

- direct capabilities stay bounded and structured
- repo writes remain specialist-owned

### Risk: Capability Tools Become Connector-Specific Ad Hoc APIs

Mitigation:

- define generic capability interfaces first
- implement connector adapters second

## Success Criteria

The refactor is successful when:

- the shipped default team no longer defines the front agent as a coordinator
- simple local actions complete without child runs
- specialist work still spawns autonomously when needed
- delegation is visible in replay and graphs
- authority boundaries remain intact
- docs, runtime code, team files, and tests all describe the same architecture

## Implementation Readiness

This design is ready for an implementation plan.

The implementation should start by refactoring the team/model/tool policy contract before adding new capability tools, because that contract decides the behavior of the whole runtime.
