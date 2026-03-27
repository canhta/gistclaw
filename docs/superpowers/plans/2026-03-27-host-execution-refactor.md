# Host-Wide Execution Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace workspace-root confinement with a storage-root plus execution-target authority model, so agents can operate across the local machine with explicit policy controls and no backward-compatibility paths.

**Architecture:** GistClaw keeps a single operator-owned `storage_root` for journals, memory, artifacts, approvals, and team state. Each run resolves an `ExecutionTarget` and `AuthorityEnvelope` from project metadata, machine policy, and run context. Tools declare intent up front and are allowed, denied, or approval-gated based on capabilities and sensitivity classes instead of `workspace_root`.

**Tech Stack:** Go 1.25+, SQLite via `modernc.org/sqlite`, stdlib `net/http`, Go `testing`, existing SSE/web runtime.

---

## Non-Negotiables

- No feature branches, worktrees, shims, compatibility aliases, or dual-path runtime logic.
- Rewrite `internal/store/migrations/001_init.sql` in place and drop local DBs after the schema lands.
- Delete `workspace_root` and `target_path` everywhere; do not retain compatibility columns.
- `--dangerously-skip-permissions` disables interactive prompts only.
- `--elevated-host-access` widens access to sensitive/system classes; it is a separate switch.
- `auto_approve + elevated` is local-operator-only. Remote connectors must not run in that combination.

### Task 1: Reset the persistent model and schema

**Files:**
- Modify: `internal/store/migrations/001_init.sql`
- Modify: `internal/store/migrate.go`
- Modify: `internal/model/types.go`
- Test: `internal/store/migrate_test.go`
- Test: `internal/store/migrate_auth_test.go`
- Test: `internal/model/types_test.go`

- [ ] **Step 1: Write failing schema tests for the new columns and removed legacy fields**

Add assertions that the schema contains `storage_root`, `approval_mode`, `host_access_mode`, `primary_path`, `cwd`, `authority_json`, and `binding_json`, and that it no longer contains `workspace_root` or `target_path`.

- [ ] **Step 2: Run the schema tests to verify they fail**

Run: `go test ./internal/store ./internal/model -run 'TestMigrate|TestAuth|TestTypes'`
Expected: FAIL because the old schema and model types still expose `workspace_root` and `target_path`.

- [ ] **Step 3: Rewrite the schema in place**

Replace the current tables with these semantics:

```sql
settings(key TEXT PRIMARY KEY, value TEXT NOT NULL)
projects(id, name, primary_path, roots_json, policy_json, source, created_at, last_used_at)
runs(..., project_id, cwd, authority_json, execution_snapshot_json, ...)
approvals(..., binding_json, fingerprint, status, resolved_by, created_at, resolved_at)
schedules(..., project_id, cwd, authority_json, schedule_kind, ...)
```

Do not keep `workspace_root` or `target_path` columns anywhere.

- [ ] **Step 4: Update the Go model types to match the new schema exactly**

Replace old fields with:

```go
type Project struct {
	ID          string
	Name        string
	PrimaryPath string
	RootsJSON   string
	PolicyJSON  string
}
```

Add equivalent `CWD`, `AuthorityJSON`, and `BindingJSON` fields where runs, schedules, and approvals need them. Remove `WorkspaceRoot` and `TargetPath` from all affected structs.

- [ ] **Step 5: Run the schema/model tests until they pass**

Run: `go test ./internal/store ./internal/model -run 'TestMigrate|TestAuth|TestTypes'`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/store/migrations/001_init.sql internal/store/migrate.go internal/model/types.go internal/store/migrate_test.go internal/store/migrate_auth_test.go internal/model/types_test.go
git commit -m "refactor: replace workspace schema with host execution model"
```

### Task 2: Replace config and CLI semantics with storage root and permission modes

**Files:**
- Modify: `internal/app/config.go`
- Modify: `internal/app/config_test.go`
- Modify: `cmd/gistclaw/main.go`
- Modify: `cmd/gistclaw/doctor.go`
- Modify: `cmd/gistclaw/schedule.go`
- Test: `cmd/gistclaw/doctor_test.go`
- Test: `cmd/gistclaw/schedule_test.go`
- Test: `internal/app/lifecycle_test.go`
- Test: `internal/app/commands_test.go`

- [ ] **Step 1: Write failing config tests for the new fields**

Add coverage for:

```yaml
storage_root: /Users/me/.gistclaw
permissions:
  approval_mode: prompt
  host_access_mode: standard
```

Reject unknown values. Reject missing `storage_root`.

- [ ] **Step 2: Run the config and CLI tests to verify they fail**

Run: `go test ./internal/app ./cmd/gistclaw -run 'TestConfig|TestDoctor|TestSchedule|TestLifecycle|TestCommands'`
Expected: FAIL because config parsing and CLI output still depend on `workspace_root`.

- [ ] **Step 3: Replace `workspace_root` config with the new permission model**

The config struct should look like:

```go
type PermissionsConfig struct {
	ApprovalMode   string `yaml:"approval_mode"`
	HostAccessMode string `yaml:"host_access_mode"`
}

type Config struct {
	StorageRoot string            `yaml:"storage_root"`
	Permissions PermissionsConfig `yaml:"permissions"`
}
```

Add CLI wiring for `--dangerously-skip-permissions` and `--elevated-host-access`.

- [ ] **Step 4: Rewrite doctor and schedule CLI output**

`gistclaw doctor` should print `storage_root`, `approval_mode`, and `host_access_mode`. Schedule commands should stop accepting or printing `workspace_root`; they should use `project_id`, `cwd`, and authority settings instead.

- [ ] **Step 5: Re-run the config and CLI tests**

Run: `go test ./internal/app ./cmd/gistclaw -run 'TestConfig|TestDoctor|TestSchedule|TestLifecycle|TestCommands'`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/app/config.go internal/app/config_test.go cmd/gistclaw/main.go cmd/gistclaw/doctor.go cmd/gistclaw/schedule.go cmd/gistclaw/doctor_test.go cmd/gistclaw/schedule_test.go internal/app/lifecycle_test.go internal/app/commands_test.go
git commit -m "refactor: add storage root and permission modes"
```

### Task 3: Add authority, execution, and location primitives

**Files:**
- Create: `internal/authority/types.go`
- Create: `internal/authority/policy.go`
- Create: `internal/authority/sensitivity.go`
- Create: `internal/authority/approval_binding.go`
- Create: `internal/authority/policy_test.go`
- Create: `internal/authority/approval_binding_test.go`
- Create: `internal/execution/target.go`
- Create: `internal/execution/resolver.go`
- Create: `internal/execution/resolver_test.go`
- Create: `internal/locations/registry.go`
- Create: `internal/locations/registry_test.go`

- [ ] **Step 1: Write failing tests for location resolution, sensitivity classes, and the mode matrix**

Cover:
- explicit user path wins
- sticky run `cwd` wins over project default
- project `primary_path` wins over fallback home directory
- sensitive classes are denied in `standard`
- `auto_approve` removes prompts but not standard-mode denials
- `elevated` widens access without changing prompt behavior

- [ ] **Step 2: Run the new package tests to verify they fail**

Run: `go test ./internal/authority ./internal/execution ./internal/locations`
Expected: FAIL because the packages do not exist yet.

- [ ] **Step 3: Implement the core types**

Define:

```go
type ApprovalMode string
type HostAccessMode string

type ExecutionTarget struct {
	CWD        string
	ReadRoots  []PathScope
	WriteRoots []PathScope
	ExecHost   string
}

type AuthorityEnvelope struct {
	ApprovalMode   ApprovalMode
	HostAccessMode HostAccessMode
	Capabilities   CapabilitySet
}
```

Define named roots in `internal/locations`: `home`, `projects`, `desktop`, `documents`, `downloads`, `storage`, `primary_path`.

- [ ] **Step 4: Implement deterministic execution resolution**

Resolver order:
1. explicit path from the current task
2. sticky `cwd` from the current thread/run
3. project `primary_path`
4. matching named root from project metadata
5. user home directory

Do not infer from `workspace_root`; it no longer exists.

- [ ] **Step 5: Implement approval binding generation**

Binding fingerprint inputs must include tool name, normalized argv, `cwd`, read roots, write roots, mutating flag, network flag, and concrete operands when known.

- [ ] **Step 6: Re-run the new package tests**

Run: `go test ./internal/authority ./internal/execution ./internal/locations`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/authority internal/execution internal/locations
git commit -m "refactor: add host authority and execution primitives"
```

### Task 4: Rewrite runtime, session, and project scoping around execution targets

**Files:**
- Modify: `internal/runtime/projects.go`
- Modify: `internal/runtime/projects_test.go`
- Modify: `internal/runtime/runs.go`
- Modify: `internal/runtime/runs_test.go`
- Modify: `internal/runtime/approval_flow_test.go`
- Modify: `internal/runtime/acceptance_test.go`
- Modify: `internal/runtime/context.go`
- Delete: `internal/runtime/workspace_context.go`
- Modify: `internal/projectscope/scope.go`
- Modify: `internal/projectscope/scope_test.go`
- Modify: `internal/sessions/service.go`
- Modify: `internal/sessions/service_test.go`
- Modify: `internal/app/commands.go`

- [ ] **Step 1: Write failing runtime tests that use `cwd` and `authority_json` instead of `workspace_root`**

Update acceptance and approval-flow tests so a run resolves an `ExecutionTarget` and produces approval bindings without touching any workspace field.

- [ ] **Step 2: Run the runtime/session tests to verify they fail**

Run: `go test ./internal/runtime ./internal/projectscope ./internal/sessions ./internal/app -run 'TestProjects|TestRuns|TestApprovalFlow|TestAcceptance|TestScope|TestService|TestCommands'`
Expected: FAIL because runtime commands, sessions, and scope queries still require `workspace_root`.

- [ ] **Step 3: Rewrite project registration and activation**

Projects should persist `primary_path`, `roots_json`, and `policy_json`. Do not auto-`git init` a folder just because it becomes the active project. A project is metadata, not a repo boundary.

- [ ] **Step 4: Rewrite run preparation and approval replay**

`prepareStartRun` should:
- resolve `ExecutionTarget`
- resolve `AuthorityEnvelope`
- persist `cwd` and `authority_json`
- pass the full execution context to tool invocation

Approval replay should reconstruct the exact binding, not a workspace path.

- [ ] **Step 5: Remove workspace-based project scoping**

`internal/projectscope/scope.go` must use project IDs only. Delete the fallback SQL that matches on `workspace_root`.

- [ ] **Step 6: Re-run the runtime/session tests**

Run: `go test ./internal/runtime ./internal/projectscope ./internal/sessions ./internal/app -run 'TestProjects|TestRuns|TestApprovalFlow|TestAcceptance|TestScope|TestService|TestCommands'`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/runtime/projects.go internal/runtime/projects_test.go internal/runtime/runs.go internal/runtime/runs_test.go internal/runtime/approval_flow_test.go internal/runtime/acceptance_test.go internal/runtime/context.go internal/projectscope/scope.go internal/projectscope/scope_test.go internal/sessions/service.go internal/sessions/service_test.go internal/app/commands.go
git rm internal/runtime/workspace_context.go
git commit -m "refactor: move runtime from workspace roots to execution targets"
```

### Task 5: Replace workspace-bound tools with host-scoped tools

**Files:**
- Modify: `internal/tools/context.go`
- Modify: `internal/tools/approvals.go`
- Modify: `internal/tools/policy.go`
- Create: `internal/tools/fs.go`
- Create: `internal/tools/exec.go`
- Create: `internal/tools/git.go`
- Delete: `internal/tools/repo_fs.go`
- Delete: `internal/tools/repo_exec.go`
- Delete: `internal/tools/repo_git.go`
- Delete: `internal/tools/path_guard.go`
- Delete: `internal/tools/workspace.go`
- Modify: `internal/tools/tools_test.go`
- Modify: `internal/tools/repo_tools_test.go`
- Modify: `internal/tools/repo_exec_test.go`
- Modify: `internal/tools/coder_exec_test.go`

- [ ] **Step 1: Write failing tool tests against the new invocation context**

Update the tests so tools receive:

```go
InvocationContext{
	StorageRoot: "...",
	Execution: ExecutionTarget{...},
	Authority: AuthorityEnvelope{...},
}
```

Add coverage for:
- reading from `~/Projects/...`
- writing to `Desktop` with approval required
- denial on `~/.ssh`
- shell exec using resolved `cwd`

- [ ] **Step 2: Run the tool tests to verify they fail**

Run: `go test ./internal/tools -run 'Test.*'`
Expected: FAIL because the tool layer still depends on `workspaceRootFromContext`.

- [ ] **Step 3: Rewrite the tool contract**

Replace `workspaceRootFromContext` with helpers that read `ExecutionTarget` and `AuthorityEnvelope`. Delete `ErrWorkspaceRequired` and replace it with errors like `ErrExecutionTargetRequired` or `ErrPermissionDenied`.

- [ ] **Step 4: Replace repo-specific tools with host-scoped equivalents**

Create `fs.go`, `exec.go`, and `git.go` that operate on resolved paths. These files own the new behavior. Do not leave wrappers around `repo_fs.go`, `repo_exec.go`, or `repo_git.go`.

- [ ] **Step 5: Replace path-based approval helpers**

`ApprovalTargetPath` should be removed. Approval helpers should serialize a binding summary from tool intent and execution context.

- [ ] **Step 6: Re-run the tool tests**

Run: `go test ./internal/tools -run 'Test.*'`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/tools/context.go internal/tools/approvals.go internal/tools/policy.go internal/tools/fs.go internal/tools/exec.go internal/tools/git.go internal/tools/tools_test.go internal/tools/repo_tools_test.go internal/tools/repo_exec_test.go internal/tools/coder_exec_test.go
git rm internal/tools/repo_fs.go internal/tools/repo_exec.go internal/tools/repo_git.go internal/tools/path_guard.go internal/tools/workspace.go
git commit -m "refactor: replace workspace tools with host-scoped tool contracts"
```

### Task 6: Rewrite approvals, onboarding, settings, runs, and schedules in the web and CLI surfaces

**Files:**
- Modify: `internal/web/routes_approvals.go`
- Modify: `internal/web/routes_runs.go`
- Modify: `internal/web/routes_projects.go`
- Modify: `internal/web/routes_onboarding.go`
- Modify: `internal/web/routes_settings.go`
- Modify: `internal/web/routes_run_submit.go`
- Modify: `internal/web/routes_sessions.go`
- Modify: `internal/web/server_test.go`
- Modify: `internal/web/approval_flow_test.go`
- Modify: `internal/web/routes_onboarding_test.go`
- Modify: `internal/web/routes_runs_test.go`
- Modify: `internal/web/run_graph_test.go`
- Modify: `internal/scheduler/types.go`
- Modify: `internal/scheduler/store.go`
- Modify: `internal/scheduler/service.go`
- Modify: `internal/scheduler/store_test.go`
- Modify: `internal/scheduler/service_test.go`
- Modify: `internal/scheduler/schedule_test.go`
- Modify: `cmd/gistclaw/schedule.go`
- Modify: `cmd/gistclaw/schedule_test.go`
- Modify: `cmd/gistclaw/export.go`

- [ ] **Step 1: Write failing web/scheduler tests for the new surfaces**

Change approval payloads to expose a binding summary instead of `target_path`. Change onboarding and settings to use `storage_root`, `primary_path`, and permission modes. Change schedules to persist `project_id`, `cwd`, and `authority_json`.

- [ ] **Step 2: Run the web and scheduler tests to verify they fail**

Run: `go test ./internal/web ./internal/scheduler ./cmd/gistclaw -run 'TestApproval|TestOnboarding|TestRuns|TestServer|TestRunGraph|TestService|TestSchedule|TestExport'`
Expected: FAIL because the UI and scheduler still read and write `workspace_root` and `target_path`.

- [ ] **Step 3: Rewrite onboarding and settings**

Onboarding should collect:
- `storage_root`
- default permission modes
- project name
- optional `primary_path`
- optional known roots

Settings should no longer expose a global workspace field.

- [ ] **Step 4: Rewrite approval and run presentation**

Approval pages and run pages should show:
- tool name
- summary of `cwd`
- read roots
- write roots
- mutating/network flags

Do not show `target_path`; it no longer exists.

- [ ] **Step 5: Rewrite scheduling**

Schedules should target `project_id` plus `cwd` and `authority_json`. CLI flags should match the same model.

- [ ] **Step 6: Re-run the web and scheduler tests**

Run: `go test ./internal/web ./internal/scheduler ./cmd/gistclaw -run 'TestApproval|TestOnboarding|TestRuns|TestServer|TestRunGraph|TestService|TestSchedule|TestExport'`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/web/routes_approvals.go internal/web/routes_runs.go internal/web/routes_projects.go internal/web/routes_onboarding.go internal/web/routes_settings.go internal/web/routes_run_submit.go internal/web/routes_sessions.go internal/web/server_test.go internal/web/approval_flow_test.go internal/web/routes_onboarding_test.go internal/web/routes_runs_test.go internal/web/run_graph_test.go internal/scheduler/types.go internal/scheduler/store.go internal/scheduler/service.go internal/scheduler/store_test.go internal/scheduler/service_test.go internal/scheduler/schedule_test.go cmd/gistclaw/schedule.go cmd/gistclaw/schedule_test.go cmd/gistclaw/export.go
git commit -m "refactor: rewrite web and scheduler surfaces for host execution"
```

### Task 7: Enforce security posture, connector restrictions, and storage-root-owned team state

**Files:**
- Modify: `internal/security/audit.go`
- Modify: `internal/security/audit_test.go`
- Modify: `internal/runtime/team.go`
- Modify: `internal/runtime/team_profiles.go`
- Modify: `internal/runtime/team_test.go`
- Modify: `internal/runtime/team_profiles_test.go`
- Modify: `internal/connectors/telegram/inbound.go`
- Modify: `internal/connectors/telegram/inbound_test.go`
- Modify: `internal/connectors/whatsapp/inbound.go`
- Modify: `internal/connectors/whatsapp/inbound_test.go`
- Modify: `internal/app/connector_supervisor_test.go`

- [ ] **Step 1: Write failing tests for dangerous-mode restrictions and team-state relocation**

Cover:
- security audit passes with `storage_root` and valid permission modes
- security audit warns on `auto_approve + elevated`
- remote connector requests cannot run in `auto_approve + elevated`
- team profiles are stored under `storage_root`, namespaced by project or team ID, not under a repo path

- [ ] **Step 2: Run the security/runtime/connector tests to verify they fail**

Run: `go test ./internal/security ./internal/runtime ./internal/connectors/telegram ./internal/connectors/whatsapp ./internal/app -run 'TestAudit|TestTeam|TestProfiles|TestInbound|TestSupervisor'`
Expected: FAIL because the current audit and connector code still assume workspace ownership.

- [ ] **Step 3: Rewrite the security audit**

Audit:
- `storage_root` exists and is writable
- permission modes are valid
- sensitive access warnings are accurate
- local-only restrictions are surfaced when dangerous combinations are enabled

- [ ] **Step 4: Move team state under storage-root namespaces**

Team and profile files should live under a storage namespace like:

```text
<storage_root>/projects/<project-id>/teams/
<storage_root>/teams/<team-id>/
```

Never under `primary_path`.

- [ ] **Step 5: Enforce connector restrictions**

Telegram and WhatsApp inbound paths must refuse or downgrade `auto_approve + elevated`. Do not allow a remote prompt channel to become a silent full-host execution path.

- [ ] **Step 6: Re-run the security/runtime/connector tests**

Run: `go test ./internal/security ./internal/runtime ./internal/connectors/telegram ./internal/connectors/whatsapp ./internal/app -run 'TestAudit|TestTeam|TestProfiles|TestInbound|TestSupervisor'`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/security/audit.go internal/security/audit_test.go internal/runtime/team.go internal/runtime/team_profiles.go internal/runtime/team_test.go internal/runtime/team_profiles_test.go internal/connectors/telegram/inbound.go internal/connectors/telegram/inbound_test.go internal/connectors/whatsapp/inbound.go internal/connectors/whatsapp/inbound_test.go internal/app/connector_supervisor_test.go
git commit -m "refactor: enforce host access policy and storage-owned team state"
```

### Task 8: Remove dead workspace code, update docs, and run full verification

**Files:**
- Modify: `README.md`
- Modify: `docs/system.md`
- Modify: `docs/kernel.md`
- Modify: `docs/extensions.md`
- Modify: `docs/roadmap.md`
- Modify: `internal/replay/replay_test.go`
- Modify: `internal/web/presentation_test.go`
- Modify: any remaining `*_test.go` that still reference `workspace_root` or `target_path`

- [ ] **Step 1: Search for any remaining legacy identifiers**

Run: `rg -n 'workspace_root|WorkspaceRoot|target_path|TargetPath|workspaceRootFromContext|ErrWorkspaceRequired' .`
Expected: results only in old git history or intentionally updated migration notes. The live tree should have zero runtime references.

- [ ] **Step 2: Update docs to match the shipped architecture**

Document:
- `storage_root` as the traceability layer
- project metadata vs execution target
- authority and approval binding model
- permission modes and connector restrictions

- [ ] **Step 3: Update any replay/export or presentation tests that still assume workspace semantics**

Run targeted packages after each edit instead of waiting for the full suite.

- [ ] **Step 4: Run full verification**

Run:
- `go test ./...`
- `go test -cover ./...`
- `go vet ./...`

Expected:
- all tests pass
- total coverage remains at or above 70%
- vet passes cleanly

- [ ] **Step 5: Commit**

```bash
git add README.md docs/system.md docs/kernel.md docs/extensions.md docs/roadmap.md internal/replay/replay_test.go internal/web/presentation_test.go
git commit -m "docs: remove workspace model from system documentation"
```

## Final Acceptance Checklist

- [ ] No `workspace_root` fields, settings, SQL columns, CLI flags, or web forms remain.
- [ ] No `target_path` approvals remain.
- [ ] Tools consume `ExecutionTarget` and `AuthorityEnvelope`.
- [ ] Approvals bind exact execution plans.
- [ ] `--dangerously-skip-permissions` only disables prompts.
- [ ] `--elevated-host-access` is a separate capability.
- [ ] Remote connectors cannot silently run with `auto_approve + elevated`.
- [ ] Team state and artifacts live under `storage_root`.
- [ ] `go test ./...`, `go test -cover ./...`, and `go vet ./...` all pass.
