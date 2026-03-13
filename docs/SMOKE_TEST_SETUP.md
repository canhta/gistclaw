# Smoke Test Setup Guide

This guide walks you through obtaining the credentials needed to run the Plan 10
manual smoke tests. Complete all steps before starting `./bin/gistclaw`.

---

## Step 1 — Create a Telegram bot and get the token

1. Open Telegram and search for **@BotFather** (the official Telegram bot creation service — blue checkmark).
2. Send `/newbot` to BotFather.
3. BotFather asks for a **name** — this is the display name (e.g. `GistClaw Dev`).
4. BotFather asks for a **username** — must end in `bot` (e.g. `gistclaw_dev_bot`). Must be unique.
5. BotFather responds with your bot token, which looks like:
   ```
   7412345678:AAExxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
   ```
6. Copy the token and set it in `.env`:
   ```sh
   TELEGRAM_TOKEN=7412345678:AAExxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
   ```

> Keep this token secret — it grants full control of your bot.

---

## Step 2 — Get your numeric Telegram user ID

1. Open Telegram and search for **@userinfobot**.
2. Send any message (e.g. `/start`).
3. The bot replies with your profile info, including your **Id** (a numeric value, e.g. `123456789`).
4. Copy the ID and set it in `.env`:
   ```sh
   ALLOWED_USER_IDS=123456789
   ```

> This is **not** your username — it is a permanent numeric identifier. Your bot will only
> respond to messages from users whose ID is in this list.

---

## Step 3 — Get a Brave Search API key

1. Go to [https://api.search.brave.com/app/keys](https://api.search.brave.com/app/keys).
2. Create a free account if you don't have one.
3. Click **Create API key**. Give it a name (e.g. `gistclaw-dev`).
4. Copy the key (starts with `BSA...`) and set it in `.env`:
   ```sh
   GISTCLAW_BRAVE_API_KEY=BSAxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
   ```

> The free tier provides 2,000 queries/month — more than enough for smoke tests.

---

## Step 4 — Verify the Copilot gRPC bridge is running

The `copilot` LLM provider requires a local gRPC bridge on `localhost:4321`. Verify it is
reachable before starting gistclaw:

```bash
nc -z localhost 4321 && echo "bridge is up" || echo "bridge not reachable"
```

If the bridge is not running, either start it or switch `.env` to `LLM_PROVIDER=openai-key`
and add `OPENAI_API_KEY=sk-...`.

---

## Step 5 — Final checklist before starting

Open `.env` and confirm every `PLACEHOLDER_*` value has been replaced:

```bash
grep PLACEHOLDER .env
```

Expected: no output (all placeholders filled in).

Then set secure permissions and start:

```bash
chmod 600 .env
export $(grep -v '^#' .env | xargs)
make build
./bin/gistclaw
```

Confirm startup logs contain (with `LOG_LEVEL=debug`):
- `gateway started`
- Log lines from opencode and claudecode services initializing

You are ready to run the 21 smoke test checks in `docs/plans/2026-03-12-10-smoke-tests.md`.
