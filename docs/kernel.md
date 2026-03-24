# Kernel

## Runtime Invariants

These are the rules the rewrite is building around.

### Front Agent

Each conversation has one front agent session that owns the user relationship and the primary outward voice.

### Worker Sessions

The front agent may spawn worker sessions. Worker sessions are real runtime actors with their own identity, state, and replay visibility.

### Collaboration Primitives

The kernel centers on four runtime collaboration primitives:

- `spawn`
- `announce`
- `steer`
- `agent-send`

These are runtime events and message types, not loose metaphors.

### Authority Boundaries

Free collaboration does not imply free authority.

The runtime keeps strict control over:

- workspace mutation
- outbound delivery
- approvals
- memory scope escalation
- privileged tool execution

### Persistence

The append-only event journal remains the source of truth. Replay, receipts, approvals, and session views should all be derived from that journal plus projections.

### Routing

Routing is deterministic and runtime-owned. Model output does not choose delivery paths or follow-up bindings.

## What The Kernel Does Not Assume

The kernel does not assume:

- repo-task identity as the entire product
- rigid handoff-edge graphs as the main collaboration model
- broad connector or plugin breadth in the first rewrite pass
