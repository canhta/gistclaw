# GistClaw

GistClaw is a personal assistant platform built on a local-first multi-agent runtime.

One front agent owns the user relationship. It can spawn worker agents behind the scenes, coordinate them through explicit runtime sessions, ask before risky actions, and keep a replayable record of what happened.

## Product Shape

- assistant-first at the surface
- multi-agent under the hood
- local-first state and control
- approvals before risky side effects
- replayable runtime history

GistClaw is not repo-task software with some agents attached. Repo work is one important workflow on the platform, not the platform's identity.

## Current Status

The repository is in an active reset.

Current work is focused on:

- deleting the old mixed doc set
- replacing rigid delegation with session-based collaboration
- rewriting the runtime around one front agent plus spawned worker sessions
- keeping the extension story explicit without rebuilding OpenClaw breadth all at once

Read these docs in order:

1. [docs/vision.md](/Users/canh/Projects/OSS/gistclaw/docs/vision.md)
2. [docs/kernel.md](/Users/canh/Projects/OSS/gistclaw/docs/kernel.md)
3. [docs/roadmap.md](/Users/canh/Projects/OSS/gistclaw/docs/roadmap.md)
4. [docs/extensions.md](/Users/canh/Projects/OSS/gistclaw/docs/extensions.md)

## Build

```bash
go build -o bin/gistclaw ./cmd/gistclaw
go test ./...
go test -cover ./...
go vet ./...
```

## Development

```bash
make dev
make hooks-install
make fmt
make test
make coverage
```

## Related Project

GistClaw grows out of lessons from [OpenClaw](https://github.com/openclaw/openclaw), but the current rewrite is intentionally choosing a cleaner kernel and a narrower immediate scope.

## License

MIT
