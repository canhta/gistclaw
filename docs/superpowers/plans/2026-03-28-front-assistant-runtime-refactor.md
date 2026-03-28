# Front Assistant Runtime Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor GistClaw so the front assistant executes simple bounded work directly, uses runtime-provided execution recommendations, and delegates only when specialist work adds clear value.

**Architecture:** Replace the current coordinator-first front agent with an adaptive front assistant backed by three new runtime concepts: layered tool policy, effective capability inventory, and execution recommendation. Keep specialist runs and replay visibility, but move delegation behind governed orchestration instead of raw free-form front-run spawning.

**Tech Stack:** Go 1.25+, SQLite, Go `testing`, existing runtime/tool/team/web packages, server-rendered Go templates, YAML team definitions.

---

## File Structure

### New Runtime Units

- Create: `internal/runtime/recommendation/`
  - Own runtime execution recommendation logic: `direct`, `delegate`, `parallelize`
- Create: `internal/runtime/capabilities/`
  - Own direct capability adapters and dispatch
- Create: `internal/runtime/delegation/`
  - Own structured delegation + governed autonomous spawn orchestration

### New Tool Units

- Create: `internal/tools/capability.go`
  - Generic capability tool registration and invocation
- Create: `internal/tools/delegation.go`
  - Structured delegation tool
- Create: `internal/tools/spawn.go`
  - Governed autonomous helper spawn tool

### Team / Policy Refactor

- Modify: `internal/model/types.go`
- Modify: `internal/teams/config.go`
- Modify: `internal/teams/spec.go`
- Modify: `internal/teams/snapshot.go`
- Modify: `internal/tools/policy.go`
- Modify: `internal/runtime/context.go`
- Modify: `internal/runtime/runs.go`

### App / Bootstrap Wiring

- Modify: `internal/app/bootstrap.go`
- Modify: `internal/tools/register.go`

### Connector Capability Adapters

- Modify: `internal/app/zalo_personal_auth.go`
- Modify: `internal/connectors/zalopersonal/`
  - add capability adapter entry points without changing transport boundaries

### Default Team

- Modify: `teams/default/team.yaml`
- Delete: `teams/default/coordinator.soul.yaml`
- Create: `teams/default/assistant.soul.yaml`
- Modify: `teams/default/researcher.soul.yaml`
- Modify: `teams/default/patcher.soul.yaml`
- Modify: `teams/default/reviewer.soul.yaml`
- Modify: `teams/default/verifier.soul.yaml`

### Web / Operator Surface

- Modify: `internal/web/routes_team.go`
- Modify: `internal/web/templates/team.html`
- Modify: run/replay routes if they need new recommendation/delegation metadata

### Docs

- Modify: `README.md`
- Modify: `docs/system.md`
- Modify: `docs/kernel.md`
- Modify: `docs/vision.md`
- Modify: `docs/extensions.md`
- Modify: `docs/roadmap.md`

---

### Task 1: Lock The New Agent Policy Model

**Files:**
- Modify: `internal/model/types.go`
- Test: `internal/model/types_test.go`

- [ ] **Step 1: Write the failing tests for the new agent policy shape**

Add table-driven tests covering:
- agent base profile field
- tool family allow/deny overlays
- delegation kinds
- specialist roster visibility flags
- removal of `CanSpawn`-centric assumptions from new helpers

- [ ] **Step 2: Run the targeted tests to verify they fail**

Run: `go test ./internal/model -run 'TestAgentPolicy|TestToolSpecFamily'`
Expected: FAIL because the new fields and validation logic do not exist yet

- [ ] **Step 3: Implement the new model fields and helpers**

Add new model types and fields for:
- `BaseProfile`
- `ToolFamilies`
- `AllowTools`
- `DenyTools`
- `DelegationKinds`
- `SpecialistSummaryVisibility`
- tool family on `ToolSpec`

Do not add compatibility aliases for the old model.

- [ ] **Step 4: Run the targeted model tests**

Run: `go test ./internal/model -run 'TestAgentPolicy|TestToolSpecFamily'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/model/types.go internal/model/types_test.go
git commit -m "refactor: add adaptive agent policy model"
```

### Task 2: Replace Team Config Semantics

**Files:**
- Modify: `internal/teams/spec.go`
- Modify: `internal/teams/config.go`
- Modify: `internal/teams/snapshot.go`
- Test: `internal/teams/config_test.go`
- Test: `internal/teams/snapshot_test.go`

- [ ] **Step 1: Write failing tests for new team YAML fields**

Add tests for:
- parsing `base_profile`
- parsing `tool_families`
- parsing `allow_tools` and `deny_tools`
- parsing `delegation_kinds`
- snapshot generation from the new shape
- rejection of mixed old/new semantics where needed

- [ ] **Step 2: Run the targeted team tests to verify failure**

Run: `go test ./internal/teams -run 'TestLoadConfig|TestSnapshot'`
Expected: FAIL because the current parser expects `tool_posture` and `can_spawn`

- [ ] **Step 3: Refactor team config and snapshot building**

Implement the new YAML contract and snapshot projection.
Delete the old `tool_posture`-only and `can_spawn`-only runtime assumptions in the new path.

- [ ] **Step 4: Run the targeted team tests**

Run: `go test ./internal/teams -run 'TestLoadConfig|TestSnapshot'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/teams/spec.go internal/teams/config.go internal/teams/snapshot.go internal/teams/config_test.go internal/teams/snapshot_test.go
git commit -m "refactor: replace team posture config with adaptive policy"
```

### Task 3: Refactor Tool Policy To Layered Resolution

**Files:**
- Modify: `internal/tools/policy.go`
- Test: `internal/tools/tools_test.go`
- Test: `internal/tools/policy_test.go`

- [ ] **Step 1: Write failing tests for layered policy resolution**

Cover:
- base profile default behavior
- tool family gating
- explicit allow override
- explicit deny override
- repo write still denied to front assistant
- connector capability tools visible to front assistant

- [ ] **Step 2: Run the targeted policy tests to verify they fail**

Run: `go test ./internal/tools -run 'TestPolicy'`
Expected: FAIL because policy only understands `tool_posture`, `session_spawn`, and `scoped_apply`

- [ ] **Step 3: Implement layered policy**

Refactor the policy engine to resolve decisions from:
- base profile
- tool family
- allow/deny overlays
- approval behavior
- runtime availability hooks

Leave no dead branches that still drive the front assistant through old coordinator semantics.

- [ ] **Step 4: Run the targeted policy tests**

Run: `go test ./internal/tools -run 'TestPolicy'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tools/policy.go internal/tools/policy_test.go internal/tools/tools_test.go
git commit -m "refactor: implement layered tool policy"
```

### Task 4: Add Effective Capability Inventory

**Files:**
- Create: `internal/runtime/capabilities/inventory.go`
- Create: `internal/runtime/capabilities/inventory_test.go`
- Modify: `internal/runtime/runs.go`

- [ ] **Step 1: Write the failing inventory tests**

Cover:
- inventory groups by source
- inventory reflects current tool policy
- inventory can include connector capabilities
- front assistant sees only effective, not theoretical, capabilities

- [ ] **Step 2: Run the targeted inventory tests to verify failure**

Run: `go test ./internal/runtime/capabilities -run 'TestInventory'`
Expected: FAIL because the package and behavior do not exist

- [ ] **Step 3: Implement effective inventory resolution**

Build an inventory service that returns:
- built-in/runtime capabilities
- connector capabilities
- delegation options

This should be runtime-owned data, not prompt-only formatting logic.

- [ ] **Step 4: Run the targeted inventory tests**

Run: `go test ./internal/runtime/capabilities -run 'TestInventory'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/capabilities/inventory.go internal/runtime/capabilities/inventory_test.go internal/runtime/runs.go
git commit -m "feat: add effective capability inventory"
```

### Task 5: Add Execution Recommendation Engine

**Files:**
- Create: `internal/runtime/recommendation/engine.go`
- Create: `internal/runtime/recommendation/engine_test.go`
- Modify: `internal/runtime/context.go`
- Modify: `internal/runtime/runs.go`

- [ ] **Step 1: Write the failing recommendation tests**

Cover representative cases:
- bounded connector action -> `direct`
- research-heavy task -> `delegate`
- two independent specialist subtasks -> `parallelize`
- direct recommendation when local capability exists

- [ ] **Step 2: Run the targeted recommendation tests to verify failure**

Run: `go test ./internal/runtime/recommendation -run 'TestRecommend'`
Expected: FAIL because the engine does not exist

- [ ] **Step 3: Implement execution recommendation**

Return:
- mode
- rationale
- confidence
- suggested specialist kind(s)
- override allowance

Wire this into context assembly so the front assistant can see the recommendation.

- [ ] **Step 4: Run the targeted recommendation tests**

Run: `go test ./internal/runtime/recommendation -run 'TestRecommend'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/recommendation/engine.go internal/runtime/recommendation/engine_test.go internal/runtime/context.go internal/runtime/runs.go
git commit -m "feat: add runtime execution recommendation engine"
```

### Task 6: Replace Front `session_spawn` With Governed Delegation Tools

**Files:**
- Create: `internal/tools/delegation.go`
- Create: `internal/tools/spawn.go`
- Create: `internal/runtime/delegation/orchestrator.go`
- Create: `internal/runtime/delegation/orchestrator_test.go`
- Modify: `internal/tools/collaboration.go`
- Modify: `internal/app/bootstrap.go`

- [ ] **Step 1: Write failing tests for governed delegation**

Cover:
- structured delegation tool routes to the correct specialist kind
- autonomous helper spawn respects depth and active-child limits
- denied delegation when no value is added
- front assistant no longer relies on raw `session_spawn` as the default path

- [ ] **Step 2: Run targeted delegation tests to verify failure**

Run: `go test ./internal/runtime/delegation ./internal/tools -run 'TestDelegate|TestSpawn'`
Expected: FAIL because the new tools and orchestrator do not exist

- [ ] **Step 3: Implement governed delegation and helper spawn**

Keep:
- structured specialist delegation
- governed autonomous helper spawn with runtime limits

Remove:
- old free-form front-agent delegation assumption as the primary interface

- [ ] **Step 4: Run the targeted delegation tests**

Run: `go test ./internal/runtime/delegation ./internal/tools -run 'TestDelegate|TestSpawn'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tools/delegation.go internal/tools/spawn.go internal/runtime/delegation/orchestrator.go internal/runtime/delegation/orchestrator_test.go internal/tools/collaboration.go internal/app/bootstrap.go
git commit -m "refactor: govern assistant delegation and helper spawn"
```

### Task 7: Add Generic Capability Tools

**Files:**
- Create: `internal/tools/capability.go`
- Create: `internal/tools/capability_test.go`
- Create: `internal/runtime/capabilities/registry.go`
- Create: `internal/runtime/capabilities/registry_test.go`
- Modify: `internal/tools/register.go`
- Modify: `internal/tools/registry.go`

- [ ] **Step 1: Write failing tests for generic capability tools**

Cover:
- capability registry lookup
- capability tool invocation
- direct execution path for bounded actions
- capability result normalization

- [ ] **Step 2: Run the targeted capability tests to verify failure**

Run: `go test ./internal/tools ./internal/runtime/capabilities -run 'TestCapability'`
Expected: FAIL because the generic capability path does not exist

- [ ] **Step 3: Implement generic capability tool plumbing**

Add generic capability surfaces:
- `connector_directory_list`
- `connector_target_resolve`
- `connector_send`
- `connector_status`
- `app_action`

- [ ] **Step 4: Run the targeted capability tests**

Run: `go test ./internal/tools ./internal/runtime/capabilities -run 'TestCapability'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tools/capability.go internal/tools/capability_test.go internal/runtime/capabilities/registry.go internal/runtime/capabilities/registry_test.go internal/tools/register.go internal/tools/registry.go
git commit -m "feat: add generic runtime capability tools"
```

### Task 8: Add Zalo Personal Capability Adapter As First Concrete Adapter

**Files:**
- Modify: `internal/app/zalo_personal_auth.go`
- Create: `internal/connectors/zalopersonal/capabilities.go`
- Create: `internal/connectors/zalopersonal/capabilities_test.go`

- [ ] **Step 1: Write failing tests for the Zalo capability adapter**

Cover:
- contact listing through generic capability path
- target resolution by contact name
- direct send through generic capability path
- unauthenticated error normalization

- [ ] **Step 2: Run the targeted Zalo capability tests to verify failure**

Run: `go test ./internal/connectors/zalopersonal -run 'TestCapabilities'`
Expected: FAIL because the adapter does not exist

- [ ] **Step 3: Implement the Zalo capability adapter**

Use existing app/auth and protocol surfaces. Do not add a Zalo-specific shortcut in the run engine.

- [ ] **Step 4: Run the targeted Zalo capability tests**

Run: `go test ./internal/connectors/zalopersonal -run 'TestCapabilities'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/zalo_personal_auth.go internal/connectors/zalopersonal/capabilities.go internal/connectors/zalopersonal/capabilities_test.go
git commit -m "feat: expose zalo personal through generic capability adapters"
```

### Task 9: Replace The Default Team Contract

**Files:**
- Modify: `teams/default/team.yaml`
- Delete: `teams/default/coordinator.soul.yaml`
- Create: `teams/default/assistant.soul.yaml`
- Modify: `teams/default/researcher.soul.yaml`
- Modify: `teams/default/patcher.soul.yaml`
- Modify: `teams/default/reviewer.soul.yaml`
- Modify: `teams/default/verifier.soul.yaml`
- Test: `teams/default_embed_test.go`

- [ ] **Step 1: Write failing tests for the new default team contract**

Cover:
- front assistant is not a coordinator
- new team file shape embeds correctly
- assistant instructions prefer direct execution first
- worker instructions remain narrow and specialist-owned

- [ ] **Step 2: Run the targeted embedded team tests to verify failure**

Run: `go test ./teams -run 'TestDefault'`
Expected: FAIL because the old coordinator-first team is still embedded

- [ ] **Step 3: Replace the default team files**

Delete `coordinator.soul.yaml`.
Create `assistant.soul.yaml`.
Update `team.yaml` and worker souls to the new policy language.

- [ ] **Step 4: Run the targeted team embed tests**

Run: `go test ./teams -run 'TestDefault'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add teams/default/team.yaml teams/default/assistant.soul.yaml teams/default/researcher.soul.yaml teams/default/patcher.soul.yaml teams/default/reviewer.soul.yaml teams/default/verifier.soul.yaml teams/default_embed_test.go
git rm teams/default/coordinator.soul.yaml
git commit -m "refactor: replace coordinator-first default team"
```

### Task 10: Rewire Runtime Context And Run Loop Behavior

**Files:**
- Modify: `internal/runtime/context.go`
- Modify: `internal/runtime/runs.go`
- Modify: `internal/runtime/collaboration.go`
- Test: `internal/runtime/runs_test.go`
- Test: `internal/runtime/collaboration_test.go`
- Test: `internal/runtime/acceptance_test.go`

- [ ] **Step 1: Write failing runtime integration tests**

Cover:
- simple bounded task stays in one front run
- research task delegates
- code-change task delegates to patcher
- direct capability is preferred over delegation when both are possible
- recommendation metadata appears in replay context

- [ ] **Step 2: Run the targeted runtime tests to verify failure**

Run: `go test ./internal/runtime -run 'TestFront|TestAcceptance|TestCollaboration'`
Expected: FAIL because the run loop still reflects coordinator-first behavior

- [ ] **Step 3: Refactor the run loop and context assembly**

Make the front assistant adaptive:
- use recommendation
- prefer direct capability path
- use structured delegation or governed spawn only when appropriate

- [ ] **Step 4: Run the targeted runtime tests**

Run: `go test ./internal/runtime -run 'TestFront|TestAcceptance|TestCollaboration'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/context.go internal/runtime/runs.go internal/runtime/collaboration.go internal/runtime/runs_test.go internal/runtime/collaboration_test.go internal/runtime/acceptance_test.go
git commit -m "refactor: make front runs adaptive and direct-first"
```

### Task 11: Update Web Team Editing And Operator Visibility

**Files:**
- Modify: `internal/web/routes_team.go`
- Modify: `internal/web/templates/team.html`
- Modify: `internal/web/routes_runs.go`
- Test: `internal/web/server_test.go`

- [ ] **Step 1: Write failing web tests for the new team model**

Cover:
- team editor accepts the new fields
- old `tool_posture`/`can_spawn` inputs are removed from the shipped editor path
- run detail can display recommendation/delegation metadata

- [ ] **Step 2: Run the targeted web tests to verify failure**

Run: `go test ./internal/web -run 'TestTeam|TestRunDetail'`
Expected: FAIL because the web layer still renders the old team semantics

- [ ] **Step 3: Refactor the web surfaces**

Update team editing to the new policy model and surface execution recommendations where useful.

- [ ] **Step 4: Run the targeted web tests**

Run: `go test ./internal/web -run 'TestTeam|TestRunDetail'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/routes_team.go internal/web/templates/team.html internal/web/routes_runs.go internal/web/server_test.go
git commit -m "refactor: update operator surfaces for adaptive assistant model"
```

### Task 12: Rewrite Documentation To Match The New Runtime

**Files:**
- Modify: `README.md`
- Modify: `docs/system.md`
- Modify: `docs/kernel.md`
- Modify: `docs/vision.md`
- Modify: `docs/extensions.md`
- Modify: `docs/roadmap.md`

- [ ] **Step 1: Write the documentation changes**

Update all product/runtime descriptions so they consistently say:
- front assistant executes directly first
- runtime recommends `direct`, `delegate`, `parallelize`
- specialists remain visible but are not the surface product
- capabilities are explicit and adapter-driven

- [ ] **Step 2: Run doc sanity checks**

Run: `rg -n "coordinator-first|tool_posture|can_spawn|session_spawn" README.md docs internal teams`
Expected: only intentional historical references remain in tests or migration notes

- [ ] **Step 3: Commit**

```bash
git add README.md docs/system.md docs/kernel.md docs/vision.md docs/extensions.md docs/roadmap.md
git commit -m "docs: rewrite runtime architecture for adaptive front assistant"
```

### Task 13: Full Verification And Coverage Gate

**Files:**
- Modify: any tests or docs required by final green state

- [ ] **Step 1: Run focused package tests in dependency order**

Run:
- `go test ./internal/model`
- `go test ./internal/teams`
- `go test ./internal/tools`
- `go test ./internal/runtime/...`
- `go test ./internal/connectors/zalopersonal`
- `go test ./internal/web`

Expected: PASS

- [ ] **Step 2: Run the full repository test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 3: Run lint and vet**

Run:
- `make lint`
- `go vet ./...`

Expected: PASS

- [ ] **Step 4: Run the coverage gate**

Run: `go test -coverprofile=/tmp/gistclaw.cover ./... && go tool cover -func=/tmp/gistclaw.cover | tail -n 1`
Expected: total coverage at or above `70%`

- [ ] **Step 5: Commit final fixes**

```bash
git add -A
git commit -m "test: finalize adaptive front assistant refactor"
```

---

## Notes For Execution

- Do not keep compatibility wrappers for `tool_posture`, `can_spawn`, or the old coordinator-first contract.
- Delete dead tests and helpers that only exist for the old semantics once the new path is in place.
- Keep the runtime journal-first. New recommendation and delegation metadata must still flow through journal-backed events.
- Favor package extraction over growing `internal/runtime/runs.go` further.
- The first behavioral milestone is not Zalo-specific. It is this: a simple bounded task should stay in one front run.

## Recommended Execution Order

1. Task 1
2. Task 2
3. Task 3
4. Task 4
5. Task 5
6. Task 6
7. Task 7
8. Task 9
9. Task 10
10. Task 8
11. Task 11
12. Task 12
13. Task 13

Reason:
- the policy/recommendation contract must exist before capability adapters and UI refactors
- default team replacement should happen before the final runtime behavior cut-over
- Zalo capability adaptation should happen after the generic capability path is proven
