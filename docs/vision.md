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

The current implementation is closer to OpenClaw in kernel direction than in product breadth.

What is still not earned yet:

- a broader channel and gateway matrix on top of the session kernel
- user-created teams and team editing at runtime instead of repo-managed structure only
- richer extension workflows around providers, connectors, tools, and plugins
- a broader control-plane style collaboration model that feels like a full assistant platform, not only a strong run engine

## Current Build Direction

The current build direction is to make the session kernel operationally trustworthy and operator-friendly.

That means the next implementation work favors:

- one journal-backed session control plane instead of mixed write paths
- session-addressed collaboration and delivery instead of run-addressed shortcuts
- durable route state on the session and thread path instead of connector-specific delivery guesses
- provider input assembled from session-local context instead of the whole conversation log
- a real local host process, where `serve` owns the operator control plane
- operator-facing visibility around sessions, routes, approvals, deliveries, and memory without breaking the runtime boundary
