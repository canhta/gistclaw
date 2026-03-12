# GistClaw Plan 9: Entrypoints & Ops

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Wire the main binary entrypoint, hook helper binary (already built in Plan 6), full Makefile, systemd unit, sample.env, and the three public documentation files.

**Architecture:** `cmd/gistclaw/main.go` is ~25 lines: load config → NewApp → signal-aware Run. All the interesting code lives in internal/. This plan adds the operational layer and docs.

**Tech Stack:** Go 1.25, systemd, make
**Design reference:** `docs/plans/design.md` §13, §16
**Depends on:** Plans 1–8

---

## Execution order

```
Task 1  cmd/gistclaw/main.go      (entrypoint binary)
Task 2  Makefile                  (full build/deploy targets)
Task 3  deploy/gistclaw.service   (systemd unit) + sample.env
Task 4  README.md                 (quickstart doc)
Task 5  ARCHITECTURE.md           (service topology + contracts)
Task 6  EXTENDING.md              (extension guides)
```

Tasks are sequential. Each task: write → verify → commit.

> **Note on design doc path:** Existing plans reference `design-v3.md` but the actual file is
> `docs/plans/design.md`. Use `docs/plans/design.md` as the canonical design reference.

---

## Task 1: `cmd/gistclaw/main.go`

**Files:**
- Create: `cmd/gistclaw/main.go`

### Step 1: Create the directory and write the file

```go
// cmd/gistclaw/main.go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/app"
	"github.com/canhta/gistclaw/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config: load failed")
	}

	// Configure zerolog level
	level, _ := zerolog.ParseLevel(cfg.LogLevel)
	zerolog.SetGlobalLevel(level)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}) // JSON in prod; override if needed

	a, err := app.NewApp(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("app: init failed")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := a.Run(ctx); err != nil {
		log.Fatal().Err(err).Msg("app: run failed")
	}
}
```

Keep the file at ~25 lines. No business logic here — everything lives in `internal/`.

### Step 2: Verify build succeeds

```bash
go build ./cmd/gistclaw
```

Expected: exits 0, produces `./gistclaw` binary (or no output on success). No compile errors.

### Step 3: Verify no-env startup behaviour

Run the binary with no environment variables set:

```bash
unset TELEGRAM_TOKEN ALLOWED_USER_IDS OPENCODE_DIR CLAUDE_DIR OPENAI_API_KEY
./gistclaw 2>&1; echo "exit: $?"
```

Expected output contains "config" (e.g., `"config: load failed"`) and exit code is non-zero (1 or 2).

Clean up the stray binary:

```bash
rm -f ./gistclaw
```

### Step 4: Verify binary is included in make build output

Skip full `make build` here (Makefile not yet written). Confirm the package compiles cleanly with:

```bash
go vet ./cmd/gistclaw/...
```

Expected: no output (no issues).

### Step 5: Commit

```bash
git add cmd/gistclaw/main.go
git commit -m "feat: add cmd/gistclaw entrypoint (~25 lines, config→NewApp→Run)"
```

---

## Task 2: Full `Makefile`

**Files:**
- Create or replace: `Makefile` (the repo root already has a skeleton; replace it entirely)

### Step 1: Write the Makefile

```makefile
.PHONY: tidy test build run lint deploy logs

BINARY_DIR := ./bin
GISTCLAW   := $(BINARY_DIR)/gistclaw
HOOK       := $(BINARY_DIR)/gistclaw-hook

tidy:
	go mod tidy

test:
	go test ./... -timeout 120s

build: tidy
	mkdir -p $(BINARY_DIR)
	go build -o $(GISTCLAW) ./cmd/gistclaw
	go build -o $(HOOK) ./cmd/gistclaw-hook

run: build
	$(GISTCLAW)

lint:
	go vet ./...

# Deploy: rsync binaries to VPS and restart the service
# Usage: make deploy VPS=user@your-vps-ip
deploy: build
	rsync -avz $(GISTCLAW) $(HOOK) $(VPS):/usr/local/bin/
	ssh $(VPS) "systemctl restart gistclaw"

# Tail logs on VPS
# Usage: make logs VPS=user@your-vps-ip
logs:
	ssh $(VPS) "journalctl -u gistclaw -f"
```

**Important:** The recipe lines (under each target) must use **tabs**, not spaces. Most editors default to tabs in Makefiles — verify this is correct before saving.

### Step 2: Verify `make build` succeeds

```bash
make build
```

Expected:
- `go mod tidy` runs (no error)
- `mkdir -p ./bin` runs
- `go build -o ./bin/gistclaw ./cmd/gistclaw` succeeds
- `go build -o ./bin/gistclaw-hook ./cmd/gistclaw-hook` succeeds
- Both binaries exist and are executable:

```bash
ls -la ./bin/
```

Expected: two files, `gistclaw` and `gistclaw-hook`, both with execute permission.

### Step 3: Verify `make lint` passes

```bash
make lint
```

Expected: no output (no vet issues).

### Step 4: Verify `make test` compiles and runs

```bash
make test
```

Expected: all tests pass (or skip if no tests require external services). No compile errors.

### Step 5: Verify no-env binary behaviour via make

```bash
unset TELEGRAM_TOKEN ALLOWED_USER_IDS OPENCODE_DIR CLAUDE_DIR OPENAI_API_KEY
./bin/gistclaw 2>&1; echo "exit: $?"
```

Expected: output contains "config", exit code non-zero.

### Step 6: Commit

```bash
git add Makefile
git commit -m "feat: add full Makefile (tidy, test, build, run, lint, deploy, logs)"
```

---

## Task 3: `deploy/gistclaw.service` + `sample.env`

**Files:**
- Create: `deploy/gistclaw.service`
- Create: `sample.env`

### Step 1: Create the deploy directory and systemd unit

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

This is the exact unit from design §16. No modifications.

### Step 2: Create `sample.env`

```sh
# /home/gistclaw/.env
# chmod 600 /home/gistclaw/.env
#
# Copy this file to /home/gistclaw/.env on the VPS and fill in the required values.
# Variables marked "required" will cause a fatal startup error if missing or empty.

# ─── Required ──────────────────────────────────────────────────────────────────

# Telegram Bot API token from @BotFather
TELEGRAM_TOKEN=your_bot_token_here

# Comma-separated numeric Telegram user IDs allowed to use the bot.
# The first ID is also used as the OperatorChatID for supervisor notifications.
# Fatal at startup if empty.
ALLOWED_USER_IDS=123456789

# Working directory for `opencode serve` (where OpenCode runs your code tasks)
OPENCODE_DIR=/home/gistclaw/projects

# Working directory for `claude -p` (where Claude Code runs your code tasks)
CLAUDE_DIR=/home/gistclaw/projects

# ─── Core LLM — choose one ─────────────────────────────────────────────────────

# openai-key: stable, official OpenAI API (recommended for most deployments)
LLM_PROVIDER=openai-key
OPENAI_API_KEY=sk-...

# OpenAI model name (used by openai-key provider)
# OPENAI_MODEL=gpt-4o

# copilot: advanced/experimental — requires local gRPC bridge running on localhost:4321
# Billing via GitHub Copilot subscription (TotalCostUSD reported as 0 — opaque billing)
# LLM_PROVIDER=copilot
# COPILOT_GRPC_ADDR=localhost:4321

# codex-oauth: advanced/experimental — uses chatgpt.com/backend-api via PKCE OAuth
# Free-tier cost savings; unofficial backend; TotalCostUSD reported as 0
# LLM_PROVIDER=codex-oauth

# ─── Search API — at least one for web_search tool ─────────────────────────────
# Provider is auto-detected: first key found wins. Set at most one for predictable behaviour.

# Brave Search (recommended — privacy-focused, generous free tier)
GISTCLAW_BRAVE_API_KEY=BSA...

# Gemini API (Google Search grounding)
# GISTCLAW_GEMINI_API_KEY=...

# xAI Grok API
# GISTCLAW_XAI_API_KEY=...

# Perplexity direct API
# GISTCLAW_PERPLEXITY_API_KEY=...

# OpenRouter API (fallback aggregator)
# GISTCLAW_OPENROUTER_API_KEY=...

# ─── Optional ──────────────────────────────────────────────────────────────────

# Daily cost cap in USD. Soft-stop: notify operator at 80% and 100%.
# Current session finishes cleanly when cap is hit.
# DAILY_LIMIT_USD=5.0

# Path to SOUL.md system prompt file injected into every agent session.
# SOUL_PATH=./SOUL.md

# Path to SQLite database file.
# SQLITE_PATH=./gistclaw.db

# Address for gistclaw-hook HTTP server (loopback only — do not expose externally).
# HOOK_SERVER_ADDR=127.0.0.1:8765

# Port for `opencode serve` (loopback only).
# OPENCODE_PORT=8766

# Path to YAML file with mcp_servers block for MCP client configuration.
# MCP_CONFIG_PATH=./gistclaw.yaml

# Log level: debug | info | warn | error
# LOG_LEVEL=info

# ─── Tuning (advanced — change only if you know what you're doing) ─────────────

# HITL approval timeout. After this duration without user response, request is auto-rejected.
# TUNING_HITL_TIMEOUT=5m

# How long before HITL timeout to send a reminder message.
# TUNING_HITL_REMINDER_BEFORE=2m

# Timeout for web_search tool calls.
# TUNING_WEB_SEARCH_TIMEOUT=10s

# Timeout for web_fetch tool calls.
# TUNING_WEB_FETCH_TIMEOUT=30s

# Session TTL: sessions older than this are purged from SQLite on startup.
# TUNING_SESSION_TTL=24h

# Cost history TTL: cost_daily rows older than this are purged on startup.
# TUNING_COST_HISTORY_TTL=720h

# How often Tier 1 heartbeat checks Telegram getMe liveness.
# TUNING_HEARTBEAT_TIER1_EVERY=30s

# How often Tier 2 heartbeat checks agent IsAlive() and auto-restarts if dead.
# TUNING_HEARTBEAT_TIER2_EVERY=5m

# How often the scheduler checks for due jobs.
# TUNING_SCHEDULER_TICK=1s

# Maximum number of missed jobs to fire immediately on scheduler startup.
# TUNING_MISSED_JOBS_FIRE_LIMIT=5
```

### Step 3: Verify all required env vars from design §15 are present

Check the design doc table (§15) against `sample.env`. All 20 variables must appear:

| Variable | Present in sample.env |
|---|---|
| `TELEGRAM_TOKEN` | required section |
| `ALLOWED_USER_IDS` | required section |
| `OPENCODE_DIR` | required section |
| `CLAUDE_DIR` | required section |
| `DAILY_LIMIT_USD` | optional section (commented out with default) |
| `LLM_PROVIDER` | LLM section (active) |
| `OPENAI_API_KEY` | LLM section |
| `OPENAI_MODEL` | LLM section (commented out with default) |
| `COPILOT_GRPC_ADDR` | LLM section (commented out with default) |
| `OPENCODE_PORT` | optional section (commented out with default) |
| `GISTCLAW_BRAVE_API_KEY` | search section (active) |
| `GISTCLAW_GEMINI_API_KEY` | search section (commented out) |
| `GISTCLAW_XAI_API_KEY` | search section (commented out) |
| `GISTCLAW_PERPLEXITY_API_KEY` | search section (commented out) |
| `GISTCLAW_OPENROUTER_API_KEY` | search section (commented out) |
| `SOUL_PATH` | optional section (commented out with default) |
| `SQLITE_PATH` | optional section (commented out with default) |
| `HOOK_SERVER_ADDR` | optional section (commented out with default) |
| `LOG_LEVEL` | optional section (commented out with default) |
| `MCP_CONFIG_PATH` | optional section (commented out with default) |

Verify `deploy/gistclaw.service` has no placeholder text and all required fields:

```bash
grep -E "Description|User=|EnvironmentFile|ExecStart|Restart=" deploy/gistclaw.service
```

Expected: all five lines present with correct values.

### Step 4: Commit

```bash
git add deploy/gistclaw.service sample.env
git commit -m "feat: add systemd unit (deploy/gistclaw.service) and annotated sample.env"
```

---

## Task 4: `README.md`

**Files:**
- Create: `README.md`

### Step 1: Write the README

```markdown
# GistClaw

GistClaw is a single Go binary that acts as a remote controller for AI coding agents.
Run it on a VPS. Interact with it from your phone via Telegram. It relays tasks to
[OpenCode](https://opencode.ai) or [Claude Code](https://docs.anthropic.com/en/docs/claude-code),
streams responses back, and asks for your approval when an agent wants to do something
that needs a human decision. Any non-command message is treated as plain chat: routed
to a configurable LLM with `web_search` and `web_fetch` tools available.

---

## Prerequisites

- **Go 1.25+** — [https://go.dev/dl/](https://go.dev/dl/)
- **opencode** installed and on `$PATH` — [https://opencode.ai](https://opencode.ai)
- **claude** CLI installed and on `$PATH` — [https://docs.anthropic.com/en/docs/claude-code](https://docs.anthropic.com/en/docs/claude-code)
- A **Telegram bot token** — create one via [@BotFather](https://t.me/BotFather)
- Your **Telegram numeric user ID** — get it from [@userinfobot](https://t.me/userinfobot)

---

## Install

```bash
git clone https://github.com/canhta/gistclaw
cd gistclaw
make build
```

This produces `./bin/gistclaw` (main binary) and `./bin/gistclaw-hook` (Claude Code hook helper).

---

## Minimal configuration

```bash
cp sample.env .env
```

Open `.env` and fill in at minimum:

```sh
TELEGRAM_TOKEN=your_bot_token_here
ALLOWED_USER_IDS=your_numeric_telegram_id
OPENCODE_DIR=/path/to/your/projects
CLAUDE_DIR=/path/to/your/projects
OPENAI_API_KEY=sk-...
```

All other variables have sensible defaults. See `sample.env` for the full annotated list.

---

## Run

**Local (foreground):**

```bash
make run
```

**Production (systemd on VPS):**

```bash
# On the VPS — one-time setup
sudo useradd -m -s /bin/bash gistclaw
sudo cp deploy/gistclaw.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable gistclaw

# Copy binaries and config
sudo cp ./bin/gistclaw ./bin/gistclaw-hook /usr/local/bin/
sudo cp .env /home/gistclaw/.env
sudo chmod 600 /home/gistclaw/.env

# Start
sudo systemctl start gistclaw
sudo systemctl status gistclaw
```

**Deploy updates from your machine:**

```bash
make deploy VPS=gistclaw@your-vps-ip
```

**Tail logs:**

```bash
make logs VPS=gistclaw@your-vps-ip
```

---

## Commands

Send these commands from your Telegram chat with the bot:

| Command | Description |
|---|---|
| `/oc <prompt>` | Start or continue an OpenCode session with the given prompt |
| `/cc <prompt>` | Start or continue a Claude Code session with the given prompt |
| `/stop` | Abort the currently active agent session |
| `/status` | Show current session status and today's cost |
| _(plain text)_ | Plain chat: routed to the core LLM with `web_search` and `web_fetch` available |

Only Telegram users listed in `ALLOWED_USER_IDS` receive responses. All others are silently ignored.
```

### Step 2: Verify section headers are present and no placeholder text remains

Check all required sections exist:

```bash
grep -E "^## " README.md
```

Expected output (all six headers):
```
## Prerequisites
## Install
## Minimal configuration
## Run
## Commands
```

Verify no placeholder text slipped through:

```bash
grep -i "TODO\|FIXME\|placeholder\|your-vps-ip" README.md
```

Expected: only intentional user-facing placeholders like `your-vps-ip` in command examples (these are correct — they tell the user what to substitute).

Verify Go module path does not appear with a wrong value:

```bash
grep "github.com/canhta/gistclaw" README.md
```

Expected: matches the correct module path.

### Step 3: Commit

```bash
git add README.md
git commit -m "docs: add README.md (quickstart, install, config, commands)"
```

---

## Task 5: `ARCHITECTURE.md`

**Files:**
- Create: `ARCHITECTURE.md`

### Step 1: Write the file

```markdown
# GistClaw Architecture

This document describes the internal structure of GistClaw: how services are arranged,
how they communicate, and the contracts that keep them decoupled.

---

## 1. Service Topology

GistClaw is a single Go binary. Five services run as goroutines under `golang.org/x/sync/errgroup`.
Everything else is plain Go called synchronously. No actor framework, no protobuf.

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
           - channel_state                      - LLMProvider interface
           - jobs                               - trackingProvider decorator
                                                - copilot / codex / openai impls
                                                internal/tools
                                                - SearchProvider (Brave/Gemini/
                                                  Grok/Perplexity — auto-detect)
                                                - WebFetcher (go-readability)
                                                internal/mcp
                                                - MCPManager (stdio + SSE/HTTP)
```

---

## 2. Main Flows

### Flow A — `/oc build the auth module`

1. Telegram update arrives at `gateway.Service`
2. `update_id` dedup check against `channel_state` in SQLite
3. Command prefix `/oc` → `opencode.Service.SubmitTask(ctx, chatID, prompt)`
4. OpenCode service POSTs to `opencode serve` REST API (reuses or creates session)
5. SSE consumer receives `message.part.updated` events → `channel.SendMessage` in chunks
6. `CostGuard.Track()` called on each `step-finish` event
7. `session.status {type:"idle"}` → `channel.SendMessage(chatID, "✅ Done")`

### Flow B — OpenCode permission request mid-task

1. SSE fires `permission.asked` event
2. `opencode.Service` calls `hitl.Approver.RequestPermission(ctx, req)`
3. `hitl.Service` writes `hitl_pending` to SQLite; sends inline keyboard via `channel.SendKeyboard`
4. Timer: reminder at `HITLReminderBefore` before timeout; auto-reject at `HITLTimeout`
5. User taps button → gateway routes callback → `hitl.Service` resolves → POST reply to OpenCode

### Flow C — Claude Code tool approval via hook helper

1. `claude` subprocess spawns `gistclaw-hook`; hook writes `PreToolUse` JSON to its stdin
2. `gistclaw-hook` POSTs to `http://127.0.0.1:8765/hook/pretool`
3. `claudecode.Service` hook handler calls `hitl.Approver.RequestPermission` and blocks
4. Same HITL flow as Flow B
5. Handler writes allow/deny JSON; `gistclaw-hook` exits with code 0 or 2

### Flow D — Zero output from agent

1. OpenCode session goes idle with empty output buffer
2. `opencode.Service` sends: `"⚠️ Agent finished but produced no output. Check the session or resend your prompt."`
3. No auto-retry; no crash; user decides next action

### Flow E — Plain chat with tool loop

1. No command prefix → `gateway.Service` builds tool registry (Core + Agent + System tools)
2. Calls `trackedLLM.Chat(ctx, messages, tools)` in a loop
3. LLM may call `web_search` or `web_fetch` (doom-loop guard: 3 identical calls → forced final answer)
4. Final text answer → `channel.SendMessage`; cost tracked automatically by decorator

---

## 3. Interface Contracts

### `channel.Channel` (`internal/channel/channel.go`)

```go
type Channel interface {
    Receive(ctx context.Context) (<-chan InboundMessage, error)
    SendMessage(ctx context.Context, chatID int64, text string) error
    SendKeyboard(ctx context.Context, chatID int64, payload KeyboardPayload) error
    SendTyping(ctx context.Context, chatID int64) error
    Name() string // "telegram", "whatsapp", etc.
}
```

`gateway.Service` only imports `channel.Channel`. Platform-specific types (e.g., `telego`) are
confined to `internal/channel/telegram`. `hitl/keyboard.go` imports only `internal/channel` —
no Telegram dependency in HITL.

### `providers.LLMProvider` (`internal/providers/llm.go`)

```go
type LLMProvider interface {
    Chat(ctx context.Context, messages []Message, tools []Tool) (*LLMResponse, error)
    Name() string
}

type Usage struct {
    PromptTokens     int
    CompletionTokens int
    TotalCostUSD     float64 // required; 0 if provider cannot determine cost
}
```

All three providers (`openai-key`, `copilot`, `codex-oauth`) implement this interface.
`NewTrackingProvider` wraps any `LLMProvider` and calls `CostGuard.Track` on every
successful response. Gateway and scheduler only see the decorated interface.

### `hitl.Approver` (`internal/hitl/service.go`)

```go
type Approver interface {
    RequestPermission(ctx context.Context, req PermissionRequest) error
    RequestQuestion(ctx context.Context, req QuestionRequest) error
}
```

OpenCode and ClaudeCode services call this interface. HITL internals (SQLite, keyboard,
timers) are hidden behind it.

### `AgentHealthChecker` (`internal/infra/heartbeat.go`)

```go
type AgentHealthChecker interface {
    IsAlive(ctx context.Context) bool
}
```

Both `opencode.Service` and `claudecode.Service` implement this. `infra.Heartbeat` Tier 2
calls it every 5 minutes and triggers a restart if the agent is dead.

---

## 4. Supervision Model

`withRestart` (`internal/app/supervisor.go`) wraps any `func(context.Context) error` in a
restart loop. `PermanentFailure` is returned when the service exhausts its restart budget.
`app.Run` owns the policy: which services are critical (cancel the root context on failure)
and which are non-critical (log + notify operator, keep running).

### Supervision strategies

| Service | Max attempts | Window | On permanent stop |
|---|---|---|---|
| `gateway.Service` | unlimited | — | Returns `PermanentFailure` → errgroup cancels root context → systemd restarts all |
| `opencode.Service` | 5 | 30s | Log + notify operator; other services continue |
| `claudecode.Service` | 5 | 30s | Log + notify operator; other services continue |
| `hitl.Service` | 10 | 10s | `DrainPending()` first (unblocks hook handlers); then log + notify |
| `scheduler.Service` | 5 | 30s | Log + notify; SQLite jobs resume on next restart |

### Critical vs non-critical

Only `gateway.Service` is critical. If it permanently fails (impossible in practice — unlimited
retries), the root context is cancelled and systemd restarts the whole process.

All other services are non-critical: a permanently failed `opencode.Service` leaves
`claudecode`, `hitl`, `scheduler`, and `gateway` fully operational. Users are notified via
Telegram and can continue using other features.

### `withRestart` signature

```go
// maxAttempts=0 means unlimited restarts.
func withRestart(
    name        string,
    maxAttempts int,
    window      time.Duration,
    fn          func(context.Context) error,
) func(context.Context) error
```

Returns `nil` on clean shutdown (ctx cancelled) or clean service exit. Returns
`PermanentFailure` when the restart budget is exhausted within `window`.
```

### Step 2: Verify all section headers are present

```bash
grep -E "^## " ARCHITECTURE.md
```

Expected:
```
## 1. Service Topology
## 2. Main Flows
## 3. Interface Contracts
## 4. Supervision Model
```

Verify flow labels A through E are all present:

```bash
grep -E "### Flow [A-E]" ARCHITECTURE.md
```

Expected: 5 lines (Flow A, B, C, D, E).

Verify all four interface contracts are documented:

```bash
grep -E "### .*(Channel|LLMProvider|Approver|HealthChecker)" ARCHITECTURE.md
```

Expected: 4 lines.

Verify Go import paths are correct (no placeholder paths):

```bash
grep "github.com/" ARCHITECTURE.md | grep -v "canhta/gistclaw"
```

Expected: only external package references like `golang.org/x/sync` — no wrong module paths.

### Step 3: Commit

```bash
git add ARCHITECTURE.md
git commit -m "docs: add ARCHITECTURE.md (service topology, flows, contracts, supervision)"
```

---

## Task 6: `EXTENDING.md`

**Files:**
- Create: `EXTENDING.md`

### Step 1: Write the file

```markdown
# Extending GistClaw

This document explains how to add new capabilities to GistClaw without modifying
existing services. Three extension patterns are supported by the architecture:

1. [Adding a new channel](#1-adding-a-new-channel)
2. [Adding a new LLM provider](#2-adding-a-new-llm-provider)
3. [Adding a new agent kind](#3-adding-a-new-agent-kind)

---

## 1. Adding a new channel

A "channel" is a chat platform adapter. The v1 implementation is Telegram. Adding
Discord, Slack, or any other platform follows this pattern.

1. Create `internal/channel/<name>/<name>.go` implementing `channel.Channel`:

   ```go
   package <name>

   import (
       "context"
       "github.com/canhta/gistclaw/internal/channel"
   )

   type Channel struct { /* platform-specific fields */ }

   func New(cfg Config) (*Channel, error) { /* ... */ }

   func (c *Channel) Receive(ctx context.Context) (<-chan channel.InboundMessage, error) { /* ... */ }
   func (c *Channel) SendMessage(ctx context.Context, chatID int64, text string) error  { /* ... */ }
   func (c *Channel) SendKeyboard(ctx context.Context, chatID int64, payload channel.KeyboardPayload) error { /* ... */ }
   func (c *Channel) SendTyping(ctx context.Context, chatID int64) error                { /* ... */ }
   func (c *Channel) Name() string { return "<name>" }
   ```

   The five methods are the complete contract. Platform-specific types (SDKs, API clients)
   are confined to this package — nothing else imports them.

2. Add any required env vars to `internal/config/config.go` and `sample.env`.
   Example: `DISCORD_BOT_TOKEN`, `DISCORD_CHANNEL_ID`.

3. Add a case to the channel factory in `internal/app/app.go` (`NewApp`):

   ```go
   // internal/app/app.go — inside NewApp
   var ch channel.Channel
   switch cfg.Channel {
   case "telegram":
       ch, err = telegram.New(cfg)
   case "<name>":
       ch, err = <name>.New(cfg)
   default:
       return nil, fmt.Errorf("unknown channel: %s", cfg.Channel)
   }
   ```

   Set `CHANNEL=<name>` in `.env` to activate the new adapter.

4. `gateway.Service` does not change — it only uses `channel.Channel`. HITL does not
   change — it only constructs `channel.KeyboardPayload` (no platform types). The
   `channel_state` table in SQLite is keyed by channel ID, so dedup works for all
   channel implementations without schema changes.

**What you do NOT need to change:** `gateway`, `hitl`, `opencode`, `claudecode`,
`scheduler`, `store`, any provider, any tool.

---

## 2. Adding a new LLM provider

A "provider" implements the `providers.LLMProvider` interface. Adding a new model
backend (e.g., Anthropic direct API, a local Ollama server) follows this pattern.

1. Create `internal/providers/<name>/<name>.go` implementing `providers.LLMProvider`:

   ```go
   package <name>

   import (
       "context"
       "github.com/canhta/gistclaw/internal/providers"
   )

   type Provider struct { /* API client, config */ }

   func New(cfg Config) (*Provider, error) { /* ... */ }

   func (p *Provider) Chat(ctx context.Context, messages []providers.Message, tools []providers.Tool) (*providers.LLMResponse, error) {
       // Call the backend API
       // Populate Usage.TotalCostUSD:
       //   - exact value if the API returns token prices (like openai-key)
       //   - 0.0 if billing is opaque (like copilot, codex-oauth)
       // A zero TotalCostUSD is valid — it does not trigger cost soft-stop thresholds.
       return &providers.LLMResponse{
           Content: "...",
           Usage: providers.Usage{
               PromptTokens:     123,
               CompletionTokens: 45,
               TotalCostUSD:     0.001, // or 0.0 if unknown
           },
       }, nil
   }

   func (p *Provider) Name() string { return "<name>" }
   ```

2. Add a case to the factory in `internal/providers/llm.go`:

   ```go
   // internal/providers/llm.go — inside New(cfg)
   switch cfg.LLMProvider {
   case "openai-key":
       return openai.New(cfg)
   case "copilot":
       return copilot.New(cfg)
   case "codex-oauth":
       return codex.New(cfg)
   case "<name>":
       return <name>.New(cfg)
   default:
       return nil, fmt.Errorf("unknown LLM_PROVIDER: %s", cfg.LLMProvider)
   }
   ```

3. Add `LLM_PROVIDER=<name>` documentation to `sample.env` (commented out) and `README.md`.

4. `providers.NewTrackingProvider` wraps the new provider automatically — no cost wiring needed.
   `gateway.Service` and `scheduler.Service` both receive the decorated provider via `app.NewApp`.

**What you do NOT need to change:** `gateway`, `hitl`, `opencode`, `claudecode`,
`scheduler`, `store`, any channel, any tool.

---

## 3. Adding a new agent kind

An "agent kind" identifies which agent service handles a job dispatched by the scheduler
or triggered by the gateway. The typed enum lives in `internal/agent/kind.go`.

1. Add a constant to `internal/agent/kind.go`:

   ```go
   // internal/agent/kind.go
   package agent

   type Kind int

   const (
       KindOpenCode   Kind = iota
       KindClaudeCode
       KindChat
       KindNewAgent   // add here
   )
   ```

2. Add a case to `String()` in the same file:

   ```go
   func (k Kind) String() string {
       switch k {
       case KindOpenCode:   return "opencode"
       case KindClaudeCode: return "claudecode"
       case KindChat:       return "chat"
       case KindNewAgent:   return "newagent"
       default:             return "unknown"
       }
   }
   ```

3. Add a case to `KindFromString` (used when scanning from the SQLite `jobs.target` column):

   ```go
   func KindFromString(s string) (Kind, error) {
       switch s {
       case "opencode":   return KindOpenCode, nil
       case "claudecode": return KindClaudeCode, nil
       case "chat":       return KindChat, nil
       case "newagent":   return KindNewAgent, nil
       default:           return 0, fmt.Errorf("unknown agent kind: %s", s)
       }
   }
   ```

4. Add a case to the `JobTarget` implementation in `internal/app/app.go`:

   ```go
   // internal/app/app.go — appJobTarget.RunAgentTask
   func (t *appJobTarget) RunAgentTask(ctx context.Context, kind agent.Kind, prompt string) error {
       switch kind {
       case agent.KindOpenCode:
           return t.opencode.SubmitTask(ctx, t.operatorChatID, prompt)
       case agent.KindClaudeCode:
           return t.claudecode.SubmitTask(ctx, t.operatorChatID, prompt)
       case agent.KindChat:
           return t.gateway.SendChat(ctx, t.operatorChatID, prompt)
       case agent.KindNewAgent:
           return t.newagent.SubmitTask(ctx, t.operatorChatID, prompt)
       default:
           return fmt.Errorf("unknown agent kind: %v", kind)
       }
   }
   ```

**What you do NOT need to change:** `scheduler.Service` does not change — it dispatches
via `JobTarget` and knows nothing about concrete agent types. `gateway.Service` does not
change. No database schema changes — `jobs.target` stores the string representation.
```

### Step 2: Verify all section headers are present

```bash
grep -E "^## " EXTENDING.md
```

Expected:
```
## 1. Adding a new channel
## 2. Adding a new LLM provider
## 3. Adding a new agent kind
```

Verify all four numbered steps for each section are present (each extension guide must have at least 4 steps):

```bash
grep -E "^[0-9]+\. " EXTENDING.md | wc -l
```

Expected: 12 or more lines (4 steps × 3 sections minimum).

Verify Go import paths are correct:

```bash
grep "github.com/" EXTENDING.md | grep -v "canhta/gistclaw"
```

Expected: no wrong module paths.

Verify no placeholder text remains (the `<name>` markers are intentional — they are extension guides, not placeholders):

```bash
grep -iE "TODO|FIXME|placeholder" EXTENDING.md
```

Expected: no output.

### Step 3: Commit

```bash
git add EXTENDING.md
git commit -m "docs: add EXTENDING.md (new channel, LLM provider, agent kind guides)"
```

---

## Final verification

After all six tasks are complete, run a full verification pass:

### Build

```bash
make build
```

Expected: both binaries produced in `./bin/` with no errors.

### Tests

```bash
make test
```

Expected: all tests pass.

### Lint

```bash
make lint
```

Expected: no vet issues.

### Binary smoke test

```bash
unset TELEGRAM_TOKEN ALLOWED_USER_IDS OPENCODE_DIR CLAUDE_DIR OPENAI_API_KEY
./bin/gistclaw 2>&1; echo "exit: $?"
```

Expected: message containing "config", non-zero exit code.

### File checklist

```bash
ls cmd/gistclaw/main.go Makefile deploy/gistclaw.service sample.env README.md ARCHITECTURE.md EXTENDING.md
```

Expected: all seven files present.

### Section header audit

```bash
grep -E "^## " README.md ARCHITECTURE.md EXTENDING.md
```

Expected: all documented sections present in each file.

---

Plan 9 complete. Next: Plan 10 (Smoke Tests).
