# OpenClaw Docs vs Code Map

## Method

I treated the official docs as the intended architecture and the repository as the source of truth.

Docs checked included:

- `https://docs.openclaw.ai/concepts/architecture`
- `https://docs.openclaw.ai/gateway/protocol`
- `https://docs.openclaw.ai/gateway/security`
- `https://docs.openclaw.ai/concepts/agent-workspace`
- `https://docs.openclaw.ai/concepts/memory`
- `https://docs.openclaw.ai/tools/subagents`
- `https://docs.openclaw.ai/reference/session-management-compaction`
- `https://docs.openclaw.ai/plugins/sdk-channel-plugins`
- `https://docs.openclaw.ai/plugins/sdk-provider-plugins`
- `https://docs.openclaw.ai/channels`
- `https://docs.openclaw.ai/gateway/sandboxing`

Key code anchors included:

- `src/gateway/server.impl.ts`
- `src/gateway/server-http.ts`
- `src/gateway/server/ws-connection/message-handler.ts`
- `src/routing/resolve-route.ts`
- `src/routing/session-key.ts`
- `src/config/sessions/store.ts`
- `src/config/sessions/transcript.ts`
- `src/agents/workspace.ts`
- `src/agents/bootstrap-cache.ts`
- `src/agents/subagent-spawn.ts`
- `src/agents/tool-policy-pipeline.ts`
- `src/infra/outbound/outbound-session.ts`

## High-confidence map

| Topic | Docs intent | Code reality | Judgment |
| --- | --- | --- | --- |
| Single long-lived gateway | One gateway owns state, channels, clients, and nodes | `src/gateway/server.impl.ts` does exactly that | Match, but the runtime is far larger than the docs suggest |
| WebSocket control plane | JSON protocol for clients and nodes | `src/gateway/protocol/index.ts`, `src/gateway/server-methods.ts`, `src/gateway/node-registry.ts` implement it | Match |
| Gateway-host state ownership | Sessions, auth, and runtime state live on the gateway host | Per-agent paths under `~/.openclaw/agents/<agentId>` confirm that | Match |
| Multi-agent namespacing | Each agent gets its own workspace, sessions, and state | `src/agents/agent-scope.ts`, `src/config/sessions/paths.ts`, `src/agents/auth-profiles/*` make that real | Match in structure |
| Workspace bootstrap files | Agent behavior is shaped by workspace files | `src/agents/workspace.ts`, `src/agents/bootstrap-files.ts` seed and load them | Match |
| Session storage and compaction | Sessions persist and compact | `src/config/sessions/store.ts`, `src/config/sessions/transcript.ts`, `src/agents/pi-embedded-runner/compact.ts` do that | Match, but with more moving parts |
| Tools, sandbox, approvals | Access control precedes tool execution | `src/agents/tool-policy-pipeline.ts`, `src/agents/sandbox/*`, `src/infra/exec-approvals.ts` enforce that | Match in principle |
| Hooks, cron, webhook, poll | Automation is first-class | `src/cron/*`, `src/hooks/*`, `src/gateway/hooks.ts` confirm it | Match |
| Provider and channel plugins | OpenClaw supports many providers/channels via plugin seams | `src/plugins/types.ts`, `src/channels/plugins/*` prove it | Match |

## Hard mismatches

### 1. Subagents inherit more than the docs say

Docs:

- `https://docs.openclaw.ai/tools/subagents`
- `https://docs.openclaw.ai/start/openclaw`

Docs claim subagents get only `AGENTS.md` and `TOOLS.md`.

Code:

- `src/agents/workspace.ts` `MINIMAL_BOOTSTRAP_ALLOWLIST`
- `src/agents/workspace.ts` `filterBootstrapFilesForSession()`

Actual inheritance also includes:

- `SOUL.md`
- `IDENTITY.md`
- `USER.md`

Why it matters:

- prompt size is larger than advertised
- subagents inherit behavior and identity state, not just task rules
- isolation is weaker than the docs imply

### 2. `MEMORY.md` is not private-only in code

Docs:

- `https://docs.openclaw.ai/concepts/memory`
- `docs/reference/templates/AGENTS.md`

Docs and templates describe `MEMORY.md` as private/main-session memory.

Code:

- `src/agents/workspace.ts` `filterBootstrapFilesForSession()`

Current behavior injects memory for every non-subagent, non-cron session, including normal group and channel sessions.

Why it matters:

- this is a trust-boundary mismatch, not wording drift
- shared-channel sessions can see curated memory the docs imply should stay private

### 3. Transcripts are not truly append-only

Docs:

- `https://docs.openclaw.ai/reference/session-management-compaction`

Docs present transcripts as append-only with compaction summaries layered on top.

Code:

- `src/agents/pi-embedded-runner/tool-result-truncation.ts`
- `src/agents/pi-embedded-runner/compact.ts`
- `src/agents/pi-embedded-runner/session-truncation.ts`
- `src/config/sessions/store.ts`

The runtime repairs, truncates, sanitizes, and rewrites transcript-adjacent state.

Why it matters:

- operator expectations about auditability are overstated
- the persistence model is more mutable than the docs suggest

### 4. Subagents can spawn sessions despite the docs saying otherwise

Docs:

- `https://docs.openclaw.ai/concepts/session-tool`

Docs say subagents are not allowed to call `sessions_spawn`.

Code:

- `src/agents/subagent-spawn.ts`
- `src/agents/pi-tools.policy.ts`
- `src/agents/pi-tools.policy.test.ts`

The code explicitly supports subagent spawning subject to policy.

Why it matters:

- recursion and loop risk are materially different from the public description

### 5. Bootstrapping seeds more files than some docs list

Docs:

- `https://docs.openclaw.ai/start/bootstrapping`

Docs emphasize `AGENTS.md`, `BOOTSTRAP.md`, `IDENTITY.md`, and `USER.md`.

Code:

- `src/agents/workspace.ts` `ensureAgentWorkspace()`

Seeding also includes:

- `SOUL.md`
- `TOOLS.md`
- `HEARTBEAT.md`

Why it matters:

- the workspace contract is wider than the first-run docs imply

### 6. `BOOT.md` is not part of normal bootstrap loading

Docs:

- `https://docs.openclaw.ai/concepts/agent-workspace`

Docs make `BOOT.md` look like part of the workspace map.

Code:

- `src/gateway/boot.ts`
- `src/hooks/bundled/boot-md/handler.ts`
- `src/agents/workspace.ts`

`BOOT.md` is gateway-startup behavior, not normal per-run bootstrap context.

Why it matters:

- operator mental models around "what the agent sees" vs "what the gateway runs" get muddied

### 7. Gateway restart semantics are looser than the docs say

Docs:

- `https://docs.openclaw.ai/cli/gateway`

Docs say `SIGUSR1` triggers an in-process restart.

Code:

- `src/cli/gateway-cli/run-loop.ts`
- `src/infra/process-respawn.ts`

The actual behavior may prefer detached respawn or supervisor-style restart.

Why it matters:

- operational behavior depends on environment and config
- the docs over-simplify failure and restart modes

### 8. "Local auto-approval" docs overstate current handshake behavior

Docs:

- `https://docs.openclaw.ai/gateway/protocol`
- `https://docs.openclaw.ai/gateway/security`
- `https://docs.openclaw.ai/concepts/architecture`

Docs suggest same-host tailnet addresses are treated like local auto-approved traffic.

Code:

- `src/gateway/auth.ts` `isLocalDirectRequest()`
- `src/gateway/net.ts` `isLocalGatewayAddress()`
- `src/gateway/server/ws-connection/message-handler.ts`

The current handshake path effectively requires loopback for the local fast path.

Why it matters:

- remote-vs-local trust behavior is stricter than some docs suggest

### 9. Sandbox default docs disagree with each other and with code

Docs:

- `https://docs.openclaw.ai/gateway/sandboxing`
- `https://docs.openclaw.ai/gateway/security`

One page says `agents.defaults.sandbox.scope` defaults to `session`.

Code:

- `src/agents/sandbox/config.ts`

The code defaults to `agent`.

Why it matters:

- sandbox reuse and file exposure differ materially between `agent` and `session`

### 10. Channel/plugin packaging docs understate bundling

Docs:

- `https://docs.openclaw.ai/channels`

Docs label several channels as separately installed plugins.

Code:

- `src/channels/plugins/bundled.ts`

Several of those channels are statically bundled.

Why it matters:

- the actual dependency and deployment model is more "single binary plus bundled modules" than the docs imply

### 11. Session index path is wrong in local docs

Docs:

- `docs/pi-dev.md`

Docs say session index lives at `agents/<agentId>/sessions.json`.

Code:

- `src/config/sessions/paths.ts`

Real path is `agents/<agentId>/sessions/sessions.json`.

Why it matters:

- debugging instructions are wrong for anyone inspecting state by hand

### 12. Startup is not a pure read-only boot path

Docs:

- gateway docs broadly describe config loading and runtime startup

Code:

- `src/gateway/server.impl.ts`
- `src/gateway/boot.ts`

Startup can migrate config, auto-enable plugins, seed control-ui origins, and generate gateway auth.

Why it matters:

- the daemon mutates durable state during boot
- that is operationally significant and under-documented

## Recurring pattern

The docs are directionally right. The code is heavier because it adds:

- legacy compatibility
- dynamic policy overlays
- in-process plugin seams
- runtime caching
- multiple persistence shapes
- environment-specific restart logic

The clean conceptual model in the docs is mostly the aspirational one. The implementation is the operational one.

## Design takeaway

Use the docs as the list of important concepts. Do not use them as the cost model. The cost model lives in the code, and the code says OpenClaw became a framework platform wrapped around a capable assistant runtime.
