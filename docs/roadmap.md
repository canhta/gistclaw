# Roadmap

Direction, not a schedule. Open an issue to discuss anything here.

---

## What's built

See [CHANGELOG.md](../CHANGELOG.md) for the full feature list.

---

## Under consideration

**Channels:** Discord, Slack adapters

**LLM providers:** Anthropic direct API, local Ollama

**Agent improvements:**
- Session history browser (`/history`, `/sessions`)
- Per-project session isolation
- File upload via Telegram attachments

**Operations:** Prometheus metrics, structured log export (Loki/Datadog), versioned releases with pre-built binaries

**Dev experience:** Docker image, integration test suite against a mock Telegram server

---

## Not planned

- Web UI — intentionally Telegram-native
- Multi-user / multi-tenant — single-operator by design
- Windows support — targets Linux VPS
