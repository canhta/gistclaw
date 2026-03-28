# Zalo Conversation State Capability Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans or superpowers:subagent-driven-development. Keep the core runtime generic. Do not add Zalo-only tools to the run engine.

**Goal:** Extend the new inbox capability work into a complete conversation-state control surface that improves operator UX across connectors, starting with Zalo Personal and designed so Telegram, WhatsApp, and future connectors can implement the same seams.

**What We Learned From Upstream**

Upstream `zca-js` exposes real conversation-state primitives beyond send and contacts:
- unread mark read/write via `getUnreadMark`, `addUnreadMark`, `removeUnreadMark`
- pin state read/write via `getPinConversations`, `setPinnedConversations`
- hidden thread read/write via `getHiddenConversations`, `setHiddenConversations`
- archive state read/write via `getArchivedChatList`, `updateArchivedChatList`
- hidden conversation PIN maintenance via `updateHiddenConversPin`, `resetHiddenConversPin`
- typing emission via `sendTypingEvent`
- chat auto-delete state via `getAutoDeleteChat`, `updateAutoDeleteChat`
- conversation delete via `deleteChat`

These should not become one-off Zalo commands. They should become generic runtime capabilities where the product semantics are portable.

## Capability Direction

Ship the conversation UX as a small generic capability family:

- `connector_inbox_list`
  Already shipped. Lists recent threads and unread state.
- `connector_inbox_mark`
  Mark a thread read or unread.
- `connector_inbox_pin`
  Pin or unpin a thread.
- `connector_inbox_archive`
  Archive or unarchive a thread.
- `connector_inbox_visibility`
  Hide or unhide a thread.
- `connector_presence_emit`
  Optional typing/presence actions for connectors that support them.
- `connector_retention_update`
  Optional message retention or auto-delete policy update.
- `connector_thread_delete`
  High-risk destructive delete with explicit approval.

The runtime should continue to prefer direct execution for these bounded tasks, with approval only on state-changing or destructive operations.

## Architecture Rules

- Keep the capability seam generic under `internal/runtime/capabilities/`.
- Keep tool registration generic under `internal/tools/`.
- Keep connector-specific protocol under `internal/connectors/<connector>/protocol/`.
- Keep read/write state projection under connector-owned storage helpers such as `internal/connectors/threadstate/`.
- Do not hardcode Zalo-specific tool names or connector IDs in planner, runtime, or conversations packages.
- Use connector metadata and capability adapters to discover support.

## Recommended Execution Order

### Task 1: Generic Conversation-State Capability Seams

- Add generic request/result interfaces for:
  - `InboxMark`
  - `InboxPin`
  - `InboxArchive`
  - `InboxVisibility`
- Register generic tools with correct intents and approval policy.
- Extend the recommendation engine so bounded conversation-state requests stay `direct`.

### Task 2: Zalo Read/Unread Control

- Implement read/unread mutation in `internal/connectors/zalopersonal/protocol/`.
- Expose it through the generic `connector_inbox_mark` seam.
- Add runtime tests proving requests like `đánh dấu cuộc chat với Mẹ là đã đọc` do not spawn a child run.

### Task 3: Zalo Pin/Archive/Hide Control

- Implement pin/unpin, archive/unarchive, and hide/unhide through the generic seams.
- Reflect the new state back into `connector_inbox_list`.
- Keep these tools direct-first and approval-light because they are reversible state changes.

### Task 4: Typing And Presence

- Add optional `connector_presence_emit` support for typing when it improves cross-platform send flows.
- Keep it connector-optional and never required for normal message delivery.

### Task 5: Risky Conversation Controls

- Add `connector_thread_delete` only after the reversible state tools are stable.
- Mark it as high-risk and approval-required.

### Task 6: UX And Docs

- Teach the front assistant examples for:
  - unread checks
  - recent inbox lookup
  - mark read/unread
  - pin/unpin
  - archive/unarchive
  - hide/unhide
- Update `docs/system.md` when each generic capability ships.

## Verification Bar

Every slice must include:
- targeted connector tests
- runtime direct-vs-delegate tests
- `go test ./...`
- `go vet ./...`
- `.bin/golangci-lint run`
- `go test -coverprofile=/tmp/gistclaw.cover ./...`
- `go tool cover -func=/tmp/gistclaw.cover | tail -n 1`

Do not ship any conversation-state tool that requires prompt hacks to discover or route.
