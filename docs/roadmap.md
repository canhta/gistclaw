# Roadmap

## Current Operator Value

Right now GistClaw gives operators a local-first assistant runtime with approvals, replay, and a control plane that can inspect sessions, routes, deliveries, and memory after a task runs.

## Current Baseline

The repository already ships:

- a daemon and local web control plane
- journal-backed runs, sessions, approvals, replay, deliveries, and memory
- provider adapters for Anthropic and OpenAI-compatible endpoints
- built-in tools plus optional Tavily research and MCP loading
- live Telegram and WhatsApp connector paths
- a default team definition in `teams/default/`

## Near-Term Priorities

1. extend external surfaces beyond the current Telegram and WhatsApp coverage without breaking the session kernel
2. move team definition from repo-managed files toward operator-selectable or user-defined runtime composition
3. deepen the operator control plane around routing, recovery, and session collaboration
4. keep hardening tests, docs, and packaging around the shipped runtime surface

## Explicit Non-Goals Right Now

These remain out of scope for the current implementation slice:

- rebuilding the full OpenClaw channel matrix
- adding a plugin marketplace or installation UX
- broad automation expansion
- weakening the journal-first runtime contract to move faster at the surface

## Remaining Gap To Broader Assistant-Platform Behavior

The session kernel is in place, but the product still does not fully behave like the broader assistant platform it is aiming toward.

Remaining gaps:

- the broader channel and gateway matrix is still intentionally narrow
- teams are still mostly repo-managed rather than operator-created at runtime
- extension seams exist, but higher-level installation and sharing workflows do not
- the control plane is strong locally, but still not the full platform surface the vision describes

## Next Slice

The next slice should make the current system more operator-complete without reopening platform sprawl:

1. deepen route and delivery recovery without bypassing the journaled runtime
2. keep the session model central as more external surfaces are added
3. improve team management and operator ergonomics instead of adding breadth-first platform features
