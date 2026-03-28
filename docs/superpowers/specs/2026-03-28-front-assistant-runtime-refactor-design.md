# Front Assistant Runtime Refactor Design

## Goal

Refactor GistClaw so the user-facing assistant is direct-execution by default, delegation is adaptive instead of default, and the runtime actively recommends when to execute directly, delegate, or parallelize.

The target product behavior is:

- the front assistant answers simple questions directly
- the front assistant executes bounded local actions through first-class capabilities
- the front assistant delegates autonomously only when the work actually benefits from a specialist
- delegation remains visible in runs, replay, and graphs
- the runtime provides decision support so the front assistant does not rely only on prompt-level reasoning when choosing direct execution versus delegation

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
3. Delegation happens only when it adds clear value.
4. The runtime should provide execution recommendations, not only guardrails.
5. Deterministic product actions should be explicit runtime capabilities, not prompt inventions.
6. The front assistant should understand the available specialist ecosystem without micromanaging raw team topology.
7. Kernel boundaries stay strict: runtime owns sessions, delegation, approvals, routing, replay, and authority.

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
- inspect the runtime-recommended execution mode
- choose direct capabilities first when the task is simple and bounded
- ask for clarification when ambiguity blocks safe action
- delegate when the task benefits from specialist expertise, scale, uncertainty reduction, or parallel work
- synthesize specialist outputs back into the user-facing thread

It is not a coordinator in the current sense. It does not decompose by default, but it is allowed to override a recommendation when the runtime permits that override and the assistant has a strong reason.

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
- the runtime recommendation is `direct`

Examples:

- list connector contacts
- resolve a target by name
- send a connector message
- show connector status
- inspect run status
- transfer data between supported systems
- cross-platform messaging or handoff where all required capabilities are already available locally

### Delegation Path

Use this when:

- external research is required
- code changes are required
- explicit review is requested
- explicit verification is requested
- the task is large enough that specialist isolation materially improves correctness
- the runtime recommendation is `delegate`

### Parallel Delegation Path

Use this when:

- two or more independent specialist subtasks exist
- the runtime recommendation is `parallelize`
- the front assistant can describe disjoint specialist objectives without duplicating work

This path should be available, but not overused. Parallelization is valuable only when the subtasks are genuinely independent and the integration cost is lower than the speed gain.

## Runtime Refactor

### Add Execution Recommendation Layer

The runtime should actively recommend how a task should be executed.

Create a planner/evaluator that produces a recommendation for the front assistant before or alongside tool exposure.

Inputs:

- effective capability inventory available right now
- current specialist roster and declared strengths
- task shape signals
- conversation and session context
- authority and approval boundaries
- runtime limits such as active-child depth or active worker count

Outputs:

- `direct`
- `delegate`
- `parallelize`

Each output should include:

- rationale
- optional suggested specialist kind or kinds
- confidence
- optional constraints

Example rationales:

- bounded connector action; no specialist advantage
- external research required; research specialist preferred
- independent research and verification subtasks detected; parallel delegation recommended

The recommendation is not purely binding. The front assistant may override it inside runtime-enforced safety and orchestration rules.

### Replace Free-Form Front Spawning With Governed Delegation

The front assistant should no longer receive unrestricted free-form team-topology spawning as its primary delegation mechanism.

Instead, the model gets governed delegation primitives:

- structured delegation for specialist-owned workflows such as:
  - `delegate_task(kind, objective, context)`
- governed autonomous spawn for bounded background help where a generic subagent is still useful

The runtime should decide which path is appropriate based on the execution recommendation and runtime policy.

Structured delegation remains the preferred path for stable specialist work.

Governed autonomous spawn remains useful for:

- bounded exploratory help
- temporary helper sessions
- future extensibility where strict workflow typing is too limiting

But it must be constrained by runtime rules similar in spirit to OpenClaw:

- allowed targets
- spawn depth
- active child limits
- sandbox/authority inheritance
- workspace inheritance rules

### Structured Delegation

For structured delegation:

- `kind` is constrained to known specialist workflows
- `objective` is the user-level or specialist-level target
- `context` carries structured supporting detail

The runtime, not the model, chooses:

- which agent to run
- whether multiple agents should run in parallel
- whether additional specialist stages are required
- whether a follow-up review or verification run is mandatory

This removes arbitrary team-topology reasoning from normal model behavior.

### Governed Autonomous Spawn

For governed autonomous spawn:

- the front assistant may request a helper run without hardcoding deep team topology
- the runtime validates the request against roster, limits, and policy
- the runtime can reject or reshape the spawn if it is unnecessary or wasteful

This preserves long-term flexibility without keeping the current over-delegation default.

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

The capability layer should support optional adapters by source, similar in spirit to OpenClaw's channel capability surfaces:

- runtime capabilities
- app capabilities
- connector capabilities
- future plugin capabilities

### Add Delegation Orchestration Layer

Create:

- `internal/runtime/delegation/`

Responsibilities:

- map typed delegation requests to specialist workflows
- govern autonomous spawn requests
- create worker runs and sessions
- enforce allowed specialist kinds and roster visibility per agent
- enforce depth, concurrency, and duplication limits
- attach orchestration metadata for replay and graph rendering

This layer owns specialist fan-out. The front assistant requests delegation or helper spawn; the runtime owns the actual orchestration.

## Tool Model Refactor

The current `tool_posture` model is too coarse. It cannot cleanly express:

- front assistant can use bounded app and connector actions
- front assistant cannot mutate repo files directly
- front assistant can request specialist delegation
- worker agents have narrow tool access tied to their role
- dynamic availability and runtime recommendation context

### Layered Tool Policy

Refactor the tool model to support layered policy instead of one static axis.

Policy inputs should include:

- base profile
- allow overlays
- deny overlays
- tool family
- tool source
- approval behavior
- runtime availability

This follows the useful flexibility lesson from OpenClaw: profile plus allow/deny overlays is more adaptable than a single posture enum.

### Replace Tool Posture With Explicit Policy Inputs

Refactor the agent/tool contract to support:

- `base_profile`
- `tool_families`
- `allow_tools`
- `deny_tools`
- `delegation_kinds`
- `approval_profile`
- `specialist_summary_visibility`

Example shape:

```text
front assistant:
  base_profile: operator
  tool_families:
    - repo_read
    - runtime_capability
    - connector_capability
    - web_read
    - delegate
  allow_tools:
    - connector_directory_list
    - connector_target_resolve
    - connector_send
  deny_tools:
    - repo_write
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

### Effective Capability Inventory

The front assistant should not only see a static allowlist. It should see the effective inventory available right now.

That inventory should be computed from:

- configured tools
- active connector capabilities
- runtime mode
- model/provider compatibility
- approval state or temporary unavailability

This is another lesson from OpenClaw: agents behave better when they can see what is actually available right now, not just a theoretical capability set.

The effective inventory should group tools and capabilities by source:

- built-in/runtime
- connector
- plugin or extension
- specialist delegation

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

Tool families should remain flexible enough to grow without rewriting the whole policy model.

## Capability Surfaces

The first-class capability approach should be generic, not Zalo-specific.

### Initial Generic Capabilities

- `connector_directory_list`
- `connector_target_resolve`
- `connector_send`
- `connector_status`
- `app_action`

These are runtime-level capability tools that map to registered connector or app handlers.

### Capability Adapter Model

Do not force every connector or app surface into one monolithic interface.

Use optional adapters, for example:

- directory adapter
- resolver adapter
- send adapter
- status adapter
- auth adapter

That gives the system more flexibility as product surfaces grow, and it mirrors the strongest idea from OpenClaw's channel plugin contract without copying its entire plugin framework.

### Specialist Roster Awareness

The front assistant should receive a concise runtime-generated summary of available specialists, such as:

- agent identity
- primary strengths
- major restrictions
- whether currently available for delegation

This is not the same as exposing raw team topology as the main reasoning surface. It is a roster summary intended to help the front assistant make better delegation decisions.

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

- `base_profile`
- `tool_families`
- `allow_tools`
- `deny_tools`
- `delegation_kinds`
- optional `message_targets` if inter-agent messaging remains necessary
- optional specialist summary metadata

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
- delegation is reserved for cases where it adds clear value
- the user should not be bounced into a child run for bounded local actions
- the runtime recommendation should be considered before choosing execution mode
- the effective capability inventory and specialist roster are authoritative

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
- runtime execution recommendation
- delegation requests
- autonomous helper spawns when used
- spawned specialist runs
- returned outputs
- approvals and blocked states

The UI should still feel like one assistant at the surface, but the operator must be able to inspect delegation without guesswork.

## Data Model And Event Implications

The journal remains the source of truth.

Likely event additions or refactors:

- explicit execution-recommended event
- explicit delegation-requested event
- delegation-resolved or worker-attached event
- autonomous-spawn-requested and autonomous-spawn-resolved events if helper spawns remain distinct
- capability-executed events with normalized structured metadata

The exact event names can be decided during implementation, but the important rule is:

- replay must distinguish direct capability execution from specialist delegation

## Migration Strategy

No compatibility layer.

Migration plan:

1. introduce the execution recommendation, capability, and delegation model in code
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
- runtime recommendation returns direct/delegate/parallelize appropriately for representative tasks
- front assistant creates a delegation request for research/write/review/verify tasks
- front assistant may override a recommendation only inside allowed policy bounds
- runtime maps typed delegation to the correct specialist workflow
- governed autonomous spawn is blocked when depth, duplication, or availability rules say no

### Policy Tests

- effective inventory resolution by profile, allow/deny, and source
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
- a simple cross-platform transfer or messaging task stays direct when all capabilities are available
- a research-heavy task routes to the research specialist
- two independent specialist subtasks can parallelize without duplicated work

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

If the new execution-recommendation and capability/delegation model lands while old `tool_posture` and free-form spawning rules still shape decisions, the runtime will become harder to reason about than it is today.

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

### Risk: Runtime Recommendation Becomes Opaque Or Wrong

If the runtime recommendation behaves like a black box, the front assistant may either follow bad guidance or learn to ignore it.

Mitigation:

- include rationale and confidence
- keep recommendations inspectable in replay
- allow bounded override with explicit rationale

## Success Criteria

The refactor is successful when:

- the shipped default team no longer defines the front agent as a coordinator
- the runtime computes and exposes execution recommendations
- simple local actions complete without child runs
- specialist work still spawns autonomously when needed
- the front assistant can intelligently choose direct execution versus delegation using effective inventory, specialist roster, and runtime recommendation
- delegation is visible in replay and graphs
- authority boundaries remain intact
- docs, runtime code, team files, and tests all describe the same architecture

## Implementation Readiness

This design is ready for an implementation plan.

The implementation should start by refactoring the team/model/tool policy and execution-recommendation contract before adding new capability tools, because that contract decides the behavior of the whole runtime.
