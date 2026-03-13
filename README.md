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
| `/oc <prompt>` | Start or continue an OpenCode session with the given prompt |
| `/cc <prompt>` | Start or continue a Claude Code session with the given prompt |
| `/stop` | Abort the currently active agent session |
| `/status` | Show current session status and today's cost |
| _(plain text)_ | Plain chat: routed to the core LLM with `web_search` and `web_fetch` available |

Only Telegram users listed in `ALLOWED_USER_IDS` receive responses. All others are silently ignored.
