# Zalo Personal Connector Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a native Go `zalo_personal` connector to GistClaw with SQLite-backed credentials, QR login from the CLI, DM send/receive, connector health, and the same runtime safety rules already enforced for Telegram and WhatsApp.

**Architecture:** Keep all reverse-engineered Zalo logic isolated under `internal/connectors/zalopersonal/`, with the brittle HTTP, crypto, cookie, and WebSocket behavior living under `internal/connectors/zalopersonal/protocol/`. Store the authenticated session in SQLite settings instead of files, start the connector whenever `zalo_personal.enabled` is true, and let the connector poll its own stored credentials so a successful login can bring it online without restarting the daemon.

**Tech Stack:** Go 1.25+, SQLite via `modernc.org/sqlite`, stdlib `net/http`, existing runtime/session/outbound-intent model, Go `testing`.

---

## Non-Negotiables

- Ship only `zalo_personal` in this plan. Do not bundle OA/bot support into the same connector.
- Keep credentials in SQLite-backed settings. Do not write cookie or session files under the repo, home directory, or temp directory.
- Treat `zalo_personal` as a remote connector for authority enforcement.
- Do not let the connector write directly to runtime tables outside the existing journal/runtime APIs.
- Start with DM support only. Group handling, mentions, and pairing belong to a later follow-up once the transport is stable.
- Keep the daemon alive when credentials are missing or expired. Missing auth is a degraded connector state, not a crash loop.
- Stay on `main`. No worktrees, no feature branches.

## File Map

**Create**
- `internal/connectors/zalopersonal/connector.go`
- `internal/connectors/zalopersonal/inbound.go`
- `internal/connectors/zalopersonal/outbound.go`
- `internal/connectors/zalopersonal/auth.go`
- `internal/connectors/zalopersonal/settings.go`
- `internal/connectors/zalopersonal/health.go`
- `internal/connectors/zalopersonal/protocol/auth.go`
- `internal/connectors/zalopersonal/protocol/client.go`
- `internal/connectors/zalopersonal/protocol/session.go`
- `internal/connectors/zalopersonal/protocol/ws_client.go`
- `internal/connectors/zalopersonal/protocol/models.go`
- `internal/connectors/zalopersonal/protocol/auth_test.go`
- `internal/connectors/zalopersonal/protocol/models_test.go`
- `internal/connectors/zalopersonal/settings_test.go`
- `internal/connectors/zalopersonal/inbound_test.go`
- `internal/connectors/zalopersonal/outbound_test.go`
- `internal/connectors/zalopersonal/connector_test.go`
- `internal/app/zalo_personal_auth.go`
- `internal/app/zalo_personal_auth_test.go`
- `cmd/gistclaw/auth_zalo_personal.go`
- `cmd/gistclaw/auth_zalo_personal_test.go`

**Modify**
- `internal/app/config.go`
- `internal/app/bootstrap.go`
- `internal/app/bootstrap_test.go`
- `internal/app/commands.go`
- `internal/app/commands_test.go`
- `internal/runtime/authority.go`
- `internal/runtime/collaboration_test.go`
- `internal/security/audit.go`
- `internal/security/audit_test.go`
- `cmd/gistclaw/main.go`
- `cmd/gistclaw/auth.go`
- `cmd/gistclaw/doctor.go`
- `cmd/gistclaw/doctor_test.go`
- `internal/web/routes_runs.go`
- `README.md`
- `docs/system.md`
- `docs/extensions.md`

**Do Not Modify In This Plan**
- `internal/store/migrations/001_init.sql`

Use the existing `settings` table for:
- `zalo_personal_credentials`
- `zalo_personal_account_id`
- `zalo_personal_display_name`

Use config only for runtime wiring:

```yaml
zalo_personal:
  enabled: true
  agent_id: assistant
```

### Task 1: Add config and connector wiring

**Files:**
- Modify: `internal/app/config.go`
- Modify: `internal/app/bootstrap.go`
- Modify: `internal/app/bootstrap_test.go`
- Modify: `internal/app/commands.go`
- Modify: `internal/app/commands_test.go`

- [ ] **Step 1: Write failing config and bootstrap tests**

Add coverage for:

```go
type ZaloPersonalConfig struct {
	Enabled bool   `yaml:"enabled"`
	AgentID string `yaml:"agent_id"`
}
```

Assert that:
- `agent_id` defaults to `assistant`
- `buildConnectors` includes `zalo_personal` when enabled
- configured connector health includes `zalo_personal` when enabled

- [ ] **Step 2: Run the app tests to verify they fail**

Run: `go test ./internal/app -run 'TestConfig|TestBootstrap|TestCommands'`
Expected: FAIL because config and bootstrap do not know about `zalo_personal`.

- [ ] **Step 3: Add the config block and bootstrap wiring**

Implement:

```go
type Config struct {
	// ...
	ZaloPersonal ZaloPersonalConfig `yaml:"zalo_personal"`
}

if c.ZaloPersonal.AgentID == "" {
	c.ZaloPersonal.AgentID = "assistant"
}
```

Wire `buildConnectors` to instantiate a `zalopersonal.NewConnector(...)` when `cfg.ZaloPersonal.Enabled` is true.

- [ ] **Step 4: Keep connector health cold-start friendly**

`ConfiguredConnectorHealth` must list the connector even before a live daemon has authenticated it, so the operator sees `zalo_personal` as configured rather than invisible.

- [ ] **Step 5: Re-run the app tests until they pass**

Run: `go test ./internal/app -run 'TestConfig|TestBootstrap|TestCommands'`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/app/config.go internal/app/bootstrap.go internal/app/bootstrap_test.go internal/app/commands.go internal/app/commands_test.go
git commit -m "feat: wire zalo personal connector config"
```

### Task 2: Add SQLite-backed settings and QR auth orchestration

**Files:**
- Create: `internal/connectors/zalopersonal/settings.go`
- Create: `internal/connectors/zalopersonal/auth.go`
- Create: `internal/connectors/zalopersonal/settings_test.go`
- Create: `internal/app/zalo_personal_auth.go`
- Create: `internal/app/zalo_personal_auth_test.go`

- [ ] **Step 1: Write failing tests for credential round-tripping and logout**

Add tests for:
- saving credentials as JSON in `settings`
- loading empty vs populated auth state
- clearing credentials on logout
- polling fresh settings after a login completes

- [ ] **Step 2: Run the targeted tests to verify they fail**

Run: `go test ./internal/connectors/zalopersonal ./internal/app -run 'TestSettings|TestZaloPersonalAuth'`
Expected: FAIL because the storage and auth helpers do not exist.

- [ ] **Step 3: Create a focused settings helper**

Implement a small auth-state store:

```go
type StoredCredentials struct {
	AccountID   string `json:"account_id"`
	DisplayName string `json:"display_name"`
	IMEI        string `json:"imei"`
	Cookie      string `json:"cookie"`
	UserAgent   string `json:"user_agent"`
	Language    string `json:"language"`
}
```

Add helpers:
- `LoadStoredCredentials(ctx, db)`
- `SaveStoredCredentials(ctx, db, creds)`
- `ClearStoredCredentials(ctx, db)`

Do not scatter raw setting keys through the rest of the codebase.

- [ ] **Step 4: Add app-facing auth methods**

Implement methods in `internal/app/zalo_personal_auth.go` that:
- start QR login
- stream the QR PNG bytes to a callback
- save credentials on success
- clear credentials on logout

The app layer should own DB access. The CLI should not update settings with ad hoc SQL.

- [ ] **Step 5: Re-run the targeted tests until they pass**

Run: `go test ./internal/connectors/zalopersonal ./internal/app -run 'TestSettings|TestZaloPersonalAuth'`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/connectors/zalopersonal/settings.go internal/connectors/zalopersonal/auth.go internal/connectors/zalopersonal/settings_test.go internal/app/zalo_personal_auth.go internal/app/zalo_personal_auth_test.go
git commit -m "feat: add zalo personal sqlite auth storage"
```

### Task 3: Port the protocol core under an isolated package

**Files:**
- Create: `internal/connectors/zalopersonal/protocol/auth.go`
- Create: `internal/connectors/zalopersonal/protocol/client.go`
- Create: `internal/connectors/zalopersonal/protocol/session.go`
- Create: `internal/connectors/zalopersonal/protocol/ws_client.go`
- Create: `internal/connectors/zalopersonal/protocol/models.go`
- Create: `internal/connectors/zalopersonal/protocol/auth_test.go`
- Create: `internal/connectors/zalopersonal/protocol/models_test.go`

- [ ] **Step 1: Write failing protocol tests for the deterministic pieces**

Cover:
- JSON decoding for QR/login payloads
- credential serialization
- request-signing helpers
- cookie/header shaping for the WebSocket client

Keep network-dependent flows behind small seams so tests can stub HTTP and WS clients.

- [ ] **Step 2: Run the protocol tests to verify they fail**

Run: `go test ./internal/connectors/zalopersonal/protocol -run 'TestAuth|TestModels'`
Expected: FAIL because the protocol package does not exist.

- [ ] **Step 3: Port only the transport-critical pieces from `goclaw`**

Implement:
- QR login returning PNG bytes through a callback
- credential-based re-login
- signed HTTP requests
- WebSocket client setup with explicit cookie handling

Do not port group policy, business logic, or operator UX into the protocol package.

- [ ] **Step 4: Define the protocol boundary explicitly**

The package API should stay narrow:

```go
type Credentials struct { /* stored session data */ }

func LoginQR(ctx context.Context, qrCallback func([]byte)) (*Credentials, error)
func LoginWithCredentials(ctx context.Context, creds Credentials) (*Session, error)
```

Everything above this boundary should depend on these functions, not on raw reverse-engineered endpoints.

- [ ] **Step 5: Re-run the protocol tests until they pass**

Run: `go test ./internal/connectors/zalopersonal/protocol -run 'TestAuth|TestModels'`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/connectors/zalopersonal/protocol/auth.go internal/connectors/zalopersonal/protocol/client.go internal/connectors/zalopersonal/protocol/session.go internal/connectors/zalopersonal/protocol/ws_client.go internal/connectors/zalopersonal/protocol/models.go internal/connectors/zalopersonal/protocol/auth_test.go internal/connectors/zalopersonal/protocol/models_test.go
git commit -m "feat: add zalo personal protocol core"
```

### Task 4: Add outbound delivery with the existing intent queue

**Files:**
- Create: `internal/connectors/zalopersonal/outbound.go`
- Create: `internal/connectors/zalopersonal/outbound_test.go`

- [ ] **Step 1: Write failing outbound tests**

Cover:
- allowed replay event kinds
- intent enqueue and dedupe
- retrying -> delivered transitions
- terminal failure projection through the shared delivery helper

- [ ] **Step 2: Run the outbound tests to verify they fail**

Run: `go test ./internal/connectors/zalopersonal -run 'TestOutbound'`
Expected: FAIL because the outbound dispatcher does not exist.

- [ ] **Step 3: Mirror the Telegram/WhatsApp dispatcher shape**

Implement:

```go
type OutboundDispatcher struct {
	connectorID string
	db          *store.DB
	cs          *conversations.ConversationStore
	maxAttempts int
	retryDelay  time.Duration
	health      *HealthState
}
```

Use `outbound_intents` exactly like the existing connectors. Do not invent a second outbound queue.

- [ ] **Step 4: Keep MVP delivery text-only**

Implement `deliverOnce` for text messages only. Record a clear terminal error for unsupported attachment payloads and leave media support for a later plan.

- [ ] **Step 5: Re-run the outbound tests until they pass**

Run: `go test ./internal/connectors/zalopersonal -run 'TestOutbound'`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/connectors/zalopersonal/outbound.go internal/connectors/zalopersonal/outbound_test.go
git commit -m "feat: add zalo personal outbound delivery"
```

### Task 5: Add inbound normalization and the connector loop

**Files:**
- Create: `internal/connectors/zalopersonal/inbound.go`
- Create: `internal/connectors/zalopersonal/connector.go`
- Create: `internal/connectors/zalopersonal/health.go`
- Create: `internal/connectors/zalopersonal/inbound_test.go`
- Create: `internal/connectors/zalopersonal/connector_test.go`

- [ ] **Step 1: Write failing inbound and connector tests**

Cover:
- DM event normalization into `model.Envelope`
- conversation key mapping
- duplicate source message handling via runtime receipts
- missing credentials => degraded health without crashing
- saved credentials => listener starts and reconnects on failure

- [ ] **Step 2: Run the connector tests to verify they fail**

Run: `go test ./internal/connectors/zalopersonal -run 'TestInbound|TestConnector'`
Expected: FAIL because the connector loop does not exist.

- [ ] **Step 3: Normalize Zalo inbound events into the existing runtime contract**

Map Zalo messages into:

```go
model.Envelope{
	ConnectorID:    "zalo_personal",
	AccountID:      storedAccountID,
	ActorID:        senderID,
	ConversationID: peerID,
	ThreadID:       "main",
	MessageID:      sourceMessageID,
	Text:           strings.TrimSpace(body),
}
```

For MVP, ignore non-text and non-DM events cleanly instead of half-supporting them.

- [ ] **Step 4: Build a long-lived connector loop**

`Start(ctx)` should:
- load saved credentials from SQLite
- mark degraded if none exist
- log in with saved credentials when present
- start the listener
- back off and retry on disconnect
- keep polling storage so a later CLI login can bring the connector online without restarting `gistclaw serve`

Expose `ConnectorHealthSnapshot()` with summaries like:
- `not authenticated`
- `connected`
- `listener disconnected: ...`

- [ ] **Step 5: Re-run the connector tests until they pass**

Run: `go test ./internal/connectors/zalopersonal -run 'TestInbound|TestConnector'`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/connectors/zalopersonal/inbound.go internal/connectors/zalopersonal/connector.go internal/connectors/zalopersonal/health.go internal/connectors/zalopersonal/inbound_test.go internal/connectors/zalopersonal/connector_test.go
git commit -m "feat: add zalo personal inbound connector"
```

### Task 6: Enforce runtime safety and operator visibility

**Files:**
- Modify: `internal/runtime/authority.go`
- Modify: `internal/runtime/collaboration_test.go`
- Modify: `internal/security/audit.go`
- Modify: `internal/security/audit_test.go`
- Modify: `cmd/gistclaw/doctor.go`
- Modify: `cmd/gistclaw/doctor_test.go`
- Modify: `internal/web/routes_runs.go`

- [ ] **Step 1: Write failing safety and operator-surface tests**

Add coverage for:
- `zalo_personal` rejected under `auto_approve + elevated`
- security audit warning when unofficial Zalo Personal is enabled
- doctor output listing the connector
- run UI label humanized as `Zalo Personal`

- [ ] **Step 2: Run the safety and operator tests to verify they fail**

Run: `go test ./internal/runtime ./internal/security ./cmd/gistclaw ./internal/web -run 'TestReceiveInboundMessage|TestAudit|TestDoctor|TestServer'`
Expected: FAIL because `zalo_personal` is not treated as a remote connector and has no operator-facing warnings.

- [ ] **Step 3: Add the safety enforcement**

Update:

```go
func isRemoteConnectorID(connectorID string) bool {
	switch strings.ToLower(strings.TrimSpace(connectorID)) {
	case "telegram", "whatsapp", "zalo_personal":
		return true
	}
	return false
}
```

- [ ] **Step 4: Add explicit unofficial-connector warnings**

`security audit` should emit a warning, not a failure, when `zalo_personal.enabled` is true:
- title: `Unofficial Zalo Personal connector enabled`
- detail: explain that the integration relies on reverse-engineered personal-account behavior and may be restricted by Zalo

`doctor` should show the configured connector and reuse persisted connector health when available.

- [ ] **Step 5: Re-run the safety and operator tests until they pass**

Run: `go test ./internal/runtime ./internal/security ./cmd/gistclaw ./internal/web -run 'TestReceiveInboundMessage|TestAudit|TestDoctor|TestServer'`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/runtime/authority.go internal/runtime/collaboration_test.go internal/security/audit.go internal/security/audit_test.go cmd/gistclaw/doctor.go cmd/gistclaw/doctor_test.go internal/web/routes_runs.go
git commit -m "feat: add zalo personal safety and operator visibility"
```

### Task 7: Add CLI login/logout and ship docs

**Files:**
- Create: `cmd/gistclaw/auth_zalo_personal.go`
- Create: `cmd/gistclaw/auth_zalo_personal_test.go`
- Modify: `cmd/gistclaw/main.go`
- Modify: `cmd/gistclaw/auth.go`
- Modify: `README.md`
- Modify: `docs/system.md`
- Modify: `docs/extensions.md`

- [ ] **Step 1: Write failing CLI tests**

Cover:
- `gistclaw auth zalo-personal login`
- `gistclaw auth zalo-personal logout`
- usage text for invalid subcommands
- login writes a PNG file under the operator-owned state directory and prints its path

- [ ] **Step 2: Run the CLI tests to verify they fail**

Run: `go test ./cmd/gistclaw -run 'TestAuth|TestMain'`
Expected: FAIL because the Zalo Personal auth subcommands do not exist.

- [ ] **Step 3: Add the CLI flow**

Support:

```text
gistclaw auth zalo-personal login
gistclaw auth zalo-personal logout
```

`login` should:
- start QR login through the app layer
- save the QR PNG under the operator-owned state directory
- print `Scan QR image: <path>`
- block until login succeeds or times out

`logout` should clear the stored credentials and print `zalo personal credentials cleared`

- [ ] **Step 4: Document the shipped scope honestly**

Update docs to say:
- live external surfaces now include Telegram DM, WhatsApp, and optional Zalo Personal DM
- Zalo Personal is unofficial
- auth is CLI-driven for now
- group support is not part of this ship

Do not imply official Zalo API support or group coverage.

- [ ] **Step 5: Run the full targeted verification**

Run: `go test ./internal/connectors/zalopersonal ./internal/app ./internal/runtime ./internal/security ./cmd/gistclaw ./internal/web`
Expected: PASS

Run: `go test -cover ./...`
Expected: PASS with coverage at or above `70%`

- [ ] **Step 6: Commit**

```bash
git add cmd/gistclaw/auth_zalo_personal.go cmd/gistclaw/auth_zalo_personal_test.go cmd/gistclaw/main.go cmd/gistclaw/auth.go README.md docs/system.md docs/extensions.md
git commit -m "feat: add zalo personal auth flow and docs"
```

## Verification Checklist

- `go test ./internal/connectors/zalopersonal/...`
- `go test ./internal/app ./internal/runtime ./internal/security ./cmd/gistclaw ./internal/web`
- `go test ./...`
- `go test -cover ./...`
- `go vet ./...`

## Follow-Up Work Explicitly Deferred

- group messages, mentions, allowlist/pairing policy, and history replay
- media send and receive
- browser-native QR login page
- a separate official `zalo_oa` connector
- multi-account Zalo Personal support

Add TODOs at the relevant call sites instead of implementing these in the MVP.
