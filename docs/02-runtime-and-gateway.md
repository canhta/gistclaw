# Runtime and Gateway

## What this subsystem is trying to solve

OpenClaw wants one always-on process that owns:

- channel connections
- agent sessions and transcripts
- tool approvals
- control UI access
- webhook ingress
- cron and hooks
- remote clients and nodes

That process is the gateway. The concept is sound. The implementation is far too broad.

## Main call chain

The real entry path is:

`openclaw.mjs` -> `src/entry.ts` -> `src/cli/run-main.ts` -> `src/cli/gateway-cli/run.ts` -> `src/cli/gateway-cli/run-loop.ts` -> `src/gateway/server.impl.ts` `startGatewayServer()`

Important points:

- `openclaw.mjs` is not trivial glue; it handles runtime checks and respawn setup.
- `runGatewayCommand()` validates bind/auth flags and local-mode assumptions before server startup.
- `runGatewayLoop()` adds locks, signal handling, drain behavior, and restart coordination.
- `startGatewayServer()` is the real runtime owner.

## What `startGatewayServer()` actually owns

`src/gateway/server.impl.ts` is not a normal composition root. It is a system kernel.

It directly or indirectly owns:

- config validation and migration
- secret snapshot activation
- gateway auth generation/persistence
- control-ui origin seeding
- plugin loading and runtime registration
- HTTP server startup
- WebSocket server startup
- channel manager startup
- node registry startup
- discovery and health timers
- cron startup
- hook/bootstrap startup
- session event subscribers
- reload/restart handlers

This is the first major design smell. Too much of the product is coupled to a single startup function.

## Transport and control plane

The gateway uses one multiplexed port:

- HTTP and HTTPS server creation in `src/gateway/server-runtime-state.ts`
- upgrade handling in `src/gateway/server-http.ts`
- main protocol handling in `src/gateway/server/ws-connection/message-handler.ts`

That part is better than it sounds. One ingress point is operationally simpler than many sidecars.

The problem is the surface area riding on top of it:

- config
- sessions
- cron
- exec approvals
- tools catalog
- skills
- models
- nodes
- control UI
- chat helpers

This is no longer "assistant runtime API." It is a broad RPC control plane.

## Authentication and pairing model

The gateway security model is more serious than the docs' friendly overview suggests.

Core files:

- `src/gateway/auth.ts`
- `src/infra/device-pairing.ts`
- `src/infra/device-bootstrap.ts`
- `src/gateway/client.ts`

Actual behavior includes:

- challenge-response handshake
- protocol version negotiation
- device identity signing
- device pairing workflow
- per-device token issuance and rotation
- trusted-proxy mode
- tailscale/control-ui-specific trust paths
- remote plaintext refusal unless break-glass is set on the client

This is genuinely strong and worth keeping in principle.

## WebSocket lifecycle

The important lifecycle is:

1. HTTP upgrade accepted
2. server emits `connect.challenge`
3. client sends first-frame `req connect`
4. gateway verifies protocol version and challenge signature
5. auth and pairing decisions run
6. node/client registration happens
7. method calls start flowing

That is a real control-plane handshake, not a weak bearer-token wrapper.

## Config loading and reload

Relevant files:

- `src/gateway/config-reload.ts`
- `src/gateway/config-reload-plan.ts`
- `src/gateway/server-reload-handlers.ts`
- `src/infra/restart.ts`
- `src/infra/process-respawn.ts`

OpenClaw classifies config changes into:

- no-op
- hot reload
- restart

It also delays restart until the system is idle enough, then chooses among:

- in-process restart
- detached respawn
- supervisor-driven restart

That is too much restart machinery for a compact runtime.

## Nodes, roles, caps, and pairing

Node transport is a real subsystem, not a doc fantasy.

Key files:

- `src/gateway/node-registry.ts`
- `src/gateway/server-methods.ts`
- `src/gateway/method-scopes.ts`
- `src/infra/device-pairing.ts`
- legacy `src/infra/node-pairing.ts`

Judgment:

- node connectivity is first-class
- live node state is mostly in-memory
- pairing approval is persisted
- there is still legacy duplication between old node-pairing and newer device-pairing paths

That duplication is pure debt.

## Strengths worth keeping

### 1. Single runtime ownership is the right shape

One process owning conversations, tools, and ingress is simpler than many microservices.

### 2. The handshake is strong

Challenge signing, device pairing, and token rotation are real defensive design.

### 3. Single-port ingress is pragmatic

Local run, reverse proxying, and tailnet exposure are simpler when HTTP and WS share one listener.

### 4. Secret snapshot handling is thoughtful

The runtime preserves last-known-good secret state instead of corrupting a live process on reload failure.

## What is over-engineered

### 1. `startGatewayServer()` is a god object in function form

It should not own migration, auth seeding, plugin enablement, runtime services, and restart policy at once.

### 2. The gateway became a platform host

It is simultaneously:

- daemon
- RPC router
- control UI backend
- plugin host
- node transport
- automation runner
- assistant runtime

That breadth is the real problem.

### 3. Restart behavior is too clever

Respawn, supervisor handoff, in-process restart, and idle deferral are all individually defensible. Together they are hard to reason about.

### 4. Startup mutates durable state

Migration and auto-fix logic in the serve path increase surprise and complicate rollback/debugging.

## Useful abstractions vs fake abstractions

Useful:

- one runtime owner
- explicit protocol schemas
- per-method auth/scopes
- single-port ingress

Fake or too expensive:

- `GatewayRequestContext` as a giant mutable service locator
- one broad RPC plane for nearly every subsystem
- legacy `node.pair.*` alongside device pairing
- mixing client-remote config and server runtime config under one mental model

## Failure handling and hidden coupling

Risk points:

- plugin startup and channel startup happen in the same central boot phase
- reload behavior depends on path-prefix classification
- request-scoped behavior still leaks through globals like fallback gateway-context resolution
- live config reads during handshake make some behavior dynamic and harder to predict

This is why the system is difficult to debug: state is not just large; it is temporally dynamic.

## Docs vs code

Strong match:

- `https://docs.openclaw.ai/concepts/architecture`
- `https://docs.openclaw.ai/gateway/protocol`
- `https://docs.openclaw.ai/gateway/remote`

Material divergence:

- `https://docs.openclaw.ai/cli/gateway` over-simplifies restart behavior
- `https://docs.openclaw.ai/gateway/security` and related pages overstate same-host tailnet local approval
- docs do not clearly describe boot-time config mutation
- docs present a cleaner gateway than the actual service-locator-heavy runtime

## Judgments

- Single-binary feasibility: high
- Runtime simplicity: low
- Control-plane complexity: high
- Config complexity: high
- Operational burden: high
- Security posture: medium-high when configured well
- Debugging difficulty: high

## What the lightweight redesign should do instead

Keep:

- one long-lived process
- one ingress port
- strong authenticated admin/session connection model

Delete:

- generic node/control-plane protocol in v1
- startup config rewriting
- multiple restart strategies
- broad remote invocation surface for every subsystem

Replace with:

- one local admin HTTP API
- optional WS stream only for admin UI event streaming
- explicit restart-on-config-change
- a smaller `GatewayRuntime` with narrow subsystem handles

The right move is not "rebuild the current gateway in Go." The right move is "make a gateway small enough that Go is not hiding a platform inside one binary."
