# Lightweight Replacement Architecture

## Design stance

Do not preserve OpenClaw's architecture.

Preserve only the parts that clearly earn their complexity:

- explicit agent identity
- explicit delegation
- durable memory
- serious tool safety
- operator-authored behavior

Everything else starts from zero.

## Design target

This is not "OpenClaw in Go."

It is a replayable low-burn agent team runtime.

The first stable release must ship together:

- multi-agent delegation
- user-defined team composition
- structured team editor backed by the same team files
- scoped persistent memory
- memory inspector and editor for durable memory
- exactly one real external connector
- first-class local web UI
- replay and inspect surfaces
- first-class run receipts
- simple comparison only if it falls out cheaply from the same receipt data
- one iconic end-to-end starter workflow
- approval-gated apply mode for the starter workflow
- verification loop with recorded evidence
- grounded inline why-explanations
- quiet-by-default trust surface
- explicit budget controls for idle burn and per-run cost

If any of those are missing, the product collapses into either:

- a smaller but still forgettable OpenClaw clone
- a framework platform for future work
- a toy single-agent runtime

Stable-release modularity rule:

- keep interfaces only at real external seams
- keep first-run, presentation, and later-phase sharing logic as concrete services inside a few packages until a second implementation proves otherwise

## 1. Runtime shape

### Recommended design

Ship one Go binary with:

- `gistclaw serve`
- `gistclaw run`
- `gistclaw inspect`

`serve` owns:

- inbound connectors
- run orchestration
- replay event capture
- provider calls
- tool execution
- approvals
- scheduler
- admin HTTP API

Stable-release trigger posture:

- inbound events may start runs
- explicit schedules may start runs
- no autonomous background agent loops may start runs on their own

### Alternative

Rebuild OpenClaw's gateway, WS control plane, and node mesh.

### Tradeoffs

- broader control planes help remote orchestration
- they also bring the largest complexity tax in the entire current repo

### Why the recommendation wins

The target is a compact assistant runtime, not a general remote-control platform.

### Intentionally excluded before stable release

- remote node mesh
- device pairing
- general-purpose WebSocket RPC

## 2. Process model

### Recommended design

One daemon process plus optional short-lived worker subprocesses for risky tools.

Rules:

- daemon is the only writer of runtime state
- worker subprocesses do not own policy
- core orchestration never depends on shelling out
- daemon restart reconciles unfinished runs into explicit `interrupted` state
- stable release requires an operator action to resume or rerun interrupted work

### Alternative

Many sidecars or long-lived worker pools.

### Tradeoffs

- more processes can isolate more aggressively
- they also turn a compact runtime into distributed-systems work

### Why the recommendation wins

It preserves single-binary operational simplicity while still allowing targeted isolation.

### Intentionally excluded before stable release

- always-on tool workers
- remote workers

## 3. Persistence

### Recommended design

Use SQLite for:

- agents
- conversations
- events
- runs
- delegations
- tool calls
- approvals
- schedules
- durable memory
- summaries
- connector bindings

Use files only for:

- `config.yaml`
- `agents/<id>/soul.yaml`
- `agents/<id>/identity.yaml`
- `agents/<id>/facts.md`
- optional `agents/<id>/notes/*.md`
- attachments
- logs

Persistence spine:

- `events` is the append-only journal for conversation and run activity
- `runs`, `delegations`, `tool_calls`, `approvals`, `schedules`, `summaries`, and durable-memory views are current-state tables or indexed projections, not rival sources of truth
- replay, live activity, receipts, and debugging must read from the same journal plus projections
- the daemon appends events and updates projections in one transaction boundary

### Alternative

- JSON plus transcript files
- Postgres
- vector storage

### Tradeoffs

- JSON is inspectable but not coherent enough
- Postgres scales further but adds service sprawl
- vectors are useful later, not necessary first

### Why the recommendation wins

It removes OpenClaw's split-truth persistence model immediately.

### Intentionally excluded before stable release

- Postgres
- vector DB
- transcript files

## 4. Multi-agent collaboration

### Recommended design

Make collaboration run-centric.

Teams are operator-defined. The runtime can ship defaults, but the operator must be able to define any team shape they want without rebuilding the binary.

That flexibility should come from composition, not arbitrary runtime primitives.

The stable release should ship one editable starter team plus a bounded capability model.

Operators should be able to name agents however they want and wire them into whatever team shape fits the work.

The runtime should care about validated capability flags and handoff structure, not a user-facing catalog of fixed role names.

The stable release should support both:

- file-backed team specs for source control and manual editing
- a lightweight visual composer in the local web UI that edits the same format

Each run carries:

- `run_id`
- `agent_id`
- `team_id`
- `conversation_id`
- `parent_run_id`
- `objective`
- `workspace_root`
- `tool_profile`
- `memory_scope`
- `max_depth`
- `max_active_children`
- `status`
- `execution_snapshot_id`

Conversation concurrency rule:

- each canonical conversation may have only one active root run at a time
- child delegations may run concurrently under that root run
- each root run has a small fixed active-child budget with backpressure
- new inbound asks for the same conversation queue behind the active root run or attach as explicit follow-up work
- stable release does not allow competing root runs to write into the same conversation at once

Delegation creates:

- a child run
- a delegation edge
- a handoff summary
- inherited budget state
- inherited execution snapshot
- inherited workspace root
- queue placement when no child slot is available

Team collaboration rule:

- each agent instance binds to a validated capability posture plus per-agent overrides
- teams define explicit allowed handoff edges between agents
- critique or return paths must be declared separately from forward delegation
- free-form peer chat is not a stable-release primitive
- only agents with workspace-write capability own workspace side effects
- only agents marked operator-facing own outbound user messaging
- most agents stay propose-only or read-heavy by default

### Alternative

- session-based subagents
- ACP-like second runtimes
- free-form peer-agent chat
- arbitrary unvalidated capability surfaces

### Tradeoffs

- session-based delegation is flexible inside chat surfaces
- run-based delegation is clearer, safer, and easier to audit

### Why the recommendation wins

The user asked for inspectable delegation with loop protection. Run records provide that directly.

### Intentionally excluded before stable release

- autonomous swarms
- thread-bound child sessions
- unrestricted peer-to-peer agent chat
- second delegation runtimes
- arbitrary runtime-defined role kinds

## 5. Soul and identity

### Recommended design

Split behavior from display identity.

Files:

- `agents/<id>/soul.yaml`
- `agents/<id>/identity.yaml`

`soul.yaml` holds:

- role
- tone
- posture
- collaboration style
- escalation rules
- decision boundaries
- tool posture
- hard prohibitions
- short notes

`identity.yaml` holds:

- display name
- short bio
- channel-facing labels or signatures

### Alternative

- markdown bootstrap files
- DB-only persona state
- raw full-prompt editor per agent
- mostly locked soul presets

### Tradeoffs

- files are better for audit and versioning
- DB editing is easier for UI-driven systems
- raw prompt editing feels flexible but becomes much harder to compare, validate, and debug
- locked presets are safer but weaken the custom-team promise

### Why the recommendation wins

Soul is authored policy, not generated runtime state.

Stable-release soul editing should be structured first:

- editable fields for role, tone, posture, collaboration style, escalation rules, decision boundaries, tool posture, and prohibitions
- one short notes field for nuance
- no raw full-prompt editor as the default or primary surface

### Intentionally excluded before stable release

- auto-mutating personality
- many overlapping persona files
- raw full-prompt soul editing
- live mutation of in-flight runs after a soul edit

## 6. Memory

### Recommended design

Use layered memory:

- conversation history in `events`
- working context in `run_summaries`
- durable facts in `memory_items`
- shared team memory via scoped `memory_items`
- curated authored facts in `facts.md`

Sharing posture:

- durable memory is local-to-agent by default
- facts move to team scope only through explicit publish rules
- replay must show when a fact was published from local to team scope

Retrieval defaults:

- recent conversation tail
- latest working summary
- compiled authored facts
- FTS lookup only when needed

Stable-release operator surface:

- inspect durable memory by scope, agent, and source
- edit or forget durable facts without touching raw tables
- show provenance and last-updated information directly in the local web UI

Durable-memory write policy:

- session events are always persisted
- working summaries are auto-generated
- durable facts may auto-promote only from narrow typed candidates
- typed candidates must carry source, provenance, confidence, dedupe key, and explicit scope
- ambiguous or high-risk candidates stay as proposals or are dropped
- human edits override model-written memory
- team-shared durable memory is never the default sink for new facts

### Alternative

- markdown-first memory
- vector-first retrieval

### Tradeoffs

- markdown is readable but hard to govern
- vectors help later but add cost and infra

### Why the recommendation wins

It keeps memory durable without recreating OpenClaw's prompt-bloat trap.

### Intentionally excluded before stable release

- daily markdown memory journals
- hidden fact promotion on every turn
- free-form model writing into durable memory
- team-wide memory writes by default
- vector retrieval

## 7. Connectors and interfaces

### Recommended design

Every connector normalizes inbound traffic into one envelope:

- connector id
- account id
- actor id
- conversation id
- thread id
- message id
- text
- attachments
- capability flags
- received timestamp

Connector packages own:

- webhook or polling specifics
- auth verification
- payload normalization
- outbound delivery

Outbound delivery rule:

- the daemon records a durable outbound intent before connector delivery
- stable release uses at-least-once delivery with retries and connector-specific dedupe keys where possible
- replay and receipts must show both outbound intent and delivery outcome
- stable release does not claim exactly-once delivery for chat connectors

The stable release should ship with exactly one real connector chosen for product clarity and operational simplicity.

Recommended stable-release connector:

- Telegram

Why Telegram wins:

- straightforward setup for self-hosters
- strong DM and group ergonomics
- good mobile presence
- clearer product-magic than email
- less enterprise friction than Slack

Stable-release Telegram scope:

- direct messages only

DM rule:

- Telegram acts as a narrow conversational surface, not a collaboration platform
- one operator-facing agent owns the DM thread alias and delegates behind the scenes
- DM replies stay concise and checkpoint-oriented while deeper inspection stays in local replay
- DM continuity must resolve through one canonical conversation-key model shared by ingest, runs, approvals, replay, and receipts
- one DM thread maps to one active root run at a time

Later-phase connector expansion:

- bounded Telegram groups can come after the DM loop is stable
- replay sharing and repo publish-back are not on the stable-release critical path

### Alternative

A large plugin framework with registries and runtime docking.

### Tradeoffs

- large plugin systems look extensible
- they create global state and hidden dependency weight early

### Why the recommendation wins

The runtime needs a narrow connector seam, not a platform.

### Intentionally excluded before stable release

- many first-party connectors at once
- third-party connector plugins
- channel DSLs

## 8. LLM provider layer

### Recommended design

Keep one small provider interface:

- generate
- stream
- tool-call decode
- usage accounting
- capability flags

Provider-specific behavior stays inside provider packages.

Stable-release model-routing policy:

- use fixed agent- and phase-based model lanes
- cheap lanes for routine work by default
- stronger lanes for escalation, synthesis, verification, and other high-signal phases
- every run receipt must show which lane was used and where escalation happened
- no hidden live auto-router choosing expensive models behind the operator's back

### Alternative

One giant provider framework with registry-heavy abstraction.

### Tradeoffs

- generic provider frameworks look clean in docs
- real providers still branch on vendor behavior

### Why the recommendation wins

OpenClaw already proves the branching is unavoidable. Keep it explicit and contained.

### Intentionally excluded before stable release

- provider plugins
- broad provider fallback mesh
- hidden dynamic model auto-routing
- embeddings abstraction unless actually needed

## 9. Tool and action framework

### Recommended design

Ship a small typed action system.

Built-in stable-release tools:

- file read
- file write
- shell exec
- HTTP request
- web search
- outbound message send
- email send
- calendar read/write

Every tool declares:

- schema
- risk level
- side-effect class
- approval mode

Approval posture:

- approvals are snapshot-bound and single-use
- an approval ticket binds to one concrete action bundle: tool, normalized arguments, targets, risk summary, and preview when available
- if the planned action changes materially, the ticket expires and the runtime must ask again
- stable release does not support broad elevated windows or fuzzy run-scoped approval grants

Policy model:

- one named tool profile per agent
- the profile comes from a small built-in tool-pack catalog such as read-only, read-heavy, workspace-write, operator-facing, and elevated
- operators may add narrow per-tool overrides when needed
- each tool still resolves to `allow`, `ask`, or `deny`
- risky tools default to `deny`
- stable release should keep real side effects concentrated in agents with explicit write or outbound capabilities instead of spreading mutation power across the whole team

Starter-workflow rule:

- each root run may apply changes only inside one declared workspace root
- apply mode requires an explicit approval checkpoint before side effects
- replay and receipts must record both the proposed change and the approved apply action
- verification results must be recorded before final handoff or apply when relevant checks exist

### Alternative

Broad tool sets plus many-layer allow/deny overlays.

Other alternatives:

- fully per-tool custom wiring for every agent
- mostly locked presets with little or no override path

### Tradeoffs

- overlay systems are flexible
- they are hard to predict and audit
- full per-tool custom wiring is powerful but turns setup into safety bookkeeping
- locked presets are simpler but weaken the custom-team promise

### Why the recommendation wins

One agent, one tool profile is easy to explain, test, and debug.

Built-in tool packs plus narrow overrides preserve that clarity while still giving users enough flexibility to shape real teams.

### Intentionally excluded before stable release

- in-process code plugins
- dynamic third-party tool registration
- browser automation
- fully custom per-tool policy authoring from a blank slate
- repo publish-back

## 10. Operational UX

### Recommended design

CLI:

- `gistclaw serve`
- `gistclaw run`
- `gistclaw inspect status`
- `gistclaw inspect replay <run_id>`
- `gistclaw inspect runs`
- `gistclaw inspect conversations`
- `gistclaw inspect approvals`
- `gistclaw doctor`

Admin UI:

Design source of truth: `docs/designs/ui-design-spec.md`. Implement against that spec, not
against the high-level bullets below.

- first-class local web UI, desktop-first (1024px minimum width)
- read-only by default; write actions require admin bearer token
- global navigation: top bar with Runs | Approvals (badge when pending) | Settings; idle
  indicator on far right shows real-time run count; no sidebar
- default view after onboarding is the Runs page, not a blank canvas
- active runs update live in the local web UI via SSE; no page reload required
- the calm-state view is the idle indicator in the nav bar: explicit "● idle" label when no
  model calls are in flight — operators should be able to trust silence
- run detail layout adapts by run state:
  - ACTIVE: live replay graph (primary, left 60%) + timeline (right 40%) + current-step bar
  - NEEDS APPROVAL: full-width blocking approval card (amber left border) as the primary content
  - COMPLETED: receipt (primary, left 40%) + static replay graph (right 60%) + timeline + why-cards
  - INTERRUPTED: status + resume/rerun actions (primary) + partial replay (secondary)
- replay graph visual form: vertical top-down flow diagram with rectangular nodes; node border
  encodes capability (circle = read-heavy, diamond = workspace-write, blue = operator-facing);
  node background encodes run state; not a force-directed graph
- approvals: inline (no modal); each pending approval shows tool, args, risk, and preview diff
  before the operator must decide; Approve/Deny keyboard-accessible
- team editor backed by the same file format the CLI uses; visual composer uses same node
  vocabulary as the replay graph
- first-time users land on a 4-step onboarding wizard (workspace bind → repo scan → task
  shortlist → preview-only first run); no blank canvas, no gallery
- no emoji anywhere in the UI; text-only throughout
- ARIA landmarks: nav, main, aside, alert roles; aria-live for SSE updates; WCAG AA contrast

Observability:

- structured JSON logs
- per-run trace ids
- provider latency and token accounting
- tool latency and approval events
- idle burn counters and budget denials
- user-facing run receipts and basic comparison views
- comparison is an explicit operator action, not an automatic hidden baseline run
- conservative per-run and daily budget caps are enabled by default with explicit override
- live replay updates stay in the local web UI, not external chat surfaces
- explicit background-activity snapshot showing active runs, scheduled work, pending approvals, and zero-model-call idle state when true
- interrupted runs remain visible until resumed, rerun, or dismissed by the operator
- outbound intents and delivery retries remain visible until delivered, failed terminally, or canceled

Config:

- one `config.yaml`
- restart on change
- no magical hot reload beyond what is proven necessary
- config-triggered restarts must reconcile active runs into explicit `interrupted` state rather than silently resuming them
- edits to config, team specs, soul, and tool policy take effect for new root runs, not active ones

Deployment:

- one binary
- one state directory
- sample systemd unit outside core runtime code

Automation:

- event-driven and explicit-schedule starts only
- no always-on proactive agent loops
- status surfaces must show scheduled work clearly enough that "idle" remains trustworthy

### Alternative

Rich remote control planes and many restart modes.

### Tradeoffs

- richer control planes support more deployment styles
- they create more failure modes and more docs burden

### Why the recommendation wins

One engineer should be able to understand, run, and debug the system alone.

### Intentionally excluded before stable release

- multi-mode supervisors
- app-managed runtime attach flows
- write-heavy hot reload

## Final recommendation

Build the smaller product on purpose.

But make the small product legible and memorable.

The core product moment is not "a daemon that can call tools."

It is:

- a custom team of named agents
- working a task through explicit delegation
- with visible memory reads and writes
- under explicit token and cost budgets
- replayable after the fact as a graph
- inspectable locally through replay and receipts
- ending with a receipt that makes cost and behavior obvious
- carrying verification evidence when the task changes real code
- demonstrated through one starter workflow that makes the whole story click in minutes
- and annotated with grounded reasons for the most important decisions

Recommended starter workflow:

- Repo Task Team

First-run posture:

- preload `Repo Task Team` as the editable default experience
- guide the user through binding a real workspace before the first run
- propose a curated shortlist of preview-only first tasks based on repo signals
- make that shortlist a balanced trio:
  - explain a subsystem
  - review a diff or branch
  - find the next safe improvement
- let the user pick the first task instead of forcing one automatically
- make the first run preview-only on that real workspace
- make that first run return a concrete preview package:
  - summary
  - grounded reasons
  - proposed diff or change sketch when relevant
  - verification plan or evidence
  - receipt
  - replay path
- after a successful first preview, the primary next step CTA is to connect the same team to Telegram
- default that Telegram onboarding to a private DM first
- use one operator-facing agent alias in that DM, with delegation happening behind the scenes
- keep that DM quiet between meaningful state changes and use rare milestone checkpoints only
- milestone checkpoints are limited to started, blocked, approval needed, and finished
- once connected, the DM should feel task-first and free-form, not like a command shell
- allow bounded inline Telegram approvals for low and medium-risk actions
- keep high-risk approvals in the local web UI or CLI
- let the user customize the team before or after that first run
- do not open on a blank team composer
- do not ship a starter-team gallery before the core loop is proven

Why it wins:

- strongest fit for the solo self-hosting engineer target
- shows planning, research, building, and review clearly
- exercises delegation, tools, replay, receipts, and bounded autonomy in one coherent loop
- can feel like a real teammate once approval-gated apply mode is enabled
- matches how engineers actually decide whether to trust a change

Stop line:

- do not spend more architecture time on Telegram groups, replay sharing, multi-root apply, or other edge mechanics before implementation proves the need
- the stable-release core loop is already defined tightly enough to build

Keep:

- agent identity
- explicit delegation
- durable memory
- operator-authored soul
- structural tool safety

Delete:

- sprawling gateway
- in-process plugin trust
- split persistence
- hidden runtime magic
- raw-transcript-as-product thinking

If the first implementation does not feel obviously smaller than OpenClaw, the redesign failed.
