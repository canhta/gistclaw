# Design System — GistClaw

## Product Context
- **What this is:** A local-first multi-agent assistant platform and control deck for software repo work. The user should feel like they are steering work through one capable assistant surface, not navigating an exposed pile of subsystems.
- **Who it's for:** Developers and operators who want one local-first assistant surface with real orchestration, recovery, and evidence — not a thin chat shell or a generic admin dashboard.
- **Space/industry:** Local AI developer tools, assistant platforms, orchestration control planes, operator workspaces. Adjacent reference products: n8n, Trigger.dev, Langfuse, Temporal.
- **Project type:** Desktop-first but responsive web application in SvelteKit with Tailwind CSS and `@xyflow/svelte`. Go runtime is the authority for auth, API, SSE, and orchestration state.

## Aesthetic Direction
- **Direction:** Warm Brutalism
- **Decoration level:** Intentional
- **Mood:** GistClaw is a working control surface. Hard edges, stamped labels, exposed structure, and orange authority — but executed with a clear three-register vocabulary so the interface has real visual hierarchy, not a uniform coat of brutalism applied everywhere.
- **Three registers:**
  - **PAGE** — Large, bold, Space Grotesk. Page titles and major headings dominate visually.
  - **CHROME** — JetBrains Mono 700 ALL CAPS with letter-spacing. Navigation items, tab labels, section eyebrows, status stamps.
  - **CONTENT** — DM Sans 400/500. Readable body copy, descriptions, panel text. Never monospace.

## Core Product Stance
- GistClaw is not a small operator dashboard with a graph bolted on.
- GistClaw is not a workflow builder first.
- GistClaw is not an observability tool first.
- GistClaw is an assistant platform cockpit. The UI must expose the machine itself: command intake, orchestration graph, active lanes, session routes, approvals, recoveries, memory, and connector state.

## User Point Of View
- The product must speak in the user's job language first and the system's internal language second.
- The user should immediately understand:
  - what they can do now
  - what the assistant is doing for them
  - what needs their decision
  - what just happened and why
- System nouns are allowed where precision matters, but they should live inside work surfaces and inspectors, not dominate the top-level mental model.
- Every major page should answer a human task before it answers a system question.

### UI Language Rules
- Prefer task language over subsystem language wherever the user's job is not inherently technical.
- Labels should describe intent and outcome, not implementation detail.
- The UI should avoid making the user think in connector plumbing, table names, or runtime policy terminology just to navigate.
- Precision terms such as route, approval, replay, or connector are valid when the user is already doing recovery or diagnostic work.

## Information Architecture

### Top-Level Navigation

```text
GistClaw | Work | Team | Knowledge | Recover | Conversations | Automate | History | Settings
```

- `Work` is the front door. Active command intake, current runs, and immediate machine signal.
- `Team` is where the user understands who is helping, how collaboration is shaped, and which roles are active.
- `Knowledge` is scoped, durable context for future work, not a hidden implementation table.
- `Recover` is the bench for approvals, blocked runs, retries, replay inspection, and route repair.
- `Conversations` owns session routes, connector health, delivery visibility, and external surfaces.
- `Automate` owns wakeups, recurring tasks, and future execution timing.
- `History` owns replay evidence, run history, delivery evidence, and operator-visible machine facts.
- `Settings` is machine and deployment configuration only.

### Navigation Rules
- Navigation items use JetBrains Mono 700 ALL CAPS — they are chrome stamps, not document links.
- The most important surfaces belong in the first rail, not buried behind "configure" groupings.
- Top-level items are product capabilities, not documentation nouns.
- If a page feels like a passive list, it is under-designed.

## Layout
- **Approach:** Hybrid command-workspace
- **Grid:** 16 columns desktop, 10 tablet, 4 mobile
- **Max content width:** `1600px`
- **Shell:** persistent application shell with:
  - left navigation rail (sidebar) for system families
  - central workspace for graph, boards, run surfaces, and live activity
  - right inspector for details, actions, and evidence (collapsible)
- **Border radius:** `0px` on all structural elements. `2px` only for pill badges and small circular indicators.

### Layout Principles
- The primary mental model is workspace, not page stack.
- Most major surfaces should be split into: navigation or lane selection → active workspace or board → inspector or evidence sidebar.
- Run graph, route graph, and active system signal belong in the central workspace, not buried below tables.
- Secondary details should move into inspector panels before adding more full-width sections.
- The UI should feel wider and more capable than the current product, not denser and more cramped.

## Page Roles

### Work
- The primary cockpit surface.
- Owns: command intake, current objective, orchestration graph, active run or lane state, immediate machine signal.
- It should feel like the operator is steering the system live.

### Team
- The topology and posture surface.
- Show: front agent, specialists, delegation posture, tool families, execution recommendations, current responsibilities and lane occupancy.

### Knowledge
- The scoped context surface.
- Show: promoted memory, project-scoped rules, machine-level facts, why each memory item matters.
- Do not render it as a bare key-value admin table.

### Recover
- The intervention bench.
- Show: approval queue, blocked runs, replay evidence, route repair actions, delivery retry actions.
- Pending operator work must dominate resolved history.

### Conversations
- The connector and route authority surface.
- Show: bound sessions, route ownership, connector health, active delivery states, last-success and last-failure evidence.

### Automate
- The future-work surface.
- Show: next wakeups, recent executions, lane occupancy, schedule health.
- It should feel operational, not calendar-like.

### History
- The evidence surface.
- Show: run history, replay, operator interventions, delivery outcomes, durable runtime evidence.

## Graphs And XYFlow
- Graphs are first-class product surfaces, not decorative illustrations.
- `@xyflow/svelte` should be used to render orchestration, team topology, and route graphs.
- Graphs must behave like instrument panels:
  - hard-edged nodes
  - visible rails and route lines
  - minimal glow
  - strong labels
  - state expressed by borders, rails, and badges before animation
- Node cards should look like mounted modules, not floating pastel cards.
- The graph must remain readable on smaller screens by collapsing inspector detail and simplifying labels, not by hiding the graph.

## Typography

### Font Stack
- **Display/Hero:** `Space Grotesk`
  - Use for page titles, command deck headings, major counters, and system-level callouts.
  - Rationale: sharp, technical, assertive — not overused in developer tooling.
- **Body:** `DM Sans`
  - Use for all readable text: descriptions, panel copy, operational instructions, form labels.
  - Rationale: excellent readability at 13-14px, clean without being clinical. Replaces mono in body contexts to create real typographic hierarchy.
- **Chrome/Stamps:** `JetBrains Mono`
  - Use for: navigation items (ALL CAPS), tab labels (ALL CAPS), section eyebrows (ALL CAPS), status badges, counter chips.
  - Always uppercase with `letter-spacing: 0.07–0.1em` in navigation and structural chrome.
  - Rationale: turns the interface chrome into an instrument panel without applying the treatment to all readable content.
- **Data/Tables:** `JetBrains Mono`
  - Use for: session IDs, timestamps, token counts, run IDs, connector facts, schedule expressions.
  - Must use `font-variant-numeric: tabular-nums`.
  - Rationale: machine facts should read like machine facts.
- **Code:** `JetBrains Mono`
  - Use for: logs, tool traces, command snippets, policy text, code-like content.
- **Loading:** Self-hosted or Google Fonts CDN during development. Final shipped UI should not depend on fragile third-party font delivery for basic readability.

### Type Scale
- Display XL:    `56px / Space Grotesk 700 / -0.03em`
- Display L:     `40px / Space Grotesk 700 / -0.025em`
- Page title:    `28px / Space Grotesk 700 / -0.02em`
- Panel title:   `18px / Space Grotesk 700 / default`
- Body:          `14px / DM Sans 400 / 1.55`
- Secondary:     `13px / DM Sans 400 / 1.5`
- Chrome stamp:  `11px / JetBrains Mono 700 / uppercase / 0.08em`
- Machine meta:  `12px / JetBrains Mono 400 / default`
- Badge:         `10px / JetBrains Mono 700 / uppercase / 0.06em`

### Typography Rules
- Use Chrome stamps (mono 700 ALL CAPS) for navigation, tabs, and section eyebrows only — not for body content.
- Body content always gets DM Sans. Never render readable paragraphs or descriptions in monospace.
- Use Space Grotesk at large sizes with negative letter-spacing for maximum impact.
- Do not use too many weights. Prefer stark contrast between display (700) and body (400/500).
- Do not use soft editorial italics or decorative serif accents.

## Color

### Approach
Restrained, high-contrast, signal-driven. Orange carries operator authority. Cyan carries live machine state.

### Surface Palette
```
--canvas:           #09090C  /* near-black, faint cool undertone */
--surface:          #0F0F14
--surface-raised:   #141419
--surface-elevated: #1B1B22  /* cards, modals, elevated panels */
```

### Brand & Signal
```
--primary:          #F97316  /* amber-orange — operator authority, primary actions */
--primary-hover:    #EA6B0A
--primary-dim:      rgba(249,115,22,0.12)

--signal:           #22D3EE  /* cyan — live machine state, topology, route rails */
--signal-dim:       rgba(34,211,238,0.12)
```

### Semantic
```
--success:          #4ADE80
--success-dim:      rgba(74,222,128,0.12)

--warning:          #FBBF24
--warning-dim:      rgba(251,191,36,0.12)

--error:            #F87171
--error-dim:        rgba(248,113,113,0.12)

--info:             #60A5FA
--info-dim:         rgba(96,165,250,0.12)
```

### Ink
```
--ink:              #EDE8DF  /* warm white — contrast against cool dark canvas */
--ink-2:            #9B8E7F
--ink-3:            #6B5F53
--ink-4:            #3D3529
```

### Borders
```
--border:           #1E1E28
--border-strong:    #2A2A38
```

### Color Usage Rules
- Use orange for operator authority: primary CTAs, approval actions, active-state indicators.
- Reserve orange strictly — do not use it for generic decoration or informational badges.
- Use cyan for machine signal: live state indicators, active route rails, topology graphs, SSE pulse.
- Let borders and surface elevation communicate structure before fills and backgrounds.
- Avoid full-surface pastel status blocks. Use `*-dim` backgrounds (12% opacity) with colored text/border instead.
- Avoid purple as an accent. It is overused in AI tooling and has no semantic role here.

### Dark Mode
- Default mode is dark. The dark theme is the primary product identity.
- The cool near-black canvas (`#09090C`) creates strong contrast for the warm orange primary.
- Glow is allowed only as a minor live-state hint. It must never be the main source of emphasis.
- Gradients are used sparingly and structurally only — not as a blanket visual effect.

### Light Mode
- Light mode preserves the same hard-edged hierarchy.
- It is not a soft inversion. It should feel like a daylight service manual version of the same machine.
- Borders and seams remain visible. Do not wash them out in light mode.

## Spacing
- **Base unit:** `8px`
- **Density:** Comfortable-compact. Page headers breathe. Data rows stay tight.
- **Scale:** `4, 8, 12, 16, 24, 32, 48, 64`

### Spacing Rules
- External panel rhythm:
  - page section headers: `24–32px` vertical padding
  - workspace lane gaps: `16–24px`
  - inspector stack gaps: `12–16px`
- Internal panel rhythm:
  - eyebrow to title: `6–10px`
  - title to content: `10–14px`
  - rows in dense machine views: `8–12px`
- Never use oversized airy spacing that makes the platform feel empty.
- Never compress so far that the interface feels cramped.

## Borders, Panels, And Surfaces
- Hard borders are part of the identity.
- Use `1.5px` borders for all structural surfaces (panels, sidebar, topbar, inspector).
- Use `1px` borders for internal dividers within a panel (table rows, list items).
- Panels should feel mounted into the UI, not floating above it.
- Shadows are minimal to none. Structure comes from borders, contrast, and surface elevation — not elevation fog.
- Blur and glass effects are out of bounds for the shipped product.
- Surface elevation (`--surface` → `--surface-raised` → `--surface-elevated`) communicates depth before borders are needed.

## Components

### Design Language
- **Panels:** hard-edged, `0px` radius, `1.5px` borders, visible seams
- **Buttons:** rectangular (`0px` radius), JetBrains Mono 700 ALL CAPS labels, strong contrast, no gradients
- **Badges:** compact, `2px` radius, JetBrains Mono 700 uppercase, `*-dim` background with semantic color text
- **Eyebrows / Section labels:** JetBrains Mono 700 ALL CAPS, `--ink-3`, `0.1em` tracking
- **Forms:** console inputs — `1.5px` border, `0px` radius, `--surface-raised` background, focus ring in `--primary`
- **Tables:** `1.5px` header border, `1px` row dividers, JetBrains Mono for IDs and timestamps, DM Sans for task descriptions
- **Cards:** mounted module aesthetic — `1.5px` border, `--surface` background, hard corners
- **Inspectors:** right-side detail panels with `1.5px` left border from workspace, stacked fact rows in mono

## DRY And SOLID Design Rules
- One surface, one primary responsibility:
  - `Work` steers current work
  - `Team` shapes collaboration
  - `Knowledge` shapes future behavior through durable context
  - `Recover` handles intervention
  - `Conversations` handles external surface control
  - `Automate` handles future execution
  - `History` explains what happened
- Do not create multiple pages that each partly solve the same job.
- Shared patterns must be extracted once: panel shell, inspector shell, graph node card, status badge, evidence row, action strip.
- Avoid duplicated one-off variants that differ only in icon, spacing, or border treatment.

## Motion
- **Approach:** Minimal-functional
- **Easing:** enter `ease-out`, exit `ease-in`, move `ease-in-out`
- **Duration:** micro `50–100ms`, short `120–180ms`, medium `180–260ms`, long `260–400ms`

### Motion Rules
- Motion exists to reveal state change, not to decorate.
- Graph transitions may ease into place, but must remain crisp.
- Avoid bounce, springiness, float, drift, or atmospheric motion.
- Live indicators (pulse, signal rails) may animate subtly — borders and labels remain the primary signal.

## Anti-Patterns
- Do not apply Brutalist treatment uniformly — the three-register vocabulary must be respected.
- Do not use monospace for body text or readable content — mono is reserved for chrome and machine data.
- Do not drift back into generic admin dashboard layouts.
- Do not use soft cards with subtle shadows as the default UI grammar.
- Do not turn the command surface into a chat page plus a sidebar.
- Do not make the graph feel like a secondary details widget.
- Do not use rounded controls or soft pill buttons for primary actions.
- Do not use purple gradients, glassmorphism, or "AI startup" gloss.
- Do not make every surface visually equivalent — the active workspace must dominate.

## Implementation Guidance
- Use Tailwind for utility expression, but encode the design system as stable CSS custom properties first.
- Build a token layer covering: colors, border weights, panel spacing, typography scale, rail sizes, inspector widths.
- Prefer reusable shell primitives before building page-specific styling.
- Token naming should mirror this document (`--primary`, `--surface-raised`, `--border-strong`, etc.).

## Decisions Log
| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-03-29 | Full redesign from scratch — replaced prior Industrial Operator Brutalism system | Prior execution was monotonous: uniform Brutalist treatment with no register differentiation. The new system keeps Brutalism as the grammar but applies it with intent. |
| 2026-03-29 | Adopted three-register vocabulary: PAGE / CHROME / CONTENT | The root cause of prior monotony was using IBM Plex Mono everywhere regardless of content type. Three registers give the UI real hierarchy without abandoning the industrial identity. |
| 2026-03-29 | DM Sans replaces monospace for body and readable content | Mono text at body sizes degrades readability and removes weight contrast. DM Sans is clean, technical-adjacent, and readable at 13–14px. |
| 2026-03-29 | JetBrains Mono replaces IBM Plex Mono | JetBrains Mono is lighter and crisper at small sizes. Better for the stamp-label register. |
| 2026-03-29 | Canvas changed to near-black cool (#09090C) from warm amber dark | Cool near-black gives orange (#F97316) maximum contrast (~8.5:1). The prior warm canvas (#0F0D0B) competed with the orange's warmth, reducing its impact. |
| 2026-03-29 | Orange (#F97316) remains primary accent | Orange is the operator authority signal. Kept from the prior system. Slightly richer amber-orange vs the prior red-orange. |
| 2026-03-29 | Borders reduced from 2px to 1.5px | Slightly crisper without losing the structural character. 1px for internal panel dividers. |
| 2026-03-29 | Border radius: 0px structural, 2px badges only | Preserves the hard Brutalist edge for all structural elements while allowing badges to read as distinct from panels. |
