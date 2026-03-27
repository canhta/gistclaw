# Zalo Personal Production Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `zalo_personal` production-ready for operator use with resilient transport, safe group messaging, and media/file delivery while preserving `gistclaw`'s connector/runtime seams.

**Architecture:** Extend the native Go connector instead of adding a sidecar. Keep protocol-specific retry, group, and upload behavior under `internal/connectors/zalopersonal/protocol/`, keep operator-safe gating in connector/app/config layers, and keep outbound persistence in `outbound_intents` using `metadata_json` for non-text payloads.

**Tech Stack:** Go 1.25+, stdlib `net/http`, stdlib `testing`, SQLite via `modernc.org/sqlite`

---

### Task 1: Document The Approved Design

**Files:**
- Create: `docs/superpowers/specs/2026-03-27-zalo-personal-production-readiness-design.md`
- Create: `docs/superpowers/plans/2026-03-27-zalo-personal-production-readiness.md`

- [ ] **Step 1: Write the design document**

Write the approved design to the spec path.

- [ ] **Step 2: Write the implementation plan**

Write this plan to the plan path.

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/specs/2026-03-27-zalo-personal-production-readiness-design.md docs/superpowers/plans/2026-03-27-zalo-personal-production-readiness.md
git commit -m "docs: plan zalo personal production readiness"
```

### Task 2: Harden Listener Retry And Recovery

**Files:**
- Modify: `internal/connectors/zalopersonal/protocol/listener.go`
- Modify: `internal/connectors/zalopersonal/listener_adapter.go`
- Modify: `internal/connectors/zalopersonal/connector.go`
- Modify: `internal/connectors/zalopersonal/health.go`
- Modify: `internal/connectors/zalopersonal/protocol/models.go`
- Test: `internal/connectors/zalopersonal/protocol/listener_test.go`
- Test: `internal/connectors/zalopersonal/connector_test.go`

- [ ] **Step 1: Write failing listener retry tests**

Add tests for:
- retry on configured transient disconnect
- WS endpoint rotation on configured close code
- duplicate-session emitting a terminal degraded path instead of hot-looping

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/connectors/zalopersonal/protocol ./internal/connectors/zalopersonal -run 'TestListener|TestConnectorStart'`

- [ ] **Step 3: Implement minimal retry state and connector recovery**

Add listener retry state, rotation, stable-connection reset, and duplicate-session handling.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/connectors/zalopersonal/protocol ./internal/connectors/zalopersonal -run 'TestListener|TestConnectorStart'`

- [ ] **Step 5: Commit**

```bash
git add internal/connectors/zalopersonal/protocol/listener.go internal/connectors/zalopersonal/listener_adapter.go internal/connectors/zalopersonal/connector.go internal/connectors/zalopersonal/health.go internal/connectors/zalopersonal/protocol/models.go internal/connectors/zalopersonal/protocol/listener_test.go internal/connectors/zalopersonal/connector_test.go
git commit -m "feat: harden zalo personal listener recovery"
```

### Task 3: Add Group-Safe Messaging

**Files:**
- Modify: `internal/app/config.go`
- Modify: `internal/app/bootstrap.go`
- Modify: `internal/connectors/zalopersonal/inbound.go`
- Modify: `internal/connectors/zalopersonal/listener_adapter.go`
- Modify: `internal/connectors/zalopersonal/protocol/message.go`
- Modify: `internal/connectors/zalopersonal/protocol/listener.go`
- Create: `internal/connectors/zalopersonal/protocol/groups.go`
- Modify: `internal/app/zalo_personal_auth.go`
- Modify: `cmd/gistclaw/auth_zalo_personal.go`
- Test: `internal/app/config_test.go`
- Test: `internal/connectors/zalopersonal/inbound_test.go`
- Test: `internal/connectors/zalopersonal/adapter_test.go`
- Test: `internal/connectors/zalopersonal/protocol/message_test.go`
- Test: `internal/connectors/zalopersonal/protocol/listener_test.go`
- Test: `internal/app/zalo_personal_auth_test.go`
- Test: `cmd/gistclaw/auth_zalo_personal_test.go`

- [ ] **Step 1: Write failing group tests**

Cover:
- group message parsing
- group allowlist filtering
- mention-required gating
- `auth zalo-personal groups`

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app ./internal/connectors/zalopersonal/... ./cmd/gistclaw -run 'TestConfig|TestNormalizeInboundMessage|TestIncomingMessageFromProtocolMessage|TestFetchGroups|TestRun_AuthZaloPersonal'`

- [ ] **Step 3: Implement minimal group-safe behavior**

Add config, parsing, group fetch, and CLI surfaces.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/app ./internal/connectors/zalopersonal/... ./cmd/gistclaw -run 'TestConfig|TestNormalizeInboundMessage|TestIncomingMessageFromProtocolMessage|TestFetchGroups|TestRun_AuthZaloPersonal'`

- [ ] **Step 5: Commit**

```bash
git add internal/app/config.go internal/app/bootstrap.go internal/connectors/zalopersonal/inbound.go internal/connectors/zalopersonal/listener_adapter.go internal/connectors/zalopersonal/protocol/message.go internal/connectors/zalopersonal/protocol/listener.go internal/connectors/zalopersonal/protocol/groups.go internal/app/zalo_personal_auth.go cmd/gistclaw/auth_zalo_personal.go internal/app/config_test.go internal/connectors/zalopersonal/inbound_test.go internal/connectors/zalopersonal/adapter_test.go internal/connectors/zalopersonal/protocol/message_test.go internal/connectors/zalopersonal/protocol/listener_test.go internal/app/zalo_personal_auth_test.go cmd/gistclaw/auth_zalo_personal_test.go
git commit -m "feat: add safe zalo personal group support"
```

### Task 4: Add Media And File Delivery

**Files:**
- Modify: `internal/connectors/zalopersonal/outbound.go`
- Modify: `internal/connectors/zalopersonal/connector.go`
- Modify: `internal/connectors/zalopersonal/protocol/send.go`
- Modify: `internal/connectors/zalopersonal/protocol/message.go`
- Modify: `internal/connectors/zalopersonal/protocol/listener.go`
- Create: `internal/connectors/zalopersonal/protocol/send_image.go`
- Create: `internal/connectors/zalopersonal/protocol/send_file.go`
- Modify: `internal/app/zalo_personal_auth.go`
- Modify: `cmd/gistclaw/auth_zalo_personal.go`
- Test: `internal/connectors/zalopersonal/outbound_test.go`
- Test: `internal/connectors/zalopersonal/connector_test.go`
- Test: `internal/connectors/zalopersonal/protocol/send_test.go`
- Create: `internal/connectors/zalopersonal/protocol/send_image_test.go`
- Create: `internal/connectors/zalopersonal/protocol/send_file_test.go`
- Modify: `cmd/gistclaw/auth_zalo_personal_test.go`

- [ ] **Step 1: Write failing media/file tests**

Cover:
- metadata-backed outbound image/file delivery
- image upload/send flow
- file upload/send flow with WS callback
- inbound non-text visibility
- CLI `send-text`, `send-image`, `send-file`

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/connectors/zalopersonal/... ./internal/app ./cmd/gistclaw -run 'TestOutbound|TestConnectorSendText|TestSendMessage|TestUploadImage|TestUploadFile|TestRun_AuthZaloPersonal'`

- [ ] **Step 3: Implement minimal metadata-backed delivery**

Add protocol helpers, outbound metadata parsing, and operator send commands.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/connectors/zalopersonal/... ./internal/app ./cmd/gistclaw -run 'TestOutbound|TestConnectorSendText|TestSendMessage|TestUploadImage|TestUploadFile|TestRun_AuthZaloPersonal'`

- [ ] **Step 5: Commit**

```bash
git add internal/connectors/zalopersonal/outbound.go internal/connectors/zalopersonal/connector.go internal/connectors/zalopersonal/protocol/send.go internal/connectors/zalopersonal/protocol/message.go internal/connectors/zalopersonal/protocol/listener.go internal/connectors/zalopersonal/protocol/send_image.go internal/connectors/zalopersonal/protocol/send_file.go internal/app/zalo_personal_auth.go cmd/gistclaw/auth_zalo_personal.go internal/connectors/zalopersonal/outbound_test.go internal/connectors/zalopersonal/connector_test.go internal/connectors/zalopersonal/protocol/send_test.go internal/connectors/zalopersonal/protocol/send_image_test.go internal/connectors/zalopersonal/protocol/send_file_test.go cmd/gistclaw/auth_zalo_personal_test.go
git commit -m "feat: add zalo personal media delivery"
```

### Task 5: Production Polish And Verification

**Files:**
- Modify: `README.md`
- Modify: `docs/system.md`
- Modify: `docs/extensions.md`
- Modify: `cmd/gistclaw/doctor.go`
- Modify: `internal/app/commands.go`
- Test: `cmd/gistclaw/doctor_test.go`
- Test: `internal/app/commands_test.go`

- [ ] **Step 1: Write failing health/docs tests where needed**

Cover the new degraded health summaries and doctor output.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app ./cmd/gistclaw -run 'TestConfiguredConnectorHealth|TestDoctor'`

- [ ] **Step 3: Implement polish**

Update health summaries, doctor output, and docs.

- [ ] **Step 4: Run full verification**

Run:

```bash
make lint
go test ./...
go test -coverprofile=/tmp/gistclaw.cover ./...
go tool cover -func=/tmp/gistclaw.cover | tail -n 1
```

- [ ] **Step 5: Commit**

```bash
git add README.md docs/system.md docs/extensions.md cmd/gistclaw/doctor.go internal/app/commands.go cmd/gistclaw/doctor_test.go internal/app/commands_test.go
git commit -m "docs: finalize zalo personal production readiness"
```

- [ ] **Step 6: Push**

```bash
git push origin main
```
