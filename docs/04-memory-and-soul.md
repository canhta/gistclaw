# Memory and Soul

## What this subsystem is trying to solve

OpenClaw uses workspace files to hold:

- agent role and behavior
- user profile
- operator instructions
- display identity
- durable memory
- startup/bootstrap rituals

The good idea is human-editable state. The bad idea is pushing too much of that state through raw prompt injection.

## Workspace files actually in play

Key code:

- `src/agents/workspace.ts`
- `src/agents/workspace-templates.ts`
- `src/agents/bootstrap-files.ts`
- `src/agents/bootstrap-cache.ts`
- `src/agents/system-prompt.ts`

`ensureAgentWorkspace()` seeds:

- `AGENTS.md`
- `SOUL.md`
- `TOOLS.md`
- `IDENTITY.md`
- `USER.md`
- `HEARTBEAT.md`
- `BOOTSTRAP.md` for first-run setup
- `.openclaw/workspace-state.json`

This is already more than the simple public story.

## What actually gets loaded into prompt context

Relevant files:

- `src/agents/bootstrap-files.ts`
- `src/agents/pi-embedded-helpers/bootstrap.ts`
- `src/agents/system-prompt.ts`

Normal bootstrap loading can include:

- `AGENTS.md`
- `SOUL.md`
- `TOOLS.md`
- `IDENTITY.md`
- `USER.md`
- `HEARTBEAT.md`
- one memory root file

Important nuance:

- daily `memory/YYYY-MM-DD.md` files are not automatically injected
- `MEMORY.md` is preferred over `memory.md`
- bootstrap file snapshots are cached by `sessionKey`

So mid-session edits can remain stale until session rollover or cache invalidation.

## BOOTSTRAP.md vs BOOT.md

These are not the same thing.

### `BOOTSTRAP.md`

- first-run ritual
- seeded by workspace setup
- part of workspace lifecycle

### `BOOT.md`

- gateway startup behavior
- executed via `src/gateway/boot.ts`
- triggered through a bundled startup hook

The docs often blur this distinction. The code does not.

## Memory in practice

OpenClaw's memory model is a hybrid:

### Curated file memory

- `MEMORY.md`
- lowercase fallback `memory.md`
- daily files under `memory/`

### Tool-driven memory retrieval

Core pieces:

- `extensions/memory-core/index.ts`
- `src/agents/tools/memory-tool.ts`
- `src/agents/memory-search.ts`

Default memory tools provide:

- `memory_search`
- `memory_get`

Retrieval defaults to the memory corpus, not to a unified session/event store.

### Silent pre-compaction memory flush

Core pieces:

- `src/auto-reply/reply/memory-flush.ts`
- `src/auto-reply/reply/agent-runner-memory.ts`

The runtime can append memory notes into `memory/YYYY-MM-DD.md` before compaction, deduped by transcript-tail hash.

That is clever, but it is also more prompt-and-file magic than a clean memory system should need.

## Soul, identity, and memory are not cleanly separated

The repo has distinct file names, but the runtime treatment is uneven.

### `SOUL.md`

- behaviorally important
- raw markdown
- no strong parser

### `IDENTITY.md`

- structured parsing exists
- mostly used for UI/display identity

### `USER.md`

- operator/user profile guidance
- prompt text, not typed state

### `MEMORY.md`

- durable context
- treated like another injected file

This is the third major design smell. The names are separate, but the runtime collapses them into one `Project Context` bucket.

## Prompt bloat is already acknowledged in code

OpenClaw has explicit size-management because the workspace model grew too large.

Evidence:

- per-file bootstrap limits
- total bootstrap limits
- truncation warnings
- reduced bootstrap for subagent/cron paths
- `systemPromptReport` diagnostics
- `doctor-bootstrap-size`

That is not a corner-case optimization. It is architectural pressure showing through.

## Strong parts worth keeping

### 1. Human-editable authored state

This is the best part of the design. Operators can inspect and edit files directly.

### 2. File seeding is transparent

Templates are visible and reproducible.

### 3. Memory flush has guardrails

Append-only target choice and dedupe hashing are thoughtful.

### 4. Prompt-size observability is good

The system at least measures the pain it created.

## What is over-engineered or risky

### 1. Too many file concepts

The workspace contains too many special files for one mental model.

### 2. Behavior and memory share one injection rail

Persona, user data, identity, tool notes, and memory all compete in prompt context.

### 3. Docs overstate privacy and determinism

The system asks the model to perform parts of memory ritual that the runtime does not enforce.

### 4. Bootstrap caching weakens live edit behavior

Editing a file does not guarantee the next turn sees it.

## Docs vs code

Material mismatches:

- `https://docs.openclaw.ai/concepts/agent-workspace` and `https://docs.openclaw.ai/start/openclaw` still show old config keys like `agent.workspace`; code uses `agents.defaults.workspace`
- `https://docs.openclaw.ai/concepts/agent` and `https://docs.openclaw.ai/concepts/system-prompt` are inconsistent about first-turn-only vs every-run injection; code rebuilds bootstrap every run
- `https://docs.openclaw.ai/concepts/memory` says `MEMORY.md` and `memory.md` both load; code prefers uppercase then falls back
- `https://docs.openclaw.ai/tools/subagents` understates subagent inheritance
- docs imply `MEMORY.md` is private-only; code injects it into normal non-subagent channel sessions

## Judgments

- Memory quality: medium
- Soul/identity quality: medium
- Human editability: high
- Prompt bloat risk: high
- Privacy-boundary clarity: low-medium

## What should be simplified

- keep authored files, but reduce them to a few typed concepts
- compile authored files into a small derived runtime snapshot
- move episodic memory, dedupe hashes, summaries, and retrieval state into a database
- enforce memory audience rules in code, not prose

## What should be deleted

- daily markdown memory as a default storage primitive
- raw markdown injection for every authored concept
- model-obedience-based "session startup memory ritual"

## What the lightweight redesign should do instead

Keep as files:

- `agents/<id>/soul.yaml`
- `agents/<id>/identity.yaml`
- `agents/<id>/facts.md`
- optional `agents/<id>/notes/*.md`

Stable-release soul authoring should stay structured.

`soul.yaml` should be an editable soul card with fields such as:

- role
- tone
- posture
- collaboration style
- escalation rules
- decision boundaries
- tool posture
- hard prohibitions
- short notes

The notes field is for nuance, not for replacing the whole structure with a raw mega-prompt.

Keep in SQLite:

- conversation events
- working summaries
- durable facts
- shared team memory
- memory-write audit trail

And keep one rule:

human-authored files are source material; runtime prompt context is a compiled product, not the raw file dump.
