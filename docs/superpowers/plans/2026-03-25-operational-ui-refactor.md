# Operational UI Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the web UI into the production-ready orchestration system defined in `DESIGN.md`, including a first-class `Team` page, read-only execution snapshots on run detail, shared filter/pagination controls, compact graph cards, and responsive behavior.

**Architecture:** Extend the runtime/web seam so the UI can read and update default team configuration and run-bound execution snapshots without inventing parallel UI state. Then standardize long-list pages and shared layout primitives before refactoring individual pages onto them.

**Tech Stack:** Go `net/http`, Go templates, SQLite, existing `internal/teams` package, local static assets, Go `testing`

---

### Task 1: Team data seam

**Files:**
- Modify: `internal/model/types.go`
- Modify: `internal/runtime/runs.go`
- Modify: `internal/app/bootstrap.go`
- Modify: `internal/web/server.go`
- Create: `internal/web/routes_team.go`
- Test: `internal/web/server_test.go`

- [ ] **Step 1: Write failing tests for the new Team page and snapshot data**
- [ ] **Step 2: Run `go test ./internal/web/...` to verify the new tests fail for missing route/data**
- [ ] **Step 3: Add runtime/team view types and accessors for default team config plus run snapshot decoding**
- [ ] **Step 4: Pass team-dir context from app bootstrap into the web/runtime seam**
- [ ] **Step 5: Implement `GET /configure/team` and any required `POST /configure/team` update path with runtime-owned writes**
- [ ] **Step 6: Re-run focused web tests and make them pass**

### Task 2: Shared list controls

**Files:**
- Modify: `internal/web/pagination.go`
- Modify: `internal/web/routes_runs.go`
- Modify: `internal/web/routes_session_pages.go`
- Modify: `internal/web/routes_approvals.go`
- Modify: `internal/web/templates/runs.html`
- Modify: `internal/web/templates/sessions.html`
- Modify: `internal/web/templates/approvals.html`
- Modify: `internal/web/templates/layout.html`
- Test: `internal/web/server_test.go`
- Test: `internal/web/design_system_test.go`

- [ ] **Step 1: Write failing tests for filter rails, page-size handling, and pagination rendering on long-list pages**
- [ ] **Step 2: Run the focused tests to verify they fail for the intended reasons**
- [ ] **Step 3: Extend shared pagination helpers to support page size and normalized query preservation**
- [ ] **Step 4: Refactor runs/sessions/approvals handlers to expose count/filter/paging data cleanly**
- [ ] **Step 5: Refactor list templates onto the shared filter rail and pager primitives**
- [ ] **Step 6: Re-run focused tests and make them pass**

### Task 3: Run detail production refactor

**Files:**
- Modify: `internal/web/routes_runs.go`
- Modify: `internal/web/templates/run_detail.html`
- Modify: `internal/web/templates/layout.html`
- Test: `internal/web/server_test.go`
- Test: `internal/web/design_system_test.go`

- [ ] **Step 1: Write failing tests for execution snapshot rendering and compact graph-card structure**
- [ ] **Step 2: Run focused tests to confirm red state**
- [ ] **Step 3: Add run snapshot view models to the run detail data path**
- [ ] **Step 4: Refactor the run detail template to show snapshot data and compact node content**
- [ ] **Step 5: Verify Cytoscape rendering still works with the new node structure**
- [ ] **Step 6: Re-run focused tests and make them pass**

### Task 4: Responsive shell and page reflow

**Files:**
- Modify: `internal/web/templates/layout.html`
- Modify: `internal/web/templates/run_detail.html`
- Modify: `internal/web/templates/runs.html`
- Modify: `internal/web/templates/sessions.html`
- Modify: `internal/web/templates/approvals.html`
- Modify: `internal/web/templates/settings.html`
- Create: `internal/web/templates/team.html`
- Test: `internal/web/design_system_test.go`

- [ ] **Step 1: Write failing tests for responsive primitives that must exist in the shared stylesheet**
- [ ] **Step 2: Run the design-system tests to confirm they fail**
- [ ] **Step 3: Add responsive layout primitives and breakpoint rules to `layout.html`**
- [ ] **Step 4: Rework page templates to use stack/reflow-friendly structure rather than fixed desktop assumptions**
- [ ] **Step 5: Re-run `go test ./internal/web/...` and make them pass**

### Task 5: Verification

**Files:**
- Modify: `internal/web/server_test.go`
- Modify: `internal/web/design_system_test.go`

- [ ] **Step 1: Run `go test ./internal/web/...`**
- [ ] **Step 2: Run `go test ./...`**
- [ ] **Step 3: Restart the local daemon and browser-QA runs, team, approvals, sessions, and a responsive viewport**
- [ ] **Step 4: Clean up any temporary QA data**
