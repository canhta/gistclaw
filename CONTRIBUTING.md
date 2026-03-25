# Contributing to GistClaw

Start with [AGENTS.md](/Users/canh/Projects/OSS/gistclaw/AGENTS.md) for project policies and [docs/system.md](/Users/canh/Projects/OSS/gistclaw/docs/system.md) for the current package map.

## Prerequisites

- Go 1.25 or later
- `make`
- `curl` (used by `make dev` when installing `golangci-lint`)

## Building

Bootstrap repo-local developer tools once:

Run: `make dev && make hooks-install`

Build the binary:

Run: `go build -o bin/gistclaw ./cmd/gistclaw`

The binary is output to `bin/gistclaw`. The module path is `github.com/canhta/gistclaw`.

## Running Locally

Create a config file at `~/.config/gistclaw/config.yaml` or pass `-c /path/to/config.yaml`.

The minimum useful config is:

```yaml
workspace_root: /absolute/path/to/repo
provider:
  name: anthropic
  api_key: REPLACE_WITH_REAL_KEY
```

Then start the daemon and local web UI:

Run: `go run ./cmd/gistclaw serve`

With the default config, the web UI listens on `127.0.0.1:8080`.

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

Run `make fmt` before committing. The pre-commit hook runs `goimports` and `golangci-lint` automatically via lefthook.

Before handing work off, run:

- `make lint`
- `go test ./...`
- `go test -cover ./...`

## Submitting changes

This repository stays on `main`; do not create feature branches or worktrees here. If you are contributing from a fork, open a pull request from your fork's `main` into this repository's `main`.

Ensure `go test ./...` passes and `make lint` reports no issues before requesting review.

Issues, questions, and focused documentation improvements are welcome.
