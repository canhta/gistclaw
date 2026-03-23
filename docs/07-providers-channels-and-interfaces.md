# Providers, Channels, and Interfaces

## What this subsystem is trying to solve

OpenClaw wants one runtime to handle:

- many LLM vendors
- many chat channels
- many attachment/media shapes
- many conversation-binding strategies

That is a reasonable goal. The issue is not ambition. The issue is layered genericity.

## Provider abstraction

Core files:

- `src/plugins/types.ts`
- `src/plugins/provider-runtime.ts`
- `src/plugins/providers.runtime.ts`
- `src/agents/provider-capabilities.ts`

Judgment:

- the provider abstraction is real
- it is actively used
- it is still over-split

Providers genuinely own:

- auth behavior
- model catalog behavior
- runtime hook overrides
- capability branching
- vendor-specific transport quirks

That part earns its keep.

## Channel abstraction

Core files:

- `src/channels/plugins/types.plugin.ts`
- `src/plugin-sdk/core.ts`
- `src/channels/plugins/registry.ts`
- `src/channels/plugins/bundled.ts`

Judgment:

- the channel abstraction is also real
- core leaks back across the boundary too often

The worst example is outbound session routing.

Core file:

- `src/infra/outbound/outbound-session.ts`

Even though channel plugins expose their own outbound-session-resolution hooks, core still hardcodes too much per-channel behavior.

## Inbound normalization and outbound delivery

This is one of the cleaner parts of the codebase.

Core files:

- `src/auto-reply/templating.ts`
- `src/auto-reply/reply/inbound-context.ts`
- `src/infra/outbound/message-action-runner.ts`
- `src/infra/outbound/deliver.ts`

The shape is:

- inbound adapter builds message context
- core normalizes it
- routing decides agent/session
- outbound pipeline formats and delivers reply actions

This is worth preserving.

## Attachments and media

Core files:

- `src/media/fetch.ts`
- `src/media/store.ts`
- `src/media/outbound-attachment.ts`
- `src/channels/plugins/media-payload.ts`

Judgment:

- media handling is partly shared, partly bespoke
- that is normal
- OpenClaw is correct not to pretend every connector has identical attachment semantics

The mistake is not branching. The mistake is building a large framework around inevitable branching.

## Account, conversation, and identity binding

Core files:

- `src/routing/resolve-route.ts`
- `src/channels/plugins/configured-binding-registry.ts`
- `src/infra/outbound/session-binding-service.ts`
- `src/plugins/conversation-binding.ts`
- `src/routing/session-key.ts`

There are effectively three binding systems:

- inbound route bindings
- configured ACP/channel bindings
- runtime conversation bindings for outbound continuity

That is more binding machinery than the core concept needs.

Identity mapping itself is narrow. `session.identityLinks` mostly helps canonicalize DM peers, not build a real cross-channel identity graph.

## What is genuinely strong

### 1. Provider adapters own real provider quirks

This is good. LLM vendors really are different.

### 2. Inbound normalization is useful

A normalized message/event envelope is the right center.

### 3. Single-binary modularity already works

Bundled channels prove OpenClaw does not need separate deploy units to stay modular.

## What is over-engineered

### 1. Too many registries

Provider registry, channel registry, outbound adapter loading, configured bindings, runtime bindings, media provider registry: the layering is heavy.

### 2. Core violates channel ownership

If core still owns per-channel routing switchboards, the abstraction boundary is not doing enough.

### 3. Provider capability ownership is spread too widely

Parallel registries for text, speech, media understanding, image generation, and search create more surfaces than necessary.

## Useful abstractions vs fake abstractions

Useful:

- normalized inbound envelope
- delivery-result abstraction
- provider-specific runtime hooks

Fake or too expensive:

- one giant generic channel framework
- one universal provider abstraction that erases vendor differences
- multiple binding frameworks as first-class product concepts

## Docs vs code

Material mismatch:

- `https://docs.openclaw.ai/channels` labels some channels as separately installed plugins even though `src/channels/plugins/bundled.ts` statically bundles them
- `https://docs.openclaw.ai/plugins/sdk-channel-plugins` gives plugins more apparent ownership of outbound behavior than core actually relinquishes
- `https://docs.openclaw.ai/nodes/media-understanding` describes vendor registration as plugin-owned, but core seeds some vendors itself

Strong match:

- `https://docs.openclaw.ai/web/webchat` correctly describes WebChat as an internal non-deliverable surface

## Judgments

- Provider abstraction quality: medium
- Channel abstraction quality: medium-low
- Dependency weight: high
- Single-binary feasibility: already proven

## What should be simplified

- one normalized envelope for inbound events
- one narrow delivery interface
- one provider interface with capability flags
- two visible binding concepts only: inbound route and outbound continuation

## What should be deleted

- core-owned per-channel routing switchboards
- parallel registry sprawl for every capability family
- pretending the framework removes the need for channel/provider-specific branching

## What the lightweight redesign should do instead

Keep:

- normalized message envelope
- explicit delivery receipts
- provider capability flags

Replace with:

- small compiled adapters for supported connectors
- small provider packages with vendor-specific code where needed
- fixed agent- and phase-based model lanes instead of a dynamic model auto-router
- optional out-of-process extensions later, not a huge in-process plugin framework in v1

Stable-release outbound delivery posture:

- every outbound message starts as a durable outbound intent recorded before delivery
- delivery is at-least-once with connector-specific retry handling and dedupe keys where the connector can support them
- replay and receipts should show intent created, delivery attempted, delivery confirmed, and final failure when relevant
- do not promise exactly-once delivery for chat connectors in the stable release

Recommended stable-release connector posture:

- Telegram only
- support direct messages only in the stable release
- keep Telegram narrow: one ongoing DM surface, no group routing model in the first stable cut

Recommended post-preview onboarding step:

- after the first successful local preview, the main CTA is to connect the same team to Telegram
- this is the bridge from local proof to ongoing real-world use
- the recommended first Telegram surface is a private DM

Recommended stable-release Telegram DM surface:

- one operator-facing agent speaks in the DM
- that agent delegates to the team behind the scenes
- the team remains visible in replay and receipts, not as multiple competing voices in chat
- the DM stays quiet between meaningful state changes
- the operator-facing agent sends rare milestone checkpoints only: started, blocked, approval needed, and finished
- step-by-step internal narration does not stream into the DM by default
- once Telegram is connected, the DM behaves like a normal chat first
- the user should be able to type a task immediately without going through a command menu or quick-action gate
- low and medium-risk approvals may be resolved inline in the DM with a compact preview and replay link
- high-risk approvals stay in the local web UI or CLI
- milestone updates and final replies use durable outbound intents so crash recovery and retry behavior stay inspectable

Later-phase connector expansion:

- bounded groups can be added after the DM loop is stable
- sharing and publish-back flows should follow only after the local replay loop is solid

Recommended stable-release provider posture:

- cheap default lane for routine agent work
- stronger lane for escalation, synthesis, verification, and other high-signal phases
- explicit model choice visible in receipts and replay

Why this wins:

- lower idle and per-run burn
- easier debugging than confidence-based live routing
- clearer operator trust because expensive hops are visible and intentional
