# GistClaw

GistClaw is a local-first multi-agent runtime for software repo tasks.

It lets you hand one task to one assistant, let that assistant coordinate specialist workers behind the scenes, approve risky actions before they touch your repo, and inspect exactly what happened afterward.

Today the repo already ships a working daemon, CLI, local web control plane, replay model, memory store, provider adapters, tool registry, and live connector paths for Telegram DM and WhatsApp.

## Why People Use It

- One assistant surface, with worker agents coordinated through runtime-managed sessions instead of ad hoc delegation.
- Risky edits stay reviewable because approvals gate workspace writes before they happen.
- Everything stays local-first: daemon, web UI, and database all run on your machine.
- Runs remain inspectable after the fact through replay, session history, route bindings, deliveries, and memory.
- Operators can start from the CLI, then move into a richer local control plane when recovery, approvals, or debugging matter.

## What Ships Today

- `gistclaw serve` starts the daemon and local web host.
- `gistclaw run`, `inspect`, `doctor`, `backup`, and `export` cover the operator CLI.
- The web UI includes onboarding plus operator-job pages for `Operate`, `Configure`, and `Recover`.
- Providers: Anthropic and OpenAI-compatible endpoints.
- Tools: built-in web fetch, optional Tavily search, optional MCP stdio tools.
- Live external surfaces: Telegram DM and WhatsApp.
- The repo includes a default team definition in [teams/default/team.yaml](/Users/canh/Projects/OSS/gistclaw/teams/default/team.yaml).

## Quick Start

Create a config file at `~/.config/gistclaw/config.yaml`:

```yaml
provider:
  name: openai
  api_key: REPLACE_WITH_REAL_KEY
  models:
    strong: gpt-4o
web:
  listen_addr: 127.0.0.1:8080
research:
  provider: tavily
  api_key: REPLACE_WITH_REAL_KEY
mcp:
  servers: []
```

Only `provider` is required. `workspace_root` is optional: on first boot, GistClaw creates a starter project under `~/.gistclaw/projects/<name>` and keeps onboarding separate from workspace existence. If you omit `database_path`, GistClaw stores state at `~/.local/share/gistclaw/runtime.db`.

Start the daemon:

```bash
go run ./cmd/gistclaw serve
```

Then open `http://127.0.0.1:8080`. You can override the config path with `GISTCLAW_CONFIG` or `gistclaw -c /path/to/config.yaml ...`.

From there, you can keep the starter project, bind an existing repo, or create a new project elsewhere during onboarding. The shell project switcher changes the active project context without burying that job in Settings. You can then submit a task from the web UI or from the CLI and use the local operator surface for runs, approvals, session inspection, team configuration, memory, and delivery recovery.

## CLI Surface

```bash
gistclaw serve
gistclaw run "fix the failing tests"
gistclaw inspect status
gistclaw inspect runs
gistclaw inspect replay <run_id>
gistclaw inspect token
gistclaw doctor
gistclaw backup --db ~/.local/share/gistclaw/runtime.db
gistclaw export --db ~/.local/share/gistclaw/runtime.db --out export.json
```

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

1. [docs/system.md](/Users/canh/Projects/OSS/gistclaw/docs/system.md)
2. [docs/vision.md](/Users/canh/Projects/OSS/gistclaw/docs/vision.md)
3. [docs/kernel.md](/Users/canh/Projects/OSS/gistclaw/docs/kernel.md)
4. [docs/roadmap.md](/Users/canh/Projects/OSS/gistclaw/docs/roadmap.md)
5. [docs/extensions.md](/Users/canh/Projects/OSS/gistclaw/docs/extensions.md)
6. [AGENTS.md](/Users/canh/Projects/OSS/gistclaw/AGENTS.md) (`CLAUDE.md` is a symlink to this file)
7. [CONTRIBUTING.md](/Users/canh/Projects/OSS/gistclaw/CONTRIBUTING.md)
8. [CHANGELOG.md](/Users/canh/Projects/OSS/gistclaw/CHANGELOG.md)

## Related Project

GistClaw grows out of lessons from [OpenClaw](https://github.com/openclaw/openclaw), but the current implementation is intentionally narrower, more local-first, and more explicit about runtime boundaries.

## License

MIT
