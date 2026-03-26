# Vision

GistClaw aims to feel like one assistant at the surface, even when the runtime is coordinating a larger team behind the scenes.

## Product Direction

GistClaw is intended to carry forward the right parts of OpenClaw:

- personal assistant as the product surface
- dynamic background agents
- editable assistant and team behavior
- multiple channels and connectors over time
- providers, tools, and plugins as extension surfaces

It is not intended to carry forward OpenClaw's old architectural sprawl.

## Core Bet

- a simple assistant is a one-front-agent system
- a richer assistant is the same front agent plus spawned worker sessions
- users should be able to define different team shapes without changing the kernel

## Immediate Constraint

The near-term goal is to earn the runtime contract before expanding platform breadth:

- front agent
- spawned worker sessions
- runtime-mediated collaboration
- strict authority boundaries
- local-first replay and approvals

## What Is Still Missing

- a broader channel and gateway matrix on top of the session kernel
- user-created teams and team editing at runtime instead of repo-managed structure only
- richer extension workflows around providers, connectors, tools, and plugins
- a broader control-plane style collaboration model that feels like a full assistant platform, not only a strong run engine

## Current Build Direction

The next implementation work favors:

- one journal-backed session control plane instead of mixed write paths
- session-addressed collaboration and delivery instead of run-addressed shortcuts
- durable route state on the session and thread path instead of connector-specific delivery guesses
- provider input assembled from session-local context instead of the whole conversation log
- a real local host process, where `serve` owns the operator control plane
- operator-facing visibility around sessions, routes, approvals, deliveries, and memory without breaking the runtime boundary
