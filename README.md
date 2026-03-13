# GistClaw

[![CI](https://github.com/canhta/gistclaw/actions/workflows/ci.yml/badge.svg)](https://github.com/canhta/gistclaw/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A single Go binary that runs on your VPS and lets you control AI coding agents from your phone via Telegram. Send a task, get streamed output, approve tool calls inline — all without opening a laptop.

Supports [OpenCode](https://opencode.ai) and [Claude Code](https://docs.anthropic.com/en/docs/claude-code). Any non-command message goes to plain chat (LLM + `web_search` + `web_fetch`).

---

## Features

- **Telegram interface** — tasks, streamed output, inline approvals, all on your phone
- **OpenCode + Claude Code** — first-class support for both; sessions persist across messages
- **Human-in-the-loop** — inline keyboard approvals and question flows; auto-reject on timeout
- **Plain chat** — LLM with `web_search`, `web_fetch`, and MCP tools
- **Scheduler** — cron/at/every jobs targeting any agent
- **Memory** — `SOUL.md` personality, `MEMORY.md` long-term memory, daily notes
- **Cost tracking** — daily USD cap with soft-stop at 80% and 100%
- **Fault-tolerant** — supervised services with restart budgets; two-tier heartbeat

See [docs/architecture.md](docs/architecture.md) for internals.

---

## Prerequisites

- Go 1.25+
- `opencode` on `$PATH` — [opencode.ai](https://opencode.ai)
- `claude` CLI on `$PATH` — [docs.anthropic.com](https://docs.anthropic.com/en/docs/claude-code)
- A Telegram bot token ([@BotFather](https://t.me/BotFather)) and your numeric user ID ([@userinfobot](https://t.me/userinfobot))

---

## Quick start

```bash
git clone https://github.com/canhta/gistclaw
cd gistclaw
make build
cp sample.env .env
```

Fill in `.env` at minimum:

```sh
TELEGRAM_TOKEN=your_bot_token_here
ALLOWED_USER_IDS=your_numeric_telegram_id
OPENCODE_DIR=/path/to/your/projects
CLAUDE_DIR=/path/to/your/projects
OPENAI_API_KEY=sk-...
```

Run locally:

```bash
export $(grep -v '^#' .env | xargs) && make run
```

See `sample.env` for all options. See [docs/testing.md](docs/testing.md) for getting credentials.

---

## Production (systemd)

```bash
# One-time VPS setup
sudo useradd -m -s /bin/bash gistclaw
sudo cp deploy/gistclaw.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable gistclaw

# Deploy
sudo cp ./bin/gistclaw ./bin/gistclaw-hook /usr/local/bin/
sudo cp .env /home/gistclaw/.env && sudo chmod 600 /home/gistclaw/.env
sudo systemctl start gistclaw
```

Deploy updates from your machine:

```bash
make deploy VPS=gistclaw@your-vps-ip
make logs   VPS=gistclaw@your-vps-ip
```

---

## Commands

| Command | What it does |
|---|---|
| `/start` or `/help` | Capability summary in the bot's own voice |
| `/oc <prompt>` | Start or continue an OpenCode session |
| `/cc <prompt>` | Start or continue a Claude Code session |
| `/stop` | Abort the active agent session |
| `/status` | Active session + today's cost |
| _(plain text)_ | Chat with LLM (`web_search` + `web_fetch` available) |

Only `ALLOWED_USER_IDS` receive responses.

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Code conventions are in [AGENTS.md](AGENTS.md).

---

## License

MIT — see [LICENSE](LICENSE).
