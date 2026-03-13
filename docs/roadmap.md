# Roadmap

Direction, not a schedule. Open an issue to discuss anything here.

Legend: `done` · `todo`

---

| Status | Item |
|---|---|
| `done` | Telegram gateway (long-poll, SQLite dedup) |
| `done` | OpenCode integration (HTTP + SSE) |
| `done` | Claude Code integration (subprocess + hook helper) |
| `done` | Human-in-the-loop approvals and question flows |
| `done` | Plain chat with `web_search`, `web_fetch`, MCP tools |
| `done` | Memory engine (`SOUL.md`, `MEMORY.md`, daily notes) |
| `done` | Scheduler (cron/at/every, SQLite, missed-job recovery) |
| `done` | Cost tracking (daily cap, soft-stop at 80% + 100%) |
| `done` | Supervision model (`WithRestart`, two-tier heartbeat) |
| `done` | Three LLM providers: `openai-key`, `copilot`, `codex-oauth` |
| `done` | Four search providers: Brave, Gemini, Grok, Perplexity |
| `done` | systemd deployment unit |
| `todo` | Hard iteration cap on plain chat tool loop |
| `todo` | LLM error classification (terminal / rate-limit / retryable) |
| `todo` | Proactive context summarisation before token limit |
| `todo` | Integration test suite against mock Telegram server |
| `todo` | Pre-built binaries via CI |
| `todo` | Session history browser (`/history`, `/sessions`) |
| `todo` | Per-project session isolation (multiple `OPENCODE_DIR` targets) |
| `todo` | File upload to agent context via Telegram attachments |
| `todo` | Discord channel adapter |
| `todo` | Slack channel adapter |
| `todo` | Anthropic direct API provider |
| `todo` | Local Ollama provider |
| `todo` | Prometheus metrics endpoint |
| `todo` | Docker image for local development |

---

## Not planned

- **Web UI** — intentionally Telegram-native
- **Multi-user / multi-tenant** — single-operator by design
- **Windows** — targets Linux VPS
