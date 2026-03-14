# Codex First-Class Agent Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Codex as a first-class coding agent alongside OpenCode and Claude Code, backed by a shared controller/runtime layer, durable task persistence, and Telegram-facing command/orchestration support.

**Architecture:** Keep plain-chat providers and coding-agent runtimes separate. Introduce a new `internal/codeagent` layer plus durable store tables, implement a real `internal/codexagent` provider on `codex app-server`, then rewire `app`, `gateway`, `scheduler`, and `tools` to talk to the controller instead of directly to OpenCode and Claude services.

**Tech Stack:** Go 1.25, stdlib `os/exec` + JSON decoding, `modernc.org/sqlite`, zerolog, existing Telegram/HITL/store packages.

**Spec:** `docs/superpowers/specs/2026-03-14-codex-first-class-agent-design.md`

**Implementation Notes:** Before execution, create an isolated worktree with `@using-git-worktrees`. Implement each task with `@test-driven-development`, and do not claim completion until `@verification-before-completion` evidence is collected.

---

## File Structure

### Existing files to modify

- `internal/agent/kind.go`
  Add `KindCodex` string/parse support.
- `internal/agent/kind_test.go`
  Extend round-trip tests for the new kind.
- `internal/config/config.go`
  Add Codex runtime config fields and helper methods without conflating them with `LLM_PROVIDER`.
- `internal/config/config_test.go`
  Add parsing/validation coverage for optional Codex runtime config.
- `sample.env`
  Document Codex runtime env vars.
- `internal/store/schema.sql`
  Add durable agent runtime tables.
- `internal/store/sqlite.go`
  Add CRUD methods for sessions, tasks, events, and pending actions.
- `internal/store/sqlite_test.go`
  Add persistence coverage for the new tables/methods.
- `internal/app/app.go`
  Build the controller, register providers, and route scheduler tasks through it.
- `internal/app/app_test.go`
  Add wiring/routing coverage for `KindCodex`.
- `internal/gateway/service.go`
  Replace direct coding-agent dependencies with a controller-facing dependency.
- `internal/gateway/router.go`
  Add `/cx`, update `/stop`, help text, and status output.
- `internal/gateway/service_test.go`
  Add `/cx`, stop, and status behavior coverage through the controller path.
- `internal/tools/agents_tool.go`
  Replace OpenCode/Claude-specific orchestration with controller-backed dispatch for all registered kinds.
- `internal/tools/agents_tool_test.go`
  Extend orchestration coverage to include Codex and generic controller routing.
- `docs/architecture.md`
  Update the architecture overview once implementation is complete.

### New files to create

- `internal/codeagent/types.go`
  Provider/session/task/event contracts and shared enums.
- `internal/codeagent/controller.go`
  Registry, session cache, task submission, event persistence, and HITL routing.
- `internal/codeagent/controller_test.go`
  Fake-provider tests for session reuse, event persistence, and result collection.
- `internal/codeagent/render.go`
  Convert normalized events into Telegram output chunks and terminal summaries.
- `internal/codeagent/keys.go`
  Internal session-cache key types and helpers.
- `internal/codexagent/config.go`
  Codex runtime config and defaults.
- `internal/codexagent/protocol.go`
  JSON-RPC envelope types, request/notification parsing, and approval payload shapes.
- `internal/codexagent/normalize.go`
  Map Codex notifications/server requests into normalized `codeagent.Event` values.
- `internal/codexagent/service.go`
  Process management, handshake, thread/turn lifecycle, and approval/question resolution.
- `internal/codexagent/service_test.go`
  Fake-transport tests for thread start, turn start, message deltas, approvals, and completion.

### Existing files to leave alone

- `internal/providers/codex/codex.go`
  This remains the plain-chat Codex OAuth provider. Do not repurpose it into the coding-agent runtime.

---

## Chunk 1: Public Surface And Persistence

### Task 1: Add `KindCodex` and Codex runtime config

**Files:**
- Modify: `internal/agent/kind.go`
- Modify: `internal/agent/kind_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `sample.env`

- [ ] **Step 1: Write the failing tests for the new kind and config**

Add to `internal/agent/kind_test.go`:

```go
func TestKindCodex_RoundTrip(t *testing.T) {
	if got := agent.KindCodex.String(); got != "codex" {
		t.Fatalf("KindCodex.String() = %q, want %q", got, "codex")
	}
	k, err := agent.KindFromString("codex")
	if err != nil {
		t.Fatalf("KindFromString(\"codex\") error: %v", err)
	}
	if k != agent.KindCodex {
		t.Fatalf("KindFromString(\"codex\") = %v, want %v", k, agent.KindCodex)
	}
}
```

Add to `internal/config/config_test.go`:

```go
func TestLoadCodexRuntimeOptionalWhenUnset(t *testing.T) {
	os.Clearenv()
	t.Setenv("TELEGRAM_TOKEN", "tok")
	t.Setenv("ALLOWED_USER_IDS", "1")
	t.Setenv("OPENCODE_DIR", "/tmp/oc")
	t.Setenv("CLAUDE_DIR", "/tmp/cc")
	t.Setenv("OPENAI_API_KEY", "sk-test")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg.HasCodexAgent() {
		t.Fatal("HasCodexAgent() = true with CODEX_DIR unset")
	}
	if cfg.CodexBin != "codex" {
		t.Fatalf("CodexBin default = %q, want %q", cfg.CodexBin, "codex")
	}
}

func TestLoadCodexRuntimeFromEnv(t *testing.T) {
	os.Clearenv()
	t.Setenv("TELEGRAM_TOKEN", "tok")
	t.Setenv("ALLOWED_USER_IDS", "1")
	t.Setenv("OPENCODE_DIR", "/tmp/oc")
	t.Setenv("CLAUDE_DIR", "/tmp/cc")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("CODEX_DIR", "/tmp/cx")
	t.Setenv("CODEX_MODEL", "gpt-5-codex")
	t.Setenv("CODEX_SANDBOX_MODE", "workspace-write")
	t.Setenv("CODEX_APPROVAL_POLICY", "on-request")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if !cfg.HasCodexAgent() {
		t.Fatal("HasCodexAgent() = false with CODEX_DIR set")
	}
	if cfg.CodexDir != "/tmp/cx" {
		t.Fatalf("CodexDir = %q, want %q", cfg.CodexDir, "/tmp/cx")
	}
}
```

- [ ] **Step 2: Run the focused tests to confirm they fail**

Run:

```bash
cd /Users/canh/Projects/Claw/gistclaw
go test ./internal/agent/... ./internal/config/... -run 'TestKindCodex_RoundTrip|TestLoadCodexRuntime' -v
```

Expected:

- `TestKindCodex_RoundTrip` fails because `KindCodex` does not exist.
- `TestLoadCodexRuntime...` fails because Codex config fields/helpers do not exist.

- [ ] **Step 3: Add the minimal implementation**

Update `internal/agent/kind.go`:

```go
const (
	KindUnknown    Kind = -1
	KindOpenCode   Kind = 0
	KindClaudeCode Kind = 1
	KindChat       Kind = 2
	KindGateway    Kind = 3
	KindCodex      Kind = 4
)
```

Add the string and parse branches:

```go
case KindCodex:
	return "codex"
```

```go
case "codex":
	return KindCodex, nil
```

Update `internal/config/config.go`:

```go
type Config struct {
	// existing fields...
	CodexBin            string `env:"CODEX_BIN"             envDefault:"codex"`
	CodexDir            string `env:"CODEX_DIR"`
	CodexHome           string `env:"CODEX_HOME"`
	CodexModel          string `env:"CODEX_MODEL"           envDefault:"gpt-5-codex"`
	CodexSandboxMode    string `env:"CODEX_SANDBOX_MODE"    envDefault:"workspace-write"`
	CodexApprovalPolicy string `env:"CODEX_APPROVAL_POLICY" envDefault:"on-request"`
}

func (c Config) HasCodexAgent() bool {
	return c.CodexDir != ""
}
```

Do **not** make Codex mandatory in `validate()`. Existing installs without Codex must continue to pass validation.

Update `sample.env` with a documented optional block:

```dotenv
# Working directory for Codex agent tasks (optional; when unset /cx is unavailable)
# CODEX_DIR=/home/gistclaw/projects
# CODEX_BIN=codex
# CODEX_HOME=/home/gistclaw/.codex
# CODEX_MODEL=gpt-5-codex
# CODEX_SANDBOX_MODE=workspace-write
# CODEX_APPROVAL_POLICY=on-request
```

- [ ] **Step 4: Run the tests to confirm they pass**

Run:

```bash
go test ./internal/agent/... ./internal/config/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/agent/kind.go internal/agent/kind_test.go internal/config/config.go internal/config/config_test.go sample.env
git commit -m "feat(config): add codex runtime kind and settings"
```

---

### Task 2: Add durable agent runtime tables and store methods

**Files:**
- Modify: `internal/store/schema.sql`
- Modify: `internal/store/sqlite.go`
- Modify: `internal/store/sqlite_test.go`

- [ ] **Step 1: Write the failing persistence tests**

Add to `internal/store/sqlite_test.go`:

```go
func TestAgentRuntimeTablesExist(t *testing.T) {
	s := newTestStore(t)
	for _, table := range []string{
		"agent_sessions",
		"agent_tasks",
		"agent_events",
		"agent_pending_actions",
	} {
		if err := s.Ping(table); err != nil {
			t.Fatalf("Ping(%q): %v", table, err)
		}
	}
}

func TestAgentTaskLifecycleRoundTrip(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()

	if err := s.InsertAgentSession("sess-1", "codex", "thread-1", 42, "/tmp/cx", "idle", now); err != nil {
		t.Fatalf("InsertAgentSession: %v", err)
	}
	if err := s.InsertAgentTask("task-1", "sess-1", "", "", "add tests", "running", now); err != nil {
		t.Fatalf("InsertAgentTask: %v", err)
	}
	if err := s.AppendAgentEvent("task-1", 1, "text.delta", `{"text":"hello"}`, now); err != nil {
		t.Fatalf("AppendAgentEvent: %v", err)
	}
	if err := s.InsertAgentPendingAction("pa-1", "task-1", "req-1", "approval", `{"command":"go test ./..."}`, now); err != nil {
		t.Fatalf("InsertAgentPendingAction: %v", err)
	}
	if err := s.ResolveAgentPendingAction("pa-1", "approved", now); err != nil {
		t.Fatalf("ResolveAgentPendingAction: %v", err)
	}
}
```

- [ ] **Step 2: Run the focused store tests to confirm they fail**

Run:

```bash
go test ./internal/store/... -run 'TestAgentRuntimeTablesExist|TestAgentTaskLifecycleRoundTrip' -v
```

Expected: FAIL because the tables and methods do not exist yet.

- [ ] **Step 3: Add schema and store methods**

Append to `internal/store/schema.sql`:

```sql
CREATE TABLE IF NOT EXISTS agent_sessions (
    id          TEXT PRIMARY KEY,
    kind        TEXT NOT NULL,
    native_id   TEXT NOT NULL,
    chat_id     INTEGER NOT NULL,
    cwd         TEXT NOT NULL,
    status      TEXT NOT NULL,
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL,
    finished_at DATETIME
);

CREATE TABLE IF NOT EXISTS agent_tasks (
    id             TEXT PRIMARY KEY,
    session_id     TEXT NOT NULL,
    parent_task_id TEXT NOT NULL DEFAULT '',
    workflow_id    TEXT NOT NULL DEFAULT '',
    prompt         TEXT NOT NULL,
    status         TEXT NOT NULL,
    created_at     DATETIME NOT NULL,
    started_at     DATETIME,
    finished_at    DATETIME,
    FOREIGN KEY(session_id) REFERENCES agent_sessions(id)
);

CREATE TABLE IF NOT EXISTS agent_events (
    task_id      TEXT NOT NULL,
    seq          INTEGER NOT NULL,
    type         TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at   DATETIME NOT NULL,
    PRIMARY KEY (task_id, seq),
    FOREIGN KEY(task_id) REFERENCES agent_tasks(id)
);

CREATE TABLE IF NOT EXISTS agent_pending_actions (
    id                  TEXT PRIMARY KEY,
    task_id             TEXT NOT NULL,
    provider_request_id TEXT NOT NULL,
    kind                TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'pending',
    payload_json        TEXT NOT NULL,
    created_at          DATETIME NOT NULL,
    resolved_at         DATETIME,
    FOREIGN KEY(task_id) REFERENCES agent_tasks(id)
);
```

Add focused row types and methods to `internal/store/sqlite.go`:

```go
type AgentSessionRow struct {
	ID        string
	Kind      string
	NativeID  string
	ChatID    int64
	Cwd       string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (s *Store) InsertAgentSession(id, kind, nativeID string, chatID int64, cwd, status string, now time.Time) error
func (s *Store) UpdateAgentSessionStatus(id, status string, now time.Time, finishedAt *time.Time) error
func (s *Store) InsertAgentTask(id, sessionID, parentTaskID, workflowID, prompt, status string, now time.Time) error
func (s *Store) UpdateAgentTaskStatus(id, status string, finishedAt *time.Time) error
func (s *Store) AppendAgentEvent(taskID string, seq int, typ, payloadJSON string, now time.Time) error
func (s *Store) InsertAgentPendingAction(id, taskID, providerRequestID, kind, payloadJSON string, now time.Time) error
func (s *Store) ResolveAgentPendingAction(id, status string, now time.Time) error
```

Follow the existing error style:

```go
return fmt.Errorf("store: insert agent session %q: %w", id, err)
```

- [ ] **Step 4: Run the store suite**

Run:

```bash
go test ./internal/store/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/store/schema.sql internal/store/sqlite.go internal/store/sqlite_test.go
git commit -m "feat(store): add durable coding-agent runtime tables"
```

---

## Chunk 2: Shared Runtime Core

### Task 3: Add the shared `internal/codeagent` contracts and controller

**Files:**
- Create: `internal/codeagent/types.go`
- Create: `internal/codeagent/controller.go`
- Create: `internal/codeagent/render.go`
- Create: `internal/codeagent/keys.go`
- Create: `internal/codeagent/controller_test.go`

- [ ] **Step 1: Write the failing controller tests**

Create `internal/codeagent/controller_test.go` with a fake provider/session/task:

```go
package codeagent_test

import (
	"context"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/agent"
	"github.com/canhta/gistclaw/internal/codeagent"
)

func TestControllerReusesSessionPerKindAndChat(t *testing.T) {
	fp := newFakeProvider(agent.KindCodex, "codex")
	c := newTestController(t, fp)

	if err := c.SubmitTask(context.Background(), agent.KindCodex, 42, "/tmp/cx", "task one"); err != nil {
		t.Fatalf("SubmitTask #1: %v", err)
	}
	if err := c.SubmitTask(context.Background(), agent.KindCodex, 42, "/tmp/cx", "task two"); err != nil {
		t.Fatalf("SubmitTask #2: %v", err)
	}
	if fp.startSessionCalls != 1 {
		t.Fatalf("StartSession calls = %d, want 1", fp.startSessionCalls)
	}
}

func TestControllerPersistsEventsAndCollectsFinalText(t *testing.T) {
	fp := newFakeProvider(agent.KindCodex, "codex")
	fp.taskEvents = []codeagent.Event{
		{Type: codeagent.EventTextDelta, Text: "hello "},
		{Type: codeagent.EventTextDelta, Text: "world"},
		{Type: codeagent.EventTaskCompleted, FinalText: "hello world"},
	}
	c := newTestController(t, fp)

	got, err := c.SubmitTaskWithResult(context.Background(), agent.KindCodex, 42, "/tmp/cx", "say hello")
	if err != nil {
		t.Fatalf("SubmitTaskWithResult: %v", err)
	}
	if got != "hello world" {
		t.Fatalf("final text = %q, want %q", got, "hello world")
	}
}
```

- [ ] **Step 2: Run the package tests to confirm they fail**

Run:

```bash
go test ./internal/codeagent/... -v
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement the shared contracts and minimal controller**

Create `internal/codeagent/types.go`:

```go
package codeagent

import "context"

type EventType string

const (
	EventTaskStarted       EventType = "task.started"
	EventTextDelta         EventType = "text.delta"
	EventPlanUpdated       EventType = "plan.updated"
	EventApprovalRequested EventType = "approval.requested"
	EventQuestionRequested EventType = "question.requested"
	EventTaskCompleted     EventType = "task.completed"
	EventTaskFailed        EventType = "task.failed"
)

type Event struct {
	Type      EventType
	Text      string
	FinalText string
	Payload   string
}

type Provider interface {
	Name() string
	Kind() agent.Kind
	Run(ctx context.Context) error
	StartSession(ctx context.Context, spec SessionSpec) (Session, error)
	IsAlive(ctx context.Context) bool
}

type SessionSpec struct {
	ChatID int64
	Cwd    string
}

type Session interface {
	ID() string
	NativeID() string
	StartTask(ctx context.Context, req TaskRequest) (Task, error)
	Stop(ctx context.Context) error
}

type TaskRequest struct {
	Prompt string
}

type Task interface {
	ID() string
	Events() <-chan Event
}
```

Create `internal/codeagent/controller.go` with a compatibility-oriented surface used by `gateway`, `app`, and tools:

```go
type Controller struct {
	providers map[agent.Kind]Provider
	store     *store.Store
	sessions  map[sessionKey]Session
}

func (c *Controller) SubmitTask(ctx context.Context, kind agent.Kind, chatID int64, cwd, prompt string) error
func (c *Controller) SubmitTaskWithResult(ctx context.Context, kind agent.Kind, chatID int64, cwd, prompt string) (string, error)
func (c *Controller) Stop(ctx context.Context, kind agent.Kind, chatID int64) error
func (c *Controller) IsAlive(ctx context.Context, kind agent.Kind) bool
```

In `SubmitTaskWithResult`, accumulate `EventTextDelta` and `EventTaskCompleted.FinalText` so orchestration tools can keep their current behavior while the rest of the runtime moves to structured events.

In `render.go`, add a small helper:

```go
func CollectFinalText(events <-chan Event) (string, error)
```

- [ ] **Step 4: Run the package tests to confirm they pass**

Run:

```bash
go test ./internal/codeagent/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/codeagent/types.go internal/codeagent/controller.go internal/codeagent/render.go internal/codeagent/keys.go internal/codeagent/controller_test.go
git commit -m "feat(codeagent): add shared controller and runtime contracts"
```

---

### Task 4: Add compatibility adapters for OpenCode and Claude

**Files:**
- Modify: `internal/codeagent/controller.go`
- Modify: `internal/codeagent/types.go`
- Create: `internal/codeagent/controller_test.go` (extend existing tests)

- [ ] **Step 1: Write the failing adapter test**

Extend `internal/codeagent/controller_test.go`:

```go
func TestControllerDispatchesDifferentKindsToDifferentProviders(t *testing.T) {
	oc := newFakeProvider(agent.KindOpenCode, "opencode")
	cc := newFakeProvider(agent.KindClaudeCode, "claudecode")
	cx := newFakeProvider(agent.KindCodex, "codex")
	c := newTestController(t, oc, cc, cx)

	if err := c.SubmitTask(context.Background(), agent.KindOpenCode, 42, "/tmp/oc", "build"); err != nil {
		t.Fatalf("OpenCode: %v", err)
	}
	if err := c.SubmitTask(context.Background(), agent.KindClaudeCode, 42, "/tmp/cc", "review"); err != nil {
		t.Fatalf("Claude: %v", err)
	}
	if err := c.SubmitTask(context.Background(), agent.KindCodex, 42, "/tmp/cx", "test"); err != nil {
		t.Fatalf("Codex: %v", err)
	}

	if oc.startSessionCalls != 1 || cc.startSessionCalls != 1 || cx.startSessionCalls != 1 {
		t.Fatal("providers were not selected by kind correctly")
	}
}
```

- [ ] **Step 2: Run the focused controller test to confirm current coverage is insufficient**

Run:

```bash
go test ./internal/codeagent/... -run TestControllerDispatchesDifferentKindsToDifferentProviders -v
```

Expected: FAIL until the controller constructor and fake-provider helpers support multiple providers.

- [ ] **Step 3: Implement the multi-provider registration path**

Add a constructor like:

```go
func NewController(s *store.Store, providers ...Provider) *Controller {
	reg := make(map[agent.Kind]Provider, len(providers))
	for _, p := range providers {
		reg[p.Kind()] = p
	}
	return &Controller{
		providers: reg,
		store:     s,
		sessions:  make(map[sessionKey]Session),
	}
}
```

Keep the API generic. Do **not** add OpenCode/Claude-specific fields to the controller.

- [ ] **Step 4: Run the controller suite**

Run:

```bash
go test ./internal/codeagent/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/codeagent/controller.go internal/codeagent/controller_test.go internal/codeagent/types.go
git commit -m "refactor(codeagent): register providers generically by kind"
```

---

## Chunk 3: Codex Runtime Provider

### Task 5: Implement `internal/codexagent` protocol parsing and normalization

**Files:**
- Create: `internal/codexagent/config.go`
- Create: `internal/codexagent/protocol.go`
- Create: `internal/codexagent/normalize.go`
- Create: `internal/codexagent/service_test.go`

- [ ] **Step 1: Write the failing normalization tests**

Create `internal/codexagent/service_test.go`:

```go
package codexagent

import (
	"testing"

	"github.com/canhta/gistclaw/internal/codeagent"
)

func TestNormalizeAgentMessageDelta(t *testing.T) {
	ev, ok, err := normalizeNotification(`{
		"method":"item/agentMessage/delta",
		"params":{"threadId":"th_1","turnId":"tu_1","itemId":"it_1","delta":"hello"}
	}`)
	if err != nil {
		t.Fatalf("normalizeNotification: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if ev.Type != codeagent.EventTextDelta || ev.Text != "hello" {
		t.Fatalf("event = %#v, want text.delta(\"hello\")", ev)
	}
}

func TestNormalizeTurnCompleted(t *testing.T) {
	ev, ok, err := normalizeNotification(`{
		"method":"turn/completed",
		"params":{"threadId":"th_1","turn":{"id":"tu_1","status":"completed","items":[],"error":null}}
	}`)
	if err != nil {
		t.Fatalf("normalizeNotification: %v", err)
	}
	if !ok || ev.Type != codeagent.EventTaskCompleted {
		t.Fatalf("event = %#v, ok = %v", ev, ok)
	}
}

func TestNormalizeApprovalRequest(t *testing.T) {
	ev, ok, err := normalizeServerRequest(`{
		"method":"item/commandExecution/requestApproval",
		"id":"req_1",
		"params":{"threadId":"th_1","turnId":"tu_1","itemId":"it_1","command":"go test ./...","cwd":"/tmp/cx"}
	}`)
	if err != nil {
		t.Fatalf("normalizeServerRequest: %v", err)
	}
	if !ok || ev.Type != codeagent.EventApprovalRequested {
		t.Fatalf("event = %#v, ok = %v", ev, ok)
	}
}
```

- [ ] **Step 2: Run the new tests to confirm they fail**

Run:

```bash
go test ./internal/codexagent/... -v
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Add the parser and normalizer**

In `internal/codexagent/protocol.go`, define the JSON-RPC envelopes and only the fields needed for phase 1:

```go
type rpcEnvelope struct {
	Method string          `json:"method"`
	ID     string          `json:"id"`
	Params json.RawMessage `json:"params"`
}

type agentMessageDeltaParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	Delta    string `json:"delta"`
}
```

In `internal/codexagent/normalize.go`, implement:

```go
func normalizeNotification(line string) (codeagent.Event, bool, error)
func normalizeServerRequest(line string) (codeagent.Event, bool, error)
```

Map these methods:

- `item/agentMessage/delta` -> `EventTextDelta`
- `turn/plan/updated` -> `EventPlanUpdated`
- `turn/completed` -> `EventTaskCompleted` or `EventTaskFailed`
- `item/commandExecution/requestApproval` -> `EventApprovalRequested`
- `item/tool/requestUserInput` -> `EventQuestionRequested`

Unknown notifications must return `(codeagent.Event{}, false, nil)`.

- [ ] **Step 4: Run the Codex package tests**

Run:

```bash
go test ./internal/codexagent/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/codexagent/config.go internal/codexagent/protocol.go internal/codexagent/normalize.go internal/codexagent/service_test.go
git commit -m "feat(codexagent): add json-rpc protocol parsing and normalization"
```

---

### Task 6: Implement the Codex provider on `codex app-server`

**Files:**
- Create: `internal/codexagent/service.go`
- Modify: `internal/codexagent/service_test.go`

- [ ] **Step 1: Write the failing provider lifecycle test**

Extend `internal/codexagent/service_test.go`:

```go
func TestServiceStartSessionAndTaskLifecycle(t *testing.T) {
	transport := newFakeTransport()
	svc := New(Config{
		Bin:            "codex",
		Dir:            "/tmp/cx",
		Model:          "gpt-5-codex",
		SandboxMode:    "workspace-write",
		ApprovalPolicy: "on-request",
	}, transport)

	sess, err := svc.StartSession(context.Background(), codeagent.SessionSpec{
		ChatID: 42,
		Cwd:    "/tmp/cx",
	})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	task, err := sess.StartTask(context.Background(), codeagent.TaskRequest{Prompt: "say hi"})
	if err != nil {
		t.Fatalf("StartTask: %v", err)
	}

	var got []codeagent.Event
	for ev := range task.Events() {
		got = append(got, ev)
	}
	if len(got) == 0 {
		t.Fatal("got no events")
	}
}
```

- [ ] **Step 2: Run the focused Codex tests to confirm they fail**

Run:

```bash
go test ./internal/codexagent/... -run TestServiceStartSessionAndTaskLifecycle -v
```

Expected: FAIL because `New`, `StartSession`, and `StartTask` are not implemented.

- [ ] **Step 3: Implement the provider/service**

In `internal/codexagent/service.go`, build around a replaceable transport so tests do not need a real Codex binary:

```go
type transport interface {
	Call(ctx context.Context, method string, params any, out any) error
	Notifications() <-chan string
	ServerRequests() <-chan string
	Close() error
}
```

Production path:

- launch `codex app-server` via `exec.CommandContext`
- wire stdin/stdout pipes
- send `initialize`
- send `initialized`
- on `StartSession`, call `thread/start`
- on `StartTask`, call `turn/start`
- fan notifications + server requests through `normalizeNotification` / `normalizeServerRequest`
- close the task event channel after terminal completion

Session IDs:

- `Session.ID()` should be the GistClaw-side stable session ID
- `Session.NativeID()` should be Codex `threadId`

- [ ] **Step 4: Run the Codex package tests**

Run:

```bash
go test ./internal/codexagent/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/codexagent/service.go internal/codexagent/service_test.go
git commit -m "feat(codexagent): run codex app-server through shared runtime"
```

---

## Chunk 4: App, Gateway, Scheduler, And Tools Wiring

### Task 7: Wire the controller and Codex provider into `app`

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`

- [ ] **Step 1: Write the failing app routing test**

Add to `internal/app/app_test.go`:

```go
func TestAppJobTarget_RunAgentTask_Codex(t *testing.T) {
	target := app.NewTestJobTargetWithAgents(
		nil, // gateway runner
		func(context.Context, int64, string) error { return nil }, // opencode
		func(context.Context, int64, string) error { return nil }, // claudecode
		func(context.Context, int64, string) error { return nil }, // codex
	)

	if err := target.RunAgentTask(context.Background(), agent.KindCodex, "run tests"); err != nil {
		t.Fatalf("RunAgentTask(KindCodex): %v", err)
	}
}
```

- [ ] **Step 2: Run the focused app test to confirm it fails**

Run:

```bash
go test ./internal/app/... -run TestAppJobTarget_RunAgentTask_Codex -v
```

Expected: FAIL because the test helper and routing path do not know about Codex.

- [ ] **Step 3: Implement the app wiring**

Refactor `internal/app/app.go`:

- build `codexagent.Config` from `config.Config`
- instantiate `codexagent.New(...)` only when `cfg.HasCodexAgent()` is true
- build `codeagent.NewController(...)` with OpenCode adapter, Claude adapter, and optional Codex provider
- change `appJobTarget` to depend on the controller for coding-agent kinds

Recommended shape:

```go
type agentSubmitter interface {
	SubmitTask(ctx context.Context, kind agent.Kind, chatID int64, cwd, prompt string) error
}

type appJobTarget struct {
	agents agentSubmitter
	ch     channel.Channel
	cfg    config.Config
	gwRun  func(ctx context.Context, chatID int64, prompt string) error
}
```

Route:

```go
case agent.KindOpenCode, agent.KindClaudeCode, agent.KindCodex:
	return t.agents.SubmitTask(ctx, kind, operatorChatID, "", prompt)
```

- [ ] **Step 4: Run the app suite**

Run:

```bash
go test ./internal/app/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/app/app.go internal/app/app_test.go internal/app/export_test.go
git commit -m "feat(app): wire codex provider through shared agent controller"
```

---

### Task 8: Expose `/cx`, update status/help, and route orchestrators through the controller

**Files:**
- Modify: `internal/gateway/service.go`
- Modify: `internal/gateway/router.go`
- Modify: `internal/gateway/service_test.go`
- Modify: `internal/tools/agents_tool.go`
- Modify: `internal/tools/agents_tool_test.go`

- [ ] **Step 1: Write the failing gateway and tool tests**

Add to `internal/gateway/service_test.go`:

```go
func TestGateway_CXCommand(t *testing.T) {
	ch := newMockChannel()
	ctrl := &mockAgentController{}
	svc := newServiceWithController(t, ch, ctrl)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "/cx run go test ./..."}
	time.Sleep(100 * time.Millisecond)

	if ctrl.lastKind != agent.KindCodex {
		t.Fatalf("lastKind = %v, want %v", ctrl.lastKind, agent.KindCodex)
	}
}
```

Add to `internal/tools/agents_tool_test.go`:

```go
func TestSpawnAgent_DispatchesCodex(t *testing.T) {
	ctrl := &mockController{}
	tool := tools.NewSpawnAgentTool(ctrl, 123, context.Background())
	result := tool.Execute(context.Background(), map[string]any{
		"kind":   "codex",
		"prompt": "run tests",
	})
	if result.ForLLM == "" {
		t.Fatal("ForLLM should not be empty")
	}
	if ctrl.lastKind != "codex" {
		t.Fatalf("lastKind = %q, want %q", ctrl.lastKind, "codex")
	}
}
```

- [ ] **Step 2: Run the focused tests to confirm they fail**

Run:

```bash
go test ./internal/gateway/... ./internal/tools/... -run 'TestGateway_CXCommand|TestSpawnAgent_DispatchesCodex' -v
```

Expected: FAIL because `/cx` and controller-backed tool constructors do not exist.

- [ ] **Step 3: Implement the gateway/tool wiring**

Update `internal/gateway/service.go` to replace `opencode` / `claudecode` fields with a controller-facing dependency:

```go
type agentController interface {
	SubmitTask(ctx context.Context, kind agent.Kind, chatID int64, cwd, prompt string) error
	SubmitTaskWithResult(ctx context.Context, kind agent.Kind, chatID int64, cwd, prompt string) (string, error)
	Stop(ctx context.Context, kind agent.Kind, chatID int64) error
	IsAlive(ctx context.Context, kind agent.Kind) bool
}
```

Update `internal/gateway/router.go`:

```go
case text == "/cx":
	_ = s.ch.SendMessage(ctx, msg.ChatID, "Usage: /cx <task> — e.g. /cx run the full Go test suite")
case strings.HasPrefix(text, "/cx "):
	prompt := strings.TrimPrefix(text, "/cx ")
	if err := s.agents.SubmitTask(ctx, agent.KindCodex, msg.ChatID, s.cfg.CodexDir, prompt); err != nil {
		_ = s.ch.SendMessage(ctx, msg.ChatID, "⚠️ Codex error: "+err.Error())
	}
```

Update the help fallback string to include `/cx`.

Update status output to show Codex:

```go
cxStatus := "idle"
if s.agents.IsAlive(ctx, agent.KindCodex) {
	cxStatus = "running"
}
fmt.Fprintf(&sb, "Codex: %s\n", cxStatus)
```

Refactor `internal/tools/agents_tool.go` to use one controller instead of separate OpenCode/Claude services:

```go
type agentController interface {
	SubmitTask(ctx context.Context, kind agent.Kind, chatID int64, cwd, prompt string) error
	SubmitTaskWithResult(ctx context.Context, kind agent.Kind, chatID int64, cwd, prompt string) (string, error)
}
```

Allow `"codex"` in tool schemas and kind parsing.

- [ ] **Step 4: Run the gateway and tools suites**

Run:

```bash
go test ./internal/gateway/... ./internal/tools/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/gateway/service.go internal/gateway/router.go internal/gateway/service_test.go internal/tools/agents_tool.go internal/tools/agents_tool_test.go
git commit -m "feat(gateway): expose codex command and controller-backed orchestration"
```

---

### Task 9: Update architecture docs and perform full verification

**Files:**
- Modify: `docs/architecture.md`

- [ ] **Step 1: Write the failing doc expectation**

Add a short regression-style test note to your execution checklist: after implementation, `docs/architecture.md` must mention:

- `KindCodex`
- `internal/codeagent`
- `internal/codexagent`
- controller-backed orchestration

There is no doc test in the repo today, so the "failing test" here is a manual grep gate:

```bash
rg -n "KindCodex|internal/codeagent|internal/codexagent|controller" docs/architecture.md
```

Expected before edits: incomplete or no matches.

- [ ] **Step 2: Update the architecture doc**

Revise the service topology and interface sections in `docs/architecture.md` so they reflect:

- a shared agent controller
- Codex as a first-class agent
- OpenCode/Claude/Codex provider registration
- scheduler dispatch through the controller

- [ ] **Step 3: Run the full verification set**

Run:

```bash
go test ./internal/agent/... -v
go test ./internal/config/... -v
go test ./internal/store/... -v
go test ./internal/codeagent/... -v
go test ./internal/codexagent/... -v
go test ./internal/app/... -v
go test ./internal/gateway/... -v
go test ./internal/tools/... -v
make test
make lint
```

Expected:

- all focused package tests PASS
- `make test` PASS
- `make lint` PASS

- [ ] **Step 4: Commit**

Run:

```bash
git add docs/architecture.md
git commit -m "docs: document codex first-class agent architecture"
```

- [ ] **Step 5: Final verification summary**

Record in the execution handoff:

- which Codex env vars are now supported
- whether `codex app-server` was exercised in tests or only fake transport tests
- any remaining gaps, especially around advanced approval decisions and resume behavior

---

## Execution Order

1. Task 1
2. Task 2
3. Task 3
4. Task 4
5. Task 5
6. Task 6
7. Task 7
8. Task 8
9. Task 9

Do not start Task 5 until Tasks 1 to 4 are passing. Do not wire `/cx` or scheduler routing before the controller and Codex provider exist behind tests.

---

## Risks To Watch During Execution

- Do not break existing installs by making Codex config mandatory.
- Do not mix the plain-chat Codex OAuth provider with the new Codex coding-agent runtime.
- Do not let provider packages send Telegram messages directly once the controller path exists.
- Keep JSON-RPC parsing narrow: only implement the fields used in phase 1.
- Pin behavior around session reuse. Use one controller-owned session per `(kind, chatID, cwd)` unless a stronger need emerges during implementation.

---

## Done Definition

The feature is done when:

- `/cx` works through the same controller path as other coding agents
- scheduler jobs can target `kind=codex`
- orchestration tools accept `kind="codex"`
- Codex tasks persist sessions, tasks, events, and pending approvals
- the existing plain-chat provider stack still works unchanged
- `make test` and `make lint` pass

