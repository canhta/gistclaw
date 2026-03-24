# Contributing to GistClaw

## Prerequisites

- Go 1.24 or later
- `make` (for bootstrapping dev tools)

## Building

Bootstrap repo-local developer tools once:

Run: `make dev && make hooks-install`

Build the binary:

Run: `go build -o bin/gistclaw ./cmd/gistclaw`

The binary is output to `bin/gistclaw`. The module path is `github.com/canhta/gistclaw`.

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

## Submitting changes

Open a pull request against `main`. Ensure `go test ./...` passes and `make lint` reports no issues before requesting review.

Issues, questions, and focused documentation improvements are welcome.
