> **Docs:** [README](../../README.md) | [implementation-plan.md](../../implementation-plan.md) | [dependencies.md](../../dependencies.md) | [12-go-package-structure.md](../../12-go-package-structure.md) | [13-core-interfaces.md](../../13-core-interfaces.md)

# GistClaw -- Milestone 3: Public Beta

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the Public Beta: a structured soul editor, a button-based visual team composer, a memory inspector, a Telegram DM connector with durable outbound delivery, daily rolling budget caps, and grounded why-cards attached to delegation and approval events -- all while preserving the single canonical `teams/` YAML as the only config model and keeping the scheduler table present but inert.

**Architecture:** Milestone 3 builds on a complete Milestone 2 (local beta: web UI, SSE, onboarding, approval flow). New packages: `internal/telegram` (long-poll only, DM only, low-risk approvals only). New routes: `/teams`, `/teams/{id}/soul`, `/teams/{id}/composer`, `/memory`. New migration: `003_scheduler.sql` (adds `schedules` table, scheduler NOT active). Budget cap enforced as a rolling 24-hour window by the run engine's existing `BudgetGuard`. Grounded why-cards stored as journal entries and rendered in the replay timeline. Memory scope escalation remains the run engine's responsibility -- the memory package stores whatever scope value it receives.

**Tech Stack:** Go 1.24+, `modernc.org/sqlite` (pure-Go SQLite, no CGO), stdlib `net/http`, Go `text/template`, Telegram Bot API (HTTP long polling via `getUpdates`, 30-second timeout), `go test ./...`

---

## Global Guardrails (Milestone 3)

1. No Telegram groups -- DM only
2. No replay sharing or export
3. No GitHub publish-back
4. No compare view requiring bespoke UI
5. No node-level intervention
6. No drag-and-drop node positioning in visual composer -- button-based wiring only
7. No raw full-prompt editor in soul editor -- typed fields only
8. Visual composer writes only to canonical `teams/` YAML -- no second config model
9. Telegram long polling only -- no webhooks
10. `schedules` table added in `003_scheduler.sql` but scheduler NOT active
11. Memory scope escalation authorized by run engine, not by memory package
12. No `internal/agents` package split -- team spec loading stays in the runtime or app layer that currently owns it
13. No `internal/runtime` importing `internal/web` (verify with `go list -deps`)

---

### Task 10: Team editor, soul editor, visual composer

**Files:**
- Create: `internal/web/routes_team.go` -- GET/POST /teams, GET/POST /teams/{id}/soul, GET/POST /teams/{id}/composer
- Create: `internal/web/templates/team.html`
- Create: `internal/web/templates/team_composer.html`
- Test: `internal/web/routes_team_test.go`

- [ ] **Step 1: Write failing tests for the soul editor**

  Write tests in `internal/web/routes_team_test.go` that cover: GET /teams/{id}/soul renders all typed fields (role, tone, posture, collaboration style, escalation rules, decision boundaries, tool posture, prohibitions, notes) without a raw prompt textarea; POST /teams/{id}/soul with a single typed field update persists only that field and leaves others unchanged; POST with an empty required field returns HTTP 422 with an inline error message; the persisted team YAML does not grow a `raw_prompt` key after a soul edit.

  Run: `go test ./internal/web -run 'TestSoulEditor' 2>&1 | head -30`
  Expected: compilation errors or test failures because `routes_team.go` does not yet exist.

- [ ] **Step 2: Write failing tests for the visual team composer**

  In the same test file add tests that cover: GET /teams/{id}/composer renders agent nodes and handoff edges that match the current `teams/{id}.yaml` on disk; POST /teams/{id}/composer with a new agent instance writes the agent into the YAML and does not create a second config file; POST wiring a handoff edge between two existing agents writes the edge into the YAML; POST with an unknown capability flag returns HTTP 422 and leaves the YAML unchanged; POST with a circular handoff chain returns HTTP 422; the YAML on disk after a valid POST passes the team loader's own Validate method.

  Run: `go test ./internal/web -run 'TestComposer' 2>&1 | head -30`
  Expected: compilation errors or test failures.

- [ ] **Step 3: Implement routes_team.go -- GET /teams and GET /teams/{id}/soul**

  Register a `GET /teams` handler that loads every YAML under `teams/`, builds a list of team names and agent counts, and executes `team.html`. Register a `GET /teams/{id}/soul` handler that loads `teams/{id}.yaml`, extracts each typed soul field into a struct, and executes `team.html` with the struct bound to the template. The handler must not expose a raw full-prompt field at any path.

  Run: `go build ./internal/web`
  Expected: compiles without error.

- [ ] **Step 4: Implement routes_team.go -- POST /teams/{id}/soul**

  The POST handler reads exactly the typed fields from the form (role, tone, posture, collaboration style, escalation rules, decision boundaries, tool posture, prohibitions, notes). It validates that no required field is blank. It loads the existing YAML, mutates only the submitted field (field-at-a-time, not full-document replace), writes back to `teams/{id}.yaml`, and redirects to GET with a success flash. On validation failure it re-renders the form with an inline error at the failing field -- no raw prompt field is added under any circumstance.

  Run: `go build ./internal/web`
  Expected: compiles without error.

- [ ] **Step 5: Implement routes_team.go -- GET and POST /teams/{id}/composer**

  GET loads `teams/{id}.yaml`, constructs a list of agent nodes (id, name, capabilities) and handoff edges (from agent, to agent, condition), and executes `team_composer.html`. The template renders each agent as a labelled rectangle and each handoff as a directional arrow label -- button controls below each agent handle add-handoff, remove-handoff, and remove-agent; no drag-and-drop positioning.

  POST accepts a form payload describing a single mutation: add agent, remove agent, wire handoff, remove handoff. The handler validates the mutation (unknown capability flag returns 422; circular chain returns 422), calls the team loader's Validate on the proposed YAML, then writes the YAML to disk only if Validate passes. Validation errors are shown inline in the composer view, YAML is not written.

  Run: `go build ./internal/web`
  Expected: compiles without error.

- [ ] **Step 6: Build team.html and team_composer.html templates**

  `team.html` renders the soul editor form following `docs/designs/ui-design-spec.md`: 0px border radius, 1.5px solid #1c1917 borders on inputs and the form container, no shadow, JetBrains Mono for agent IDs and field labels that carry metadata weight, weight 700 for section headings. Each typed field is a labelled textarea or select -- no raw prompt textarea at any path.

  `team_composer.html` renders each agent node as a rectangle labelled with agent name and capability list. Handoff edges are shown as a table of (from, to, condition) rows below the graph. Add-agent, add-handoff, remove buttons are plain HTML form submits -- no JavaScript drag events. Design tokens from `docs/designs/ui-design-spec.md` apply identically to the composer view.

  Run: `go build ./internal/web`
  Expected: compiles without error.

- [ ] **Step 7: Run team tests**

  Run: `go test ./internal/web -run 'TestSoulEditor|TestComposer' -v`
  Expected: all tests PASS.

- [ ] **Step 8: Commit**

  Stage `internal/web/routes_team.go`, `internal/web/templates/team.html`, `internal/web/templates/team_composer.html`, `internal/web/routes_team_test.go`.

  Run: `git add internal/web/routes_team.go internal/web/templates/team.html internal/web/templates/team_composer.html internal/web/routes_team_test.go && git commit -m "feat(m3): soul editor and visual team composer"`
  Expected: commit created, hook passes.

---

### Task 11: Memory inspector

**Files:**
- Create: `internal/web/routes_memory.go` -- GET /memory, POST /memory/{id}/forget, POST /memory/{id}/edit
- Create: `internal/web/templates/memory.html`
- Modify: `internal/memory/store.go` -- add Edit, Forget, Filter methods
- Modify: `internal/memory/promote.go` -- expose scope escalation gate (called by run engine, not by memory store)
- Test: `internal/web/routes_memory_test.go`
- Test: `internal/memory/memory_inspector_test.go`

- [ ] **Step 1: Write failing tests for memory store primitives**

  Write tests in `internal/memory/memory_inspector_test.go` that cover: Forget marks a fact as forgotten and the fact is excluded from subsequent List results; Edit updates a fact's value and the new value appears in List; a human edit applied after a model-promoted fact causes the human value to take precedence (human outranks model); Filter with scope=local returns only local-scoped facts; Filter with scope=team returns only team-scoped facts; Filter with agent_id returns only facts whose provenance names that agent; a newly written fact carries the scope value passed to WriteFact verbatim -- the store does not reclassify it.

  Run: `go test ./internal/memory -run 'TestInspector' 2>&1 | head -30`
  Expected: compilation errors or test failures because Edit, Forget, and Filter are not yet implemented.

- [ ] **Step 2: Write failing tests for memory inspector routes**

  Write tests in `internal/web/routes_memory_test.go` that cover: GET /memory with no filter params returns all non-forgotten facts with scope and provenance visible; GET /memory?scope=local returns only local-scoped facts; GET /memory?agent_id=X returns only facts from agent X; POST /memory/{id}/forget with confirmation param set marks the fact forgotten and redirects to GET /memory; POST /memory/{id}/forget without confirmation param re-renders a confirmation prompt and does not forget the fact; POST /memory/{id}/edit with a new value updates the fact and redirects.

  Run: `go test ./internal/web -run 'TestMemoryInspector' 2>&1 | head -30`
  Expected: compilation errors or test failures.

- [ ] **Step 3: Implement Edit, Forget, and Filter in internal/memory/store.go**

  Forget sets a `forgotten_at` timestamp on the fact row and excludes forgotten facts from all List calls. Edit writes a new value to the fact row and records `source=human` in the fact's provenance column, replacing whatever source was stored previously. Filter accepts a struct with optional scope and agent_id fields; when a field is set it is added as a WHERE clause; when a field is unset no constraint is added for that column. The scope escalation gate lives in `promote.go` and is called by the run engine -- the store does not call it.

  Run: `go build ./internal/memory`
  Expected: compiles without error.

- [ ] **Step 4: Expose scope escalation gate in internal/memory/promote.go**

  Add a exported function (e.g. `AuthorizeEscalation`) that accepts the current run's authorization context and the proposed target scope, and returns an error if escalation is not permitted. The run engine calls this before invoking WriteFact with a higher scope. The memory store does not call it; it stores whatever scope it receives.

  Run: `go build ./internal/memory`
  Expected: compiles without error.

- [ ] **Step 5: Implement routes_memory.go**

  GET /memory reads filter params `scope` and `agent_id` from the query string, calls Filter with those values, and executes `memory.html` with the resulting fact list. Each fact row shows: fact ID, value, scope, provenance agent, created timestamp. Forgotten facts are excluded from the list entirely.

  POST /memory/{id}/forget checks for a `confirm=yes` form field. Without it the handler re-renders a confirmation view showing the fact value and a submit button. With it the handler calls Forget and redirects to GET /memory with a success flash.

  POST /memory/{id}/edit reads the `value` field from the form, calls Edit, and redirects to GET /memory.

  Run: `go build ./internal/web`
  Expected: compiles without error.

- [ ] **Step 6: Build memory.html template**

  `memory.html` renders a filter bar (scope dropdown, agent_id text input, a plain submit button) and a table of facts. Each row shows fact ID in JetBrains Mono, value, scope badge, provenance agent, timestamp in JetBrains Mono. A forget button per row posts to POST /memory/{id}/forget without `confirm=yes`, triggering the confirmation view. An edit button opens an inline form (no modal, just a replacement row with a textarea and a save button). Design tokens from `docs/designs/ui-design-spec.md` apply: 0px border radius, 1.5px solid #1c1917 borders, no shadow, weight 700 for column headers.

  Run: `go build ./internal/web`
  Expected: compiles without error.

- [ ] **Step 7: Run memory tests**

  Run: `go test ./internal/memory -run 'TestInspector' -v`
  Expected: all tests PASS.

  Run: `go test ./internal/web -run 'TestMemoryInspector' -v`
  Expected: all tests PASS.

- [ ] **Step 8: Commit**

  Stage all new and modified memory files.

  Run: `git add internal/memory/store.go internal/memory/promote.go internal/memory/memory_inspector_test.go internal/web/routes_memory.go internal/web/templates/memory.html internal/web/routes_memory_test.go && git commit -m "feat(m3): memory inspector with edit, forget, and filter"`
  Expected: commit created, hook passes.

---

### Task 12: Daily rolling budget cap and grounded why-cards

**Files:**
- Modify: `internal/runtime/budget.go` -- rolling 24-hour window enforcement, budget-stop journal event
- Modify: `internal/web/routes_runs.go` -- surface budget-remaining in run detail view
- Modify: `internal/web/routes_settings.go` -- add daily budget cap field to settings page
- Modify: `internal/journal/events.go` -- add BudgetStop event type, WhyCard payload struct
- Modify: `internal/replay/replay.go` -- render why-cards in timeline for delegation, approval, memory load, and budget stop events
- Modify: `internal/web/templates/run_detail.html` -- show budget-remaining line
- Modify: `internal/web/templates/settings.html` -- add daily budget cap input
- Test: `internal/runtime/budget_test.go`
- Test: `internal/replay/replay_test.go`

- [ ] **Step 1: Write failing tests for rolling budget cap**

  Write tests in `internal/runtime/budget_test.go` that cover: a run within the daily cap proceeds without a budget-stop event; a run that would exceed the rolling 24-hour cap is halted and a `BudgetStop` journal event is appended; the rolling window counts only spend from the last 24 hours -- spend from 25 hours ago is excluded; the cap is read from settings and a settings update takes effect on the next cap check without restart; the `BudgetStop` event carries a `WhyCard` with a plain-prose reason field that is not empty.

  Run: `go test ./internal/runtime -run 'TestBudgetCap' 2>&1 | head -30`
  Expected: compilation errors or test failures.

- [ ] **Step 2: Write failing tests for why-cards in replay**

  Write tests in `internal/replay/replay_test.go` that cover: a `Delegation` event with a `WhyCard` renders the why-card reason text in the timeline entry; an `ApprovalNeeded` event with a `WhyCard` renders the why-card reason; a `MemoryLoad` event with a `WhyCard` renders the why-card reason; a `BudgetStop` event with a `WhyCard` renders the why-card reason; events without a `WhyCard` render without a why-card section (no empty container).

  Run: `go test ./internal/replay -run 'TestWhyCard' 2>&1 | head -30`
  Expected: compilation errors or test failures.

- [ ] **Step 3: Add BudgetStop event type and WhyCard struct to journal/events.go**

  Add a `BudgetStop` event type constant. Add a `WhyCard` struct with a single plain-prose `Reason` field. Update `Delegation`, `ApprovalNeeded`, `MemoryLoad`, and `BudgetStop` event payloads to carry an optional `WhyCard` field. The `WhyCard` is populated by the caller (run engine or budget guard) at the time the event is appended -- the journal package does not generate why-card text.

  Run: `go build ./internal/journal`
  Expected: compiles without error.

- [ ] **Step 4: Implement rolling 24-hour budget cap in internal/runtime/budget.go**

  The cap check queries the journal for all `TokensCharged` (or equivalent cost) events within the past 24 hours, sums their cost, and compares to the configured daily cap. If the sum plus the projected cost of the current turn would exceed the cap, the run engine stops the turn, appends a `BudgetStop` event with a `WhyCard` reason such as "Daily budget of $X reached; $Y spent in the last 24 hours", and surfaces the stop to the caller. The window is a rolling 24-hour lookback from now -- not a UTC midnight reset.

  Run: `go build ./internal/runtime`
  Expected: compiles without error.

- [ ] **Step 5: Surface budget-remaining in the web UI**

  In `internal/web/routes_runs.go`, when building the run detail view data, compute remaining budget as (daily cap minus rolling 24-hour spend) and include it in the template data struct.

  In `run_detail.html`, add a single line below the cost figure showing remaining daily budget, formatted with JetBrains Mono, weight 400.

  In `internal/web/routes_settings.go`, add a daily budget cap field (numeric, USD) to the settings form. On POST save the value to the settings store. Add the corresponding input to `settings.html` following the existing form field style.

  Run: `go build ./internal/web`
  Expected: compiles without error.

- [ ] **Step 6: Render why-cards in replay timeline**

  In `internal/replay/replay.go`, when building a timeline entry for a `Delegation`, `ApprovalNeeded`, `MemoryLoad`, or `BudgetStop` event, check if the event payload carries a `WhyCard`. If it does, include the reason text as a sub-line in the timeline entry rendered to the template. If it does not, omit the sub-line entirely -- no empty container.

  Run: `go build ./internal/replay`
  Expected: compiles without error.

- [ ] **Step 7: Run budget and why-card tests**

  Run: `go test ./internal/runtime -run 'TestBudgetCap' -v`
  Expected: all tests PASS.

  Run: `go test ./internal/replay -run 'TestWhyCard' -v`
  Expected: all tests PASS.

- [ ] **Step 8: Commit**

  Stage all budget cap and why-card files.

  Run: `git add internal/runtime/budget.go internal/runtime/budget_test.go internal/journal/events.go internal/replay/replay.go internal/replay/replay_test.go internal/web/routes_runs.go internal/web/routes_settings.go internal/web/templates/run_detail.html internal/web/templates/settings.html && git commit -m "feat(m3): rolling 24h budget cap and grounded why-cards"`
  Expected: commit created, hook passes.

---

### Task 13: Third migration -- 003_scheduler.sql

**Files:**
- Create: `internal/store/migrations/003_scheduler.sql`
- Modify: `internal/store/migrate.go` -- register 003 migration
- Test: `internal/store/migrate_test.go`

- [ ] **Step 1: Write a failing migration test**

  Add a test to `internal/store/migrate_test.go` that: opens a fresh in-memory SQLite database, runs all migrations up to and including 003, queries the `schedules` table with a SELECT that returns zero rows (no data needed), and asserts the table exists without error. Also assert that the daemon binary compiles and the `internal/telegram` package (added in the next task) can reference the `schedules` table name without a missing-schema panic.

  Run: `go test ./internal/store -run 'TestMigration003' 2>&1 | head -30`
  Expected: compilation errors or test failures.

- [ ] **Step 2: Write 003_scheduler.sql**

  Create `internal/store/migrations/003_scheduler.sql`. The file adds a `schedules` table with columns for: schedule ID (primary key), team ID (foreign key to teams), cron expression, enabled flag, created timestamp, and updated timestamp. No triggers, no views, no data. The scheduler feature is NOT active in Milestone 3 -- this migration exists only so `internal/telegram` and any other new package can compile against a complete schema.

  Run: `go build ./internal/store`
  Expected: compiles without error.

- [ ] **Step 3: Register the migration in migrate.go**

  In `internal/store/migrate.go`, add the 003 migration to the ordered migration list using the same registration pattern used for 001 and 002. Confirm the migration runner applies migrations in order and is idempotent (running twice does not error).

  Run: `go build ./internal/store`
  Expected: compiles without error.

- [ ] **Step 4: Run migration test**

  Run: `go test ./internal/store -run 'TestMigration003' -v`
  Expected: PASS -- `schedules` table exists after migration, zero rows.

  Run: `go test ./internal/store -v`
  Expected: all store tests PASS (no regressions).

- [ ] **Step 5: Commit**

  Run: `git add internal/store/migrations/003_scheduler.sql internal/store/migrate.go internal/store/migrate_test.go && git commit -m "feat(m3): add 003_scheduler.sql migration (table only, scheduler not active)"`
  Expected: commit created, hook passes.

---

### Task 14: Telegram DM connector -- bot and inbound

**Files:**
- Create: `internal/telegram/bot.go` -- long-poll loop, bot token management, update dispatch
- Create: `internal/telegram/inbound.go` -- normalize Telegram DM update to internal Envelope
- Test: `internal/telegram/bot_test.go`
- Test: `internal/telegram/inbound_test.go`

- [ ] **Step 1: Write failing tests for bot startup and long polling**

  Write tests in `internal/telegram/bot_test.go` that cover: when no bot token is configured the connector does not start and no goroutine is leaked; when a bot token is configured the connector calls `getUpdates` with a 30-second timeout; the poll loop dispatches each update to the inbound handler; when the context is cancelled the poll loop exits cleanly within 5 seconds; the connector does not register any webhook endpoint.

  Run: `go test ./internal/telegram -run 'TestBot' 2>&1 | head -30`
  Expected: package does not exist yet -- compilation error is expected.

- [ ] **Step 2: Write failing tests for inbound normalization**

  Write tests in `internal/telegram/inbound_test.go` that cover: a Telegram Update from a private chat (DM) is normalized to an internal Envelope with ConnectorID="telegram", AccountID=chat_id, ExternalID=message_id, ThreadID="main"; a Telegram Update from a group chat is rejected with an explicit "DM only" error and is not forwarded to the run engine; a Telegram Update with an unrecognized command (not a task submission) is rejected; the normalized Envelope carries the user's message text verbatim as the task body.

  Run: `go test ./internal/telegram -run 'TestInbound' 2>&1 | head -30`
  Expected: compilation errors or test failures.

- [ ] **Step 3: Implement internal/telegram/bot.go**

  Create the `internal/telegram` package. `bot.go` exports a `Bot` struct with a `Start(ctx context.Context) error` method. On start it reads the bot token from the app config; if the token is empty it returns immediately without error and without starting the poll loop. The poll loop issues HTTP GET requests to `https://api.telegram.org/bot{TOKEN}/getUpdates` with `timeout=30` and the last-seen `offset` as query params. Each returned update is dispatched to `inbound.go` for normalization. The loop exits when the context is cancelled.

  Run: `go build ./internal/telegram`
  Expected: compiles without error.

- [ ] **Step 4: Implement internal/telegram/inbound.go**

  `inbound.go` exports a `NormalizeUpdate` function that accepts a raw Telegram Update struct and returns an internal Envelope or an error. If the update's `message.chat.type` is not "private", return an error with reason "DM only". If the message text does not match the expected task submission pattern, return an error with reason "unrecognized command". Otherwise populate an Envelope: ConnectorID="telegram", AccountID set to the string form of `chat.id`, ExternalID set to the string form of `message.message_id`, ThreadID="main", Body set to the message text.

  Run: `go build ./internal/telegram`
  Expected: compiles without error.

- [ ] **Step 5: Run bot and inbound tests**

  Run: `go test ./internal/telegram -run 'TestBot|TestInbound' -v`
  Expected: all tests PASS.

- [ ] **Step 6: Commit**

  Run: `git add internal/telegram/bot.go internal/telegram/inbound.go internal/telegram/bot_test.go internal/telegram/inbound_test.go && git commit -m "feat(m3): telegram bot long-poll and inbound normalization"`
  Expected: commit created, hook passes.

---

### Task 15: Telegram DM connector -- outbound and retry queue

**Files:**
- Create: `internal/telegram/outbound.go` -- deliver outbound messages, drain outbound_intents table
- Create: `internal/telegram/queue.go` -- retry queue logic for durable delivery
- Modify: `internal/app/bootstrap.go` -- conditionally start Telegram connector when bot token is configured
- Modify: `internal/web/routes_settings.go` -- add Telegram token field to settings page
- Test: `internal/telegram/outbound_test.go`

- [ ] **Step 1: Write failing tests for outbound delivery**

  Write tests in `internal/telegram/outbound_test.go` that cover: a `started` run event for a subscribed chat ID delivers the correct message to that chat ID's Telegram sendMessage endpoint; a `blocked` event delivers a message; an `approval_needed` event for a low-risk approval delivers a message with an inline approve/reject option; an `approval_needed` event for a medium-risk approval delivers a message with only a "approve in the web UI" URL and no inline action; a `finished` event delivers a message; other event types (e.g. TokensCharged, MemoryLoad) produce no outbound message; a dedup key prevents the same intent from being delivered twice even if the queue drains twice.

  Run: `go test ./internal/telegram -run 'TestOutbound' 2>&1 | head -30`
  Expected: compilation errors or test failures.

- [ ] **Step 2: Write failing tests for retry queue**

  Add tests to `internal/telegram/outbound_test.go` that cover: when the Telegram API returns an error, the delivery is retried with exponential backoff; a mock API that fails 3 times then succeeds results in the message being delivered and the receipt showing a retry count of 3; after 5 consecutive failures the intent is marked as terminal and a `delivery_failed` journal event is appended; `delivery_failed` carries the intent ID, the target chat ID, the original event type, and the final error message; on connector startup any `pending` intents from a previous session are drained and delivery is retried.

  Run: `go test ./internal/telegram -run 'TestRetry' 2>&1 | head -30`
  Expected: compilation errors or test failures.

- [ ] **Step 3: Implement internal/telegram/outbound.go**

  `outbound.go` subscribes to the run engine's event stream. For each event it checks the type against the allowed outbound set (started, blocked, approval-needed, finished). If the event type is not in that set, it is silently dropped. For allowed events it builds a short plain-text message (no full transcript, no run detail dump), writes an intent row to `outbound_intents` with status=pending and a dedup key derived from (run ID, event type, external message ID), then attempts delivery via `sendMessage`. On success it marks the intent as delivered.

  Run: `go build ./internal/telegram`
  Expected: compiles without error.

- [ ] **Step 4: Implement internal/telegram/queue.go**

  `queue.go` exports a `Drain` function that selects all `pending` and `retrying` intents from `outbound_intents`, attempts delivery for each, and applies exponential backoff between retries. The retry schedule is: attempt 1 immediate, attempt 2 after ~1 min, attempt 3 after ~2 min, attempt 4 after ~8 min, attempt 5 after ~16 min -- five total attempts, terminal after ~30 minutes of elapsed time. On terminal failure call `AppendEvent` with a `delivery_failed` event payload. `Drain` is called once on connector startup (to catch up missed deliveries from a previous session) and is called again after each failed delivery attempt until the intent is either delivered or terminal.

  Run: `go build ./internal/telegram`
  Expected: compiles without error.

- [ ] **Step 5: Wire Telegram connector into bootstrap.go**

  In `internal/app/bootstrap.go`, after loading app config, check whether a Telegram bot token is present. If it is, instantiate a `telegram.Bot`, pass it the event stream and the conversation store, and start it with the app-level context. If the token is absent, skip this step entirely -- no panic, no warning log. On graceful shutdown the context cancel propagates to the poll loop and the connector exits cleanly.

  Run: `go build ./...`
  Expected: compiles without error.

- [ ] **Step 6: Add Telegram token field to settings page**

  In `internal/web/routes_settings.go`, add a handler path for the Telegram bot token field: GET renders the current token (masked after the first 8 characters), POST saves the new token to the settings store and triggers a connector restart if the token changed. Add the corresponding input to `settings.html` using the existing form field style: 1.5px solid #1c1917 border, 0px border radius, JetBrains Mono for the masked token display.

  Run: `go build ./internal/web`
  Expected: compiles without error.

- [ ] **Step 7: Restrict approval resolution by risk level**

  In `inbound.go`, when a Telegram reply is received in the context of an approval-needed event, look up the approval record and check its risk level. If risk level is low, allow the reply to resolve the approval (approve or reject based on the reply content). If risk level is medium or high, respond via Telegram with a plain message that says the approval must be completed in the web UI and includes the run's web URL. Do not resolve or reject the approval from Telegram for medium or high risk.

  Run: `go build ./internal/telegram`
  Expected: compiles without error.

- [ ] **Step 8: Run all Telegram tests**

  Run: `go test ./internal/telegram -v`
  Expected: all tests PASS.

- [ ] **Step 9: Commit**

  Run: `git add internal/telegram/outbound.go internal/telegram/queue.go internal/telegram/outbound_test.go internal/app/bootstrap.go internal/web/routes_settings.go internal/web/templates/settings.html && git commit -m "feat(m3): telegram outbound delivery with durable retry queue"`
  Expected: commit created, hook passes.

---

## Milestone 3 Exit Criteria

Run: `go test ./... -run 'Milestone3|PublicBeta'`
Expected: PASS

Run: `go test ./...`
Expected: PASS (no regressions across all packages)

Run: `go list -deps ./internal/runtime | grep internal/web`
Expected: no output (runtime must not import web)

Run: `go build ./...`
Expected: compiles without error

Manual proof:
- Edit a soul field from the web UI; restart daemon; confirm field change persists in `teams/{id}.yaml`
- Add an agent via the visual composer; confirm `teams/{id}.yaml` updated on disk; run `go build ./...` to confirm startup validation passes
- Send a task from Telegram DM; see run appear at /runs in the web UI
- Receive a Telegram message when a run completes
- Inspect memory facts via /memory; forget one fact; restart daemon; confirm forgotten fact does not reappear
- Trigger a daily cap breach; confirm the run stops with a budget-stop why-card visible in the replay timeline
- Trigger a low-risk approval from Telegram; confirm it resolves the run
- Trigger a medium-risk approval; confirm Telegram replies with web UI URL instead of resolving inline
- Navigate to /teams/{id}/composer; add a handoff with an unknown capability flag; confirm 422 response and YAML unchanged on disk
