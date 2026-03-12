# GistClaw — Revised Design Document (v3)

> A lightweight Go binary in the openclaw/picoclaw family.
> Controls OpenCode and Claude Code from Telegram, 24/7.
> Built by analysing openclaw, picoclaw, opencode v1.2.24, and claude-code.

> **Revision note (v3):** This document supersedes `design.md` (v2).
> Six structural improvements over v2:
> 1. `withRestart` is generic (no Notifier coupling); returns `PermanentFailure` sentinel;
>    supervision policy lives in `internal/app`.
> 2. `channel.Channel` interface clarified; `KeyboardPayload` is a channel-layer type;
>    `gateway.Service` is fully channel-agnostic.
> 3. `LLMProvider` contract tightened: `Usage.TotalCostUSD` required on all providers;
>    `cost.Provider` decorator wraps `LLMProvider` for unified plain-chat cost tracking;
>    OpenCode/ClaudeCode call `CostGuard.Track()` directly from their event streams.
> 4. Scheduler `Job.Target` is `agent.Kind` (typed enum in `internal/agent`); tool registry
>    has three explicit categories (Core / Agent / System).
> 5. `internal/app` owns construction + supervision + `Run`; `internal/infra` replaces
>    three micro-packages (`soul`, `heartbeat`, `cost`); `internal/agent` isolates enum.
> 6. Three public-facing docs added: `README.md`, `ARCHITECTURE.md`, `EXTENDING.md`.
> Go version: 1.25 (loop variable capture fixed; no `svc := svc` workarounds needed).

---

## Table of Contents

1. [What GistClaw Is](#1-what-gistclaw-is)
2. [Positioning in the Claw Family](#2-positioning-in-the-claw-family)
3. [Scope — v1 In / Out](#3-scope--v1-in--out)
4. [Architecture — Service Topology](#4-architecture--service-topology)
5. [Message Flow Examples](#5-message-flow-examples)
6. [Package Layout](#6-package-layout)
7. [Key Interface Contracts](#7-key-interface-contracts)
8. [Supervision Model](#8-supervision-model)
9. [Key Design Decisions (All Resolved)](#9-key-design-decisions-all-resolved)
10. [Dependency Budget](#10-dependency-budget)
11. [What Gets Built — Single Phase](#11-what-gets-built--single-phase)
12. [Behavior & Failure Modes](#12-behavior--failure-modes)
13. [Operations](#13-operations)
14. [Data, Privacy & Persistence](#14-data-privacy--persistence)
15. [Security & Configuration](#15-security--configuration)
16. [Deployment & DX](#16-deployment--dx)

---

## 1. What GistClaw Is

GistClaw is a single Go binary (plus a `gistclaw-hook` helper binary) that acts as a remote
controller for AI coding agents.
You run it on a VPS. You interact with it from your phone via Telegram.
It relays your tasks to OpenCode or Claude Code, streams responses back, and asks for
your approval when an agent wants to do something that needs a human decision.

Any non-command message (no `/oc`, `/cc`, `/stop`, `/status` prefix) is treated
as **plain chat**: routed to the core LLM with built-in tools available — `web_search`
(query → ranked results) and `web_fetch` (URL → readable text). The LLM decides freely
whether and when to call them. Multi-round tool loops are supported with a doom-loop guard.
Response is streamed back as one or more Telegram messages.

### Core LLM

GistClaw has a configurable core LLM used for all plain chat turns. Three backends are
supported in v1, tagged by stability:

- **`openai-key`** — default, stable/official; calls `api.openai.com/v1/chat/completions`
- **`copilot`** — advanced/experimental; via `github/copilot-sdk/go` gRPC to local bridge
  at `localhost:4321`
- **`codex-oauth`** — advanced/experimental; PKCE OAuth to `auth.openai.com`, calling
  `chatgpt.com/backend-api` _(intentional: provides a free-tier backend through chatgpt.com
  for cost savings)_

The active backend is selected by `LLM_PROVIDER` env var. All three implement the shared
`LLMProvider` interface. All three are compiled into the same binary — no build tags,
no profiles.

It is part of the claw family (openclaw → picoclaw → **gistclaw**) but adds three things
neither predecessor has:

- **Native OpenCode control** — via `opencode serve` REST + SSE API
- **Native Claude Code control** — via subprocess with a hook helper binary
- **Plain chat with web tools** — `web_search` (Brave/Gemini/Grok/Perplexity) + `web_fetch`
  (go-readability); LLM decides when to call each

The name reflects focus: gist = the essential point. Only what matters.

---

## 2. Positioning in the Claw Family

| | openclaw | picoclaw | **gistclaw** |
|---|---|---|---|
| Language | TypeScript | Go | **Go** |
| Binary size | ~1 GB | < 10 MB | **Lightweight Go binary¹** |
| Chat platforms | 15+ | 10+ | **Pluggable, Telegram in v1** |
| Core LLM | Copilot OAuth + Codex OAuth | Copilot gRPC + Codex OAuth | **openai-key (default) + copilot + codex-oauth** |
| Controls OpenCode | No | No | **Yes — HTTP + SSE** |
| Controls Claude Code | No | No | **Yes — subprocess + hook helper binary** |
| Plain chat w/ web search | No | No | **Yes — LLMProvider.Chat() + web_search + web_fetch tools; LLM decides; multi-round with doom-loop guard** |
| HITL | Full, complex | None | **Simple: approve / reject / 5-min timeout** |
| Actor model | No | No | **No — errgroup + withRestart** |
| State persistence² | No | No | **Yes — SQLite** |

> ¹ Binary size and runtime RSS have not been measured yet. Numbers will be added after
> the first successful build.
>
> ² "State persistence" means completed session history survives in SQLite across restarts.
> Active tasks at restart time are **lost**; pending HITL is auto-rejected. This is cleanup,
> not full session recovery.

---

## 3. Scope — v1 In / Out

### In scope

| Feature | Notes |
|---|---|
| Telegram gateway | Long-poll. Pluggable `channel.Channel` interface so other platforms can be added without gateway changes. |
| OpenCode service | Controls `opencode serve` via HTTP + SSE |
| Claude Code service | Controls `claude -p` subprocess + hook helper binary |
| HITL engine | Approve / reject / 5-min timeout. Simple, not openclaw-level complex. |
| Plain chat | Any non-command message → LLMProvider.Chat() with `web_search` + `web_fetch` tools available; LLM decides; doom-loop guard; stateless per turn. |
| `/stop` command | Abort active session on OpenCode or ClaudeCode. |
| `question.asked` handling | Separate from `permission.asked`; sequential per question; single reply with all answers. |
| MCP client | GistClaw-owned `MCPManager`; stdio + SSE/HTTP transports; user-configured servers; tools available in plain chat |
| Scheduler | `scheduler.Service`; job kinds: `at` / `every` / `cron`; targets: `agent.Kind` enum; LLM creates/manages jobs via tools |
| SOUL.md injection | mtime-cached load; injected as `system` field on OpenCode prompt_async; as `--system-prompt` flag on ClaudeCode. |
| Heartbeat Tier 1 + 2 | Tier 1: Telegram liveness (3 retries then crash). Tier 2: agent health check + auto-restart. |
| Cost guard | Soft-stop: notify at 80% and 100%; current session finishes cleanly. Tracking via `infra.CostGuard`. |
| SQLite state store | Sessions, pending HITL, daily cost, last Telegram update_id |
| `openai-key` provider | Calls `api.openai.com/v1/chat/completions`; default and stable |
| `copilot` provider | Via `github/copilot-sdk/go` gRPC to local bridge at `localhost:4321`; advanced/experimental |
| `codex-oauth` provider | PKCE to `auth.openai.com`, calls `chatgpt.com/backend-api`; token stored in SQLite; advanced/experimental |
| Session auto-purge | On startup: DELETE sessions older than 24h |
| Graceful shutdown | On SIGTERM: cancel root context → services stop → drain (10s window) |
| systemd service | `Restart=always`, `StartLimitBurst=5` |
| Telegram message splitting | Hard-split at 4096 chars when accumulated output exceeds Telegram limit |

### Explicitly out of scope for v1

| Feature | Reason |
|---|---|
| Heartbeat Tier 3 (goal drift) | Requires persistent goal state; post-v1 |
| SOUL.md checkpoint cron | Low priority; post-v1 |
| Brave Search as OpenCode/ClaudeCode tool | Those agents manage their own tool registries; not injectable |
| Skills system | Separate feature |
| Multi-agent orchestration | Out of scope |
| Additional chat channels (Discord, Slack…) | Interface is ready; implementations are post-v1 |
| `/history` and `/cleanup` commands | Post-v1 maintenance features |
| Conversation history for random chat | Stateless in v1; post-v1 feature |
| MCP auto-reconnect | Restart binary to recover failed MCP servers; post-v1 |

---

## 4. Architecture — Service Topology

Five services run as goroutines under `golang.org/x/sync/errgroup`. Everything else is
plain Go called synchronously. No actor framework, no protobuf.

```
systemd (Restart=always, StartLimitBurst=5)
    └── gistclaw binary
            │
      errgroup (root context)   ← owned by internal/app.App.Run()
            │
      ┌─────┴──────────────────────────────────────────┐
      │                                                 │
  withRestart("gateway", unlimited)        withRestart("hitl", 10, 10s)
  - channel.Channel interface              - permission + question HITL
  - v1: Telegram long-poll impl            - per-request channel pattern
  - update_id dedup (SQLite)               - 5-min timer + 3-min reminder
  - /oc /cc /status /stop routing          - sequential per-question
  - plain chat → trackedLLM               - auto-reject stale on startup
    (web_search + web_fetch
     + MCP tools [Agent category]
     + scheduler tools [System category])
        │ injected interfaces
        │ (channel.Channel, hitl.Approver,
        │  opencode.Service, claudecode.Service,
        │  providers.LLMProvider [decorated])
        └────────────────────────────────────────────────┐
                                                         │
                              ┌──────────────────────────┤
                              │                          │
               withRestart("opencode", 5, 30s) withRestart("claudecode", 5, 30s)
               - start opencode serve           - start claude -p subprocess
               - HTTP client                    - stream-json parser
               - SSE consumer                   - FSM: Idle→Running
               - session lifecycle                →WaitingInput→Idle
               - no FSM needed                  - hook helper: gistclaw-hook
               - CostGuard.Track() on           - HTTP server :8765
                 step-finish events             - CostGuard.Track() on
                                                  total_cost_usd events
                              │
                  withRestart("scheduler", 5, 30s)
                  - 1s ticker
                  - job kinds: at/every/cron
                  - targets: agent.Kind enum
                  - SQLite jobs table
                  - fires via scheduler.JobTarget interface
                              │
               ┌──────────────┴─────────────────────┐
           SQLiteStore                          internal/infra
           (not a service loop)                 - CostGuard (atomic.Int64)
           - sessions                           - SOULLoader (mtime cache)
           - hitl_pending                       - Heartbeat (Tier 1+2)
           - cost_daily                         internal/providers
           - channel_state¹                     - LLMProvider interface
           - jobs                               - trackingProvider decorator
                                                - copilot / codex / openai impls
                                                internal/tools
                                                - SearchProvider (Brave/Gemini/
                                                  Grok/Perplexity — auto-detect)
                                                - WebFetcher (go-readability;
                                                  5 MB / 30s limits)
                                                internal/mcp
                                                - MCPManager (stdio + SSE/HTTP;
                                                  user-configured servers)

¹ channel_state is keyed by channel ID so each channel implementation stores its own
  dedup cursor (e.g. last_update_id for Telegram) without schema changes.
```

### Why errgroup + withRestart instead of protoactor-go

Each service has a `Run(ctx context.Context) error` method. The `withRestart` wrapper
owns the restart loop; `Run` owns internal goroutines. This mirrors picoclaw's approach
exactly — picoclaw uses a plain `MessageBus` (typed channels + direct service calls)
and errgroup-style loops, not actors.

Benefits over protoactor-go:
- No external actor framework dependency (removes ~1 MB binary bloat, no CGo transitive deps)
- No protobuf schema for internal messages — plain Go structs, compiler-checked
- Simpler supervision: `withRestart` is ~40 lines of Go, fully readable
- No mailbox abstraction: services call each other via injected interfaces directly
- Easier testing: inject mocks via interfaces, no actor system bootstrap required

### ClaudeCode hook architecture — helper binary pattern

Claude Code hooks work via **stdin/stdout process invocation**, not HTTP:
- Claude Code spawns the registered hook command process
- Writes hook event JSON to the process's **stdin**
- Reads the response from **stdout** (allow) or **stderr** (deny, exit code 2)

To bridge this to HITL over Telegram, gistclaw ships a second binary: `gistclaw-hook`.

```
Claude Code
  └── spawns gistclaw-hook process
        │
        ├── reads PreToolUse JSON from stdin
        ├── POSTs to http://127.0.0.1:8765/hook/pretool (gistclaw's HTTP server)
        ├── blocks waiting for HTTP response
        └── on response:
              Allow → writes {"hookSpecificOutput":{"permissionDecision":"allow"}} to stdout, exit 0
              Deny  → writes {"hookSpecificOutput":{"permissionDecision":"deny"},"systemMessage":"..."} to stderr, exit 2
```

The HTTP server at `:8765` is owned by `claudecode.Service`. It blocks per-request using a channel:

```go
1. gistclaw-hook POSTs /hook/pretool → claudecode's hook handler goroutine starts
2. Handler creates a per-request channel: decisionCh := make(chan HITLDecision, 1)
3. Handler calls hitl.Approver.RequestPermission(ctx, PermissionRequest{ChatID: ..., decisionCh: decisionCh, ...})
4. Handler blocks: select { case d := <-decisionCh: ...; case <-time.After(tuning.HITLTimeout): ... }
5. hitl.Service resolves → sends HITLDecision back on decisionCh
6. Handler writes JSON response, closes HTTP connection
7. gistclaw-hook receives response, writes allow/deny to stdout/stderr, exits
```

### Dead message handling (no mailbox)

Because there is no actor mailbox, there is no "dead letter" path. If a service is
temporarily stopped and restarting, callers block on the call or receive an error via
the returned interface method. The caller handles the error: log `WARN`, notify Telegram,
and drop the request. The `withRestart` loop brings the service back; the next request
will succeed.

---

## 5. Message Flow Examples

### Flow A — User sends `/oc build the auth module`

```
1. Telegram update arrives at gateway.Service
2. update_id checked against SQLite channel_state.last_update_id
   - if update_id ≤ last_seen → drop (duplicate)
   - else → update SQLite, continue
3. Command prefix "/oc" → call opencode.Service.SubmitTask(ctx, chatID, "build the auth module")
4. opencode.Service checks session:
   - if no active session → POST /session (no body needed for basic session)
   - if active session → reuse
5. POST /session/:id/prompt_async {parts:[{type:"text",text:"build the auth module"}], system: <SOUL.md content>}
   → returns 204 immediately
6. SSE watcher receives message.part.updated events (part.type == "text")
   → accumulate text chunks; split and forward via channel.Channel.SendMessage at 4096-char boundary
7. CostGuard.Track() on each step-finish part event
   - if spend ≥ 80% daily limit → channel.SendMessage(chatID, "⚠️ 80% of daily cost used ($X.XX / $Y.YY)")
8. SSE receives session.status {status:{type:"idle"}} → channel.SendMessage(chatID, "✅ Done")
```

### Flow B — OpenCode asks for permission mid-task

```
1. SSE fires permission.asked {id:"permission_<ulid>", sessionID, permission:"edit", patterns:["/path/to/file"], ...}
2. opencode.Service calls hitl.Approver.RequestPermission(ctx, PermissionRequest{
       ChatID: chatID, ID: "permission_<ulid>", ...
   })
3. hitl.Service writes to SQLite hitl_pending {id, status:pending, agent:opencode, ts:now}
4. hitl.Service sends keyboard via channel.Channel.SendKeyboard(ctx, chatID, KeyboardPayload{...}):
   "edit /path/to/file
    [✅ Once]  [✅ Always]  [❌ Reject]  [⏹ Stop]"
5. Timer starts: HITLReminderBefore before timeout → reminder "⏰ Approval pending"
               HITLTimeout → auto-reject
6. User taps ✅ Once:
   - Telegram callback → gateway.Service → hitl.Service
   - hitl.Service POST /permission/permission_<ulid>/reply {"reply":"once"}
   - SQLite hitl_pending updated {status:resolved}
   - Timer cancelled
7. Agent continues; SSE resumes streaming
```

### Flow C — Claude Code tool approval via hook helper

```
1. claude subprocess running; PreToolUse fires
2. Claude Code spawns gistclaw-hook process, writes JSON to its stdin
3. gistclaw-hook reads stdin, POSTs to http://127.0.0.1:8765/hook/pretool
4. claudecode.Service hook handler goroutine receives request
5. Creates: decisionCh := make(chan HITLDecision, 1)
6. Calls hitl.Approver.RequestPermission(ctx, PermissionRequest{ChatID: chatID, decisionCh: decisionCh, ...})
7. Handler blocks on: select { case d := <-decisionCh: ...; case <-time.After(tuning.HITLTimeout): auto-deny }
8. Same HITL flow as Flow B (keyboard, reminder, auto-reject)
9. User replies → hitl.Service sends HITLDecision on decisionCh
10. Hook handler writes JSON response to gistclaw-hook
11. gistclaw-hook writes allow/deny to stdout/stderr, exits
12. Claude Code continues or aborts tool call
```

### Flow D — Zero output from agent

```
1. OpenCode session goes idle with zero text parts received
2. opencode.Service detects: session.status.type = "idle" AND outputBuffer is empty
3. channel.SendMessage(chatID, "⚠️ Agent finished but produced no output. Check the session or resend your prompt.")
4. No auto-retry. No crash. User decides what to do next.
```

### Flow E — Plain chat with multi-tool loop: `what is the latest Go version?`

```
1. User sends "what is the latest Go version?" (no command prefix)
2. gateway.Service detects no command prefix
3. channel.SendTyping(ctx, chatID)
4. gateway.Service builds tool registry:
   - Core: web_search, web_fetch
   - Agent: MCP-derived tools (namespaced {server}__{tool})
   - System: schedule_job, list_jobs, update_job, delete_job
5. gateway.Service enters tool loop (doom-loop guard: 3 identical calls → forced final answer):

   Iteration 1:
   - Call trackedLLM.Chat(ctx, messages, tools)   ← decorated; cost tracked automatically
   - LLM returns tool_call: web_search({query:"latest Go release 2026", count:5})
   - GistClaw calls active SearchProvider
   - Results appended as tool_result

   Iteration 2:
   - LLM returns tool_call: web_fetch({url:"https://go.dev/dl/", format:"markdown"})
   - GistClaw: GET https://go.dev/dl/ → go-readability → markdown
   - Content appended as tool_result (truncated to 50 KB / 2000 lines)

   Iteration 3:
   - LLM returns final text answer (no tool calls) → loop exits

6. channel.SendMessage(chatID, answer)
   (CostGuard.Track() called automatically by trackingProvider decorator on each Chat() call)
```

### Flow F — Plain chat without search: `what's a good name for a Go error wrapping helper?`

```
1. gateway.Service detects no command prefix
2. channel.SendTyping(ctx, chatID)
3. Calls trackedLLM.Chat(ctx, messages, tools)
4. LLM decides NOT to call any tool — answers directly
5. channel.SendMessage(chatID, answer)
   (CostGuard.Track() called automatically by decorator)
```

### Flow G — question.asked with multiple questions

```
1. SSE fires question.asked {id:"question_<ulid>", sessionID,
   questions:[
     {question:"Which test framework?", header:"Test", options:[{label:"testify",...},...], multiple:false, custom:true},
     {question:"Enable coverage?",      header:"Cover", options:[{label:"yes",...},{label:"no",...}], multiple:false, custom:false}
   ]}
2. opencode.Service calls hitl.Approver.RequestQuestion(ctx, QuestionRequest{ChatID: chatID, ...})
3. hitl.Service processes questions sequentially:
   a. Sends questions[0] keyboard (channel.KeyboardPayload) to Telegram, waits for user reply
      (custom:true → also shows "Type your own" button)
   b. Receives answer, stores ["testify"]
   c. Sends questions[1] keyboard, waits
   d. Receives answer, stores ["yes"]
4. hitl.Service sends single reply: POST /question/question_<ulid>/reply
   {"answers": [["testify"], ["yes"]]}
5. Agent continues with both answers
```

---

## 6. Package Layout

```
gistclaw/
├── cmd/
│   ├── gistclaw/
│   │   └── main.go              # ~25 lines: config.Load → app.NewApp → app.Run; SIGTERM context
│   └── gistclaw-hook/
│       └── main.go              # Hook helper: read stdin → POST :8765 → write stdout/stderr + exit
│
├── go.mod                       # module github.com/canhta/gistclaw; go 1.25
│
├── internal/
│   ├── app/
│   │   ├── app.go               # App struct; NewApp(cfg Config) (*App, error); Run(ctx) error
│   │   │                        # Owns: full dependency graph construction, errgroup,
│   │   │                        # supervision policy (which services are critical vs non-critical)
│   │   └── supervisor.go        # withRestart, PermanentFailure sentinel, restartDelay
│   │
│   ├── agent/
│   │   └── kind.go              # AgentKind type; KindOpenCode, KindClaudeCode, KindChat constants
│   │                            # String() and SQLite scan helper
│   │
│   ├── config/
│   │   └── config.go            # Env vars + YAML config, validation
│   │                            # TELEGRAM_TOKEN, OPENCODE_DIR, CLAUDE_DIR,
│   │                            # ALLOWED_USER_IDS (required; fail at startup if empty),
│   │                            # DAILY_LIMIT_USD, LLM_PROVIDER, search API keys,
│   │                            # OPENCODE_PORT (default 8766), COPILOT_GRPC_ADDR,
│   │                            # OperatorChatID() int64 → AllowedUserIDs[0]
│   │                            # Tuning struct (all timeouts and TTLs)
│   │
│   ├── gateway/
│   │   └── service.go           # channel-agnostic controller: reads InboundMessage from
│   │                            # channel.Channel; command routing (/oc /cc /stop /status);
│   │                            # plain chat tool loop (buildToolRegistry); callback dispatch
│   │
│   ├── channel/
│   │   ├── channel.go           # Channel interface; InboundMessage; OutboundMessage
│   │   │                        # KeyboardPayload{Text string; Rows []ButtonRow}
│   │   │                        # ButtonRow = []Button{Label, CallbackData string}
│   │   └── telegram/
│   │       └── telegram.go      # Long-poll impl, update_id dedup (SQLite channel_state),
│   │                            # message splitting at 4096 chars, KeyboardPayload→telego types
│   │
│   ├── opencode/
│   │   ├── service.go           # Run(ctx): start opencode serve + SSE consumer
│   │   │                        # CostGuard.Track() on step-finish events
│   │   ├── config.go            # opencode-specific config
│   │   └── stream.go            # SSE event parsing + dispatch
│   │
│   ├── claudecode/
│   │   ├── service.go           # Run(ctx): spawn claude subprocess, FSM
│   │   │                        # CostGuard.Track() on total_cost_usd result events
│   │   ├── hooksrv.go           # HTTP server :8765 — blocking hook handler
│   │   ├── config.go            # claudecode-specific config
│   │   └── stream.go            # Parse stream-json lines → output chunks
│   │
│   ├── hitl/
│   │   ├── service.go           # Run(ctx): register → notify → wait → reply
│   │   ├── types.go             # PermissionRequest, QuestionRequest, HITLDecision
│   │   └── keyboard.go          # Build channel.KeyboardPayload (no telego imports)
│   │
│   ├── scheduler/
│   │   └── service.go           # Run(ctx): 1s ticker; job dispatch via JobTarget
│   │                            # Job.Target is agent.Kind (not string)
│   │                            # Job kinds: at / every / cron (gronx)
│   │                            # Tools(): returns []providers.Tool for plain chat registry
│   │
│   ├── providers/
│   │   ├── llm.go               # LLMProvider interface; Message, Tool, ToolCall types
│   │   │                        # Usage{PromptTokens, CompletionTokens, TotalCostUSD}
│   │   │                        # LLMResponse{Content, ToolCall, Usage}
│   │   │                        # New(cfg) LLMProvider — factory by LLM_PROVIDER
│   │   ├── tracking.go          # NewTrackingProvider(LLMProvider, *infra.CostGuard) LLMProvider
│   │   │                        # decorator: wraps Chat(); calls guard.Track(resp.Usage.TotalCostUSD)
│   │   ├── copilot/
│   │   │   └── copilot.go       # GitHub Copilot gRPC; Usage.TotalCostUSD = 0 (opaque billing)
│   │   ├── codex/
│   │   │   └── codex.go         # Codex OAuth PKCE + chatgpt.com/backend-api; token in SQLite
│   │   │                        # Usage.TotalCostUSD = 0 (unofficial backend; billing opaque)
│   │   └── openai/
│   │       └── openai.go        # OpenAI API key; Usage.TotalCostUSD computed from token counts
│   │
│   ├── infra/
│   │   ├── cost.go              # CostGuard: atomic.Int64 counter; Track(usd float64);
│   │   │                        # notify at 80%+100%; reset via SQLite date check
│   │   ├── heartbeat.go         # Tier1: Telegram getMe (3 retries, then crash)
│   │   │                        # Tier2: AgentHealthChecker.IsAlive() every 5 min; restart if dead
│   │   │                        # AgentHealthChecker interface
│   │   └── soul.go              # SOULLoader: mtime cache; reload only when file changes
│   │
│   ├── tools/
│   │   ├── search.go            # SearchProvider interface + auto-detect factory + backends
│   │   │                        # (Brave, Gemini, Grok, Perplexity)
│   │   └── fetch.go             # WebFetcher interface + implementation (go-readability)
│   │                            # Chrome UA, 5 MB limit, 30s timeout, Cloudflare retry
│   │
│   ├── mcp/
│   │   ├── manager.go           # MCPManager: Connect, CallTool, GetAllTools, Close
│   │   └── config.go            # MCPServerConfig: Command/Args/Env/EnvFile or URL/Headers
│   │
│   └── store/
│       ├── sqlite.go            # SQLite client, WAL mode, startup purge
│       └── schema.sql           # sessions, hitl_pending, cost_daily, channel_state,
│                                # provider_credentials, jobs
│
├── README.md                    # Quickstart: what it is, install, minimal .env, run
├── ARCHITECTURE.md              # Service topology, main flows, interface contracts
├── EXTENDING.md                 # How to add: new channel, new LLM provider, new agent
└── Makefile                     # build, run, deploy, logs, test
```

---

## 7. Key Interface Contracts

### channel.Channel

```go
// internal/channel/channel.go

type InboundMessage struct {
    ID       string
    ChatID   int64
    UserID   int64
    Text     string
    // CallbackData is non-empty for inline keyboard button presses
    CallbackData string
}

type Button struct {
    Label        string
    CallbackData string
}

type ButtonRow []Button

// KeyboardPayload is a platform-agnostic keyboard definition.
// hitl constructs this type; channel adapters translate it to platform types.
type KeyboardPayload struct {
    Text string      // message text accompanying the keyboard
    Rows []ButtonRow
}

type Channel interface {
    // Receive returns a channel of inbound messages. Runs until ctx is cancelled.
    // For v1: returns (<-chan InboundMessage, error).
    // Post-v1 consideration: split into Start(ctx) error + Receive() <-chan InboundMessage
    // if startup errors need to be separated from runtime errors.
    Receive(ctx context.Context) (<-chan InboundMessage, error)
    SendMessage(ctx context.Context, chatID int64, text string) error
    SendKeyboard(ctx context.Context, chatID int64, payload KeyboardPayload) error
    SendTyping(ctx context.Context, chatID int64) error
    Name() string // "telegram", "whatsapp", etc.
}
```

`gateway.Service` only imports `channel.Channel`. The `telegram` adapter translates
`KeyboardPayload` to `telego.InlineKeyboardMarkup`. `hitl/keyboard.go` only imports
`internal/channel` — no `telego` dependency in `hitl`.

### providers.LLMProvider

```go
// internal/providers/llm.go

// Usage represents token consumption for a single Chat call.
// All providers must populate TotalCostUSD.
// Providers that cannot compute exact cost (copilot, codex-oauth — billing is opaque
// on unofficial/gRPC backends) return 0.
// The trackingProvider decorator always calls CostGuard.Track(); a zero value is a
// valid no-op — it does not trigger soft-stop thresholds.
type Usage struct {
    PromptTokens     int
    CompletionTokens int
    TotalCostUSD     float64 // required; 0 if provider cannot determine cost
}

type LLMResponse struct {
    Content  string
    ToolCall *ToolCall // nil if no tool call
    Usage    Usage     // always populated; never nil
}

type LLMProvider interface {
    Chat(ctx context.Context, messages []Message, tools []Tool) (*LLMResponse, error)
    Name() string
}
```

### providers.trackingProvider (decorator)

```go
// internal/providers/tracking.go

// NewTrackingProvider wraps any LLMProvider and calls CostGuard.Track on every
// successful Chat response. All plain-chat LLM calls go through this decorator.
// OpenCode and ClaudeCode call CostGuard.Track directly from their event streams.
func NewTrackingProvider(inner LLMProvider, guard *infra.CostGuard) LLMProvider

// app.NewApp wires this once:
//   rawLLM    := providers.New(cfg)
//   trackedLLM := providers.NewTrackingProvider(rawLLM, costGuard)
// trackedLLM is injected into gateway.Service and scheduler.Service.
// Neither gateway nor scheduler imports any concrete provider package.
```

### hitl.Approver

```go
// internal/hitl/types.go
type PermissionRequest struct {
    ChatID     int64
    ID         string              // "permission_<ulid>"
    SessionID  string
    Permission string              // "edit", "run", etc.
    Patterns   []string
    DecisionCh chan<- HITLDecision
}

type QuestionRequest struct {
    ChatID    int64
    ID        string              // "question_<ulid>"
    SessionID string
    Questions []Question
}

type HITLDecision struct {
    Allow  bool
    Always bool
}

// internal/hitl/service.go
type Approver interface {
    RequestPermission(ctx context.Context, req PermissionRequest) error
    RequestQuestion(ctx context.Context, req QuestionRequest) error
}
```

### opencode.Service and claudecode.Service

```go
// internal/opencode/service.go
type Service interface {
    Run(ctx context.Context) error
    SubmitTask(ctx context.Context, chatID int64, prompt string) error
    Stop(ctx context.Context) error
    IsAlive(ctx context.Context) bool
}

// internal/claudecode/service.go
type Service interface {
    Run(ctx context.Context) error
    SubmitTask(ctx context.Context, chatID int64, prompt string) error
    Stop(ctx context.Context) error
    IsAlive(ctx context.Context) bool
}
```

### scheduler.JobTarget

```go
// internal/scheduler/service.go
type JobTarget interface {
    RunAgentTask(ctx context.Context, kind agent.Kind, prompt string) error
    SendChat(ctx context.Context, chatID int64, text string) error
}
```

`JobTarget` is transport-agnostic. The scheduler does not import `gateway` — `app.NewApp`
wires the dependency. `agent.Kind` is the typed enum; no stringly-typed agent identifiers.

### agent.Kind

```go
// internal/agent/kind.go
package agent

type Kind int

const (
    KindOpenCode   Kind = iota
    KindClaudeCode
    KindChat
)

func (k Kind) String() string { /* "opencode" | "claudecode" | "chat" */ }

// KindFromString is used when scanning from SQLite jobs.target column.
func KindFromString(s string) (Kind, error)
```

SQLite `jobs.target` column stores the string representation. Adding a new agent kind
requires one new constant and one new case in `KindFromString` — nothing else.

### Tuning

```go
// internal/config/config.go
type Tuning struct {
    HITLTimeout          time.Duration // default: 5m
    HITLReminderBefore   time.Duration // default: 2m
    WebSearchTimeout     time.Duration // default: 10s
    WebFetchTimeout      time.Duration // default: 30s
    SessionTTL           time.Duration // default: 24h (startup purge cutoff)
    CostHistoryTTL       time.Duration // default: 30d (cost_daily purge cutoff)
    HeartbeatTier1Every  time.Duration // default: 30s
    HeartbeatTier2Every  time.Duration // default: 5m
    SchedulerTick        time.Duration // default: 1s
    MissedJobsFireLimit  int           // default: 5 (max immediate fires on startup)
}
```

All timeouts and TTLs in one place. No magic constants scattered across packages.

---

## 8. Supervision Model

### withRestart — generic, no Notifier coupling

```go
// internal/app/supervisor.go

// PermanentFailure is returned by withRestart when a service exhausts its restart budget.
// The caller (app.Run) decides what to do: cancel root context (critical services)
// or notify the operator and continue (non-critical services).
type PermanentFailure struct {
    Name string
    Err  error
}

func (e PermanentFailure) Error() string {
    return fmt.Sprintf("service %s permanently failed: %v", e.Name, e.Err)
}

// withRestart wraps fn in a restart loop.
// maxAttempts=0 means unlimited restarts (used for gateway.Service).
// Returns:
//   nil            — clean shutdown (ctx cancelled) or clean service exit (fn returned nil)
//   PermanentFailure — fn failed maxAttempts times within window
//   other error    — BUG: fn leaked an unexpected error; callers log and treat as permanent
func withRestart(name string, maxAttempts int, window time.Duration, fn func(context.Context) error) func(context.Context) error {
    return func(ctx context.Context) error {
        attempts := 0
        windowStart := time.Now()
        var lastErr error
        for {
            err := fn(ctx)
            if ctx.Err() != nil {
                return nil // clean shutdown
            }
            if err == nil {
                return nil // clean exit
            }
            lastErr = err
            now := time.Now()
            if now.Sub(windowStart) > window {
                attempts = 0
                windowStart = now
            }
            attempts++
            if maxAttempts > 0 && attempts >= maxAttempts {
                return PermanentFailure{Name: name, Err: lastErr}
            }
            log.Warn().Str("service", name).Err(err).Int("attempt", attempts).Msg("restarting service")
            time.Sleep(restartDelay(attempts)) // exponential backoff, cap 30s
        }
    }
}
```

### Supervision policy — owned by app.Run

`app.Run` classifies services as critical or non-critical. The policy is explicit and
readable in one place.

```go
// internal/app/app.go

func (a *App) Run(ctx context.Context) error {
    eg, ctx := errgroup.WithContext(ctx)

    // Critical: gateway — permanent failure cancels root context → systemd restarts all
    eg.Go(func(ctx context.Context) error {
        err := withRestart("gateway", 0, 0, a.gateway.Run)(ctx)
        var pf PermanentFailure
        if errors.As(err, &pf) {
            // unlimited retries: this path is unreachable in normal operation
            log.Error().Str("service", pf.Name).Err(pf.Err).Msg("gateway permanently failed")
            return pf // propagates → errgroup cancels root context → systemd restarts
        }
        if err != nil {
            log.Error().Str("service", "gateway").Err(err).Msg("BUG: unexpected error from withRestart")
            return err
        }
        return nil
    })

    // Non-critical: log + notify operator on permanent failure; other services continue
    type nonCritical struct {
        name        string
        maxAttempts int
        window      time.Duration
        fn          func(context.Context) error
    }
    services := []nonCritical{
        {"opencode",   5,  30 * time.Second, a.opencode.Run},
        {"claudecode", 5,  30 * time.Second, a.claudecode.Run},
        {"hitl",       10, 10 * time.Second, a.hitl.Run},
        {"scheduler",  5,  30 * time.Second, a.scheduler.Run},
    }
    for _, svc := range services { // Go 1.22+: loop variable captured correctly per iteration
        eg.Go(func(ctx context.Context) error {
            err := withRestart(svc.name, svc.maxAttempts, svc.window, svc.fn)(ctx)
            var pf PermanentFailure
            if errors.As(err, &pf) {
                log.Error().Str("service", pf.Name).Err(pf.Err).Msg("permanently stopped")
                _ = a.channel.SendMessage(ctx, a.cfg.OperatorChatID(), formatDegradedMsg(pf.Name))
                return nil // do not cancel root context
            }
            if err != nil {
                log.Error().Str("service", svc.name).Err(err).Msg("BUG: unexpected error from withRestart")
            }
            return nil
        })
    }

    return eg.Wait()
}
```

**`cfg.OperatorChatID()`** returns `cfg.AllowedUserIDs[0]` — guaranteed non-empty by startup
validation. No `OPERATOR_CHAT_ID` env var needed; for a single-user VPS bot, operator ==
allowed user.

### Supervision strategies

| Service | Max attempts | Window | On permanent stop |
|---|---|---|---|
| `gateway.Service` | unlimited | — | Returns `PermanentFailure` → errgroup cancels root context → systemd restarts all |
| `opencode.Service` | 5 | 30s | Log + notify operator: `"⚠️ OpenCode is unavailable. Use /stop then try /oc again."` |
| `claudecode.Service` | 5 | 30s | Log + notify operator: `"⚠️ Claude Code is unavailable. Use /stop then try /cc again."` |
| `hitl.Service` | 10 | 10s | `DrainPending()` first; then log + notify: `"⚠️ HITL unavailable. Approval requests will be auto-rejected."` |
| `scheduler.Service` | 5 | 30s | Log + notify: `"⚠️ Scheduler unavailable. Scheduled jobs are paused."` Jobs in SQLite resume on restart. |

### In-flight HITL on hitl.Service stop

When `hitl.Service` permanently stops, `DrainPending()` is called before the operator
notification is sent:

```go
// hitl.Service tracks in-flight requests in a sync.Map
func (s *Service) DrainPending() {
    s.pending.Range(func(key, val any) bool {
        req := val.(pendingRequest)
        req.DecisionCh <- HITLDecision{Allow: false}
        return true
    })
}
```

This ensures `claudecode.Service` hook handler goroutines unblock with an auto-deny
rather than hanging until `HITLTimeout`.

---

## 9. Key Design Decisions (All Resolved)

### 9.1 No actor framework — errgroup + withRestart only

The five services each have their own lifecycle (start, crash, restart independently).
Achieved with:
- `errgroup.WithContext` for concurrent startup/shutdown
- `withRestart` wrapper for per-service restart loops (in `internal/app/supervisor.go`)
- Plain Go interfaces for inter-service communication
- `internal/app` owns the supervision policy (critical vs non-critical classification)

protoactor-go is removed. Reasons: picoclaw does not use it; actor supervision adds ~500
lines for behavior achievable in ~40 lines; protobuf for internal messages removes
compile-time type safety; single-process, single-user binary needs none of it.

### 9.2 No protobuf for internal messages

All internal messages are plain Go structs. No `.proto` schema, no generated code,
no `google.golang.org/protobuf` dependency.

### 9.3 All three LLM providers retained; tagged by stability

All three compile into the same binary. No build tags, no profiles.

```
LLM_PROVIDER=openai-key    → stable, official, default
LLM_PROVIDER=copilot       → advanced/experimental (requires local gRPC bridge)
LLM_PROVIDER=codex-oauth   → advanced/experimental (unofficial chatgpt.com backend)
```

Stability tagging is in README.md and `sample.env` comments only. `copilot` and
`codex-oauth` report `Usage.TotalCostUSD = 0` (billing opaque); the decorator treats
0 as a valid no-op — cost tracking is consistent, not broken.

### 9.4 No RouterActor / no RouterService

The `/oc` / `/cc` routing is a `switch` on command prefix inside `gateway.Service`.
No separate routing service or lifecycle boundary.

### 9.5 OpenCode: no FSM needed

OpenCode manages session state server-side. The bot detects busy by catching HTTP 500
with body containing `"is busy"`. No FSM in `opencode.Service`.

Claude Code requires an FSM because it is a subprocess: `Idle → Running → WaitingInput → Idle`.

### 9.6 Telegram: long-poll + SQLite dedup by update_id

`last_update_id` stored in SQLite `channel_state` keyed by channel ID (e.g.
`"telegram:<chat_id>"`). Channel-agnostic: adding WhatsApp adds a new row, not a new column.

### 9.7 Session auto-purge: 24h on startup

```go
store.PurgeSessions(time.Now().Add(-tuning.SessionTTL))
```

Single user, single VPS. Restart = natural clean slate.

### 9.8 Zero-output exit: notify + stop

If agent exits cleanly but produced zero output: notify operator, no auto-retry, no crash.

### 9.9 Cost guard: soft-stop with atomic.Int64, three call sites

`infra.CostGuard` uses `atomic.Int64`. Three call sites:

| Site | Mechanism |
|---|---|
| Plain chat (all turns + tool rounds) | `providers.trackingProvider` decorator — automatic |
| OpenCode step-finish SSE event | `a.costGuard.Track(event.CostUSD)` in `opencode.Service` |
| ClaudeCode `total_cost_usd` result | `a.costGuard.Track(result.TotalCostUSD)` in `claudecode.Service` |

Scheduler-fired chat turns go through `trackedLLM` (the decorated provider) — covered
by the decorator automatically.

Thresholds: 80% → warning; 100% → warning, session finishes cleanly (no hard kill).

Counter reset: on every `Track()` call, compare `cost_daily.date` vs today UTC; if
different, reset counter and update date.

### 9.10 Tier 1 heartbeat: 3 retries then crash

`getMe` fails → wait 10s → retry. 3 consecutive failures → `log.Fatal`. systemd restarts.

Tier 2: `AgentHealthChecker.IsAlive()` (interface in `internal/infra`) every
`tuning.HeartbeatTier2Every` (default 5 min).

### 9.11 HITL: two-phase with SQLite registration

Write `hitl_pending` record before sending Telegram keyboard. Prevents the race where
a user replies faster than the record is created.

On restart: auto-reject all `status:pending` records, notify operator for each.

### 9.12 SOUL.md: mtime cache + correct injection

- **OpenCode**: `system` field on every `POST /session/:id/prompt_async`
- **Claude Code**: `--system-prompt "<soul content>"` on `claude -p` invocation

`infra.SOULLoader` caches by file mtime; reloads only when file changes.

### 9.13 Claude Code settings.local.json: merge strategy

On startup, `claudecode.Service`:
1. Backup existing file to `/tmp/gistclaw-claude-settings.bak`
2. Read existing file (or start with `{}`)
3. Merge: patch only the `hooks` key
4. Write back

### 9.14 Graceful shutdown

```
1. Cancel root context → all services receive ctx.Done()
2. errgroup.Wait() returns when all goroutines exit
3. context.WithTimeout(10s) wraps the wait — force exit if drain stalls
4. claudecode.Service.Stop(): SIGTERM → 2s wait → SIGKILL for claude subprocess
5. opencode.Service.Stop(): POST /session/:id/abort
```

### 9.15 Tool registry: three categories, built at request time

```go
// internal/gateway/service.go
func (s *Service) buildToolRegistry() []providers.Tool {
    var tools []providers.Tool

    // Core — always present if configured
    if s.search != nil {
        tools = append(tools, webSearchTool())
    }
    tools = append(tools, webFetchTool())

    // Agent — MCP-derived, namespaced {server}__{tool}
    tools = append(tools, s.mcp.GetAllTools()...)

    // System — scheduler control
    tools = append(tools, s.scheduler.Tools()...)

    return tools
}
```

Doom-loop guard and multi-round tool loop are unchanged — they operate on whatever
`buildToolRegistry()` returns. Adding or removing a category is one block in the builder.

`web_search` omitted if no search API key configured.
MCP tools omitted per-server if that server failed to connect at startup.

Tool loop and doom-loop guard:
```
for {
    resp := trackedLLM.Chat(ctx, messages, tools)
    if resp has no tool calls → break, send final text
    for each tool_call in resp:
        execute tool
        append tool_result to messages (truncated: 50 KB / 2000 lines)
    check doom-loop:
        last 3 consecutive tool calls identical (name + input JSON)?
        → inject: "[Tool call loop detected. Provide your best answer now.]"
        → break, call LLM one more time without tools
}
```

### 9.16 Heartbeat Tier 2: AgentHealthChecker interface

```go
// internal/infra/heartbeat.go
type AgentHealthChecker interface {
    Name() string
    IsAlive(ctx context.Context) bool
}
```

`opencode.Service.IsAlive()` — calls `GET /global/health`; returns true if 200.
`claudecode.Service.IsAlive()` — returns true if FSM state is `Idle`, `Running`, or
`WaitingInput`. `os.FindProcess` not used — FSM state is the source of truth.

### 9.17 question.asked: sequential, not written to hitl_pending

`question.asked` is NOT written to `hitl_pending`. On restart, stale questions are lost
with the session.

Per-question timeout: `tuning.HITLTimeout`. Auto-cancel sends empty answers to unblock agent.

### 9.18 ALLOWED_USER_IDS: fail at startup if empty

```
FATAL: ALLOWED_USER_IDS is required.
```

No "allow all" mode. `AllowedUserIDs[0]` is also the `OperatorChatID()`.

### 9.19 Telegram message length: hard-split at 4096 chars

All output is delivered. No truncation.

### 9.20 Codex OAuth token persistence

Stored in SQLite `provider_credentials` keyed by `"codex"`. On startup: load if exists;
refresh if expired; trigger PKCE CLI flow if missing.

### 9.21 Error handling matrix

| Error | Detection | Response |
|---|---|---|
| LLM HTTP 429 | `resp.StatusCode == 429` | Notify operator: `"⚠️ LLM rate limited."` No silent retry. |
| LLM HTTP 5xx / timeout | `resp.StatusCode >= 500` or `context.DeadlineExceeded` | 2 retries: 5s then 10s. If still failing: notify operator. |
| LLM malformed response | JSON parse error | Log `ERROR`. Notify: `"⚠️ LLM returned unreadable response."` |
| OpenCode start failure | exits with non-zero within 5s | Notify: `"⚠️ OpenCode failed to start."` |
| OpenCode SSE malformed | SSE line parse fails | Skip line, continue. Log `WARN`. |
| ClaudeCode malformed JSON | stream-json line fails to parse | Skip line, continue. Log `WARN`. Zero lines → zero-output path. |
| Telegram 429 | `telegoErr.Parameters.RetryAfter > 0` | Sleep `RetryAfter`; retry. |
| Telegram 5xx / network | Non-429 send failure | 3 retries: 500ms → 1s → 2s. Log `ERROR`, drop on exhaustion. |
| Telegram 403 / blocked | `telegoErr.ErrorCode == 403` | Log `WARN`. No retry. |
| web_fetch 403 Cloudflare | `403` + `cf-mitigated: challenge` | Retry once with `User-Agent: gistclaw`. Second 403 → return as tool_result. |
| web_fetch non-2xx | Any other non-2xx | Return `"HTTP <code> from <url>"` as tool_result. |
| web_fetch body too large | Read body > 5 MB | Return `"Page too large (> 5 MB)"` as tool_result. |
| web_search provider error | HTTP error from search API | Return `"Search failed: <error>"` as tool_result. |
| hitl.Service permanently stopped | withRestart exhausted | `DrainPending()` then operator notification. |
| MCP server not connected | `MCPManager.CallTool` for offline server | Return `"MCP server '<name>' is not available"` as tool_result. |
| MCP tool call timeout | No response within 10s | Return `"MCP tool call timed out"` as tool_result. |
| MCP server disconnects | stdio exits or SSE drops | Log `WARN`. No auto-reconnect in v1. |
| Scheduler fires, agent busy | SubmitTask returns error | Notify: `"⏰ Scheduled job skipped: agent busy."` Do not advance `next_run_at`. |
| Scheduler cron expr invalid | `gronx` parse error | Return error as `schedule_job` tool_result. Job not saved. |
| scheduler.Service permanently stopped | withRestart exhausted | Notify operator. Jobs in SQLite resume on restart. |
| withRestart unexpected error | err != nil && !PermanentFailure | Log `ERROR` with "BUG:" prefix. Treated as permanent by caller. |
| Graceful shutdown timeout | 10s drain exceeded | `log.Fatal("drain timeout")`. |

All user-visible errors prefixed with `⚠️`. No stack traces in Telegram messages.

### 9.22 MCP: GistClaw-owned client, plain-chat tools only

`MCPManager` is plain Go — not a service loop. Starts at binary startup, connects
user-configured servers, exposes tools to `gateway.Service` via the Agent category.

Tool namespacing: `{serverName}__{toolName}` (double underscore, matching picoclaw).
Connect failure: log `WARN`, skip — one bad server must not block startup.

### 9.23 Scheduler: typed AgentKind, LLM-controlled jobs

```go
// internal/scheduler/service.go
type Job struct {
    ID             string     // ULID
    Kind           string     // "at" | "every" | "cron"
    Target         agent.Kind // typed enum; stored as string in SQLite
    Prompt         string
    Schedule       string     // RFC3339 (at) | seconds string (every) | cron expr (cron)
    NextRunAt      time.Time
    LastRunAt      *time.Time
    Enabled        bool
    DeleteAfterRun bool       // auto-set true for "at" jobs
    CreatedAt      time.Time
}

type JobTarget interface {
    RunAgentTask(ctx context.Context, kind agent.Kind, prompt string) error
    SendChat(ctx context.Context, chatID int64, text string) error
}
```

Missed jobs on restart:
1. Load enabled jobs where `next_run_at <= now`
2. Fire up to `tuning.MissedJobsFireLimit` immediately (staggered 500ms)
3. Remaining: advance `next_run_at` to next valid future time, log `WARN`
4. `"at"` jobs past due: fire once then delete

---

## 10. Dependency Budget

Target: ≤ 15 direct dependencies.

| Package | Purpose |
|---|---|
| `golang.org/x/sync/errgroup` | Concurrent service lifecycle management |
| `github.com/mymmrac/telego` | Telegram Bot API (inline keyboard support) |
| `modernc.org/sqlite` | Pure-Go SQLite — no CGo |
| `github.com/rs/zerolog` | Structured logging |
| `github.com/caarlos0/env/v11` | Env var config binding |
| `github.com/google/uuid` | Session and HITL record IDs |
| `github.com/openai/openai-go` | OpenAI API key provider |
| `github.com/github/copilot-sdk/go` | GitHub Copilot gRPC provider |
| `golang.org/x/oauth2` | OAuth2 PKCE flow for codex-oauth provider |
| `github.com/go-shiori/go-readability` | Mozilla Readability (Go port) for web_fetch |
| `github.com/modelcontextprotocol/go-sdk/mcp` | MCP client — stdio + SSE/HTTP |
| `github.com/adhocore/gronx` | Cron expression parsing + next-run calculation |
| `gopkg.in/yaml.v3` | Parse `gistclaw.yaml` for MCP server configs |

Standard library for everything else: `net/http`, `bufio`, `os/exec`, `sync/atomic`,
`time`, `encoding/json`.

**Explicitly not adding:**
- No actor framework
- No protobuf
- No WebSocket library (Telegram is HTTP long-poll)
- No ORM (raw SQL)
- No HTTP router framework (standard `net/http`)
- No process manager (`withRestart` + systemd)
- No ntfy.sh (HITL reminder goes to Telegram directly)

---

## 11. What Gets Built — Single Phase

One phase, ship when done. No artificial phase boundaries.

### Scaffold

- [ ] Go module: `github.com/canhta/gistclaw`, `go 1.25`
- [ ] `internal/config/` — env vars + Tuning struct
- [ ] `internal/store/` — SQLite schema + WAL mode + startup purge
- [ ] `internal/agent/kind.go` — AgentKind enum + String() + KindFromString()
- [ ] `internal/app/supervisor.go` — withRestart, PermanentFailure, restartDelay
- [ ] `internal/app/app.go` — App struct, NewApp, Run (errgroup + supervision policy)
- [ ] `cmd/gistclaw/main.go` — ~25 lines: config → NewApp → Run

### Hook helper binary (`cmd/gistclaw-hook/`)

- [ ] Read PreToolUse JSON from stdin
- [ ] POST to `http://127.0.0.1:8765/hook/pretool`
- [ ] Block waiting for HTTP response
- [ ] Allow → write JSON to stdout, exit 0
- [ ] Deny → write JSON to stderr, exit 2
- [ ] Timeout (>5 min) → deny, exit 2

### channel.Channel + Telegram adapter

- [ ] `internal/channel/channel.go` — Channel interface, InboundMessage, KeyboardPayload, Button
- [ ] `internal/channel/telegram/telegram.go` — long-poll, update_id dedup, message splitting,
      KeyboardPayload → telego.InlineKeyboardMarkup

### gateway.Service (`internal/gateway/`)

- [ ] Reads InboundMessage from channel.Channel
- [ ] User ID whitelist check
- [ ] Command routing: `/oc` → opencode, `/cc` → claudecode, no-prefix → tool loop
- [ ] `/status` → formatted reply
- [ ] `/stop` → stop both agents if active
- [ ] Callback query handler → hitl.Service
- [ ] `buildToolRegistry()` — Core / Agent / System categories
- [ ] Tool loop with doom-loop guard
- [ ] Tool result truncation: 50 KB / 2000 lines

### opencode.Service (`internal/opencode/`)

- [ ] `Run(ctx)`: start `opencode serve --port <OPENCODE_PORT> --hostname 127.0.0.1`
- [ ] `POST /session` → session ID
- [ ] `POST /session/:id/prompt_async` with soul content
- [ ] SSE consumer on `GET /event?directory=<path>`
- [ ] `message.part.updated` (text) → accumulate + send at 4096-char boundary
- [ ] `message.part.updated` (step-finish) → `costGuard.Track(event.CostUSD)`
- [ ] `permission.asked` → `hitl.Approver.RequestPermission()`
- [ ] `question.asked` → `hitl.Approver.RequestQuestion()`
- [ ] `session.status {type:"idle"}` → send "✅ Done"
- [ ] Zero-output detection
- [ ] HTTP 500 "is busy" guard
- [ ] `IsAlive(ctx)` → GET /global/health

### claudecode.Service (`internal/claudecode/`)

- [ ] `Run(ctx)`: spawn `claude -p` subprocess, FSM
- [ ] Parse stream-json: text deltas + `total_cost_usd` → `costGuard.Track()`
- [ ] HTTP server `127.0.0.1:8765`
  - [ ] `POST /hook/pretool` — block via channel, call hitl.Approver, respond
  - [ ] `POST /hook/notification` — forward to channel.SendMessage
  - [ ] `POST /hook/stop` — FSM → Idle, notify
- [ ] Zero-output detection
- [ ] Write `.claude/settings.local.json` (merge strategy)
- [ ] `Stop(ctx)`: SIGTERM → 2s → SIGKILL
- [ ] `IsAlive(ctx)`: FSM state check

### hitl.Service (`internal/hitl/`)

- [ ] `Run(ctx)`: event loop
- [ ] `RequestPermission`: write SQLite, send `channel.KeyboardPayload` via channel.Channel
- [ ] `RequestQuestion`: sequential processing
  - [ ] `multiple: false` — single-choice keyboard
  - [ ] `multiple: true` — multi-select (toggle + confirm)
  - [ ] `custom: true` — free-text override button
  - [ ] After all questions → single POST /question/:id/reply
- [ ] `HITLTimeout` timer per request; reminder at `timeout - HITLReminderBefore`
- [ ] `DrainPending()`: deny all in-flight DecisionCh channels
- [ ] Startup: auto-reject all `status:pending` in hitl_pending
- [ ] `hitl/keyboard.go` builds `channel.KeyboardPayload` only (no telego imports)

### LLM Providers (`internal/providers/`)

- [ ] `LLMProvider` interface with `Usage.TotalCostUSD` (required on all impls)
- [ ] `providers/openai/` — OpenAI key; compute `TotalCostUSD` from token counts + model pricing
- [ ] `providers/copilot/` — Copilot gRPC; `TotalCostUSD = 0` (opaque billing)
- [ ] `providers/codex/` — Codex OAuth PKCE; `TotalCostUSD = 0` (unofficial backend)
- [ ] `providers/tracking.go` — `NewTrackingProvider` decorator
- [ ] `providers.New(cfg)` factory — selects impl by `LLM_PROVIDER`

### infra (`internal/infra/`)

- [ ] `cost.go` — CostGuard: `Track(usd float64)`, atomic counter, 80%+100% notify, daily reset
- [ ] `heartbeat.go` — Tier1 + Tier2; AgentHealthChecker interface
- [ ] `soul.go` — SOULLoader: mtime cache

### agent (`internal/agent/`)

- [ ] `kind.go` — Kind type, constants, String(), KindFromString()

### Plain chat tools (`internal/tools/`)

- [ ] `search.go` — SearchProvider interface, auto-detect factory, all backends
- [ ] `fetch.go` — WebFetcher interface + implementation

### MCP client (`internal/mcp/`)

- [ ] MCPServerConfig struct
- [ ] MCPManager: connect, GetAllTools(), CallTool() with 10s timeout, Close()
- [ ] Auto-detect transport: URL → SSE/HTTP, Command → stdio
- [ ] Tool namespacing: `{serverName}__{toolName}`
- [ ] Connect failure: log WARN, skip

### scheduler.Service (`internal/scheduler/`)

- [ ] `Run(ctx)`: 1s ticker, load due jobs, fire via JobTarget
- [ ] Job struct with `Target agent.Kind`
- [ ] CRUD in SQLite `jobs` table
- [ ] `at` / `every` / `cron` kinds; gronx for cron
- [ ] Missed jobs on startup
- [ ] `Tools()` — returns []providers.Tool for gateway tool registry (System category)

### Operations

- [ ] `Makefile` — build, run, deploy, logs, test
- [ ] `deploy/gistclaw.service` — systemd unit
- [ ] `README.md` — quickstart
- [ ] `ARCHITECTURE.md` — topology + flows + interface contracts
- [ ] `EXTENDING.md` — new channel / new provider / new agent guide

### Manual smoke test checklist

- [ ] Flow A: `/oc` task → streaming output → "Done"
- [ ] Flow A (busy): second `/oc` while first running → "Agent is busy"
- [ ] Flow B: permission ask → keyboard → approve → task continues
- [ ] Flow B (timeout): ask → wait → reminder → auto-reject
- [ ] Flow C: PreToolUse hook → keyboard → deny → Claude aborts
- [ ] Flow D: zero-output prompt → warning
- [ ] Flow E: web_search called → answer
- [ ] Flow E (web_fetch): web_fetch called → summary
- [ ] Flow E (doom-loop): 3 identical calls → loop aborted → final answer
- [ ] Flow F: direct answer without search
- [ ] Flow G: multi-question → sequential → both answers received
- [ ] Restart with pending HITL → auto-reject notification
- [ ] Cost guard: 80% → warning; 100% → warning, task continues
- [ ] SIGTERM mid-task → no dangling gistclaw-hook
- [ ] ALLOWED_USER_IDS empty → fatal exit
- [ ] Codex OAuth PKCE → token stored → plain chat works
- [ ] No search key → startup warning; plain chat works without web_search
- [ ] hitl.Service 10 crashes in 10s → degraded-mode notification; gateway still running
- [ ] opencode.Service 5 crashes in 30s → degraded-mode notification; other services running
- [ ] gateway.Service permanent failure → root context cancelled → process exits → systemd restarts
- [ ] `agent.KindFromString` with unknown string → returns error, job not saved

---

## 12. Behavior & Failure Modes

### Service failure escalation

```
Service crashes once
  → withRestart restarts immediately (1s initial delay, exponential backoff, cap 30s)
  → no user notification

Service crashes beyond retry limit
  → withRestart returns PermanentFailure to app.Run
  → if gateway: root context cancelled → systemd restarts all
  → if non-critical: log error + operator Telegram notification (once)
  → other services continue running
```

`gateway.Service` uses unlimited retries. Telegram connectivity issues are transient
network events; it should always come back. If it somehow returns an unexpected error
on unlimited retries, that is a bug and treated as fatal.

`hitl.Service` has a higher limit (10) because it holds blocking channels that agents
depend on. `DrainPending()` is called before the operator notification.

### Telegram send failures

- **Rate limited (429)**: read `retry_after`; sleep; retry
- **Temporary failures**: 3 retries at 500ms / 1s / 2s; log ERROR; drop on exhaustion
- **Bot blocked (403)**: log WARN only; no retry

### Plain chat failures

- **LLM 429**: surface to user immediately; no retry
- **LLM 5xx / timeout**: 2 retries; notify on exhaustion
- **Tool error**: return error as tool_result; LLM decides
- **Doom-loop**: 3 identical calls → force final answer

### Startup failures

| Condition | Behavior |
|---|---|
| `ALLOWED_USER_IDS` empty | `log.Fatal` |
| Telegram token invalid | Tier 1 heartbeat 3×; then `log.Fatal` |
| OpenCode already on port | opencode.Service logs + notifies operator |
| SQLite locked | startup fails with `log.Fatal` |
| No search API key | Log warning; `web_search` omitted; plain chat works |

---

## 13. Operations

### Health check endpoints

| Endpoint | Owner | Returns | Used by |
|---|---|---|---|
| `GET /global/health` | OpenCode server | `{healthy:true, version:string}` | Heartbeat Tier 2 |
| `claudecode.Service.IsAlive()` | GistClaw (FSM state) | bool | Heartbeat Tier 2 |

No GistClaw-owned HTTP health endpoint in v1.

### Logging

`rs/zerolog` structured JSON to stdout. `journalctl -u gistclaw -f` for tailing.

Key fields: `level`, `service`, `chat_id`, `session_id`, `hitl_id`, `error`,
`restart_count`, `cost_usd`.

Sensitive values (tokens, API keys) never logged.

### Resource model (estimates)

| Resource | Expected (idle) | Expected (active task) |
|---|---|---|
| RSS memory | ~15–30 MB | ~40–80 MB (+ OpenCode server) |
| CPU | < 1% | varies (LLM streaming) |
| SQLite DB size | < 1 MB (24h purge) | < 5 MB |

### `/status` command output format

```
GistClaw status (UTC)
Uptime: 2h 34m
OpenCode: idle  (last: 3m ago)
ClaudeCode: idle
HITL pending: 0
Scheduled jobs: 3 active  (next: in 14 min)
Daily cost: $0.42 / $5.00 (8%)
MCP servers: filesystem ✓  github ✓  myserver ✗ (failed)
```

---

## 14. Data, Privacy & Persistence

### SQLite schema

```sql
-- WAL mode enabled on startup: PRAGMA journal_mode=WAL;

CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,
    agent       TEXT NOT NULL,           -- 'opencode' | 'claudecode'
    status      TEXT NOT NULL,           -- 'active' | 'done' | 'aborted'
    prompt      TEXT,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    finished_at DATETIME
);

CREATE TABLE IF NOT EXISTS hitl_pending (
    id          TEXT PRIMARY KEY,        -- 'permission_<ulid>'
    agent       TEXT NOT NULL,
    tool_name   TEXT,
    status      TEXT NOT NULL,           -- 'pending' | 'resolved' | 'auto_rejected'
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    resolved_at DATETIME
);

CREATE TABLE IF NOT EXISTS cost_daily (
    date        TEXT PRIMARY KEY,        -- 'YYYY-MM-DD' UTC
    total_usd   REAL NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS channel_state (
    channel_id      TEXT PRIMARY KEY,    -- e.g. 'telegram:<chat_id>'
    last_update_id  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS provider_credentials (
    provider    TEXT PRIMARY KEY,        -- 'codex'
    data        TEXT NOT NULL,           -- JSON blob (access_token, refresh_token, expiry)
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS jobs (
    id               TEXT PRIMARY KEY,
    kind             TEXT NOT NULL,      -- 'at' | 'every' | 'cron'
    target           TEXT NOT NULL,      -- agent.Kind.String(): 'opencode' | 'claudecode' | 'chat'
    prompt           TEXT NOT NULL,
    schedule         TEXT NOT NULL,
    next_run_at      DATETIME NOT NULL,
    last_run_at      DATETIME,
    enabled          INTEGER NOT NULL DEFAULT 1,
    delete_after_run INTEGER NOT NULL DEFAULT 0,
    created_at       DATETIME NOT NULL DEFAULT (datetime('now'))
);
```

### Migration strategy

v1 uses `CREATE TABLE IF NOT EXISTS`. Breaking schema changes require manual `DROP TABLE`
+ binary restart. `golang-migrate` preferred if a migration framework becomes necessary in v2.

### Data retention

- Sessions older than `tuning.SessionTTL` (24h) purged on every startup
- `hitl_pending` records older than 24h purged on startup
- `cost_daily` rows older than `tuning.CostHistoryTTL` (30d) purged on startup
- `provider_credentials` never purged
- `channel_state` never purged (one row per channel)

### Privacy

- No message content stored in SQLite (except `sessions.prompt`; purged at 24h)
- No Telegram user data stored (only numeric IDs in `ALLOWED_USER_IDS`)
- Only `provider_credentials` stores tokens. DB file: `chmod 600`.

---

## 15. Security & Configuration

### All environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `TELEGRAM_TOKEN` | Yes | — | Telegram Bot API token |
| `ALLOWED_USER_IDS` | Yes | — | Comma-separated numeric IDs; fatal if empty; `[0]` is OperatorChatID |
| `OPENCODE_DIR` | Yes | — | Working directory for `opencode serve` |
| `CLAUDE_DIR` | Yes | — | Working directory for `claude -p` |
| `DAILY_LIMIT_USD` | No | `5.0` | Daily cost cap; soft-stop at 100% |
| `LLM_PROVIDER` | No | `openai-key` | `openai-key` (stable) \| `copilot` (advanced) \| `codex-oauth` (advanced) |
| `OPENAI_API_KEY` | If `openai-key` | — | OpenAI API key |
| `OPENAI_MODEL` | No | `gpt-4o` | OpenAI model name; used by `openai-key` provider |
| `COPILOT_GRPC_ADDR` | No | `localhost:4321` | Copilot gRPC bridge address |
| `OPENCODE_PORT` | No | `8766` | Port for `opencode serve` |
| `GISTCLAW_BRAVE_API_KEY` | No | — | Brave Search API key |
| `GISTCLAW_GEMINI_API_KEY` | No | — | Gemini API key |
| `GISTCLAW_XAI_API_KEY` | No | — | xAI Grok API key |
| `GISTCLAW_PERPLEXITY_API_KEY` | No | — | Perplexity direct API key |
| `GISTCLAW_OPENROUTER_API_KEY` | No | — | OpenRouter API key |
| `SOUL_PATH` | No | `./SOUL.md` | Path to SOUL.md system prompt file |
| `SQLITE_PATH` | No | `./gistclaw.db` | Path to SQLite database file |
| `HOOK_SERVER_ADDR` | No | `127.0.0.1:8765` | Address for gistclaw-hook HTTP server |
| `LOG_LEVEL` | No | `info` | `debug` \| `info` \| `warn` \| `error` |
| `MCP_CONFIG_PATH` | No | `./gistclaw.yaml` | YAML file with `mcp_servers` block |

### Access control

- **User whitelist**: all messages checked against `ALLOWED_USER_IDS` before processing.
  Unknown senders receive no reply (logged at `DEBUG` only).
- **Hook server**: binds to `127.0.0.1` only.
- **OpenCode server**: `--hostname 127.0.0.1` — loopback only.
- **No auth on hook server**: trusted by loopback binding on single-user VPS.

---

## 16. Deployment & DX

### systemd service unit

```ini
# deploy/gistclaw.service
[Unit]
Description=GistClaw — Telegram-controlled AI coding agent
After=network.target

[Service]
Type=simple
User=gistclaw
WorkingDirectory=/home/gistclaw
EnvironmentFile=/home/gistclaw/.env
ExecStart=/usr/local/bin/gistclaw
Restart=always
RestartSec=5
StartLimitBurst=5
StartLimitIntervalSec=60
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

### sample.env

```sh
# /home/gistclaw/.env  (chmod 600)

# Required
TELEGRAM_TOKEN=your_bot_token_here
ALLOWED_USER_IDS=123456789   # [0] is used as OperatorChatID for supervisor notifications
OPENCODE_DIR=/home/gistclaw/projects
CLAUDE_DIR=/home/gistclaw/projects

# Core LLM — choose one
# openai-key: stable, official (recommended for most deployments)
LLM_PROVIDER=openai-key
OPENAI_API_KEY=sk-...

# copilot: advanced/experimental — requires local gRPC bridge on localhost:4321
# LLM_PROVIDER=copilot

# codex-oauth: advanced/experimental — uses chatgpt.com backend (free-tier cost savings)
# LLM_PROVIDER=codex-oauth

# Search (at least one for web_search tool)
GISTCLAW_BRAVE_API_KEY=BSA...
# GISTCLAW_GEMINI_API_KEY=...
# GISTCLAW_XAI_API_KEY=...
# GISTCLAW_PERPLEXITY_API_KEY=...
# GISTCLAW_OPENROUTER_API_KEY=...

# Optional
DAILY_LIMIT_USD=5.0
SOUL_PATH=/home/gistclaw/SOUL.md
LOG_LEVEL=info
```

### Makefile targets

```makefile
build:   go build ./cmd/gistclaw ./cmd/gistclaw-hook
run:     go run ./cmd/gistclaw
deploy:  scp gistclaw gistclaw-hook user@vps:/usr/local/bin/ && \
         ssh user@vps systemctl restart gistclaw
logs:    ssh user@vps journalctl -u gistclaw -f
test:    go test ./...
```

### Adding a new channel (EXTENDING.md summary)

1. Implement `internal/channel/<name>/<name>.go` — the `channel.Channel` interface
2. `Receive`, `SendMessage`, `SendKeyboard` (accepts `channel.KeyboardPayload`), `SendTyping`, `Name`
3. Add `CHANNEL=<name>` env var; add case to `app.NewApp` channel factory switch
4. `gateway.Service` does not change — it only uses `channel.Channel`
5. `channel_state` table keyed by channel ID — dedup works for all channels automatically

### Adding a new LLM provider (EXTENDING.md summary)

1. Create `internal/providers/<name>/<name>.go` implementing `providers.LLMProvider`
2. Populate `Usage.TotalCostUSD` (exact if available; 0 if billing is opaque)
3. Add case to `providers.New(cfg)` factory, add `LLM_PROVIDER=<name>` to docs
4. `providers.NewTrackingProvider` wraps it automatically — no cost wiring needed

### Adding a new agent kind (EXTENDING.md summary)

1. Add constant to `internal/agent/kind.go`: `KindNewAgent Kind = iota`
2. Add case to `KindFromString` and `String()`
3. Add case to `JobTarget` implementation in `app.go`
4. No scheduler changes; no gateway changes

### Build artifacts

Two binaries per release:
- `gistclaw` — main bot process
- `gistclaw-hook` — hook helper (must be on VPS where `claude` runs)

Both: `GOOS=linux GOARCH=amd64`. GoReleaser config is post-v1.

---

_Stack: Go 1.25, golang.org/x/sync/errgroup, mymmrac/telego, modernc.org/sqlite, rs/zerolog, caarlos0/env, openai-go, github/copilot-sdk/go, golang.org/x/oauth2, go-shiori/go-readability, modelcontextprotocol/go-sdk, adhocore/gronx, gopkg.in/yaml.v3_
_References: openclaw (Node.js, 551-file agents/), picoclaw (Go, 26 packages), opencode v1.2.24, claude-code hooks/CLI_
_v3.0 — six structural improvements over v2: generic withRestart with PermanentFailure sentinel; channel-agnostic gateway with KeyboardPayload in channel layer; LLMProvider Usage contract + trackingProvider decorator; agent.Kind typed enum; internal/app owns wiring + supervision; internal/infra replaces three micro-packages; Go 1.25 (loop capture fixed)_
