# Zalo Personal Production Readiness Design

**Goal:** Make `zalo_personal` operator-ready for real use in `gistclaw` with resilient DM transport, safe group support, media/file delivery, and clear operator recovery paths, while preserving the existing connector/runtime boundaries.

**Context:** `gistclaw` already has a native Go `zalo_personal` connector with QR auth, SQLite-backed credentials, DM send/receive, a basic WebSocket listener, and CLI auth flows. It is functional but still too coarse in its retry behavior, too narrow in its message surface, and too thin in its operator tooling for a real operator-facing deployment.

## Scope

This production-readiness pass includes:

- listener-level retry using server-provided socket retry settings
- WebSocket endpoint rotation where the protocol advertises rotatable close codes
- slower, explicit duplicate-session handling with degraded health reporting
- group list fetching and group message support
- group safety controls with restrictive defaults
- image send and file send using the existing connector/runtime seams
- inbound non-text visibility as text plus metadata instead of silent drops
- operator CLI surfaces for contacts, groups, and direct sends
- updated health summaries and documentation

This pass does not include:

- official Zalo OA support
- browser-based auth UX beyond the current QR file flow
- runtime-wide binary attachment ingestion for model context
- automatic policy generation for groups

## Architectural Direction

Keep all reverse-engineered protocol logic under `internal/connectors/zalopersonal/protocol/` and keep `gistclaw` product decisions in the connector, app, and CLI layers.

Borrow protocol truths from `goclaw`, not its product shape:

- retry timing and endpoint rotation from the WebSocket listener
- group list fetch flow
- group message parsing
- image upload/send flow
- file upload/send flow plus WebSocket completion callback handling

Do not import `goclaw`'s gateway method layer or its channel model. `gistclaw` already has the correct seams:

- connector interface in `internal/model/connector.go`
- inbound routing in `internal/runtime/collaboration.go`
- persisted outbound queue in `outbound_intents`
- operator auth flows in `internal/app` and `cmd/gistclaw`

## Connector Boundary

`zalo_personal` remains a single native Go connector with three responsibilities:

1. Authenticate and recover sessions.
2. Normalize inbound Zalo events into `gistclaw` envelopes and inbound commands.
3. Deliver outbound text/media/file messages through the existing queue.

The connector must not bypass runtime state or add a parallel outbound transport store.

## Transport Resilience

The protocol listener should own transient WebSocket recovery. The existing `Session.Settings.Features.Socket` structure already exposes:

- retry schedules by close code
- close codes that should be retried
- close codes that should rotate endpoints
- ping intervals

The listener should:

- keep retry counters per close code
- reset retry counters after a stable connection window
- rotate among `zpw_ws` endpoints when the close code is configured as rotatable
- emit a terminal closed/disconnected signal only after exhausting listener-level retries

The connector loop should:

- perform full re-auth only when listener-level recovery gives up
- treat duplicate-session close codes specially with a slower reconnect backoff
- expose degraded health summaries for unauthenticated, retrying, duplicate-session, and auth-failed states

## Group Support

Group support should be explicit and restrictive.

Add `zalo_personal` config controls for:

- `groups.enabled`
- `groups.allowlist`
- `groups.reply_mode`

Recommended semantics:

- `groups.enabled = false` by default
- `groups.allowlist = []` by default
- `groups.reply_mode = "mention_required"` by default when groups are enabled

Inbound group messages should only be accepted when:

- group support is enabled
- the group is allowlisted
- the reply mode admits the message

Accepted modes:

- `mention_required`
- `open`

This keeps the shipped surface safe for operators using a personal account.

## Inbound Message Shape

DM text remains the primary input surface.

For inbound non-text messages:

- do not silently drop them
- convert them into readable text placeholders
- preserve attachment metadata in envelope metadata where useful

For group messages:

- normalize the conversation identity to the group ID
- preserve sender identity
- preserve whether the message mentioned the account or `@all`

This pass should not try to plumb binary attachments into provider context. That is a separate runtime-wide project.

## Outbound Media and Files

Keep `outbound_intents` as the only queue. Use the existing `metadata_json` column to describe non-text delivery.

Suggested metadata shape:

```json
{
  "kind": "image",
  "path": "/abs/path/to/file.png",
  "caption": "optional caption"
}
```

or:

```json
{
  "kind": "file",
  "path": "/abs/path/to/file.pdf"
}
```

Delivery rules:

- text messages keep using `message_text`
- image sends upload to the file service, then send the image message
- file sends upload asynchronously, wait for the WebSocket completion callback, then send the file message
- delivery failures still go through the existing delivery-failed event flow

## Operator Surface

Add operator CLI commands under `gistclaw auth zalo-personal` for:

- `contacts`
- `groups`
- `send-text`
- `send-image`
- `send-file`

These commands are for authenticated operators and dogfooding. They should reuse stored credentials and the same protocol code as the connector.

## Health and Recovery

Health summaries should communicate actionable states:

- `not authenticated`
- `connected`
- `retrying websocket connection`
- `duplicate session; waiting to retry`
- `authentication failed: ...`
- `file upload callback timed out`

`doctor` should surface these without inventing a new status plane.

## Testing Strategy

This work must stay TDD-first:

- write failing tests for each new behavior
- verify the failure reason
- implement the smallest passing change
- keep the repo-wide coverage floor at or above 70%

Test focus:

- listener retry and endpoint rotation
- duplicate-session behavior
- group parsing and gating
- image and file protocol flows
- outbound metadata persistence and delivery
- CLI command behavior
- health summaries and doctor output

## Rollout

The implementation should land in four slices:

1. transport hardening
2. group-safe messaging
3. media/file delivery
4. operator/docs polish

Each slice should be independently green before moving to the next.
