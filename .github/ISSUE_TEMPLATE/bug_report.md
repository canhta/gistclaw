---
name: Bug report
about: Report a crash, wrong behaviour, or regression
title: "bug: "
labels: bug
assignees: ""
---

## Description

A clear and concise description of what the bug is.

## Steps to reproduce

1. Configure `.env` with `...`
2. Send Telegram message `...`
3. Observe `...`

## Expected behaviour

What you expected to happen.

## Actual behaviour

What actually happened. Include log output if relevant (`make logs VPS=...` or `journalctl -u gistclaw`).

## Environment

- GistClaw commit: <!-- git rev-parse --short HEAD -->
- Go version: <!-- go version -->
- OS: <!-- e.g. Ubuntu 22.04 -->
- LLM provider: <!-- openai-key / copilot / codex-oauth -->
- Agent: <!-- opencode / claudecode / both / neither -->

## Additional context

Anything else that might be relevant.
