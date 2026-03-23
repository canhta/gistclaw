# Security Model

## Design goal

Security has to be structural, not aspirational.

OpenClaw's strongest lesson is that "access control before intelligence" is correct, but the blast radius stays high if the runtime surface is too broad. The replacement should keep the principle and narrow the surface.

## Threat model

Assume:

- hostile inbound messages
- prompt injection in attachments and fetched content
- compromised connector credentials
- compromised model output
- misconfigured tool permissions
- malicious or buggy extensions

Do not assume:

- the model is obedient
- plugins are trustworthy
- every operator will configure least privilege correctly

## Trust boundaries

### Boundary 1: operator

The operator can configure agents, approve actions, and read logs. This is the top trust level.

### Boundary 2: daemon

The daemon is trusted to enforce policy, store audit records, and broker tool execution.

### Boundary 3: model provider

The provider is not trusted with local authority. It only receives prompt context and returns model output.

### Boundary 4: connectors

Connectors authenticate inbound events and deliver outbound messages. They are not allowed to mutate core runtime state directly.

### Boundary 5: tool workers

Risky tools execute in constrained subprocesses or sandboxes. They do not own policy.

### Boundary 6: extensions

At stable release, there are no in-process third-party code plugins. Later extensions should be out of process and capability-scoped.

## Identity and authentication

### Admin API

Recommended:

- local bind by default
- admin token or local socket auth
- explicit remote exposure only behind reverse proxy or configured listen address

### Connectors

Each connector authenticates inbound events using connector-native secrets.

Examples:

- webhook signature
- bot token
- OAuth access token

Connector auth does not grant admin API access.

Later-phase group-surface rules:

- if Telegram groups are added later, they must use explicit invocation rules
- group conversations must stay isolated from direct-message conversations unless an operator binds them intentionally
- durable memory must not silently merge across users just because they share a group
- low-risk approvals may resolve only inside the bound group thread
- medium and high-risk approval details must not spill into the group thread

## Authorization model

Use explicit roles and tool profiles.

### Agent role

Defines:

- collaboration posture
- escalation behavior
- default risk tolerance

### Tool profile

Defines per-tool decisions:

- `allow`
- `ask`
- `deny`

Stable-release posture:

- start from one built-in tool profile such as read-only, read-heavy, workspace-write, operator-facing, or elevated
- allow narrow per-tool overrides when needed
- do not make operators author every single tool decision from scratch for every agent
- freeze the effective tool-policy snapshot at root-run start so approvals and replay stay tied to the same rules the run actually used
- keep side effects concentrated in a narrow executor set instead of granting mutation power across every role

### Memory scope

Defines readable/writable memory domains:

- local
- team
- project
- private

This replaces many-layer policy overlays with three understandable knobs.

Stable-release sharing posture:

- durable memory writes default to local scope
- wider sharing requires explicit publish behavior and audit visibility

### Durable memory writes

Rules:

- durable memory writes are constrained state mutation, not free-form model output
- only bounded typed candidates may auto-promote
- candidates must include provenance, confidence, dedupe key, and explicit scope
- human edit and forget actions go through the same audit path as runtime writes
- private or sensitive memory scopes require stricter policy than local working summaries
- team-scope publish is stricter than local durable writes

## Tool execution security

### Shell

Rules:

- disabled by default
- allowed only for agents with explicit shell profile
- executed in worker sandbox
- command, cwd, env, exit code, stdout/stderr are audited

### File write

Rules:

- explicit allowlist roots
- each root run binds one declared workspace root inside those allowlists
- no writes outside that run's declared workspace root or runtime data root
- destructive operations require approval
- repo apply mode must show a preview before approval
- approved apply actions are bound to one run and one workspace scope
- first-run onboarding for `Repo Task Team` must stay preview-only on the user's real workspace

### HTTP

Rules:

- outbound allowlist support
- block private-network destinations by default unless explicitly allowed
- log host, method, status, and byte counts

### Messaging/email/calendar

Rules:

- external writes default to `ask`
- stable release should route these through operator-facing agents by default instead of every agent sending outward directly
- approval can be bypassed only by an explicit trusted agent profile

### Repo publish-back

Rules:

- repo publish-back is a later-phase outbound write, not a stable-release requirement
- when added later, it must show a rendered preview before posting
- it should use scoped credentials and avoid any always-on GitHub app or ambient repo bot

## Approval model

Approval request fields:

- request id
- run id
- agent id
- tool name
- normalized arguments fingerprint
- target scope
- normalized action summary
- preview ref when available
- risk level
- timeout
- created at

Rules:

- timeout defaults to deny
- approvals are single-use
- approvals are auditable
- approvals can be pre-authorized only by named tool profile
- approvals bind to one concrete action snapshot, not to a whole run or an elevated time window
- if tool arguments, preview, or target scope changes materially, the old ticket is invalid
- apply approvals must be tied to a concrete diff or change preview
- Telegram DM may resolve only low and medium-risk approvals and only for the bound conversation
- Telegram group threads may resolve only low-risk approvals and only for the bound thread
- medium-risk approvals that originate from groups must route to the operator's private DM
- high-risk approvals must require the local web UI or CLI
- Telegram approval cards must include a compact action summary and a replay or preview link

## Prompt injection posture

Core rule:

prompt text never authorizes actions.

Mitigations:

- external content labeled and provenance-tagged
- raw fetched content is not merged into soul or curated facts
- tools require policy allowance regardless of model request
- sensitive tools require structured action summaries before execution
- prompt text cannot directly create arbitrary durable memory

This is more important than elaborate prompt disclaimers.

## Secrets handling

Rules:

- secrets live outside prompt context
- connectors and providers receive only the secrets they need
- secret values never enter durable event logs
- redaction happens before log write

## Audit and observability

Audit records must exist for:

- model calls
- tool calls
- approvals
- memory writes
- outbound connector writes
- delegation creation

Recommended log fields:

- `trace_id`
- `run_id`
- `conversation_id`
- `agent_id`
- `connector_id`
- `tool`
- `provider`
- `latency_ms`
- `tokens_in`
- `tokens_out`
- `cost_usd`

## Operational defaults

Defaults:

- local-only admin API
- no shell access
- no destructive file writes
- no outbound messaging without approval
- no third-party in-process code
- conservative per-run and daily budget caps enabled

These defaults matter more than sophisticated documentation.

## Alternatives considered

### OpenClaw-style many-layer policy stack

Pros:

- highly configurable

Cons:

- hard to reason about
- easy to misconfigure

### Plugin-heavy extension model

Pros:

- flexible ecosystem story

Cons:

- high blast radius
- weak trust boundary

## Recommendation

Keep the security model boring:

- few trust boundaries
- few authority paths
- explicit approvals
- no prompt-granted power
- no third-party code in the core process

## Intentionally excluded before stable release

- native in-process plugins
- remote node execution
- wide remote admin/control plane
- implicit authority inheritance through prompt context
