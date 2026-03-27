# GistClaw

GistClaw is a local-first multi-agent runtime for software repo tasks.

It gives you one assistant surface, lets that assistant coordinate specialist workers behind the scenes, asks for approval before risky repo changes, and keeps a replayable record of what happened.

Today the repo already ships a working daemon, CLI, local web control plane, replay model, memory store, provider adapters, tool registry, scheduled local tasks, and live Telegram DM, WhatsApp, and optional Zalo Personal DM paths.

## Why It Exists

- Keep state, approvals, and replay on your own machine.
- Start with one assistant surface, but let the runtime manage real worker sessions underneath.
- Recover and debug from facts in the journal, not from guesswork.
- Move between CLI and local web controls without switching systems.

## What Ships Today

- `gistclaw serve` starts the daemon and local web host.
- `gistclaw version` prints the installed release identity.
- `gistclaw auth set-password` bootstraps or rotates the built-in browser login for VPS operators.
- `gistclaw auth zalo-personal login`, `gistclaw auth zalo-personal logout`, and `gistclaw auth zalo-personal contacts` manage optional Zalo Personal connector credentials and DM target lookup through the CLI.
- `gistclaw run`, `inspect`, `security audit`, `schedule`, `doctor`, `backup`, and `export` cover the operator CLI.
- `gistclaw inspect systemd-unit` prints the canonical service file used by the Ubuntu installer.
- `gistclaw inspect token` prints the admin token stored in the runtime settings table.
- The web UI includes a built-in login gate plus onboarding and operator-job pages for `Operate`, `Configure`, and `Recover`.
- GitHub Releases now carry a self-contained binary for the blessed Ubuntu installer path and Apple Silicon download path.
- The Ubuntu installer supports either a quick-start provider key path or an exact `--config-file` reinstall path for VPS operators, plus optional `--public-domain` Caddy bootstrap.
- Providers: Anthropic and OpenAI-compatible endpoints.
- Tools: built-in web fetch, optional Tavily search, optional MCP stdio tools.
- Live external surfaces: Telegram DM, WhatsApp, and optional unofficial Zalo Personal DM.
- The repo includes a default team definition in [teams/default/team.yaml](teams/default/team.yaml).
- The Team page supports named per-project team profiles under `storage_root/projects/<project-id>/teams/<profile>/`, with the machine default under `storage_root/teams/default/`.

## Quick Start

### Public OSS Quick Start

Start from [GitHub Releases](https://github.com/canhta/gistclaw/releases).

- Ubuntu 24 VPS: [docs/install-ubuntu.md](docs/install-ubuntu.md)
- macOS Apple Silicon: [docs/install-macos.md](docs/install-macos.md)
- Recovery and rollback: [docs/recovery.md](docs/recovery.md)

The Ubuntu path installs a `systemd` service, and the binary itself can show the canonical unit:

```bash
gistclaw version
gistclaw auth set-password
gistclaw inspect systemd-unit
gistclaw inspect token
```

### Contributor Quick Start

If you are developing from source, use [CONTRIBUTING.md](CONTRIBUTING.md) for the full setup. The shortest local loop is:

```bash
make dev && make hooks-install
go run ./cmd/gistclaw serve
```

Create `~/.config/gistclaw/config.yaml` using the minimal example in [CONTRIBUTING.md](CONTRIBUTING.md), then open `http://127.0.0.1:8080`.

## Common Commands

```bash
gistclaw serve
gistclaw auth set-password
gistclaw auth zalo-personal login
gistclaw auth zalo-personal contacts
gistclaw run "fix the failing tests"
gistclaw inspect status
gistclaw inspect replay <run_id>
gistclaw inspect systemd-unit
gistclaw inspect token
gistclaw security audit
gistclaw schedule --help
gistclaw doctor
gistclaw backup --db ~/.local/share/gistclaw/runtime.db
gistclaw export --db ~/.local/share/gistclaw/runtime.db --out export.json
```

Use `gistclaw help`, `gistclaw inspect --help`, and `gistclaw schedule --help` for the full command surface. `gistclaw inspect status` includes storage-health details, `gistclaw doctor` summarizes connector and storage health, and `gistclaw security audit` reports deployment-risk findings with distinct exit codes for warnings vs failures. Zalo Personal remains an unofficial, DM-only connector with CLI-driven authentication.

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

## Documentation Guide

- Start with [docs/system.md](docs/system.md) if you want the shipped surface and package ownership.
- Use [docs/install-ubuntu.md](docs/install-ubuntu.md), [docs/install-macos.md](docs/install-macos.md), and [docs/recovery.md](docs/recovery.md) if you want to run a release build.
- Use [docs/vision.md](docs/vision.md), [docs/kernel.md](docs/kernel.md), [docs/roadmap.md](docs/roadmap.md), and [docs/extensions.md](docs/extensions.md) if you want product direction and runtime rules.
- Use [AGENTS.md](AGENTS.md) (`CLAUDE.md` is a symlink) and [CONTRIBUTING.md](CONTRIBUTING.md) if you want to work in the repo.
- Use [CHANGELOG.md](CHANGELOG.md) for release history.

## Related Project

GistClaw grows out of lessons from [OpenClaw](https://github.com/openclaw/openclaw), but the current implementation is intentionally narrower, more local-first, and more explicit about runtime boundaries.

## License

MIT
