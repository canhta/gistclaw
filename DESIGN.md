# Design System — GistClaw

## Product Context
- **What this is:** A local-first multi-agent assistant platform and control deck for software repo work. The user should feel like they are steering work through one capable assistant surface, not navigating an exposed pile of subsystems.
- **Who it's for:** Developers and operators who want one local-first assistant surface with real orchestration, recovery, and evidence, not a thin chat shell or a generic admin dashboard.
- **Space/industry:** Local AI developer tools, assistant platforms, orchestration control planes, and operator workspaces. Closest DNA: OpenClaw and GoClaw. Adjacent reference products: n8n, Trigger.dev, Langfuse, Temporal.
- **Project type:** Desktop-first but responsive web application, being rewritten in SvelteKit with Tailwind CSS and `@xyflow/svelte`, while the Go runtime remains the authority for auth, API, SSE, and orchestration state.

## Aesthetic Direction
- **Direction:** Industrial Operator Brutalism
- **Decoration level:** Intentional
- **Mood:** GistClaw should feel like a working control surface, not a polished SaaS dashboard. It should look assembled, instrumented, and authority-bearing: visible seams, hard panels, stamped labels, live signal rails, and enough visual heat to show that the system is active.
- **Reference sites:** [https://openclaw.ai](https://openclaw.ai), [https://github.com/openclaw/openclaw](https://github.com/openclaw/openclaw), [https://docs.goclaw.sh](https://docs.goclaw.sh), [https://n8n.io](https://n8n.io), [https://trigger.dev](https://trigger.dev), [https://langfuse.com](https://langfuse.com), [https://temporal.io](https://temporal.io)

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
gistclaw | Work | Team | Knowledge | Recover | Conversations | Automate | History | Settings
```

- `Work` is the front door. It replaces the feeling of a passive dashboard with an active control deck.
- `Team` is where the user understands who is helping, how collaboration is shaped, and which roles are active.
- `Knowledge` is scoped, durable context for future work, not a hidden implementation table.
- `Recover` is the bench for approvals, blocked runs, retries, replay inspection, and route repair.
- `Conversations` owns session routes, connector health, delivery visibility, and external surfaces.
- `Automate` owns wakeups, recurring tasks, and future execution timing.
- `History` owns replay evidence, run history, delivery evidence, and operator-visible machine facts.
- `Settings` is machine and deployment configuration only.

### Navigation Rules
- The shell must feel like a machine console, not a website navbar.
- The most important surfaces belong in the first rail, not buried behind “configure” groupings.
- Top-level items are product capabilities, not documentation nouns.
- If a page feels like a passive list, it is under-designed.

## Layout
- **Approach:** Hybrid command-workspace
- **Grid:** 16 columns desktop, 10 tablet, 4 mobile
- **Max content width:** `1600px`
- **Shell:** a persistent application shell with:
  - left navigation rail for system families
  - central workspace for graph, boards, run surfaces, and live activity
  - right inspector for details, actions, and evidence
- **Border radius:** `0px` by default. Exception: tiny circular indicators only.

### Layout Principles
- The primary mental model is workspace, not page stack.
- Most major surfaces should be split into:
  - navigation or lane selection
  - active workspace or board
  - inspector or evidence sidebar
- Run graph, route graph, and active system signal belong in the central workspace, not buried below tables.
- Secondary details should move into inspector panels before adding more full-width sections.
- The UI should feel wider and more capable than the current product, not denser and more cramped.

## Page Roles

### Work
- The primary cockpit surface.
- Owns:
  - command intake
  - current objective
  - orchestration graph
  - active run or lane state
  - immediate machine signal
- It should feel like the operator is steering the system live.

### Team
- The topology and posture surface.
- Show:
  - front agent
  - specialists
  - delegation posture
  - tool families
  - execution recommendations
  - current responsibilities and lane occupancy

### Knowledge
- The scoped context surface.
- Show:
  - promoted memory
  - project-scoped rules
  - machine-level facts
  - why each memory item matters
- Do not render it as a bare key-value admin table.

### Recover
- The intervention bench.
- Show:
  - approval queue
  - blocked runs
  - replay evidence
  - route repair actions
  - delivery retry actions
- Pending operator work must dominate resolved history.

### Conversations
- The connector and route authority surface.
- Show:
  - bound sessions
  - route ownership
  - connector health
  - active delivery states
  - last-success and last-failure evidence

### Automate
- The future-work surface.
- Show:
  - next wakeups
  - recent executions
  - lane occupancy
  - schedule health
- It should feel operational, not calendar-like.

### History
- The evidence surface.
- Show:
  - run history
  - replay
  - operator interventions
  - delivery outcomes
  - durable runtime evidence

## Graphs And XYFlow
- Graphs are first-class product surfaces, not decorative illustrations.
- `@xyflow/svelte` should be used to render:
  - orchestration graph on `Command`
  - team/delegation topology on `Agents`
  - route and delivery topology on `Channels` or `Recover` when useful
- Graphs must behave like instrument panels:
  - hard-edged nodes
  - visible rails and route lines
  - minimal glow
  - strong labels
  - state expressed by borders, rails, and badges before animation
- Avoid playful graph styling, soft blobs, or generic “workflow canvas” aesthetics.
- Node cards should look like mounted modules, not floating pastel cards.
- The graph must remain readable on smaller screens by collapsing inspector detail and simplifying labels, not by hiding the graph.

## Typography
- **Display/Hero:** `Space Grotesk`
  - Use for page titles, command deck headings, major counters, and system-level callouts.
  - Rationale: sharp, technical, assertive, and less generic than typical startup sans choices.
- **Body:** `Instrument Sans`
  - Use for reading text, controls, panel copy, and operational instructions.
  - Rationale: neutral enough for dense UI, but cleaner and more contemporary than system-ui.
- **UI/Labels:** `IBM Plex Mono`
  - Use for stamped labels, panel headers, route facts, statuses, tabs, counters, metadata, and badges.
  - Rationale: turns the interface into an instrument panel rather than a polished content app.
- **Data/Tables:** `IBM Plex Mono`
  - Must use tabular numerals where supported.
  - Rationale: session IDs, timestamps, tokens, connectors, schedules, and run counters should read like machine facts.
- **Code:** `JetBrains Mono`
  - Use for logs, tool traces, command snippets, policy text, and code-like content.
- **Loading:** Use self-hosted fonts or explicit CDN loading during preview and early implementation. Final shipped UI should not depend on fragile third-party font delivery for basic readability.

### Type Scale
- Display XL: `88px / 700 / 0.9`
- Display L: `64px / 700 / 0.92`
- Page title: `40px / 700 / 0.95`
- Section title: `28px / 700 / 1.0`
- Panel title: `18px / 700 / 1.1`
- Body: `15px / 500 / 1.5`
- Secondary: `13px / 500 / 1.45`
- Label: `11px / 700 / 1.0`
- Machine meta: `12px / 500 / 1.45`

### Typography Rules
- Major headings should often be uppercase or near-uppercase when the tone benefits from it.
- Do not use too many weights. Prefer a strong contrast between body and emphasis.
- Use mono labels aggressively where the UI is describing machine state.
- Do not use soft editorial italics or decorative serif accents in the app shell.

## Color
- **Approach:** Restrained, high-contrast, signal-driven
- **Primary:** `#FF5C39`
  - Meaning: operator heat, primary action, escalation, focus, approval-adjacent urgency
- **Primary hover:** `#FF744F`
- **Secondary / Signal:** `#53C7F0`
  - Meaning: topology, live machine state, route signal, orchestration rails, information status
- **Canvas:** `#0A0F14`
- **Surface:** `#121A23`
- **Surface raised:** `#16212D`
- **Surface soft:** `#1D2734`
- **Ink:** `#F4F7FB`
- **Secondary text:** `#97A6B6`
- **Tertiary text:** `#6F8194`
- **Border:** `#314356`
- **Border strong:** `#4C637C`

### Semantic Colors
- **Success:** `#65D98A`
- **Warning:** `#F5B64D`
- **Error:** `#FF6A78`
- **Info:** `#53C7F0`

### Dark Mode
- **Strategy:** Default mode is dark. The dark theme is the primary identity of the product.
- Dark mode should feel like a functioning machine room, not a neon sci-fi fantasy.
- Use gradients sparingly and structurally, not as a blanket visual effect.
- Glow is allowed only as a minor live-state hint, never as the main source of emphasis.

### Light Mode
- Light mode should preserve the same hard-edged hierarchy.
- It is not a soft inversion. It should feel like a daylight service manual version of the same machine.
- Borders and seams remain visible. Do not wash them out.

### Color Usage Rules
- Use orange for operator authority, not for generic decoration.
- Use cyan for machine signal, not for CTA competition.
- Let borders and rails communicate state before fills and backgrounds.
- Avoid full-surface pastel status blocks.
- Avoid purple as a default accent.

## Spacing
- **Base unit:** `4px`
- **Density:** Comfortable-compact
- **Scale:** `2, 4, 8, 12, 16, 24, 32, 48, 64`

### Spacing Rules
- External panel rhythm:
  - page sections: `24-32px`
  - workspace lane gaps: `16-24px`
  - inspector stack gaps: `12-16px`
- Internal panel rhythm:
  - labels to titles: `6-10px`
  - titles to content: `10-14px`
  - rows in dense machine views: `8-12px`
- Never use oversized airy spacing that makes the platform feel empty.
- Never compress so far that the interface feels cramped or “small”.

## Borders, Panels, And Surfaces
- Hard borders are part of the identity.
- Panels should feel mounted into the UI, not floating above it.
- Prefer `2px` borders for primary structural surfaces.
- Use visible seams between navigation, workspace, and inspector columns.
- Shadows are minimal to none. Structure comes from borders, contrast, and layout, not elevation fog.
- Blur and glass effects are out of bounds for the shipped product.

## Components
- **Panels:** hard-edged, visible seams, no rounded friendliness, no soft glass
- **Buttons:** rectangular, mono-friendly labels, strong contrast, no gradients
- **Badges:** compact, stamped, mono, state-driven
- **Eyebrows:** boxed or bordered mono labels, not airy section whispers
- **Forms:** feel like console inputs or control fields, not glossy SaaS forms
- **Tables:** strong row separators, mono-heavy metadata, machine readability first
- **Cards:** should read like mounted modules or control plates, not content marketing cards
- **Inspectors:** right-side detail panels should feel like diagnostic trays

## DRY And SOLID Design Rules
- The UI architecture must follow `DRY` and `SOLID`, not just the backend code.
- One surface, one primary responsibility:
  - `Work` steers current work
  - `Team` shapes collaboration
  - `Knowledge` shapes future behavior through durable context
  - `Recover` handles intervention
  - `Conversations` handles external surface control
  - `Automate` handles future execution
  - `History` explains what happened
- Do not create multiple pages that each partly solve the same job.
- Shared patterns must be extracted once:
  - panel shell
  - inspector shell
  - graph node card
  - status badge
  - evidence row
  - action strip
- Avoid duplicated one-off variants that differ only in icon, spacing, or border treatment.
- If two surfaces present the same class of state, they should use the same visual primitive unless there is a real user-task reason not to.
- Component responsibilities must stay narrow:
  - navigation components navigate
  - graph components explain topology
  - inspector components explain one selected object
  - action components mutate one object or one workflow
  - evidence components explain what happened
- Structural changes and behavior changes should be separated where possible. Do not hide new behavior inside cosmetic refactors.

## Motion
- **Approach:** Minimal-functional
- **Easing:** enter `ease-out`, exit `ease-in`, move `ease-in-out`
- **Duration:** micro `50-100ms`, short `120-180ms`, medium `180-260ms`, long `260-400ms`

### Motion Rules
- Motion exists to reveal state change, not to decorate.
- Graph transitions may ease into place, but should remain crisp.
- Avoid bounce, springiness, float, drift, or atmospheric motion.
- Live indicators may pulse subtly, but borders and labels remain the primary signal.

## Anti-Patterns
- Do not drift back into generic admin dashboard layouts.
- Do not use soft cards with subtle shadows as the default UI grammar.
- Do not turn the command surface into a chat page plus a sidebar.
- Do not make the graph feel like a secondary details widget.
- Do not use overly rounded controls.
- Do not use purple gradients, glassmorphism, or “AI startup” gloss.
- Do not make every surface visually equivalent; the active workspace must dominate.

## Implementation Guidance
- The SvelteKit rewrite should preserve the product promise of a single authoritative Go runtime.
- SvelteKit should own the frontend experience, not redefine the system model.
- Use Tailwind for utility expression, but encode the design system as stable tokens first.
- Build a token layer for:
  - colors
  - border weights
  - panel spacing
  - typography
  - rail sizes
  - inspector widths
- Prefer reusable shell primitives before building page-specific styling.

## Decisions Log
| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-03-28 | Replaced the prior operator-panel design with Industrial Operator Brutalism | The old design made the product feel smaller and more limited than the actual runtime |
| 2026-03-28 | Reframed GistClaw from operator dashboard to assistant platform control deck | OpenClaw and GoClaw DNA point to a broader machine-facing product, not a narrow admin surface |
| 2026-03-28 | Reframed navigation and page naming around user jobs instead of system nouns | The product must read from the user's point of view first, with system precision moved into the work surfaces |
| 2026-03-28 | Chose `Space Grotesk`, `Instrument Sans`, `IBM Plex Mono`, and `JetBrains Mono` | The new system needs a sharper product voice plus machine-readable UI language |
| 2026-03-28 | Chose claw orange and signal cyan as the two key accents | Orange carries operator authority and heat; cyan carries live machine signal and topology |
| 2026-03-28 | Required XYFlow graphs to behave like instrument surfaces, not decorative canvases | The graph is core product evidence and must read as part of the machine itself |
| 2026-03-28 | Made DRY and SOLID explicit design constraints for the UI architecture | The rewrite must avoid overlapping surfaces and one-off component drift, not just backend duplication |
