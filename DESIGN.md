# Design System — GistClaw

## Product Context

- **What this is:** A local-first multi-agent runtime for repo tasks. The operator drops in a task; a configurable agent team plans, executes, and verifies the work; the operator approves risky actions; the system leaves a clear audit trail.
- **Who it's for:** Developers and operators running it locally on their own machine.
- **Space/industry:** Local AI developer tools. Adjacent to: terminal-native tools (Warp), code editors (Zed), lightweight internal dashboards.
- **Project type:** Local operator dashboard — desktop-only, server-rendered (Go `html/template`), no JS framework, no build step.

## Aesthetic Direction

- **Direction:** Orchestrated Workshop Brutalism
- **Decoration level:** None — raw structure is the aesthetic. Hard borders, sharp corners, stark weight contrasts. No gradients, no textures, no illustration, no shadows.
- **Mood:** A serious instrument laid out on a workbench, but with a visible command board for coordination. The UI should explain who owns the work right now, where delegation happened, and what is blocking progress.
- **What to avoid:** Rounded corners (anywhere), drop shadows, subtle tint backgrounds for status, gradient buttons, soft hover states, any animation that isn't information, friendly onboarding iconography.
- **Key differentiator:** Most developer tools are either cold-brutalist (black/white, pure contrast) or polish-minimal (Linear-style). GistClaw uses warm stone neutrals with brutalist structure, darker anchors, and explicit orchestration surfaces. The warmth signals "workshop"; the graph and lane system signal "control room."

## Typography

- **Body/UI:** `system-ui, -apple-system, "Segoe UI", sans-serif`
  — No CDN for body text. Works offline. The font should be invisible.
- **Code/Metadata:** `"JetBrains Mono", "Fira Code", monospace`
  — Run IDs, timestamps, token counts, cost figures, command output, diffs, file paths. Not just code blocks — anywhere the data is technical and precise.
- **Weight philosophy:** Brutalist typography uses weight jumps, not weight steps. Primarily 400 (regular) and 700 (bold). Use 600 for headings. Avoid the 400→500→600 gradation — it creates visual mush.
- **Scale:**

  | Name   | Size | Weight | Usage                                     |
  |--------|------|--------|-------------------------------------------|
  | `2xl`  | 24px | 700    | Primary heading (used sparingly)          |
  | `xl`   | 20px | 700    | Page headings (Runs, Settings)            |
  | `lg`   | 16px | 700    | Section headings within a page            |
  | `md`   | 15px | 700    | Table headers, group labels               |
  | `base` | 14px | 400    | Body text, descriptions, row titles       |
  | `sm`   | 13px | 400    | Secondary text, table cells               |
  | `xs`   | 11px | 400    | Labels, badges, uppercase group headers   |

- **Monospace scale:**

  | Name       | Size | Weight | Usage                                |
  |------------|------|--------|--------------------------------------|
  | `mono-md`  | 13px | 500    | Run IDs, cost, token counts          |
  | `mono-sm`  | 12px | 400    | Diffs, command output                |
  | `mono-xs`  | 11px | 400    | Timestamps, file paths, secondary    |
  | `mono-2xs` | 10px | 400    | Badge text, inline code in labels    |

- **Line height:** body 1.5 · headings 1.2 · monospace 1.6
- **Letter spacing:** uppercase labels `0.08em` · normal prose `0`
- **Font loading:** JetBrains Mono via Google Fonts CDN — `?family=JetBrains+Mono:wght@400;500&display=swap`

## Color

- **Approach:** Warm monochrome base. Color is a signal, not decoration. Status colors appear only on borders and text — never as background fills (except code diffs which require fill for legibility).
- **No subtle tints:** Background tints for state (`active-subtle`, `approval-subtle`) are eliminated. The border IS the state signal.

### Base palette

| Token           | Value     | Usage                                       |
|-----------------|-----------|---------------------------------------------|
| `bg`            | `#ede5d8` | Page background — warm stone canvas         |
| `surface`       | `#fffdf8` | Cards and work surfaces                     |
| `border-hard`   | `#1c1917` | UI chrome borders (cards, buttons, inputs)  |
| `border-soft`   | `#cfc5b6` | Row separators, subtle dividers             |

### Dark palette

Dark mode is not a simple inversion. It should feel like a night-shift control room while preserving the same structural semantics.

| Token           | Value     | Usage                                               |
|-----------------|-----------|-----------------------------------------------------|
| `bg`            | `#12100e` | Page background — warm black canvas                 |
| `surface`       | `#1b1815` | Cards and graph nodes                               |
| `border-hard`   | `#f5f0e8` | High-contrast borders and header anchors            |
| `border-soft`   | `#4a433c` | Dividers and graph connectors                       |
| `text-1`        | `#f5f0e8` | Primary text                                        |
| `text-2`        | `#b6aa9a` | Secondary text                                      |
| `text-3`        | `#8f8477` | Tertiary/meta text                                  |
| `brand`         | `#6ea0ff` | Interactive highlight in dark mode                  |
| `brand-hover`   | `#8bb4ff` | Hover accent in dark mode                           |

### Text

| Token    | Value     | Usage                          |
|----------|-----------|--------------------------------|
| `text-1` | `#1c1917` | Primary — warm near-black      |
| `text-2` | `#6b6258` | Secondary — body, descriptions |
| `text-3` | `#9b9083` | Tertiary — labels, timestamps  |

### Brand

| Token         | Value     | Usage                                          |
|---------------|-----------|------------------------------------------------|
| `brand`       | `#1c5dff` | Interactive element borders, primary actions    |
| `brand-hover` | `#1848c7` | Hover state (border color deepens)              |

Brand color is used ONLY on interactive elements (buttons in primary state, active nav link, focused inputs). It is not used as a background fill.

### Theme system

- **Modes:** `System`, `Light`, `Dark`
- **Default:** Follow system preference unless the operator explicitly overrides it
- **Persistence:** Store the override locally in the browser so long-running operators are not forced to re-toggle every session
- **Rule:** Both themes must preserve the same information hierarchy and state semantics. Theme changes should not change what a color means.

### Semantic — Run states

State is communicated via border color and text color only. No background fills.

| Token       | Value     | Usage                                            |
|-------------|-----------|--------------------------------------------------|
| `pending`   | `#1c5dff` | Queued run/node border and text                  |
| `active`    | `#0284c7` | Active run left border (4px) + status text       |
| `approval`  | `#b45309` | Approval card left border (4px) + heading text   |
| `success`   | `#15803d` | Completed left border (4px) + receipt heading    |
| `error`     | `#dc2626` | Failed left border (4px) + error text            |
| `muted`     | `#6b7280` | Interrupted — muted left border + text           |

**Diff exception:** Code diffs (`+` lines, `-` lines) retain subtle background fills for legibility — this is standard diff convention and does not break the no-fill rule for UI chrome.

### Status pattern — hard left border, no fill

State is communicated via a 4px solid left border. No background tint. The border is the entire visual signal.

```css
.run-row {
  border: 1.5px solid var(--border-hard);
  border-left: 4px solid var(--border-hard);  /* default — warm black */
}
.run-row.is-active    { border-left-color: var(--active); }
.run-row.is-approval  { border-left-color: var(--approval); }
.run-row.is-complete  { border-left-color: var(--success); }
.run-row.is-error     { border-left-color: var(--error); }
.run-row.is-interrupted { border-left-color: var(--muted); }
```

## Spacing

- **Base unit:** 4px
- **Density:** Comfortable — not cramped. Brutalism is about structure, not density.
- **Scale:**

  | Token | Value | Common usage                          |
  |-------|-------|---------------------------------------|
  | `2xs` | 2px   | Micro gaps (badge padding)            |
  | `xs`  | 4px   | Tight gaps (icon + label)             |
  | `sm`  | 8px   | Row internal padding, inline spacing  |
  | `md`  | 12px  | Component internal padding            |
  | `lg`  | 16px  | Card padding, row vertical padding    |
  | `xl`  | 20px  | Section gaps                          |
  | `2xl` | 24px  | Page content padding                  |
  | `3xl` | 32px  | Between major page sections           |
  | `4xl` | 48px  | Top-level page padding                |

## Layout

- **Approach:** Grid-disciplined. Strict column structure. No editorial asymmetry.
- **Desktop-first:** 1024px minimum width. No mobile reflow in v1.
- **Max content width:** 1080px
- **Nav height:** 44px top bar, full-width. `border-bottom: 1.5px solid var(--border-hard)`.
- **Page padding:** 24px horizontal, 32px top.
- **Shell rule:** Use the warm canvas as active negative space. Do not wrap an entire page in one oversized white panel. Prefer a page header on canvas plus smaller work panels beneath it.

### Navigation structure

```
gistclaw  |  Runs  |  Control  |  Sessions  |  Approvals  |  Settings   [spacer]  Theme: System / Light / Dark
```

- No sidebar. Top bar only.
- Brand anchor: `gistclaw` appears in a solid black block with off-white text.
- Active nav item: bold weight (700), text underline offset 4px. No colored bottom border accent — underline directly on the text.
- Theme switcher: segmented control in the shell. It uses the same brutalist button language as the rest of the product.

### Run detail — layout per state

```
ACTIVE:
  [page heading (xl/700) + badge + run ID (mono-xs)]
  [live-status banner — full width, hard border, state-colored left rail]
  [collaboration graph — full width, grouped by delegation depth, statuses live]
  [assistant output 50% left] [event timeline 50% right]

NEEDS APPROVAL:
  [page heading + badge]
  [live-status banner — approval tone]
  [collaboration graph — approval nodes visible in place]
  [approval card — full width, 4px amber left border, no amber bg fill]
  [diff block — code fill for legibility]
  [Approve / Deny buttons]

COMPLETED:
  [page heading + badge]
  [completion banner]
  [static graph with final statuses]
  [assistant output 50% left] [event timeline 50% right]

INTERRUPTED:
  [page heading + badge]
  [interrupted banner]
  [graph showing which branch stopped]
  [partial replay below]
```

### Orchestration graph

- The graph is a command-board view, not a decorative network chart.
- Group runs by delegation depth from left to right or top to bottom depending on viewport.
- Each node shows: status, run ID, objective, agent ID, optional session ID, and parent relationship.
- The graph headline must summarize the current bottleneck: active work, pending queue, approval gate, failure, or completion.
- Live status matters more than edge artistry. If forced to choose, prioritize truthful state over fancy connectors.
- Polling or SSE-backed refresh is acceptable as long as the operator sees status changes quickly and reliably.

## Border Radius

**Zero.** Everything is a rectangle.

| Element                                | Radius  |
|----------------------------------------|---------|
| All UI elements (buttons, cards, inputs, badges, nav) | 0px |
| Avatar/dot indicators                  | 50% (circles only) |

No exceptions for "softness." If it's a box, it's a rectangle.

## Motion

- **Approach:** Minimal-functional. Only transitions that carry information. Brutalist UIs do not animate for personality.
- **Easing:** `ease-out 150ms` for enters. `ease-in 100ms` for exits. No spring, no bounce.
- **What moves:**
  - SSE new run rows: `opacity 0→1, 150ms ease-out` — new row arrives, no position animation
  - Approval card: `opacity 0→1, 150ms ease-out` — no slide, no transform
  - Hover state (button invert): `background 100ms ease-out, color 100ms ease-out`
  - Focus ring: instant (no transition on focus ring itself)
- **What does not move:** Page transitions, card entrances, graph nodes, timeline items.
- **No:** Parallax, scroll-driven animations, loading spinners for <200ms operations, any animation that doesn't directly aid comprehension.

## Components

### Buttons

Default is **ghost** (no fill). Primary actions invert on hover.

| Variant     | Default                              | Hover                          | Usage                          |
|-------------|--------------------------------------|--------------------------------|--------------------------------|
| `primary`   | Blue bg, white text, no border       | Darker blue                    | Submit task, Approve           |
| `secondary` | White bg, 1.5px black border         | Black bg, white text           | View receipt, Inspect          |
| `ghost`     | No bg, no border, text-2             | text-1                         | Cancel, tertiary actions       |
| `danger`    | White bg, 1.5px solid error border, error text | Error bg, white text | Deny, destructive              |

```css
.btn {
  font-size: 13px;
  font-weight: 700;
  padding: 7px 14px;
  border-radius: 0;
  border: 1.5px solid transparent;
  cursor: pointer;
  transition: background 100ms ease-out, color 100ms ease-out;
}
.btn-secondary {
  background: var(--surface);
  border-color: var(--border-hard);
  color: var(--text-1);
}
.btn-secondary:hover {
  background: var(--border-hard);
  color: var(--surface);
}
```

### Badges

No background fill. Border and text only.

```css
.badge {
  font-family: var(--font-mono);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  padding: 2px 6px;
  border-radius: 0;
  border: 1px solid currentColor;
  background: transparent;
}
```

Status badge colors: `active` → `var(--active)`, `approval` → `var(--approval)`, etc. Border and text are both the status color.

### Inputs

```css
.input {
  border: 1.5px solid var(--border-hard);
  border-radius: 0;
  padding: 8px 12px;
  font-size: 14px;
  font-weight: 400;
  background: var(--surface);
  color: var(--text-1);
}
.input:focus {
  border-color: var(--brand);
  outline: none;
  box-shadow: none;  /* no glow — brutalism uses border, not shadow */
}
```

### Cards

```css
.card {
  background: var(--surface);
  border: 1.5px solid var(--border-hard);
  border-radius: 0;
  padding: 16px 20px;
  box-shadow: none;
}
```

No shadows. Ever. The border is the depth signal.

### Approval card

Full-width, inline (not a modal). Hard 4px amber left border. No amber background.

```css
.approval-card {
  border: 1.5px solid var(--border-hard);
  border-left: 4px solid var(--approval);
  border-radius: 0;
  background: var(--surface);
  padding: 20px 24px;
}
.approval-heading {
  color: var(--approval);
  font-size: 15px;
  font-weight: 700;
}
```

Keyboard: `Enter` = Approve, `Shift+Enter` = Deny. Document inline.

### Replay graph nodes

- Shape: rectangle, `border-radius: 0`
- Border: 1.5px `var(--border-hard)` default with a 4px left state rail
- Background: `var(--surface)` always — no state-based background fill
- State via text: status in `mono-2xs` uppercase, run title in `base/700`, agent/session metadata in `sm`, parent label in `mono-xs`
- Layout: depth-grouped columns, not force-directed. The operator should read orchestration order immediately.
- Running state: 4px left accent in `var(--active)` — do not change background
- Pending state: 4px left accent in `var(--brand)`
- Approval state: 4px left accent in `var(--approval)`
- Completed state: 4px left accent in `var(--success)`
- Failed state: 4px left accent in `var(--error)`

### Segmented controls

Checkboxes are too weak for important state filters. Use segmented controls for mutually exclusive filters like theme and binding state.

```css
.segmented-control {
  display: inline-flex;
  border: 1.5px solid var(--border-hard);
  background: var(--surface);
}
.segmented-option input:checked + span {
  background: var(--border-hard);
  color: var(--surface);
}
```

### Settings rows

Inline edit, no full-page form.

```
[label 140px, 400] [value — mono for keys/paths, 400] [Edit — text link, brand color]
```

### Receipt

Full-width. 4px green left border. No green background.

```css
.receipt {
  border: 1.5px solid var(--border-hard);
  border-left: 4px solid var(--success);
  border-radius: 0;
  background: var(--surface);
  padding: 16px 20px;
}
```

4-column grid: Model | Tokens | Cost | Duration. Values in `mono-md` (13px/500). Labels in `xs` uppercase.

## Accessibility

- **Contrast:** WCAG AA minimum. `text-1` (#1c1917) on `bg` (#f8f7f5) = 14.7:1. `text-2` (#78716c) on surface (#fff) = 5.8:1. Brand blue (#2563eb) on white = 4.7:1 (passes AA).
- **Focus:** `outline: 2px solid var(--brand); outline-offset: 2px` — hard outline, no glow.
- **ARIA:** `<nav>`, `<main>`, `<header>` landmarks required. `aria-live="polite"` on SSE update region. `aria-label` on all icon-only buttons.
- **Keyboard:** All interactive elements reachable by Tab. Approval card: `Enter` = Approve, `Shift+Enter` = Deny — documented inline in the card.
- **No emoji:** Text-only throughout.

## Decisions Log

| Date       | Decision                                   | Rationale                                                                               |
|------------|--------------------------------------------|-----------------------------------------------------------------------------------------|
| 2026-03-24 | Initial proposal: Workshop Minimal         | Based on competitive research (Warp, Zed, Linear)                                       |
| 2026-03-24 | Revised to Warm Brutalism                  | User preference; more opinionated and distinctive                                        |
| 2026-03-24 | Warm neutrals retained (stone family)      | Warm brutalism is unusual in dev tool space; differentiates from cold-brutalist defaults |
| 2026-03-24 | 0px border radius everywhere               | Core brutalist rule; no exceptions for "softness"                                        |
| 2026-03-24 | 1.5px hard borders on all UI chrome        | Structure is visible; border is the depth signal (no shadows)                            |
| 2026-03-24 | No background fills for status states      | Color on border + text only; harder to scan at a glance but structurally honest          |
| 2026-03-24 | Monospace for metadata (not just code)     | Technical authenticity — run IDs, timestamps, cost use JetBrains Mono                   |
| 2026-03-24 | Brand blue deepened to #2563eb             | Blue-600 reads as more tool-like than Tailwind blue-500 (#3b82f6)                        |
| 2026-03-24 | Weight jumps: 400/700 primarily            | Brutalist typography — weight contrast, not weight gradation                             |
| 2026-03-24 | Light mode as default                      | Warm brutalism on light bg; "calm" reads as light, not dark terminal                     |
| 2026-03-24 | No emoji anywhere                          | Operator tool tone; explicitly specified                                                 |
| 2026-03-25 | Direction evolved to Orchestrated Workshop Brutalism | The UI must explain multi-agent collaboration, not just present records                  |
| 2026-03-25 | Added dual-theme system (`System`, `Light`, `Dark`) | Production operators need day/night choice without losing semantic consistency           |
| 2026-03-25 | Delegation graph made first-class on run detail | Multi-agent coordination is the product differentiator and needs its own visual grammar  |
| 2026-03-25 | Replaced binary binding checkbox with segmented state filter | Session binding is not a minor toggle; it is a structural filter                         |
