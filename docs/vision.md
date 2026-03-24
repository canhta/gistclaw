# Vision

## Product Direction

GistClaw is an assistant-first platform.

The user should experience one assistant. That assistant may stay simple, or it may spin up a larger working team behind the scenes. The product surface stays personal; the runtime underneath stays multi-agent.

## Long-Term Shape

GistClaw is intended to carry forward the right parts of OpenClaw:

- personal assistant as the product surface
- dynamic background agents
- editable assistant and team behavior
- multiple channels and connectors over time
- providers, tools, and plugins as extension surfaces

It is not intended to carry forward OpenClaw's old architectural sprawl.

## Core Bet

The core bet is that one-agent and many-agent behavior should come from the same runtime model.

- a simple assistant is a one-front-agent system
- a richer assistant is the same front agent plus spawned worker sessions
- users should be able to define different team shapes without changing the kernel

## Immediate Constraint

The immediate rewrite does not try to rebuild all of OpenClaw.

The near-term goal is to earn the new runtime contract first:

- front agent
- spawned worker sessions
- runtime-mediated collaboration
- strict authority boundaries
- local-first replay and approvals

## What Is Still Missing

The current reset is closer to OpenClaw in kernel direction than in product breadth.

What is still not earned yet:

- broader runtime-owned routing across surfaces instead of mostly local queued delivery
- only Telegram has a recovered live connector path so far; the wider channel/gateway set is still missing
- WhatsApp now joins Telegram as an earned live surface, but the wider channel/gateway set is still missing
- dynamic user-created teams at runtime instead of config-defined team shape only
- restored channel/gateway surfaces on top of the new kernel
- real extension contracts for providers, connectors, tools, and plugins
- a broader control-plane style collaboration model that feels like OpenClaw instead of just a better run engine

## Current Build Direction

The immediate build direction after the reset is to make the session kernel operationally trustworthy, not just structurally suggestive.

That means the next implementation work favors:

- one journal-backed session control plane instead of mixed write paths
- session-addressed collaboration and delivery instead of run-addressed shortcuts
- durable route state on the session/thread path instead of connector-specific delivery guesses
- provider input assembled from session-local context instead of the whole conversation log
- a real host process, where `serve` owns the local web control plane instead of only background preparation
- operator-facing session visibility starts with the local session directory and mailbox APIs, and can grow into richer control-plane tools later
- operator-facing session visibility now also includes session-scoped delivery state on the local host, so route attachment and delivery health can be inspected together
- the local host now also exposes connector-level delivery queue health, so operators can see backlog and terminal-pressure by surface without drilling into individual sessions first
- the local host now also exposes a global delivery queue and top-level retry path, so connector health can flow into concrete operator action without session hunting
- the local host now also exposes a global route directory, so operators can inspect how external surfaces are currently bound into the session kernel
- the local host can now create a new route binding for an existing session, so operators can recover or rebind external surfaces through the journal-backed control plane
- the local host can now send directly to a route by binding id, so route discovery can flow straight into session wake-up without manual ID translation
- the local host can now deactivate a bound route explicitly, so stale or misrouted external bindings can be cleared without direct database work
- the route directory can now also expose inactive history with deactivation timestamps, so route replacement and cleanup leave an inspectable trail instead of disappearing
- route history now also records whether a binding was explicitly deactivated or replaced by a newer route, so the control plane can explain why a route went inactive
- the local host can now request a controlled retry for terminal deliveries through the same session-scoped control plane, instead of requiring direct database intervention
- session-scoped delivery failures on the local host are now filtered down to actionable failures, so a successful redrive clears the active failure view without erasing the underlying run audit trail
- the local host now authenticates server-rendered operator writes through an HttpOnly host session with same-origin checks, so HTML control-plane forms no longer depend on manual bearer-header injection
- the local host now has a server-rendered control page for route bindings and delivery pressure, with shared query filters that let operators narrow the same route and delivery directories exposed over JSON
- the local host now also has server-rendered session directory and session detail pages, and the session directory now speaks the same queryable filter model as the control-plane APIs so sessions can remain the primary operator object as the runtime grows
- explicit session send/wake behavior should flow through the runtime so the same session contract can back both local tools and future channel recovery
- external channel recovery should keep reusing the same inbound-message runtime contract rather than teaching each connector its own session-start logic
- external retries and redeliveries should be absorbed by one runtime-owned inbound receipt model, so connectors stay thin and duplicate delivery does not fork extra runs
- outbound delivery should also converge on one audit shape, where terminal failures attach to the owning conversation/run instead of disappearing into connector-local logs
