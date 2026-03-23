# Memory Model

## Design goal

Keep memory useful, bounded, and inspectable.

OpenClaw's failure mode was mixing authored files, episodic memory, and prompt context into one fuzzy markdown-heavy system. The replacement should split those clearly.

## Memory layers

| Layer | Purpose | Storage | Human-editable | Default visibility |
| --- | --- | --- | --- | --- |
| Session memory | Exact conversation history | SQLite `events` | No | conversation-scoped |
| Working memory | Current compressed context for ongoing work | SQLite `run_summaries` | No | run-scoped |
| Durable facts | Stable facts promoted under bounded rules or written by humans | SQLite `memory_items` | Indirectly | scoped |
| Shared team memory | Facts intentionally shared across agents | SQLite `memory_items` | Indirectly | team-scoped |
| Agent-local memory | Facts only one agent should reuse | SQLite `memory_items` | Indirectly | agent-scoped |
| Curated facts | Operator-authored stable notes | `agents/<id>/facts.md` | Yes | agent-scoped |
| Optional notes | Human notes, not auto-injected | `agents/<id>/notes/*.md` | Yes | opt-in search only |

## What belongs in files

Keep in files:

- stable operator-authored facts
- short policy notes
- long-form notes worth hand editing

Do not keep in files:

- per-turn memory writes
- summaries
- dedupe hashes
- compaction checkpoints
- cross-agent fact records

Files are for authored truth, not runtime exhaust.

## What belongs in SQLite

SQLite owns:

- all conversation events
- all summaries
- all durable facts
- all memory provenance
- all visibility and scope metadata
- all memory-write approvals

Memory changes should also emit journal events so replay and receipts can show when facts were proposed, promoted, edited, forgotten, or published across scopes.

The stable release should expose that state through a memory inspector and editor in the local web UI.

This keeps the queryable system queryable.

## Why not Postgres before stable release

Postgres is not needed for the first sharp version.

Reasons:

- one binary is a core design goal
- SQLite handles one-writer daemon workloads well
- FTS5 is enough for the first stable release
- migrations and backup are much simpler

Use Postgres only after real evidence of:

- multi-writer deployment
- very large event volumes
- operational need for external DB tooling

## Why not vector storage before stable release

Vector search is optional, not foundational.

Reasons to exclude it:

- semantic search is not required for the first useful memory system
- vector infra adds cost and write amplification
- bad fact promotion hurts more than missing semantic recall

Use FTS first. Add embeddings only if real retrieval quality demands it.

## Memory tables

Recommended tables:

### `memory_items`

- `id`
- `scope_type` (`agent`, `team`, `user`, `project`)
- `scope_id`
- `kind` (`fact`, `preference`, `constraint`, `plan`, `note_ref`)
- `body`
- `source_type` (`human`, `run`, `import`, `tool`)
- `source_ref`
- `confidence`
- `visibility`
- `created_at`
- `updated_at`
- `expires_at`

### `memory_links`

- link durable facts to conversations, runs, users, or projects

### `run_summaries`

- `run_id`
- `conversation_id`
- `summary_kind` (`working`, `handoff`, `checkpoint`, `final`)
- `body`
- `token_estimate`
- `created_at`

### `memory_write_audit`

- records who wrote memory and why

## Memory write rules

Accepted policy:

- bounded auto-promotion
- local-by-default durable memory with explicit publish to team scope

Meaning:

- the runtime may promote a small set of typed durable facts without asking a human every time
- the runtime may not dump arbitrary free-form conclusions into long-term memory
- risky, ambiguous, or weak-confidence memory stays a proposal or gets dropped
- newly promoted durable memory belongs to the writing agent unless a publish rule says otherwise

Rules:

1. Session events are always persisted.
2. Working summaries are auto-generated.
3. Durable facts are not auto-promoted on every turn.
4. Only typed memory candidates are eligible for auto-promotion.
5. A candidate must carry source, provenance, confidence, dedupe key, and explicit scope.
6. A run may propose a fact; promotion policy decides whether it becomes durable.
7. Ambiguous, weak-confidence, or high-risk candidates remain proposals or are rejected.
8. Human-authored or human-edited facts override model-generated facts.
9. Facts can expire.
10. Shared-team facts require explicit shared scope.
11. Team-scoped durable memory is created only through explicit publish rules, not as the default destination.

This is important. Memory should be write-constrained, not write-happy.

## Memory inspection and editing

Stable-release UI requirements:

- list durable facts by scope, agent, kind, and source
- show provenance, confidence, and last-updated timestamp
- allow edit
- allow forget
- make scope visible before edit or delete

Rules:

- editing a fact updates provenance and audit state
- forgetting a fact should be explicit and reversible only through normal history or backup paths
- human edits outrank model-generated content

## Retrieval rules

Default read path for a run:

1. recent conversation tail
2. latest working summary
3. curated `facts.md`
4. soul-derived constraints
5. targeted FTS lookup only if the active agent or coordination logic says more context is needed

Do not automatically query the whole memory store on every turn.

## Summarization and compaction

Recommended strategy:

- keep recent raw events
- summarize older spans into `run_summaries`
- preserve message and tool provenance in the event log
- never rewrite the underlying event history

Compaction trigger options:

- token threshold
- message-count threshold
- explicit handoff/delegation
- run completion

Why this wins:

- compact prompt context
- durable audit trail
- no transcript mutation theater
- one journaled history for replay, receipts, and memory inspection

## Prompt bloat control

Rules:

- soul and curated facts are compiled before prompt use
- only one working summary is loaded by default
- no raw markdown dump of `notes/`
- no automatic loading of all durable memories
- retrieval results are capped and cited by source id

## Cross-agent memory sharing

Support three scopes:

- `local` to one agent
- `team` for an explicit agent set
- `project` for all agents in a project

Accepted posture:

- local by default
- explicit publish to `team`

Rules:

- child runs may read only the scope they are granted
- an agent does not automatically publish its durable memory to the team
- publish events must keep source agent, reason, and prior scope visible

Never infer shared scope from "same workspace."

## Privacy and trust boundaries

Rules:

- channel conversations never automatically see private user facts
- delegated child runs inherit only the memory scope explicitly granted
- human-private facts must be marked and enforced in code
- audit all durable-memory writes and reads by scope

## Alternatives considered

### Markdown-first memory

Pros:

- easy to inspect

Cons:

- hard to scope
- hard to query
- easy to bloat prompts

### Vector-first memory

Pros:

- better fuzzy retrieval later

Cons:

- infra and cost overhead
- hides bad memory hygiene

## Recommendation

Use:

- files for curated authored notes
- SQLite for runtime memory
- FTS for retrieval
- bounded fact promotion

That is enough for a strong first stable release.

## Intentionally excluded before stable release

- vector DB
- Postgres
- arbitrary free-form durable-memory writes from model output
- daily markdown memory journals
