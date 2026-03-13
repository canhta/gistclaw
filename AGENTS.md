# AGENTS.md — GistClaw Coding Agent Guide

GistClaw is a pure-Go single binary — a Telegram-driven controller for AI coding agents (OpenCode, Claude Code).

---

## Commands

```bash
make test          # go test ./... -timeout 120s
make lint          # go vet + golangci-lint run ./...
make build         # builds ./bin/gistclaw and ./bin/gistclaw-hook
make tidy          # go mod tidy

# Focused runs
go test ./internal/app/...
go test ./internal/app/... -run TestWithRestartPermanentFailure
go test -race ./...
go test -v ./internal/config/...
```

---

## Project Layout

```
internal/
  agent/           # Kind enum (KindOpenCode, KindClaudeCode, KindChat)
  app/             # WithRestart supervisor + PermanentFailure error type
  channel/         # Channel interface + Telegram implementation
  channel/telegram/
  claudecode/      # ClaudeCode service (runs claude -p subprocess)
  config/          # Config + Tuning structs, env parsing, validation
  conversation/    # Conversation history manager
  gateway/         # Telegram message router + LLM chat loop
  hitl/            # Human-in-the-loop permission + question flows
  infra/           # CostGuard, Heartbeat, SOULLoader
  mcp/             # MCP server manager
  memory/          # Memory engine (soul + MEMORY.md + notes)
  opencode/        # OpenCode service (HTTP client for opencode serve)
  providers/       # LLM provider interface + OpenAI/Copilot/Codex impls
  scheduler/       # Cron-based job scheduler
  store/           # SQLite store (sessions, hitl_pending, cost history)
  tools/           # Web search + web fetch tools
cmd/
  gistclaw/        # main binary
  gistclaw-hook/   # Claude Code hook helper binary
docs/plans/        # implementation blueprints (Plans 1-10)
SOUL.md            # bot personality loaded at runtime
sample.env         # reference env — copy to .env and fill in values
```

---

## Code Style

### Packages
Short, lowercase, single word. No plurals, no underscores.

### Imports
Two groups separated by a blank line — stdlib first, then third-party. Internal packages go in a third group when both third-party and internal are present.

```go
import (
    "context"
    "fmt"

    "github.com/rs/zerolog/log"

    "github.com/canhta/gistclaw/internal/config"
)
```

### Naming

| Item | Convention | Example |
|---|---|---|
| Exported type | PascalCase | `PermanentFailure` |
| Unexported func | camelCase | `restartDelay` |
| Enum type | `type Kind int` | — |
| Enum constants | `Kind<Name>` prefix | `KindOpenCode` |
| Receiver | Single letter matching type | `c` for `Config` |
| Test func | `Test<Subject><Scenario>` | `TestWithRestartPermanentFailure` |

### Types
- Custom integer enums: `type Kind int`
- `time.Duration` for all timeouts — never raw `int64` seconds
- Struct env tags: `env:"VAR_NAME"`, `envDefault:"value"`, `envSeparator:","`
- Computed behaviour as methods: `Config.OperatorChatID()`, `Config.HasSearchProvider()`

### Error handling
- Always wrap with package prefix: `fmt.Errorf("config: parse env: %w", err)`
- Custom errors implement `Unwrap() error`
- `errors.As` for typed matching, `errors.Is` for sentinels
- Accumulate validation errors as `[]string`, then join
- Context cancellation is clean shutdown — return `nil`

### Logging
```go
log.Warn().
    Str("service", name).
    Err(err).
    Int("attempt", attempts).
    Msg("service crashed, restarting")
```
No `fmt.Sprintf` inside log calls. Level via `LOG_LEVEL` env var (default: `info`).

### Design patterns
- All service boundaries are Go interfaces; concrete types are unexported
- Decorator pattern for cross-cutting concerns (e.g. `trackingProvider`)
- No global state; `Config` passed by value; no `init()` side effects
- All timeouts in `config.Tuning` — no magic constants

---

## Test Conventions

- External test packages: `package foo_test` — always black-box
- Table-driven tests with `t.Run`
- Stdlib `testing` only — no third-party frameworks
- `t.Setenv()` for single vars; `os.Clearenv()` for full isolation in config tests
- `sync/atomic` for concurrency counters — never raw `int` with a mutex
- Red-green-commit TDD cycle per `docs/plans/`

---

## Configuration

Required env vars (no defaults):
- `TELEGRAM_TOKEN`
- `ALLOWED_USER_IDS` (comma-separated int64s)
- `OPENCODE_DIR`
- `CLAUDE_DIR`

Key optional vars:
- `LLM_PROVIDER` — `openai-key` (default) | `copilot` | `codex-oauth`
- `OPENAI_API_KEY` — required when `LLM_PROVIDER=openai-key`
- `LOG_LEVEL` — `debug` | `info` | `warn` | `error` (default: `info`)
- `SQLITE_PATH` — default `./gistclaw.db`
- `SOUL_PATH` — default `./SOUL.md`
- `OPENCODE_PORT` — default `8766`
- `OPENCODE_SERVER_USERNAME` / `OPENCODE_SERVER_PASSWORD` — Basic Auth for opencode serve

See `sample.env` for the full annotated list.

---

## Module

- Module: `github.com/canhta/gistclaw`
- Go: 1.25
- Key deps: `github.com/rs/zerolog`, `github.com/caarlos0/env/v11`
- Do not add new deps without a plan in `docs/plans/`
