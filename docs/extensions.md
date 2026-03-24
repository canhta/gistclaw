# Extensions

## Extension Story

Providers, connectors, tools, and plugins are part of GistClaw's long-term platform direction.

That does not mean they are all immediate implementation requirements.

## Required In The Reset

The current rewrite must leave behind clean seams for:

- tools
- providers
- connectors

The kernel should know these seams exist without forcing the rewrite to implement broad extension breadth right now.

## Deferred After The Reset

The following are intentionally deferred until the session-first runtime is stable:

- broad connector expansion
- marketplace or installation UX
- compatibility layers for legacy extension shapes
- large plugin/runtime breadth

## Rule

The extension layer should stay outside the kernel.

The kernel owns:

- sessions
- collaboration primitives
- replay
- approvals
- authority boundaries

Extensions plug into that kernel. They do not redefine it.
