# Core Interfaces

## Design goal

Keep interfaces narrow enough that they enforce clarity instead of creating a fake platform SDK.

The point is not maximum reuse. The point is controlled seams.

## Stable-release abstraction rule

Keep interfaces only where they isolate a real seam the runtime is likely to swap, mock heavily, or split later.

Stable-release interface seams:

- connector
- provider
- tool and tool policy
- approval gate
- team-spec store
- memory store
- conversation store
- run engine
- live replay stream
- receipt reporter
- budget guard

Keep these as concrete services in the first implementation:

- connector-local DM matching
- starter-team validation and handoff validation
- starter-team loading
- first-run guidance and preview packaging
- operator-surface resolution and progress notification
- memory promotion and publish rules
- completion formatting, activity snapshots, and grounded explanations
- later-phase sharing and repo publish-back helpers

## Normalized inbound envelope

```go
type Envelope struct {
    ConnectorID     string
    AccountID       string
    ActorID         string
    ConversationID  string
    ThreadID        string
    MessageID       string
    Text            string
    Attachments     []AttachmentRef
    ReceivedAt      time.Time
    Capabilities    CapabilitySet
    Metadata        map[string]string
}
```

Rules:

- every inbound connector must normalize into this shape
- connector-specific raw payloads do not cross the boundary

## Canonical conversation key

```go
type ConversationKey struct {
    ConnectorID string
    AccountID   string
    ExternalID  string
    ThreadID    string
}
```

Rules:

- every durable conversation is identified by one structured `ConversationKey`
- every `ConversationKey` also has one normalized string form for logs, receipts, indexes, and replay references
- the normalized key must be built from connector id, account id, external conversation id, and normalized thread id
- missing thread ids normalize to one boring default value such as `main`
- actor ids never define conversation identity
- connectors do not invent their own durable key formats outside this model
- the same inbound event must always resolve to the same canonical key
- each canonical conversation may have only one active root run at a time
- team binding is carried by the Run, not the Conversation; the conversation key identifies the external channel; runs and delegations carry team context
- removing TeamID means the same external channel always maps to the same conversation regardless of which team is currently assigned to it

## Append-only runtime journal

```go
type Event struct {
    ID             string
    ConversationID string
    RunID          string
    ParentRunID    string
    Kind           EventKind
    PayloadJSON    []byte
    CreatedAt      time.Time
}
```

Rules:

- `events` is the durable append-only journal for inbound messages, run lifecycle changes, delegations, tool calls, approvals, memory publishes, verification checkpoints, and outbound delivery milestones
- current-state tables may cache status, indexes, and read-optimized views, but they are projections, not peer sources of truth
- replay, live replay, receipts, activity snapshots, and debugging should derive from the same journal plus projections
- the daemon owns journal appends and projection updates inside the same transaction boundary
- the runtime must not rebuild truth by scraping prompt text or mutating transcript files
- outbound intents, delivery attempts, confirmations, retries, and terminal failures must all be first-class journal events

## Connector interface

```go
type Connector interface {
    ID() string
    Capabilities() ConnectorCapabilities
    Start(ctx context.Context, sink InboundSink) error
    Deliver(ctx context.Context, msg OutboundMessage) (DeliveryReceipt, error)
}

type InboundSink interface {
    Accept(ctx context.Context, env Envelope) error
}
```

Decision:

- one inbound entrypoint
- one outbound delivery method
- no connector-owned routing engine

Rules:

- the daemon persists a durable outbound intent before calling `Deliver`
- stable release uses at-least-once delivery semantics with connector-specific dedupe keys when available
- connector delivery receipts feed replay and receipt projections instead of living only in ephemeral logs

## Connector invocation matching

Keep Telegram DM matching as a concrete connector-local service in the stable release.

Rules:

- stable release does not rely on a group-invocation policy because Telegram groups are deferred
- later connector expansions may add explicit invocation matching for bounded group surfaces

## Provider error type

All provider adapters must translate vendor-specific errors into `ProviderError` before returning to the run engine. The run engine switches on `Code` only and must not type-assert vendor error types.

```go
type ProviderErrorCode string

const (
    ErrRateLimit             ProviderErrorCode = "rate_limit"
    ErrContextWindowExceeded ProviderErrorCode = "context_window_exceeded"
    ErrModelRefusal          ProviderErrorCode = "model_refusal"
    ErrProviderTimeout       ProviderErrorCode = "provider_timeout"
    ErrMalformedResponse     ProviderErrorCode = "malformed_response"
)

type ProviderError struct {
    Code      ProviderErrorCode
    Message   string
    Retryable bool
}

func (e *ProviderError) Error() string { return string(e.Code) + ": " + e.Message }
```

Rules:

- all five error codes are required; any new adapter must handle all five
- adapters may not return raw vendor errors to callers outside the provider package
- `Retryable` is advisory; the run engine applies its own retry budget regardless

## Provider interface

```go
type Provider interface {
    ID() string
    Capabilities() ModelCapabilities
    Generate(ctx context.Context, req GenerateRequest, stream StreamSink) (GenerateResult, error)
}
```

`GenerateRequest` should contain:

- compiled instructions
- conversation context
- tool specs
- model id
- temperature/thinking settings
- attachment refs

Decision:

- one main method
- streaming is passed as a sink
- provider-specific branching stays inside adapter packages

## Model selection policy

```go
type ModelSelectionPolicy interface {
    Select(ctx context.Context, agent AgentProfile, phase RunPhase, req GenerateRequest) (ModelSelection, error)
}
```

Rules:

- stable release uses fixed agent- and phase-based lanes, not a hidden dynamic router
- selection must be explainable from agent configuration and run phase
- escalation to a stronger lane must be explicit in replay and receipts

## Tool interface

```go
type Tool interface {
    Name() string
    Spec() ToolSpec
    Invoke(ctx context.Context, call ToolCall) (ToolResult, error)
}

type ToolSpec struct {
    Name            string
    Description     string
    InputSchemaJSON string
    Risk            ToolRisk
    SideEffect      SideEffectClass
    Approval        ApprovalMode
}
```

Decision:

- tools declare risk and approval requirements explicitly
- policy evaluation is separate from implementation

## Tool policy interface

```go
type ToolPolicy interface {
    Decide(ctx context.Context, agent AgentProfile, run RunProfile, tool ToolSpec) ToolDecision
}

type ToolDecision struct {
    Mode   DecisionMode // allow, ask, deny
    Reason string
}
```

Decision:

- one decision point
- no many-layer policy pipeline before the first stable release

Rules:

- every agent starts from one built-in tool profile
- operators may add narrow per-tool overrides without redefining the entire policy from scratch
- stable release should ship a small built-in profile catalog such as read-only, read-heavy, workspace-write, operator-facing, and elevated
- stable release should reserve real side effects for agents with explicit write or outbound capabilities
- most other roles should remain read-heavy or propose-only even if they participate deeply in reasoning

## Approval gate

```go
type ApprovalGate interface {
    Request(ctx context.Context, req ApprovalRequest) (ApprovalTicket, error)
    Resolve(ctx context.Context, ticketID string) (ApprovalDecision, error)
}
```

Rules:

- daemon persists approval requests
- each approval request must bind to one concrete action snapshot rather than a fuzzy permission scope
- the snapshot should include the tool name, normalized arguments, target scope, risk summary, and preview reference when available
- CLI and admin UI can resolve all approvals
- Telegram DM may resolve low and medium-risk approvals for the bound conversation
- high-risk approvals must resolve in the local web UI or CLI
- timeout defaults to deny
- approval tickets are single-use and expire if the proposed action changes materially before execution

## Team spec store

```go
type TeamSpecStore interface {
    List(ctx context.Context) ([]TeamSpecRef, error)
    Load(ctx context.Context, teamID string) (TeamSpec, error)
    Save(ctx context.Context, spec TeamSpec) (SaveResult, error)
}
```

Rules:

- file-backed team specs remain the canonical source
- CLI and local web UI edit the same format
- each team agent declares a validated capability posture plus editable identity and soul
- team specs may rename agents freely without changing runtime semantics
- saves must validate agent capabilities, delegation wiring, tool profiles, and memory scopes before writing
- saves must also validate allowed handoff and return edges between agents

## Concrete stable-release team and onboarding services

Keep these as concrete services inside `internal/agents`, `internal/runs`, `internal/replay`, and `internal/api`:

- starter-team resolution
- handoff validation
- starter-team loading
- first-run workspace binding
- first-run task suggestion
- preview package building
- post-preview guidance
- operator-surface resolution
- Telegram DM progress notification

Rules:

- the stable release ships one editable starter team plus a bounded capability model
- agent names are operator-defined; runtime semantics come from capabilities and validated handoff structure
- stable release uses explicit agent-to-agent handoff edges
- critique or return paths must be declared separately from forward delegation
- only agents with workspace-write capability own workspace mutation
- only agents marked operator-facing own outbound operator messaging
- most agents default to propose-only or read-heavy behavior
- the root run binds one workspace root and child runs inherit it
- the default team is the hero workflow, not a template gallery
- stable release first-run binds to a real workspace, not a synthetic demo repo
- the first guided run uses preview-only mode
- the shortlist should cover understanding, review, and safe-improvement work
- the runtime does not auto-pick and run a first task without user selection
- after a successful first local preview, the default next step is Telegram DM onboarding
- stable release Telegram DM uses one operator-facing agent alias plus rare milestone checkpoints only

## Workspace applier

```go
type WorkspaceApplier interface {
    Preview(ctx context.Context, runID string) (ChangePreview, error)
    Apply(ctx context.Context, runID string, approval ApprovalTicket) (ApplyResult, error)
}
```

Rules:

- apply is limited to the declared workspace root for that run
- apply requires a valid approval ticket for side effects
- preview and apply are both durable replay events
- the apply approval must bind to the exact preview snapshot being approved
- WorkspaceApplier is a concrete service inside internal/tools — it is the apply-gated boundary enforcement function for workspace mutations
- the run engine calls WorkspaceApplier.Apply after receiving a valid approval ticket from ApprovalGate
- WorkspaceApplier receives the workspace root as a parameter derived from the run's execution snapshot — it does not read the snapshot itself
- WorkspaceApplier is distinct from normal tool invocation: it is an apply checkpoint, not a tool in the agent's callable tool catalog

## Verification runner

```go
type VerificationRunner interface {
    Plan(ctx context.Context, runID string) (VerificationPlan, error)
    Execute(ctx context.Context, plan VerificationPlan) (VerificationReport, error)
}
```

Rules:

- run the most relevant checks, not every possible check
- verification evidence must be attached to replay and receipts
- when checks are skipped, the reason must be explicit

## Preview package builder

The preview package builder is a concrete service inside `internal/replay`. It is not defined as an interface because there is only one implementation and it does not need to be mocked in run-engine tests.

Rules:

- preview package assembly reads from the durable journal and projections only — it makes no model calls
- internal/replay owns preview package building; internal/api calls internal/replay to retrieve a built preview package
- if explanation generation (explain.go) requires model calls, it must move out of internal/replay into a separate presentation service with provider access — this must be decided before Task 7 starts
- preview packaging must not imply apply readiness unless a later explicit approval flow exists

## Memory store interface

```go
type MemoryStore interface {
    WriteFact(ctx context.Context, item MemoryItem) error
    UpdateFact(ctx context.Context, item MemoryItem) error
    ForgetFact(ctx context.Context, memoryID string) error
    Search(ctx context.Context, query MemoryQuery) ([]MemoryItem, error)
    SummarizeConversation(ctx context.Context, conversationID string) (SummaryRef, error)
}
```

Decision:

- one store for durable facts and summaries
- no separate vector interface before the first stable release

Rules:

- memory items must keep provenance and scope metadata visible
- user-facing edit and forget actions operate on the same store, not a shadow layer
- free-form model output does not write directly into durable memory
- local scope is the default durable-memory target unless policy explicitly publishes wider
- the memory store accepts WriteFact calls with any scope value but does not validate scope policy itself
- scope escalation from local to team scope must be authorized by the run engine before calling WriteFact — the run engine checks the agent's capability profile
- the memory store is a storage service; it does not interpret team topology or agent capabilities

## Concrete stable-release memory policies

Keep promotion and publish decisions as concrete services inside `internal/memory`.

Rules:

- auto-promotion is allowed only for narrow typed candidates
- ambiguous memory stays a proposal or is rejected
- candidates must include provenance, confidence, dedupe key, and explicit scope
- stable release uses local-by-default durable memory
- publish to team scope must be explicit and auditable
- replay and receipts should show when a fact was published beyond local scope
- human edits outrank model-promoted facts
- auto-promotion and scope publish authorization are enforced by the run engine and memory service layer, not by the WriteFact storage path
- the memory store WriteFact method does not trigger auto-promotion — promotion is a separate explicit call

## Conversation store interface

```go
type ConversationStore interface {
    Resolve(ctx context.Context, key ConversationKey) (Conversation, error)
    AppendEvent(ctx context.Context, evt Event) error
    ListEvents(ctx context.Context, conversationID string, limit int) ([]Event, error)
    ActiveRootRun(ctx context.Context, conversationID string) (RunRef, error)
}
```

Rules:

- conversation history is append-only at the database event-log level
- summaries are additional derived rows, not transcript rewrites
- conversation lookups may use projected indexes, but the journal remains the durable history spine
- the store must support one-active-root-run arbitration for each canonical conversation
- ConversationStore.AppendEvent is the single canonical journal append path for ALL event types: connector inbound, run lifecycle, tool calls, approvals, memory publishes, and delivery milestones
- the run engine holds a ConversationStore reference and calls AppendEvent for all lifecycle events — it does not write to store.DB directly for journal entries
- the single-transaction boundary rule is maintained by the ConversationStore implementation: AppendEvent appends the event and updates projections atomically in one store transaction
- no package other than ConversationStore may append to the events table directly

## Run engine interface

```go
type RunEngine interface {
    Start(ctx context.Context, cmd StartRun) (Run, error)
    Continue(ctx context.Context, cmd ContinueRun) (Run, error)
    Delegate(ctx context.Context, cmd DelegateRun) (Run, error)
    Resume(ctx context.Context, cmd ResumeRun) (Run, error)
    ReconcileInterrupted(ctx context.Context) (ReconcileReport, error)
}
```

Decision:

- one runtime for normal and delegated work
- no ACP-style second engine

### Delegation constraint (explicit)

An agent may only delegate to a target that is declared in the frozen execution snapshot's
team handoff edges. Delegation to any undeclared target must fail with a named error event
in the journal before creating a child run.

This constraint is structural, not advisory:
- agents cannot discover or select delegate targets dynamically at runtime
- the set of possible handoff targets is fixed at root-run start via the execution snapshot
- adding a new agent type requires a new team spec and a new root run
- this keeps handoff graphs auditable, replayable, and safe

Validation must happen inside `Delegate` before any child run is created. The validation
reads from the frozen execution snapshot, not from the live team.yaml on disk.

### Execution snapshot retention

The execution snapshot must include the full team YAML (verbatim or content-addressed)
stored as a DB row. This enables:
- replay to reconstruct handoff edges from the time of the run, even if team.yaml changes later
- the `Delegate` validator to read the declared edges without touching the filesystem
- future `gistclaw inspect team <run_id>` to show what team config was active

Retention: execution snapshots are kept for the lifetime of their run record. They are
not pruned on team spec edits.

Rules:

- run lifecycle transitions must emit journal events before they are considered durable
- delegation, interruption, completion, and budget stops must all be visible as first-class events
- daemon startup reconciles unfinished runs into explicit interrupted state
- stable release requires an explicit operator resume or rerun action after interruption
- the runtime must not silently auto-resume runs on process restart
- stable release allows concurrent child delegations within one root run, but not multiple competing root runs for the same conversation
- each root run has a small fixed active-child budget with explicit queueing and backpressure
- coordination logic may choose delegation order, but it may not bypass the root-run child budget
- a root run freezes an execution snapshot at start, including team wiring, soul, tool policy, and key config needed for the run
- child delegations inherit that same snapshot unless the operator starts a fresh root run
- a root run also owns one declared workspace root and child delegations may not escape it
- edits to team specs, soul, or tool policy apply to new root runs, not active ones
- the run engine emits all lifecycle events through ConversationStore.AppendEvent — it does not own a separate journal write path

## Replay reader

```go
type ReplayReader interface {
    LoadRun(ctx context.Context, runID string) (RunReplay, error)
    LoadGraph(ctx context.Context, rootRunID string) (ReplayGraph, error)
}
```

Rules:

- replay is built from durable run, event, tool, memory, and approval records
- replay does not scrape prompt text to guess what happened
- the replay surface is product scope, not debug garnish
- replay reads one durable journal plus projected state, not parallel peer histories

## Live replay stream

```go
type LiveReplayStream interface {
    Subscribe(ctx context.Context, runID string) (<-chan ReplayDelta, error)
}
```

Rules:

- stable release supports live replay in the local web UI
- external messaging surfaces do not receive internal live narration by default
- live replay is driven by the same durable event stream used for post-run inspection
- the SSE broadcaster implements both RunEventSink (for inbound event push from the runtime) and LiveReplayStream (for client subscription management)

## Run event sink

```go
type RunEventSink interface {
    Emit(ctx context.Context, runID string, evt ReplayDelta) error
}
```

Rules:

- the runtime accepts a RunEventSink at construction, not a concrete SSE broadcaster
- internal/runtime must never import internal/web or any transport-layer package
- the SSE broadcaster in internal/web implements RunEventSink
- the run engine calls Emit after each durable journal write — events are fan-out notifications, not the source of truth
- RunEventSink lives in internal/model so both internal/runtime and internal/web can import it without a cycle

## Later-phase sharing services

Replay export, team-card generation, and local-draft-from-share are later-phase concrete services.

Rules:

- they are not part of the stable-release core loop
- any future export must redact secrets and private memory by policy
- any future sharing surface should be built from replay data, not transcript scraping
- local draft generation is not equivalent to full bundle import

## Receipt reporter

```go
type ReceiptReporter interface {
    Build(ctx context.Context, rootRunID string) (RunReceipt, error)
    Compare(ctx context.Context, leftRunID string, rightRunID string) (ReceiptComparison, error)
}
```

Rules:

- every completed run gets a receipt
- receipts include token, cost, wall-clock, approvals, and budget status
- receipts include verification status when verification ran or was skipped
- receipts are computed from the same durable journal and projections used by replay
- comparison stays simple in the stable release: one run versus one run or one configuration versus one configuration
- baseline comparison starts only from an explicit operator action
- stable-release comparison may target a single-agent or smaller-team baseline, but never runs automatically

## Concrete stable-release presentation services

Keep completion formatting, activity snapshots, and grounded explanations as concrete services in `internal/replay` and `internal/api`.

Rules:

- stable-release external completion surfaces default to a compact DM-friendly completion card
- the stable-release DM surface should include final outcome, receipt summary, and replay access
- report active runs, scheduled work, pending approvals, and current model-call activity
- when the system is idle, the snapshot should say so explicitly
- explanations should answer "why did this happen?" from policy and event metadata without pretending to expose hidden chain-of-thought
- stable release explanations should cover delegation, memory loads, approvals, and budget stops first

## Budget guard

```go
type BudgetGuard interface {
    BeforeTurn(ctx context.Context, run RunProfile) error
    RecordUsage(ctx context.Context, runID string, usage UsageRecord) error
    CheckDailyCap(ctx context.Context, accountID string) error
    RecordIdleBurn(ctx context.Context, runID string, duration time.Duration) error
}
```

Rules:

- idle burn defaults to zero; BudgetGuard.RecordIdleBurn is called only if a run is in a waiting state with open model context
- budget exhaustion fails closed for all cap types: per-run, daily, and active-child
- CheckDailyCap checks rolling 24-hour usage aggregated from UsageRecord history; reset boundary is rolling, not UTC midnight
- cost and token accounting are durable and inspectable via the events journal
- active-child budget is NOT part of BudgetGuard — it is enforced inside the run engine's delegation queue logic; BudgetGuard does not coordinate concurrency
- conservative default caps are enabled before the operator customizes them
- raising or disabling caps requires an explicit operator action

## Concrete stable-release soul and scheduling services

Keep soul loading and scheduling as concrete services inside `internal/soul` and `internal/scheduler`.

Rules:

- soul is parsed and compiled before the model sees it
- stable release soul editing is structured, not raw-prompt-first
- the scheduler starts only declared jobs
- inbound event handlers and webhook handlers may enqueue runs through the same run engine
- no proactive background agent loop may create runs outside explicit triggers

## Interfaces intentionally omitted before stable release

- vector index interface
- plugin host interface
- remote node runtime interface
- general WS control-plane interface

If an interface is not needed to ship the first usable runtime, do not create it.
