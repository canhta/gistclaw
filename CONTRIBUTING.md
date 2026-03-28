# Contributing to GistClaw

Start with [AGENTS.md](AGENTS.md) for project policies and [docs/system.md](docs/system.md) for the current package map.

## Prerequisites

- Go 1.25 or later
- `make`
- `curl` (used by `make dev-tools` when installing `golangci-lint`)
- `bun`

## Building

Bootstrap repo-local developer tools once:

Run: `make dev-tools && make hooks-install`

Build the binary:

Run: `go build -o bin/gistclaw ./cmd/gistclaw`

The binary is output to `bin/gistclaw`. The module path is `github.com/canhta/gistclaw`.

## Running Locally

Create a config file at `~/.config/gistclaw/config.yaml` or pass `-c /path/to/config.yaml`.

The minimum useful config is:

```yaml
storage_root: /absolute/path/to/gistclaw-storage
provider:
  name: anthropic
  api_key: REPLACE_WITH_REAL_KEY
```

Pick or create the working project through onboarding or the project switcher after the daemon starts; the config now stores only the daemon's `storage_root`, while run location is resolved from the active project and task context.

Then start the dev loop:

Run: `make dev`

With the default config, Air keeps the Go daemon on `127.0.0.1:8080`, Vite serves the frontend on `127.0.0.1:5173`, and Vite proxies `/api/*` back to Go.

## Testing

Run the full test suite:

Run: `go test ./...`

Run tests for a single package:

Run: `go test ./internal/store/...`

Run a single test:

Run: `go test -run TestFoo ./...`

Check coverage (minimum 70% required):

Run: `make coverage`

## Code style

Run `make fmt` before committing. The pre-commit hook runs `goimports` plus staged-file `golangci-lint --fast-only` checks for Go changes and `bun run lint && bun run check` for staged frontend changes. The pre-push hook runs full Go plus frontend verification through `make prepush`.

Before handing work off, run:

- `make lint`
- `go test ./...`
- `go test -cover ./...`
- `cd frontend && bun run check`
- `cd frontend && bun run lint`
- `cd frontend && bun run test:unit -- --run`
- `cd frontend && bun run build`

## Submitting changes

This repository stays on `main`; do not create feature branches or worktrees here. If you are contributing from a fork, open a pull request from your fork's `main` into this repository's `main`.

Ensure `go test ./...` passes and `make lint` reports no issues before requesting review.

Issues, questions, and focused documentation improvements are welcome.
