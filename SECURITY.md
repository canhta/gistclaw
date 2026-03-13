# Security Policy

## Supported Versions

GistClaw does not yet use semantic versioning. Only the current `main` branch
is actively maintained and receives security fixes.

| Version | Supported |
|---|---|
| `main` | Yes |
| Any tagged release | Only if it matches current `main` |

---

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Please report security vulnerabilities via
[GitHub private security advisories](https://github.com/canhta/gistclaw/security/advisories/new).

Include as much of the following as possible:

- A description of the vulnerability and its potential impact
- The affected component(s) (e.g. `internal/hitl`, `internal/channel/telegram`)
- Steps to reproduce or a proof-of-concept
- Any suggested mitigations

---

## What to Expect

- **Acknowledgement** within 48 hours of receiving your report.
- **Initial assessment** (confirmed / not confirmed) within 7 days.
- **Fix and disclosure** within 30 days for critical vulnerabilities; 90 days
  for non-critical.
- Credit in the release notes if you wish to be acknowledged.

---

## Scope

GistClaw is a single-operator tool — it is designed to run on a private VPS
and accept connections only from a whitelist of Telegram user IDs. Issues
relevant to this threat model include:

- Bypassing the `ALLOWED_USER_IDS` access control
- Arbitrary command execution via crafted Telegram messages
- Credential or token leakage (`.env`, SQLite database)
- HITL approval bypass (causing an agent to execute unapproved actions)

Issues that are out of scope:

- Vulnerabilities requiring physical access to the VPS
- Telegram platform vulnerabilities (report to Telegram)
- OpenCode or Claude Code vulnerabilities (report to their respective maintainers)
