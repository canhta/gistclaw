# System Diagrams

Start here if you want to understand GistClaw quickly.

These diagrams are the shortest path to the runtime shape, the work loop, and the trust model.

## 1. System Context

GistClaw is a single-writer local daemon with multiple operator surfaces.
The daemon owns state, orchestration, approvals, and replay.

```mermaid
flowchart LR
    User["Operator"] --> Web["Local web UI"]
    User --> CLI["CLI"]
    User -. "later" .-> Telegram["Telegram DM"]

    Web --> Ingress
    CLI --> Ingress
    Telegram --> Ingress

    subgraph Runtime["GistClaw daemon (gistclaw)"]
        Ingress["Command + connector ingress"]
        Engine["Run engine + team orchestration"]
        Gate["Approval gate"]
        Ingress --> Engine
        Engine --> Gate
    end

    Engine --> DB["SQLite journal + projections"]
    Engine --> Provider["Model provider"]
    Engine --> Tools["Repo tools"]
    Gate -. "side-effect approval" .-> Tools
    Gate --> DB
    Tools --> Workspace["One workspace root per root run"]
    DB --> Replay["Replay + receipts"]
    Replay --> Web
    Replay --> CLI
    Replay -. "later" .-> Telegram
```

## 2. Team Topology

GistClaw is not a single agent with internal moods.
It is an explicit team with a configurable agent graph, explicit handoffs, and bounded execution ownership.

```mermaid
flowchart LR
    Operator["Operator"] --> Surface["Operator-facing agent"]

    subgraph Team["Editable starter team"]
        Surface --> AgentA["Agent A"]
        AgentA --> AgentB["Agent B"]
        AgentA --> AgentC["Agent C"]
        AgentB <--> AgentC
    end

    AgentA --> Queue["Delegation queue<br/>when child budget is full"]
    AgentB --> Tools["Repo tools inside one workspace root"]
    AgentC --> Evidence["Checks and verification evidence"]
    Surface --> Receipt["Final reply, replay, and receipt"]
```

Notes:

- v1 ships one editable starter team, not a fixed role catalog as the product identity
- agent names stay operator-defined
- there is one active root run per conversation, with bounded child concurrency under that root
- at least one agent must own workspace-write capability
- at least one agent may be designated operator-facing

## 3. Provider And Model Lanes

GistClaw is multi-agent before it is multi-provider.
The first useful release keeps one configured provider adapter and two explicit lanes.

```mermaid
flowchart LR
    Input["Agent config + run phase"] --> Policy["Model selection policy"]
    Policy -->|routine work| Cheap["Cheap lane"]
    Policy -->|escalation or high-signal phase| Strong["Strong lane"]
    Cheap --> Adapter["Provider interface"]
    Strong --> Adapter
    Adapter --> Today["One configured provider adapter in v1"]
    Adapter -. "same seam can host more adapters later" .-> Later["Future adapters"]
```

Notes:

- lane selection is explainable from agent configuration and run phase
- the stronger lane is for escalations, synthesis, verification, and other high-signal phases
- the cheaper lane handles routine work by default
- any escalation to the stronger lane must be visible in replay and receipts

## 4. Runtime Workflow

The core product loop is not "chat with one model."
It is a bounded team workflow that turns a repo task into a reviewed result with an approval checkpoint before side effects.

```mermaid
sequenceDiagram
    actor Operator
    participant Surface as Operator-facing agent
    participant AgentA as Agent A
    participant AgentB as Agent B
    participant AgentC as Agent C
    participant Gate as Approval gate
    participant Repo as Workspace tools
    participant Journal as Journal

    Operator->>Surface: Submit repo task
    Surface->>Journal: Persist inbound event
    Surface->>AgentA: Start root run
    AgentA->>Journal: Persist run_started
    AgentA->>AgentB: Delegate implementation work
    AgentB->>Repo: Read repo and draft change
    AgentB->>Journal: Persist preview and tool events
    AgentB->>Gate: Request apply approval
    Gate->>Journal: Persist approval request
    Operator-->>Gate: Approve or deny
    Gate->>Journal: Persist approval decision

    alt Approved
        AgentB->>Repo: Apply patch inside workspace root
        AgentB->>Journal: Persist apply result
        AgentB->>AgentC: Request review and verification
        AgentC->>Repo: Inspect diff and run checks
        AgentC->>Journal: Persist verification evidence
        Surface->>Journal: Persist receipt and completion
        Surface-->>Operator: Final result with replay and receipt
    else Denied or expired
        Surface->>Journal: Persist blocked outcome
        Surface-->>Operator: Preview only or blocked result
    end
```

## 5. Durable Data Model

The journal is the history spine.
Everything the operator trusts later, including replay and receipts, is derived from the same durable event stream.

```mermaid
flowchart TB
    Writer["Single-writer daemon"] --> Events["events journal"]

    subgraph Database["SQLite (runtime.db)"]
        Events
        Runs["runs projection"]
        Delegations["delegations projection"]
        ToolCalls["tool_calls projection"]
        Approvals["approvals projection"]
        Receipts["receipts projection"]
        Memory["memory_items projection"]
        Summaries["run_summaries (compaction projection)"]
        Snapshots["execution_snapshots (team YAML, frozen at root-run start)"]
        Outbound["outbound_intents (durable delivery queue)"]
    end

    Events --> Runs
    Events --> Delegations
    Events --> ToolCalls
    Events --> Approvals
    Events --> Receipts
    Events --> Memory
    Events --> Summaries
    Runs --> Snapshots

    subgraph Surfaces["Read surfaces"]
        Replay["Replay timeline + tree"]
        Activity["Live activity view"]
        ReceiptView["Receipts"]
        Status["Run status + approvals"]
    end

    Events --> Replay
    Snapshots --> Replay
    Runs --> Activity
    Delegations --> Replay
    ToolCalls --> Replay
    Approvals --> Status
    Approvals --> Replay
    Receipts --> ReceiptView
    Memory --> Activity
    Outbound --> Status
```

## 6. Run Lifecycle

The lifecycle is intentionally boring.
There is one active root run per conversation, approvals are explicit, and restarts produce `interrupted` instead of magical recovery.

```mermaid
stateDiagram-v2
    [*] --> queued: inbound accepted
    queued --> running: root slot available
    queued --> canceled: operator cancel

    running --> waiting_approval: side effect needs approval
    waiting_approval --> running: approved
    waiting_approval --> blocked: denied or expired

    running --> verifying: preview or apply complete
    verifying --> completed: evidence recorded
    running --> blocked: budget stop or policy deny
    running --> failed: provider or tool failure

    running --> interrupted: daemon restart
    waiting_approval --> interrupted: daemon restart
    verifying --> interrupted: daemon restart

    interrupted --> queued: explicit resume or rerun
```

## What to read next

- `19-buildable-v1-plan.md` for the first shippable product loop
- `20-v1-implementation-backlog.md` for the concrete build order
- `11-architecture-redesign.md` for the full architectural stance
