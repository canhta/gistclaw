> **Docs:** [README](../../README.md) | [implementation-plan.md](../../implementation-plan.md) | [dependencies.md](../../dependencies.md) | [12-go-package-structure.md](../../12-go-package-structure.md) | [13-core-interfaces.md](../../13-core-interfaces.md)

# GistClaw -- Milestone 2: Local Beta

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a fully usable local operator tool. An engineer with a git repo can bind their workspace, submit tasks via the web UI, watch runs update live via SSE, approve patch proposals, and inspect receipts showing model, tokens, cost, and approval outcome. The daemon is idle-safe: it makes zero model calls when no run is active.

**Architecture:** Builds on Milestone 1's journal-backed run engine, ConversationStore, tools, and replay packages. Adds the local web UI (internal/web), the SSE broadcaster wired as the run engine's RunEventSink, admin token auth on all write-path handlers, a default four-agent team, and a four-step first-run onboarding wizard. Read-path handlers bypass the runtime and read from store and replay directly — this is intentional and documented. Write-path handlers call methods on runtime.Runtime — no direct DB writes from handlers.

**Tech Stack:** Go 1.24+, stdlib net/http, html/template, SSE (no WebSocket), SQLite via modernc.org/sqlite, Go testing + httptest, no frontend framework.

---

## Dependency and import rules (enforced throughout)

- internal/runtime must NEVER import internal/web. Verify after every task with: `go list -deps ./internal/runtime/...`
- internal/web imports internal/model for RunEventSink and ReplayDelta — NOT internal/runtime directly for the broadcaster type
- Write-path handlers (approvals, settings, run submit) call methods on runtime.Runtime
- Read-path handlers (run list, run detail, replay) may read from store and replay directly
- ConversationStore.AppendEvent is the single canonical journal append path — no other package appends to the events table
- All team and soul YAML validated at startup — daemon fails loudly on schema error

---

## Task 8: Local web UI shell

**Files:**
- Create: `internal/web/server.go`
- Create: `internal/web/sse.go`
- Create: `internal/web/routes_runs.go`
- Create: `internal/web/routes_run_submit.go`
- Create: `internal/web/routes_approvals.go`
- Create: `internal/web/routes_settings.go`
- Create: `internal/web/templates/layout.html`
- Create: `internal/web/templates/runs.html`
- Create: `internal/web/templates/run_detail.html`
- Create: `internal/web/templates/run_submit.html`
- Create: `internal/web/templates/approvals.html`
- Create: `internal/web/templates/settings.html`
- Test: `internal/web/server_test.go`

- [ ] **Step 1: Write failing HTTP tests**
  Write tests covering all routes before writing any handler code. Tests must cover:
  - GET /runs returns 200 with a run list when the store has rows and 200 with an empty state when it does not
  - GET /runs/{id} returns 200 for a known run ID and 404 for an unknown ID
  - GET /approvals returns 200
  - GET /settings returns 200
  - Admin token auth: a request to POST /run with the correct bearer token in the Authorization header returns 200; a request with no Authorization header returns 401; a request with a wrong token returns 401
  - SSE fan-out: two test clients subscribe to GET /runs/{id}/events; emit one ReplayDelta via the broadcaster; both clients receive the event; disconnect one client; emit a second delta; the remaining client receives it and the disconnected client's channel is cleaned up without blocking
  Run: `go test ./internal/web -run 'TestRuns|TestApprovals|TestSettings|TestAdminToken|TestSSE'`
  Expected: all tests fail with "not implemented" or compile errors — this is the desired red state

- [ ] **Step 2: Implement server.go**
  Create the HTTP server struct, mux wiring for all six route groups, and middleware chain. Middleware must handle: admin token extraction and validation for write-path routes, request logging with run ID when present, and panic recovery. Template loading must happen at server construction time so any missing template causes an immediate startup error rather than a runtime 500. The server must expose a ServeHTTP method so it can be tested with httptest.NewRecorder without starting a real listener.
  Run: `go build ./internal/web`
  Expected: compiles with no errors

- [ ] **Step 3: Implement sse.go**
  Create the SSEBroadcaster struct. It must hold an in-memory subscriber map of type map[RunID][]chan ReplayDelta, protected by a sync.RWMutex. Implement the Subscribe method that returns a channel for a given run ID and registers it in the map. Implement the Unsubscribe method that removes the channel and closes it. Implement the Emit method that satisfies the model.RunEventSink interface: it looks up all channels for the given run ID and sends the delta to each one without blocking — if a channel is full, skip it rather than blocking the emit path. Emit must not call any method on internal/runtime or internal/web server. After implementing, verify the import graph is clean.
  Run: `go list -deps ./internal/runtime/... | grep internal/web`
  Expected: no output (internal/runtime does not import internal/web)

- [ ] **Step 4: Implement routes_runs.go**
  Implement GET /runs to read the run list directly from the store (intentional read-path bypass). Implement GET /runs/{id} to read run detail and timeline from the replay service directly. Implement GET /runs/{id}/events as the SSE endpoint: it calls SSEBroadcaster.Subscribe, writes the SSE headers, and streams ReplayDelta events as server-sent events until the client disconnects, then calls Unsubscribe. No write operations in this file.
  Run: `go test ./internal/web -run 'TestRuns'`
  Expected: PASS

- [ ] **Step 5: Implement routes_run_submit.go**
  Implement GET /run to render the task submission form. Implement POST /run to parse the form body, validate the task string is non-empty and the workspace path matches the bound workspace, and call runtime.StartRun. On success, redirect to GET /runs/{id} for the new run. On validation error, re-render the form with an inline error message. This is a write-path handler — it must call a method on runtime.Runtime, not write to the store directly.
  Run: `go test ./internal/web -run 'TestRunSubmit'`
  Expected: PASS

- [ ] **Step 6: Implement routes_approvals.go**
  Implement GET /approvals to read pending approval tickets directly from the store (read-path bypass). Implement POST /approvals/{id}/resolve to accept an approve or deny decision from the form body and call runtime.ResolveApproval. On success, redirect back to GET /approvals. On error, render the approvals page with an inline error. This is a write-path handler.
  Run: `go test ./internal/web -run 'TestApprovals'`
  Expected: PASS

- [ ] **Step 7: Implement routes_settings.go**
  Implement GET /settings to read current settings directly from the store (read-path bypass), including the admin token display field (masked, with a reveal toggle in JS). Implement POST /settings to update settings via a runtime method. The admin token must never appear in a form value that could be submitted back — it is display-only on the GET page.
  Run: `go test ./internal/web -run 'TestSettings'`
  Expected: PASS

- [ ] **Step 8: Implement admin token generation**
  On first daemon start, if no admin token exists in the settings table, generate a cryptographically random 32-byte token, hex-encode it, and store it in the settings table. Expose it via gistclaw inspect token (a new subcommand). The token is required as a bearer header on all write-path handlers. On token mismatch, return 401 with a plain-text body "unauthorized". Never log the token at info level.
  Run: `go test ./internal/web -run 'TestAdminToken'`
  Expected: PASS

- [ ] **Step 9: Implement the six HTML templates**
  All templates must implement the layout and visual decisions in docs/designs/ui-design-spec.md exactly. Do not invent design decisions not present in that document. Rules to enforce from DESIGN.md: 0px border radius everywhere (rectangles only; 50% for dot indicators), 1.5px solid #1c1917 borders on all UI chrome, 4px left border for run state signaling with no background fills for state, no shadows anywhere, JetBrains Mono for run IDs, timestamps, cost figures, and token counts, weight 400 for body text and 700 for headings and labels. No emoji anywhere. The layout template defines the top navigation bar with the idle indicator, pending approvals badge, and nav items as specified in the UI design spec section 1. The runs template renders the three-section grouped list (ACTIVE, NEEDS ATTENTION, RECENT) with 4px left border state indicators only. The run_detail template renders the timeline, delegation graph, and receipt panel. The run_submit template renders the task form. The approvals template renders the pending ticket list with diff preview and approve/deny actions. The settings template renders editable settings fields and the token display.
  Run: `go build ./internal/web`
  Expected: template parse errors would surface here — expected: compiles and all templates load without error

- [ ] **Step 10: Wire bootstrap.go**
  In internal/app/bootstrap.go, inside webWiring, construct the SSEBroadcaster. Pass it to runtimeWiring typed as model.RunEventSink, not as a concrete *web.SSEBroadcaster. The runtime constructor must accept model.RunEventSink. Verify the type signature is correct: the runtime constructor parameter must be declared as model.RunEventSink, not *sse.SSEBroadcaster or any web-package type.
  Run: `go list -deps ./internal/runtime/... | grep internal/web`
  Expected: no output

- [ ] **Step 11: Run all web tests**
  Run: `go test ./internal/web`
  Expected: PASS

- [ ] **Step 12: Commit**
  Message: `add local web UI shell with SSE broadcaster and six route groups`

---

## Task 9: Default team and repo-task starter workflow

**Files:**
- Create: `teams/default/team.yaml`
- Create: `teams/default/agent-01.soul.yaml`
- Create: `teams/default/agent-02.soul.yaml`
- Create: `teams/default/agent-03.soul.yaml`
- Create: `teams/default/agent-04.soul.yaml`
- Modify: `internal/runtime/runs.go`
- Modify: `internal/tools/runner.go`
- Modify: `internal/tools/workspace.go`
- Test: `internal/runtime/starter_workflow_test.go`

- [ ] **Step 1: Write failing starter workflow tests**
  Write tests covering the full repo-task happy path before writing any YAML or implementation code. Tests must cover:
  - A preview-only run produces a preview package containing a summary, proposed diff, verification plan, receipt, and replay path — and makes no apply call
  - An apply attempt without a valid approval ticket is rejected with a named error event in the journal
  - An approval fingerprint computed from a different action snapshot (tool name, args, or target path changed) does not match the original ticket and is rejected before apply
  - A verification result is attached to the run receipt after agent-04 completes
  - team.yaml with a missing required field (any of: agents, capability_flags, handoff_edges) fails validation at startup with a descriptive error
  - A handoff edge referencing an agent ID not declared in the agents list fails validation at startup with a descriptive error
  Run: `go test ./internal/runtime -run 'StarterWorkflow|TeamValidation'`
  Expected: all tests fail — this is the desired red state

- [ ] **Step 2: Author the four soul YAML files**
  Write agent-01.soul.yaml through agent-04.soul.yaml. Each file must declare typed fields: role, tone, posture, collaboration_style, escalation_rules, decision_boundaries, tool_posture, prohibitions, and notes. agent-01 is operator-facing: it accepts task input, clarifies ambiguity with the operator, and delegates to agent-02. agent-02 is workspace-write: it proposes and applies patches, requests approval before any workspace mutation, and cannot delegate outside the declared handoff edges. agent-03 is read-heavy: it reviews proposed and applied diffs and returns a review event without mutating workspace. agent-04 is a verification agent: it runs the declared verification plan and attaches a verification report to the receipt. None of the soul files may include a tool_posture that grants capabilities above the agent's declared capability flag.
  Run: `go build ./...` (YAML is not compiled — this just confirms no Go file was accidentally broken)
  Expected: builds clean

- [ ] **Step 3: Author team.yaml**
  Write teams/default/team.yaml declaring: all four agents with their IDs and soul file references, explicit capability flags per agent (workspace-write for agent-02, operator-facing for agent-01, read-heavy for agent-03, propose-only for agent-04 until verification is confirmed), and explicit handoff edges (agent-01 to agent-02, agent-02 to agent-03 for review, agent-03 return to agent-01, agent-01 to agent-04 for verification, agent-04 return to agent-01). Return edges must be declared separately from forward delegation edges.
  Run: `go build ./...`
  Expected: builds clean

- [ ] **Step 4: Implement startup validation**
  In internal/app/bootstrap.go, after loading team.yaml and all soul YAML files, parse and validate the full schema before serving any requests. Validation must cover: all required fields are present, all agent IDs referenced in handoff edges are declared in the agents list, all capability flags are from the allowed set, and all soul files referenced in team.yaml exist on disk. On any validation error, print a descriptive message identifying the failing field and file, then exit with a non-zero status code. Do not silently fall back to a default.
  Run: `go test ./internal/app -run 'TeamValidation'`
  Expected: PASS

- [ ] **Step 5: Implement the repo-task happy path**
  In internal/runtime/runs.go, at root-run start, load the team spec, validate it, and freeze it as the execution snapshot stored in the DB before any agent turn. Child runs inherit this frozen snapshot. Handoff validation in Delegate reads from the frozen snapshot, not from the live team.yaml on disk.

  In internal/tools/runner.go, enforce approval gating: when the tool policy decision for a call is ask, create an approval ticket, suspend the run waiting for resolution, and only execute the tool after receiving an approved decision. On deny, record the denial as a journal event and do not execute the tool.

  In internal/tools/workspace.go, enforce workspace root: WorkspaceApplier.Apply must receive the workspace root as a parameter from the run's execution snapshot and must reject any target path that is outside that root before touching the filesystem. The approval fingerprint must be computed as sha256 of the tool name, colon, sorted normalized args JSON, colon, and the target path. The fingerprint is single-use: after one apply attempt (approved or denied), the ticket is consumed and cannot be reused.

  The full happy path is: agent-01 receives task from the operator via the web UI, delegates to agent-02, agent-02 proposes a patch and requests approval, agent-02 applies the patch after the operator approves in the web UI, agent-03 reviews the applied diff and posts a review event, agent-04 runs the verification plan and attaches the verification report to the receipt. Each handoff creates a child run with a journal event before any model call in the child.
  Run: `go test ./internal/runtime -run 'StarterWorkflow'`
  Expected: PASS

- [ ] **Step 6: Run the full suite**
  Run: `go test ./...`
  Expected: PASS

- [ ] **Step 7: Run Milestone 2 acceptance tests**
  Run: `go test ./... -run 'Milestone2|LocalBeta'`
  Expected: PASS

- [ ] **Step 8: Commit**
  Message: `add default team YAML and repo-task starter workflow with approval-gated apply`

---

## Task 9b: First-run onboarding wizard

**Files:**
- Create: `internal/web/routes_onboarding.go`
- Create: `internal/web/templates/onboarding.html`
- Modify: `internal/app/bootstrap.go`
- Test: `internal/web/routes_onboarding_test.go`

- [ ] **Step 1: Write failing onboarding tests**
  Write tests covering the wizard flow before writing any handler code. Tests must cover:
  - A first open with no workspace bound redirects to GET /onboarding regardless of the requested path
  - Workspace bind validation: submitting a path that does not exist returns an inline error; submitting a path that exists but is not a git repo (no .git directory) returns an inline error; submitting a path that exists, is a git repo, but is not writable returns an inline error; submitting a valid path succeeds and advances to step 2
  - Repo-signal scan: given a test directory with at least one file and one simulated recent commit in the git log, the scan returns a non-empty shortlist of at least three task candidates
  - Fallback trio: given an empty or new repo with no recent commits, the scan returns exactly three candidates covering explain a subsystem, review a diff, and find next safe improvement
  - Balanced trio: the shortlist returned for a valid repo includes at least one candidate of each type (explain, review, improve)
  - Task pick at step 3: selecting one candidate and submitting the form dispatches a preview-only root run via runtime.StartRun and redirects to step 4
  - No model calls during repo scan: the scan function must not call any method on the provider or runtime that would result in a model API call; verify by injecting a spy provider that fails the test if Generate is called
  Run: `go test ./internal/web -run 'Onboarding'`
  Expected: all tests fail — this is the desired red state

- [ ] **Step 2: Implement step 1 — workspace bind**
  Create routes_onboarding.go. Add a check in bootstrap.go: after starting the HTTP server, if no workspace is bound in the settings table, redirect all non-static requests to GET /onboarding. Implement GET /onboarding/step/1 to render the workspace path form. Implement POST /onboarding/step/1 to validate the submitted path: check it exists on the filesystem, check it contains a .git directory, check the process can write a temp file inside it. On validation error, re-render step 1 with an inline error message. On success, persist the workspace root to the settings table and redirect to step 2.
  Run: `go test ./internal/web -run 'TestOnboardingStep1'`
  Expected: PASS

- [ ] **Step 3: Implement step 2 — repo-signal scan**
  Implement GET /onboarding/step/2 to trigger the repo-signal scan and render the shortlist. The scan reads three signals from the bound workspace: the git log for the last 30 days (commit messages and changed file paths only, via a shell call to git log), the list of open files in the working tree with uncommitted changes (via git status), and the top-level directory structure (one level deep). The scan makes no model calls. It uses heuristics only: directories named after known subsystems score as explain candidates; files with uncommitted changes score as review candidates; recently touched files with no recent test-file changes nearby score as improve candidates. Produce three to five candidates, one per row, with a short plain-English description of why the candidate was suggested.
  Run: `go test ./internal/web -run 'TestOnboardingStep2'`
  Expected: PASS

- [ ] **Step 4: Implement step 3 — balanced trio shortlist pick**
  Implement GET /onboarding/step/3 to render the three suggested task candidates for operator selection. The three candidates must cover one explain task, one review task, and one improve task. Do not auto-select. Render each as a selectable card with a radio input, the candidate description, and the signal that produced it. If the scan from step 2 returned fewer than three candidates or was missing a type, fill in the generic fallback for that type (explain: "Explain what [top-level directory] does", review: "Review uncommitted changes in the working tree", improve: "Find the safest next improvement in [most recently changed file]"). Implement POST /onboarding/step/3 to accept the selected candidate and redirect to step 4.
  Run: `go test ./internal/web -run 'TestOnboardingStep3'`
  Expected: PASS

- [ ] **Step 5: Implement step 4 — preview-only first run dispatch**
  Implement GET /onboarding/step/4 to dispatch a preview-only root run using the task selected in step 3. Call runtime.StartRun with a flag that sets the run to preview-only mode. The run must produce a preview package without applying any changes. Render step 4 as a live view: the page subscribes to the SSE channel for the new run ID and updates in real time as the run progresses. When the run reaches the preview-complete state, render the preview package inline: summary, proposed diff, verification plan, receipt panel (model, tokens, cost), and replay path link. Include a call-to-action at the bottom of the completed step: "Connect Telegram to get updates on future runs" with a link to the settings page. Do not auto-advance or auto-connect anything.
  Run: `go test ./internal/web -run 'TestOnboardingStep4'`
  Expected: PASS

- [ ] **Step 6: Add onboarding template**
  Create internal/web/templates/onboarding.html. It must extend layout.html and render all four steps using the same design rules as the other templates: 0px border radius, 1.5px solid #1c1917 borders, no shadows, JetBrains Mono for run IDs and token counts, no emoji. The active step indicator must use a 4px left border to mark the current step, not a background fill. Steps that are completed show a plain check glyph using a text character, not an emoji.
  Run: `go build ./internal/web`
  Expected: compiles and all templates parse without error

- [ ] **Step 7: Run all onboarding tests**
  Run: `go test ./internal/web -run 'Onboarding'`
  Expected: PASS

- [ ] **Step 8: Commit**
  Message: `add four-step first-run onboarding wizard with repo-signal scan and preview dispatch`

---

## Task 10: Per-run budgets wired

**Files:**
- Modify: `internal/runtime/runs.go`
- Modify: `internal/app/bootstrap.go`
- Modify: `internal/web/routes_settings.go`
- Modify: `internal/web/templates/settings.html`
- Test: `internal/runtime/budget_test.go`

- [ ] **Step 1: Write failing budget tests**
  Write tests covering budget enforcement before modifying any implementation. Tests must cover:
  - A run that would exceed its per-run token budget is stopped before the next model call, not mid-turn
  - A run stopped by a budget guard emits a journal event of kind budget_stop with the limit type and the usage at the time of stop
  - The run is marked interrupted, not failed, when stopped by budget
  - A daily cap check rejects a new run start when rolling 24-hour usage already exceeds the cap
  - RecordIdleBurn is called only when a run is in a waiting-for-approval state with open model context, not when the daemon is simply idle between runs
  - Raising a budget cap via the settings page takes effect on the next new run start, not on any active run
  Run: `go test ./internal/runtime -run 'Budget'`
  Expected: all tests fail — this is the desired red state

- [ ] **Step 2: Wire BudgetGuard into the run engine**
  In internal/runtime/runs.go, call BudgetGuard.BeforeTurn at the start of each agent turn before any provider call. If BeforeTurn returns an error, emit a budget_stop journal event, mark the run as interrupted, and return without calling the provider. Call BudgetGuard.RecordUsage after each provider call with the token and cost data from the GenerateResult. Call BudgetGuard.CheckDailyCap at the start of each new root run; on cap exceeded, reject StartRun with a named error that the web handler can surface to the operator.
  Run: `go test ./internal/runtime -run 'Budget'`
  Expected: PASS

- [ ] **Step 3: Expose budget settings in the settings page**
  In routes_settings.go and settings.html, add fields for per-run token budget, per-run cost cap (in USD), and daily cost cap (in USD). Display the current rolling 24-hour usage alongside the daily cap so the operator can see how close they are. Enforce that the POST /settings handler routes through a runtime method and does not write to the settings table directly.
  Run: `go test ./internal/web -run 'TestSettings'`
  Expected: PASS

- [ ] **Step 4: Set conservative defaults**
  In the migration file for the settings table, set conservative default values: per-run token budget at 50,000 tokens, per-run cost cap at $0.50 USD, daily cost cap at $5.00 USD. These defaults are active before any operator customization. Document the defaults in a comment in the migration file. Raising or disabling any cap requires an explicit operator action via the settings page.
  Run: `go test ./internal/store -run 'Migration'`
  Expected: PASS

- [ ] **Step 5: Run the full suite**
  Run: `go test ./...`
  Expected: PASS

- [ ] **Step 6: Commit**
  Message: `wire per-run and daily budget guards into the run engine with conservative defaults`

---

## Global Guardrails (Milestone 2)

1. No Telegram connector -- that is Milestone 3
2. No visual team composer -- that is Milestone 3
3. No memory inspector -- that is Milestone 3
4. No compare view
5. No WebSocket -- SSE only, every time
6. No frontend framework -- stdlib html/template only, no Tailwind, no utility framework
7. internal/runtime must NOT import internal/web -- verify after every task: `go list -deps ./internal/runtime/... | grep internal/web` -- expected output: nothing
8. No journal writes outside ConversationStore.AppendEvent -- no package may append to the events table directly
9. No direct DB writes from write-path HTTP handlers -- all mutations route through runtime methods
10. All team and soul YAML validated at startup -- daemon fails loudly on invalid schema before serving any request
11. Admin token required for all write-path handlers -- readable via `gistclaw inspect token`
12. No model calls during onboarding repo scan -- verified by spy provider in tests

---

## Milestone 2 Exit Criteria

Run: `go test ./... -run 'Milestone2|LocalBeta'`
Expected: PASS

Manual proof (run these in order against a real local git repo):

1. Start the daemon: `gistclaw serve`
   Expected: daemon starts, logs confirm team YAML validated, admin token generated, listening on localhost:8080

2. Open the browser to localhost:8080
   Expected: redirected to /onboarding/step/1 because no workspace is bound

3. Bind a local git repo via the onboarding wizard
   Expected: all four steps complete, a preview-only run finishes, the preview package is visible with diff, token count, and cost

4. Submit a task from the web UI at /run
   Expected: the new run appears in /runs with status "active" and a 4px left border state indicator; SSE updates the status in real time without a page reload

5. Approve a patch proposal in /approvals
   Expected: the approval card resolves, the run resumes, verification completes, and the run moves to "completed" in /runs

6. Open the run detail page for the completed run
   Expected: timeline shows all agent turns, delegation edges, tool calls, approval event, verification result; receipt panel shows model name, total tokens, total cost in USD, and approval outcome

7. Confirm the receipt reflects the approval fingerprint and verification status

8. Confirm the daemon is idle between runs: `gistclaw inspect status`
   Expected: output shows "idle", zero active runs, zero model calls in flight

9. Confirm the admin token is readable: `gistclaw inspect token`
   Expected: token printed to stdout, same value stored in the settings table
