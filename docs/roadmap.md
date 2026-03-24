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

- durable front-session reuse, thread binding, mailbox reads, session-scoped provider context, explicit session-message provenance, session-addressed collaboration, session discovery/history reads, and queued outbound delivery are now in place
- Telegram and WhatsApp now have recovered live inbound paths on top of the session kernel, and duplicate external deliveries collapse at the runtime boundary instead of spawning duplicate turns; the broader channel matrix is still not back
- outbound delivery failures now journal back to the owning conversation/run, and the connector delivery path is converging on one retry/terminal-state model instead of per-connector drift
- `gistclaw serve` now hosts the local web control plane instead of only preparing state
- the local web host now exposes read-only session directory and mailbox APIs on top of the session kernel
- the local web host can now send a message into a session and wake it through the runtime instead of only reading mailbox state
- session detail on the local host now exposes the active bound route plus recent outbound deliveries and delivery failures, and terminal deliveries can now be re-queued from that same session control plane
- the local web host now also exposes connector-level delivery queue health, so operators can inspect pending, retrying, and terminal backlog by surface
- the local web host now also exposes a filtered global delivery queue plus top-level retry by delivery id, so operators can move from connector health to recovery without first locating the owning session
- the local web host now also exposes a global route directory, so operators can inspect active external bindings across Telegram, WhatsApp, and future surfaces from one place
- the local web host can now create a specific route binding for an existing session, so operators can rebind external surfaces without manual database work
- the local web host can now deactivate a specific route by binding id, so stale external bindings can be cleared without manual database intervention
- web submit and Telegram ingress now share the same runtime inbound-message path, so follow-up user turns reuse the bound session instead of depending on surface-specific startup logic
- Telegram and WhatsApp ingress now carry source message identity through the runtime, so retries are idempotent and provenance stays explicit in the session mailbox
- WhatsApp webhook ingress and outbound draining now use that same session/runtime contract, giving the platform a second real external surface
- plugins and extension seams are documented, not operational
- teams are still mostly designed ahead of time, not created dynamically by the user

## Next Slice

The next implementation slice should make the session runtime feel more like OpenClaw without reopening platform sprawl:

1. extend the recovered channel path beyond Telegram without rebuilding the full OpenClaw matrix
2. prepare the routing layer for later channel and gateway recovery without reintroducing connector-specific logic into the kernel
3. keep moving team definition from predeclared structure toward user-defined runtime composition
4. extend the session control plane beyond the local host send/wake path into richer operator tools and recovered channel/gateway surfaces

## Locked Review Outcomes

The 2026-03-24 engineering review locked the following implementation decisions for long-term scale:

- one session control-plane facade with journal-backed writes
- session-addressed collaboration as the durable model
- explicit provenance on session messages instead of inference-only debugging
- session-scoped provider context instead of conversation-wide event loading
- an indexed active-run-by-session lookup path for session routing
- durable route state plus runtime-owned external delivery on top of existing `outbound_intents`
