# GistClaw

GistClaw is a local-first multi-agent runtime for software repo tasks.

It lets you hand one task to one assistant, let that assistant coordinate specialist workers behind the scenes, approve risky actions before they touch your repo, and inspect exactly what happened afterward.

Today the repo already ships a working daemon, CLI, local web control plane, replay model, memory store, provider adapters, tool registry, scheduled local tasks, and live connector paths for Telegram DM and WhatsApp.

## Why People Use It

- One assistant surface, with worker agents coordinated through runtime-managed sessions instead of ad hoc delegation.
- Risky edits stay reviewable because approvals gate workspace writes before they happen.
- Everything stays local-first: daemon, web UI, and database all run on your machine.
- Runs remain inspectable after the fact through replay, session history, route bindings, deliveries, and memory.
- Operators can start from the CLI, then move into a richer local control plane when recovery, approvals, or debugging matter.
- Repetitive local tasks can be scheduled durably in SQLite and recovered cleanly after restarts.
- Security, connector, and storage health can be audited from the operator CLI before drift turns into a recovery problem.

## What Ships Today

- `gistclaw serve` starts the daemon and local web host.
- `gistclaw version` prints the installed release identity.
- `gistclaw run`, `inspect`, `security audit`, `schedule`, `doctor`, `backup`, and `export` cover the operator CLI.
- `gistclaw inspect systemd-unit` prints the canonical service file used by the Ubuntu installer.
- The web UI includes onboarding plus operator-job pages for `Operate`, `Configure`, and `Recover`.
- GitHub Releases now carry the blessed Ubuntu installer path and Apple Silicon download path.
- Providers: Anthropic and OpenAI-compatible endpoints.
- Tools: built-in web fetch, optional Tavily search, optional MCP stdio tools.
- Live external surfaces: Telegram DM and WhatsApp.
- The repo includes a default team definition in [teams/default/team.yaml](teams/default/team.yaml).
- The Team page supports named per-project team profiles under `.gistclaw/teams/<profile>/`.

## Quick Start

### Public OSS Quick Start

Start from [GitHub Releases](https://github.com/canhta/gistclaw/releases).

- Ubuntu 24 VPS: [docs/install-ubuntu.md](docs/install-ubuntu.md)
- macOS Apple Silicon: [docs/install-macos.md](docs/install-macos.md)
- Recovery and rollback: [docs/recovery.md](docs/recovery.md)

The Ubuntu path installs a `systemd` service, and the binary itself can show the canonical unit:

```bash
gistclaw version
gistclaw inspect systemd-unit
```

### Contributor Quick Start

If you are developing from source, use [CONTRIBUTING.md](CONTRIBUTING.md) for the full setup. The shortest local loop is:

```bash
make dev && make hooks-install
go run ./cmd/gistclaw serve
```

Create `~/.config/gistclaw/config.yaml` using the minimal example in [CONTRIBUTING.md](CONTRIBUTING.md), then open `http://127.0.0.1:8080`.

## CLI Surface

```bash
gistclaw serve
gistclaw version
gistclaw run "fix the failing tests"
gistclaw inspect status
gistclaw inspect runs
gistclaw inspect replay <run_id>
gistclaw inspect systemd-unit
gistclaw inspect token
gistclaw security audit
gistclaw schedule add --name "Daily review" --objective "Inspect repository status" --at 2030-01-01T00:00:00Z
gistclaw schedule update <schedule_id> --objective "Inspect repository status after the update"
gistclaw schedule status
gistclaw schedule list
gistclaw schedule show <schedule_id>
gistclaw schedule run <schedule_id>
gistclaw schedule enable <schedule_id>
gistclaw schedule disable <schedule_id>
gistclaw schedule delete <schedule_id>
gistclaw doctor
gistclaw backup --db ~/.local/share/gistclaw/runtime.db
gistclaw export --db ~/.local/share/gistclaw/runtime.db --out export.json
```

`gistclaw inspect status` now includes storage-health fields such as database bytes, WAL bytes, free disk bytes, backup status, and any active storage warnings. `gistclaw doctor` now summarizes connector and storage health, and `gistclaw security audit` reports deployment-risk findings with exit codes that distinguish warn-only from failing posture.

## Build And Test

```bash
go build -o bin/gistclaw ./cmd/gistclaw
go test ./...
go test -cover ./...
go vet ./...
```

For repo-local tooling and contributor workflows:

```bash
make dev
make hooks-install
make fmt
make lint
make test
make coverage
```

The coverage floor remains `70%`.

## Documentation Map

Read these in order:

1. [docs/system.md](docs/system.md)
2. [docs/install-ubuntu.md](docs/install-ubuntu.md)
3. [docs/install-macos.md](docs/install-macos.md)
4. [docs/recovery.md](docs/recovery.md)
5. [docs/vision.md](docs/vision.md)
6. [docs/kernel.md](docs/kernel.md)
7. [docs/roadmap.md](docs/roadmap.md)
8. [docs/extensions.md](docs/extensions.md)
9. [AGENTS.md](AGENTS.md) (`CLAUDE.md` is a symlink to this file)
10. [CONTRIBUTING.md](CONTRIBUTING.md)
11. [CHANGELOG.md](CHANGELOG.md)

## Related Project

GistClaw grows out of lessons from [OpenClaw](https://github.com/openclaw/openclaw), but the current implementation is intentionally narrower, more local-first, and more explicit about runtime boundaries.

## License

MIT
