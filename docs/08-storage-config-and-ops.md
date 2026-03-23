# Storage, Config, and Ops Complexity

## What this subsystem is trying to solve

OpenClaw is trying to be a real self-hostable runtime, not a demo:

- persistent config
- durable sessions
- auth and credentials
- automation
- service management
- logging and diagnostics
- recovery and migration

That seriousness is admirable. The operational model is too fragmented.

## State layout in practice

Important files:

- `src/config/paths.ts`
- `src/config/sessions/paths.ts`
- `src/config/sessions/store.ts`
- `src/agents/workspace.ts`
- `src/cron/store.ts`

The current state model sprawls across:

- config files
- workspace files
- per-agent state dirs
- session metadata JSON
- transcript JSONL
- cron stores and run logs
- sandbox roots
- credentials dirs
- auth-profile files
- device identity files
- temporary logs under `/tmp/openclaw`
- macOS service/runtime-specific locations

This is the sixth major design smell. Too many files matter.

## Compatibility and path resolution burden

Core file:

- `src/config/paths.ts`

OpenClaw supports:

- legacy state directories
- legacy config filenames
- multiple active-config candidates
- old and new session paths

That is careful engineering for compatibility. It is also exactly the sort of complexity a clean-slate replacement should refuse to inherit.

## Sessions, archives, and cleanup

Core files:

- `src/config/sessions/store.ts`
- `src/config/sessions/targets.ts`
- `src/gateway/session-archive.fs.ts`
- `src/commands/sessions-cleanup.ts`

Sessions are not just "one file per conversation." The system also handles:

- lock files
- store discovery
- archive retention
- disk-budget eviction
- repair and cleanup

That is robust, but it is not simple.

## Credential fragmentation

Core files:

- `src/agents/auth-profiles/store.ts`
- `src/pairing/pairing-store.ts`
- `src/infra/device-identity.ts`
- `src/infra/device-auth-store.ts`
- `src/agents/cli-credentials.ts`

There is no single credential registry. State is spread across:

- agent auth profiles
- pairing allowlists
- device identity
- device auth tokens
- imported external CLI credentials

That makes operator reasoning harder than it should be.

## Logging and observability

Core files:

- `src/logging/logger.ts`
- `src/config/io.ts`
- `src/cron/run-log.ts`
- `apps/macos/Sources/OpenClaw/LogLocator.swift`

Logging exists in many planes:

- primary runtime logs
- config-audit logs
- cron run logs
- session transcripts
- optional diagnostics logs
- macOS launchd/unified logs

This is not one logging story. It is several.

## Automation surface

Core files:

- `src/cron/service/ops.ts`
- `src/hooks/*`
- `src/gateway/hooks.ts`
- `extensions/msteams/src/polls.ts`

OpenClaw supports:

- cron
- hooks
- webhooks
- pollers
- heartbeat

These are real features, not placeholders. They also pile more statefulness into the main runtime.

One especially telling detail: some read paths can persist repaired cron state. Even "read" behavior is not reliably read-only.

## Deployment and service-management burden

Core files:

- `src/daemon/service.ts`
- `src/cli/daemon-cli/install.ts`
- `src/commands/doctor-gateway-services.ts`
- `apps/macos/Sources/OpenClaw/GatewayProcessManager.swift`

The product supports:

- foreground CLI run
- background daemon install
- macOS app-attached gateway management
- launchd
- systemd
- Windows scheduled tasks

This is far beyond the needs of a clean single-binary v1.

## What is genuinely strong

### 1. Hardening is real

Path containment, permissions, symlink rejection, and file-lock discipline are serious and worth copying.

### 2. Repair tooling exists

`doctor`, health checks, cleanup commands, and path audits are real operator features.

### 3. Migration is taken seriously

The repo does not assume clean state.

## What is over-engineered

### 1. Too many persistence formats

JSON config, JSON stores, JSONL transcripts, markdown workspace files, SQLite memory, platform logs, and extension-specific stores all coexist.

### 2. Too many runtime modes

Local, remote, paired, app-managed, daemon-managed, webhook-driven, and cron-driven behavior all sit on the same core.

### 3. Too much auto-magic

Repair, migration, attach-or-spawn, store discovery, and hidden writes make behavior harder to predict.

## Useful abstractions vs fake abstractions

Useful:

- one visible state root
- per-agent state separation
- explicit doctor/health flows

Fake or too expensive:

- many operational modes in one core product
- reads that mutate state
- multiple sources of truth for auth and runtime logs

## Docs vs code

Material mismatch:

- `docs/pi-dev.md` uses the wrong session-index path
- logging docs understate the number of real log planes
- cron docs understate that read flows can repair and write state
- macOS gateway docs do not fully surface attach-or-spawn behavior

Strong match:

- docs are correct that OpenClaw is operationally ambitious and durable

## Judgments

- Operational burden: high
- Debugging difficulty: high
- Persistence design quality: medium
- Single-binary practicality if simplified: high
- Current one-engineer-operability: medium-low

## What should be simplified

- one canonical state tree
- one primary database
- one credential registry
- one logging plane plus optional diagnostics
- one automation model in v1
- one service-management story outside the runtime binary

## What should be deleted

- legacy path auto-detection in the hot path
- multi-format persistence for core entities
- attach-or-spawn magic as a default runtime expectation
- read paths that can mutate durable state

## What the lightweight redesign should do instead

Use one data directory:

- `config.yaml`
- `runtime.db`
- `attachments/`
- `agents/<id>/soul.yaml`
- `agents/<id>/facts.md`
- `logs/current.jsonl`

Then keep one operational rule:

only the daemon writes runtime state; CLI tools inspect or request changes through the daemon.

This includes:

- local web UI mutations
- approval resolution
- Telegram-side approval actions
- team edits and memory edits
- interrupted-run reconciliation on process restart

Stable-release restart posture:

- daemon startup scans for in-flight runs left without a terminal event
- those runs become explicit `interrupted` runs with a visible stop reason
- the operator may choose resume or rerun from the local web UI or CLI
- the daemon must not auto-resume unfinished runs on boot
- "best effort" hidden recovery is out of scope until the system proves its boring path first

Stable-release config and spec reload posture:

- edits to `config.yaml`, team specs, `soul.yaml`, and tool-policy configuration apply only to new root runs
- a root run freezes the relevant execution snapshot at start
- child delegations inherit from that frozen root snapshot rather than re-reading live files mid-run
- active runs do not change behavior underfoot because an operator edited configuration during execution
- a root run also freezes one declared workspace root so previews, apply, and verification all target the same filesystem boundary

Stable-release automation posture:

- start runs from inbound events, webhooks, or explicit schedules only
- no free-roaming proactive background agents
- no hidden polling loops that can silently trigger model work
- idle status must stay trustworthy when nothing is scheduled or inbound

Later-phase repo integration posture:

- repo-task runs may eventually publish back to GitHub through an explicit operator action
- do not add an always-on GitHub app, webhook sync, or background repo bot to the stable release
