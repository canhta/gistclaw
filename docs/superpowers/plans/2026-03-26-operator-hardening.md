# Operator Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the shipped `gistclaw` runtime with security auditing, connector supervision, team profiles, and storage-health reporting without changing the journal-first architecture.

**Architecture:** Add three small operator-facing seams around the current kernel: `internal/security` for deployment risk checks, `internal/app` supervision helpers for connector health, and `internal/store` health reporting for storage visibility. Extend the existing Team page and runtime team resolution to support named per-project team profiles, but keep onboarding out of scope for the first pass because staged onboarding changes are already in flight. All write paths remain runtime-owned or store-owned; no alternate state stores are introduced.

**Tech Stack:** Go 1.25+, SQLite via `internal/store`, stdlib `context`/`database/sql`/`net/http`/`os`, Go `testing` package, existing server-rendered `html/template` UI

---

## Guardrails

- Stay on `main`. Do not create branches or worktrees.
- TDD always: every behavior change starts with a failing test.
- Do not touch onboarding flows in the team-profile slice.
- Do not introduce any JSON or file-backed side store for runtime state.
- Do not add new migration files; if schema changes are required, edit `internal/store/migrations/001_init.sql` in place.
- Keep interfaces in the consuming package.

## File Map

**Create**

- `internal/security/audit.go` — audit finding model and audit runner
- `internal/security/audit_test.go` — table-driven coverage for risk findings
- `cmd/gistclaw/security.go` — `gistclaw security audit` CLI surface
- `cmd/gistclaw/security_test.go` — CLI coverage for audit output and exit codes
- `internal/app/connector_supervisor.go` — optional connector-health interfaces and supervisor loop
- `internal/app/connector_supervisor_test.go` — supervision, cooldown, and restart-budget coverage
- `internal/connectors/telegram/health.go` — telegram health state helpers
- `internal/connectors/telegram/health_test.go` — telegram health calculations
- `internal/connectors/whatsapp/health.go` — webhook freshness and delivery health helpers
- `internal/connectors/whatsapp/health_test.go` — whatsapp health calculations
- `internal/runtime/team_profiles.go` — active-profile selection and per-project profile resolution
- `internal/runtime/team_profiles_test.go` — runtime profile selection coverage
- `internal/teams/profile.go` — filesystem helpers for listing, cloning, creating, and deleting profiles
- `internal/teams/profile_test.go` — profile filesystem behavior coverage
- `internal/store/health.go` — database and filesystem health summary helpers
- `internal/store/health_test.go` — size, backup age, and warning coverage

**Modify**

- `cmd/gistclaw/main.go` — register `security` command and optionally enrich `inspect status`
- `cmd/gistclaw/main_test.go` — command usage coverage
- `cmd/gistclaw/doctor.go` — consume security, connector, and storage health summaries
- `cmd/gistclaw/doctor_test.go` — verify richer health output
- `internal/app/bootstrap.go` — construct connector supervisor dependencies
- `internal/app/lifecycle.go` — start and stop connector supervision alongside scheduler/connectors
- `internal/app/lifecycle_test.go` — lifecycle coverage for supervision startup/shutdown
- `internal/connectors/telegram/connector.go` — publish telegram health state
- `internal/connectors/telegram/connector_test.go` — connector health/state behavior
- `internal/connectors/whatsapp/connector.go` — publish outbound health state
- `internal/connectors/whatsapp/inbound.go` — publish inbound webhook freshness
- `internal/connectors/whatsapp/inbound_test.go` — inbound health behavior
- `internal/runtime/team.go` — resolve active team profile instead of hardcoded `default`
- `internal/runtime/projects.go` — persist active team profile per project
- `internal/runtime/projects_test.go` — active-project profile setting coverage
- `internal/web/routes_team.go` — add profile selection and management actions
- `internal/web/templates/team.html` — profile selector and actions UI
- `internal/web/server_test.go` — team page profile workflows
- `README.md` — document new security and operator-hardening commands
- `docs/system.md` — update shipped operator surfaces
- `docs/roadmap.md` — move completed hardening work out of near-term gaps

## Not In Scope

- onboarding integration for team profiles
- new connectors beyond Telegram and WhatsApp
- plugin installation UX
- automatic pruning of the append-only journal
- WebSocket control plane or remote gateway features

## Execution Order

1. security audit domain and CLI
2. connector supervision seam
3. telegram and whatsapp health reporting
4. doctor integration for connector and audit summaries
5. team profile runtime resolution
6. team page profile actions
7. storage health reporting
8. docs and end-to-end verification

### Task 1: Add Security Audit Domain

**Files:**
- Create: `internal/security/audit.go`
- Test: `internal/security/audit_test.go`

- [ ] **Step 1: Write the failing audit-domain tests**

```go
func TestAudit_FlagsLoopbackWithoutAdminToken(t *testing.T) {
	cfg := app.Config{}
	report := security.RunAudit(security.Input{Config: cfg, AdminTokenPresent: false})
	if !containsFinding(report.Findings, "admin_token.missing") {
		t.Fatalf("expected missing admin token finding, got %#v", report.Findings)
	}
}

func TestAudit_FlagsUnsafeWebBind(t *testing.T) {
	cfg := app.Config{}
	cfg.Web.ListenAddr = "0.0.0.0:8080"
	report := security.RunAudit(security.Input{Config: cfg, AdminTokenPresent: true})
	if !containsFinding(report.Findings, "web.listen_addr.exposed") {
		t.Fatalf("expected exposed web finding, got %#v", report.Findings)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/security -run TestAudit_ -v`
Expected: FAIL because `internal/security` does not exist yet

- [ ] **Step 3: Write minimal audit types and runner**

```go
type Severity string

const (
	SeverityInfo Severity = "info"
	SeverityWarn Severity = "warn"
	SeverityFail Severity = "fail"
)

type Finding struct {
	ID          string
	Severity    Severity
	Title       string
	Detail      string
	Remediation string
}
```

- [ ] **Step 4: Implement checks for current shipped risks**

Implement checks for:
- missing admin token
- non-loopback web bind
- missing provider config
- invalid research config
- enabled MCP tool with missing binary

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/security -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/security/audit.go internal/security/audit_test.go
git commit -m "feat: add security audit domain"
```

### Task 2: Add Security Audit CLI Surface

**Files:**
- Create: `cmd/gistclaw/security.go`
- Test: `cmd/gistclaw/security_test.go`
- Modify: `cmd/gistclaw/main.go`
- Modify: `cmd/gistclaw/main_test.go`

- [ ] **Step 1: Write the failing CLI tests**

```go
func TestRunSecurityAudit_PrintsFindingsAndFailsOnUnsafeConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"security", "audit"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit for unsafe default config")
	}
	if !strings.Contains(stdout.String(), "admin_token.missing") {
		t.Fatalf("expected finding in output, got %s", stdout.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/gistclaw -run TestRunSecurityAudit_ -v`
Expected: FAIL because `security` subcommand is unknown

- [ ] **Step 3: Add `security audit` command routing**

Add usage text to [main.go](/Users/canh/Projects/OSS/gistclaw/cmd/gistclaw/main.go) and route:

```go
case "security":
	return runSecurity(configPath, args[1:], stdout, stderr)
```

- [ ] **Step 4: Implement CLI runner**

The runner should:
- load config
- inspect current admin-token state from SQLite if possible
- run `internal/security` audit
- print structured lines
- exit non-zero when any `fail` finding exists

- [ ] **Step 5: Run targeted tests**

Run: `go test ./cmd/gistclaw -run 'TestRunSecurityAudit_|TestRun_Help' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/gistclaw/security.go cmd/gistclaw/security_test.go cmd/gistclaw/main.go cmd/gistclaw/main_test.go
git commit -m "feat: add security audit command"
```

### Task 3: Add Connector Supervision Seam

**Files:**
- Create: `internal/app/connector_supervisor.go`
- Test: `internal/app/connector_supervisor_test.go`
- Modify: `internal/app/lifecycle.go`
- Modify: `internal/app/lifecycle_test.go`

- [ ] **Step 1: Write the failing supervisor tests**

```go
func TestConnectorSupervisor_RestartsDegradedConnectorWithinBudget(t *testing.T) {
	conn := &stubHealthConnector{degraded: true}
	supervisor := newConnectorSupervisor([]model.Connector{conn}, SupervisorConfig{
		CheckInterval: time.Millisecond,
		MaxRestarts:   1,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go supervisor.Run(ctx)
	waitForRestart(t, conn)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app -run TestConnectorSupervisor_ -v`
Expected: FAIL because supervisor types do not exist

- [ ] **Step 3: Define optional health interfaces in `internal/app`**

Use consumer-owned interfaces, for example:

```go
type connectorHealthReporter interface {
	HealthSnapshot() model.ConnectorHealthSnapshot
}

type connectorRestarter interface {
	Restart(context.Context) error
}
```

- [ ] **Step 4: Implement app-owned supervision loop**

The loop should support:
- startup grace
- check interval
- cooldown between restarts
- restart budget per hour
- clean shutdown on app context cancel

- [ ] **Step 5: Wire supervisor into app lifecycle**

Start the supervisor from [lifecycle.go](/Users/canh/Projects/OSS/gistclaw/internal/app/lifecycle.go) after connectors are constructed, and stop it when `Start` exits.

- [ ] **Step 6: Run targeted tests**

Run: `go test ./internal/app -run 'TestConnectorSupervisor_|TestLifecycle_' -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/app/connector_supervisor.go internal/app/connector_supervisor_test.go internal/app/lifecycle.go internal/app/lifecycle_test.go
git commit -m "feat: add connector supervision seam"
```

### Task 4: Add Telegram Health Reporting

**Files:**
- Create: `internal/connectors/telegram/health.go`
- Test: `internal/connectors/telegram/health_test.go`
- Modify: `internal/connectors/telegram/connector.go`
- Modify: `internal/connectors/telegram/connector_test.go`

- [ ] **Step 1: Write the failing telegram health tests**

```go
func TestTelegramHealth_MarksStalePollLoopAsDegraded(t *testing.T) {
	state := newHealthState(func() time.Time { return now })
	state.MarkPollSuccess(now.Add(-10 * time.Minute))
	if snapshot := state.Snapshot(now); snapshot.State != "degraded" {
		t.Fatalf("expected degraded, got %#v", snapshot)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/connectors/telegram -run TestTelegramHealth_ -v`
Expected: FAIL because health helpers do not exist

- [ ] **Step 3: Implement telegram health state**

Track:
- last successful poll/update receipt
- last drain success/failure
- consecutive startup failures

- [ ] **Step 4: Publish health from the connector**

Update [connector.go](/Users/canh/Projects/OSS/gistclaw/internal/connectors/telegram/connector.go) so the connector updates health timestamps during normal loop progress and exposes a snapshot method for the app supervisor.

- [ ] **Step 5: Run targeted tests**

Run: `go test ./internal/connectors/telegram -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/connectors/telegram/health.go internal/connectors/telegram/health_test.go internal/connectors/telegram/connector.go internal/connectors/telegram/connector_test.go
git commit -m "feat: add telegram connector health reporting"
```

### Task 5: Add WhatsApp Health Reporting

**Files:**
- Create: `internal/connectors/whatsapp/health.go`
- Test: `internal/connectors/whatsapp/health_test.go`
- Modify: `internal/connectors/whatsapp/connector.go`
- Modify: `internal/connectors/whatsapp/inbound.go`
- Modify: `internal/connectors/whatsapp/inbound_test.go`

- [ ] **Step 1: Write the failing whatsapp health tests**

```go
func TestWhatsAppHealth_MarksMissingWebhookActivityAsDegraded(t *testing.T) {
	state := newHealthState(func() time.Time { return now })
	state.MarkWebhook(now.Add(-30 * time.Minute))
	if snapshot := state.Snapshot(now); snapshot.State != "degraded" {
		t.Fatalf("expected degraded, got %#v", snapshot)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/connectors/whatsapp -run TestWhatsAppHealth_ -v`
Expected: FAIL because health helpers do not exist

- [ ] **Step 3: Implement shared health state**

The health state should track:
- last inbound webhook timestamp
- last successful outbound drain
- last outbound failure

- [ ] **Step 4: Thread health state through connector and webhook handler**

Update [inbound.go](/Users/canh/Projects/OSS/gistclaw/internal/connectors/whatsapp/inbound.go) so webhook traffic updates freshness without creating a second runtime path.

- [ ] **Step 5: Run targeted tests**

Run: `go test ./internal/connectors/whatsapp -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/connectors/whatsapp/health.go internal/connectors/whatsapp/health_test.go internal/connectors/whatsapp/connector.go internal/connectors/whatsapp/inbound.go internal/connectors/whatsapp/inbound_test.go
git commit -m "feat: add whatsapp connector health reporting"
```

### Task 6: Surface Audit And Connector Health In Doctor

**Files:**
- Modify: `cmd/gistclaw/doctor.go`
- Modify: `cmd/gistclaw/doctor_test.go`
- Modify: `internal/model/types.go`
- Modify: `internal/web/routes_routes_deliveries.go`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: Write the failing doctor and web tests**

```go
func TestDoctor_PrintsSecurityAndConnectorSections(t *testing.T) {
	output := runDoctorForTest(t)
	for _, want := range []string{"security", "connector", "storage"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in doctor output", want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/gistclaw ./internal/web -run 'TestDoctor_|TestRoutesDeliveries' -v`
Expected: FAIL because richer sections are not printed yet

- [ ] **Step 3: Extend doctor output**

Consume:
- security audit summary
- current connector health summary
- existing scheduler health

- [ ] **Step 4: Expose degraded connector state on recover surfaces**

Reuse the existing Routes & Deliveries surface instead of creating a new page.

- [ ] **Step 5: Run targeted tests**

Run: `go test ./cmd/gistclaw ./internal/web -run 'TestDoctor_|TestRoutesDeliveries' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/gistclaw/doctor.go cmd/gistclaw/doctor_test.go internal/model/types.go internal/web/routes_routes_deliveries.go internal/web/server_test.go
git commit -m "feat: surface security and connector health"
```

### Task 7: Add Runtime Team Profile Resolution

**Files:**
- Create: `internal/runtime/team_profiles.go`
- Test: `internal/runtime/team_profiles_test.go`
- Create: `internal/teams/profile.go`
- Test: `internal/teams/profile_test.go`
- Modify: `internal/runtime/team.go`
- Modify: `internal/runtime/projects.go`
- Modify: `internal/runtime/projects_test.go`

- [ ] **Step 1: Write the failing profile-resolution tests**

```go
func TestRuntime_TeamConfigUsesActiveProfileForProject(t *testing.T) {
	rt := newTestRuntime(t)
	setActiveProfile(t, rt.store, "project-1", "review")
	cfg, err := rt.TeamConfig(ctx)
	if err != nil {
		t.Fatalf("TeamConfig: %v", err)
	}
	if cfg.Name != "Review Team" {
		t.Fatalf("expected review profile, got %q", cfg.Name)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime ./internal/teams -run 'TestRuntime_.*Profile|TestProfiles_' -v`
Expected: FAIL because profile helpers do not exist

- [ ] **Step 3: Add profile filesystem helpers**

Support:
- list profiles
- create from shipped default
- clone existing profile
- delete non-active profile

- [ ] **Step 4: Resolve active profile per project**

Persist the active profile name beside project state and update [team.go](/Users/canh/Projects/OSS/gistclaw/internal/runtime/team.go) to load `.gistclaw/teams/<profile>/team.yaml`.

- [ ] **Step 5: Keep run snapshot semantics unchanged**

Add regression coverage that changing the active profile only affects future runs.

- [ ] **Step 6: Run targeted tests**

Run: `go test ./internal/runtime ./internal/teams -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/runtime/team_profiles.go internal/runtime/team_profiles_test.go internal/teams/profile.go internal/teams/profile_test.go internal/runtime/team.go internal/runtime/projects.go internal/runtime/projects_test.go
git commit -m "feat: add per-project team profiles"
```

### Task 8: Add Team Page Profile Actions

**Files:**
- Modify: `internal/web/routes_team.go`
- Modify: `internal/web/templates/team.html`
- Modify: `internal/web/server_test.go`

- [ ] **Step 1: Write the failing Team-page tests**

```go
func TestTeamPage_RendersProfileSelectorAndActions(t *testing.T) {
	rr := getTeamPage(t)
	for _, want := range []string{"Active Profile", "Clone Profile", "Create Profile"} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/web -run 'TestTeamPage_|TestTeamProfile' -v`
Expected: FAIL because profile UI and handlers do not exist

- [ ] **Step 3: Extend Team routes**

Add handlers for:
- select active profile
- create profile
- clone profile
- delete profile

- [ ] **Step 4: Extend Team template**

Add a profile chooser and management actions above the existing editor while keeping import/export and save actions visually separate.

- [ ] **Step 5: Run targeted tests**

Run: `go test ./internal/web -run 'TestTeamPage_|TestTeamProfile' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/web/routes_team.go internal/web/templates/team.html internal/web/server_test.go
git commit -m "feat: add team profile management"
```

### Task 9: Add Store Health Reporting

**Files:**
- Create: `internal/store/health.go`
- Test: `internal/store/health_test.go`
- Modify: `cmd/gistclaw/doctor.go`
- Modify: `cmd/gistclaw/doctor_test.go`
- Modify: `cmd/gistclaw/main.go`
- Modify: `cmd/gistclaw/run_test.go`
- Modify: `cmd/gistclaw/backup.go`

- [ ] **Step 1: Write the failing store-health tests**

```go
func TestHealth_ReportIncludesDBSizeAndBackupAge(t *testing.T) {
	report, err := store.LoadHealth(dbPath, now)
	if err != nil {
		t.Fatalf("LoadHealth: %v", err)
	}
	if report.DatabaseBytes == 0 {
		t.Fatal("expected non-zero database size")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store -run TestHealth_ -v`
Expected: FAIL because `LoadHealth` does not exist

- [ ] **Step 3: Implement store health summary**

Collect:
- database file size
- WAL size if present
- free disk bytes
- newest `.db.bak` timestamp
- warning flags for low disk or missing recent backup

- [ ] **Step 4: Surface storage health**

Add the storage summary to `doctor`, and extend `inspect status` if it still fits the output without becoming noisy.

- [ ] **Step 5: Keep maintenance explicit**

If you add a checkpoint or vacuum helper, expose it as an explicit operator command later; do not auto-run destructive maintenance here.

- [ ] **Step 6: Run targeted tests**

Run: `go test ./internal/store ./cmd/gistclaw -run 'TestHealth_|TestDoctor_|TestInspectStatus' -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/store/health.go internal/store/health_test.go cmd/gistclaw/doctor.go cmd/gistclaw/doctor_test.go cmd/gistclaw/main.go cmd/gistclaw/run_test.go cmd/gistclaw/backup.go
git commit -m "feat: add storage health reporting"
```

### Task 10: Update Docs And Verify End To End

**Files:**
- Modify: `README.md`
- Modify: `docs/system.md`
- Modify: `docs/roadmap.md`
- Modify: `docs/superpowers/specs/2026-03-26-operator-hardening-design.md`

- [ ] **Step 1: Update docs for shipped surfaces**

Document:
- `gistclaw security audit`
- connector supervision and degraded-state reporting
- team profiles
- storage health in doctor/inspect

- [ ] **Step 2: Run focused verification**

Run:

```bash
go test ./internal/security ./internal/app ./internal/connectors/telegram ./internal/connectors/whatsapp ./internal/runtime ./internal/teams ./internal/store ./internal/web ./cmd/gistclaw
```

Expected: PASS

- [ ] **Step 3: Run full verification**

Run:

```bash
go test ./...
go test -cover ./...
go vet ./...
```

Expected:
- all tests pass
- coverage remains at or above `70%`
- `go vet` passes

- [ ] **Step 4: Commit**

```bash
git add README.md docs/system.md docs/roadmap.md docs/superpowers/specs/2026-03-26-operator-hardening-design.md docs/superpowers/plans/2026-03-26-operator-hardening.md
git commit -m "docs: add operator hardening rollout"
```

## Review Notes

- Do not fold Task 7 into onboarding work. The first team-profile pass is a Team-page feature.
- If schema changes are needed for active profile persistence, make them in `001_init.sql` and add migration coverage before runtime code.
- If connector health cannot be generalized cleanly, keep the optional interfaces small and use type assertions from `internal/app` rather than widening `model.Connector`.

## Execution Handoff

Plan complete. Recommended execution mode: subagent-driven per task, because the slices are naturally reviewable and the repo already has staged changes that should stay isolated from the team-profile work until its turn.
