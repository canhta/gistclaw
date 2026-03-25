# Operator-Job Web Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the legacy web information architecture with the approved operator-job structure, without backward-compatible page aliases.

**Architecture:** Introduce one shared page-path/navigation model for the web UI, move page routes under `Operate`, `Configure`, and `Recover`, and update handlers/templates/tests together so the graph, queue, configuration, and recovery surfaces all reflect the new hierarchy. Keep runtime and API behavior intact while replacing page URLs, redirects, labels, and shared layout plumbing.

**Tech Stack:** Go 1.25+, stdlib `net/http`, Go `html/template`, Go `testing`

---

### Task 1: Lock the new page map in tests

**Files:**
- Modify: `internal/web/server_test.go`
- Modify: `internal/web/routes_onboarding_test.go`
- Modify: `internal/web/design_system_test.go`

- [ ] **Step 1: Write failing tests for the new route map**
- [ ] **Step 2: Run the focused web tests and confirm they fail for old paths/nav**
- [ ] **Step 3: Cover root redirect, top-level group nav, sub-navigation, and no-legacy path expectations**

### Task 2: Replace shared page paths and navigation plumbing

**Files:**
- Create: `internal/web/paths.go`
- Modify: `internal/web/server.go`
- Modify: `internal/web/templates/layout.html`

- [ ] **Step 1: Add shared page-path helpers for group routes, detail routes, and form redirects**
- [ ] **Step 2: Refactor route registration to use the new URLs only**
- [ ] **Step 3: Update shared layout data so top navigation, sub-navigation, and the Start Task CTA render from one source of truth**
- [ ] **Step 4: Run focused tests and keep them green before moving on**

### Task 3: Move handlers and templates to the new hierarchy

**Files:**
- Modify: `internal/web/routes_runs.go`
- Modify: `internal/web/routes_session_pages.go`
- Modify: `internal/web/routes_run_submit.go`
- Modify: `internal/web/routes_team.go`
- Modify: `internal/web/routes_memory.go`
- Modify: `internal/web/routes_settings.go`
- Modify: `internal/web/routes_routes_deliveries.go`
- Modify: `internal/web/routes_approvals.go`
- Modify: `internal/web/routes_onboarding.go`
- Modify: `internal/web/templates/runs.html`
- Modify: `internal/web/templates/run_detail.html`
- Modify: `internal/web/templates/run_submit.html`
- Modify: `internal/web/templates/sessions.html`
- Modify: `internal/web/templates/session_detail.html`
- Modify: `internal/web/templates/team.html`
- Modify: `internal/web/templates/memory.html`
- Modify: `internal/web/templates/settings.html`
- Modify: `internal/web/templates/routes_deliveries.html`
- Modify: `internal/web/templates/approvals.html`
- Modify: `internal/web/templates/onboarding.html`

- [ ] **Step 1: Update render paths, redirects, and pagination bases to the new URL structure**
- [ ] **Step 2: Update page copy and headings to match Operate / Configure / Recover roles**
- [ ] **Step 3: Keep the compact graph on Runs and the full graph first on Run Detail**
- [ ] **Step 4: Rename Control to Routes & Deliveries across handlers and templates**
- [ ] **Step 5: Run targeted tests after each route group change**

### Task 4: Sync docs and final verification

**Files:**
- Modify: `docs/system.md`
- Modify: `README.md`
- Modify: `AGENTS.md`
- Modify: `DESIGN.md`

- [ ] **Step 1: Update shipped web surface docs to the new page map**
- [ ] **Step 2: Run `go test ./internal/web/...`**
- [ ] **Step 3: Run `go test ./...`**
- [ ] **Step 4: Run `go test -cover ./...` and confirm coverage stays at or above 70%**
