# GistClaw Control UI — Full Redesign Spec

**Date:** 2026-03-29
**Status:** Approved
**Scope:** Full frontend rewrite — new shell, 12 sections, design system implementation

---

## Problem

The current frontend is a KPI dashboard with stamped panels. It exposes internal system nouns (Work, Knowledge, Recover) rather than operator jobs. It lacks real-time log tailing, skills management, node inspection, exec approval controls, and a config editor. The layout is a 3-column fixed shell that wastes space on a static inspector.

---

## Goal

A single-page control UI that lets an operator supervise live runs, manage channels and sessions, configure cron jobs, edit config, inspect nodes, handle exec approvals, debug runtime state, read logs, and run updates — without leaving the shell.

**Design principle:** Dense workflows, fast switching, persistent inspectors. Not a KPI dashboard.

---

## Users

| Role | Capabilities |
|---|---|
| **Operator** | Chat, Sessions, Logs, limited Cron Jobs |
| **Builder** | Operator + Channels, Skills, Config |
| **Admin** | Builder + Exec Approvals, Update, full Debug |

**Role implementation:** Deferred to Phase N (multi-user auth). Phase 1–3 implementation treats all authenticated users as Admin (all 12 sections unlocked). The lock UI in the nav is built as a component but not activated until the backend serves a `role` field on `/api/auth/session`. No backend changes required for Phase 1–3.

---

## Out of Scope

- Public landing page
- Billing portal
- KPI dashboards / summary cards as primary UI
- End-user mobile-first UI
- Separate "knowledge product" or "analytics workspace"

---

## Shell

```
┌─────────────┬──────────────────────────────┬──────────────────┐
│  Left Rail  │         Workspace            │  Right Inspector │
│  (fixed)    │     (section content)        │  (collapsible)   │
│  ~56px icon │                              │  ~280px          │
│  or ~200px  │                              │                  │
└─────────────┴──────────────────────────────┴──────────────────┘
```

- **Left rail:** icon-only default (56px), expandable to labeled (200px). Fixed. Sticky. Expand/collapse via a toggle button at the bottom of the rail. State persists in `localStorage` (`gc-rail-expanded`). Keyboard: `[` to toggle.
- **Workspace:** full remaining width. Each section owns its tabs and layout.
- **Right inspector:** collapsible (280px). **When nothing is selected:** shows section-level summary — section title, key stats (e.g. active run count, session count), and a primary quick action for the section (e.g. "New Job" in Cron, "Send Message" in Chat). **When an item is selected:** shows object detail — metadata, actions, related state. Inspector collapse state persists per-section.
- **Top bar:** workspace selector, connection status, active session/job indicator, search/command palette (⌘K), notifications, user/env switch.

---

## Navigation (Left Rail Order)

1. Chat
2. Channels
3. Instances
4. Sessions
5. Cron Jobs
6. Skills
7. Nodes
8. Exec Approvals
9. Config
10. Debug
11. Logs
12. Update

### Role-Gated Sections

All 12 nav items are always visible regardless of role. Sections outside a user's role show a lock badge in the rail and a "Permission required" state in the workspace.

| Role | Accessible sections |
|---|---|
| **Operator** | Chat, Sessions, Logs, Cron Jobs (view only) |
| **Builder** | Operator + Channels, Skills, Config |
| **Admin** | All sections |

**Lock treatment:**
- Rail: small lock icon overlaid on section icon (bottom-right corner), `--ink-3` color.
- Workspace: centered panel with `PERMISSION REQUIRED` stamp, section name, and one-line description of what role unlocks it.
- Lock badge does NOT appear on placeholder sections (Instances, Skills, Nodes, Debug, Update) — those show "COMING SOON" regardless of role.

### CommandPalette (⌘K)

**Scope:** Navigation + recent items only (no full-text search).

**Contents:**
- `SECTIONS` group: all 12 sections by name. Locked sections show lock icon but are still listed (selecting them navigates and shows the permission state).
- `RECENT` group: last 10 visited items across sessions, cron jobs, and channels. Format: `type · name · time-ago`.
- No search across message content, config values, or logs in Phase 1.
- Keyboard: ↑↓ to navigate, Enter to jump, Escape to close. `[` still toggles the rail while palette is open.

---

## Route Map

### Current → New

| Current route | New route | Status |
|---|---|---|
| `/work` | `/chat` | Rewritten — Chat is the primary runtime surface |
| `/work/[runId]` | `/chat` (inspector) | Run detail moves to right inspector |
| `/conversations` | `/channels` + `/sessions` | Split into two sections |
| `/conversations/[sessionId]` | `/sessions` (inspector) | Moves to inspector |
| `/automate` | `/cron` | Renamed + restructured |
| `/recover` | `/approvals` | Renamed + restructured |
| `/settings` | `/config` | Merged with team/knowledge into Config |
| `/team` | `/config` (Agents & Routing tab) | Folded into Config |
| `/knowledge` | `/config` (General tab or removed) | Out of scope per spec |
| `/history` | `/chat` (history tab) + `/sessions` | Distributed |
| `/login` | `/login` | Keep as-is |
| `/onboarding` | `/onboarding` | Keep as-is |

### New routes (no backend yet)

| Route | Status |
|---|---|
| `/instances` | Placeholder |
| `/skills` | Placeholder |
| `/nodes` | Placeholder |
| `/debug` | Placeholder |
| `/logs` | Placeholder |
| `/update` | Placeholder |

---

## Section Specs

### Chat `/chat`

**Tabs:** Transcript · Run Events · Usage

**Workspace layout:**
```
┌──────────────────────────────────────────────────────┐
│  Session header (session name, status badge, actions) │
├──────────────────────────────────────────────────────┤
│                                                      │
│  Transcript timeline                                 │
│  (scrollable, fills available height)                │
│  Auto-scrolls to latest during active run            │
│                                                      │
├──────────────────────────────────────────────────────┤
│  Composer (fixed bottom, expands up to 5 lines)      │
│  [Stop] [Inject]  [text input ············] [Send]   │
└──────────────────────────────────────────────────────┘
```
Stop and Inject controls live in the composer bar — not floating. Stop is visible but dimmed when no run is active. Send becomes Stop when a run is in flight.

**Transcript visual treatment:** Timeline events — NOT chat bubbles. Each event is a hard-edged panel row:
- `USER` events: `--surface-raised` background, `USER` stamp (JetBrains Mono 700, `--ink-3`), timestamp right-aligned, message in DM Sans.
- `AGENT` events: `--surface` background, `AGENT` stamp in `--ink-2`. Tool call cards render inline as nested `--surface-elevated` panels with `TOOL` stamp + tool name + duration + result state (`✓` or `✗`).
- `SYSTEM` events (operator notes, injections): `--signal-dim` background, `NOTE` stamp in `--signal`.
- Tool cards collapse after completion. Expand on click to show full input/output.
- No avatars. No rounded corners on transcript elements. Borders: 1px dividers between events, 1.5px on tool cards.

**Components:** Session header, transcript timeline, composer, inline tool call cards, inline tool output cards, stop/inject controls, right inspector (run metadata + actions)

**Live actions:** Send (chat.send), Stop (chat.abort), Inject note (chat.inject), Load history (chat.history)

**Real-time:** SSE event stream (`GET /api/work/{id}/events`) — same pattern as current `/work/[runId]`. No WebSocket. Reuses existing `connectEventStream()` client utility.

**Backend:** Exists (current Work + stream_url)

---

### Channels `/channels`

**Tabs:** Status · Login · Settings

**Components:** Channel list, connection state badge, QR/login panel, error drawer, settings drawer

**Live actions:** Connect/reconnect, start QR login, edit channel settings

**Backend:** Exists (current Conversations connector endpoints)

---

### Instances `/instances`

**Tabs:** Presence · Details

**Placeholder:** "Instances are not connected to a backend yet."

---

### Sessions `/sessions`

**Tabs:** List · Overrides · History

**Components:** Session table, search/filters, override form, jump-to-chat action

**Live actions:** sessions.list, sessions.patch, open in Chat

**Backend:** Exists (current conversations/[sessionId])

---

### Cron Jobs `/cron`

**Tabs:** Jobs · Runs · Editor

**Editor groups:** Identity, Schedule, Execution, Payload, Delivery, Advanced

**Components:** Job list, run history table, editor form, right inspector (next run, recent run status)

**Live actions:** Create/edit/enable-disable/run-now job, view run history

**Backend:** Exists (current Automate)

---

### Skills `/skills`

**Tabs:** Installed · Available · Credentials

**Placeholder:** "Skill management is not connected to a backend yet."

---

### Nodes `/nodes`

**Tabs:** List · Capabilities

**Placeholder:** "Node inventory is not connected to a backend yet."

---

### Exec Approvals `/approvals`

**Tabs:** Gateway · Nodes · Allowlists

**Components:** Policy editor, allowlist editor, warning state, confirmation modal

**Live actions:** Edit gateway/node exec policy, update allowlists (require confirmation)

**Backend:** Exists (current Recover approval endpoints)

---

### Config `/config`

**Tabs:** General · Agents & Routing · Models · Channels · Raw JSON5 · Apply

**Components:** Schema-driven form renderer, raw editor, validation panel, apply/restart action bar

**Live actions:** Edit config, validate, apply, apply with restart

**Placement rules:** Agents → Agents & Routing tab. Models → Models tab. Deep channel config → Channels tab (live connectivity stays in Channels section).

**Backend:** Exists (current Settings + Team endpoints)

---

### Debug `/debug`

**Tabs:** Status · Health · Models · Events · RPC

**Placeholder (RPC tab):** Requires confirmation + clear warning. Placeholder for other tabs if endpoints missing.

**Backend:** Status/health/models exist partially. RPC console is new.

---

### Logs `/logs`

**Tabs:** Live Tail · Filters · Export

**Components:** Log stream, filter toolbar, expandable log row, export action

**Placeholder:** "Log streaming is not connected to a backend yet." until stream endpoint ships.

---

### Update `/update`

**Tabs:** Run Update · Restart Report

**Placeholder:** "Update workflow is not connected to a backend yet."

---

## User Journey

### First-Use Flow (new operator, no prior sessions)

| Step | User does | User feels | UI response |
|---|---|---|---|
| 1 | Opens GistClaw | Orientation: "where am I?" | Chat section active by default. Empty state: "No active session. Start a conversation to begin." Composer is present and focused. |
| 2 | Types first message, hits Send | Action, small anticipation | Composer clears. User message appears immediately. Run indicator in inspector spins up. Session header shows new session ID. |
| 3 | Watches first response arrive | Curiosity, novelty | Tokens stream in. Tool call cards appear inline as agent executes. Stop button becomes orange + active. |
| 4 | Sees tool calls completing | Understanding: "it's doing things" | Tool call cards collapse to compact stamp ("✓ read_file 0.3s"). Run Events tab gets a badge count. |
| 5 | Run completes | Satisfaction + "now what?" | Final message rendered. Stop → Send. Inspector shows token count, model, duration. |
| 6 | Explores other sections via left rail | Discovery | Each section shows its empty state with a clear description and primary action. |

### Primary Operator Flow (experienced user, active run)

| Step | User does | User feels | UI response |
|---|---|---|---|
| 1 | Navigates to Chat | Focus | Last session auto-loaded. Transcript shows history. |
| 2 | Sends a complex task | Trust + wait | Run starts. Transcript shows message sent. Inspector shows model + status ACTIVE. |
| 3 | Watches run in progress | Monitoring | Tokens stream. Tool cards appear/collapse. Run Events tab updates live. |
| 4 | Decides to stop the run | Control | Hits Stop (orange). Confirmation not required for Stop. Run terminates. Final state in inspector. |
| 5 | Injects a note | Intervention | Inject drawer opens. Types note. Note appears in transcript as a distinct "operator note" event. |

### Critical Moment: Exec Approval Required

| Step | User does | User feels | UI response |
|---|---|---|---|
| 1 | Run is in progress | Normal monitoring | — |
| 2 | Agent hits exec gate | Surprise / urgency | Exec Approvals nav item gets an orange badge. Inspector shows "1 approval pending" if on Chat. |
| 3 | User navigates to Exec Approvals | Decision mode | Pending approval is the first and most prominent item. Clear command detail, risk level, allow/deny. |
| 4 | User approves or denies | Resolution | ConfirmModal for Allow (dangerous action). Deny has no confirmation. Run resumes or terminates. |

### Design Notes
- The emotional arc must go: **orientation → action → feedback → understanding → trust**. Never skip feedback (step 3).
- The Chat section is the cockpit. Every other section is a control panel. Navigation should feel like leaving the cockpit to adjust a control, then returning.
- Decisions (approvals, config changes) must feel weighted — they should require one deliberate action, never happen by accident.

---

## Interaction States

Each live section must implement all four states. Placeholder sections show only the placeholder state.

### Global Patterns

| State | Treatment |
|---|---|
| **Loading** | Skeleton panels — surface-colored rectangles at content dimensions, no spinners. Skeleton matches the shape of the actual content (table rows, list items, cards). |
| **Empty** | Section-specific message (see below) + primary action CTA in `--primary`. Never "No items found." |
| **Error** | Error banner (`ValidationBanner` variant) pinned below `WorkspaceHeader`. Message + Retry button. Preserve stale data if available. |
| **Success** | `Toast` (non-blocking, 3s, bottom-right). Never a full-page confirmation. |
| **Partial** | Content visible, stream still active. Chat: transcript renders tokens as they arrive, stop button active. Channels: list shown, one row with "Connecting…" spinner badge. Cron: jobs loaded, run history row shows "Running" badge with elapsed time counter. |
| **Placeholder** | Centered panel: section title, one-line description of what it will do, `gc-stamp` label "COMING SOON". No fake data. |

### Section Empty States

| Section | Empty message | Primary CTA |
|---|---|---|
| Chat | "No active session. Start a conversation to begin." | Send a message |
| Channels | "No channels connected. Add a channel to receive messages." | Connect channel |
| Sessions | "No sessions yet. Sessions appear when conversations arrive." | — (no manual create) |
| Cron Jobs | "No jobs scheduled. Create a job to run tasks automatically." | New job |
| Exec Approvals | "No pending approvals. The gateway is clear." | — |
| Config | Never empty — always shows current config, even if minimal. | — |

### Loading Skeletons

| Section | Skeleton shape |
|---|---|
| Chat | 3-4 message rows (alternating left/right alignment), composer bar |
| Channels | 3 channel rows (icon + name + status badge) |
| Sessions | 5-row table skeleton |
| Cron Jobs | 4-row table skeleton + next-run column |
| Exec Approvals | 2-row approval queue skeleton |
| Config | Form field skeletons (label + input pairs) |

---

## Responsive Behavior

### Breakpoints

| Breakpoint | Shell behavior |
|---|---|
| `< 768px` (mobile) | Hamburger menu drawer. Left rail hidden. Inspector hidden. Single column workspace. |
| `768px–1279px` (tablet) | Left rail icon-only (56px, non-collapsible). Right inspector hidden. Workspace full width. |
| `≥ 1280px` (desktop) | Full 3-column shell: left rail (56px or 200px) + workspace + right inspector (280px). |

### Mobile Navigation

- Top bar shows: logo, section title, hamburger icon (right).
- Hamburger opens a full-height drawer from the left with the full labeled nav (same 12 items, same order).
- Active section highlighted in drawer. Tap item → close drawer + navigate.
- Bottom of drawer: theme toggle.
- No bottom tab bar — 12 items do not fit a tab paradigm.

### Tablet

- Left rail visible, icon-only, non-interactive expansion (no toggle at this breakpoint).
- Inspector content moves into a `Drawer` triggered by a "Details" button in the workspace header.
- All tab bars remain visible (horizontal scroll if needed at narrow tablet).

### Mobile Inspector Access

Inspector content is accessible on mobile via a slide-up panel triggered by tapping a selected item in a list. The panel covers the bottom 60% of the screen and is swipe-dismissible.

---

## Accessibility

- **Keyboard navigation:** Full keyboard operability. Tab order: TopBar → LeftNav → SectionTabs → Workspace content → RightInspector.
- **Left rail toggle:** `[` key shortcut announced via `aria-keyshortcuts`. Toggle button has `aria-label="Collapse navigation"` / `"Expand navigation"` and `aria-expanded`.
- **Section tabs:** `role="tablist"` + `role="tab"` + `aria-selected`. Arrow key navigation within tab bar.
- **DataTable:** `role="grid"`, column headers use `scope="col"`, sortable headers announce sort direction via `aria-sort`.
- **ConfirmModal:** `role="dialog"`, `aria-modal="true"`, focus trap on open, returns focus on close. Escape key dismisses.
- **Toast:** `role="status"` (non-blocking) or `role="alert"` (error). Auto-dismiss after 3s, keyboard-dismissible.
- **Inspector empty/selected:** `aria-live="polite"` region so screen readers announce context changes.
- **Color contrast:** All text combinations must meet WCAG AA (4.5:1 for body, 3:1 for large text). `--ink` on `--canvas` is ~14:1. `--primary` on `--canvas` is ~8.5:1.
- **Focus rings:** `--primary` focus ring (2px offset) on all interactive elements. Never remove `outline` without a visible replacement.
- **Icons:** All Tabler icons used as visual-only get `aria-hidden="true"`. Icons used as the only label get `aria-label`.

---

## Edge Cases

- **Long names (47+ chars):** Section titles, job names, session IDs, channel names — all truncate with `text-overflow: ellipsis`. Full value shown in inspector or on hover via `title` attribute.
- **Zero-width inspector:** If workspace width < 900px at desktop, inspector auto-collapses. User can still re-expand manually.
- **Cron expression display:** Show human-readable label ("Every day at 9am") alongside raw expression. If expression is unparseable, show raw only with a warning badge.
- **Long run transcript:** Timeline virtualizes after 50 items (only renders visible rows + buffer). Load-more button anchored to top.
- **Disconnected channel:** Channel row shows `ERROR` badge with last-seen timestamp. "Reconnect" CTA in inspector.
- **Config apply in progress:** Apply button enters loading state, workspace dims slightly (opacity 0.6 on non-apply controls). Escape key blocked during apply.
- **Approval queue overflow (>20 pending):** Table paginates at 20 items. Count badge in LeftNav nav item shows total count, capped at "99+" display.
- **LeftNav badge overflow:** If a section has an unread count, show a compact badge (JetBrains Mono 700, max 2 chars wide). Only Chat and Exec Approvals show count badges.

---

## Shared Component Library

| Component | Purpose |
|---|---|
| `LeftNav` | Left rail with icon + label nav items, collapsible |
| `TopBar` | Workspace selector, connection status, search, notifications |
| `WorkspaceHeader` | Section title + tabs |
| `RightInspector` | Collapsible right panel for selected object details |
| `SectionTabs` | Tab bar for within-section navigation |
| `DataTable` | Dense sortable/filterable table with row actions |
| `Drawer` | Slide-over panel for forms and details |
| `SplitPane` | Resizable two-pane layout |
| `Timeline` | Vertical event timeline. Each event: role stamp (USER/AGENT/SYSTEM) + timestamp + content. `--surface` background, 1px bottom border between events. No bubbles. |
| `EventCard` | Nested inside Timeline agent events. `--surface-elevated` background, `TOOL` stamp + tool name + duration badge. Collapsed by default after completion; click to expand full I/O in `RawEditor`-style block. |
| `LogStream` | Virtualized live log output. Rows: `timestamp (mono) · level badge · message (DM Sans)`. Filter toolbar above. Auto-scroll toggle (pinned to bottom when on). |
| `FormRenderer` | Schema-driven form for Config |
| `RawEditor` | JSON5 editor using **CodeMirror 6** (`@codemirror/lang-json` + `codemirror` core). ~100KB bundle. Used for Config raw tab and tool call I/O expansion in Chat. |
| `ValidationBanner` | Config validation result |
| `ConfirmModal` | Confirmation gate for dangerous actions |
| `Toast` | Non-blocking notification |
| `CommandPalette` | ⌘K search and section jump |

---

## Real-time Model

Subscribe to a unified event stream for:
- Chat token stream
- Tool call started/finished
- Job started/finished
- Session updated
- Presence updated
- Log line appended
- Notification created

---

## Design System

Follows `DESIGN.md` — Warm Brutalism:
- Canvas `#09090C`, Surface `#0F0F14`, Surface raised `#141419`
- Primary `#F97316`, Signal `#22D3EE`
- Space Grotesk (display) + DM Sans (body) + JetBrains Mono (stamps/data)
- 0px radius structural, 1.5px borders, 8px base unit

---

## Architecture Notes

### Go Server SPA Route Registration

The Go backend (`internal/web/server.go`) registers each SPA page route explicitly. Adding the 12 new frontend routes requires updating `server.go` and `paths.go`. Phase 1 implementation must:

1. Add new page constants to `paths.go`: `pageChat`, `pageChannels`, `pageInstances`, `pageSessions`, `pageCron`, `pageSkills`, `pageNodes`, `pageApprovals`, `pageConfig`, `pageDebug`, `pageLogs`, `pageUpdate`.
2. Register `handleSPADocument` for each new route in `server.go`.
3. Remove registrations for deleted routes: `pageWork`, `pageTeam`, `pageKnowledge`, `pageRecover`, `pageConversations`, `pageAutomate`, `pageHistory`, `pageSettings`.

### Inspector Content Delivery

The current `AppShell` receives static `inspectorItems` props. The new `RightInspector` needs dynamic content (section-level summary vs. object detail). Pattern:

- Each section page calls `setInspectorContent(content)` via a Svelte context function provided by `AppShell`.
- `RightInspector` reactively renders the current content from that context store.
- Default (no content set): section-level summary from a static config map (one entry per section — title, stat slots, primary CTA).
- Reuses the existing `getInspectorItems()` context store pattern in `$lib/shell/inspector.svelte`.

### surfaces.ts Migration

`$lib/config/surfaces.ts` currently maps `SurfaceID` (8 values) to inspector metadata. This becomes the `LeftNav` section config for 12 sections. Phase 1 replaces `SurfaceID` with the 12 new section IDs and removes the old static inspector item pattern.

### Chat Section API Mapping

| Spec action | Backend endpoint | Notes |
|---|---|---|
| Send (chat.send) | `POST /api/work` | Creates a new run |
| Stop (chat.abort) | `POST /api/work/{id}/dismiss` | Dismisses interrupted run |
| Inject note (chat.inject) | Not yet implemented | Placeholder in Phase 1 — show "coming soon" in composer |
| Load history (chat.history) | `GET /api/work` + `GET /api/history` | Paginated run list |
| Live stream | `GET /api/work/{id}/events` (SSE) | Reuse `connectEventStream()` |

---

## Implementation Phases

### Phase 1 — Foundation (this session)
1. `layout.css` — new design tokens + utility classes (remove old `--gc-*`, add new, remove decorative grid gradient)
2. New `AppShell.svelte` — left rail (56px/200px toggle) + workspace + collapsible right inspector
3. Core shared components: `LeftNav`, `WorkspaceHeader`, `SectionTabs`, `RightInspector`
4. Replace `surfaces.ts` — new 12-section config (`SurfaceID` expanded, old inspector item pattern removed)
5. New route structure:
   - Create: `/chat`, `/channels`, `/instances`, `/sessions`, `/cron`, `/skills`, `/nodes`, `/approvals`, `/config`, `/debug`, `/logs`, `/update`
   - Delete: `/work`, `/team`, `/knowledge`, `/recover`, `/conversations`, `/automate`, `/history`, `/settings`
   - Delete old route tests: `work/page.test.ts` and any tests asserting old routes
6. Update `internal/web/paths.go` — add new page constants, remove old ones
7. Update `internal/web/server.go` — register new SPA routes, remove old registrations
8. Update `loadAppShell` entry href: `/work` → `/chat`
9. Login + Onboarding updated to new design tokens
10. `SplitPane` if needed: implement as CSS Grid with drag handle (no third-party library)
11. All `localStorage` reads/writes wrapped in try/catch (silent fallback when storage is unavailable — private browsing mode)

### Phase 1 — Test Requirements

Unit tests (Vitest, `*.test.ts` alongside components):

| Component/file | Tests required |
|---|---|
| `LeftNav.svelte` | Active item highlight for current route; icon-only vs labeled mode; count badge renders for Chat/Approvals |
| `AppShell.svelte` | Rail toggle state (expand/collapse); `localStorage` key `gc-rail-expanded` written on toggle; inspector shows section summary when no item prop; inspector shows object detail when item prop provided |
| `SectionTabs.svelte` | Arrow key navigation updates `aria-selected`; selected tab is accessible |
| `RightInspector.svelte` | Collapse/expand toggle; renders slot content correctly |
| `surfaces.ts` (new) | All 12 section IDs export correctly; old 8 IDs absent |

Go unit tests (`internal/web/`):

| File | Tests required |
|---|---|
| `paths.go` | New page path constants return expected strings |
| `server_test.go` | New SPA routes (`GET /chat`, `/channels`, etc.) return 200; old routes (`GET /work`, `/team`, etc.) return 404 |

E2E tests (Playwright, `*.e2e.ts`):

| Flow | Test |
|---|---|
| Navigation | Click each nav item → URL updates, workspace header updates |
| Rail toggle | Click collapse button → rail narrows to 56px; persist across reload |

### Phase 2 — Live sections
6. Chat (`/chat`) — transcript, composer, run events, live stream
7. Channels (`/channels`) — connector list, login flows
8. Sessions (`/sessions`) — session list, overrides, history
9. Cron Jobs (`/cron`) — job list, run history, editor
10. Exec Approvals (`/approvals`) — policy editor, allowlists
11. Config (`/config`) — form renderer, raw editor, apply

### Phase 3 — Placeholder sections
12. Instances, Skills, Nodes, Debug, Logs, Update — shell + placeholder states

---

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| Design Review | `/plan-design-review` | UI/UX gaps | 2 | clean ✓ | score: 4/10 → 10/10, 13 decisions made |
| Eng Review | `/plan-eng-review` | Architecture & tests (required) | 1 | clean ✓ | 8 issues, 0 critical gaps, 20 test gaps covered |
| CEO Review | `/plan-ceo-review` | Scope & strategy | 0 | — | — |
| Codex Review | `/codex review` | Independent 2nd opinion | 0 | — | — |

**Eng Review Summary (2026-03-29):**
- **Architecture:** 4 issues — WebSocket→SSE (fixed), roles deferred to Phase N, SPA route registration added, inspector content delivery mechanism specified
- **Code Quality:** 4 issues — surfaces.ts migration, SplitPane without library, old test deletion, entry href updated
- **Tests:** 20 gaps — unit tests for 5 components, Go server route tests, 2 E2E flows
- **Performance:** localStorage guards, DataTable pagination note for Phase 2
- **Critical gaps:** 2 (localStorage private-mode guard added; SPA 404 covered by Go test)
- **Dependencies added:** `@codemirror/lang-json` + `codemirror` core (~100KB)

**VERDICT:** CLEARED — Design + Eng reviews passed. Ready to implement Phase 1.
