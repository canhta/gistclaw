# Automatic Presence Runtime Design

## Goal

Add production-ready automatic typing and presence behavior to GistClaw so the front assistant can signal progress in remote conversations without requiring the model to explicitly call a tool.

The target product behavior is:

- typing is emitted automatically for long-running user-facing runs
- simple and fast runs do not flash noisy typing indicators
- presence is runtime-managed, not prompt-managed
- connector implementations stay behind declared capability seams
- the design is reusable for Zalo first, then other connectors later

This is a clean refactor and extension of the current capability model. No backward-compatibility layer is needed.

## Problem

GistClaw now has direct capability tools for inbox, directory, resolve, send, and status, but it still lacks a system-owned presence lifecycle.

Today:

- long-running direct tasks can leave the user with no visible progress signal
- presence behavior would have to be hardcoded in connectors or improvised by the assistant
- runtime behavior would drift between connectors
- typing would be fragile if the model had to remember to emit it explicitly

That is the wrong boundary. Presence is not user intent. It is runtime behavior.

## What We Learned From OpenClaw, GoClaw, And ZCA-JS

Useful patterns:

- `openclaw` treats typing as best-effort runtime behavior with keepalive, TTL safety, and early start signals.
- `goclaw` uses a channel-agnostic typing controller with keepalive, TTL, and explicit cleanup boundaries.
- `zca-js` confirms Zalo typing is a dedicated protocol surface and can be emitted independently of message send.

These point to one design:

- runtime owns the typing lifecycle
- connectors only implement emission adapters
- the assistant should not carry the burden of remembering when to start or stop typing

## Design Principles

1. Runtime-managed, not assistant-managed.
2. Best-effort only. Presence failures must never fail the run.
3. DRY lifecycle logic. Keepalive, TTL, and breaker logic live once in the runtime.
4. SOLID connector seams. Runtime depends on a small adapter interface, not connector-specific code.
5. User-facing only. Background and internal runs should not emit typing.
6. No connector ID checks in runtime core.

## Scope

### In Scope

- generic presence capability seam
- runtime presence controller and manager
- automatic typing lifecycle for user-facing runs
- Zalo Personal as the first connector adapter
- default timing policy and connector overrides
- planner and run integration so the feature is automatic
- tests for lifecycle, stop conditions, and connector adaptation

### Out Of Scope

- assistant-exposed typing tools
- presence for local web or internal conversations
- read receipts or inbox mutation behavior
- full presence dashboards
- rich status text or multi-state presence beyond typing

## Target Architecture

```text
user message
  -> front run starts
  -> runtime decides whether the run is user-facing and presence-capable
  -> presence manager creates or reuses a controller for the route
  -> startup delay elapses
  -> controller emits typing through connector adapter
  -> keepalive continues while the user is still waiting
  -> controller stops on output, pause, cancel, error, finish, or TTL
```

## Capability And Package Design

### Runtime Capability Seam

Add a generic presence seam under `internal/runtime/capabilities/registry.go`.

New types:

- `PresenceMode`
- `PresenceEmitRequest`
- `PresencePolicy`
- `PresenceAdapter`

Recommended request shape:

- `ConnectorID`
- `ConversationID`
- `ThreadID`
- `ThreadType`
- `Mode`

Recommended policy shape:

- `StartupDelay`
- `KeepaliveInterval`
- `MaxDuration`
- `MaxConsecutiveFailures`
- `SupportsStop`

The registry stays generic. It only knows how to locate a registered connector adapter and call it.

### Runtime Presence Package

Add a dedicated runtime package:

- `internal/runtime/presence/controller.go`
- `internal/runtime/presence/manager.go`

Responsibilities:

- `controller.go`
  - startup delay
  - keepalive loop
  - TTL safety
  - consecutive-failure breaker
  - idempotent stop behavior
- `manager.go`
  - map user-facing routes to active controllers
  - start and stop controllers from run lifecycle events
  - ensure only one active controller exists per live route

This keeps lifecycle logic out of connector implementations and out of the assistant.

### Connector Adapter Contract

Connectors that support typing implement the presence adapter.

Recommended methods:

- `CapabilityPresencePolicy(context.Context) capabilities.PresencePolicy`
- `CapabilityEmitPresence(context.Context, capabilities.PresenceEmitRequest) error`
- optional `CapabilityStopPresence(context.Context, capabilities.PresenceEmitRequest) error`

The runtime depends only on these methods. It does not know or care whether the connector is Zalo, Telegram, WhatsApp, or something else.

## Lifecycle Rules

### When Presence Starts

Presence should start only when all of the following are true:

- the run is the user-facing front run
- the run source is a remote connector conversation
- the source connector has a registered presence adapter
- the run is still active after `StartupDelay`
- the run has not already produced user-facing output

This avoids typing noise on fast requests and prevents child or background runs from emitting user-visible typing.

### While A Run Is Active

After the delay:

- emit an initial typing signal
- keep emitting on the configured interval
- stop if the failure breaker trips

If the front run delegates internally but the user is still waiting in the same thread, the front run keeps ownership of presence. Delegation must not silently drop typing.

### When Presence Stops

Presence stops when:

- the first user-facing assistant output is sent
- the run enters approval pause
- the run is cancelled
- the run errors
- the run finishes
- the TTL is exceeded

If the connector has an explicit stop capability, call it once. Otherwise stop the keepalive and let the external platform clear the indicator naturally.

## Zalo First Adapter

Zalo Personal is the first connector implementation.

Implementation shape:

- add a Zalo presence adapter in the connector package
- keep the protocol-specific call in a dedicated protocol file
- map runtime `ThreadType` to Zalo user/group typing endpoints

The wire truth comes from `zca-js`:

- user typing uses the chat typing endpoint
- group typing uses the group typing endpoint
- payload includes thread ID plus `imei`

That protocol detail belongs only in the Zalo connector and protocol packages.

## Defaults

Recommended defaults:

- `StartupDelay`: `800ms`
- `KeepaliveInterval`: `8s`
- `MaxDuration`: `60s`
- `MaxConsecutiveFailures`: `2`

Connectors may override timing where the platform requires shorter expiry windows.

## Error Handling

Presence is best-effort.

Rules:

- emission errors are logged at debug or warn level
- presence errors never fail the run
- consecutive failures trip the controller breaker
- a tripped controller stops future emits for that route until a new run starts

This protects the main product flow from noisy or unstable presence endpoints.

## Planner And Runtime Integration

This feature is not a normal assistant tool.

The front assistant should not explicitly choose whether to emit typing. Instead:

- the runtime inspects route and connector capabilities at run start
- the runtime starts presence automatically when the lifecycle conditions are met
- the runtime stops presence automatically from run state transitions

This keeps the one-assistant experience clean and avoids coupling model behavior to UI polish behavior.

## Testing Strategy

### Core Runtime Tests

1. local and internal runs do not create controllers
2. short runs completing before startup delay do not emit typing
3. long runs emit initial typing after delay
4. keepalive fires at the configured interval
5. first assistant output stops the controller
6. approval pause stops the controller
7. cancel, error, and finish stop the controller
8. TTL forces cleanup
9. consecutive presence failures trip the breaker without failing the run
10. delegated work behind a waiting front run does not drop presence prematurely

### Connector Tests

1. Zalo user thread emits to the user typing endpoint with the expected payload
2. Zalo group thread emits to the group typing endpoint with the expected payload
3. unsupported thread types fail inside the connector adapter, not in runtime core

### Integration Tests

1. a direct connector task that takes longer than the delay emits typing automatically
2. a fast task does not emit typing
3. a run paused for approval stops presence before the approval prompt is shown

## Future Capability Family

This design should grow into a broader conversation-state family, but those capabilities remain separate seams:

- inbox list
- inbox update
- presence emit
- read receipt or seen state
- retention or auto-delete policy
- destructive conversation actions with stricter approval

Presence is the next correct slice because it improves the user experience immediately without distorting the existing inbox or send boundaries.

## Success Criteria

The design is successful when:

- the user sees automatic typing on slow remote runs
- quick runs stay quiet
- the assistant never has to remember to call a typing tool
- runtime core contains no connector ID checks for presence
- Zalo works through the same generic seam other connectors can implement later
