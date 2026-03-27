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

- host and project mutation
- outbound delivery
- approvals
- memory scope escalation
- privileged tool execution

### Persistence

The append-only event journal remains the source of truth. Replay, receipts, approvals, and session views should all be derived from that journal plus projections.

### Routing

Routing is deterministic and runtime-owned. Model output does not choose delivery paths or follow-up bindings.

Session routing should be carried by durable session/thread route state, not connector-specific side logic.

The kernel should therefore own:

- which session is bound to a given thread
- which connector/account/external target that bound thread can deliver back to
- how assistant replies become outbound intents
- how inter-session messages retain sender and route provenance

### Context Assembly

Provider input should be assembled from the session's own mailbox, routed inter-session messages, and memory summaries.

Conversation-wide event history is useful for replay and audit, but it is not the long-term context boundary for an assistant session.

## What The Kernel Does Not Assume

The kernel does not assume:

- repo-task identity as the entire product
- rigid handoff-edge graphs as the main collaboration model
- broad connector or plugin breadth in the first rewrite pass
