# GistClaw — Validated Design Document

> A lightweight Go binary in the openclaw/picoclaw family.
> Controls OpenCode and Claude Code from Telegram, 24/7.
> Built by analysing openclaw, picoclaw, opencode v1.2.24, and claude-code.

_Version: 1.0 — March 2026. All decisions validated through Q&A with the author._

---

## Table of Contents

1. [What GistClaw Is](#1-what-gistclaw-is)
2. [Positioning in the Claw Family](#2-positioning-in-the-claw-family)
3. [Scope — v1 In / Out](#3-scope--v1-in--out)
4. [Architecture — Actor Topology](#4-architecture--actor-topology)
5. [Message Flow Examples](#5-message-flow-examples)
6. [Package Layout](#6-package-layout)
7. [Key Design Decisions (All Resolved)](#7-key-design-decisions-all-resolved)
8. [Dependency Budget](#8-dependency-budget)
9. [What Gets Built — Single Phase](#9-what-gets-built--single-phase)
10. [Corrections from Earlier Design Docs](#10-corrections-from-earlier-design-docs)

---

## 1. What GistClaw Is

GistClaw is a single Go binary that acts as a remote controller for AI coding agents.
You run it on a VPS. You interact with it from your phone via Telegram.
It relays your tasks to OpenCode or Claude Code, streams responses back, and asks for
your approval when an agent wants to do something that needs a human decision.

It is part of the claw family (openclaw → picoclaw → **gistclaw**) but adds two things
neither predecessor has:

- **Native OpenCode control** — via `opencode serve` REST + SSE API
- **Native Claude Code control** — via subprocess with a hook callback server

The name reflects focus: gist = the essential point. Only what matters.

---

## 2. Positioning in the Claw Family

| | openclaw | picoclaw | **gistclaw** |
|---|---|---|---|
| Language | TypeScript | Go | **Go** |
| RAM | ~1 GB | < 10 MB | **< 15 MB** |
| Chat platforms | 15+ | 10+ | **Pluggable, Telegram in v1** |
| AI providers | Many | Many | **Abstracted; OpenAI + Copilot in v1** |
| Controls OpenCode | No | No | **Yes — HTTP + SSE** |
| Controls Claude Code | No | No | **Yes — subprocess + hooks** |
| HITL | Full, complex | None | **Simple: approve / reject / 5-min timeout** |
| Actor model | No | No | **Yes — protoactor-go for core actors** |
| Session recovery on restart | No (silent drop) | No | **Yes — SQLite as source of truth** |

---

## 3. Scope — v1 In / Out

### In scope

| Feature | Notes |
|---|---|
| Telegram gateway | Long-poll. Pluggable channel interface so others can be added later. |
| OpenCode actor | Controls `opencode serve` via HTTP + SSE |
| Claude Code actor | Controls `claude -p` subprocess + hook server on port 8765 |
| HITL engine actor | Approve / reject / 5-min timeout. Simple, not openclaw-level complex. |
| SOUL.md injection | mtime-cached load; injected at session creation |
| Heartbeat Tier 1 + 2 | Tier 1: Telegram liveness (3 retries then crash). Tier 2: agent health check + auto-restart. |
| Cost guard | Soft-stop: notify at 80% and 100%; current session finishes cleanly. |
| SQLite state store | Sessions, pending HITL, daily cost, last Telegram update_id |
| OpenAI provider | v1 AI provider |
| GitHub Copilot provider | v1 AI provider |
| Session auto-purge | On startup: DELETE sessions older than 24h |
| systemd service | `Restart=always`, `StartLimitBurst=5` |

### Explicitly out of scope for v1

| Feature | Reason |
|---|---|
| Cron scheduler / cron jobs | No scheduler in v1; add in v2 |
| Heartbeat Tier 3 (goal drift) | Requires scheduler; post-v1 |
| SOUL.md checkpoint cron | Requires scheduler; post-v1 |
| Skills system | Separate feature |
| Memory / MCP | Different problem |
| Multi-agent orchestration | Out of scope |
| Additional chat channels (Discord, Slack…) | Interface is ready; implementations are post-v1 |
| Session TTL config knob | No auto-purge config; fixed 24h on startup |
| `/history` and `/cleanup` commands | Post-v1 maintenance features |

---

## 4. Architecture — Actor Topology

Three actors run under protoactor-go. Everything else is plain Go.

```
systemd (Restart=always, StartLimitBurst=5)
    └── gistclaw binary
            │
      RootContext (protoactor-go)
            │
      ┌─────┴───────────────────────────────┐
      │                                     │
  TelegramGatewayActor              (protoactor built-in supervision)
  - long-poll loop                         │
  - update_id dedup (SQLite)         ┌─────┴──────────────────┐
  - inline keyboard builder          │                        │
  - /oc /cc /status routing   OpenCodeActor           ClaudeCodeActor
      │                        - start opencode serve  - start claude -p
      │ messages                - HTTP client           - stream-json parser
      │                         - SSE consumer          - FSM: Idle→Running
      │                         - session lifecycle       →WaitingInput→Idle
      │                         - no FSM needed         - hook server :8765
      │                               │                        │
      └───────────────────────────────┴────────────────────────┘
                                      │
                               HITLActor
                               - two-phase: register ID → send keyboard → wait → reply
                               - permission.asked (OpenCode SSE)
                               - PreToolUse hook (Claude Code)
                               - question.asked (OpenCode SSE, free-text or keyboard)
                               - 5-min timer; ntfy reminder at 3 min; auto-reject at 5 min
                                      │
                        ┌─────────────┴──────────────────┐
                    SQLiteStore                     plain Go services
                    (not an actor)                  - CostGuard (atomic.Int64)
                    - sessions                      - SOULLoader (mtime cache)
                    - hitl_pending                  - Heartbeat (Tier 1+2)
                    - cost_daily                    - ntfy notifier
                    - telegram_state                - Provider abstraction
                                                      (OpenAI, Copilot)
```

### Why protoactor-go for exactly these three actors

The three actors (TelegramGateway, OpenCode, ClaudeCode) each have:
- Their own lifecycle (start, crash, restart independently)
- Their own mailbox (messages queue without blocking other actors)
- Their own supervision strategy (configurable restart on panic/error)

The HITL engine is also an actor because it must hold pending approval state
across goroutines without a mutex and must receive messages from both OpenCode
(SSE events) and TelegramGateway (user button taps).

Everything else (SQLite, CostGuard, Heartbeat, SOUL) has no lifecycle of its own
and is called synchronously — plain Go is correct.

### Dead actor message handling

If a message is sent to an actor that has crashed and not yet restarted:
- **Log a warning** with zerolog (actor name, message type, timestamp)
- **Drop the message** — do not crash the sender
- The protoactor supervisor restarts the actor; the next message will succeed

This is the right call for a single-user system where uptime > correctness of
any individual message.

---

## 5. Message Flow Examples

### Flow A — User sends `/oc build the auth module`

```
1. Telegram update arrives at TelegramGatewayActor
2. update_id checked against SQLite telegram_state.last_update_id
   - if update_id ≤ last_seen → drop (duplicate)
   - else → update SQLite, continue
3. Command prefix "/oc" → send SubmitTask{Agent:"opencode", Prompt:"build the auth module"}
   to OpenCodeActor
4. OpenCodeActor checks session:
   - if no active session → POST /session with SOUL.md as systemPrompt
   - if active session → reuse
5. POST /session/:id/prompt_async → returns 204 immediately
6. OpenCodeActor's SSE watcher receives message.part.updated events
   → forward text chunks to TelegramGatewayActor → stream to user
7. CostGuard.Track(inputTokens, outputTokens) on each step-finish event
   - if spend ≥ 80% daily limit → notify Telegram "⚠️ 80% of daily cost used"
8. SSE receives session.status {type:"idle"} → TelegramGatewayActor sends "✅ Done"
```

### Flow B — OpenCode asks for permission mid-task

```
1. SSE fires permission.asked {id:"perm_123", toolName:"Bash", pattern:"rm -rf /tmp/old"}
2. OpenCodeActor sends PermissionRequest{ID:"perm_123", ...} to HITLActor
3. HITLActor writes to SQLite hitl_pending {id, status:pending, agent:opencode, ts:now}
4. HITLActor sends Telegram keyboard via TelegramGatewayActor:
   "Bash wants to run: rm -rf /tmp/old
    [✅ Once]  [✅ Always]  [❌ Reject]  [⏹ Stop]"
5. Timer starts: 3-min → ntfy.sh push "Waiting for your approval"; 5-min → auto-reject
6. User taps ✅ Once:
   - Telegram callback → TelegramGatewayActor → HITLActor
   - HITLActor POST /permission/perm_123/reply {"reply":"once"}
   - SQLite hitl_pending updated {status:resolved}
   - Timer cancelled
7. Agent continues; SSE resumes streaming
```

### Flow C — Claude Code tool approval via hook server

```
1. claude subprocess running; PreToolUse fires
2. Claude Code POSTs to http://127.0.0.1:8765/hook/pretool
   Body: {hook_event_name:"PreToolUse", tool_name:"Write", tool_input:{...}}
3. ClaudeCodeActor's hook server receives request — keeps HTTP connection OPEN
4. Sends PermissionRequest to HITLActor (same flow as Flow B steps 3-6)
5. User replies → HITLActor sends decision back to ClaudeCodeActor
6. Hook server responds to Claude:
   Allow: {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow"}}
   Deny:  {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny",
           "permissionDecisionReason":"User rejected via Telegram"}}
7. HTTP connection closes → Claude Code continues or aborts tool call
```

### Flow D — Zero output from agent

```
1. OpenCode session goes idle with zero text parts received
2. OpenCodeActor detects: session.status = idle AND outputBuffer is empty
3. Sends notification to TelegramGatewayActor:
   "⚠️ Agent finished but produced no output. Check the session or resend your prompt."
4. No auto-retry. No crash. User decides what to do next.
```

---

## 6. Package Layout

```
gistclaw/
├── main.go                      # Wire actor system, start services
├── go.mod                       # module github.com/canh/gistclaw
│
├── proto/                       # Shared protoactor message types
│   ├── messages.proto           # All actor message structs defined here
│   └── messages.pb.go           # Generated — prevents import cycles
│
├── config/                      # Env vars + YAML config, validation
│   └── config.go
│
├── store/                       # SQLite persistence (plain Go)
│   ├── sqlite.go
│   └── schema.sql               # sessions, hitl_pending, cost_daily, telegram_state
│
├── actors/
│   ├── gateway/                 # TelegramGatewayActor
│   │   └── gateway.go           # Long-poll, dedup, inline keyboards, routing
│   ├── opencode/                # OpenCodeActor
│   │   ├── actor.go             # Lifecycle, session management
│   │   ├── api.go               # Typed HTTP client for all /session endpoints
│   │   └── events.go            # SSE consumer + event dispatch
│   ├── claudecode/              # ClaudeCodeActor
│   │   ├── actor.go             # subprocess + FSM (Idle/Running/WaitingInput)
│   │   ├── hooks.go             # HTTP server :8765 — blocking hook handler
│   │   └── stream.go            # Parse stream-json lines → Telegram chunks
│   └── hitl/                   # HITLActor
│       ├── actor.go             # Core: register → notify → wait → reply
│       ├── keyboard.go          # Build Telegram inline keyboards
│       └── timeout.go           # 5-min timer, ntfy at 3 min, auto-reject at 5 min
│
├── providers/                   # AI provider abstraction (used by agent actors)
│   ├── provider.go              # LLMProvider interface
│   ├── openai/
│   │   └── openai.go
│   └── copilot/
│       └── copilot.go
│
├── soul/                        # SOUL.md loader
│   └── loader.go                # mtime cache: reload only when file changes
│
├── heartbeat/                   # Tier 1 + Tier 2 health checks (plain Go)
│   └── heartbeat.go             # Tier1: Telegram getMe (3 retries, then crash)
│                                # Tier2: GET /path + PID check every 5 min
│
├── cost/                        # Daily cost guard (plain Go)
│   └── guard.go                 # atomic.Int64 counter; notify at 80%+100%; soft-stop
│
├── notify/                      # Backup push via ntfy.sh (plain Go, fire-and-forget)
│   └── ntfy.go
│
└── Makefile                     # build, run, deploy, logs
```

---

## 7. Key Design Decisions (All Resolved)

### 7.1 Actor model — protoactor-go for three actors only

Three actors: `TelegramGatewayActor`, `OpenCodeActor`, `ClaudeCodeActor`, `HITLActor`.
Everything else (SQLite, CostGuard, Heartbeat, SOUL, ntfy) is plain Go — called
synchronously with no mailbox or lifecycle of its own.

### 7.2 No RouterActor

The `/oc` / `/cc` routing is a `switch` on command prefix — not a lifecycle boundary.
`TelegramGatewayActor` routes directly to the appropriate agent actor. One fewer
actor hop with no isolation loss.

### 7.3 OpenCode: no FSM needed

OpenCode manages session state server-side (`idle` | `busy` | `retry`). The bot only
needs to handle HTTP 409/400 when a session is busy. No FSM in `OpenCodeActor`.

Claude Code requires a FSM because it is a subprocess:
`Idle → Running → WaitingInput → Idle`.

### 7.4 Telegram: long-poll + SQLite dedup by update_id

Long-poll is sequential — Telegram holds the next batch until you ACK the current one,
so duplicates are rare. But on VPS restart, Telegram may resend the last batch.
Store `last_update_id` in SQLite (`telegram_state` table). On each incoming update:
- `update_id ≤ last_seen` → drop silently
- `update_id > last_seen` → process + update SQLite

One SQLite read per message. Negligible for single-user.

### 7.5 Session auto-purge: 24h on startup

On every binary start, before the actor system initializes:
```go
DELETE FROM sessions WHERE created_at < datetime('now', '-24 hours')
```
Single user, single VPS. A restart is a natural "clean slate" moment.
No scheduler needed (v1 has none). No config knob — fixed 24h.

### 7.6 Zero-output exit: notify + stop

If an agent exits cleanly (no crash) but produced zero output:
- Send Telegram message: `"⚠️ Agent finished but produced no output. Resend or rephrase."`
- No auto-retry (risks burning cost on a prompt that will fail again)
- No crash (clean exit ≠ crash)
- User decides what to do next

### 7.7 Cost guard: soft-stop with atomic.Int64

`atomic.Int64` counter for daily token spend. Thresholds:
- 80% of `DAILY_LIMIT_USD` → Telegram warning `"⚠️ 80% of daily cost used ($X.XX / $Y.YY)"`
- 100% → Telegram warning `"⚠️ Daily limit reached. Current session will finish cleanly."`
- No hard kill. Killing mid-session can leave files half-written on the VPS.
- Counter resets at midnight UTC.

### 7.8 Tier 1 heartbeat: 3 retries then crash

`getMe` fails → wait 10s → retry. After 3 consecutive failures → `log.Fatal`.
systemd `Restart=always` brings it back. Handles transient network blips on VPS
without unnecessary restarts.

### 7.9 HITL: two-phase with SQLite registration

Before sending the Telegram keyboard message, write the pending approval to SQLite:
```
{id, agent, tool_name, status:"pending", created_at}
```
This prevents the race condition where a user replies faster than the record is created.
On restart, any `status:pending` records in SQLite are either auto-rejected (with
Telegram notification) or replayed — decision at startup.

**HITL pending records on restart:** auto-reject stale pending approvals older than
5 minutes and notify the user. Any within 5 minutes are replayed (re-send keyboard).

### 7.10 SOUL.md: mtime cache

```go
type SOULLoader struct {
    path    string
    content string
    modTime time.Time
}
func (s *SOULLoader) Load() string {
    info, _ := os.Stat(s.path)
    if info.ModTime().Equal(s.modTime) { return s.content }
    // reload
}
```
Injected as `systemPrompt` on `POST /session` (OpenCode) and as first message on
`claude -p` invocation (Claude Code).

---

## 8. Dependency Budget

Target: ≤ 15 direct dependencies. No ORM, no HTTP framework, no pub/sub library.

| Package | Purpose |
|---|---|
| `github.com/asynkron/protoactor-go` | Actor system for gateway, agent actors, HITL |
| `github.com/mymmrac/telego` | Telegram Bot API (better inline keyboard support than alternatives) |
| `modernc.org/sqlite` | Pure-Go SQLite — no CGo |
| `github.com/rs/zerolog` | Structured logging |
| `github.com/caarlos0/env/v11` | Env var config binding |
| `github.com/google/uuid` | Session and HITL record IDs |
| `google.golang.org/protobuf` | Proto message serialization (protoactor messages) |

Standard library for everything else: `net/http`, `bufio`, `os/exec`, `sync/atomic`,
`time`, `encoding/json`.

**Explicitly not adding:**
- No WebSocket library (Telegram is HTTP long-poll)
- No ORM (raw SQL)
- No HTTP router framework (standard `net/http`)
- No message queue (protoactor mailboxes are sufficient)
- No process manager (internal protoactor supervision + systemd)
- No gorilla/mux (not needed for the two HTTP endpoints: hook server + health)

---

## 9. What Gets Built — Single Phase

One phase, ship when done. No artificial phase boundaries.

### Core binary

- [ ] Go module scaffold: `github.com/canh/gistclaw`
- [ ] `proto/messages.proto` — all actor message types
- [ ] `config/` — env vars: `TELEGRAM_TOKEN`, `OPENCODE_DIR`, `CLAUDE_DIR`, `ALLOWED_USER_IDS`, `DAILY_LIMIT_USD`
- [ ] `store/` — SQLite schema + `sessions`, `hitl_pending`, `cost_daily`, `telegram_state` tables
- [ ] Session auto-purge on startup (24h cutoff)

### TelegramGatewayActor

- [ ] Long-poll loop with telego
- [ ] `update_id` dedup via SQLite
- [ ] User ID whitelist check (reject unknown senders immediately)
- [ ] Command routing: `/oc <prompt>` → OpenCodeActor, `/cc <prompt>` → ClaudeCodeActor
- [ ] `/status` → uptime, session status, today's cost
- [ ] `/stop` → abort active session on target agent
- [ ] Inline keyboard builder for HITL approval messages
- [ ] Callback query handler → HITLActor

### OpenCodeActor

- [ ] Start `opencode serve --port 8766 --hostname 127.0.0.1`
- [ ] `POST /session` with SOUL.md as `systemPrompt`
- [ ] `POST /session/:id/prompt_async` — fire-and-forget prompt delivery
- [ ] SSE consumer on `GET /event?directory=<path>` with auto-reconnect
- [ ] Handle `message.part.updated` → stream text to TelegramGatewayActor
- [ ] Handle `permission.asked` → send to HITLActor
- [ ] Handle `question.asked` → send to HITLActor
- [ ] Handle `session.status {type:"idle"}` → notify Telegram "Done"
- [ ] Handle `server.heartbeat` → reset Tier-2 liveness timer
- [ ] Zero-output detection → notify Telegram
- [ ] HTTP 409/400 busy guard → notify "Agent is busy, wait or /stop"

### ClaudeCodeActor

- [ ] Spawn `claude -p <prompt> --output-format stream-json --verbose --include-partial-messages`
- [ ] FSM: `Idle → Running → WaitingInput → Idle`
- [ ] Parse stream-json lines: extract `session_id`, stream text deltas, capture `total_cost_usd`
- [ ] Hook HTTP server on `127.0.0.1:8765`
  - [ ] `POST /hook/pretool` — block connection, send to HITLActor, respond with decision
  - [ ] `POST /hook/notification` — forward to TelegramGatewayActor
  - [ ] `POST /hook/stop` — transition FSM to Idle, notify Telegram "Done"
- [ ] Zero-output detection → notify Telegram
- [ ] Write `~/.claude/settings.json` with hook URLs on startup

### HITLActor

- [ ] On `PermissionRequest`: write to `hitl_pending` (SQLite), send Telegram keyboard
- [ ] On `QuestionRequest`: free-text or inline keyboard depending on `answers` field
- [ ] 5-minute timer per pending request
  - [ ] 3-minute mark → ntfy.sh push reminder
  - [ ] 5-minute mark → auto-reject + notify Telegram
- [ ] On user reply (Telegram callback): resolve `hitl_pending`, send decision to origin actor
- [ ] On startup: auto-reject stale pending (> 5 min old), notify Telegram for each

### Supporting services

- [ ] `soul/loader.go` — mtime-cached SOUL.md load
- [ ] `heartbeat/heartbeat.go`
  - [ ] Tier 1: `getMe` every 30s, 3 retries × 10s, then `log.Fatal`
  - [ ] Tier 2: `GET /path?directory=<dir>` every 5 min; restart OpenCode if dead; PID check for ClaudeCode
- [ ] `cost/guard.go` — `atomic.Int64`, notify at 80% + 100%, reset at midnight UTC
- [ ] `notify/ntfy.go` — fire-and-forget `POST` to `ntfy.sh/<topic>`
- [ ] `providers/openai/` — OpenAI provider implementation
- [ ] `providers/copilot/` — GitHub Copilot provider implementation

### Operations

- [ ] `Makefile` — `build`, `run`, `deploy`, `logs`, `test`
- [ ] `deploy/gistclaw.service` — systemd unit with `Restart=always`, `StartLimitBurst=5`, `EnvironmentFile`

---

## 10. Corrections from Earlier Design Docs

The original `AI_Agent_Controller_Bot_Design.md` was written before validating against
the opencode codebase. The corrected doc `2026-03-12-ai-agent-controller-bot-design.md`
captures these. Summarised here for a single source of truth:

| Original claim | Correction | Validated against |
|---|---|---|
| `opencode server` starts the API | Correct command is `opencode serve` | `src/cli/cmd/serve.ts` |
| `GET /health` for Tier-2 liveness | No `/health` endpoint; use `GET /path?directory=<dir>` | `src/server/server.ts` |
| `status: waiting_input` detects HITL | No such status; HITL detected via `permission.asked` SSE only | `src/session/status.ts` |
| OpenCode needs its own FSM | Server manages state; bot only handles HTTP 409/400 | `src/session/prompt.ts:87` |
| Pipe `y\n`/`n\n` to stdin for approval | `POST /permission/:id/reply {"reply":"once"|"always"|"reject"}` | `src/server/routes/permission.ts` |
| `POST /session/create` | Correct path is `POST /session` | `src/server/routes/session.ts` |
| Poll `GET /session/{id}` for status | Use SSE `GET /event` for reactive updates | `src/server/server.ts:499` |
| `question.asked` not mentioned | Must handle — separate from `permission.asked`, different reply endpoint | `src/server/routes/question.ts` |

---

_Stack: Go 1.22+, protoactor-go, mymmrac/telego, modernc.org/sqlite, rs/zerolog, caarlos0/env_
_References analysed: openclaw (Node.js, 551-file agents/), picoclaw (Go, 26 packages), opencode v1.2.24, claude-code hooks/CLI_
_All design decisions validated through author Q&A — March 2026_
