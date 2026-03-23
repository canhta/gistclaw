# Dependencies

> **Related:** [implementation-plan.md](implementation-plan.md) — file-by-file build plan | [12-go-package-structure.md](12-go-package-structure.md) — package ownership | [README.md](README.md) — doc index

## Go Version

**Minimum: Go 1.25**

`go.mod` must declare `go 1.25`. Use the latest patch release available at build time.

Rationale: Go 1.25 is required because `testing/synctest` is generally available and the stdlib continues to improve in areas we already use, including `net/http` and `sync`. No CGO is required; all dependencies are pure-Go.

---

## Direct Dependencies

### `modernc.org/sqlite` — latest

**What:** Pure-Go SQLite driver. No CGO, no system libraries required.

**Why this over `mattn/go-sqlite3`:** CGO-free means cross-compilation works without a C toolchain. Self-hosted local-first tool; no build complexity.

**Use for:** All SQLite access in `internal/store`. Open via `sql.Open("sqlite", path)`.

**Do not use:** `mattn/go-sqlite3`, `zombiezen.com/go/sqlite` (CGO or WASM overhead not needed).

---

### `go.yaml.in/yaml/v4`

**What:** YAML parsing and encoding.

**Why this over `gopkg.in/yaml.v3`:** v4 is the project default import path. Supports struct tags, strict decoding (`KnownFields(true)` to reject unknown fields), and inline marshaling.

**Use for:** Loading `config.yaml`, `team.yaml`, `*.soul.yaml`, and `identity.yaml` in `internal/app`, `internal/agents` (future split), and startup validation.

**Use `KnownFields(true)`** when decoding team and soul files — fail loudly on unknown fields.

---

## Standard Library — Key Packages

These are stdlib; no third-party equivalent is needed.

| Package | Use |
|---------|-----|
| `net/http` | HTTP server and SSE in `internal/web` |
| `html/template` | Server-rendered templates in `internal/web/templates/` |
| `database/sql` | SQL abstraction layer wrapping modernc.org/sqlite |
| `crypto/sha256` | Approval fingerprint: `sha256(tool_name + ":" + sorted_args_json + ":" + target_path)` |
| `encoding/json` | JSON encoding for receipts, export, and SSE event payloads |
| `os/exec` | Shell exec tool in `internal/tools/runner.go` |
| `testing` | All unit and integration tests |
| `net/http/httptest` | HTTP handler tests in `internal/web` |
| `context` | Cancellation and timeout propagation throughout |
| `log/slog` | Structured logging (Go 1.21+, stable in 1.25) |
| `sync` | `sync.Map` and `sync.RWMutex` for SSE subscriber map in `internal/web/sse.go` |
| `embed` | `//go:embed` for SQL migration files in `internal/store/migrations/` |
| `path/filepath` | Workspace path containment checks in `internal/tools/workspace.go` |
| `os` | File operations, signal handling, disk-space checks |

---

## Explicitly Excluded

| Package | Reason |
|---------|--------|
| `mattn/go-sqlite3` | Requires CGO — excluded for cross-compilation |
| `gorilla/mux` | Not needed; stdlib `net/http` ServeMux is sufficient for v1 |
| `gin`, `echo`, `fiber` | Not needed; no routing complexity justifies a framework |
| `gorm`, `sqlx`, `sqlc` | ORM/query builder — journal-first design uses raw SQL; ORM hides the append-only constraint |
| `viper` | Config loading is simple; `go.yaml.in/yaml/v4` + a config struct is sufficient |
| `cobra` | CLI is three subcommands; stdlib `flag` + `os.Args` switch is sufficient |
| `logrus`, `zap` | `log/slog` (stdlib, Go 1.21+) is sufficient |
| Any frontend JS framework | Templates are stdlib `html/template`; no JS build step in v1 |

---

## Telegram (Milestone 3 only)

No official Go SDK required. Telegram Bot API is a simple REST+polling API.

**Approach:** implement a minimal client in `internal/telegram/` using `net/http` directly.

- `getUpdates` with `timeout=30` for long polling
- `sendMessage` for outbound delivery

Do not add `go-telegram-bot-api` or any third-party Telegram SDK. The v1 surface is: started, blocked, approval-needed, finished — four message types, one polling loop. A dedicated SDK is unnecessary complexity.

---

## go.sum and Reproducibility

- Run `go mod tidy` after adding each dependency.
- Commit both `go.mod` and `go.sum`.
- Pin to a specific tag/commit for `modernc.org/sqlite` and `go.yaml.in/yaml/v4` — do not use `@latest` in go.mod; use an explicit version after `go get`.
- Use `GONOSUMCHECK` or the standard sum database — no private modules in v1.
