# GistClaw Frontend

SvelteKit app for the GistClaw control deck. The frontend is built with `bun`, emits static assets into [`internal/web/appdist`](/Users/canh/Projects/OSS/gistclaw/internal/web/appdist), and is served by the Go daemon.

## Commands

```sh
bun install
bun run dev
bun run check
bun run lint
bun run format
bun run test:unit -- --run
bun run build
bun run preview -- --host 127.0.0.1 --port 4173
```

## Notes

- Static output is written to `../internal/web/appdist`.
- The Go runtime remains the authority for auth, API, SSE, scheduling, and orchestration state.
- Do not switch this workspace back to `npm`; `bun` is the required package manager for this repo.
