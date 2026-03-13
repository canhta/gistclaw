# GistClaw

[![CI](https://github.com/canhta/gistclaw/actions/workflows/ci.yml/badge.svg)](https://github.com/canhta/gistclaw/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

GistClaw is a single Go binary that acts as a remote controller for AI coding agents.
Run it on a VPS. Interact with it from your phone via Telegram. It relays tasks to
[OpenCode](https://opencode.ai) or [Claude Code](https://docs.anthropic.com/en/docs/claude-code),
streams responses back, and asks for your approval when an agent wants to do something
that needs a human decision. Any non-command message is treated as plain chat: routed
to a configurable LLM with `web_search` and `web_fetch` tools available.

---

## Problem

AI coding agents are powerful but awkward to operate remotely. You either need a laptop
open, a VPN into your server, or you trust the agent to run unsupervised. GistClaw solves
this: it sits on your VPS 24/7, keeps your agents supervised, streams their output to your
phone, and pings you the moment a human decision is needed — so you can approve or reject
from anywhere.

---

## Key features

- **Telegram-native interface** — send tasks, receive streamed output, approve tool calls, all from your phone
- **OpenCode + Claude Code** — first-class support for both agents; sessions persist across messages
- **Human-in-the-loop (HITL)** — inline keyboard approvals and multi-question flows; auto-reject on timeout
- **Plain chat with tools** — any non-command message routes to a configurable LLM with `web_search`, `web_fetch`, and MCP tools
- **Multi-agent orchestration** — `spawn_agent`, `run_parallel`, `chain_agents` tools for delegating subtasks
- **Memory & personality** — `SOUL.md` system prompt, `MEMORY.md` long-term memory, daily notes, proactive summarisation
- **Scheduler** — cron/at/every jobs targeting any agent; SQLite-backed with missed-job recovery
- **Cost tracking** — daily USD cap with soft-stop notifications; per-provider cost reporting
- **Fault-tolerant** — supervised services with configurable restart budgets; two-tier heartbeat with auto-restart
- **Extensible** — add a new channel, LLM provider, or agent kind without touching existing services (see [docs/extending.md](docs/extending.md))

---

## Architecture

GistClaw is a composition of five supervised goroutine services wired together in
`internal/app`. All service boundaries are Go interfaces — nothing is coupled to a
concrete type. A `WithRestart` supervisor wraps each service with a configurable
restart budget; only the gateway is critical (its failure restarts the whole process
via systemd). See [docs/architecture.md](docs/architecture.md) for the full topology,
interface contracts, and flow diagrams.

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

If your `opencode serve` instance requires HTTP Basic Auth, also set:

```sh
OPENCODE_SERVER_USERNAME=your_username
OPENCODE_SERVER_PASSWORD=your_password
```

---

## Run

**Local (foreground):**

Environment variables are not auto-loaded from `.env`. Export them first:

```bash
export $(grep -v '^#' .env | xargs) && make run
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
| `/start` or `/help` | Show a capability summary in the bot's own voice |
| `/oc <prompt>` | Start or continue an OpenCode session with the given prompt |
| `/cc <prompt>` | Start or continue a Claude Code session with the given prompt |
| `/stop` | Abort the currently active agent session |
| `/status` | Show current session status and today's cost |
| _(plain text)_ | Plain chat: routed to the core LLM with `web_search` and `web_fetch` available |

Only Telegram users listed in `ALLOWED_USER_IDS` receive responses. All others are silently ignored.

---

## Contributing

Contributions are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening
a PR. The project conventions (import style, error wrapping, test patterns) are documented
in [AGENTS.md](AGENTS.md). Extension patterns (new channels, providers, agent kinds) are
documented in [docs/extending.md](docs/extending.md).

---

## License

MIT — see [LICENSE](LICENSE).
