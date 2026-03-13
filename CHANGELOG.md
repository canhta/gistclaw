# Changelog

Notable changes, newest first.

---

## 2026-03-14

- Telegram gateway (long-poll, SQLite dedup)
- OpenCode integration (HTTP + SSE streaming)
- Claude Code integration (subprocess + `gistclaw-hook` helper)
- Human-in-the-loop approvals and question flows; auto-reject on timeout
- Plain chat mode with `web_search`, `web_fetch`, MCP tools
- Multi-agent tools: `spawn_agent`, `run_parallel`, `chain_agents`
- Memory engine: `SOUL.md`, `MEMORY.md`, daily notes, `remember`/`note` tools
- Conversation manager with proactive summarisation
- Tool engine with Core / Agent / System categories and doom-loop guard
- Scheduler: cron/at/every, SQLite-backed, missed-job recovery
- MCP server manager (stdio + SSE/HTTP)
- Cost tracking: daily USD cap, soft-stop at 80% and 100%
- Heartbeat Tier 1 (Telegram liveness) + Tier 2 (agent health) with auto-restart
- Supervision model: `WithRestart` with configurable budgets per service
- LLM providers: `openai-key`, `copilot` (gRPC), `codex-oauth` (PKCE)
- Search providers: Brave, Gemini, xAI Grok, Perplexity (auto-detected from env)
- systemd deployment unit (`deploy/gistclaw.service`)
- `/start`, `/help`, `/status`, `/oc`, `/cc`, `/stop` commands
