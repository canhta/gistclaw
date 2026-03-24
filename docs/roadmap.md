# Roadmap

## Immediate Work

The reset currently targets four concrete outcomes:

1. Replace the stale doc set with a small source-of-truth doc set.
2. Rewrite runtime collaboration around front sessions and spawned worker sessions.
3. Replace rigid delegation-era team and replay semantics.
4. Keep approvals, auditability, and local-first control surfaces intact during the rewrite.

## Immediate Non-Goals

These are explicitly out of scope for the current rewrite:

- rebuilding the full OpenClaw channel matrix
- restoring full gateway/control-plane complexity
- broad plugin marketplace work
- large automation expansion
- polishing every workflow before the kernel is stable

## Expected Sequence

1. Doc reset
2. Schema and domain model reset
3. Session package and team spec rewrite
4. Runtime collaboration rewrite
5. Replay, tools, and web surface alignment
6. Removal of deferred breadth from the active build

## Remaining Gap To OpenClaw-Like Behavior

The reset kernel is in place, but the product still does not fully behave like OpenClaw yet.

Remaining gaps:

- front-agent identity is still too run-shaped instead of conversation-shaped
- worker collaboration exists, but the runtime still lacks a fuller mailbox and routing model
- channels and connectors are no longer core to the active build
- plugins and extension seams are documented, not operational
- teams are still mostly designed ahead of time, not created dynamically by the user

## Next Slice

The next implementation slice should make the assistant more OpenClaw-like without reopening platform sprawl:

1. make the front agent durable across multiple runs in the same conversation
2. route worker messages through that durable assistant session instead of treating each run as its own front identity
3. prepare the runtime for richer mailbox, routing, and channel work after the assistant session model is stable
