# Tools, Skills, Plugins, and Security

## What this subsystem is trying to solve

OpenClaw wants agents to be powerful without being reckless.

That means:

- tools
- tool policy
- sandboxing
- host exec approvals
- skills
- plugin-added capabilities
- gateway method auth

The intent is serious. The composition is too large.

## Tool construction

Core files:

- `src/agents/openclaw-tools.ts`
- `src/agents/pi-tools.ts`
- `src/plugins/tools.ts`

Built-in tools already include a wide surface:

- sessions
- subagents
- web fetch/search
- file-ish helpers
- messaging
- cron
- nodes
- canvas/browser/media helpers
- plugin tools

OpenClaw does not start small and expand. It starts wide and then layers policy on top.

## Tool policy pipeline

This is one of the clearest examples of real engineering becoming too layered.

Core file:

- `src/agents/tool-policy-pipeline.ts`

Policy order is explicit:

- profile
- provider-profile
- global
- agent
- group
- sandbox
- subagent

That is disciplined. It is also too many overlapping policy dimensions for the default operating model.

## Sandbox and approvals

Core files:

- `src/agents/sandbox/config.ts`
- `src/agents/sandbox/tool-policy.ts`
- `src/agents/sandbox/validate-sandbox-security.ts`
- `src/infra/exec-approvals.ts`
- `src/agents/bash-tools.exec.ts`

Good news:

- sandbox hardening is real
- security checks reject obviously unsafe container settings
- approvals default deny on misses

Bad news:

- elevated full-host mode can bypass some of the approval safety story
- sandboxing is not the universal baseline
- tool safety still depends heavily on correct configuration and role setup

## Skills

Core file:

- `src/agents/skills/workspace.ts`

Skills are not just snippets. They are a second capability/governance surface:

- multiple source layers
- frontmatter parsing
- eligibility filters
- prompt injection into the run
- slash-command-like dispatch behavior

That is more than "convenient reusable instructions."

## Plugins

Core files:

- `src/plugins/types.ts`
- `src/plugins/registry.ts`
- `src/plugins/runtime/index.ts`

Plugin capabilities are broad. Native plugins can register:

- tools
- hooks
- HTTP routes
- gateway methods
- providers
- services
- CLI surfaces

This is the fifth major design smell. Third-party code runs inside the trusted core with a very large surface.

## Prompt injection blast radius

Core file:

- `src/security/external-content.ts`

OpenClaw does think about prompt injection, but the defense is mostly labeling and containment guidance, not a narrow execution surface.

Blast-radius contributors:

- raw workspace bootstrap files
- skills as prompt text
- large built-in tool surface
- plugin-added tools
- in-process plugin code

That is too much trust stacked in one place.

## What is genuinely strong

### 1. Access control exists before tool execution

The repo does not rely purely on prompt obedience.

### 2. Sandbox and tool policy are correctly treated as different layers

That is an important distinction and worth keeping.

### 3. Host exec approvals are a real subsystem

They are not hand-wavy docs.

### 4. Gateway auth scopes are real

Operator APIs have actual scope checks before handler execution.

## What is over-engineered or unsafe

### 1. Policy composition is too broad

The policy stack is powerful, but it is not easy to predict.

### 2. Native plugins are too trusted

In-process third-party code with access to tools, routes, and services is a large blast radius.

### 3. Skills are too close to capabilities

They are nominally guidance, but in practice they influence available action paths and command behavior.

### 4. Default tool surface is too large

The system begins from broad power and tries to claw safety back through policy.

## Useful abstractions vs fake abstractions

Useful:

- explicit tool-policy pipeline
- explicit approval subsystem
- separate sandbox and elevated modes

Fake or too expensive:

- plugin trust as a normal extension strategy
- skills as a quasi-policy layer
- policy-by-many-overlays as the default mental model

## Docs vs code

Strong match:

- `https://docs.openclaw.ai/gateway/sandbox-vs-tool-policy-vs-elevated`
- `https://docs.openclaw.ai/tools/exec-approvals`

Material mismatch:

- docs understate how broad the skill-loading precedence is
- docs understate plugin trust and runtime reach
- sandbox default scope docs conflict with code
- `plugins.allow` is described more softly than the real trust consequences justify

## Judgments

- Tool safety: medium
- Sandbox realism: medium-high
- Plugin risk: high
- Prompt injection resistance: medium-low
- Security stance quality: good intent, overly broad execution surface

## What should be simplified

- start from a small built-in tool set
- make shell and outbound side effects opt-in
- treat skills as passive authored guidance only
- remove in-process third-party plugins from v1
- use scoped credentials for admin HTTP APIs

## What should be deleted

- plugin-owned gateway methods in core
- plugin-owned in-process services in v1
- broad baseline tool power with layered deny lists trying to recover safety

## What the lightweight redesign should do instead

Adopt these rules:

1. Tools are typed actions with declared risk level.
2. Agents get one named built-in tool profile, not seven overlapping policy dimensions.
3. Shell runs through a worker sandbox and is disabled by default.
4. External extensions, if added later, run out of process with explicit capability contracts.
5. Prompt text never grants authority. Only policy does.

Stable-release tool access should use tool packs plus narrow overrides.

Recommended built-in tool profiles:

- read-only
- read-heavy
- workspace-write
- operator-facing
- elevated

Rules:

- every agent starts from one built-in tool profile
- operators may add a few explicit per-tool overrides when needed
- stable release should not require users to wire every tool decision from scratch
- stable release should not hide all tool access behind locked presets with no override path
- approvals bind to one concrete action snapshot, not an elevated time window or a vague run-wide permission grant
- if the planned action changes materially, the runtime must ask again
- stable release should also keep most roles propose-only and reserve real side effects for a narrow executor set
