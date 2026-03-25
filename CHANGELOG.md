# Changelog

## Unreleased

### Operator-facing

Live model output now streams through the runtime into run detail, session pages, and Telegram draft replies instead of only appearing at turn boundaries.

The tool surface now includes optional Tavily research and minimal MCP stdio loading, giving runs a broader read-only research path without changing the approval model.

### For contributors

Provider adapters now use the official Anthropic and OpenAI SDK clients.

Template layout and style coverage now have dedicated regression tests.

## v1.0.0 — 2026-03-24

### Milestone 4 — Stable 1.0

Recovery hardening, doctor/backup/export operator tooling, onboarding and approval UX polish, and documentation sync brought the project to a stable 1.0 release.

### Milestone 3 — Public Beta

Telegram DM approval integration, budget guard for token and cost limits, and multi-repo workspace support shipped, making the daemon ready for real daily use.

### Milestone 2 — Web UI and SSE

A live run stream via Server-Sent Events, a web UI covering run list, run detail, and approval tickets, and the admin token authentication layer shipped.

### Milestone 1 — Kernel Proof

Single-repo task execution, a durable append-only event journal, run receipts with token and cost accounting, and interrupted-run recovery on daemon restart shipped.
