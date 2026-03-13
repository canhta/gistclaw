# Changelog

All notable changes to GistClaw are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased]

---

## [0.1.0] — 2026-03-14

Initial working release. All core features implemented and running in production.

### Added

- **Telegram gateway** — long-poll message router with `update_id` deduplication via SQLite
- **OpenCode integration** — spawns `opencode serve`, submits tasks via REST API, streams SSE responses back to Telegram
- **Claude Code integration** — runs `claude -p` subprocess, streams JSON output, hook helper binary (`gistclaw-hook`) for tool approval
- **Human-in-the-loop (HITL)** — inline keyboard approval and sequential question flows for both agents; auto-reject with reminder on timeout
- **Plain chat mode** — any non-command message is routed to a configurable LLM with `web_search` and `web_fetch` tools
- **Multi-agent tools** — `spawn_agent`, `run_parallel`, `chain_agents` tools available in chat mode
- **Memory engine** — `SOUL.md` system prompt, `MEMORY.md` long-term memory, daily notes; `remember` and `note` tools
- **Conversation manager** — proactive LLM summarisation to keep context windows bounded
- **Tool engine** — formal registry with Core / Agent / System categories; doom-loop guard
- **Scheduler** — cron/at/every job types targeting any agent kind; SQLite-backed, missed-job recovery
- **MCP support** — stdio and SSE/HTTP MCP server manager; tools exposed in chat mode
- **Cost tracking** — daily USD cap with soft-stop notifications at 80% and 100%; per-provider cost reporting
- **Heartbeat** — Tier 1 (Telegram liveness) and Tier 2 (agent health) with auto-restart
- **Supervision model** — `WithRestart` wrapping all services; configurable budgets and windows per service
- **Three LLM providers** — `openai-key`, `copilot` (gRPC bridge), `codex-oauth` (PKCE)
- **Four search providers** — Brave, Gemini, xAI Grok, Perplexity; auto-detected from env
- **systemd unit** — `deploy/gistclaw.service` for production VPS deployment
- **`/start` and `/help` commands** — LLM-generated capability summary in the bot's own voice
- **`/status` command** — shows active session and today's cost

[Unreleased]: https://github.com/canhta/gistclaw/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/canhta/gistclaw/releases/tag/v0.1.0
