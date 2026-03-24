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
- explicit session send/wake behavior should flow through the runtime so the same session contract can back both local tools and future channel recovery
- external channel recovery should keep reusing the same inbound-message runtime contract rather than teaching each connector its own session-start logic
