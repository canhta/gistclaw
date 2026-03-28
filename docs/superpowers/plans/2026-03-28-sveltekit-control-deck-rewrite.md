# SvelteKit Control Deck Rewrite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the existing Go `html/template` operator UI with a full `SvelteKit + Tailwind + @xyflow/svelte` frontend that is served by the Go daemon, follows the new user-first information architecture, and exposes more of the runtime's current capabilities without weakening auth, runtime, journal, or scheduler boundaries.

**Architecture:** The Go daemon remains the only authority for auth, onboarding state, runtime mutation, SSE, scheduler dispatch, and data shaping. A new `frontend/` SvelteKit app builds static assets into `internal/web/appdist/`; `internal/web` serves those assets plus JSON browser APIs grouped by user surfaces (`Work`, `Team`, `Knowledge`, `Recover`, `Conversations`, `Automate`, `History`, `Settings`). The old `/operate`, `/configure`, and template-era page system is removed rather than preserved behind redirects or shims.

**Tech Stack:** Go 1.25, SvelteKit, Tailwind CSS, `@xyflow/svelte`, Vite, Vitest, Playwright, SSE, SQLite-backed Go services.

---

## Guardrails

- Implementation workers must use `@superpowers:test-driven-development` before each task and `@superpowers:verification-before-completion` before claiming any task is done.
- Stay on `main`. No feature branches and no worktrees.
- Do not add a production Node server. SvelteKit only produces static assets for the Go binary to serve.
- Do not keep the old `html/template` UI alive in parallel. Delete it once the new SPA route and browser APIs are in place.
- Do not preserve `/operate`, `/configure`, or other legacy route groups as compatibility aliases. The new route map is the product.
- Keep every write path in Go. The frontend may orchestrate user interactions, but runtime, approvals, routes, memory, settings, and schedules still mutate through Go handlers and services.
- Keep the top-level IA user-first:

```text
/work
/team
/knowledge
/recover
/conversations
/automate
/history
/settings
/login
/onboarding
```

## File Structure

### Create

- `frontend/package.json`
- `frontend/svelte.config.js`
- `frontend/vite.config.ts`
- `frontend/tsconfig.json`
- `frontend/vitest.config.ts`
- `frontend/playwright.config.ts`
- `frontend/src/app.html`
- `frontend/src/app.css`
- `frontend/src/lib/config/routes.ts`
- `frontend/src/lib/http/client.ts`
- `frontend/src/lib/http/events.ts`
- `frontend/src/lib/types/api.ts`
- `frontend/src/lib/components/shell/AppShell.svelte`
- `frontend/src/lib/components/shell/NavRail.svelte`
- `frontend/src/lib/components/shell/InspectorRail.svelte`
- `frontend/src/lib/components/graph/RunGraph.svelte`
- `frontend/src/lib/components/graph/RunGraphNode.svelte`
- `frontend/src/lib/components/graph/layout.ts`
- `frontend/src/routes/+layout.ts`
- `frontend/src/routes/+layout.svelte`
- `frontend/src/routes/+error.svelte`
- `frontend/src/routes/+page.svelte`
- `frontend/src/routes/login/+page.svelte`
- `frontend/src/routes/onboarding/+page.svelte`
- `frontend/src/routes/work/+page.svelte`
- `frontend/src/routes/work/[runId]/+page.svelte`
- `frontend/src/routes/team/+page.svelte`
- `frontend/src/routes/knowledge/+page.svelte`
- `frontend/src/routes/recover/+page.svelte`
- `frontend/src/routes/conversations/+page.svelte`
- `frontend/src/routes/automate/+page.svelte`
- `frontend/src/routes/history/+page.svelte`
- `frontend/src/routes/settings/+page.svelte`
- `frontend/src/lib/http/client.test.ts`
- `frontend/src/lib/components/graph/RunGraph.test.ts`
- `frontend/src/routes/work/page.test.ts`
- `frontend/e2e/auth.spec.ts`
- `frontend/e2e/work.spec.ts`
- `internal/web/appdist/index.html`
- `internal/web/spa_assets.go`
- `internal/web/spa_routes.go`
- `internal/web/api_auth.go`
- `internal/web/api_bootstrap.go`
- `internal/web/api_work.go`
- `internal/web/api_team.go`
- `internal/web/api_knowledge.go`
- `internal/web/api_recover.go`
- `internal/web/api_conversations.go`
- `internal/web/api_automate.go`
- `internal/web/api_history.go`
- `internal/web/api_settings.go`
- `internal/web/api_onboarding.go`
- `internal/web/spa_assets_test.go`
- `internal/web/api_auth_test.go`
- `internal/web/api_bootstrap_test.go`
- `internal/web/api_work_test.go`
- `internal/web/api_team_test.go`
- `internal/web/api_knowledge_test.go`
- `internal/web/api_recover_test.go`
- `internal/web/api_conversations_test.go`
- `internal/web/api_automate_test.go`
- `internal/web/api_history_test.go`
- `internal/web/api_settings_test.go`
- `internal/web/api_onboarding_test.go`

### Modify

- `Makefile`
- `.gitignore`
- `internal/app/bootstrap.go`
- `internal/web/server.go`
- `internal/web/paths.go`
- `internal/web/auth_middleware.go`
- `internal/web/routes_auth.go`
- `internal/web/routes_onboarding.go`
- `internal/web/routes_runs.go`
- `internal/web/routes_run_submit.go`
- `internal/web/routes_team.go`
- `internal/web/routes_memory.go`
- `internal/web/routes_approvals.go`
- `internal/web/routes_sessions.go`
- `internal/web/routes_session_pages.go`
- `internal/web/routes_routes_deliveries.go`
- `internal/web/routes_settings.go`
- `internal/web/server_test.go`
- `README.md`
- `docs/system.md`
- `CONTRIBUTING.md`

### Delete

- `internal/web/assets.go`
- `internal/web/design_system_test.go`
- `internal/web/templates/layout.html`
- `internal/web/templates/login.html`
- `internal/web/templates/runs.html`
- `internal/web/templates/run_detail.html`
- `internal/web/templates/run_submit.html`
- `internal/web/templates/approvals.html`
- `internal/web/templates/settings.html`
- `internal/web/templates/team.html`
- `internal/web/templates/onboarding.html`
- `internal/web/templates/memory.html`
- `internal/web/templates/sessions.html`
- `internal/web/templates/routes_deliveries.html`
- `internal/web/templates/session_detail.html`
- `internal/web/static/vendor/cytoscape.min.js`
- `internal/web/static/vendor/cytoscape-dagre.js`
- `internal/web/static/vendor/dagre.min.js`

## API Contract Shape

Use one bootstrap payload plus surface-scoped APIs. Keep shape stable and explicit.

```go
type BootstrapResponse struct {
	Auth struct {
		Authenticated bool   `json:"authenticated"`
		SetupRequired bool   `json:"setup_required"`
		LoginReason   string `json:"login_reason,omitempty"`
	} `json:"auth"`
	Project struct {
		ActiveID   string `json:"active_id"`
		ActiveName string `json:"active_name"`
		ActivePath string `json:"active_path"`
	} `json:"project"`
	Navigation []struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Href  string `json:"href"`
	} `json:"navigation"`
}
```

```ts
export type AppRoute =
	| '/work'
	| '/team'
	| '/knowledge'
	| '/recover'
	| '/conversations'
	| '/automate'
	| '/history'
	| '/settings'
	| '/login'
	| '/onboarding';
```

```go
type ScheduleService interface {
	CreateSchedule(ctx context.Context, in scheduler.CreateScheduleInput) (scheduler.Schedule, error)
	UpdateSchedule(ctx context.Context, scheduleID string, patch scheduler.UpdateScheduleInput) (scheduler.Schedule, error)
	ListSchedules(ctx context.Context) ([]scheduler.Schedule, error)
	LoadSchedule(ctx context.Context, scheduleID string) (scheduler.Schedule, error)
	EnableSchedule(ctx context.Context, scheduleID string) (scheduler.Schedule, error)
	DisableSchedule(ctx context.Context, scheduleID string) (scheduler.Schedule, error)
	DeleteSchedule(ctx context.Context, scheduleID string) error
	ScheduleStatus(ctx context.Context) (scheduler.StatusSummary, error)
	RunScheduleNow(ctx context.Context, scheduleID string) (*scheduler.ClaimedOccurrence, error)
}
```

## Task 1: Scaffold Frontend Workspace And Build Pipeline

**Files:**
- Create: `frontend/package.json`
- Create: `frontend/svelte.config.js`
- Create: `frontend/vite.config.ts`
- Create: `frontend/tsconfig.json`
- Create: `frontend/vitest.config.ts`
- Create: `frontend/playwright.config.ts`
- Create: `frontend/src/app.html`
- Create: `frontend/src/routes/+layout.svelte`
- Create: `frontend/src/routes/+page.svelte`
- Create: `internal/web/appdist/index.html`
- Create: `internal/web/spa_assets.go`
- Create: `internal/web/spa_assets_test.go`
- Modify: `Makefile`
- Modify: `.gitignore`

- [ ] **Step 1: Write the failing Go tests for SPA asset serving**

```go
func TestSPAIndexServesCommittedPlaceholder(t *testing.T) {}

func TestSPAStaticAssetLookupRejectsMissingFiles(t *testing.T) {}
```

- [ ] **Step 2: Run the Go tests to verify the asset layer does not exist yet**

Run: `go test ./internal/web -run 'TestSPA(IndexServesCommittedPlaceholder|StaticAssetLookupRejectsMissingFiles)' -count=1`

Expected: FAIL with missing `spaAssetsFS` or missing `index.html` helper.

- [ ] **Step 3: Create the frontend workspace and committed placeholder build output**

Use this dependency skeleton in `frontend/package.json`:

```json
{
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite dev",
    "build": "svelte-kit sync && vite build",
    "check": "svelte-kit sync && svelte-check --tsconfig ./tsconfig.json",
    "test:unit": "vitest run",
    "test:e2e": "playwright test"
  }
}
```

Use this build output rule in `frontend/vite.config.ts`:

```ts
export default defineConfig({
	build: {
		outDir: '../internal/web/appdist',
		emptyOutDir: true
	}
});
```

Use this placeholder in `internal/web/appdist/index.html`:

```html
<!doctype html>
<html lang="en">
  <head><meta charset="utf-8"><title>gistclaw frontend build missing</title></head>
  <body>Run `make ui-build` to generate the SvelteKit bundle.</body>
</html>
```

Add `Makefile` targets:

```make
ui-install:
	npm --prefix frontend install

ui-build:
	npm --prefix frontend run build

ui-check:
	npm --prefix frontend run check

ui-test:
	npm --prefix frontend run test:unit
```

Add `.gitignore` entries:

```gitignore
frontend/node_modules/
frontend/.svelte-kit/
frontend/playwright-report/
frontend/test-results/
```

- [ ] **Step 4: Implement `internal/web/spa_assets.go` and re-run the targeted Go tests**

Run: `go test ./internal/web -run 'TestSPA(IndexServesCommittedPlaceholder|StaticAssetLookupRejectsMissingFiles)' -count=1`

Expected: PASS

- [ ] **Step 5: Install frontend dependencies, build once, and verify the frontend toolchain**

Run: `npm --prefix frontend install`

Run: `npm --prefix frontend run build`

Run: `npm --prefix frontend run check`

Expected: PASS and generated files under `internal/web/appdist/`

- [ ] **Step 6: Commit**

```bash
git add .gitignore Makefile frontend internal/web/appdist/index.html internal/web/spa_assets.go internal/web/spa_assets_test.go
git commit -m "feat: scaffold sveltekit frontend pipeline"
```

## Task 2: Replace Template Shell Delivery With SPA Routes And Auth Bootstrap

**Files:**
- Create: `internal/web/spa_routes.go`
- Create: `internal/web/api_auth.go`
- Create: `internal/web/api_bootstrap.go`
- Create: `internal/web/api_auth_test.go`
- Create: `internal/web/api_bootstrap_test.go`
- Modify: `internal/web/server.go`
- Modify: `internal/web/paths.go`
- Modify: `internal/web/auth_middleware.go`
- Modify: `internal/web/routes_auth.go`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: Write failing auth and shell-delivery tests**

```go
func TestGETLoginServesSPAIndex(t *testing.T) {}

func TestAuthenticatedGETWorkServesSPAIndex(t *testing.T) {}

func TestAuthSessionAPIReportsSetupAndDeviceState(t *testing.T) {}
```

- [ ] **Step 2: Run the targeted tests**

Run: `go test ./internal/web -run 'Test(GETLoginServesSPAIndex|AuthenticatedGETWorkServesSPAIndex|AuthSessionAPIReportsSetupAndDeviceState)' -count=1`

Expected: FAIL because `/login`, `/work`, and `/api/auth/session` are still template-era flows.

- [ ] **Step 3: Replace route registration and auth delivery**

Register SPA document routes only for:

```go
var spaPaths = []string{
	"/",
	"/login",
	"/onboarding",
	"/work",
	"/team",
	"/knowledge",
	"/recover",
	"/conversations",
	"/automate",
	"/history",
	"/settings",
}
```

Register JSON auth/bootstrap endpoints:

```go
s.mux.HandleFunc("GET /api/auth/session", s.handleAuthSession)
s.mux.HandleFunc("POST /api/auth/login", s.handleAuthLogin)
s.mux.HandleFunc("POST /api/auth/logout", s.handleAuthLogout)
s.mux.HandleFunc("GET /api/bootstrap", s.handleBootstrap)
```

Update `auth_middleware.go` allow-list to permit:

```go
case path == "/login", path == "/api/auth/session", path == "/api/auth/login":
	return next
case path == "/_app" || strings.HasPrefix(path, "/_app/"):
	return next
```

Delete the old `renderTemplate*` dependency path from `server.go`. The server should serve either JSON/API responses or the SPA document.

- [ ] **Step 4: Re-run the targeted Go tests**

Run: `go test ./internal/web -run 'Test(GETLoginServesSPAIndex|AuthenticatedGETWorkServesSPAIndex|AuthSessionAPIReportsSetupAndDeviceState)' -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/server.go internal/web/paths.go internal/web/auth_middleware.go internal/web/routes_auth.go internal/web/spa_routes.go internal/web/api_auth.go internal/web/api_bootstrap.go internal/web/api_auth_test.go internal/web/api_bootstrap_test.go internal/web/server_test.go
git commit -m "feat: serve svelte shell and auth bootstrap from go"
```

## Task 3: Build The Work Surface API Contract

**Files:**
- Create: `internal/web/api_work.go`
- Create: `internal/web/api_work_test.go`
- Modify: `internal/web/server.go`
- Modify: `internal/web/paths.go`
- Modify: `internal/web/routes_runs.go`
- Modify: `internal/web/routes_run_submit.go`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: Write failing Work API tests**

```go
func TestWorkIndexReturnsQueueAndProjectSummary(t *testing.T) {}

func TestWorkDetailReturnsRunSummaryGraphAndInspectorSeed(t *testing.T) {}

func TestCreateWorkTaskStartsRunAndReturnsRunID(t *testing.T) {}
```

- [ ] **Step 2: Run the targeted Go tests**

Run: `go test ./internal/web -run 'Test(WorkIndexReturnsQueueAndProjectSummary|WorkDetailReturnsRunSummaryGraphAndInspectorSeed|CreateWorkTaskStartsRunAndReturnsRunID)' -count=1`

Expected: FAIL because `/api/work` endpoints do not exist.

- [ ] **Step 3: Implement `Work` handlers by moving existing run loaders into explicit browser APIs**

Use JSON endpoints:

```go
s.mux.HandleFunc("GET /api/work", s.handleWorkIndex)
s.mux.HandleFunc("GET /api/work/{id}", s.handleWorkDetail)
s.mux.HandleFunc("GET /api/work/{id}/graph", s.handleRunGraph)
s.mux.HandleFunc("GET /api/work/{id}/nodes/{node_id}", s.handleRunNodeDetail)
s.mux.HandleFunc("GET /api/work/{id}/events", s.handleRunEvents)
s.mux.Handle("POST /api/work", s.adminAuth(http.HandlerFunc(s.handleRunSubmit)))
s.mux.Handle("POST /api/work/{id}/dismiss", s.adminAuth(http.HandlerFunc(s.handleRunDismiss)))
```

Keep the current graph DTOs and SSE mechanics, but rename path builders to the new route family. The `Work` payload must include:

```go
type WorkIndexResponse struct {
	ActiveProjectName string               `json:"active_project_name"`
	ActiveProjectPath string               `json:"active_project_path"`
	QueueStrip        runQueueStripView    `json:"queue_strip"`
	Clusters          []runListClusterView `json:"clusters"`
	Paging            pageLinks            `json:"paging"`
}
```

- [ ] **Step 4: Re-run the targeted Go tests plus current run graph tests**

Run: `go test ./internal/web -run 'Test(WorkIndexReturnsQueueAndProjectSummary|WorkDetailReturnsRunSummaryGraphAndInspectorSeed|CreateWorkTaskStartsRunAndReturnsRunID)' -count=1`

Run: `go test ./internal/web -run 'TestBuildRunGraphView' -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/api_work.go internal/web/api_work_test.go internal/web/server.go internal/web/paths.go internal/web/routes_runs.go internal/web/routes_run_submit.go internal/web/server_test.go
git commit -m "feat: expose work surface browser api"
```

## Task 4: Build Team And Knowledge Browser APIs

**Files:**
- Create: `internal/web/api_team.go`
- Create: `internal/web/api_knowledge.go`
- Create: `internal/web/api_team_test.go`
- Create: `internal/web/api_knowledge_test.go`
- Modify: `internal/web/server.go`
- Modify: `internal/web/routes_team.go`
- Modify: `internal/web/routes_memory.go`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: Write failing Team and Knowledge API tests**

```go
func TestTeamAPIListsProfilesAndEditableMembers(t *testing.T) {}

func TestTeamMutationAPISelectsAndSavesProfiles(t *testing.T) {}

func TestKnowledgeAPIListsAndMutatesMemoryItems(t *testing.T) {}
```

- [ ] **Step 2: Run the targeted tests**

Run: `go test ./internal/web -run 'Test(TeamAPIListsProfilesAndEditableMembers|TeamMutationAPISelectsAndSavesProfiles|KnowledgeAPIListsAndMutatesMemoryItems)' -count=1`

Expected: FAIL because those surface APIs do not exist.

- [ ] **Step 3: Implement JSON APIs by moving template-centric logic into handler functions**

Add endpoints:

```go
s.mux.HandleFunc("GET /api/team", s.handleTeamAPI)
s.mux.Handle("POST /api/team/select", s.adminAuth(http.HandlerFunc(s.handleTeamSelectAPI)))
s.mux.Handle("POST /api/team/create", s.adminAuth(http.HandlerFunc(s.handleTeamCreateAPI)))
s.mux.Handle("POST /api/team/clone", s.adminAuth(http.HandlerFunc(s.handleTeamCloneAPI)))
s.mux.Handle("POST /api/team/delete", s.adminAuth(http.HandlerFunc(s.handleTeamDeleteAPI)))
s.mux.Handle("POST /api/team/save", s.adminAuth(http.HandlerFunc(s.handleTeamSaveAPI)))
s.mux.Handle("POST /api/team/import", s.adminAuth(http.HandlerFunc(s.handleTeamImportAPI)))
s.mux.HandleFunc("GET /api/knowledge", s.handleKnowledgeIndex)
s.mux.Handle("POST /api/knowledge/{id}/edit", s.adminAuth(http.HandlerFunc(s.handleMemoryEdit)))
s.mux.Handle("POST /api/knowledge/{id}/forget", s.adminAuth(http.HandlerFunc(s.handleMemoryForget)))
```

Keep one response shape per surface. Do not expose raw form field names in the API.

- [ ] **Step 4: Re-run the targeted tests**

Run: `go test ./internal/web -run 'Test(TeamAPIListsProfilesAndEditableMembers|TeamMutationAPISelectsAndSavesProfiles|KnowledgeAPIListsAndMutatesMemoryItems)' -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/api_team.go internal/web/api_knowledge.go internal/web/api_team_test.go internal/web/api_knowledge_test.go internal/web/server.go internal/web/routes_team.go internal/web/routes_memory.go internal/web/server_test.go
git commit -m "feat: add team and knowledge browser apis"
```

## Task 5: Build Recover And Conversations Browser APIs

**Files:**
- Create: `internal/web/api_recover.go`
- Create: `internal/web/api_conversations.go`
- Create: `internal/web/api_recover_test.go`
- Create: `internal/web/api_conversations_test.go`
- Modify: `internal/web/server.go`
- Modify: `internal/web/routes_approvals.go`
- Modify: `internal/web/routes_routes_deliveries.go`
- Modify: `internal/web/routes_sessions.go`
- Modify: `internal/web/routes_session_pages.go`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: Write failing Recover and Conversations API tests**

```go
func TestRecoverAPIListsApprovalsRoutesAndDeliveries(t *testing.T) {}

func TestRecoverApproveAndRetryMutationsFlowThroughRuntime(t *testing.T) {}

func TestConversationsAPIListsSessionsAndSessionDetail(t *testing.T) {}
```

- [ ] **Step 2: Run the targeted tests**

Run: `go test ./internal/web -run 'Test(RecoverAPIListsApprovalsRoutesAndDeliveries|RecoverApproveAndRetryMutationsFlowThroughRuntime|ConversationsAPIListsSessionsAndSessionDetail)' -count=1`

Expected: FAIL because the new surface routes do not exist.

- [ ] **Step 3: Implement Recover and Conversations APIs**

Add endpoints:

```go
s.mux.HandleFunc("GET /api/recover", s.handleRecoverIndex)
s.mux.Handle("POST /api/recover/approvals/{id}/resolve", s.adminAuth(http.HandlerFunc(s.handleApprovalResolve)))
s.mux.Handle("POST /api/recover/routes/{id}/messages", s.adminAuth(http.HandlerFunc(s.handleRouteSend)))
s.mux.Handle("POST /api/recover/routes/{id}/deactivate", s.adminAuth(http.HandlerFunc(s.handleRouteDeactivate)))
s.mux.Handle("POST /api/recover/deliveries/{id}/retry", s.adminAuth(http.HandlerFunc(s.handleDeliveryRetry)))
s.mux.HandleFunc("GET /api/conversations", s.handleConversationsIndex)
s.mux.HandleFunc("GET /api/conversations/{id}", s.handleConversationDetail)
s.mux.Handle("POST /api/conversations/{id}/messages", s.adminAuth(http.HandlerFunc(s.handleSessionSend)))
s.mux.Handle("POST /api/conversations/{id}/deliveries/{delivery_id}/retry", s.adminAuth(http.HandlerFunc(s.handleSessionRetryDelivery)))
```

Keep existing project scoping and visibility checks. Move them to shared helpers instead of duplicating per handler.

- [ ] **Step 4: Re-run the targeted tests and existing delivery/session tests**

Run: `go test ./internal/web -run 'Test(RecoverAPIListsApprovalsRoutesAndDeliveries|RecoverApproveAndRetryMutationsFlowThroughRuntime|ConversationsAPIListsSessionsAndSessionDetail)' -count=1`

Run: `go test ./internal/web -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/api_recover.go internal/web/api_conversations.go internal/web/api_recover_test.go internal/web/api_conversations_test.go internal/web/server.go internal/web/routes_approvals.go internal/web/routes_routes_deliveries.go internal/web/routes_sessions.go internal/web/routes_session_pages.go internal/web/server_test.go
git commit -m "feat: add recover and conversations browser apis"
```

## Task 6: Build Automate And History Browser APIs

**Files:**
- Create: `internal/web/api_automate.go`
- Create: `internal/web/api_history.go`
- Create: `internal/web/api_automate_test.go`
- Create: `internal/web/api_history_test.go`
- Modify: `internal/app/bootstrap.go`
- Modify: `internal/web/server.go`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: Write failing Automate and History API tests**

```go
func TestAutomateAPIListsSchedulesAndStatus(t *testing.T) {}

func TestAutomateMutationAPIControlsSchedules(t *testing.T) {}

func TestHistoryAPIListsRunsInterventionsAndDeliveryEvidence(t *testing.T) {}
```

- [ ] **Step 2: Run the targeted tests**

Run: `go test ./internal/web -run 'Test(AutomateAPIListsSchedulesAndStatus|AutomateMutationAPIControlsSchedules|HistoryAPIListsRunsInterventionsAndDeliveryEvidence)' -count=1`

Expected: FAIL because web has no scheduler interface and no `History` surface.

- [ ] **Step 3: Wire the schedule service into `web.Options` and implement the APIs**

In `internal/app/bootstrap.go`, pass `application` into `web.Options` through the new consumer-side schedule interface.

Add endpoints:

```go
s.mux.HandleFunc("GET /api/automate", s.handleAutomateIndex)
s.mux.Handle("POST /api/automate", s.adminAuth(http.HandlerFunc(s.handleAutomateCreate)))
s.mux.Handle("POST /api/automate/{id}", s.adminAuth(http.HandlerFunc(s.handleAutomateUpdate)))
s.mux.Handle("POST /api/automate/{id}/run", s.adminAuth(http.HandlerFunc(s.handleAutomateRunNow)))
s.mux.Handle("POST /api/automate/{id}/enable", s.adminAuth(http.HandlerFunc(s.handleAutomateEnable)))
s.mux.Handle("POST /api/automate/{id}/disable", s.adminAuth(http.HandlerFunc(s.handleAutomateDisable)))
s.mux.Handle("POST /api/automate/{id}/delete", s.adminAuth(http.HandlerFunc(s.handleAutomateDelete)))
s.mux.HandleFunc("GET /api/history", s.handleHistoryIndex)
```

`History` should aggregate:

```go
type HistoryIndexResponse struct {
	Runs          []runListClusterView          `json:"runs"`
	Approvals     []approvalListItemView        `json:"approvals"`
	Deliveries    []model.DeliveryQueueItem     `json:"deliveries"`
	Interventions []historyInterventionListItem `json:"interventions"`
}
```

- [ ] **Step 4: Re-run the targeted tests and current scheduler app tests**

Run: `go test ./internal/web -run 'Test(AutomateAPIListsSchedulesAndStatus|AutomateMutationAPIControlsSchedules|HistoryAPIListsRunsInterventionsAndDeliveryEvidence)' -count=1`

Run: `go test ./internal/app -run 'TestApp_ScheduleStatusReturnsNextWakeAndCounts' -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/bootstrap.go internal/web/api_automate.go internal/web/api_history.go internal/web/api_automate_test.go internal/web/api_history_test.go internal/web/server.go internal/web/server_test.go
git commit -m "feat: add automate and history browser apis"
```

## Task 7: Build The Svelte Shell, Tokens, And Shared HTTP Layer

**Files:**
- Create: `frontend/src/app.css`
- Create: `frontend/src/lib/config/routes.ts`
- Create: `frontend/src/lib/http/client.ts`
- Create: `frontend/src/lib/http/events.ts`
- Create: `frontend/src/lib/types/api.ts`
- Create: `frontend/src/lib/components/shell/AppShell.svelte`
- Create: `frontend/src/lib/components/shell/NavRail.svelte`
- Create: `frontend/src/lib/components/shell/InspectorRail.svelte`
- Create: `frontend/src/routes/+layout.ts`
- Create: `frontend/src/routes/+layout.svelte`
- Create: `frontend/src/routes/+error.svelte`
- Create: `frontend/src/lib/http/client.test.ts`

- [ ] **Step 1: Write failing frontend unit tests for bootstrap loading and shell navigation**

```ts
describe('client bootstrap', () => {
	it('loads /api/bootstrap and returns user-first navigation', async () => {});
});
```

- [ ] **Step 2: Run the frontend unit tests**

Run: `npm --prefix frontend run test:unit -- client`

Expected: FAIL because the HTTP client and shell do not exist.

- [ ] **Step 3: Implement the shared HTTP layer and shell primitives**

The API client should be simple and explicit:

```ts
export async function api<T>(input: string, init?: RequestInit): Promise<T> {
	const response = await fetch(input, {
		credentials: 'same-origin',
		headers: { 'content-type': 'application/json', ...(init?.headers ?? {}) },
		...init
	});
	if (!response.ok) throw new Error(await response.text());
	return (await response.json()) as T;
}
```

Import design tokens in `frontend/src/app.css` and encode the brutalist shell there. Do not use a component library. Tailwind plus project primitives only.

- [ ] **Step 4: Re-run frontend unit tests and type-check**

Run: `npm --prefix frontend run test:unit`

Run: `npm --prefix frontend run check`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app.css frontend/src/lib/config/routes.ts frontend/src/lib/http/client.ts frontend/src/lib/http/events.ts frontend/src/lib/types/api.ts frontend/src/lib/components/shell/AppShell.svelte frontend/src/lib/components/shell/NavRail.svelte frontend/src/lib/components/shell/InspectorRail.svelte frontend/src/routes/+layout.ts frontend/src/routes/+layout.svelte frontend/src/routes/+error.svelte frontend/src/lib/http/client.test.ts
git commit -m "feat: build svelte shell and shared http layer"
```

## Task 8: Implement The Work Surface With XYFlow

**Files:**
- Create: `frontend/src/lib/components/graph/RunGraph.svelte`
- Create: `frontend/src/lib/components/graph/RunGraphNode.svelte`
- Create: `frontend/src/lib/components/graph/layout.ts`
- Create: `frontend/src/routes/work/+page.svelte`
- Create: `frontend/src/routes/work/[runId]/+page.svelte`
- Create: `frontend/src/lib/components/graph/RunGraph.test.ts`
- Create: `frontend/src/routes/work/page.test.ts`
- Create: `frontend/e2e/work.spec.ts`

- [ ] **Step 1: Write failing frontend tests for run queue, run detail, and live graph rendering**

```ts
describe('Work page', () => {
	it('renders queue strip and active run graph', async () => {});
});
```

```ts
test('login then open /work and inspect a run', async ({ page }) => {});
```

- [ ] **Step 2: Run the frontend tests**

Run: `npm --prefix frontend run test:unit -- work`

Run: `npm --prefix frontend run test:e2e -- --grep work`

Expected: FAIL because the Work routes and graph components do not exist.

- [ ] **Step 3: Implement the `Work` pages and XYFlow renderer**

Use `@xyflow/svelte` directly:

```ts
import { SvelteFlow, Background, Controls, MiniMap } from '@xyflow/svelte';
import '@xyflow/svelte/dist/style.css';
```

Use a dedicated layout helper:

```ts
export function layoutGraph(nodes: Node[], edges: Edge[]) {
	// keep dagre-style top-to-bottom orchestration readability
}
```

Subscribe to live updates with `EventSource` against `/api/work/{id}/events`. Keep the right inspector fed by `/api/work/{id}/nodes/{node_id}`.

- [ ] **Step 4: Re-run unit tests, e2e tests, and frontend type-check**

Run: `npm --prefix frontend run test:unit`

Run: `npm --prefix frontend run test:e2e -- --grep work`

Run: `npm --prefix frontend run check`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/components/graph/RunGraph.svelte frontend/src/lib/components/graph/RunGraphNode.svelte frontend/src/lib/components/graph/layout.ts frontend/src/routes/work/+page.svelte frontend/src/routes/work/[runId]/+page.svelte frontend/src/lib/components/graph/RunGraph.test.ts frontend/src/routes/work/page.test.ts frontend/e2e/work.spec.ts
git commit -m "feat: implement work cockpit with xyflow"
```

## Task 9: Implement Team, Knowledge, Recover, Conversations, Automate, History, Settings, Login, And Onboarding

**Files:**
- Create: `frontend/src/routes/login/+page.svelte`
- Create: `frontend/src/routes/onboarding/+page.svelte`
- Create: `frontend/src/routes/team/+page.svelte`
- Create: `frontend/src/routes/knowledge/+page.svelte`
- Create: `frontend/src/routes/recover/+page.svelte`
- Create: `frontend/src/routes/conversations/+page.svelte`
- Create: `frontend/src/routes/automate/+page.svelte`
- Create: `frontend/src/routes/history/+page.svelte`
- Create: `frontend/src/routes/settings/+page.svelte`
- Create: `frontend/e2e/auth.spec.ts`

- [ ] **Step 1: Write failing tests for auth and the remaining user surfaces**

```ts
test('login flow redirects to /work after successful auth', async ({ page }) => {});
test('team page edits the active setup', async ({ page }) => {});
test('automate page creates and triggers a schedule', async ({ page }) => {});
```

- [ ] **Step 2: Run the targeted frontend tests**

Run: `npm --prefix frontend run test:e2e -- --grep 'auth|team|automate'`

Expected: FAIL because those routes are still missing.

- [ ] **Step 3: Implement each route against its browser API without reintroducing template-era grouping**

Page mapping:

```text
/login           -> auth session and login mutation
/onboarding      -> onboarding step API
/team            -> profile selection and team topology editor
/knowledge       -> memory list/edit/forget
/recover         -> approvals, route repair, delivery retries
/conversations   -> session mailbox, route state, delivery state
/automate        -> schedules, status, run-now controls
/history         -> replay and intervention evidence
/settings        -> machine settings, password, devices
```

Use the `DESIGN.md` shell and language rules. The UI should talk about the user's work first, then expose system nouns inside inspectors and evidence cards.

- [ ] **Step 4: Re-run e2e tests and frontend type-check**

Run: `npm --prefix frontend run test:e2e`

Run: `npm --prefix frontend run check`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/routes/login/+page.svelte frontend/src/routes/onboarding/+page.svelte frontend/src/routes/team/+page.svelte frontend/src/routes/knowledge/+page.svelte frontend/src/routes/recover/+page.svelte frontend/src/routes/conversations/+page.svelte frontend/src/routes/automate/+page.svelte frontend/src/routes/history/+page.svelte frontend/src/routes/settings/+page.svelte frontend/e2e/auth.spec.ts
git commit -m "feat: implement remaining user-first surfaces"
```

## Task 10: Delete The Legacy Template System And Update Docs

**Files:**
- Delete: `internal/web/assets.go`
- Delete: `internal/web/design_system_test.go`
- Delete: `internal/web/templates/*.html`
- Delete: `internal/web/static/vendor/cytoscape.min.js`
- Delete: `internal/web/static/vendor/cytoscape-dagre.js`
- Delete: `internal/web/static/vendor/dagre.min.js`
- Modify: `internal/web/server_test.go`
- Modify: `README.md`
- Modify: `docs/system.md`
- Modify: `CONTRIBUTING.md`

- [ ] **Step 1: Write failing cleanup and contract tests**

```go
func TestLegacyTemplateAssetsAreNotReferenced(t *testing.T) {}

func TestServerRegistersOnlySPAAndBrowserAPIPageRoutes(t *testing.T) {}
```

- [ ] **Step 2: Run the cleanup tests**

Run: `go test ./internal/web -run 'Test(LegacyTemplateAssetsAreNotReferenced|ServerRegistersOnlySPAAndBrowserAPIPageRoutes)' -count=1`

Expected: FAIL because template files and old route groups still exist.

- [ ] **Step 3: Delete the legacy files and update repo documentation**

Docs updates must cover:

```text
- SvelteKit frontend workspace under frontend/
- Go still serves the operator UI
- New top-level route map
- New contributor commands: make ui-install, make ui-build, make ui-check, make ui-test
```

- [ ] **Step 4: Run the full verification matrix**

Run: `npm --prefix frontend run check`

Run: `npm --prefix frontend run test:unit`

Run: `npm --prefix frontend run test:e2e`

Run: `go test ./...`

Run: `go test -cover ./...`

Run: `go vet ./...`

Expected: PASS and total Go coverage stays at or above `70%`.

- [ ] **Step 5: Commit**

```bash
git add README.md docs/system.md CONTRIBUTING.md internal/web/server.go
git add -u internal/web
git commit -m "refactor: remove legacy template web ui"
```

## Final Verification Checklist

- [ ] `npm --prefix frontend run build`
- [ ] `npm --prefix frontend run check`
- [ ] `npm --prefix frontend run test:unit`
- [ ] `npm --prefix frontend run test:e2e`
- [ ] `go test ./...`
- [ ] `go test -cover ./...`
- [ ] `go vet ./...`
- [ ] manual smoke: login -> onboarding -> work -> recover -> automate -> settings

## Notes For The Implementer

- The current `run_graph.go` contract is already close to what `@xyflow/svelte` wants. Reuse the graph DTOs unless the frontend exposes a concrete gap.
- `Automate` is new browser functionality, not a reskin. Wire the schedule service into `web.Options` through a small consumer-side interface implemented by `*app.App`.
- `History` should not become a second `Work` page. Keep it evidence-first: replay, interventions, approvals, and delivery outcomes.
- Delete the legacy UI only after the SPA and browser APIs are fully green.
- The worktree is already dirty outside the design docs. Do not revert unrelated user changes while implementing this plan.
