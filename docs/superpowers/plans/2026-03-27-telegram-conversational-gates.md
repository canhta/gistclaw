# Telegram Conversational Gates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Telegram's web-first approval interruption with a Telegram-first conversational gate flow that keeps approvals and blocked-state decisions inside chat.

**Architecture:** Add a runtime-owned conversational gate projection that journals gate lifecycle events and keeps tool approval tickets as the execution authority boundary. Route inbound Telegram messages through a dedicated gate resolver before starting new runs, and deliver gate prompts and resolution summaries through the existing session/outbound intent path so Telegram behaves like normal human chat.

**Tech Stack:** Go 1.25+, SQLite, stdlib `net/http`, existing runtime provider interface, Go `testing`

---

### Task 1: Add Conversational Gate Storage And Projection

**Files:**
- Modify: `internal/store/migrations/001_init.sql`
- Modify: `internal/model/types.go`
- Modify: `internal/conversations/service.go`
- Test: `internal/conversations/service_test.go`

- [ ] **Step 1: Write the failing projection tests**

Add tests covering:
- `conversation_gate_opened` inserts a pending gate row.
- `conversation_gate_resolved` marks the gate resolved.
- `conversation_gate_opened` can carry approval metadata and language hint.

- [ ] **Step 2: Run the projection tests to verify they fail**

Run: `go test ./internal/conversations -run TestConversationStore_ConversationGate -count=1`
Expected: FAIL because the new event kinds and projection table do not exist yet.

- [ ] **Step 3: Add the minimal gate schema and model types**

Create a `conversation_gates` table in `001_init.sql` with:
- `id`
- `conversation_id`
- `run_id`
- `session_id`
- `kind`
- `status`
- `approval_id`
- `title`
- `body`
- `options_json`
- `metadata_json`
- `language_hint`
- `created_at`
- `resolved_at`

Add matching Go model types in `internal/model/types.go`.

- [ ] **Step 4: Project gate lifecycle events**

Teach `ConversationStore.applyProjection` to:
- insert pending gates on `conversation_gate_opened`
- mark them resolved or expired on `conversation_gate_resolved`

- [ ] **Step 5: Run the projection tests to verify they pass**

Run: `go test ./internal/conversations -run TestConversationStore_ConversationGate -count=1`
Expected: PASS

### Task 2: Add Runtime Gate APIs And Approval Bridging

**Files:**
- Modify: `internal/runtime/runs.go`
- Modify: `internal/runtime/conversation_status.go`
- Modify: `internal/runtime/collaboration.go`
- Modify: `internal/model/types.go`
- Test: `internal/runtime/approval_flow_test.go`
- Test: `internal/runtime/collaboration_test.go`

- [ ] **Step 1: Write the failing runtime tests**

Add tests covering:
- approval requests create a conversational gate alongside the approval ticket
- resolving the approval resolves the gate
- conversation status exposes the active gate summary
- gate prompt delivery uses the session/outbound intent path instead of connector event copy

- [ ] **Step 2: Run the runtime tests to verify they fail**

Run: `go test ./internal/runtime -run 'TestRunEngine_ApprovalRequestCreatesConversationalGate|TestResolveApproval_ResolvesConversationalGate|TestInspectConversation_ReturnsActiveGate' -count=1`
Expected: FAIL because runtime does not create or expose conversational gates yet.

- [ ] **Step 3: Add gate event helpers and projection writes**

Implement helpers in runtime to append:
- `conversation_gate_opened`
- `conversation_gate_resolved`

When a tool approval is requested:
- keep writing the approval ticket
- append `approval_requested`
- append `conversation_gate_opened`
- append a session assistant message for the gate prompt
- queue an outbound intent for the same prompt

- [ ] **Step 4: Resolve gates when approvals resolve**

When `ResolveApproval` succeeds:
- append `conversation_gate_resolved`
- append a short assistant summary message
- queue that summary outbound

- [ ] **Step 5: Extend conversation status**

Expose:
- `ActiveGate`
- `PendingGateCount`

so connectors and `/status` can describe the waiting state natively.

- [ ] **Step 6: Run the runtime tests to verify they pass**

Run: `go test ./internal/runtime -run 'TestRunEngine_ApprovalRequestCreatesConversationalGate|TestResolveApproval_ResolvesConversationalGate|TestInspectConversation_ReturnsActiveGate' -count=1`
Expected: PASS

### Task 3: Add Dedicated Gate Resolver With Natural-Language And Command Lanes

**Files:**
- Modify: `internal/runtime/collaboration.go`
- Modify: `internal/runtime/runs.go`
- Create: `internal/runtime/gates.go`
- Test: `internal/runtime/collaboration_test.go`

- [ ] **Step 1: Write the failing gate resolver tests**

Add tests covering:
- active gate intercepts Telegram inbound before a new run starts
- `/approve <id> allow-once|deny` resolves deterministically
- natural language like `yes, go ahead` resolves through the provider
- ambiguous or unrelated replies produce a clarification message and do not start a new run
- the resolver records the inbound user message in the session mailbox

- [ ] **Step 2: Run the gate resolver tests to verify they fail**

Run: `go test ./internal/runtime -run 'TestReceiveInboundMessage_ActiveGateInterceptsReply|TestReceiveInboundMessage_ApproveCommandResolvesGate|TestReceiveInboundMessage_AmbiguousGateReplyRequestsClarification' -count=1`
Expected: FAIL because inbound messages always start runs today.

- [ ] **Step 3: Add the dedicated gate resolver**

Implement a small runtime gate resolver that:
- loads the active gate for the conversation
- first checks deterministic command syntax
- otherwise asks the provider for a structured JSON decision over only the gate context plus latest user reply
- requires high confidence for risky approval
- emits clarification instead of guessing

- [ ] **Step 4: Preserve multilingual hints**

Pass through:
- a soft `language_hint`
- the latest reply text
- recent gate context

Store language hints on inbound provenance and gate metadata so the resolver can mirror the user's language when clear.

- [ ] **Step 5: Run the gate resolver tests to verify they pass**

Run: `go test ./internal/runtime -run 'TestReceiveInboundMessage_ActiveGateInterceptsReply|TestReceiveInboundMessage_ApproveCommandResolvesGate|TestReceiveInboundMessage_AmbiguousGateReplyRequestsClarification' -count=1`
Expected: PASS

### Task 4: Refactor Telegram Inbound Routing Around Runtime Gates

**Files:**
- Modify: `internal/connectors/telegram/connector.go`
- Modify: `internal/connectors/telegram/dispatch.go`
- Modify: `internal/connectors/telegram/inbound.go`
- Test: `internal/connectors/telegram/control_test.go`
- Test: `internal/connectors/telegram/dispatch_test.go`

- [ ] **Step 1: Write the failing Telegram routing tests**

Add tests covering:
- `/approve` is treated as a gate response, not a new task
- natural-language gate replies are consumed by runtime and do not start new runs
- normal chat still starts a new run when no active gate exists
- Telegram language code is captured into inbound metadata

- [ ] **Step 2: Run the Telegram routing tests to verify they fail**

Run: `go test ./internal/connectors/telegram -run 'TestConnector_HandleEnvelopeRoutesGateReplyBeforeRuntimeTask|TestInboundDispatcher_DispatchesTelegramLanguageHint' -count=1`
Expected: FAIL because the connector does not know about gates yet.

- [ ] **Step 3: Extend the connector runtime interface**

Add a runtime call for conversational gate handling, then update Telegram routing so the order is:
1. native help/status/reset
2. runtime gate handler
3. normal inbound task

- [ ] **Step 4: Capture Telegram language metadata**

Add Telegram sender language handling to normalized envelopes so runtime gets a soft language hint.

- [ ] **Step 5: Run the Telegram routing tests to verify they pass**

Run: `go test ./internal/connectors/telegram -run 'TestConnector_HandleEnvelopeRoutesGateReplyBeforeRuntimeTask|TestInboundDispatcher_DispatchesTelegramLanguageHint' -count=1`
Expected: PASS

### Task 5: Replace Web-First Approval Copy With Telegram-Native Messaging

**Files:**
- Modify: `internal/connectors/control/dispatcher.go`
- Modify: `internal/connectors/telegram/outbound.go`
- Modify: `internal/app/run_events.go`
- Test: `internal/connectors/telegram/outbound_test.go`
- Test: `internal/connectors/telegram/control_test.go`
- Test: `internal/app/run_events_test.go`

- [ ] **Step 1: Write the failing delivery tests**

Add tests covering:
- Telegram no longer builds approval copy that says `Review it in the web UI.`
- `/status` reports pending gate state in chat-native wording
- connector route fanout no longer sends duplicate approval prompts when runtime already queued the gate prompt

- [ ] **Step 2: Run the delivery tests to verify they fail**

Run: `go test ./internal/connectors/telegram ./internal/app -run 'TestOutbound_ApprovalRequestedEventDoesNotRedirectToWeb|TestConnector_HandleEnvelopeRoutesStatusToNativeReply|TestConnectorRouteNotifier_DoesNotForwardApprovalRequested' -count=1`
Expected: FAIL because approval text and forwarding are still web-first.

- [ ] **Step 3: Remove connector-side approval copy dependency**

Keep event fanout for streaming deltas, but stop relying on connector-built approval messages for Telegram-native approval UX.

- [ ] **Step 4: Update native status copy**

Make `/status` describe pending gate state in Telegram-first language.

- [ ] **Step 5: Run the delivery tests to verify they pass**

Run: `go test ./internal/connectors/telegram ./internal/app -run 'TestOutbound_ApprovalRequestedEventDoesNotRedirectToWeb|TestConnector_HandleEnvelopeRoutesStatusToNativeReply|TestConnectorRouteNotifier_DoesNotForwardApprovalRequested' -count=1`
Expected: PASS

### Task 6: Verify The Full Refactor

**Files:**
- Modify: `docs/system.md`
- Modify: `docs/roadmap.md`

- [ ] **Step 1: Run targeted packages**

Run: `go test ./internal/conversations ./internal/runtime ./internal/connectors/telegram ./internal/app -count=1`
Expected: PASS

- [ ] **Step 2: Run the full suite with coverage**

Run: `go test -cover ./...`
Expected: PASS with coverage at or above project policy.

- [ ] **Step 3: Update docs for the shipped behavior**

Document that Telegram approvals and blocked-state gates stay native to chat, with command fallback and natural-language resolution.

- [ ] **Step 4: Re-run the touched package tests**

Run: `go test ./internal/runtime ./internal/connectors/telegram ./internal/app -count=1`
Expected: PASS
