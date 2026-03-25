# Design System — GistClaw

## Product Context

- **What this is:** A local-first multi-agent runtime for software repo tasks. The operator submits work, approves risky actions, and inspects how the runtime delegated, executed, and recovered that work.
- **Who it's for:** Developers and operators running GistClaw locally on their own machine.
- **Space/industry:** Local AI developer tools and operator dashboards. Adjacent to repo-task assistants, terminal-native tools, and control-plane-style internal tooling.
- **Project type:** Local operator web app. Desktop-first, but fully responsive. Server-rendered with Go `html/template`, no frontend framework, no build step.

## Aesthetic Direction

- **Direction:** Orchestrated Workshop Brutalism
- **Decoration level:** Minimal
- **Mood:** A disciplined workbench for multi-agent operations. The product should feel serious, local, inspectable, and in control. It must communicate ownership, blockage, approval state, and recovery paths without looking crowded.
- **Reference sites:** None. This system was derived from product context and an approved in-repo preview, not competitive research.
- **What to avoid:** Soft gradients, drop shadows, generic dashboard gloss, rounded-corner friendliness, background status tints, or any page composition where multiple panels compete equally for attention.

## Core Principle

The UI is organized around **operator jobs first** and **system nouns second**.

Top-level navigation should answer:

1. What am I operating right now?
2. What shapes future runs?
3. What needs intervention or recovery?

The system nouns still matter, but they live one level down instead of all competing equally in the top bar.

## Information Architecture

### Top-Level Navigation

```text
gistclaw | Operate | Configure | Recover                         [Start Task] [Theme]
```

- `Operate` is for active work and runtime inspection.
- `Configure` is for shaping future behavior.
- `Recover` is for approvals, routing, and delivery intervention.
- `Start Task` is a persistent primary action. It is not buried as just another page.

### Second-Level Navigation

- **Operate**
  - `Runs`
  - `Sessions`
- **Configure**
  - `Team`
  - `Memory`
  - `Settings`
- **Recover**
  - `Approvals`
  - `Routes & Deliveries`

### Naming Rules

- Use job-oriented labels at the top level.
- Use precise system nouns inside each section.
- Rename the current `Control` page to **Routes & Deliveries**. The new name says exactly what the page is for.
- Treat `Onboarding` as a temporary setup flow, not a peer destination once the workspace is bound.

## Page Roles

Every page must have one primary sentence of purpose. If a page cannot be described in one sentence, its hierarchy is wrong.

### Operate

- **Runs:** The operational queue. This page answers what is active, blocked, pending approval, completed, or failed.
- **Run Detail:** The live execution board for one run. This page owns orchestration, status, output, and evidence.
- **Sessions:** The mailbox and actor directory. It explains who the runtime actors are and what they have been told.
- **Session Detail:** Conversation, route, and delivery context for one session. It should not repeat the role of Run Detail.

### Configure

- **Team:** The builder for future runs. This is where the operator shapes default collaboration behavior.
- **Memory:** Editable remembered facts that shape future assistant behavior.
- **Settings:** Machine-level and runtime-level configuration only.

### Recover

- **Approvals:** The operator decision queue. Open approval work should dominate; resolved history is secondary.
- **Routes & Deliveries:** The recovery bench for route bindings, delivery failures, retries, and operator interventions.

## Graph Placement

The orchestration graph is a core product concept. It must stay central.

- `Operate > Runs` includes a **compact live orchestration strip** near the top of the page.
  - Purpose: show the current collaboration shape without turning the queue into a full forensic view.
- `Run Detail` includes the **full collaboration graph as the first major panel**.
  - Purpose: explain ownership, delegation, and blockage before the operator reads output or event logs.
- `Sessions` does **not** own the main graph.
  - Sessions are important, but they are supporting runtime structure, not the primary explanation of a run.

## Hierarchy Rules

- A page header explains the page in plain language before any dense panel appears.
- Each page has one **primary board** and any number of **secondary evidence panels**.
- The primary board appears first and should be visually dominant.
- Reference material, metadata, or historical evidence should never compete with the primary board for equal emphasis.
- Filters belong above the directory they control, not mixed into unrelated control panels.
- Actions belong beside the object they act on. Avoid mixing file import/export, editing, and recovery actions into one visual cluster unless they are directly related.

## Typography

- **Body/UI:** `system-ui, -apple-system, "Segoe UI", sans-serif`
  - Local, fast, reliable, and visually quiet.
- **Code/Metadata:** `"JetBrains Mono", "Fira Code", monospace`
  - Use for run IDs, session IDs, routes, timestamps, tokens, paths, technical metadata, and graph facts.
- **Weight philosophy:** Prefer strong jumps in emphasis instead of many near-identical weights. The app should mainly rely on `400` and `700`, with limited `600` use for section labels.
- **Scale:**

  | Role | Size | Weight | Usage |
  |------|------|--------|-------|
  | Page title | 28px | 700 | Primary page heading |
  | Section title | 16px | 700 | Major panel heading |
  | Card title | 14px | 700 | Row and card emphasis |
  | Body | 14px | 400 | Standard reading text |
  | Secondary | 13px | 400 | Descriptions and supporting copy |
  | Label | 11px | 700 | Eyebrows, small labels, grouped controls |
  | Mono metadata | 12px | 400/500 | IDs, timestamps, routes, graph facts |

- **Line height:** body `1.5`, headings `1.2`, monospace `1.6`
- **Letter spacing:** uppercase labels `0.08em`, prose `0`
- **Loading:** JetBrains Mono via Google Fonts CDN or a self-hosted equivalent. Body text must not depend on a remote font.

## Color

- **Approach:** Restrained warm monochrome. Color communicates state and interactivity, not decoration.
- **Primary action:** `#1c5dff`
- **Primary hover:** `#1848c7`
- **Canvas:** `#ede5d8`
- **Surface:** `#fffdf8`
- **Ink:** `#1c1917`
- **Secondary text:** `#6b6258`
- **Tertiary text:** `#9b9083`

### Semantic Colors

- **Active:** `#0284c7`
- **Approval:** `#b45309`
- **Success:** `#15803d`
- **Error:** `#dc2626`
- **Muted/interrupted:** `#6b7280`

### Neutrals

- **Light neutrals:** `#fffdf8`, `#f7f0e5`, `#ede5d8`
- **Border neutrals:** `#cfc5b6`, `#1c1917`

### Dark Mode

- **Strategy:** Night-shift control room, not literal inversion.
- **Dark canvas:** `#12100e`
- **Dark surface:** `#1b1815`
- **Dark border:** `#f5f0e8`
- **Dark secondary border:** `#4a433c`
- **Dark text:** `#f5f0e8`, `#b6aa9a`, `#8f8477`
- **Dark action:** `#6ea0ff`
- **Rule:** Preserve the same meaning in both themes. Dark mode changes atmosphere, not hierarchy.

### Status Usage Rule

Status colors appear on borders, text, and graph rails. Avoid soft filled boxes for system state.

Preferred pattern:

```css
.status-panel {
  border: 1.5px solid var(--border-hard);
  border-left: 4px solid var(--active);
}
```

Use filled backgrounds only where convention requires them for legibility, such as code diffs.

## Spacing

- **Base unit:** 4px
- **Density:** Comfortable. The app should feel ordered and deliberate, not cramped.
- **Scale:** `2, 4, 8, 12, 16, 24, 32, 48`
- **Priority gaps:**
  - page header to primary board: `24px`
  - primary board to secondary board: `32px`
  - controls inside one board: `12px` to `16px`
  - micro relationships like icon + label or badge + text: `4px` to `8px`

## Layout

- **Approach:** Hybrid
  - top-level navigation is grouped by operator jobs
  - page internals remain grid-disciplined and brutalist
- **Grid:** 12 columns desktop, 8 tablet, 4 mobile
- **Max content width:** `1080px`
- **Shell:** full-width top bar with a centered content column below it
- **Border radius:** `0px` everywhere
- **Shadows:** none

### Responsive Rules

- Never depend on horizontal page scroll for primary workflows.
- Top-level jobs must remain visible and understandable on mobile.
- Sub-navigation may wrap, but must stay readable.
- Multi-panel detail pages stack cleanly on smaller screens.
- The graph must remain readable on narrow widths through stacking and content reduction, not by becoming inaccessible.

## Page Composition Rules

### Runs

- Header explains the queue in plain language.
- Compact graph strip appears near the top.
- Queue rows appear below the graph strip.
- Rows emphasize:
  - run ID
  - objective
  - owner
  - current blocker
  - status

### Run Detail

- Header + live status banner
- Full collaboration graph first
- Execution snapshot second
- Output and event timeline after the graph
- Approval content appears inline when needed, but the graph still stays first

### Sessions

- Sessions are navigable and inspectable, but visually quieter than the run queue.
- Session Detail focuses on mailbox, route context, and delivery state.
- Do not make Session Detail feel like a second run dashboard.

### Team

- Team cards are operational objects, not profile cards.
- Show:
  - agent ID
  - role
  - tool posture
  - collaboration edges
  - capability summary
- Import/export actions should be visually separated from save/edit actions.

### Approvals

- Pending approval work appears first and most prominently.
- Resolved history is available but visually demoted.
- Approval actions must sit directly on the approval card they affect.

### Routes & Deliveries

- This page is a recovery bench, not a generic status dump.
- Separate route bindings, live delivery queues, and route history into clearly named panels.
- Keep retry and deactivate actions beside the exact route or delivery item they affect.

## Components

- **Panels:** hard borders, no shadows, no decorative fills
- **Buttons:** rectangular, strong contrast, no gradient fills
- **Badges:** compact, mono-friendly, used for state or category
- **Eyebrows:** uppercase mono labels for page and section framing
- **Tables and card directories:** use tables on wide screens, card stacks on narrow screens
- **Forms:** grouped by task; do not mix unrelated intents in one undifferentiated field column

## Motion

- **Approach:** Minimal-functional
- **Use motion only for:** live status updates, section transitions, expand/collapse, and state changes that improve comprehension
- **Avoid:** decorative animation, bounce, float, or motion that competes with live operational signals
- **Easing:** enter `ease-out`, exit `ease-in`, move `ease-in-out`
- **Duration:** micro `50-100ms`, short `150-250ms`, medium `250-400ms`

## Decisions Log

| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-03-25 | Reframed the app around operator jobs: Operate, Configure, Recover | The previous top-level structure exposed too many system nouns at once and made several pages feel overlapping or unclear |
| 2026-03-25 | Kept the warm brutalist visual language | The identity already fit the product; the problem was page hierarchy, not visual brand mismatch |
| 2026-03-25 | Kept a compact graph on Runs and the full graph on Run Detail | The graph is central to the product story and should stay visible without overwhelming the queue |
| 2026-03-25 | Renamed Control to Routes & Deliveries | The new name explains the page's purpose directly |
