# Testing

## Automated tests

```bash
make test              # go test ./... -timeout 120s
go test -race ./...    # run before opening a PR
go test ./internal/app/...                                    # single package
go test ./internal/app/... -run TestWithRestartPermanentFailure  # single test
```

---

## Credential setup

### Telegram bot token

1. Message [@BotFather](https://t.me/BotFather) → `/newbot` → follow prompts
2. Copy the token and set `TELEGRAM_TOKEN=<token>` in `.env`

Keep the token secret — it grants full control of your bot.

### Your Telegram user ID

1. Message [@userinfobot](https://t.me/userinfobot) → it replies with your numeric **Id**
2. Set `ALLOWED_USER_IDS=<id>` in `.env`

This is a permanent numeric ID, not your username.

### Brave Search API key (optional, for `web_search`)

1. Go to [api.search.brave.com/app/keys](https://api.search.brave.com/app/keys) → create a free key
2. Set `GISTCLAW_BRAVE_API_KEY=<key>` in `.env`

Free tier: 2,000 queries/month.

### LLM provider

Default is `openai-key`:
```sh
LLM_PROVIDER=openai-key
OPENAI_API_KEY=sk-...
```

For `copilot`, verify the local gRPC bridge is up:
```bash
nc -z localhost 4321 && echo "up" || echo "not reachable"
```

### Final checklist

```bash
grep -E '^\w+=your_|PLACEHOLDER' .env  # expect no output
chmod 600 .env
export $(grep -v '^#' .env | xargs)
make build && ./bin/gistclaw
```

With `LOG_LEVEL=debug`, confirm `gateway started` appears in the logs.
