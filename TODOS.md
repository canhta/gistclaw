# TODOS

This file tracks deferred work from the CEO and engineering reviews.
Items here were explicitly considered and deferred — not forgotten.

---

## P2: Design system — run /design-consultation before Task 8

**What:** Run `/design-consultation` to produce `DESIGN.md` — a complete design system
covering aesthetic stance, typography scale with weights, full color system with semantic
tokens, component vocabulary, and motion/animation principles.

**Why:** `docs/designs/ui-design-spec.md` provides minimal tokens (font stack, color
palette, spacing scale) but leaves micro-decisions (button hover states, focus ring style,
form field design, loading animation specifics) to the implementer. Without DESIGN.md,
these decisions will be made inconsistently across 8+ templates.

**How to apply:** Run `/design-consultation` before implementing Task 8 templates.
The output DESIGN.md becomes the calibration source for all template work and the
subsequent `/design-review` pass.

**Effort:** M → CC+gstack: M (design consultation is architecture-level work)
**Priority:** P2
**Depends on:** None (can run before Task 8)

---

## P2: Visual QA — run /design-review after Task 8

**What:** Run `/design-review` after the Task 8 templates are implemented to verify:
the built UI matches `docs/designs/ui-design-spec.md`, spacing and color tokens are
applied consistently, interaction states (loading, empty, error) are present in rendered
HTML, and keyboard navigation works as specified.

**Why:** The design spec is detailed, but implementation drift is inevitable without
a formal visual QA gate. A /design-review catches: missing empty states, inconsistent
spacing, wrong colors, absent ARIA landmarks, and broken focus management.

**How to apply:** Run `/design-review` after `go test ./internal/web` passes and the
templates are rendered with real data. This gate must pass before Milestone 2 exits.

**Effort:** S → CC+gstack: S (automated visual scan + fixes)
**Priority:** P2
**Depends on:** Task 8 (templates built and running locally)

---

## P2: Approval material-change fingerprint spec

**What:** Define the exact fingerprint formula for snapshot-bound approval tickets.
Currently specified as: `sha256(tool_name + normalized_args_json + target_path)`.
Needs a canonical implementation and cross-reference in the approval tests.

**Why:** Without a precise definition, two implementers could produce incompatible
fingerprints, breaking approval expiry logic.

**How to apply:** Lock the formula before Task 6 is implemented. Add it to
`docs/13-core-interfaces.md` under the approval gate interface.

**Effort:** S → CC+gstack: S
**Priority:** P2
**Depends on:** Task 6

---

## P2: SSE live-replay event schema + reconnect protocol

**What:** Define the canonical SSE event payload format for live replay updates.
The plan specifies SSE (not WebSockets) but does not specify the event schema:
what fields, what event types, how the client reconstructs the graph incrementally.

Also define the reconnect protocol: clients must send `Last-Event-ID` on reconnect,
and the server must replay missed events from that point. This is standard SSE behavior
but must be explicitly implemented and tested — it is the recovery path when compaction
causes SSE clients to fall behind during heavy runs.

**Why:** Without a canonical schema, the SSE client and server will diverge.
Incremental graph rendering requires knowing the event type before the run finishes.
Without Last-Event-ID handling, clients miss events silently during compaction cycles.

**How to apply:** Define the schema before Task 8 is implemented. Add to
`docs/13-core-interfaces.md` or a new `docs/21-sse-event-schema.md`.
Include: event types, payload fields, `id:` field format, and reconnect behavior.

**Effort:** S → CC+gstack: S
**Priority:** P2
**Depends on:** Task 8

---

## P3: Telegram webhook support

**What:** Add optional webhook mode for Telegram DM as an alternative to long polling.
Long polling is the stable-release default. Webhook is a later-phase option for
operators who want faster response times and have public HTTPS available.

**Why:** Long polling requires the daemon to be always online and polling.
Webhook mode is more efficient and responsive but needs public HTTPS.

**How to apply:** Implement after Milestone 3 (public beta) is stable.
Add webhook config option alongside the existing polling config.

**Effort:** S → CC+gstack: S
**Priority:** P3
**Depends on:** Task 11 (long polling first)

---

## P3: Config default path convention

**What:** The default config path is unspecified. Candidate options:
- `~/.config/gistclaw/config.yaml` (XDG standard, recommended)
- `./gistclaw.yaml` (project-local)
- `/etc/gistclaw/config.yaml` (system-wide, for server deployments)

**Why:** All three are reasonable; inconsistency creates confusion in docs and tests.

**How to apply:** Pick one before Task 1 is implemented. XDG path is recommended
for a local-first tool that may be installed alongside other CLI tools.

**Effort:** S → CC+gstack: S
**Priority:** P3
**Depends on:** Task 1

---

## P3: Upgrade and migration path

**What:** The plan has no task for the binary upgrade experience:
- How does an operator upgrade from v0.x to a newer version?
- What happens to the SQLite schema on upgrade (new migrations)?
- Does the doctor command check for schema version compatibility?

**Why:** The first stable release is not the last release. An upgrade story prevents
future pain and makes the doctor command more useful.

**How to apply:** Add to Task 12 (hardening): schema version check in doctor,
migration runner is idempotent on upgrade, interrupts in-flight runs before
running new migrations.

**Effort:** S → CC+gstack: S
**Priority:** P3
**Depends on:** Task 12

---

## P4: Replay sharing and redacted export (Phase 5)

**What:** CEO review (docs/17) items 1, 43, 44: shareable replay pages,
polished redacted replay artifacts, direct-link sharing, GitHub publish-back.

**Why:** These extend the replay surface beyond local inspection into a social/viral
product loop. They were accepted in the CEO review but explicitly deferred from
stable 1.0 in the buildable plan.

**How to apply:** Build after stable 1.0 proves the local replay loop. Do not
add a sharing package until local replay is solid enough to be worth sharing.

**Effort:** L → CC+gstack: M
**Priority:** P4
**Depends on:** Stable 1.0 shipped

---

## P4: Node-level intervention and replay reruns (Phase 5)

**What:** CEO review (docs/17) item 5: pause one branch, approve or redirect one node,
re-run one node or subtree with explicit lineage.

**Why:** Powerful but materially increases runtime and state complexity. Deferred
until operators already trust the core graph.

**Effort:** L → CC+gstack: M
**Priority:** P4
**Depends on:** Stable 1.0 + local replay solid

---

## P3: Team spec snapshot query surface

**What:** Add `gistclaw inspect team <run_id>` — show the exact team spec (YAML) that was
frozen in the execution snapshot for a given run.

**Why:** When an operator asks "why did this run delegate to agent B instead of agent C?",
the replay graph shows what happened but not what team config was active. Direct snapshot
inspection closes the remaining gap in post-incident debugging.

**How to apply:** Implement after Task 5 (execution snapshot storage). Read from the
`execution_snapshots` table added in Task 5 Step 3. Output: raw YAML of the frozen team spec
plus the snapshot timestamp and run_id.

**Effort:** S → CC+gstack: S
**Priority:** P3
**Depends on:** Task 5 Step 3 (snapshot storage)

---

## P3: Provider error conformance test suite

**What:** Add a `ProviderConformanceTest` suite in `internal/runtime/provider_test.go`
(or later `internal/providers/`) that any new provider adapter must pass:
- All five error classes (RateLimit, ContextExceeded, Refusal, Timeout, MalformedResponse)
  produce correctly typed `*ProviderError` with the right `Code` field.
- The run engine receives only `*ProviderError`, never raw vendor error types.

**Why:** After the `ProviderError` typed Code decision (eng review Issue 3), every new
adapter must translate vendor errors correctly. Without a shared conformance suite,
the second and third adapter will silently break run-engine error handling.

**How to apply:** Implement when a second provider adapter is added. The test harness
should accept any `Provider` implementation and exercise all five error paths via mock.

**Effort:** S → CC+gstack: S
**Priority:** P3
**Depends on:** Task 4 (ProviderError type), plus second provider adapter

---

## P4: Forkable team blueprints and starter packs (Phase 5)

**What:** CEO review (docs/17) item 2: team diff and fork UX, higher-level copy
and sharing flow for team setups, starter team packs beyond the default one.

**Why:** Custom team definition is in stable 1.0. Polished blueprint-pack ergonomics
should come after the replayable team loop is proven.

**Effort:** M → CC+gstack: S
**Priority:** P4
**Depends on:** Stable 1.0 + visual composer shipped
