# Codex First-Class Agent Integration — Design Spec

**Date:** 2026-03-14
**Status:** Approved for planning
**Scope:** Add Codex as a first-class coding agent alongside OpenCode and Claude Code, without collapsing the existing separation between chat providers and coding-agent runtimes.

---

## Overview

GistClaw already has two distinct runtime shapes for coding agents:

1. **OpenCode** — a long-lived local server with HTTP + SSE task submission
2. **Claude Code** — a per-task subprocess with stream-json output and a local hook server

Codex should be integrated as a **third coding-agent runtime**, not as an `LLMProvider`.
The existing Codex chat provider remains a separate concern for plain-chat use cases.

The recommended direction is:

- Introduce a shared **code-agent runtime layer**
- Implement **Codex first** on that layer
- Use **`codex app-server` over stdio JSON-RPC** as the primary integration surface
- Keep phase 1 local-first, with cloud/runtime backends left open behind provider interfaces

This gives GistClaw a stable way to support:

- structured task events
- durable sessions and turns
- richer approvals and questions
- multi-agent orchestration across providers
- future migration of OpenCode and Claude to the same runtime model

---

## Problem

The current architecture separates plain-chat providers from coding agents well, but the coding-agent side is still thin and adapter-specific.

Current issues:

- OpenCode and Claude are exposed through nearly identical but ad hoc service interfaces
- agent state is mostly held in memory
- the `sessions` table is too coarse to represent rich agent task history
- orchestration tools are hard-coded to OpenCode and Claude
- task output is streamed directly to Telegram instead of first becoming normalized runtime events

This is acceptable for simple prompt-in / text-out task submission, but Codex exposes a richer execution model:

- durable threads and turns
- structured command execution events
- structured file-change events
- approval requests
- user-input requests
- plan updates
- token-usage updates
- sub-agent collaboration items

If Codex is added as another thin adapter, GistClaw will either ignore those capabilities or grow a third incompatible runtime path.

---

## Goals

- Add Codex as a first-class coding agent targeted by commands, scheduler jobs, and orchestration tools
- Preserve the current separation between chat providers and coding-agent runtimes
- Define a clean provider/runtime boundary that works for Codex, OpenCode, and Claude
- Normalize task execution into structured events before sending user-facing output
- Support multi-agent orchestration across Claude and Codex
- Add durable session and task persistence suitable for restart recovery and history inspection
- Define a reusable instruction model for coding-agent execution

---

## Non-Goals

- Replacing the existing plain-chat `providers.LLMProvider` stack
- Rewriting OpenCode and Claude integrations in the same change
- Building Codex Cloud support in phase 1
- Reimplementing Codex-native tools inside GistClaw
- Shipping a new user-facing UI beyond existing Telegram controls

---

## Assumption

Phase 1 assumes:

- Codex runs locally on the same machine as GistClaw
- GistClaw manages Codex as a local process
- the preferred transport is `codex app-server` over stdio JSON-RPC

This assumption keeps the design concrete without blocking future cloud or remote-runtime support. Cloud support should be modeled later as another runtime backend, not as a change to the orchestration model.

---

## Approaches Considered

### A. Thin `codex exec` wrapper

Run `codex exec <prompt>` as a subprocess and stream stdout back to Telegram.

**Pros**

- fastest initial spike
- low implementation cost
- no new persistent task model required for a demo

**Cons**

- poor fit for durable sessions
- weak HITL support
- no structured approvals, plan updates, or token usage
- difficult to resume or inspect work after restart
- turns Codex into another special case

### B. Shared runtime + Codex app-server

Introduce a shared code-agent runtime layer and implement Codex against `codex app-server`.

**Pros**

- uses Codex’s structured thread/turn/event model directly
- cleanly maps approvals and questions into GistClaw HITL flows
- supports durable sessions and event replay
- gives GistClaw a long-term runtime boundary for all coding agents

**Cons**

- more up-front design work
- requires new persistence tables and event normalization
- protocol fields marked experimental must be version-pinned

### C. Full runtime refactor before adding Codex

Refactor OpenCode and Claude onto a shared runtime layer first, then add Codex.

**Pros**

- cleanest eventual shape
- no temporary asymmetry

**Cons**

- too much scope before delivering Codex support
- delays user value

### Recommendation

Choose **B**.

Build the shared runtime boundary now, implement Codex first on it, and leave OpenCode and Claude as compatibility adapters until they can be migrated later.

---

## Architectural Direction

The core change is to add a new internal boundary:

```text
gateway / scheduler / tools
        |
        v
AgentController
        |
        +-- CodeAgentProvider: opencode adapter
        +-- CodeAgentProvider: claudecode adapter
        +-- CodeAgentProvider: codexagent
```

The `AgentController` becomes the single owner of:

- provider registration
- session creation and lookup
- task submission
- event fan-out
- HITL request routing
- persistence of sessions, tasks, and events
- translation from runtime events to Telegram messages

Providers become transport/runtime adapters. They should not own user messaging or orchestration policy.

---

## Runtime Interfaces

The new coding-agent layer should live in a dedicated package such as `internal/codeagent/`.

### Provider contract

```go
type Provider interface {
    Name() string
    Kind() agent.Kind
    Capabilities() Capabilities

    Run(ctx context.Context) error
    StartSession(ctx context.Context, spec SessionSpec) (Session, error)
    ResumeSession(ctx context.Context, nativeID string, spec SessionSpec) (Session, error)
    StopSession(ctx context.Context, sessionID string) error
    IsAlive(ctx context.Context) bool
}
```

### Session contract

```go
type Session interface {
    ID() string
    NativeID() string
    StartTask(ctx context.Context, req TaskRequest) (Task, error)
    ResolveApproval(ctx context.Context, res ApprovalResolution) error
    ResolveQuestion(ctx context.Context, res QuestionResolution) error
}
```

### Task contract

```go
type Task interface {
    ID() string
    Events() <-chan Event
    Wait(ctx context.Context) (TaskResult, error)
}
```

### Capabilities

`Capabilities` should capture what a runtime can do without exposing transport details:

- `StructuredApprovals`
- `StructuredQuestions`
- `PlanUpdates`
- `TokenUsage`
- `Resume`
- `Subagents`
- `FilesystemOps`
- `CommandExecution`
- `FilePatchTracking`

This allows orchestration logic to branch on capability instead of on provider name.

---

## Codex Runtime Design

Codex should be implemented in a new package such as `internal/codexagent/`.

### Transport

Use `codex app-server` launched as a subprocess with stdio pipes.

Reasons:

- it exposes threads and turns directly
- it supports structured server notifications
- it supports structured approval and question requests
- it does not require introducing a websocket dependency for phase 1
- it fits the current supervised-local-process model already used elsewhere in GistClaw

### Native mapping

| GistClaw concept | Codex concept |
| --- | --- |
| session | thread |
| task | turn |
| task event | notification or thread item delta |
| stop | turn interrupt or thread close |
| HITL approval | server request / approval response |
| question flow | tool request user input |

### Session lifecycle

1. Start `codex app-server`
2. Initialize JSON-RPC handshake
3. Start or resume a thread
4. Start a turn for each submitted task
5. Consume notifications and server requests
6. Normalize them into GistClaw runtime events
7. Persist all events

### Turn configuration

Per-turn overrides should carry:

- cwd
- sandbox policy
- approval policy
- model
- developer instructions
- optional output schema

This allows scheduler jobs and orchestration chains to tailor behavior without mutating the whole session.

---

## OpenCode and Claude Positioning

OpenCode and Claude should remain usable immediately, but they should be treated as transitional implementations of the same higher-level runtime role.

### OpenCode

OpenCode already behaves like a session-oriented local service. Its current GistClaw adapter maps naturally to:

- session = OpenCode session
- task = prompt submission
- events = SSE stream

Phase 1 does not need to rewrite it, but the long-term target is to emit normalized runtime events instead of sending Telegram messages directly.

### Claude Code

Claude Code remains the least structured runtime:

- task = subprocess run
- session = pseudo-session or synthetic thread
- approvals = hook server callbacks

Claude can still fit the shared runtime by emitting coarser normalized events.

---

## Agent Execution Loop

The runtime loop should be event-driven.

### Task flow

1. Gateway or scheduler requests a task from `AgentController`
2. Controller chooses provider by `agent.Kind`
3. Controller creates or resumes a session
4. Provider starts a task
5. Provider emits runtime events
6. Controller persists events and routes HITL requests
7. Controller renders selected events to Telegram
8. Controller marks task terminal state and stores result summary

### Normalized event model

At minimum:

- `task.started`
- `text.delta`
- `plan.updated`
- `command.started`
- `command.output`
- `command.completed`
- `file.change`
- `approval.requested`
- `question.requested`
- `token_usage.updated`
- `task.completed`
- `task.failed`

The controller should be the component that decides which events become user-visible messages.

This is a deliberate change from the current model where providers stream directly to Telegram.

---

## Tool Integration

The design must not confuse two very different tool systems:

1. **Gateway chat tools** used in plain-chat mode
2. **Coding-agent native tools** used by OpenCode, Claude, and Codex while performing coding work

### Plain-chat tools

Keep the current gateway tool loop for:

- `web_search`
- `web_fetch`
- scheduler tools
- memory tools
- MCP-exposed tools
- agent orchestration tools

These belong to the plain-chat assistant path.

### Coding-agent native tools

Codex already owns command execution, file changes, approvals, and tool/user-input requests.
GistClaw should not wrap these as fake gateway tools.

Instead, GistClaw should configure policy:

- working directory
- writable roots
- sandbox mode
- approval mode
- environment
- result/output schema

The same principle applies to OpenCode and Claude. GistClaw should orchestrate them, not impersonate their internal tool runtimes.

---

## Multi-Agent Orchestration

The current orchestration tools in `internal/tools/agents_tool.go` are hard-coded to OpenCode and Claude. That should be replaced with controller-backed orchestration.

### New orchestration model

Represent orchestration as workflows of tasks:

- each task targets one `agent.Kind`
- tasks may run independently or depend on prior task completion
- results can be passed forward as structured task output

This supports:

- fire-and-forget delegation
- parallel fan-out
- sequential chaining
- review/fix loops

### Parent-child relationships

Persist:

- `parent_task_id`
- `workflow_id`
- `depends_on_task_id`

This is more general than provider-native subagents and lets Claude and Codex participate in the same orchestration graph.

### Provider-native subagents

If Codex emits collaboration/sub-agent items, treat them as runtime details that can be surfaced in events, but do not make them the system-wide orchestration abstraction.

---

## Session and Task Persistence

The current `sessions` table is too coarse for this design.

Add or replace with the following persistence model:

### `agent_sessions`

- `id`
- `kind`
- `provider`
- `native_id`
- `chat_id`
- `cwd`
- `status`
- `created_at`
- `updated_at`
- `finished_at`

### `agent_tasks`

- `id`
- `session_id`
- `parent_task_id`
- `workflow_id`
- `prompt`
- `status`
- `result_summary`
- `created_at`
- `started_at`
- `finished_at`

### `agent_events`

- `id`
- `task_id`
- `seq`
- `type`
- `payload_json`
- `created_at`

### `agent_pending_actions`

- `id`
- `task_id`
- `provider_request_id`
- `kind`
- `status`
- `payload_json`
- `created_at`
- `resolved_at`

This allows:

- replay after restart
- exact routing of approvals and questions
- reconstruction of task history
- better debugging and operator inspection

The old `sessions` table can remain temporarily during migration if needed, but it should not stay the primary execution state store.

---

## HITL Mapping

GistClaw already has a working HITL service. The key change is that HITL requests should originate from normalized runtime events, not from provider-specific side effects.

### Command approvals

For Codex, map approval requests into GistClaw HITL records and resolve them back into:

- accept once
- accept for session
- decline
- cancel

Where Codex supports richer amendment decisions, phase 1 should store the proposed amendment payload even if Telegram only exposes a simpler approval set at first.

### File-change approvals

Support at least:

- allow once
- allow for session
- reject

### Questions / request-user-input

Codex question requests should flow through the existing question UI model already used for OpenCode. This is a good fit for the current HITL abstractions.

---

## Instruction Model

Coding-agent tasks need more structure than plain-chat prompts.

The instruction stack should be explicitly layered.

### 1. Base instructions

Stable GistClaw-wide contract for coding agents:

- work scope
- verification expectations
- output expectations
- safety boundaries

This should be shared across OpenCode, Claude, and Codex where possible.

### 2. Developer instructions

Runtime-specific execution policy:

- provider quirks
- expected reporting format
- orchestration metadata
- task-local constraints

For Codex, this maps well to per-thread or per-turn developer instructions.

### 3. Repository instructions

Repository guidance from `AGENTS.md` and nested repo instructions.

Codex already has native AGENTS support. GistClaw should prefer native behavior instead of flattening the entire file tree into the user message.

### 4. Task envelope

Per-task payload:

- user request
- target directory
- branch or worktree context
- success criteria
- tests to run
- completion or stop conditions

### 5. Output schema

For machine-readable chaining and summaries, phase 1 should define a structured final output contract such as:

```json
{
  "status": "completed|blocked|failed",
  "summary": "string",
  "files_changed": ["string"],
  "tests_run": [
    {
      "command": "string",
      "status": "passed|failed|not_run"
    }
  ],
  "follow_ups": ["string"]
}
```

Codex should use a structured final output when possible. Other providers can emulate this gradually.

---

## Config Changes

Phase 1 should add Codex runtime config separate from `LLM_PROVIDER`.

Recommended fields:

- `CODEX_BIN`
- `CODEX_DIR`
- `CODEX_HOME`
- `CODEX_MODEL`
- `CODEX_SANDBOX_MODE`
- `CODEX_APPROVAL_POLICY`

And one new `agent.Kind`:

- `KindCodex`

Important rule:

- `LLM_PROVIDER=codex-oauth` remains the plain-chat provider choice
- `KindCodex` is the coding-agent runtime target

These are related to the same vendor but represent different architectural roles.

---

## Failure Handling

### Provider crash

If the Codex app-server process exits unexpectedly:

- provider returns an error to supervision
- active tasks are marked failed or interrupted
- pending HITL actions are closed
- controller can notify the operator

### Restart recovery

Because session/task/event history is durable, recovery should at minimum preserve:

- failed task history
- unresolved approvals
- session metadata

Full automatic reattachment to live tasks can be deferred if too risky for phase 1.

### Version drift

Codex app-server includes experimental fields. Mitigate this by:

- pinning a supported Codex CLI version
- isolating protocol mapping in `internal/codexagent`
- treating unknown notifications as non-fatal

---

## Testing Strategy

Phase 1 should cover:

- config validation for Codex runtime fields
- `KindCodex` parsing and scheduler routing
- controller registry and provider dispatch
- Codex notification parsing and normalization
- approval and question routing
- persistence of sessions, tasks, and events
- orchestration tool support for Codex targets

Use fake provider transports for most tests. Reserve end-to-end tests with a real Codex binary for optional integration coverage.

---

## Rollout Plan

### Phase 1

- add `KindCodex`
- add Codex runtime config
- introduce `codeagent` runtime interfaces
- add `AgentController`
- implement `codexagent` on `codex app-server`
- add durable session/task/event persistence
- add a `/cx` command and scheduler support
- update orchestration tools to use the controller registry

### Phase 2

- migrate OpenCode onto normalized runtime events
- migrate Claude onto normalized runtime events
- improve Telegram UX for richer approval decisions
- add session/task inspection commands

### Phase 3

- support remote or cloud Codex runtimes behind the same provider interface
- support deeper workflow visualization and history replay

---

## Risks and Tradeoffs

### Main tradeoff

The recommended design does more up front than a simple `codex exec` wrapper, but it avoids locking GistClaw into a third incompatible runtime shape.

### Key risks

- Codex protocol drift if versioning is not pinned
- scope creep if OpenCode and Claude migrations are pulled into phase 1
- too much Telegram UX ambition too early for advanced approvals

### Mitigations

- pin Codex version
- keep OpenCode and Claude as compatibility adapters for now
- ship the minimum HITL decision set first and preserve richer payloads in storage

---

## Acceptance Criteria

This design is considered correctly implemented when:

- Codex can be targeted as a first-class agent kind
- a Codex task runs through the same controller path as other coding agents
- task state is persisted as session, task, and event records
- Codex approvals and question flows route through HITL
- multi-agent orchestration tools can target Codex and Claude uniformly
- instruction layering is explicit and separated from plain-chat provider prompts
- the existing Codex chat provider remains independent from the new Codex agent runtime

---

## References

External research used for this design:

- OpenCode repository and server/session API
- Codex CLI repository
- Codex app-server protocol types
- Codex documentation for non-interactive execution, sandboxing, prompts, and AGENTS handling
