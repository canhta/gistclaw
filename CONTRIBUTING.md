# Contributing to GistClaw

Thanks for contributing! Here's what you need to know.

---

## Setup

**Prerequisites:** Go 1.25+, `golangci-lint`, `opencode` and `claude` on `$PATH`.

```bash
git clone https://github.com/<your-fork>/gistclaw
cd gistclaw
git checkout -b feat/my-feature
make install-hooks   # installs pre-commit (vet + lint) and pre-push (test) hooks
```

For credential setup (Telegram token, user ID, etc.) see [docs/testing.md](docs/testing.md).

---

## Workflow

```bash
make build    # compile both binaries
make test     # go test ./... -timeout 120s
make lint     # go vet + golangci-lint
go test -race ./...   # run before opening a PR
```

---

## Code style

Follow [AGENTS.md](AGENTS.md). Key points:

- Imports: two groups — stdlib, then third-party + internal
- Errors: always wrap — `fmt.Errorf("config: parse env: %w", err)`
- Logging: zerolog structured chains, no `fmt.Sprintf` inside log calls
- Tests: `package foo_test`, stdlib `testing` only, table-driven

---

## Commits and PRs

Commit style:
```
feat(pkg): short description
fix(pkg): short description
docs: short description
chore: short description
```

Subject line under 72 chars. Reference issues in the body.

PRs: `make test` and `make lint` must pass; one logical change per PR; fill in the PR template.

---

## Issues and questions

Use the GitHub issue templates for bugs and feature requests. For questions, open a Discussion or an issue labelled `question`.
