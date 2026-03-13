# Testing Guide

This guide covers both the automated test suite and the manual credential setup
needed to run GistClaw against real services.

---

## Automated tests

Run the full test suite:

```bash
make test
# equivalent to: go test ./... -timeout 120s
```

Run with the race detector (required before opening a PR):

```bash
go test -race ./...
```

Run a single package:

```bash
go test ./internal/app/...
```

Run a single test by name (regex-matched):

```bash
go test ./internal/app/... -run TestWithRestartPermanentFailure
```

Run verbosely:

```bash
go test -v ./internal/config/...
```

---

## Credential setup for manual / integration testing

The steps below walk you through obtaining the credentials needed to run
GistClaw end-to-end against real services.

### Step 1 — Create a Telegram bot and get the token

1. Open Telegram and search for **@BotFather** (official — blue checkmark).
2. Send `/newbot`.
3. BotFather asks for a **name** — display name, e.g. `GistClaw Dev`.
4. BotFather asks for a **username** — must end in `bot`, e.g. `gistclaw_dev_bot`.
5. BotFather responds with your token:
   ```
   7412345678:AAExxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
   ```
6. Set it in `.env`:
   ```sh
   TELEGRAM_TOKEN=7412345678:AAExxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
   ```

> Keep this token secret — it grants full control of your bot.

---

### Step 2 — Get your numeric Telegram user ID

1. Open Telegram and search for **@userinfobot**.
2. Send any message (e.g. `/start`).
3. The bot replies with your **Id** (numeric, e.g. `123456789`).
4. Set it in `.env`:
   ```sh
   ALLOWED_USER_IDS=123456789
   ```

> This is not your username — it is a permanent numeric identifier. Only IDs
> in this list will receive responses from the bot.

---

### Step 3 — Get a Brave Search API key (optional, for `web_search`)

1. Go to [https://api.search.brave.com/app/keys](https://api.search.brave.com/app/keys).
2. Create a free account and click **Create API key**.
3. Set it in `.env`:
   ```sh
   GISTCLAW_BRAVE_API_KEY=BSAxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
   ```

> The free tier provides 2,000 queries/month.

---

### Step 4 — Configure an LLM provider

The default provider is `openai-key`. Set your key in `.env`:

```sh
LLM_PROVIDER=openai-key
OPENAI_API_KEY=sk-...
```

If using the `copilot` provider, verify the local gRPC bridge is reachable:

```bash
nc -z localhost 4321 && echo "bridge is up" || echo "bridge not reachable"
```

---

### Step 5 — Final checklist before starting

Confirm all required values are filled in `.env`:

```bash
grep -E '^\w+=your_|PLACEHOLDER' .env
```

Expected: no output.

Set secure permissions, build, and start:

```bash
chmod 600 .env
export $(grep -v '^#' .env | xargs)
make build
./bin/gistclaw
```

With `LOG_LEVEL=debug`, confirm startup logs contain `gateway started` and
service initialisation lines for opencode and claudecode.
