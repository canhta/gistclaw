# Automatic Presence Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add automatic runtime-managed typing/presence for user-facing remote runs, starting with Zalo Personal, without exposing typing as a normal assistant tool.

**Architecture:** Add a generic presence adapter seam to the capability registry, then build a runtime-owned presence controller/manager that starts and stops automatically from run lifecycle events. Keep all protocol details inside connector adapters so the runtime remains connector-agnostic and future connectors can plug into the same behavior.

**Tech Stack:** Go 1.25+, stdlib `context`/`time`/`sync`, existing `internal/runtime/capabilities` and `internal/runtime` seams, Zalo Personal protocol client, Go `testing`.

---

## File Structure

### Existing Files To Modify

- `internal/runtime/capabilities/registry.go`
  Add presence request/policy interfaces and registry lookup methods.
- `internal/runtime/capabilities/registry_test.go`
  Cover registration and dispatch of presence adapters.
- `internal/runtime/runs.go`
  Start and stop presence from run lifecycle boundaries.
- `internal/runtime/provider.go`
  Keep `StreamSink` integration aligned if the run loop needs a wrapped sink for first-output stop behavior.
- `internal/runtime/runs_test.go`
  Add direct lifecycle tests for presence start, keepalive, and stop conditions.
- `internal/app/bootstrap.go`
  Inject the capability registry into runtime wiring instead of forcing runtime to go through tools.
- `internal/connectors/zalopersonal/connector.go`
  Expose presence capability from the connector.
- `internal/connectors/zalopersonal/capabilities_test.go`
  Cover connector-level presence behavior.
- `docs/system.md`
  Document automatic presence as a shipped runtime behavior after implementation lands.

### New Files To Create

- `internal/runtime/presence/controller.go`
  Generic lifecycle controller: delay, keepalive, TTL, breaker, idempotent stop.
- `internal/runtime/presence/controller_test.go`
  Unit tests for controller timing and failure handling.
- `internal/runtime/presence/manager.go`
  Route-scoped controller ownership for user-facing runs.
- `internal/runtime/presence/manager_test.go`
  Unit tests for manager start/stop semantics and dedupe.
- `internal/connectors/zalopersonal/capabilities_presence.go`
  Zalo adapter implementing generic presence methods.
- `internal/connectors/zalopersonal/protocol/typing.go`
  Zalo typing endpoint and payload encoding.
- `internal/connectors/zalopersonal/protocol/typing_test.go`
  Protocol tests for DM/group endpoint selection and encrypted payload shape.

## Task 1: Add Generic Presence Capability Seam

**Files:**
- Modify: `internal/runtime/capabilities/registry.go`
- Test: `internal/runtime/capabilities/registry_test.go`

- [ ] **Step 1: Write the failing registry tests**

```go
func TestRegistry_PresenceEmitDispatchesToConnectorAdapter(t *testing.T) {
	reg := NewRegistry()
	conn := &stubConnector{meta: model.ConnectorMetadata{ID: "zalo_personal"}}
	reg.RegisterConnector(conn)

	_, err := reg.PresencePolicy(context.Background(), "zalo_personal")
	if err != nil {
		t.Fatalf("PresencePolicy: %v", err)
	}

	err = reg.EmitPresence(context.Background(), PresenceEmitRequest{
		ConnectorID: "zalo_personal",
		ThreadID:    "123",
		ThreadType:  "contact",
		Mode:        PresenceModeTyping,
	})
	if err != nil {
		t.Fatalf("EmitPresence: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime/capabilities -run 'TestRegistry_Presence'`

Expected: FAIL with missing presence types and registry methods.

- [ ] **Step 3: Write the minimal implementation**

```go
type PresenceMode string

const PresenceModeTyping PresenceMode = "typing"

type PresenceEmitRequest struct {
	ConnectorID    string
	ConversationID string
	ThreadID       string
	ThreadType     string
	Mode           PresenceMode
}

type PresencePolicy struct {
	StartupDelay           time.Duration
	KeepaliveInterval      time.Duration
	MaxDuration            time.Duration
	MaxConsecutiveFailures int
	SupportsStop           bool
}

type PresenceAdapter interface {
	CapabilityPresencePolicy(context.Context) PresencePolicy
	CapabilityEmitPresence(context.Context, PresenceEmitRequest) error
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/runtime/capabilities`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/capabilities/registry.go internal/runtime/capabilities/registry_test.go
git commit -m "feat: add generic presence capability seam"
```

## Task 2: Add Runtime Presence Controller And Manager

**Files:**
- Create: `internal/runtime/presence/controller.go`
- Create: `internal/runtime/presence/controller_test.go`
- Create: `internal/runtime/presence/manager.go`
- Create: `internal/runtime/presence/manager_test.go`

- [ ] **Step 1: Write the failing controller and manager tests**

```go
func TestController_DelaysInitialEmitAndKeepsAlive(t *testing.T) {
	var emits int
	ctrl := NewController(Options{
		StartupDelay:      800 * time.Millisecond,
		KeepaliveInterval: 2 * time.Second,
		MaxDuration:       10 * time.Second,
		StartFn: func(context.Context) error {
			emits++
			return nil
		},
	})
	// assert no emit before delay, then emit after delay, then keepalive
}

func TestManager_ReusesOneControllerPerRoute(t *testing.T) {
	mgr := NewManager()
	// assert repeated Start(route) does not create duplicate controllers
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/runtime/presence -run 'TestController|TestManager'`

Expected: FAIL with missing package and symbols.

- [ ] **Step 3: Write the minimal implementation**

```go
type Controller struct {
	// route-scoped lifecycle state, timers, breaker counters
}

func (c *Controller) Start(ctx context.Context) {}
func (c *Controller) Stop() {}
func (c *Controller) MarkOutputStarted() {}
func (c *Controller) MarkPaused() {}
```

```go
type Manager struct {
	mu          sync.Mutex
	controllers map[string]*Controller
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/runtime/presence`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/presence/controller.go internal/runtime/presence/controller_test.go internal/runtime/presence/manager.go internal/runtime/presence/manager_test.go
git commit -m "feat: add runtime presence controller"
```

## Task 3: Wire Presence Into Runtime Run Lifecycle

**Files:**
- Modify: `internal/app/bootstrap.go`
- Modify: `internal/runtime/runs.go`
- Modify: `internal/runtime/provider.go`
- Test: `internal/runtime/runs_test.go`

- [ ] **Step 1: Write the failing runtime tests**

```go
func TestRunEngine_RemoteLongRunStartsPresence(t *testing.T) {
	// remote source connector + blocking provider
	// assert presence emit recorded after startup delay
}

func TestRunEngine_FirstOutputStopsPresence(t *testing.T) {
	// streaming provider emits delta
	// assert presence stops on first output
}

func TestRunEngine_ApprovalPauseStopsPresence(t *testing.T) {
	// approval-required connector_send call
	// assert controller stops before run returns needs_approval
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/runtime -run 'TestRunEngine_.*Presence'`

Expected: FAIL because runtime does not own presence yet.

- [ ] **Step 3: Write the minimal implementation**

```go
type Runtime struct {
	// existing fields...
	capabilities *capabilities.Registry
	presence     *presence.Manager
}
```

```go
func runtimeWiring(...) *runtime.Runtime {
	return runtime.New(db, cs, reg, capabilityRegistry, mem, prov, sink)
}
```

Implementation rules:
- inject capability registry into `runtime.New`
- start presence only for front, remote, presence-capable runs
- wrap `StreamSink` so first output stops presence
- stop presence on approval pause, cancel, error, and completion
- never route presence through `tools.Registry`

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/runtime`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/bootstrap.go internal/runtime/runs.go internal/runtime/provider.go internal/runtime/runs_test.go
git commit -m "feat: wire automatic presence into runtime"
```

## Task 4: Add Zalo Presence Adapter And Protocol

**Files:**
- Create: `internal/connectors/zalopersonal/capabilities_presence.go`
- Create: `internal/connectors/zalopersonal/protocol/typing.go`
- Create: `internal/connectors/zalopersonal/protocol/typing_test.go`
- Modify: `internal/connectors/zalopersonal/connector.go`
- Test: `internal/connectors/zalopersonal/capabilities_test.go`

- [ ] **Step 1: Write the failing connector and protocol tests**

```go
func TestSendTyping_UserThreadUsesChatEndpoint(t *testing.T) {}

func TestSendTyping_GroupThreadUsesGroupEndpoint(t *testing.T) {}

func TestConnector_CapabilityEmitPresenceRequiresStoredCredentials(t *testing.T) {}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/connectors/zalopersonal/... -run 'TestSendTyping|TestConnector_CapabilityEmitPresence'`

Expected: FAIL with missing adapter and protocol implementation.

- [ ] **Step 3: Write the minimal implementation**

```go
func (c *Connector) CapabilityPresencePolicy(context.Context) capabilities.PresencePolicy {
	return capabilities.PresencePolicy{
		StartupDelay:           800 * time.Millisecond,
		KeepaliveInterval:      8 * time.Second,
		MaxDuration:            60 * time.Second,
		MaxConsecutiveFailures: 2,
	}
}
```

```go
func SendTyping(ctx context.Context, sess Session, threadID, threadType string) error {
	// choose chat or group endpoint, encrypt payload, POST params
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/connectors/zalopersonal/...`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/connectors/zalopersonal/capabilities_presence.go internal/connectors/zalopersonal/protocol/typing.go internal/connectors/zalopersonal/protocol/typing_test.go internal/connectors/zalopersonal/connector.go internal/connectors/zalopersonal/capabilities_test.go
git commit -m "feat: add zalo automatic typing adapter"
```

## Task 5: Final Integration, Docs, And Verification

**Files:**
- Modify: `docs/system.md`
- Test: `internal/app/bootstrap_test.go`
- Test: `internal/runtime/acceptance_test.go`

- [ ] **Step 1: Write the failing integration tests**

```go
func TestBootstrap_RuntimeReceivesCapabilityRegistryForPresence(t *testing.T) {}

func TestAcceptance_RemoteDirectRunCanEmitAutomaticPresence(t *testing.T) {}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app ./internal/runtime -run 'TestBootstrap_.*Presence|TestAcceptance_.*Presence'`

Expected: FAIL until wiring and docs assumptions are complete.

- [ ] **Step 3: Write the minimal implementation and docs update**

Update:
- runtime/bootstrap assertions for injected capability registry
- `docs/system.md` shipped behavior for automatic typing/presence

- [ ] **Step 4: Run the full verification suite**

Run:

```bash
go test ./...
go vet ./...
.bin/golangci-lint run
go test -coverprofile=/tmp/gistclaw.cover ./...
go tool cover -func=/tmp/gistclaw.cover | tail -n 1
```

Expected:
- all tests pass
- lint passes
- coverage remains `>= 70%`

- [ ] **Step 5: Commit**

```bash
git add docs/system.md internal/app/bootstrap_test.go internal/runtime/acceptance_test.go
git commit -m "docs: document automatic presence runtime"
```

## Manual Dogfood After Code Lands

Run these after the implementation commits are green:

```bash
go build -o bin/gistclaw ./cmd/gistclaw
```

Then dogfood on the live Telegram + Zalo sandbox:

1. Send a fast request and confirm no typing flash.
2. Send a slower direct Zalo request and confirm automatic typing appears.
3. Trigger an approval-required send and confirm typing stops before the approval prompt.
4. Trigger a delegated-but-user-facing flow and confirm front-run typing stays alive until output.

## Notes For The Implementer

- Do not add `connector_presence_emit` as an assistant-visible tool.
- Do not hardcode `zalo_personal` or any other connector ID in runtime core.
- Do not route runtime presence through `tools.Registry`; use the capability registry directly.
- Keep lifecycle logic in `internal/runtime/presence/`, not inside connectors.
- If the runtime needs route identity beyond current fields, extend structured route data instead of reparsing strings.
