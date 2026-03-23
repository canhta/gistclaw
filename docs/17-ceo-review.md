# CEO Review Reflection

## Source of truth

This pass treats the approved `/office-hours` design as the product thesis and `docs/` as the implementation-facing mirror.

Implementation note:

- this document is intentionally expansion-mode
- use `19-buildable-v1-plan.md` and `16-roadmap-and-kill-list.md` for the locked build order

The thesis is no longer "make OpenClaw simpler."

It is:

- build a replayable low-burn agent team runtime
- keep custom team composition at stable release
- keep persistent memory at stable release
- keep the system calm when idle
- make trust visible through replay, budgets, and bounded autonomy

## Pre-review system audit

### Repo state

- Base branch: `main`
- Working tree vs `main`: clean
- Stash entries: none
- `TODOS.md`: absent

This matters because there is no hidden local implementation plan to honor. The redesign docs themselves are the plan.

### Recent churn signal

The last 30 days are concentrated in:

- `CHANGELOG.md`
- `package.json`
- `src/config/schema.help.ts`
- `docs/gateway/configuration-reference.md`
- `src/plugin-sdk/index.ts`
- `src/agents/pi-embedded-runner/run/attempt.ts`
- `src/plugins/loader.ts`

Judgment:

OpenClaw is still paying platform-kernel tax. The churn is broad, configuration-heavy, and runtime-surface heavy. That reinforces the redesign thesis instead of weakening it.

### Design scope detection

`DESIGN_SCOPE = yes`

The replay surface is part of the product, not just diagnostics. Any serious review has to judge the operator experience and inspect surface, not only backend structure.

## Taste calibration

### Patterns worth stealing

#### 1. Session key canonicalization is honest and useful

`src/routing/session-key.ts` is one of the best parts of the codebase.

Why it is strong:

- explicit agent-scoped keys
- explicit DM isolation modes
- boring normalization rules
- predictable string forms that are easy to inspect

This is the kind of abstraction that is real. It compresses a messy problem into a simple shape.

#### 2. Security audit treats access control as a first-class system concern

`src/security/audit.ts` is the right instinct.

Why it is strong:

- named findings
- explicit remediation guidance
- fail-closed thinking around auth, sandboxing, safe bins, and network posture

This is worth keeping in spirit, but with a smaller runtime.

#### 3. Inbound reply dispatch has a useful normalized seam

`src/plugin-sdk/inbound-reply-dispatch.ts` gets one thing right:

- record inbound session state first
- then dispatch through a normalized outbound path

That is the right kind of narrow seam between transport and runtime.

### Patterns to avoid repeating

#### 1. Gateway bootstrap has become a god function

`src/gateway/server.impl.ts:startGatewayServer` does too much:

- config migration
- config mutation
- secret activation
- auth bootstrap
- startup migrations
- plugin loading
- hook wiring
- runtime config derivation
- channel service setup

This is not a runtime entrypoint anymore. It is a platform control room.

#### 2. Outbound session routing drags channel-specific logic into core

`src/infra/outbound/outbound-session.ts` is a warning sign.

It contains provider and channel-specific parsing and routing logic for multiple transports inside one core infrastructure unit. That is exactly the kind of generic-looking file that quietly becomes impossible to shrink.

#### 3. Delegation complexity is duplicated

`src/agents/subagent-registry.ts` and `src/context-engine/delegate.ts` show the system carrying multiple delegation and compaction stories at once.

That is not flexibility. It is overlap.

## Step 0A: premise challenge

### Is this the right problem?

Mostly yes, but the framing needed correction.

The real problem is not "design a cleaner architecture than OpenClaw."

That framing is too inward-facing and too easy to get wrong. It produces a technical rewrite with no product magnetism.

The real problem is:

- deliver the feeling of a capable OpenClaw-style agent team
- without the operational heaviness and token burn
- while making trust and inspection the product surface

### What is the actual outcome?

The actual outcome is not architectural elegance.

It is a system where a solo operator can:

- define a custom team
- run meaningful multi-agent work
- inspect the result as a graph
- trust that the system is not silently wasting money

### What happens if we do nothing?

If we do nothing, the current redesign risks shipping as a technically respectable but emotionally flat rewrite.

That would be a mistake.

The system needs a visible product moment. Without that, "lighter OpenClaw" is respected infrastructure, not a category-defining product.

## Step 0B: existing code leverage

### What should be reused conceptually

- `src/routing/session-key.ts`
  - explicit agent and session scoping
- `src/security/audit.ts`
  - access-control-before-intelligence mindset
- `src/plugin-sdk/inbound-reply-dispatch.ts`
  - normalized record-then-dispatch seam
- `src/infra/outbound/session-binding-service.ts`
  - narrow conversation binding adapter, not a giant connector framework
- `src/gateway/server-methods/chat.ts`
  - usage and cost metadata are preserved intentionally for UI rendering
- `src/gateway/server-methods/usage.ts`
  - cost usage is queryable, not just buried in logs

### What should not be rebuilt

- OpenClaw's WS-heavy gateway control plane
- startup-time config mutation behavior
- session JSON plus transcript JSONL plus registry side-state
- dual delegation stories
- core files that know too much about individual channels

Judgment:

Reuse the good primitives as principles, not as a porting checklist.

## Step 0C: dream-state mapping

```text
CURRENT STATE                       THIS PLAN                            12-MONTH IDEAL
OpenClaw is a broad control-        A single-binary Go runtime          A replayable agent-team product
plane platform with strong          that keeps explicit agent           with custom teams, durable scoped
primitives but too much runtime     identity, durable memory, and       memory, strict budgets, and a
surface, too much persistence       structural safety while deleting    replay surface that makes the whole
fragmentation, and too much         the node mesh, split persistence,   system understandable in minutes.
token and ops ambiguity.            plugin host, and hidden magic.
```

Judgment:

The current `docs/` redesign is directionally right, but before this review it still read more like "a sharp runtime architecture" than "a sharp product."

That is now corrected.

## Step 0C-bis: implementation alternatives

### Approach A: Replayable Team Kernel

Summary:
Build the runtime around the replay graph as a first-class product surface. Multi-agent runs, memory actions, tool actions, approvals, and costs all produce durable events that can be replayed cleanly.

Effort: L
Risk: Medium

Pros:

- best match for the approved product thesis
- strongest viral and demo surface
- makes trust visible instead of rhetorical

Cons:

- forces product taste, not just runtime correctness
- replay can bloat if it becomes a generic observability framework

Reuses:

- session scoping lessons from `src/routing/session-key.ts`
- usage and cost lessons from `src/gateway/server-methods/chat.ts` and `src/gateway/server-methods/usage.ts`

### Approach B: Trusted Autopilot Kernel

Summary:
Build the same low-burn multi-agent runtime, but center the product on bounded autonomy, strict budgets, and low-HITL execution instead of replay.

Effort: L
Risk: Medium

Pros:

- strongest direct fit for the "less HITL" requirement
- excellent trust story for real operators
- reinforces safe-by-default execution

Cons:

- harder to explain in one screenshot
- weaker viral surface than replay-first
- easier to appreciate after adoption than before adoption

Reuses:

- security posture from `src/security/audit.ts`
- approvals and policy lessons from the existing sandbox and approval surfaces

### Approach C: Benchmark-first Thin Runtime

Summary:
Ship the minimum viable replacement that wins blatantly on RAM, idle token burn, and operator simplicity, with benchmarking built into the pitch.

Effort: M
Risk: Low

Pros:

- smallest diff from idea to usable proof
- forces discipline on idle behavior and cost accounting
- easiest way to justify switching

Cons:

- least emotionally magnetic
- risks becoming a utilitarian migration target instead of a product people talk about
- underplays multi-agent team identity

Reuses:

- existing session and usage accounting lessons
- existing operator pain as the comparison target

## Recommendation

Choose **Approach A: Replayable Team Kernel**.

Reason:

It is the only option that simultaneously satisfies:

- custom team identity
- lower hallucination through inspectability
- low-burn operator trust
- a shareable product moment

Approach C is the minimum viable path.
Approach A is the right public product.

## Expansion mode

The current plan is now the floor.

The review posture from here is:

- keep the single-binary low-burn kernel
- keep custom teams and replay as mandatory
- push product ambition up where it creates a clearer category and a stronger sharing loop

## 10x check

The 10x version is not "more agent autonomy."

It is:

- a custom agent team runtime
- where every serious run becomes a replayable artifact
- where teams themselves are forkable products
- and where the system proves, visibly, that it solved the task with bounded cost and bounded authority

That turns the product from "good local infrastructure" into "something people can watch, trust, compare, and share."

The key point is that replay, receipts, and team definitions are not support features. They are how the product spreads.

## Platonic ideal

The best version of this product feels like watching a disciplined small company work.

The operator picks or forks a team.

Each agent has a crisp role card:

- what it is for
- what it can touch
- what memory it can see
- when it must escalate

The operator drops in a task.

The team gets to work without noise:

- agents split the work
- agents gather evidence and produce changes or recommendations
- agents critique each other when the team graph allows it
- the operator-facing agent interrupts only when a real threshold is crossed

The replay is beautiful and obvious:

- delegation graph on the left
- timeline in the middle
- final outputs and artifacts on the right
- cost, latency, memory reads, memory writes, approvals, and tool calls all visible

The system never feels haunted.

When idle, it is quiet.
When active, it is legible.
When it fails, the reason has a name.

The user feeling is:

- "I understand what happened."
- "I can trust this team."
- "I can fork this setup for my own use."
- "I want to show this to someone."

## Delight opportunities

These are the highest-signal adjacent upgrades discovered in expansion mode.

1. Shareable replay bundles
   A finished run can be exported as a static artifact with graph, timeline, outputs, and receipts.
2. Forkable team blueprints
   Teams are first-class files users can copy, edit, version, and share.
3. Budget receipts everywhere
   Every run ends with an explicit receipt for tokens, dollars, wall-clock time, and idle burn impact.
4. Why-explanations inline
   Replay shows why an agent delegated, why a memory was loaded, and why approval was required.
5. Intervention at the edge, not the center
   Users can pause, approve, redirect, or re-run one node in the graph instead of restarting the whole task.
6. Quiet-by-default trust surface
   The product shows zero-background-work posture explicitly so "not burning money" is visible, not assumed.
7. Team diff and fork view
   Users can compare two team definitions and see role, tool, soul, and memory-scope differences directly.

## Expansion candidates

These are the leading scope-expansion proposals to evaluate one by one:

1. Shareable replay artifacts
2. Forkable team blueprints and starter packs
3. Node-level intervention and replay reruns
4. Built-in low-burn receipts and comparison view
5. One iconic end-to-end starter workflow that makes the team product instantly legible

## Scope decisions

### Accepted

#### 1. Shareable replay artifacts

Decision:

- accepted into the stable-release scope

Reason:

- this is the cleanest bridge from replay-as-debugging to replay-as-product
- it reuses core replay data instead of inventing a second presentation system
- it makes the system legible to people who were not present during the run

Implementation direction:

- polished redacted replay page first
- strict secret and private-memory redaction
- graph, timeline, outputs, and cost receipt included
- direct-link sharing first, with no built-in gallery or discovery layer in the stable release

### Deferred to later phase

#### 2. Forkable team blueprints and starter packs

Decision:

- deferred beyond the stable release

Reason:

- custom team definition remains mandatory
- but polished blueprint-pack ergonomics are not required to prove the core product
- this should come after the replayable team loop is already strong

Implementation direction later:

- starter team packs
- team diff and fork UX
- higher-level copy and sharing flow for team setups

### Accepted

#### 3. First-class run receipts and comparison

Decision:

- accepted into the stable-release scope

Reason:

- it directly proves the low-burn thesis that motivated the project
- it turns accounting into product surface instead of hidden telemetry
- it gives users a concrete way to compare teams, models, and runs without guesswork

Implementation direction:

- every completed run gets a receipt
- receipt includes token, cost, latency, approvals, and budget status
- stable release comparison stays narrow and legible

### Accepted

#### 4. One iconic end-to-end starter workflow

Decision:

- accepted into the stable-release scope

Reason:

- a strong runtime without a face is easy to admire and easy to forget
- one canonical workflow gives the product a demo, a teaching surface, and a memorable identity
- this raises product clarity without requiring a whole preset marketplace

Implementation direction:

- one polished workflow only, not a broad catalog
- it must show delegation, replay, receipts, and bounded autonomy clearly
- it should be useful enough to feel real, not synthetic

### Deferred to later phase

#### 5. Node-level intervention and replay reruns

Decision:

- deferred beyond the stable release

Reason:

- this is the first expansion that materially increases runtime and state complexity
- the stable release should prove the replayable team loop before it becomes an interactive control surface
- this is a strong later phase once operators already trust the core graph

Implementation direction later:

- pause one branch without freezing the whole run
- approve or redirect one node
- re-run one node or subtree with explicit lineage

### Accepted

#### 6. Grounded inline why-explanations

Decision:

- accepted into the stable-release scope

Reason:

- this is one of the cheapest ways to reduce suspicion and hallucination anxiety
- it makes replay explain itself instead of forcing users to infer intent from raw events
- it can be grounded in runtime policy and metadata rather than invented narrative

Implementation direction:

- start with delegation, memory loads, approvals, and budget stops
- explanations come from policy and event data, not hidden reasoning
- keep explanations short, factual, and inspectable

### Accepted

#### 7. One real external connector at stable release

Decision:

- accepted into the stable-release scope

Reason:

- one real connector makes the product feel alive outside the terminal
- exactly one connector preserves focus and avoids turning stable release into a transport matrix
- this is enough to prove the runtime is a real assistant system, not just a local demo

Implementation direction:

- choose one connector for clarity and operational simplicity
- keep the connector seam narrow
- push connector spread to later phases

Connector choice:

- Telegram

Why:

- best mix of demo clarity, mobile presence, self-hosted friendliness, and low setup friction
- better product surface than email for visible teamwork
- less organizational friction than Slack for a first stable release

### Accepted

#### 8. First-class local web UI

Decision:

- accepted into the stable-release scope

Reason:

- replay, receipts, quiet-state trust, and the starter workflow are much more legible in a visual surface than in CLI alone
- this preserves the single-binary local product shape without requiring a hosted control plane
- CLI remains valuable, but it should not be the only place the product feels obvious

Implementation direction:

- local-only web UI
- read-only by default
- centered on replay, receipts, quiet-state trust, and the hero workflow

### Accepted

#### 9. Starter workflow choice: Repo Task Team

Decision:

- accepted as the stable-release hero workflow

Reason:

- best fit for the target user: a strong self-hosting engineer
- clearest way to show visible multi-agent collaboration without collapsing into one black box
- exercises the exact surfaces the product wants to prove: delegation, tools, replay, receipts, and bounded autonomy

Implementation direction:

- agents decompose the repo task across explicit handoffs
- the team can gather evidence, produce a change or change proposal, and critique before final output

### Accepted

#### 10. Approval-gated apply mode for `Repo Task Team`

Decision:

- accepted into the stable-release scope

Reason:

- it delivers the "less HITL with bounded autonomy" thesis more honestly than proposal-only mode
- it keeps the human at the real safety boundary instead of forcing them to become the manual executor every time
- it makes the system feel like a teammate, not just an analyst

Implementation direction:

- always show a preview before apply
- limit apply to configured workspace roots
- require an explicit approval checkpoint before side effects
- record preview and apply as replayable events

### Accepted

#### 11. Verification loop with recorded evidence

Decision:

- accepted into the stable-release scope

Reason:

- engineers trust evidence more than assertions
- this reduces false confidence and bad patches in the exact workflow we chose as the hero product
- it makes receipts and replay materially more useful after a change is proposed or applied

Implementation direction:

- run the most relevant checks, not every possible check
- attach pass, fail, or skipped-with-reason evidence to replay and receipts
- make verification part of the normal handoff path for `Repo Task Team`

### Accepted

#### 12. Visual team composer backed by the same team format

Decision:

- accepted into the stable-release scope

Reason:

- user-defined teams are one of the core promises of the product
- a visual composer makes that promise feel real instead of purely technical
- backing it with the same file format preserves auditability and source control friendliness

Implementation direction:

- local web UI editor for agents, roles, soul, tool profile, memory scope, and delegation wiring
- no second hidden config model
- validate before save and write back to the canonical team files

### Accepted

#### 13. Memory inspector and editor for durable memory

Decision:

- accepted into the stable-release scope

Reason:

- persistent memory without an inspect and edit surface feels magical in the wrong way
- this makes memory a trustable product feature instead of a hidden subsystem
- it gives operators a clean way to correct or forget bad memory without database surgery

Implementation direction:

- local web UI list and detail views for durable facts
- show scope, source, provenance, confidence, and last-updated state
- support edit and forget on the same canonical memory store

### Accepted

#### 14. Bounded auto-promotion for durable memory

Decision:

- accepted into the stable-release scope

Reason:

- requiring approval for every durable memory write would add the wrong kind of HITL
- allowing arbitrary free-form model writes would rot the memory layer quickly
- bounded auto-promotion preserves autonomy while keeping memory narrow, inspectable, and cheap

Implementation direction:

- allow auto-promotion only for typed candidates
- require provenance, confidence, dedupe key, and explicit scope
- keep ambiguous or risky candidates as proposals or reject them
- let human edits and forget actions correct the canonical store directly

### Accepted

#### 15. Fixed agent- and phase-based model lanes

Decision:

- accepted into the stable-release scope

Reason:

- one model for everything is either too expensive or too weak
- a hidden dynamic auto-router would reintroduce mystery cost and debugging pain
- fixed agent- and phase-based lanes keep quality high where it matters and keep routine work cheap

Implementation direction:

- cheap default lane for routing, extraction, and routine executor work
- stronger lane for escalation, verification, synthesis, and other high-signal phases
- model-lane choice and escalation must be visible in replay and receipts

### Accepted

#### 16. Explicit handoff edges between agents

Decision:

- accepted into the stable-release scope

Reason:

- single-coordinator star topologies are easy to debug but too stiff for a real team product
- free-form peer chat is the fastest path back to noise, loops, and token burn
- explicit agent-to-agent edges preserve real collaboration while keeping the runtime inspectable

Implementation direction:

- team specs define allowed forward and return edges between agents
- critique loops must be explicit, not implied
- replay shows both the allowed graph and the actual handoff path taken

### Accepted

#### 17. Local-by-default durable memory with explicit team publish

Decision:

- accepted into the stable-release scope

Reason:

- team-shared memory by default would spread bad facts too quickly across the whole group
- no team-shared durable memory would weaken the point of building a team runtime
- local-by-default with explicit publish preserves collaboration while containing contamination

Implementation direction:

- newly promoted durable facts land in agent-local scope by default
- selected facts may publish into team scope through explicit rules
- replay and the memory inspector must show local versus team scope and publish history

### Accepted

#### 18. Event-driven and explicit-schedule automation only

Decision:

- accepted into the stable-release scope

Reason:

- always-on proactive agents would bring back mystery cost and make idle status harder to trust
- manual-only starts would cut too much of the autonomy story
- explicit triggers preserve useful automation while keeping the system calm and inspectable

Implementation direction:

- runs may start from inbound messages, webhooks, or declared schedules
- no free-roaming background agent loop
- replay and status surfaces must show why a run started and what trigger fired

### Accepted

#### 19. Telegram direct messages plus bounded groups

Decision:

- accepted into the stable-release scope

Reason:

- DM-only would leave too much virality and shared visibility on the table
- fully open group collaboration would raise routing, privacy, and memory-isolation risk too quickly
- bounded groups preserve the social product surface without blowing up the trust model

Implementation direction:

- support Telegram direct messages and bounded group chats
- require explicit trigger rules in groups
- keep conversation isolation and durable-memory isolation strict and inspectable

### Accepted

#### 20. Mention-to-start plus thread-bound continuation in groups

Decision:

- accepted into the stable-release scope

Reason:

- requiring a mention every single turn would make group use feel stiff
- ambient group listening would destroy the low-burn and trust story
- mention-to-start with thread-bound continuation keeps invocation explicit while preserving a coherent visible run

Implementation direction:

- a direct mention or explicit command starts the run
- the run continues only inside that same thread or scoped conversation until completion or timeout
- replay and status surfaces must show the exact trigger and conversation scope

### Accepted

#### 21. Compact completion card plus replay or receipt access in groups

Decision:

- accepted into the stable-release scope

Reason:

- plain final answers are too forgettable for the social product surface you want
- live internal narration would turn the group into noisy theater
- a compact completion card makes the result legible, trustable, and shareable without flooding the room

Implementation direction:

- group runs end with a concise result card
- the card includes outcome, receipt summary, and replay or artifact access
- detailed inspection stays opt-in outside the room

### Accepted

#### 22. Explicit export or share action from the completion card

Decision:

- accepted into the stable-release scope

Reason:

- private-only inspect paths would weaken the sharing loop too much
- automatic public sharing would create avoidable privacy and trust failures
- explicit export keeps sharing easy, deliberate, and compatible with strong redaction defaults

Implementation direction:

- the completion card exposes an explicit export or share action
- export produces a redacted static replay artifact or receipt bundle
- no finished run becomes public unless an operator intentionally shares it

### Accepted

#### 23. Redacted team card inside shared artifacts

Decision:

- accepted into the stable-release scope

Reason:

- outcome-only artifacts are weaker at teaching why the product is interesting
- full import-ready team bundles would drag stable release toward blueprint-pack scope too early
- a redacted team card shows how the team works without overcommitting the product surface

Implementation direction:

- shared artifacts include a compact team card
- the card shows roles, handoff graph, model lanes, tool posture, and memory posture
- the card omits secrets, private facts, and full import-ready team definitions

### Accepted

#### 24. Explicit baseline compare action

Decision:

- accepted into the stable-release scope

Reason:

- no comparison path would make it harder to prove when the team shape actually helped
- automatic baseline runs would quietly undermine the low-burn promise
- an explicit compare action gives the product a proof loop without turning every run into double spend

Implementation direction:

- replay and completion surfaces expose an explicit compare action
- comparison may target a single-agent or smaller-team baseline
- comparison runs only when an operator intentionally requests it

### Accepted

#### 25. Live replay in the local web UI only

Decision:

- accepted into the stable-release scope

Reason:

- post-run only replay would weaken the "watch the team work" moment too much
- live narration in external chat would create noise and cost in the wrong place
- local live replay keeps active work visible and memorable while preserving calm external surfaces

Implementation direction:

- the local web UI graph, timeline, and status views update live during active runs
- external chat surfaces remain concise and final-first
- live replay is driven by the same durable event stream used for post-run inspection

### Accepted

#### 26. One-click local draft from the redacted team card

Decision:

- accepted into the stable-release scope

Reason:

- manual rebuild from a shared artifact is too cold for the growth loop you want
- full import-ready bundles would reopen the bigger blueprint-pack scope too early
- a local draft from the redacted card gives users a concrete next step without pretending the artifact is a perfect transport format

Implementation direction:

- a shared artifact may generate a local draft team
- the draft preserves the visible role graph, model lanes, tool posture, and memory posture
- the draft still requires local review and completion before it can run

### Accepted

#### 27. One preloaded editable default team on first open

Decision:

- accepted into the stable-release scope

Reason:

- a blank canvas weakens the first-run "I get it" moment too much
- a starter gallery would push the product back toward template-catalog behavior too early
- one strong editable default team gives the fastest path to product understanding without sacrificing flexibility

Implementation direction:

- first open lands on a preloaded editable `Repo Task Team`
- users can run it immediately or customize it before the first run
- this is the default onboarding path, not a full starter-team marketplace

### Accepted

#### 28. Real-workspace bind wizard with preview-only first run

Decision:

- accepted into the stable-release scope

Reason:

- a synthetic demo would weaken the first proof too much
- manual setup before any real run would slow the product moment down
- a real-workspace bind with preview-only execution gives a credible first win without asking for irreversible trust too early

Implementation direction:

- first-run onboarding binds `Repo Task Team` to a real workspace
- the first guided run is preview-only
- apply stays behind a later explicit approval step

### Accepted

#### 29. Balanced first-run trio shortlist

Decision:

- accepted into the stable-release scope

Reason:

- explain-only onboarding would undersell the collaborative nature of the team
- a larger adaptive menu would make first-run onboarding noisier than it needs to be
- a balanced trio shows the product's range without turning the first-run choice into a task marketplace

Implementation direction:

- after workspace bind, the first-run shortlist includes:
  - explain a subsystem
  - review a diff or branch
  - find the next safe improvement
- the user chooses one of those as the first preview-only run

### Accepted

#### 30. Curated first-run task shortlist from repo signals

Decision:

- accepted into the stable-release scope

Reason:

- a blank task prompt would make the user invent their own first win
- automatic task selection would feel arbitrary too often
- a curated shortlist keeps the first run grounded, concrete, and user-chosen

Implementation direction:

- after workspace bind, the product proposes a few preview-only first tasks from repo signals
- the user picks one task for the first run
- the runtime does not auto-select the first task silently

### Accepted

#### 31. Conservative default budget caps with explicit override

Decision:

- accepted into the stable-release scope

Reason:

- observe-only budgets would still allow the first bad surprise to happen
- manual budget setup before first use would add friction at exactly the wrong moment
- conservative defaults make the low-burn promise real on day one without blocking onboarding

Implementation direction:

- stable release enables sensible per-run and daily caps by default
- operators may raise or disable them intentionally later
- budget denials and overrides must be visible in receipts, replay, and status surfaces

### Accepted

#### 32. Concrete first-run preview package

Decision:

- accepted into the stable-release scope

Reason:

- analysis-only first runs would feel too abstract for a real workspace onboarding moment
- apply-ready patch packages would be too aggressive for the first trust-building interaction
- a concrete preview package proves capability without crossing the safety boundary

Implementation direction:

- the first preview-only run returns:
  - summary
  - grounded reasons
  - proposed diff or change sketch when relevant
  - verification plan or evidence
  - receipt
  - replay path
- the package must feel like real work, not just commentary

### Accepted

#### 33. Connect Telegram as the default next step after first preview

Decision:

- accepted into the stable-release scope

Reason:

- pushing users straight into team customization would keep the product in setup mode too long
- sharing first is good for spread but weaker than real repeated use
- connecting Telegram right after the first local proof turns the team into part of a real workflow

Implementation direction:

- after a successful first preview, the primary CTA is to connect the same team to Telegram
- this becomes the default bridge from local proof to ongoing use

### Accepted

#### 34. Telegram DM first as the default onboarding surface

Decision:

- accepted into the stable-release scope

Reason:

- bounded groups are better for visibility, but they add friction and trust complexity too early
- private DM is the cleanest path from local proof to repeated real use
- groups stay important, but they should come after the user already trusts the team

Implementation direction:

- post-preview connector onboarding defaults to Telegram DM
- bounded groups come after DM habit is established

### Accepted

#### 35. One operator-facing agent alias in Telegram DM

Decision:

- accepted into the stable-release scope

Reason:

- exposing all agents directly in DM would make daily use noisier and less natural
- a command-shell-only DM would undershoot the conversational habit we want
- one clear operator-facing agent alias keeps the chat surface simple while preserving the real team underneath

Implementation direction:

- Telegram DM presents one operator-facing agent alias
- that agent delegates to the team behind the scenes
- replay and receipts show the full multi-agent graph

### Accepted

#### 36. Rare milestone checkpoints in Telegram DM

Decision:

- accepted into the stable-release scope

Reason:

- final-only DM updates would feel too silent for an ongoing teammate surface
- frequent narration would quickly become noisy and cheapen the experience
- rare milestone checkpoints keep the DM alive without turning it into status spam

Implementation direction:

- the operator-facing agent sends checkpoint updates only for started, blocked, approval needed, and finished
- internal step-by-step narration stays in replay, not in Telegram DM
- quiet between meaningful state changes is the default

### Accepted

#### 37. Free-form task-first Telegram DM

Decision:

- accepted into the stable-release scope

Reason:

- the DM should feel like talking to a capable teammate, not operating a bot control panel
- quick-action gating would add friction right where habit formation should feel easiest
- free-form first keeps the surface flexible enough for custom teams without inventing a command language

Implementation direction:

- after Telegram is connected, the user can type a task immediately
- the system may suggest examples, but it does not require commands or menu-first interaction
- delegation and team structure stay visible in replay and receipts, not in the DM input model

### Accepted

#### 38. Bounded inline Telegram approvals

Decision:

- accepted into the stable-release scope

Reason:

- forcing every approval back into the local app would add too much HITL friction to the daily loop
- putting every approval in chat would widen the blast radius too far
- bounded inline approvals keep routine work moving while preserving a higher-trust surface for risky actions

Implementation direction:

- Telegram DM can resolve low and medium-risk approvals inline
- high-risk approvals stay in the local web UI or CLI
- approval cards in Telegram include a compact action summary plus replay or preview access

### Accepted

#### 39. Risk-split approvals for Telegram groups

Decision:

- accepted into the stable-release scope

Reason:

- forcing every group approval into DM would create unnecessary friction for lightweight actions
- leaving medium or high-risk approval details inside the group would create privacy and trust problems fast
- a risk-split group policy keeps the group useful without turning it into a risky control surface

Implementation direction:

- low-risk approvals may resolve in the bound group thread
- medium-risk approvals move to the operator's private DM
- high-risk approvals stay in the local web UI or CLI
- the group sees a neutral status note when approval is moved off-thread

### Accepted

#### 40. Role-typed custom teams

Decision:

- accepted into the stable-release scope

Reason:

- fully arbitrary agent graphs would push the product back toward framework sprawl and make debugging much harder
- preset teams alone would undershoot the core promise of flexible team creation
- bounded capability contracts keep user freedom high while keeping the runtime legible, testable, and cheaper to operate

Implementation direction:

- stable release ships one editable starter team plus a bounded capability model
- users can compose, duplicate, rename, and wire agents into whatever team they want
- each agent instance can override soul, tool profile, memory scope, and budget posture without becoming a new runtime primitive

### Accepted

#### 41. Structured soul cards plus short notes

Decision:

- accepted into the stable-release scope

Reason:

- fully raw prompt editing would make agent behavior harder to compare, validate, and debug
- locked presets would make the custom-team promise feel weaker than it should
- structured soul cards keep the behavior model clear while still leaving room for human nuance

Implementation direction:

- each agent edits typed soul fields such as role, tone, posture, collaboration style, escalation rules, decision boundaries, tool posture, and prohibitions
- each agent also gets a short notes field for nuance
- raw full-prompt soul editing is not the primary or default stable-release surface

### Accepted

#### 42. Tool packs plus narrow overrides

Decision:

- accepted into the stable-release scope

Reason:

- fully per-tool wiring would make team setup too fiddly and safety-heavy for normal users
- locked presets with no override path would make custom teams feel fake
- tool packs plus narrow overrides keep setup fast, understandable, and still genuinely flexible

Implementation direction:

- stable release ships a small built-in tool-profile catalog such as read-only, read-heavy, workspace-write, operator-facing, and elevated
- each agent starts from one tool pack
- users may add a few explicit per-tool overrides without redefining the whole policy model

### Accepted

#### 43. Polished redacted replay pages

Decision:

- accepted into the stable-release scope

Reason:

- raw export files are useful but too cold to become the product's main social object
- private-only replay would waste one of the strongest growth surfaces in the design
- a polished redacted replay page makes the work legible, shareable, and remixable without exposing unsafe detail

Implementation direction:

- stable release export produces a polished redacted replay page by default
- the page shows outcome, replay highlights, receipt summary, and a redacted team card
- recipients can generate a one-click local draft from the redacted team card

### Accepted

#### 44. Direct-link share pages only

Decision:

- accepted into the stable-release scope

Reason:

- a built-in gallery or discovery layer would drag stable release into hosting, moderation, and platform complexity too early
- private-only replay would waste too much of the product's natural spread
- direct-link sharing keeps the social object strong while preserving a compact self-hosted system

Implementation direction:

- stable release supports direct-link sharing of polished redacted replay pages
- users distribute those links through existing channels rather than an in-product discovery feed
- replay discovery or public galleries are later-phase expansion, not stable-release scope

### Accepted

#### 45. Explicit GitHub publish-back action

Decision:

- accepted into the stable-release scope

Reason:

- replay pages are stronger when they can land back inside the engineering discussion they came from
- manual copy-paste would work, but it weakens the habit loop and makes the product feel less native to repo work
- a full GitHub app would add too much platform scope too early

Implementation direction:

- repo-task runs may publish a compact result summary plus replay-page link to a PR, issue, or commit discussion
- publish-back starts only from an explicit operator action with a rendered preview
- stable release does not require an always-on GitHub app, inbound webhook flow, or ambient repo bot

### Accepted

#### 7. Quiet-by-default trust surface

Decision:

- accepted into the stable-release scope

Reason:

- the original problem was not just cost, it was mystery cost
- this makes low-burn behavior visible at a glance instead of forcing users to inspect logs
- it turns "the system is calm" into a product truth the operator can verify

Implementation direction:

- status view in CLI and admin UI
- show active runs, pending approvals, scheduled work, and model-call activity
- say explicitly when the system is idle and making zero background model calls

## CEO judgment

### What the plan should do

- make replay the product, not just observability
- make custom teams mandatory in the stable release
- make budgets part of runtime policy, not a dashboard garnish
- keep memory disciplined and visible
- keep the runtime single-binary and anti-platform

### What the plan should not do

- drift back toward OpenClaw's generic control-plane ambitions
- treat plugin loading as the answer to flexibility
- ship a single-agent first release and promise teams later
- confuse transcript browsing with replay

## Reflections written back into `docs/`

This pass required updating the design docs themselves:

- `docs/11-architecture-redesign.md`
  - replay and low-burn product target made explicit
- `docs/12-go-package-structure.md`
  - added a dedicated replay projection package
- `docs/13-core-interfaces.md`
  - added replay and budget interfaces
- `docs/16-roadmap-and-kill-list.md`
  - clarified that single-agent is an internal phase, not the stable release

## Next decision

The approved implementation approach remains **Approach A: Replayable Team Kernel** unless explicitly changed.

The next CEO-level choice is not the approach.

It is the review mode:

- push scope up aggressively
- cherry-pick expansion opportunities
- hold scope and bulletproof it
- or cut scope ruthlessly
