# Contributing to GistClaw

Thank you for your interest in contributing. This document explains how to get
set up, the conventions the project follows, and what to expect from the review
process.

---

## Prerequisites

- **Go 1.25+** — [https://go.dev/dl/](https://go.dev/dl/)
- **golangci-lint** — [https://golangci-lint.run/usage/install/](https://golangci-lint.run/usage/install/)
- **opencode** on `$PATH` — [https://opencode.ai](https://opencode.ai)
- **claude** CLI on `$PATH` — [https://docs.anthropic.com/en/docs/claude-code](https://docs.anthropic.com/en/docs/claude-code)
- A Telegram bot token and your numeric user ID (see [docs/testing.md](docs/testing.md))

---

## Fork, clone, branch

```bash
# Fork on GitHub, then:
git clone https://github.com/<your-fork>/gistclaw
cd gistclaw
git checkout -b feat/my-feature
```

Use short, descriptive branch names: `feat/discord-channel`, `fix/hitl-timeout`, `docs/roadmap`.

---

## Build and test

```bash
make build        # compiles ./bin/gistclaw and ./bin/gistclaw-hook
make test         # go test ./... -timeout 120s
make vet          # go vet ./...
make lint         # golangci-lint run ./...
```

Run tests with the race detector before opening a PR:

```bash
go test -race ./...
```

---

## Code style

GistClaw follows a strict set of Go conventions documented in [AGENTS.md](AGENTS.md).
Key points:

- **Imports:** two groups — stdlib first, then third-party + internal, separated by a blank line.
- **Naming:** `PascalCase` for exported types, `camelCase` for unexported functions. Enum constants use a `Kind<Name>` prefix.
- **Errors:** always wrap with a package prefix — `fmt.Errorf("config: parse env: %w", err)`.
- **Logging:** `zerolog` structured key-value chains — never `fmt.Sprintf` inside log calls.
- **Tests:** external test packages (`package foo_test`), stdlib `testing` only, table-driven, `t.Setenv` for env isolation.

Read [AGENTS.md](AGENTS.md) in full before writing code.

---

## Commit messages

Follow the existing style visible in `git log`:

```
feat(pkg): short imperative description
fix(pkg): short imperative description
docs: short description
chore: short description
refactor(pkg): short description
test(pkg): short description
```

Keep the subject line under 72 characters. Reference issues in the body if relevant.

---

## Opening a pull request

1. Make sure `make test` and `make lint` both pass locally.
2. Keep PRs focused — one logical change per PR.
3. Fill in the PR template completely.
4. Link the issue your PR addresses (if any).
5. Be prepared to iterate — reviews focus on correctness, style, and architectural fit.

---

## Reporting issues

Use the GitHub issue templates:
- **Bug report** — for crashes, wrong behaviour, or regressions.
- **Feature request** — for new ideas or improvements.

Before opening an issue, search existing issues to avoid duplicates.

---

## Questions

Open a GitHub Discussion or start a GitHub issue labelled `question`.
