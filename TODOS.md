# TODOS

This file tracks meaningful deferred work that still matches the current tree.

The reset-era TODO set was collapsed on 2026-03-25. Completed items and references to removed planning docs were dropped so this file reflects only live deferred work.

## P2: `/cancel` control command

**What:** Add a transport-level `/cancel` command for active inbound runs.

**Why:** The control dispatcher already notes this gap in code. Operators can inspect status, but they still cannot cancel an active connector-driven turn from the transport surface.

**How to apply:** Implement after inbound runs own cancellable contexts outside the transport loop, then expose `/cancel` through the existing control command registry.

**Effort:** M → CC+gstack: S
**Priority:** P2
**Depends on:** runtime-owned cancellation path for inbound runs

---

## P2: Operator-selectable and user-defined teams

**What:** Move team composition from repo-managed files and settings toward an operator-visible workflow.

**Why:** The repo ships [teams/default/team.yaml](/Users/canh/Projects/OSS/gistclaw/teams/default/team.yaml) and validates `team_dir`, but the live product still lacks first-class team selection and editing.

**How to apply:** Build on the current `internal/teams/` seam and expose selection or editing through the local control plane without bypassing runtime boundaries.

**Effort:** M → CC+gstack: S
**Priority:** P2
**Depends on:** current team validation seam

---

## P3: Telegram webhook mode

**What:** Add optional webhook mode for Telegram as an alternative to long polling.

**Why:** WhatsApp already uses a webhook path, while Telegram still depends on a continuously running poller.

**How to apply:** Add webhook configuration alongside the existing Telegram connector wiring without changing the session runtime contract.

**Effort:** S → CC+gstack: S
**Priority:** P3
**Depends on:** current Telegram connector path

---

## P3: Broader connector matrix

**What:** Extend external coverage beyond the current web, Telegram, and WhatsApp surfaces.

**Why:** The session and routing kernel is ready for more surfaces, but the live connector matrix is still intentionally narrow.

**How to apply:** Reuse the existing inbound message, route binding, and outbound delivery contracts instead of adding connector-specific runtime shortcuts.

**Effort:** M → CC+gstack: S
**Priority:** P3
**Depends on:** keeping the session kernel as the shared contract
