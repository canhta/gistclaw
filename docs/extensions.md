# Extensions

## Extension Story

Providers, connectors, tools, and plugins are part of GistClaw's long-term platform direction.

That does not mean they all have to ship at once.

## Shipped Seams Today

The current tree already has concrete seams for:

- tools in `internal/tools/`
- providers in `internal/providers/`
- connectors in `internal/connectors/`

The shipped surface today includes built-in web fetch, optional Tavily search, optional MCP stdio tools, provider adapters for Anthropic and OpenAI-compatible APIs, and live Telegram and WhatsApp connector wiring.

## Still Deferred

The following are still intentionally deferred until the session-first runtime is more mature:

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
