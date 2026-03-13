# Roadmap

This document describes what has been built and where the project is headed.
Dates are not promised — this is a direction, not a schedule.

---

## Done

- Single Go binary with five supervised services under `errgroup`
- Telegram long-poll gateway with `update_id` deduplication
- OpenCode integration (HTTP + SSE)
- Claude Code integration (subprocess + hook helper)
- Human-in-the-loop (HITL) approval and question flows
- Plain chat mode with `web_search`, `web_fetch`, and MCP tools
- Multi-agent tools (`spawn_agent`, `run_parallel`, `chain_agents`)
- Memory engine (SOUL, MEMORY.md, daily notes)
- Conversation manager with proactive summarisation
- Formal tool engine with doom-loop guard
- Cron/at/every scheduler with SQLite-backed missed-job recovery
- MCP server manager (stdio + SSE/HTTP)
- Daily cost cap with soft-stop notifications
- Two-tier heartbeat with auto-restart
- Three LLM providers: `openai-key`, `copilot`, `codex-oauth`
- Four search providers: Brave, Gemini, xAI Grok, Perplexity
- systemd deployment unit
- `/start`, `/help`, `/status`, `/oc`, `/cc`, `/stop` commands

---

## Ideas / Under Consideration

These are possibilities, not commitments. Open an issue to discuss any of them.

### Additional channels
- Discord adapter implementing `channel.Channel`
- Slack adapter implementing `channel.Channel`

### Additional LLM providers
- Anthropic direct API (claude-3-x models without the CLI wrapper)
- Local Ollama server

### Agent improvements
- Session history browser (`/history`, `/sessions`)
- Per-project session isolation (multiple `OPENCODE_DIR` targets)
- File upload to agent context via Telegram attachments

### Operational
- Prometheus metrics endpoint (`/metrics`)
- Structured log export to external sinks (Loki, Datadog)
- First-class versioned releases with pre-built binaries

### Developer experience
- Docker image for easier local development
- Integration test suite against a mock Telegram server

---

## Not Planned

- Web UI — this is intentionally a Telegram-native tool
- Multi-user / multi-tenant mode — single-operator design is a feature, not a limitation
- Windows support — targets Linux VPS deployments
