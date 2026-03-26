# Changelog

## Unreleased

### Operator-facing

Release binaries now embed the web templates and static assets they need to boot outside a source checkout.

The Ubuntu installer now locks down `/etc/gistclaw/config.yaml` and hands `/var/lib/gistclaw` to the `gistclaw` service user before startup.

Live model output now streams through the runtime into run detail, session pages, and Telegram draft replies instead of only appearing at turn boundaries.

The tool surface now includes optional Tavily research and minimal MCP stdio loading, giving runs a broader read-only research path without changing the approval model.

### For contributors

Provider adapters now use the official Anthropic and OpenAI SDK clients.

Template layout and style coverage now have dedicated regression tests.

## v0.1.0 — 2026-03-26

### First public OSS release

You can now install GistClaw from GitHub Releases instead of building from source first.

This release includes:

- release archives for `darwin/arm64` and `linux/amd64`
- `SHA256SUMS.txt`
- an Ubuntu 24 installer with a blessed `systemd` path
- `gistclaw version`
- `gistclaw inspect systemd-unit`
- install, update, and recovery docs for operators
