# GistClaw Assistant-Platform Reset Design

Date: 2026-03-24
Status: Approved during brainstorming

## Purpose

Reset GistClaw around what we want to build next, not around the current mixed doc set.

The new direction is:

- assistant-first at the product layer
- OpenClaw-like in breadth and ambition
- team-runtime-based underneath
- free to replace current runtime semantics and package boundaries
- unconcerned with backward compatibility or migration shims

This design does not preserve the old docs as a source of truth. The old docs are treated as disposable. The next implementation phase should delete them and replace them with a new minimal doc set that matches the new contract.

## Problem

The repository currently carries multiple conflicting truths:

- an early narrow kernel story
- a later milestone-complete local repo-task product story
- a broader OpenClaw-like assistant-platform ambition

Those are not just different milestones. They imply different product identities and different runtime contracts.

The reset is meant to remove that ambiguity and define one clear product and architecture direction for the next refactor.

## Product Thesis

GistClaw should be a rebuilt OpenClaw-like assistant platform.

The top-level product story is personal assistant first. The system should feel like one assistant the user talks to. That assistant is the product surface.

Underneath that surface, the true runtime model is multi-agent. The personal assistant is not a special case outside the architecture. It is the front agent of a team runtime and may spawn additional agents behind the scenes when needed.

That gives GistClaw one stable identity:

- the user starts with one assistant
- that assistant may dynamically create and coordinate more agents
- users may later define different team shapes and assistant structures
- one-agent and many-agent cases use the same kernel

## Core Runtime Contract

### Front Agent

Each user-facing assistant has one primary front agent.

The front agent:

- owns the user relationship
- owns the main conversational identity
- may spawn additional agents during a run
- remains the default outward voice unless another agent is explicitly granted outbound authority

### Spawned Agents

Spawned agents are real runtime actors, not hidden prompt fragments.

Each spawned agent should have explicit runtime identity, explicit run/session records, and explicit replay visibility. They may do work in the background, report back, receive steering, and coordinate with other allowed agents.

### Collaboration Model

The runtime should move away from rigid declared handoff edges as the primary collaboration mechanism.

Instead, it should support:

- front-agent spawning of background agents
- runtime-mediated agent-to-agent communication
- announce, steer, and send-style collaboration flows
- explicit identity and replay for every participant

The communication model should feel closer to OpenClaw's session-based subagent model than to a fixed delegation graph.

### Authority Boundaries

Free collaboration does not mean free authority.

The runtime must keep strict control over:

- workspace mutation
- outbound delivery
- approvals
- memory scope escalation
- privileged tool execution

Agents may communicate broadly, but side effects remain capability-gated and runtime-mediated.

## OpenClaw Carry-Forward

The reset intentionally carries forward the right parts of OpenClaw:

- assistant-first product positioning
- broad long-term platform ambition
- dynamic background agents
- strong channel and routing identity
- editable operator-authored behavior
- plugin, connector, and provider expansion as part of the product story

The reset does not carry forward OpenClaw's most expensive mistakes:

- uncontrolled architectural sprawl
- overlapping truths across docs
- hidden authority through vague collaboration language
- preserving breadth without a clean kernel contract

The target is "more like OpenClaw" in product feel and extensibility, while being stricter and cleaner in runtime ownership.

## What Stays

The following current ideas still fit the new core and should be treated as reusable:

- append-only event journal
- replayable state and receipts
- explicit approvals and auditability
- local-first operator control surfaces
- editable assistant and team behavior

These are still aligned with the new direction even if their current implementation gets replaced.

## What Must Change

The following current ideas no longer define the system correctly:

- repo-task runtime as product identity
- rigid handoff-edge-based delegation as the main collaboration model
- current package boundaries if they block the new runtime model
- existing docs as architecture truth
- any legacy compatibility requirement that keeps old semantics alive

Repo-task work should remain important, but only as a starter workflow on the platform, not as the platform definition.

## Next Build/Refactor Sequence

### 1. Replace the collaboration model

Refactor the runtime from a rigid parent-child delegation graph into a front-agent-plus-spawned-agents model with runtime-mediated coordination.

Primary changes:

- main/front session identity
- spawned worker sessions
- announce and steer style coordination
- explicit inter-agent communication path

### 2. Make session identity first-class

GistClaw should borrow more directly from OpenClaw's session model.

The next runtime should treat session identity and routing as first-class primitives for:

- assistant main sessions
- spawned sessions
- thread or follow-up binding where supported
- deterministic routing across surfaces

### 3. Recast repo work as a starter workflow

Preview, apply, verify, and approval flows remain valuable, but they become one powerful workflow on top of the assistant platform rather than the definition of the platform itself.

### 4. Define a real extension surface

Tools, providers, connectors, and plugins should be defined as a deliberate extension layer.

The next design should not keep adding concrete packages for these surfaces without a clean contract.

### 5. Keep authority strict while making collaboration looser

Collaboration expands.
Authority remains narrow.

This is the main guardrail that keeps the new OpenClaw-like direction from drifting back into chaos.

## Documentation Contract Going Forward

The replacement docs should separate three truths and never blur them:

- vision truth: what GistClaw is becoming
- kernel truth: runtime invariants that must remain true across milestones
- shipped truth: what actually exists now

The next doc set should be minimal and focused on the reset, not on preserving historical material.

Recommended replacement set:

- `README.md`: product overview and current status
- `docs/vision.md`: assistant-first platform direction
- `docs/kernel.md`: runtime invariants and authority model
- `docs/roadmap.md`: what we build next
- `docs/extensions.md`: providers, connectors, plugins, and what is deferred

All old architecture docs should be deletable once the new set exists.

## Rewrite Posture

The next phase should be executed as a replacement refactor.

Rules:

- no backward compatibility shims
- no legacy support for old runtime semantics
- no preserving package boundaries because they already exist
- no preserving old event shapes if they no longer fit the model
- no old-doc migration effort

If a package, event, route, or concept is wrong for the new design, it should be replaced rather than adapted.

## Non-Goals For The Immediate Next Phase

Even with the broader platform ambition, the next phase should not try to reintroduce all of OpenClaw at once.

Avoid:

- rebuilding full OpenClaw control-plane complexity immediately
- restoring every channel before the runtime model is right
- describing free agent communication as vague hive-mind behavior
- keeping legacy semantics for convenience

The next step is to earn the new runtime contract first.

## Success Criteria

This reset is successful when:

- GistClaw has one clear product identity
- the assistant-first story and the multi-agent runtime no longer conflict
- session/spawn collaboration replaces rigid delegation as the primary model
- repo-task work is clearly framed as a workflow, not the entire product
- the extension story is explicit
- the new docs talk only about what we are building next

## Immediate Planning Consequence

The next implementation plan should focus on:

- deleting the old doc set
- writing the new minimal doc set
- redesigning runtime/session/collaboration around front agent plus spawned agents
- identifying which current packages survive, which are replaced, and which are deferred
