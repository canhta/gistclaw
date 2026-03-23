# OpenClaw Reverse-Engineering Research Plan

## Mission

Reverse-engineer how OpenClaw actually works from the local repository, then cross-check that implementation against the official OpenClaw docs and design a much lighter replacement.

This is explicitly a code-first pass. The repository is the source of truth. The docs are only the intended model until verified in code.

## Evidence Standard

Every subsystem write-up must use:

- real code paths
- real symbols, types, and tests
- real runtime/state directories
- real config keys
- real docs pages or claims

Every subsystem investigation must answer:

1. What problem the subsystem is trying to solve
2. How it is actually implemented
3. What is genuinely strong
4. What is over-engineered
5. What is fake abstraction vs useful abstraction
6. What is confusing, risky, or hard to maintain
7. What should be simplified
8. What should be deleted
9. Where docs and code disagree
10. What the lightweight redesign should do instead

## Source-of-Truth Order

1. Local repository code
2. Local tests
3. Local docs pages
4. Official docs site pages
5. Inference, clearly labeled

## Investigation Split

The work is intentionally split into independent passes instead of one monolithic sweep.

### Parallel investigation ownership

1. Runtime + Gateway
   - Entry points
   - Gateway bootstrap and lifecycle
   - Bind/auth/token handling
   - WebSocket control plane
   - Config load/reload/restart

2. Agent Runtime + Sessions
   - Embedded runtime
   - Session keying/store/delivery
   - Queueing and streaming
   - Transcript persistence
   - Compaction

3. Memory + Soul
   - Workspace bootstrap files
   - Memory file layout
   - Prompt assembly
   - Soul/identity separation quality

4. Multi-Agent + Delegation
   - Agent routing and bindings
   - agentDir/workspace/session isolation
   - Subagents, ACP, thread binding, on-behalf-of behavior

5. Tools + Skills + Plugins + Security
   - Built-in tools
   - Plugin-provided tools
   - Skills loading
   - Sandbox/tool policy/approvals
   - Trust boundaries and prompt injection blast radius

6. Providers + Channels + Interfaces
   - LLM provider abstraction
   - Channel/account/binding model
   - Message normalization
   - Media/attachment flow

7. Storage + Ops Complexity
   - State layout
   - Credentials/logs/transcripts
   - Hooks/cron/poll/webhook/heartbeat
   - Debugging and deployment burden

8. Synthesis
   - Docs-vs-code map
   - Strengths and weaknesses
   - Clean-slate Go redesign

## Required Judgments

Each subsystem will be judged through these lenses:

- single-binary feasibility
- Go-first practicality
- runtime simplicity
- package clarity
- dependency weight
- control-plane complexity
- config complexity
- debugging difficulty
- operational burden
- performance and resource usage
- multi-agent quality
- memory quality
- soul/identity quality
- provider abstraction quality
- app/channel abstraction quality
- tool safety
- sandbox realism
- plugin risk
- persistence design quality
- developer ergonomics
- end-user clarity

## Deliverables

This research produces the following files:

- `docs/01-docs-vs-code-map.md`
- `docs/02-runtime-and-gateway.md`
- `docs/03-agent-runtime-and-sessions.md`
- `docs/04-memory-and-soul.md`
- `docs/05-multi-agent-and-delegation.md`
- `docs/06-tools-skills-plugins-security.md`
- `docs/07-providers-channels-and-interfaces.md`
- `docs/08-storage-config-and-ops.md`
- `docs/09-openclaw-strengths.md`
- `docs/10-openclaw-weaknesses.md`
- `docs/11-architecture-redesign.md`
- `docs/12-go-package-structure.md`
- `docs/13-core-interfaces.md`
- `docs/14-memory-model.md`
- `docs/15-security-model.md`
- `docs/16-roadmap-and-kill-list.md`

## Research Rules

- Do not preserve OpenClaw patterns by default
- Do not optimize for backward compatibility
- Do not stay close to the current system unless evidence justifies it
- Prefer deletion over patching abstractions that are not earning their keep
- Prefer operational simplicity over framework generality
- Prefer narrow, explicit seams over giant pluggable layers
- Treat security boundaries as architecture, not polish

## Redesign End State

The redesign target is a single-binary, Go-first, multi-agent runtime with:

- a compact startup model
- explicit delegation
- layered memory
- separate soul vs memory vs runtime policy
- narrow connector interfaces
- safer-by-default tool execution
- clear observability
- simpler failure modes

The redesign phase starts only after the subsystem findings and docs-vs-code mismatches are written down.
