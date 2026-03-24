# Assistant Platform Reset Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace GistClaw's current repo-task/delegation-centered contract with an assistant-first, session-centric runtime built around one front agent plus spawned worker sessions, while deleting the stale doc set and rewriting only the docs that describe the new contract.

**Architecture:** Keep the SQLite event journal, replay, approvals, and local-first surfaces. Replace rigid handoff-edge delegation with explicit session primitives: `spawn`, `announce`, `steer`, and `agent-send`. Introduce first-class session identity and a new team spec package, defer channel/plugin breadth, and do the rewrite without backward-compatibility shims.

**Tech Stack:** Go 1.24+, SQLite via `modernc.org/sqlite`, stdlib `net/http`, Go `testing`, YAML via `go.yaml.in/yaml/v4`

---

## File Structure

### New/rewritten docs

- `README.md`
  New top-level product overview. Assistant-first story, current status, and local-first runtime summary.
- `docs/vision.md`
  Long-term assistant-platform vision.
- `docs/kernel.md`
  Runtime invariants: front agent, sessions, collaboration primitives, authority boundaries.
- `docs/roadmap.md`
  Immediate build/refactor scope only.
- `docs/extensions.md`
  Providers, connectors, and plugins as deferred extension seams.

### New runtime packages

- `internal/sessions/keys.go`
  Session key normalization and helper constructors for front and worker sessions.
- `internal/sessions/service.go`
  Session lifecycle, follow-up binding, and inter-agent message persistence APIs.
- `internal/sessions/service_test.go`
  Session lifecycle, routing, and message provenance tests.
- `internal/teams/spec.go`
  New assistant/team spec parser and validator.
- `internal/teams/spec_test.go`
  Team spec validation tests for front-agent and spawn/message permissions.
- `internal/runtime/collaboration.go`
  Runtime entrypoints for `spawn`, `announce`, `steer`, and `agent-send`.
- `internal/runtime/collaboration_test.go`
  Collaboration primitive and authority tests.

### Existing files to rewrite

- `internal/model/types.go`
  Add session and collaboration domain types; remove delegation-specific types that no longer define the kernel.
- `internal/conversations/service.go`
  Keep journal/projection ownership, add session projections and collaboration events.
- `internal/conversations/service_test.go`
  Projection coverage for new session/collaboration events.
- `internal/store/migrations/001_init.sql`
  Rewrite the schema in place for session-first runtime state.
- `internal/store/migrate_test.go`
  Schema assertions for new tables/indexes and removed legacy tables.
- `internal/app/team_validation.go`
  Bootstrap-time team spec validation must move from `runtime.LoadTeamSpec` to the new `teams` package.
- `internal/app/team_validation_test.go`
  Coverage for the new team spec rules and capability flags.
- `internal/runtime/runs.go`
  Retain only the parts that still fit the new front-session run loop.
- `internal/replay/service.go`
  Replace handoff-edge graph loading with session/run lineage replay.
- `internal/replay/replay_test.go`
  Replay graph coverage for front and worker sessions.
- `internal/replay/preview_package.go`
  Update preview summaries so they describe session collaboration rather than delegation graphs.
- `internal/runtime/acceptance_test.go`
  Replace milestone acceptance coverage with front-agent/session-centric coverage.
- `internal/runtime/starter_workflow_test.go`
  Reframe repo work as a workflow running on the new runtime, not as the product identity.
- `internal/tools/policy.go`
  Express authority gates in session/team terms instead of delegation edges.
- `internal/tools/approvals.go`
  Keep fingerprint-based approvals, but bind them to the new session/runtime flow.
- `internal/tools/tools_test.go`
  Coverage for new authority checks.
- `internal/app/bootstrap.go`
  Wire the new session/team runtime and stop bootstrapping deferred breadth.
- `internal/app/lifecycle.go`
  Update startup preparation and remove scheduler assumptions from the immediate rewrite.
- `internal/app/config.go`
  Keep config minimal and aligned with the reset.
- `internal/web/server.go`
  Trim routes to the surfaces that still match the new contract.
- `internal/web/routes_run_submit.go`
  Start a front-agent session instead of a repo-task-only run.
- `internal/web/routes_runs.go`
  Show session-backed runtime state.
- `internal/web/server_test.go`
  Keep route coverage aligned with the reduced surface.
- `internal/web/templates/run_submit.html`
  Update the form language from "task" to assistant/session-oriented wording.
- `internal/web/templates/runs.html`
  Reflect front session and worker activity instead of delegation-first language.
- `teams/default/team.yaml`
  Rewrite default team spec around `front_agent`, spawn permissions, and message permissions.

### Files to delete

- `docs/00-research-plan.md`
- `docs/00-system-diagrams.md`
- `docs/01-docs-vs-code-map.md`
- `docs/02-runtime-and-gateway.md`
- `docs/03-agent-runtime-and-sessions.md`
- `docs/04-memory-and-soul.md`
- `docs/05-multi-agent-and-delegation.md`
- `docs/06-tools-skills-plugins-security.md`
- `docs/07-providers-channels-and-interfaces.md`
- `docs/08-storage-config-and-ops.md`
- `docs/09-openclaw-strengths.md`
- `docs/10-openclaw-weaknesses.md`
- `docs/11-architecture-redesign.md`
- `docs/12-go-package-structure.md`
- `docs/13-core-interfaces.md`
- `docs/14-memory-model.md`
- `docs/15-security-model.md`
- `docs/16-roadmap-and-kill-list.md`
- `docs/17-ceo-review.md`
- `docs/18-eng-review.md`
- `docs/README.md`
- `docs/dependencies.md`
- `docs/implementation-plan.md`
- `docs/designs/ui-design-spec.md`
- `docs/designs/v1-implementation-plan.md`
- `docs/superpowers/plans/2026-03-24-m1-kernel-proof.md`
- `docs/superpowers/plans/2026-03-24-m2-local-beta.md`
- `docs/superpowers/plans/2026-03-24-m3-public-beta.md`
- `docs/superpowers/plans/2026-03-24-m4-stable-1-0.md`
- `internal/runtime/delegations.go`
- `internal/runtime/delegations_test.go`
- `internal/runtime/team.go`
- `internal/scheduler/cron.go`
- `internal/scheduler/cron_test.go`
- `internal/scheduler/dispatcher.go`
- `internal/scheduler/dispatcher_test.go`
- `internal/store/migrations/003_scheduler.sql`
- `internal/web/routes_team.go`
- `internal/web/routes_team_test.go`
- `internal/web/templates/team.html`
- `internal/web/templates/team_composer.html`

The connector packages under `internal/connectors/` remain in the tree for now but must be unwired from bootstrap during the immediate rewrite. Do not expand them in this phase.

## Task 1: Replace The Doc Set

**Files:**
- Modify: `README.md`
- Create: `docs/vision.md`
- Create: `docs/kernel.md`
- Create: `docs/roadmap.md`
- Create: `docs/extensions.md`
- Delete: `docs/00-research-plan.md`
- Delete: `docs/00-system-diagrams.md`
- Delete: `docs/01-docs-vs-code-map.md`
- Delete: `docs/02-runtime-and-gateway.md`
- Delete: `docs/03-agent-runtime-and-sessions.md`
- Delete: `docs/04-memory-and-soul.md`
- Delete: `docs/05-multi-agent-and-delegation.md`
- Delete: `docs/06-tools-skills-plugins-security.md`
- Delete: `docs/07-providers-channels-and-interfaces.md`
- Delete: `docs/08-storage-config-and-ops.md`
- Delete: `docs/09-openclaw-strengths.md`
- Delete: `docs/10-openclaw-weaknesses.md`
- Delete: `docs/11-architecture-redesign.md`
- Delete: `docs/12-go-package-structure.md`
- Delete: `docs/13-core-interfaces.md`
- Delete: `docs/14-memory-model.md`
- Delete: `docs/15-security-model.md`
- Delete: `docs/16-roadmap-and-kill-list.md`
- Delete: `docs/17-ceo-review.md`
- Delete: `docs/18-eng-review.md`
- Delete: `docs/README.md`
- Delete: `docs/dependencies.md`
- Delete: `docs/implementation-plan.md`
- Delete: `docs/designs/ui-design-spec.md`
- Delete: `docs/designs/v1-implementation-plan.md`
- Delete: `docs/superpowers/plans/2026-03-24-m1-kernel-proof.md`
- Delete: `docs/superpowers/plans/2026-03-24-m2-local-beta.md`
- Delete: `docs/superpowers/plans/2026-03-24-m3-public-beta.md`
- Delete: `docs/superpowers/plans/2026-03-24-m4-stable-1-0.md`
- Test: doc inventory via `rg --files docs README.md`

- [ ] **Step 1: Delete the stale docs**

```bash
git rm docs/00-research-plan.md \
  docs/00-system-diagrams.md \
  docs/01-docs-vs-code-map.md \
  docs/02-runtime-and-gateway.md \
  docs/03-agent-runtime-and-sessions.md \
  docs/04-memory-and-soul.md \
  docs/05-multi-agent-and-delegation.md \
  docs/06-tools-skills-plugins-security.md \
  docs/07-providers-channels-and-interfaces.md \
  docs/08-storage-config-and-ops.md \
  docs/09-openclaw-strengths.md \
  docs/10-openclaw-weaknesses.md \
  docs/11-architecture-redesign.md \
  docs/12-go-package-structure.md \
  docs/13-core-interfaces.md \
  docs/14-memory-model.md \
  docs/15-security-model.md \
  docs/16-roadmap-and-kill-list.md \
  docs/17-ceo-review.md \
  docs/18-eng-review.md \
  docs/README.md \
  docs/dependencies.md \
  docs/implementation-plan.md \
  docs/designs/ui-design-spec.md \
  docs/designs/v1-implementation-plan.md \
  docs/superpowers/plans/2026-03-24-m1-kernel-proof.md \
  docs/superpowers/plans/2026-03-24-m2-local-beta.md \
  docs/superpowers/plans/2026-03-24-m3-public-beta.md \
  docs/superpowers/plans/2026-03-24-m4-stable-1-0.md
```

- [ ] **Step 2: Rewrite `README.md` around the new contract**

```md
# GistClaw

GistClaw is a personal assistant platform built on a local-first multi-agent runtime.

One front agent owns the user relationship. It can spawn worker agents behind the scenes, coordinate them through explicit runtime sessions, ask before risky actions, and keep a replayable record of what happened.
```

- [ ] **Step 3: Write `docs/vision.md` and `docs/kernel.md`**

```md
# Vision

- assistant-first product
- OpenClaw-like breadth over time
- teams and plugins as expansion, not day-one requirements
```

```md
# Kernel

- front agent
- spawned worker sessions
- collaboration primitives: spawn, announce, steer, agent-send
- runtime-owned approvals and authority
```

- [ ] **Step 4: Write `docs/roadmap.md` and `docs/extensions.md`**

```md
# Roadmap

Immediate:
- doc reset
- session-first runtime reset
- team spec reset

Deferred:
- channel matrix
- plugin marketplace
- broad automation
```

- [ ] **Step 5: Verify the replacement doc set**

Run: `rg --files docs README.md | sort`
Expected: only the new docs, `docs/superpowers/specs/2026-03-24-assistant-platform-reset-design.md`, and this implementation plan remain under active documentation.

- [ ] **Step 6: Commit**

```bash
git add README.md docs
git commit -m "docs: replace legacy docs with assistant platform reset docs"
```

## Task 2: Rewrite The Schema And Domain Types For Sessions

**Files:**
- Modify: `internal/store/migrations/001_init.sql`
- Modify: `internal/store/migrate_test.go`
- Modify: `internal/model/types.go`
- Modify: `internal/model/types_test.go`

- [ ] **Step 1: Write the failing migration test for session-first tables**

```go
func TestMigrateCreatesSessionRuntimeTables(t *testing.T) {
	db := openTestDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{
		"conversations",
		"events",
		"runs",
		"sessions",
		"session_messages",
		"session_bindings",
		"tool_calls",
		"approvals",
		"receipts",
	} {
		assertTableExists(t, db, table)
	}

	assertTableMissing(t, db, "delegations")
}
```

- [ ] **Step 2: Run the migration test and confirm it fails**

Run: `go test ./internal/store -run TestMigrateCreatesSessionRuntimeTables -v`
Expected: FAIL because `sessions`, `session_messages`, and `session_bindings` do not exist yet and `delegations` still exists.

- [ ] **Step 3: Rewrite `001_init.sql` and define the session domain model in `internal/model/types.go`**

```sql
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    key TEXT NOT NULL UNIQUE,
    agent_id TEXT NOT NULL,
    role TEXT NOT NULL,
    parent_session_id TEXT,
    controller_session_id TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS session_messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    sender_session_id TEXT,
    kind TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS session_bindings (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    thread_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
```

```go
type SessionRole string

const (
	SessionRoleFront  SessionRole = "front"
	SessionRoleWorker SessionRole = "worker"
)

type SessionMessageKind string

const (
	MessageUser      SessionMessageKind = "user"
	MessageAssistant SessionMessageKind = "assistant"
	MessageSpawn     SessionMessageKind = "spawn"
	MessageAnnounce  SessionMessageKind = "announce"
	MessageSteer     SessionMessageKind = "steer"
	MessageAgentSend SessionMessageKind = "agent_send"
)

type Session struct {
	ID                  string
	ConversationID      string
	Key                 string
	AgentID             string
	Role                SessionRole
	ParentSessionID     string
	ControllerSessionID string
	Status              string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type SessionMessage struct {
	ID              string
	SessionID       string
	SenderSessionID string
	Kind            SessionMessageKind
	Body            string
	CreatedAt       time.Time
}
```

Required invariants for these types:

- a front session has empty `ParentSessionID` and empty `ControllerSessionID`
- a worker session has a non-empty `ControllerSessionID`
- `Session.Key` is stable and unique for the life of the session
- `SessionMessage.Kind` must always be explicit; runtime-generated collaboration traffic is never stored as plain user text without provenance

Ownership rule:

- `SessionRole`, `SessionMessageKind`, `Session`, and `SessionMessage` live in `internal/model/types.go`
- `internal/sessions/service.go` must reuse those `model.*` types and must not define shadow copies

- [ ] **Step 4: Add the model tests for new enum validation and zero-value behavior**

```go
func TestSessionMessageKindsRemainStable(t *testing.T) {
	got := []SessionMessageKind{
		MessageUser, MessageAssistant, MessageSpawn, MessageAnnounce, MessageSteer, MessageAgentSend,
	}
	want := []SessionMessageKind{
		"user", "assistant", "spawn", "announce", "steer", "agent_send",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected message kinds: %#v", got)
	}
}
```

- [ ] **Step 5: Run the targeted tests**

Run: `go test ./internal/store ./internal/model -run 'TestMigrateCreatesSessionRuntimeTables|TestSessionMessageKindsRemainStable' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/store/migrations/001_init.sql internal/store/migrate_test.go internal/model/types.go internal/model/types_test.go
git commit -m "refactor(store): introduce session-first runtime schema"
```

## Task 3: Add The `internal/sessions` Package

**Files:**
- Create: `internal/sessions/keys.go`
- Create: `internal/sessions/service.go`
- Create: `internal/sessions/service_test.go`
- Modify: `internal/model/types.go`
- Modify: `internal/conversations/service.go`
- Modify: `internal/conversations/service_test.go`

- [ ] **Step 1: Write the failing session lifecycle tests**

```go
func TestService_OpenFrontSession(t *testing.T) {
	svc := newTestSessionService(t)
	sess, err := svc.OpenFrontSession(context.Background(), OpenFrontSession{
		ConversationID: "conv-1",
		AgentID:        "assistant",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sess.Role != model.SessionRoleFront {
		t.Fatalf("expected front session, got %s", sess.Role)
	}
}

func TestService_SpawnWorkerSessionPersistsMessage(t *testing.T) {
	svc := newTestSessionService(t)
	parent := openFrontSession(t, svc)
	child, err := svc.SpawnWorkerSession(context.Background(), SpawnWorkerSession{
		ControllerSessionID: parent.ID,
		AgentID:             "researcher",
		InitialPrompt:       "Investigate the repo layout.",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertMessageKind(t, svc.DB(), child.ID, model.MessageSpawn)
}
```

- [ ] **Step 2: Run the session tests and confirm failure**

Run: `go test ./internal/sessions -run 'TestService_OpenFrontSession|TestService_SpawnWorkerSessionPersistsMessage' -v`
Expected: FAIL because the package does not exist yet.

- [ ] **Step 3: Implement `keys.go` and `service.go`**

```go
func BuildFrontSessionKey(conversationID string) string {
	return "front:" + conversationID
}

func BuildWorkerSessionKey(parentSessionID, workerID string) string {
	return "worker:" + parentSessionID + ":" + workerID
}
```

```go
type Service struct {
	db   *store.DB
	conv *conversations.ConversationStore
}

type OpenFrontSession struct {
	ConversationID string
	AgentID        string
	WorkspaceRoot  string
}

type SpawnWorkerSession struct {
	ConversationID       string
	ParentSessionID      string
	ControllerSessionID  string
	AgentID              string
	InitialPrompt        string
}

type BindFollowUp struct {
	ConversationID string
	ThreadID       string
	SessionID      string
}

func (s *Service) OpenFrontSession(ctx context.Context, cmd OpenFrontSession) (model.Session, error) { /* ... */ }
func (s *Service) SpawnWorkerSession(ctx context.Context, cmd SpawnWorkerSession) (model.Session, error) { /* ... */ }
func (s *Service) AppendMessage(ctx context.Context, msg model.SessionMessage) error { /* ... */ }
func (s *Service) BindFollowUp(ctx context.Context, cmd BindFollowUp) error { /* ... */ }
```

Do not re-declare `Session` or `SessionMessage` in `internal/sessions`. The service layer must depend on the domain types added to `internal/model/types.go` in Task 2.

Invariants to enforce in implementation:

- one active front session per conversation
- worker sessions always reference a parent or controller session
- `announce`, `steer`, and `agent_send` messages are persisted as session messages with explicit kind
- follow-up binding is optional, but when present it resolves to one active session per `(conversation_id, thread_id)`

- [ ] **Step 4: Extend `internal/conversations/service.go` projections for new session events**

```go
case "session_opened":
    _, err := tx.ExecContext(ctx, `INSERT INTO sessions (...) VALUES (...)`, ...)
case "session_message_added":
    _, err := tx.ExecContext(ctx, `INSERT INTO session_messages (...) VALUES (...)`, ...)
case "session_bound":
    _, err := tx.ExecContext(ctx, `INSERT INTO session_bindings (...) VALUES (...)`, ...)
```

- [ ] **Step 5: Run the targeted tests**

Run: `go test ./internal/sessions ./internal/conversations -run 'TestService_|TestConversationStore' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/sessions internal/conversations/service.go internal/conversations/service_test.go
git commit -m "feat(runtime): add session lifecycle store and projections"
```

## Task 4: Replace The Team Spec Model

**Files:**
- Create: `internal/teams/spec.go`
- Create: `internal/teams/spec_test.go`
- Modify: `internal/app/team_validation.go`
- Modify: `internal/app/team_validation_test.go`
- Modify: `teams/default/team.yaml`
- Modify: `internal/model/types.go`
- Delete: `internal/runtime/team.go`

- [ ] **Step 1: Write the failing team spec validation tests**

```go
func TestLoadSpec_RequiresFrontAgent(t *testing.T) {
	_, err := LoadSpec([]byte("name: default\nagents: []\n"))
	if err == nil || !strings.Contains(err.Error(), "front_agent") {
		t.Fatalf("expected front_agent validation error, got %v", err)
	}
}

func TestLoadSpec_RejectsUnknownSpawnTarget(t *testing.T) {
	data := []byte(`
name: default
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    can_spawn: ["ghost"]
`)
	_, err := LoadSpec(data)
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("expected unknown spawn target error, got %v", err)
	}
}
```

- [ ] **Step 2: Run the team spec tests and confirm failure**

Run: `go test ./internal/teams -run 'TestLoadSpec_' -v`
Expected: FAIL because the package does not exist yet.

- [ ] **Step 3: Implement `internal/teams/spec.go`**

```go
type Spec struct {
	Name       string      `yaml:"name"`
	FrontAgent string      `yaml:"front_agent"`
	Agents     []AgentSpec `yaml:"agents"`
}

type AgentSpec struct {
	ID         string   `yaml:"id"`
	SoulFile   string   `yaml:"soul_file"`
	CanSpawn   []string `yaml:"can_spawn"`
	CanMessage []string `yaml:"can_message"`
}
```

- [ ] **Step 4: Move bootstrap validation to `internal/teams`**

```go
spec, err := teams.LoadSpec(data)
if err != nil {
	return fmt.Errorf("team validation: %w", err)
}

for _, agent := range spec.Agents {
	soulPath := filepath.Join(teamDir, agent.SoulFile)
	// verify soul exists
}
```

- [ ] **Step 5: Rewrite `teams/default/team.yaml`**

```yaml
name: default
front_agent: assistant
agents:
  - id: assistant
    soul_file: coordinator.soul.yaml
    can_spawn: [patcher, reviewer, verifier]
    can_message: [patcher, reviewer, verifier]
  - id: patcher
    soul_file: patcher.soul.yaml
    can_spawn: []
    can_message: [assistant, reviewer, verifier]
```

- [ ] **Step 6: Add `spawn` to capability validation in `internal/model/types.go` if needed**

```go
const CapSpawn AgentCapability = "spawn"
```

- [ ] **Step 7: Run the targeted tests**

Run: `go test ./internal/teams ./internal/app ./internal/model -run 'TestLoadSpec_|TestValidateTeamDir|TestIsValidCapability' -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/teams internal/app/team_validation.go internal/app/team_validation_test.go teams/default/team.yaml internal/model/types.go
git rm internal/runtime/team.go
git commit -m "refactor(team): replace handoff graph spec with front-agent team spec"
```

## Task 5: Replace Delegations With Collaboration Primitives

**Files:**
- Modify: `internal/runtime/runs.go`
- Create: `internal/runtime/collaboration.go`
- Create: `internal/runtime/collaboration_test.go`
- Modify: `internal/runtime/acceptance_test.go`
- Modify: `internal/runtime/starter_workflow_test.go`
- Delete: `internal/runtime/delegations.go`
- Delete: `internal/runtime/delegations_test.go`

- [ ] **Step 1: Write the failing collaboration tests**

```go
func TestRuntime_StartFrontSessionCreatesFrontRunAndSession(t *testing.T) {
	rt := newTestRuntime(t)
	run, err := rt.StartFrontSession(context.Background(), StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Help me inspect this repo",
		WorkspaceRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.ParentRunID != "" {
		t.Fatalf("front run must not have a parent, got %q", run.ParentRunID)
	}
}

func TestRuntime_SpawnCreatesWorkerRunAndSession(t *testing.T) {
	rt := newTestRuntime(t)
	parent := startFrontRun(t, rt, "Investigate the repo")

	child, err := rt.Spawn(context.Background(), SpawnCommand{
		ControllerRunID: parent.ID,
		AgentID:         "researcher",
		Prompt:          "Inspect the docs folder.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentRunID != parent.ID {
		t.Fatalf("expected parent %s, got %s", parent.ID, child.ParentRunID)
	}
}

func TestRuntime_AnnouncePersistsInterAgentMessage(t *testing.T) {
	rt := newTestRuntime(t)
	parent, child := startParentAndChildRuns(t, rt)
	if err := rt.Announce(context.Background(), AnnounceCommand{
		WorkerRunID: child.ID,
		TargetRunID: parent.ID,
		Body:        "Tests passed.",
	}); err != nil {
		t.Fatal(err)
	}
	assertRunEvent(t, rt.DB(), parent.ID, "session_message_added")
}
```

- [ ] **Step 2: Run the collaboration tests and confirm failure**

Run: `go test ./internal/runtime -run 'TestRuntime_StartFrontSessionCreatesFrontRunAndSession|TestRuntime_SpawnCreatesWorkerRunAndSession|TestRuntime_AnnouncePersistsInterAgentMessage' -v`
Expected: FAIL because `StartFrontSession`, `Spawn`, and `Announce` do not exist and the old delegation model is still wired in.

- [ ] **Step 3: Implement `internal/runtime/collaboration.go` and trim `runs.go`**

```go
type StartFrontSession struct {
	ConversationKey conversations.ConversationKey
	FrontAgentID    string
	InitialPrompt   string
	WorkspaceRoot   string
}

type SpawnCommand struct {
	ControllerRunID string
	AgentID         string
	Prompt          string
}

type AnnounceCommand struct {
	WorkerRunID string
	TargetRunID string
	Body        string
}

type SteerCommand struct {
	ControllerRunID string
	TargetRunID     string
	Body            string
}

type AgentSendCommand struct {
	FromRunID string
	ToRunID   string
	Body      string
}

func (r *Runtime) StartFrontSession(ctx context.Context, cmd StartFrontSession) (model.Run, error) { /* ... */ }
func (r *Runtime) Spawn(ctx context.Context, cmd SpawnCommand) (model.Run, error) { /* ... */ }
func (r *Runtime) Announce(ctx context.Context, cmd AnnounceCommand) error { /* ... */ }
func (r *Runtime) Steer(ctx context.Context, cmd SteerCommand) error { /* ... */ }
func (r *Runtime) AgentSend(ctx context.Context, cmd AgentSendCommand) error { /* ... */ }
```

`StartFrontSession` is the replacement for the old `SubmitTask` web entrypoint. It should:

- resolve the conversation from `ConversationKey`
- open or resume the active front session for that conversation
- create the root run for the front agent
- persist the initial user message as a session message
- enter the normal runtime loop

- [ ] **Step 4: Reframe acceptance coverage around front session + worker session behavior**

```go
func TestAcceptance_FrontSessionCanSpawnAndReceiveAnnounce(t *testing.T) {
	// start front run
	// spawn worker
	// worker announces completion
	// assert replay and receipts still build
}
```

- [ ] **Step 5: Reframe the starter workflow as a workflow on top of the new runtime**

```go
func TestStarterWorkflow_RepoPatchRunsAsWorkerFlow(t *testing.T) {
	// front agent requests repo change
	// patch worker proposes/apply path
	// verifier worker announces result
}
```

- [ ] **Step 6: Run the targeted runtime tests**

Run: `go test ./internal/runtime -run 'TestRuntime_|TestAcceptance_|TestStarterWorkflow_' -v`
Expected: PASS

- [ ] **Step 7: Delete the delegation files and commit**

```bash
git rm internal/runtime/delegations.go internal/runtime/delegations_test.go
git add internal/runtime/runs.go internal/runtime/collaboration.go internal/runtime/collaboration_test.go internal/runtime/acceptance_test.go internal/runtime/starter_workflow_test.go
git commit -m "refactor(runtime): replace delegations with session collaboration primitives"
```

## Task 6: Rewrite Replay And Preview Around Sessions

**Files:**
- Modify: `internal/replay/service.go`
- Modify: `internal/replay/replay_test.go`
- Modify: `internal/replay/preview_package.go`
- Modify: `internal/web/routes_runs.go`
- Modify: `internal/web/sse.go`

- [ ] **Step 1: Write the failing replay graph test**

```go
func TestLoadGraph_UsesSessionLineageInsteadOfHandoffEdges(t *testing.T) {
	db := openReplayDB(t)
	insertRun(t, db, "run-front", "", "assistant")
	insertRun(t, db, "run-worker", "run-front", "reviewer")

	graph, err := replay.NewService(db).LoadGraph(context.Background(), "run-front")
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Edges) != 1 || graph.Edges[0] != (GraphEdge{From: "run-front", To: "run-worker"}) {
		t.Fatalf("unexpected graph: %#v", graph.Edges)
	}
}
```

- [ ] **Step 2: Run the replay test and confirm failure**

Run: `go test ./internal/replay -run TestLoadGraph_UsesSessionLineageInsteadOfHandoffEdges -v`
Expected: FAIL because replay still reads `handoff_edges` out of `execution_snapshot_json`.

- [ ] **Step 3: Rewrite `internal/replay/service.go` to use actual runtime lineage**

```go
func (s *Service) LoadGraph(ctx context.Context, rootRunID string) (ReplayGraph, error) {
	rows, err := s.db.RawDB().QueryContext(ctx,
		`SELECT id, COALESCE(parent_run_id, '') FROM runs
		 WHERE id = ? OR parent_run_id = ?
		 ORDER BY created_at ASC`,
		rootRunID, rootRunID,
	)
	// build edges from run lineage instead of snapshot handoff edges
}
```

- [ ] **Step 4: Update preview/runs UI summaries to describe front and worker sessions**

```go
Summary: fmt.Sprintf("Run %s: front session with %d worker runs", runID, workerCount)
```

- [ ] **Step 5: Run the targeted replay/web tests**

Run: `go test ./internal/replay ./internal/web -run 'TestLoadGraph_|TestRun' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/replay/service.go internal/replay/replay_test.go internal/replay/preview_package.go internal/web/routes_runs.go internal/web/sse.go
git commit -m "refactor(replay): render runtime replay from session lineage"
```

## Task 7: Rebind Tools And Approvals To Session Authority

**Files:**
- Modify: `internal/tools/policy.go`
- Modify: `internal/tools/approvals.go`
- Modify: `internal/tools/workspace.go`
- Modify: `internal/tools/tools_test.go`
- Modify: `internal/model/types.go`

- [ ] **Step 1: Write the failing authority tests**

```go
func TestPolicy_DeniesSpawnForAgentWithoutCapability(t *testing.T) {
	decision := DefaultPolicy().Decide(context.Background(), model.AgentProfile{
		AgentID:      "worker",
		Capabilities: []model.AgentCapability{model.CapReadHeavy},
	}, model.RunProfile{}, model.ToolSpec{Name: "session_spawn"})
	if decision.Mode != model.DecisionDeny {
		t.Fatalf("expected deny, got %s", decision.Mode)
	}
}

func TestWorkspaceApply_RequiresApprovedTicketForWorkerRun(t *testing.T) {
	// mirror the old approval fingerprint check, but through worker-run authority
}
```

- [ ] **Step 2: Run the tool tests and confirm failure**

Run: `go test ./internal/tools -run 'TestPolicy_DeniesSpawnForAgentWithoutCapability|TestWorkspaceApply_RequiresApprovedTicketForWorkerRun' -v`
Expected: FAIL because the policy does not know about session collaboration authority yet.

- [ ] **Step 3: Update policy and approval code**

```go
switch tool.Name {
case "session_spawn":
	if !hasCapability(agent.Capabilities, model.CapSpawn) {
		return model.ToolDecision{Mode: model.DecisionDeny, Reason: "spawn capability required"}
	}
case "workspace_apply":
	if !hasCapability(agent.Capabilities, model.CapWorkspaceWrite) {
		return model.ToolDecision{Mode: model.DecisionDeny, Reason: "workspace_write capability required"}
	}
}
```

- [ ] **Step 4: Tag inter-agent messages and keep approval fingerprints unchanged**

```go
type ApprovalRequest struct {
	RunID      string
	ToolName   string
	ArgsJSON   []byte
	TargetPath string
	SessionID  string
}
```

- [ ] **Step 5: Run the targeted tests**

Run: `go test ./internal/tools ./internal/model -run 'TestPolicy_|TestWorkspaceApply_|TestApproval' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tools/policy.go internal/tools/approvals.go internal/tools/workspace.go internal/tools/tools_test.go internal/model/types.go
git commit -m "refactor(tools): bind authority and approvals to session runtime"
```

## Task 8: Rewire App Bootstrap And Web Surface

**Files:**
- Modify: `internal/app/bootstrap.go`
- Modify: `internal/app/lifecycle.go`
- Modify: `internal/app/config.go`
- Modify: `internal/web/server.go`
- Modify: `internal/web/routes_run_submit.go`
- Modify: `internal/web/routes_runs.go`
- Modify: `internal/web/server_test.go`
- Modify: `internal/web/templates/run_submit.html`
- Modify: `internal/web/templates/runs.html`
- Delete: `internal/web/routes_team.go`
- Delete: `internal/web/routes_team_test.go`
- Delete: `internal/web/templates/team.html`
- Delete: `internal/web/templates/team_composer.html`

- [ ] **Step 1: Write the failing web/bootstrap tests**

```go
func TestHandleRunSubmit_StartsFrontAgentSession(t *testing.T) {
	srv := newTestServer(t)
	resp := postForm(t, srv, "/run", url.Values{"task": {"Help me review this repo"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", resp.StatusCode)
	}
}

func TestBootstrap_DoesNotWireDeferredConnectorsOrScheduler(t *testing.T) {
	app := bootstrapTestApp(t)
	if len(app.connectors) != 0 {
		t.Fatalf("expected deferred connectors to be unwired, got %d", len(app.connectors))
	}
	if app.scheduler != nil {
		t.Fatal("expected scheduler to be nil in the reset phase")
	}
}
```

- [ ] **Step 2: Run the web/app tests and confirm failure**

Run: `go test ./internal/app ./internal/web -run 'TestHandleRunSubmit_StartsFrontAgentSession|TestBootstrap_DoesNotWireDeferredConnectorsOrScheduler' -v`
Expected: FAIL because the current submit flow still calls `SubmitTask` and bootstrap still wires scheduler/connectors.

- [ ] **Step 3: Rewire bootstrap and lifecycle to the immediate reset scope**

```go
func Bootstrap(cfg Config) (*App, error) {
	// keep db, conversation store, sessions service, runtime, replay, web server
	// do not wire scheduler or deferred connectors in this phase
}
```

- [ ] **Step 4: Simplify the web surface**

```go
func (s *Server) handleRunSubmit(w http.ResponseWriter, r *http.Request) {
	message := strings.TrimSpace(r.FormValue("task"))
	run, err := s.rt.StartFrontSession(r.Context(), runtime.StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: message,
		WorkspaceRoot: lookupSetting(s.db, "workspace_root"),
	})
	// redirect to /runs/{id}
}
```

The web handler must not persist the initial user message separately. `StartFrontSession` owns both session open/resume and initial prompt persistence.

- [ ] **Step 5: Delete the team-composer web surface**

```bash
git rm internal/web/routes_team.go internal/web/routes_team_test.go internal/web/templates/team.html internal/web/templates/team_composer.html
```

- [ ] **Step 6: Run the targeted tests**

Run: `go test ./internal/app ./internal/web -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/app/bootstrap.go internal/app/lifecycle.go internal/app/config.go internal/web/server.go internal/web/routes_run_submit.go internal/web/routes_runs.go internal/web/server_test.go internal/web/templates/run_submit.html internal/web/templates/runs.html
git commit -m "refactor(app): rewire bootstrap and web surface for front-agent runtime"
```

## Task 9: Remove Deferred Breadth From The Immediate Build

**Files:**
- Delete: `internal/scheduler/cron.go`
- Delete: `internal/scheduler/cron_test.go`
- Delete: `internal/scheduler/dispatcher.go`
- Delete: `internal/scheduler/dispatcher_test.go`
- Delete: `internal/store/migrations/003_scheduler.sql`
- Modify: `internal/app/bootstrap.go`
- Modify: `internal/app/lifecycle.go`
- Modify: `cmd/gistclaw/main.go`

- [ ] **Step 1: Write the failing startup/build test**

```go
func TestMain_StartsWithoutSchedulerPackage(t *testing.T) {
	// smoke-build the command and verify the reset bootstrap path compiles
}
```

- [ ] **Step 2: Remove scheduler references**

```bash
git rm internal/scheduler/cron.go internal/scheduler/cron_test.go internal/scheduler/dispatcher.go internal/scheduler/dispatcher_test.go internal/store/migrations/003_scheduler.sql
```

- [ ] **Step 3: Remove scheduler fields and startup calls from app/bootstrap lifecycle**

```go
type App struct {
	cfg       Config
	db        *store.DB
	convStore *conversations.ConversationStore
	runtime   *runtime.Runtime
	replay    *replay.Service
	webServer *web.Server
}
```

- [ ] **Step 4: Run package build/tests**

Run: `go test ./internal/app ./cmd/gistclaw -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/bootstrap.go internal/app/lifecycle.go cmd/gistclaw/main.go
git commit -m "refactor(app): drop deferred scheduler breadth from reset build"
```

## Task 10: Final Verification And Cleanup

**Files:**
- Modify: any small fallout fixes discovered during full verification

- [ ] **Step 1: Run formatting**

Run: `gofmt -w $(rg --files . -g '*.go')`
Expected: no output and formatted files only.

- [ ] **Step 2: Run the full test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 3: Run coverage**

Run: `go test -cover ./...`
Expected: PASS with total coverage at or above `70%`.

- [ ] **Step 4: Run vet**

Run: `go vet ./...`
Expected: PASS with no diagnostics.

- [ ] **Step 5: Run a final docs inventory check**

Run: `rg --files docs README.md | sort`
Expected: only the new reset-oriented docs plus `docs/superpowers/specs/2026-03-24-assistant-platform-reset-design.md` and this implementation plan remain.

- [ ] **Step 6: Commit final fallout fixes**

```bash
git add -A
git commit -m "chore: finalize assistant platform reset rewrite"
```
