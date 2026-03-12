# AGENTS.md — GistClaw Coding Agent Guide

GistClaw is a pure-Go single binary that acts as a Telegram-driven controller for AI
coding agents (OpenCode, Claude Code). It is implemented in ten incremental plans
located in `docs/plans/`. Only Plan 1 (foundation packages) is complete as of writing.

---

## Build & Test Commands

```bash
# Run all tests
go test ./...
make test          # alias

# Run a single package
go test ./internal/app/...

# Run a single test by name (regex-matched)
go test ./internal/app/... -run TestWithRestartPermanentFailure

# Run tests with race detector (required for concurrency work)
go test -race ./...

# Run tests verbosely
go test -v ./internal/config/...

# Build the binary (once cmd/gistclaw/main.go exists)
go build ./...

# Tidy dependencies
go mod tidy
make tidy          # alias
```

No lint tooling (golangci-lint, staticcheck) is configured yet. When added it will
appear in the Makefile. Until then, `go vet ./...` is the available static check.

---

## Project Layout

```
internal/
  agent/      # Kind enum (KindOpenCode, KindClaudeCode, KindChat)
  app/        # WithRestart supervisor + PermanentFailure error type
  config/     # Config + Tuning structs, env parsing, validation
  (planned)   # gateway, channel/telegram, opencode, claudecode,
              # hitl, scheduler, providers, infra, tools, mcp, store
cmd/
  gistclaw/        # main binary (planned)
  gistclaw-hook/   # webhook helper (planned)
docs/plans/        # implementation blueprints (Plans 1-10)
```

---

## Code Style

### Package Names
- Short, lowercase, single word: `agent`, `app`, `config`.
- No plural forms, no underscores.

### Imports
Organize in exactly **two groups**, separated by a blank line:
1. Standard library
2. Third-party packages

```go
import (
    "context"
    "fmt"
    "time"

    "github.com/rs/zerolog/log"
)
```

Never mix stdlib and third-party in the same group. Internal packages (`github.com/canhta/gistclaw/internal/...`) go in a third group when both third-party and internal are present.

### Naming Conventions
| Item | Convention | Example |
|------|-----------|---------|
| Exported type | PascalCase | `PermanentFailure`, `Config`, `Tuning` |
| Unexported func | camelCase | `validate`, `restartDelay` |
| Enum type | `type Kind int` with `iota` | `KindOpenCode`, `KindClaudeCode` |
| Enum constants | `Kind<Name>` prefix | `KindOpenCode = iota` |
| Receiver name | Single letter matching type initial | `k` for `Kind`, `c` for `Config` |
| Test functions | `Test<Subject><Scenario>` | `TestWithRestartPermanentFailure` |

### Types
- Use custom integer types for enums: `type Kind int`.
- Use `time.Duration` for all timeout/TTL values (never raw `int64` seconds).
- Struct tags for env parsing: `env:"VAR_NAME"`, `envDefault:"value"`, `envSeparator:","`.
- Expose computed behaviour as methods on structs rather than extra fields
  (e.g. `Config.OperatorChatID()`, `Config.HasSearchProvider()`).

### Error Handling
- Always wrap with a package-name prefix: `fmt.Errorf("config: parse env: %w", err)`.
- Custom error structs must implement `Unwrap() error` for chain support.
- Use `errors.As` for typed matching; `errors.Is` for sentinel matching.
- Accumulate multiple validation errors as `[]string`, then join:
  ```go
  var errs []string
  errs = append(errs, "TELEGRAM_TOKEN is required")
  return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
  ```
- Context cancellation (`ctx.Err() != nil`) is **clean shutdown** — return `nil`, not an error.

### Logging
Use `github.com/rs/zerolog/log` with structured key-value chaining. Never use
`fmt.Sprintf` inside log calls.

```go
log.Warn().
    Str("service", name).
    Err(err).
    Int("attempt", attempts).
    Msg("service crashed, restarting")
```

Log level is configured via `LOG_LEVEL` env var (default: `"info"`).

### Design Patterns
- **Interface-driven:** all service boundaries are Go interfaces. Concrete types are
  unexported; constructors return the interface.
- **Decorator pattern:** e.g. `trackingProvider` wraps `LLMProvider` for cost tracking.
- **No global state:** `Config` is passed by value; no `init()` side effects.
- **No magic constants:** all timeouts live in `config.Tuning`.

---

## Test Conventions

- **External test packages:** always `package agent_test`, `package app_test`, etc.
  (black-box testing — never `package agent`).
- **Table-driven tests:**
  ```go
  cases := []struct {
      name  string
      input string
      want  Kind
  }{
      {"opencode", "opencode", KindOpenCode},
  }
  for _, tc := range cases {
      t.Run(tc.name, func(t *testing.T) { ... })
  }
  ```
- **No third-party test frameworks.** Use stdlib `testing` only. Assert with
  `t.Fatalf` / `t.Errorf` directly.
- **Env isolation:** use `t.Setenv()` for single vars (auto-cleaned); `os.Clearenv()`
  for complete isolation in config tests.
- **Concurrency counters:** use `sync/atomic` (`atomic.Int32`) — never a raw `int` with
  a mutex in tests.
- Follow the **red-green-commit** TDD cycle per `docs/plans/`:
  1. Write the failing test.
  2. Implement the minimal code to make it pass.
  3. Verify `go test ./...` is green.
  4. Commit with the message prescribed in the plan.

---

## Configuration

All runtime config is loaded from environment variables via
`github.com/caarlos0/env/v11` into `internal/config.Config`.

Required vars (no defaults):
- `TELEGRAM_TOKEN`
- `ALLOWED_USER_IDS` (comma-separated int64s)
- `OPENCODE_DIR`
- `CLAUDE_DIR`

Key optional vars:
- `LLM_PROVIDER` — `openai-key` (default) | `copilot` | `codex-oauth`
- `OPENAI_API_KEY` — required when `LLM_PROVIDER=openai-key`
- `LOG_LEVEL` — `debug` | `info` | `warn` | `error` (default: `info`)
- `SQLITE_PATH` — default `./gistclaw.db`
- `SOUL_PATH` — system-prompt markdown file (default `./SOUL.md`)

---

## Module & Toolchain

- Module: `github.com/canhta/gistclaw`
- Go: 1.25
- Key direct deps: `github.com/rs/zerolog`, `github.com/caarlos0/env/v11`
- Planned deps (do not add without a plan): `modernc.org/sqlite`,
  `golang.org/x/sync/errgroup`, Telegram bot client, `github.com/adhocore/gronx`.

---

## Implementation Plans

All features are built incrementally via the plans in `docs/plans/`. Each plan is
self-contained, specifies exact file paths, exact test names, and a commit message.
Before implementing anything beyond Plan 1, read the relevant plan file first.
