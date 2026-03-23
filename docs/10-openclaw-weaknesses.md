# OpenClaw Weaknesses

## 1. The gateway became a platform kernel

OpenClaw is simultaneously:

- assistant runtime
- daemon
- RPC control plane
- plugin host
- scheduler
- hook/webhook host
- control UI backend
- node transport

That is the root design failure.

## 2. Persistence is split across too many formats

Core entities are spread across:

- JSON config
- JSON session stores
- JSONL transcripts
- markdown workspace files
- SQLite memory
- platform-specific logs and service state

That fragmentation makes the system harder to reason about than it needs to be.

## 3. Docs and code diverge on important trust boundaries

The worst examples are not cosmetic:

- subagent inheritance is broader than documented
- `MEMORY.md` privacy is weaker in code than in docs
- transcripts are more mutable than append-only docs imply

That weakens operator trust.

## 4. Multi-agent behavior is too complicated

The namespacing story is good. The runtime story is not. Subagents, ACP, thread binding, steering, descendant tracking, and session control create too much machinery for what should be explicit child-task delegation.

## 5. Prompt assembly is overloaded

OpenClaw uses markdown files as:

- operator UX
- behavior source
- identity source
- memory source
- runtime prompt substrate

That is too much responsibility for raw file injection.

## 6. Security is fighting breadth, not narrowness

OpenClaw has real controls, but the default blast radius remains large because it allows too much:

- in-process plugins
- many built-in tools
- prompt-driven skills
- host exec
- optional sandboxing

The system is not careless. It is overpowered.

## 7. Channel and provider abstractions leak

The repo keeps building registries and adapters, then core reaches back in with channel-specific and provider-specific logic anyway. That is a sign the abstraction stack is not earning its weight.

## 8. Operational burden is high

OpenClaw can be run by one strong engineer, but it demands a lot of attention:

- many state paths
- many logs
- many runtime modes
- many automation surfaces
- many restart behaviors

That is too expensive for the core product idea.

## 9. Too much behavior depends on policy overlays

Tool access, sandbox behavior, agent capabilities, subagent behavior, and connector rules are all shaped by multiple overlapping policy layers. That makes prediction and debugging harder.

## 10. Backward compatibility cost is everywhere

Path fallbacks, legacy pairing flows, startup migration, state discovery, and restart variants all add up. They make sense for an existing mature product. They should not shape a clean-slate replacement.
