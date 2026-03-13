# Picoclaw-Inspired Architecture Ideas

> Architectural analysis comparing picoclaw and gistclaw to extract design
> patterns, system philosophies, and improvement ideas for gistclaw's evolution.
>
> This is an ideas document. Nothing here proposes copying code or porting
> implementations. The focus is on architectural inspiration.

---

## 1. Gistclaw Architecture Overview

### What It Is

Gistclaw is a **pure-Go single binary** (`github.com/canhta/gistclaw`) that acts as a
Telegram-driven remote controller for AI coding agents (OpenCode and Claude Code). It
follows a composition-root pattern where `internal/app/app.go` wires together five
goroutine services under `golang.org/x/sync/errgroup`.

### Service Topology

| Service | Criticality | Restart Budget |
|---------|------------|----------------|
| gateway | Critical (kills app on failure) | Unlimited |
| opencode | Non-critical | 5 restarts / 30s window |
| claudecode | Non-critical | 5 restarts / 30s window |
| hitl | Non-critical | 10 restarts / 10s window |
| scheduler | Non-critical | 5 restarts / 30s window |

Two heartbeat tickers: Tier 1 (Telegram liveness, 30s) and Tier 2 (agent health, 5m).

### Key Modules

| Module | Path | Purpose |
|--------|------|---------|
| app | `internal/app/` | Composition root, supervisor with restart budgets |
| config | `internal/config/` | YAML + env config loading and validation |
| gateway | `internal/gateway/` | Message routing, command dispatch, LLM tool loop |
| channel | `internal/channel/` | Channel abstraction + Telegram implementation |
| opencode | `internal/opencode/` | OpenCode agent subprocess + SSE streaming |
| claudecode | `internal/claudecode/` | Claude Code subprocess + stream-json + hook server |
| hitl | `internal/hitl/` | Human-in-the-loop permission/question flows |
| providers | `internal/providers/` | LLM provider interface + 3 implementations (OpenAI, Copilot, Codex) |
| infra | `internal/infra/` | Heartbeat, SOUL loader, cost guard |
| tools | `internal/tools/` | Web search (5 backends) + web fetch |
| mcp | `internal/mcp/` | MCP client (stdio + SSE transport) |
| scheduler | `internal/scheduler/` | Cron-like job scheduler |
| store | `internal/store/` | SQLite persistence layer (6 tables) |

### Agent Lifecycle

- **OpenCode**: Subprocess (`opencode serve`) with HTTP REST + SSE communication. Session
  lifecycle: create -> prompt_async -> consume SSE -> idle.
- **Claude Code**: Subprocess (`claude -p`) with stream-json output. FSM with idle/running
  states. Hook server at :8765 for HITL integration.

### Tool Execution

Three tool categories: core (web_search, web_fetch), agent (MCP-derived, namespaced as
`{server}__{tool}`), and system (schedule_job, list_jobs, etc.). Tool loop in
`handlePlainChat()`: LLM returns tool calls -> execute -> feed results -> repeat.

### Orchestration Model

Gateway is the central orchestrator. Telegram message -> command or plain chat -> either
route to agent's `SubmitTask()` or enter the tool loop with direct LLM interaction.

### State & Storage

SQLite (pure Go, WAL mode) with 6 tables: sessions, hitl_pending, cost_daily,
channel_state, provider_credentials, jobs. CostGuard uses atomic int64 for lock-free
micro-dollar tracking.

### Authentication

- Telegram: bot token + chat ID whitelist
- LLM providers: API keys (OpenAI), SDK auth (Copilot), OAuth2 PKCE (Codex)
- MCP: per-server env vars in config

### Context Management

- SOUL system: file-based system prompt with mtime caching
- Conversation history: in-memory per-session `[]providers.Message`
- No cross-session memory persistence

### Doom Loop Prevention

Sliding window of last 3 tool calls. If all identical, injects guard message and calls
LLM without tools, forcing a text response.

### Extensibility

Clean interfaces for channels, providers, agents, and MCP servers. Documented in
`EXTENDING.md`. New components require implementing an interface and wiring into
`app.go`.

---

## 2. Capability Map

The following capability areas form the analysis domains for comparing the two systems:

| Capability Area | Gistclaw Status | Picoclaw Status |
|----------------|----------------|-----------------|
| Auth / OAuth | API keys + single OAuth (Codex) | Multi-provider OAuth + PKCE + device code + web UI |
| LLM Orchestration | Single provider per request, basic tool loop | Fallback chains, cooldown tracking, model routing |
| Sub-agent Architecture | External agent control (OpenCode, Claude) | Native multi-agent with spawn/subagent tools |
| Tool Abstraction | Simple registry with namespace dispatch | Interface-based registry with TTL, discovery, async |
| Tool Discovery | Static registration only | BM25 + regex search with TTL-based promotion |
| Search / Retrieval | 5 backends, priority-based selection | 6 backends with API key pooling and rotation |
| MCP Integrations | Client with stdio + SSE transport | Client with stdio + SSE + discovery mode |
| Messaging Integrations | Telegram only | 17 channels across 15 platforms |
| Execution Loop Management | Basic iteration with guard message | MaxIterations cap + retry + compression + fallback |
| Doom Loop Prevention | 3-identical-call detection | Hard iteration cap + TTL tool expiry + force compression |
| Context Management | SOUL file + in-memory history | Layered system prompt + mtime caching + Anthropic cache markers |
| State Persistence | SQLite (6 tables) | File-based (JSONL + JSON + Markdown, atomic writes) |
| Plugin / Extension Architecture | Interface-based, manual wiring | Factory registry + init() auto-discovery + config-driven |
| External API Integrations | Search APIs + Telegram Bot API | Search APIs + 15 platform SDKs + voice transcription |
| Memory Systems | No long-term memory | File-based MEMORY.md + daily notes + conversation summaries |
| Supervisor / Coordinator | Restart supervisor with budgets | Config-driven hot reload + ordered shutdown + health probes |

---

## 3. Parallel Sub-Agent Analysis

### AuthAgent

**What picoclaw offers:**

A comprehensive authentication system supporting three methods (browser OAuth with PKCE,
device code flow, direct API token paste) across three providers (OpenAI, Anthropic,
Google Antigravity), with three surfaces (CLI, web backend, web frontend).

**Core abstractions:**
- `AuthCredential` value object with `IsExpired()` / `NeedsRefresh()` (5-min pre-expiry buffer)
- `AuthStore` backed by a single JSON file at `~/.picoclaw/auth.json` with atomic writes
- `OAuthProviderConfig` factory functions per provider
- `TokenSource` pattern: lazy-loading closures that auto-refresh transparently
- `oauthFlow` state machine for web-based OAuth with automatic GC of expired flows

**Architectural approach:**
Auth is fully separated from providers and channels. Providers consume auth via the
`TokenSource` abstraction. The system supports graceful degradation for headless
environments (device code flow) and multi-surface consistency (CLI and web use the same
`pkg/auth` package).

**Design principles:**
- Defense in depth (PKCE + state parameter + code verifier)
- Progressive enhancement (simplest case first, complexity added per-provider)
- Testability via `var` indirection for all external calls
- Pre-emptive token refresh prevents edge-of-expiry failures

**Tradeoffs:**
- Plaintext credential storage (0600 permissions but no encryption at rest)
- No file-level locking (last-write-wins on concurrent access)
- Hardcoded OAuth configs (new providers require code changes)

---

### LLMOrchestratorAgent

**What picoclaw offers:**

An 8-provider LLM system with fallback chains, cooldown tracking, model routing, and
multi-tier context management.

**Core abstractions:**
- `LLMProvider` interface: `Chat(ctx, messages, tools, model, options)`
- Optional `StatefulProvider` (adds `Close()`) and `ThinkingCapable` interfaces
- `ContextBuilder` with `BuildSystemPromptWithCache()` — mtime-based invalidation
- `FallbackChain` with `CooldownTracker` (exponential backoff per provider)
- `ClassifyError()` — ~40 regex patterns categorizing errors for retry decisions
- Model routing via `RouteResolver` with 7-level priority cascade
- Thinking levels: off/low/medium/high/xhigh/adaptive mapped to provider-specific configs

**Architectural approach:**
The iteration loop (`runLLMIteration()`) is bounded by `MaxIterations` (default 20).
Each iteration: build tools -> call LLM via fallback chain -> if tool calls, execute in
parallel -> append results -> loop. Retry logic distinguishes timeout errors (backoff)
from context window errors (force compression). Model routing evaluates message complexity
once per turn then stays sticky for all tool iterations.

**Design principles:**
- Error classification drives retry strategy (not blanket retry)
- Cooldown is provider-scoped, not request-scoped (prevents hammering failing providers)
- Billing errors get much longer cooldowns (5h-24h vs 1m-1h for standard errors)
- Streaming is consumed-then-returned (no real-time streaming through the agent loop)

**Tradeoffs:**
- Token estimation uses a 2.5 chars/token heuristic (fast but approximate)
- Streaming is buffered, not passed through (simpler but higher latency)
- Dual factory system (legacy + new) adds complexity

---

### ToolSystemAgent

**What picoclaw offers:**

An interface-based tool system with a thread-safe registry, TTL-based visibility,
BM25/regex discovery, async execution, and comprehensive safety guards.

**Core abstractions:**
- `Tool` interface: `Name()`, `Description()`, `Parameters()`, `Execute(ctx, args)`
- `AsyncExecutor` extension interface for background tool execution
- `ToolRegistry` with `Register()` (core, always visible) and `RegisterHidden()` (discoverable)
- `ToolResult` with dual channels: `ForLLM` (model context) and `ForUser` (human display)
- Request-scoped context injection via `WithToolContext(ctx, channel, chatID)`
- Deterministic tool ordering via `sortedToolNames()` for stable LLM prefix caching

**Architectural approach:**
The registry uses an atomic version counter incremented on every registration, used for
BM25 search cache invalidation. TTL-based visibility: hidden tools are promoted by search
tools with a TTL counter that decrements after each tool execution round. ~20 built-in
tools covering filesystem, shell, web, messaging, spawning, cron, hardware I2C/SPI, and
skill management. Shell tool has 40+ deny regex patterns plus SSRF protection on web
fetch.

**Design principles:**
- Async callback as parameter (not mutable state) eliminates data races
- Dual-channel results separate model context from user display
- TTL prevents stale tool accumulation in the LLM's context window
- Deterministic ordering preserves LLM prefix cache efficiency

**Tradeoffs:**
- Tool registration is centralized (not fully pluggable without code changes)
- No interactive tool approval (configuration-driven deny/allow only)

---

### SearchAgent

**What picoclaw offers:**

Three distinct search systems: web search (6 providers), tool discovery (BM25/regex), and
skills search (registry fan-out with trigram caching).

**Core abstractions:**
- `SearchProvider` strategy interface with 6 implementations
- `APIKeyPool` with `APIKeyIterator` for round-robin key rotation
- BM25 engine (`pkg/utils/bm25.go`) with version-gated caching
- Trigram similarity cache for skills registry search

**Architectural approach:**
Web search uses a priority-based failover chain: Perplexity > Brave > SearXNG > Tavily >
DuckDuckGo > GLM Search. API key pooling enables multiple keys per provider with
automatic rotation on rate limit errors. Web fetch includes defense-in-depth SSRF
protection: pre-flight private IP check + connect-time DNS rebinding guard.

**Design principles:**
- Provider abstraction enables zero-config failover
- Key pooling multiplies effective rate limits
- No vector/embedding search — purely lexical (BM25) and API-delegated

---

### MCPAgent

**What picoclaw offers:**

A pure MCP client with auto-detecting transport, lazy tool discovery, and graceful
concurrent lifecycle management.

**Core abstractions:**
- `Manager` with `ServerConnection` (groups Client + Session + Tools per server)
- `MCPTool` adapter: wraps `mcp.Tool` to satisfy the native `Tool` interface
- `MCPManager` interface for testability (only exposes `CallTool`)
- `mcpRuntime` with `sync.Once` for exactly-once initialization
- Tool name sanitization: `mcp_{server}_{tool}`, 64-char cap with FNV32a hash suffix

**Architectural approach:**
Transport auto-detection: URL -> SSE, Command -> stdio. Environment layering: parent env
< .env file < inline config. Concurrent startup with partial failure tolerance (only fails
if ALL servers fail). Close safety: `atomic.Bool` + double-checked locking +
`sync.WaitGroup` drain.

**Design principles:**
- Adapter pattern bridges external protocol to internal interface
- Interface segregation (MCPManager exposes only CallTool)
- Graceful degradation (partial server failures don't block startup)
- TTL-based lazy discovery prevents context window bloat from many MCP tools

**Tradeoffs:**
- No automatic reconnection (server drop requires loop restart)
- Tool names are opaquely transformed (lossy sanitization with hash suffix)

---

### MessagingAgent

**What picoclaw offers:**

17 channel implementations across 15 platforms with a sophisticated capability-discovery
pattern and per-channel worker queues.

**Core abstractions:**
- `Channel` interface: `Start()`, `Stop()`, `Send()`, `IsRunning()`
- `BaseChannel` embed providing allowlist filtering, group trigger logic, media store
- 8 optional capability interfaces: `TypingCapable`, `MessageEditor`, `ReactionCapable`,
  `PlaceholderCapable`, `CommandRegistrarCapable`, `MediaSender`, `WebhookHandler`,
  `HealthChecker`
- Factory registry with `init()` auto-discovery
- `MessageBus` with 3 typed buffered channels (inbound, outbound, outbound media)
- Per-channel worker queues with rate limiting
- Code-block-aware message splitting

**Architectural approach:**
Each channel sub-package registers itself via `init()` + `RegisterFactory()`. The manager
discovers capabilities at runtime via Go type assertions. Inbound flow: platform event ->
channel handler -> build SenderInfo -> allowlist check -> media download -> bus publish.
Outbound flow: bus consume -> per-channel worker -> rate limit -> split -> preSend (stop
typing, undo reaction, try placeholder edit) -> send with retry.

**Design principles:**
- Capability interfaces are opt-in (channels only implement what they support)
- Runtime discovery via type assertions (no mandatory capability declarations)
- preSend orchestrates typing/reaction/placeholder lifecycle automatically
- Error classification drives retry strategy (sentinel errors for rate limit, temporary, etc.)

**Tradeoffs:**
- Adding a new channel still requires a config struct + manager init check
- No dynamic channel loading (compile-time registration via init())

---

### LoopControlAgent

**What picoclaw offers:**

A defense-in-depth approach with 8 layers of loop protection.

**Core abstractions:**
- `MaxIterations` hard cap (default 20, configurable)
- Retry loop with error-type-specific handling (timeout -> backoff, context window -> compression)
- `FallbackChain` with `CooldownTracker` (exponential backoff)
- `forceCompression()` — emergency drop of oldest 50% of messages
- `maybeSummarize()` — proactive summarization at 20 messages or 75% context usage
- Tool TTL system — discovery-promoted tools auto-expire
- Provider hot reload with `activeRequests sync.WaitGroup` for safe teardown

**Architectural approach:**
Layer 1: MaxIterations prevents infinite tool loops. Layer 2: No-tool-calls early exit.
Layer 3: Fallback chain + cooldown for provider failures. Layer 4: Context compression +
summarization prevents overflow. Layer 5: Error classification drives retry logic. Layer
6: Heartbeat statelessness prevents session bloat. Layer 7: Tool TTL prevents stale
accumulation. Layer 8: Atomic state + provider reload for crash safety.

**Design principles:**
- Multiple independent guard mechanisms (defense in depth)
- Error classification separates retriable from terminal errors
- Proactive prevention (summarize before overflow) + reactive recovery (compress on error)
- Billing errors treated with much higher cooldown thresholds

---

### AgentArchitectureAgent

**What picoclaw offers:**

A hub-and-spoke multi-agent system with registry-based management, configuration-driven
topology, and two delegation tools (sync and async).

**Core abstractions:**
- `AgentInstance` struct (not interface): self-contained unit with its own workspace,
  tools, sessions, and model config
- `AgentRegistry`: thread-safe map managing all agent instances
- `AgentLoop`: central hub consuming from MessageBus and routing to agents
- `SpawnTool` (async) + `SubagentTool` (sync): two delegation mechanisms
- `RunToolLoop()`: reusable LLM+tool iteration loop shared between main and subagent paths
- `RouteResolver` with 7-level priority cascade: peer > parent_peer > guild > team >
  account > channel_* > default
- `SubagentsConfig.AllowAgents`: per-agent spawn allowlist

**Architectural approach:**
No peer-to-peer agent communication. All coordination flows through the AgentLoop hub.
Each agent gets isolated sessions, memory, tools, and workspace. Subagent results flow
back via the message bus as system messages. Hot-reload atomically rebuilds the entire
registry.

**Design principles:**
- Hub-and-spoke prevents complex inter-agent coordination bugs
- Registry pattern enables runtime topology changes
- Allowlist enforcement prevents uncontrolled agent proliferation
- Reusable tool loop avoids code duplication between main and subagent paths

---

### ExtensibilityAgent

**What picoclaw offers:**

A multi-pattern extensibility architecture with varying pluggability levels.

**Extension mechanisms:**

| Mechanism | Pluggability | Pattern |
|-----------|-------------|---------|
| Channels | High | Factory + init() auto-registration |
| MCP servers | Very high | Config-only, zero code changes |
| Skills | High | File-based + pluggable registry interface |
| Model list | High | Config-only with protocol/model notation |
| Multi-agent | High | Config-only agent topology |
| Commands | Medium | Definition registry + handler callbacks |
| Tools | Medium | Config-gated registry, central registration |
| Providers | Low | Hardcoded switch/case (planned refactor) |

**Core abstractions:**
- `SkillsLoader` with 3-tier priority: workspace > global > builtin
- `SkillRegistry` interface for pluggable skill marketplaces (ClawHub implementation)
- `SkillInstaller` for GitHub/ClawHub downloads with malware detection
- `commands.Registry` + `commands.Executor` with sub-command routing
- `commands.Runtime` as dependency injection container for handlers
- Config-driven hot reload for the entire service stack

**Design principles:**
- Progressive pluggability (config-only where possible, code changes where necessary)
- init() registration decouples component packages from the main module
- Interface-based extension points for complex integrations
- Skills as markdown files enable non-programmer extensibility

---

### MemoryAgent

**What picoclaw offers:**

A file-based memory system with two tiers of persistence and LLM-driven curation.

**Core abstractions:**
- `MEMORY.md` — persistent long-term memory (single file, read/written by agent)
- Daily notes: `workspace/memory/YYYYMM/YYYYMMDD.md` — date-partitioned entries
- `JSONLStore` — append-only conversation persistence with logical truncation and compaction
- Sharded concurrency: 64 `sync.Mutex` shards via FNV hash
- `maybeSummarize()` — LLM-driven summarization with multi-part handling for large histories
- Atomic writes everywhere via temp file + fsync + rename

**Architectural approach:**
No vector database, no embeddings, no RAG. Memory retrieval is purely temporal: recent
daily notes + full MEMORY.md injected into system prompt. The agent itself decides what
to persist (emergent curation via system prompt instructions). Session history uses JSONL
with logical truncation (skip offset in metadata) and on-demand compaction.

**Design principles:**
- Simplicity over sophistication (flat files, not databases)
- Inspectability (human-readable Markdown and JSONL)
- Crash safety (atomic writes on flash storage)
- Agent-directed memory (LLM decides what to remember)
- Fire-and-forget writes (errors logged, not propagated)

---

### SupervisorAgent

**What picoclaw offers:**

Configuration-driven process management with hot reload, ordered shutdown, and Kubernetes-
compatible health probes.

**Core abstractions:**
- `gatewayServices` container holding all running service references
- Config file polling (2s interval) with validate-then-apply pattern
- `ReloadProviderAndConfig()`: goroutine with panic recovery, atomic swap, 100ms drain
- Ordered startup (strict dependency sequence) and ordered shutdown
- Health server: `/health` (liveness) + `/ready` (readiness with registered checks)
- Heartbeat service: periodic ticker with stateless execution (NoHistory: true)
- Cron service: file-backed JSON store with three schedule kinds (at, every, cron)
- Bus shutdown: `CompareAndSwap` for idempotency + drain-not-close pattern

**Design principles:**
- Hot reload enables zero-downtime configuration changes
- Ordered lifecycle prevents dependency race conditions
- Drain-not-close prevents send-on-closed panics during concurrent shutdown
- Kubernetes-native health probes for container orchestration

---

## 4. Extracted Design Principles

From analyzing all 11 capability domains, these recurring patterns emerge:

### 4.1 Interface-Based Capability Discovery

Picoclaw uses optional interfaces extensively. Rather than a monolithic interface that all
implementations must satisfy, it defines a minimal core interface plus many optional
capability interfaces. The orchestrator discovers capabilities at runtime via Go type
assertions. This pattern appears in channels (8 optional interfaces), providers
(`StatefulProvider`, `ThinkingCapable`), and tools (`AsyncExecutor`).

### 4.2 TTL-Based Resource Management

Time-to-live counters manage temporary resource visibility. Discovered MCP tools get a
TTL that decrements after each agent loop iteration, auto-hiding them when expired. Media
store entries can have TTL cleanup. OAuth flows expire after 10-15 minutes. This prevents
resource accumulation without requiring explicit cleanup.

### 4.3 Defense-in-Depth Error Handling

Every subsystem has multiple independent guard mechanisms. The LLM loop has 8 layers.
Web fetch has 2-layer SSRF protection. Shell execution has deny patterns + workspace
restriction + remote channel blocking. Auth has PKCE + state parameter + pre-expiry
refresh. No single mechanism is trusted to be sufficient.

### 4.4 Atomic Writes for Crash Safety

Every file write in picoclaw goes through temp file -> fsync -> rename. This pattern is
consistent across credentials, config, state, cron jobs, sessions, and memory. The system
is designed for unreliable storage (flash, embedded devices).

### 4.5 Error Classification Drives Strategy

Rather than blanket retry, picoclaw classifies errors into categories (~40 regex patterns)
and applies different strategies: rate limits get cooldown, billing gets very long
cooldowns, format errors are never retried, timeouts get exponential backoff. This
prevents wasting resources on unrecoverable errors.

### 4.6 Dual-Channel Information Flow

Tool results carry separate content for the LLM (`ForLLM`) and the user (`ForUser`). This
separation allows tools to return rich data for model reasoning while showing concise
summaries to humans. The `Silent` flag enables tools to communicate only with the LLM.

### 4.7 Factory + init() Auto-Registration

Channel implementations register themselves in Go `init()` functions. The manager
discovers them by name lookup from config. This decouples component packages from the
main application module, enabling clean separation and conditional compilation (build
tags for platform-specific channels).

### 4.8 Hub-and-Spoke Agent Coordination

All inter-agent communication is mediated by the AgentLoop hub. There is no peer-to-peer
agent communication. This simplifies reasoning about system behavior and prevents
coordination deadlocks, at the cost of the hub being a bottleneck.

### 4.9 Progressive Enhancement

Systems start simple and add capability as needed. Auth supports API key paste (simplest)
before OAuth (complex). Channels implement core interface first, optional capabilities
as needed. Tools can be core (always visible) or hidden (discovered on demand).

### 4.10 Configuration as Primary Extension Mechanism

Where possible, picoclaw favors configuration over code for extensibility. MCP servers,
model lists, agent topologies, skill directories, and bindings are all config-driven.
Code changes are reserved for new abstractions or protocols.

---

## 5. Architectural Philosophy of Picoclaw

### Design Mindset

Picoclaw is designed as a **platform**, not just an application. Its architecture assumes:

1. **Many platforms, one brain**: The system is channel-agnostic at its core. The agent
   loop has no knowledge of Telegram, Discord, or WhatsApp. The MessageBus creates a
   clean boundary.

2. **Agents as citizens**: Agents are first-class entities with their own workspaces,
   sessions, tools, and memory. The system supports arbitrary agent topologies configured
   without code changes.

3. **Tools as the interface**: The LLM interacts with the world through tools. Tools are
   the primary extension mechanism, and the discovery system ensures the LLM can find
   relevant tools without being overwhelmed.

4. **Files over databases**: Picoclaw uses JSONL, JSON, and Markdown files instead of
   SQLite or other databases. This trades query capability for inspectability, portability,
   and simplicity. Files are the universal storage abstraction.

5. **Configuration over code**: The system maximizes what can be changed without
   recompilation. Model lists, agent topologies, MCP servers, skills, and bindings are
   all configuration-driven.

6. **Safety by default**: Shell commands are deny-by-default with 40+ blocked patterns.
   Remote channels can't execute shell commands unless explicitly allowed. Filesystem
   access is sandboxed. Web fetch has SSRF protection. Tools are gated by configuration.

7. **Graceful degradation**: Partial failures don't crash the system. MCP servers can
   fail individually. LLM providers fall back to alternatives. Channels can be
   independently unhealthy. The system continues operating with reduced capability.

### Why This Architecture

The design makes sense when you consider picoclaw's broader goals:

- **Multi-platform support** requires clean channel abstraction (not monolithic Telegram code)
- **Embedded/IoT targets** (MaixCam, I2C, SPI) require crash-safe file I/O (atomic writes)
- **Many LLM providers** require fallback chains and error classification
- **Extensibility** requires factory patterns and configuration-driven composition
- **Safety** requires multiple independent guard mechanisms (users trust the system with shell access)

### Engineering Values

1. **Reliability > Features**: Multiple safety layers, atomic writes, graceful degradation
2. **Simplicity > Cleverness**: Files over databases, flat config over complex schemas
3. **Extensibility > Completeness**: Clean interfaces + config-driven composition
4. **Independence > Integration**: Each component can operate independently with reduced capability

---

## 6. Inspiration for Gistclaw

### 6.1 Auth / OAuth

**Picoclaw approach**: Multi-provider OAuth with PKCE, device code flow, token auto-
refresh via TokenSource pattern, web UI for credential management.

**Gistclaw current approach**: API keys for OpenAI, SDK auth for Copilot, single OAuth2
PKCE flow for Codex, stored in SQLite.

**Inspiration**:
- **TokenSource abstraction**: A lazy-loading closure that transparently handles token
  refresh would simplify provider auth. Currently, Codex OAuth is tightly coupled to its
  provider implementation.
- **Pre-emptive refresh**: The 5-minute pre-expiry buffer prevents edge-of-expiry failures.
  Gistclaw's Codex token refresh could benefit from this pattern.
- **Auth as a separate package**: Extracting auth logic from provider implementations into
  a standalone package would enable reuse across future providers.

### 6.2 LLM Orchestration

**Picoclaw approach**: Fallback chains with per-provider cooldown, error classification
(~40 patterns), model routing with 7-level priority cascade, extended thinking support.

**Gistclaw current approach**: Single provider per request, no fallback, basic error
handling.

**Inspiration**:
- **Fallback chains**: If the primary provider is down, gistclaw currently fails. A
  fallback mechanism would improve reliability, especially for the gateway's direct LLM
  mode.
- **Error classification**: Distinguishing rate limits from billing errors from format
  errors enables smarter retry strategies. Gistclaw currently treats all errors the same.
- **Cooldown tracking**: Per-provider cooldown prevents hammering a failing provider.
  With different cooldown durations for different error types (1m for rate limits, 5h
  for billing), the system can make intelligent routing decisions.
- **Model routing**: The 7-level priority cascade (peer > guild > team > account >
  channel > default) could inspire per-user or per-channel model selection in gistclaw.

### 6.3 Sub-agent Architecture

**Picoclaw approach**: Native multi-agent with AgentRegistry, per-agent workspaces, sync
and async delegation tools, allowlist-based spawn control.

**Gistclaw current approach**: External agent control (OpenCode, Claude Code as
subprocesses). No native multi-agent capability.

**Inspiration**:
- **Agent as first-class entity**: Each agent having its own workspace, tools, and sessions
  enables specialization. Gistclaw could evolve toward agents with different capabilities
  or personalities, configured without code changes.
- **Reusable tool loop**: The `RunToolLoop()` function shared between main agent and
  subagent execution avoids code duplication. Gistclaw's gateway tool loop could be
  extracted similarly.
- **Allowlist-based delegation**: Controlling which agents can spawn which subagents
  prevents uncontrolled proliferation.

### 6.4 Tool Abstraction

**Picoclaw approach**: Interface-based Tool with async extension, dual-channel results
(ForLLM/ForUser), TTL-based discovery, deterministic ordering for cache efficiency.

**Gistclaw current approach**: Tools registered in `buildToolRegistry()`, dispatched via
switch on tool name. MCP tools detected by `__` separator.

**Inspiration**:
- **Tool interface**: A formal `Tool` interface with `Name()`, `Description()`,
  `Parameters()`, `Execute()` would enable cleaner tool registration and dispatch.
- **Dual-channel results**: Separating model context from user display would allow tools
  to return rich data for reasoning while showing concise summaries in Telegram.
- **Deterministic ordering**: Sorting tool names alphabetically for stable KV cache is a
  subtle but impactful optimization for LLM efficiency.
- **Request-scoped context**: Injecting channel/chatID via context (not tool struct state)
  eliminates concurrency issues.

### 6.5 Tool Discovery

**Picoclaw approach**: BM25 + regex search over hidden tools with TTL-based promotion.

**Gistclaw current approach**: All tools always visible to the LLM.

**Inspiration**:
- **Hidden tools with on-demand discovery**: As the number of MCP tools grows, exposing
  all of them in every request wastes context tokens. A discovery mechanism would let the
  LLM find relevant tools without the overhead.
- **TTL-based visibility**: Promoted tools auto-expire, preventing context window bloat
  from accumulated tool definitions.

### 6.6 Search / Retrieval

**Picoclaw approach**: 6 providers with API key pooling, automatic failover, SSRF-
hardened web fetch.

**Gistclaw current approach**: 5 providers with priority-based selection. Basic Cloudflare
retry on web fetch.

**Inspiration**:
- **API key pooling**: Round-robin across multiple API keys per provider multiplies
  effective rate limits. Useful for high-volume search usage.
- **SSRF protection**: Connect-time DNS rebinding guard + private IP blocking is more
  robust than gistclaw's basic fetching. Important for security.

### 6.7 MCP Integrations

**Picoclaw approach**: Auto-detecting transport, env file support, discovery mode,
graceful concurrent lifecycle, name sanitization with hash suffix.

**Gistclaw current approach**: Stdio + SSE transport, namespace-based tool naming
(`{server}__{tool}`).

**Inspiration**:
- **Discovery mode**: Registering MCP tools as hidden and promoting via search would help
  when connecting to MCP servers with many tools.
- **Env file support**: Loading .env files for MCP server processes simplifies configuration
  of complex server environments.
- **Name sanitization**: FNV32a hash suffix for collision avoidance on long/lossy tool
  names is more robust than simple concatenation.

### 6.8 Messaging Integrations

**Picoclaw approach**: 17 channels with capability-based interfaces, factory registration,
per-channel worker queues with rate limiting.

**Gistclaw current approach**: Telegram only with long polling.

**Inspiration**:
- **Capability interfaces**: If gistclaw adds more channels, the optional capability
  interface pattern (TypingCapable, MessageEditor, etc.) would allow each channel to
  implement only what it supports.
- **BaseChannel embed**: Shared logic (allowlist, group triggers, media handling) in a
  base struct prevents duplication across channel implementations.
- **Per-channel worker queues**: Rate limiting per channel prevents one busy channel from
  starving others.
- **Code-block-aware splitting**: The message splitter that respects code block boundaries
  is a quality-of-life improvement over simple character-count splitting.

### 6.9 Execution Loop Management

**Picoclaw approach**: MaxIterations hard cap, retry with error classification, context
compression, proactive summarization.

**Gistclaw current approach**: 3-identical-call detection, guard message injection.

**Inspiration**:
- **Hard iteration cap**: A simple `MaxIterations` counter (default 20) provides an
  absolute safety net that gistclaw's sliding-window approach doesn't guarantee.
- **Context compression**: Emergency drop of oldest 50% of messages when context window
  errors occur enables recovery without losing the entire conversation.
- **Proactive summarization**: Summarizing at 75% context usage prevents hitting limits,
  rather than only reacting when errors occur.
- **Configurable limits**: Making iteration caps configurable allows tuning for different
  use cases.

### 6.10 Context Management

**Picoclaw approach**: Layered system prompt with mtime-based caching, Anthropic cache
markers, bootstrap file system, skills integration.

**Gistclaw current approach**: SOUL file with mtime caching, in-memory conversation
history.

**Inspiration**:
- **Layered system prompt**: Separating static (identity, skills, memory) from dynamic
  (timestamp, session info) enables caching the static portion across requests.
- **Anthropic cache markers**: `cache_control: ephemeral` on system prompt blocks enables
  Anthropic's prefix caching, reducing costs for repeated system prompts.
- **Bootstrap file system**: Multiple named files (AGENTS.md, SOUL.md, USER.md,
  IDENTITY.md) instead of a single SOUL file enables modular prompt composition.
- **Memory injection**: Reading MEMORY.md + daily notes into the system prompt gives the
  agent persistent context across sessions.

### 6.11 State Persistence

**Picoclaw approach**: File-based (JSONL + JSON + Markdown), atomic writes everywhere,
no database.

**Gistclaw current approach**: SQLite with 6 tables, WAL mode.

**Inspiration**:
- **JSONL for conversation history**: Append-only JSONL with logical truncation and
  compaction is simpler than SQL for conversation storage and enables easy inspection.
- **Sharded concurrency**: 64 mutex shards via FNV hash provides bounded memory usage
  regardless of session count — an alternative to SQLite's single-writer model.
- **Inspectability**: File-based storage makes debugging easier (just cat the file).

Note: Gistclaw's SQLite approach has its own advantages (queries, transactions, schema
enforcement). This is a philosophical difference, not necessarily an improvement.

### 6.12 Extensibility

**Picoclaw approach**: Factory + init() auto-registration for channels, config-driven
MCP/agents/models, interface-based skills registry.

**Gistclaw current approach**: Interface-based extension with manual wiring in app.go.

**Inspiration**:
- **init() auto-registration**: Channel packages registering themselves removes the need
  for central wiring. The main module just imports the packages.
- **Skills system**: A directory-based skill system (markdown files with frontmatter)
  enables non-programmer extensibility. Skills are discovered and presented in the system
  prompt automatically.
- **Commands as an extension point**: A registry-based command system with handler
  callbacks would be cleaner than gistclaw's switch/case command dispatch.

### 6.13 Memory Systems

**Picoclaw approach**: File-based MEMORY.md + daily notes, agent-directed curation.

**Gistclaw current approach**: No long-term memory. Each interaction starts fresh.

**Inspiration**:
- **Long-term memory file**: A simple MEMORY.md that the agent reads and writes would give
  gistclaw persistent context. The agent would remember user preferences, project context,
  and past interactions.
- **Daily notes**: Date-partitioned notes provide temporal context without unbounded growth
  (only recent days are loaded).
- **Agent-directed curation**: Letting the LLM decide what to remember (via system prompt
  instructions) is simpler than building a programmatic extraction pipeline.

### 6.14 Supervisor / Coordinator

**Picoclaw approach**: Config-driven hot reload, ordered lifecycle, Kubernetes health
probes, drain-not-close shutdown.

**Gistclaw current approach**: Restart supervisor with budgets, heartbeat system,
errgroup-based lifecycle.

**Inspiration**:
- **Hot reload**: Gistclaw currently requires a restart for config changes. Config file
  watching with atomic service swap would enable zero-downtime reconfiguration.
- **Health probes**: `/health` and `/ready` endpoints would enable Kubernetes deployment
  with proper liveness and readiness probes.
- **Drain-not-close**: The bus shutdown pattern (drain buffered messages, don't close
  channels) prevents send-on-closed panics that can occur with errgroup cancellation.

---

## 7. Priority Opportunities

Ranked by impact-to-effort ratio and alignment with gistclaw's evolution:

### Tier 1: High Impact, Moderate Effort

1. **Long-term memory system**: Add MEMORY.md + daily notes to give gistclaw persistent
   context across sessions. The agent-directed curation approach requires minimal code
   (file read/write in context builder, system prompt instructions).

2. **Hard iteration cap**: Add a configurable MaxIterations to the gateway tool loop as
   an absolute safety net, complementing the existing doom-loop detection.

3. **Proactive context summarization**: Summarize conversation history before hitting
   context limits, rather than only reacting to errors. Uses the existing LLM to compress
   history.

4. **Error classification for LLM calls**: Classify provider errors into categories
   (rate limit, billing, timeout, format) and apply different retry strategies. This
   improves reliability without architectural changes.

### Tier 2: High Impact, Higher Effort

5. **LLM fallback chains**: Enable the gateway to fall back to alternative providers
   when the primary fails. Requires abstracting provider selection into a chain with
   cooldown tracking.

6. **Tool interface formalization**: Define a formal `Tool` interface with dual-channel
   results (ForLLM/ForUser) and request-scoped context. This would clean up the tool
   dispatch system and enable richer tool responses.

7. **Channel capability interfaces**: If additional messaging platforms are planned,
   adopting the optional capability interface pattern early would prevent the channel
   abstraction from becoming monolithic.

8. **Config-driven hot reload**: Enable configuration changes without process restart.
   Watch the config file, validate changes, and atomically swap service configurations.

### Tier 3: Strategic, Long-term

9. **Tool discovery with TTL**: As MCP tool count grows, implement hidden tool
   registration with BM25/regex search and TTL-based promotion to manage context window
   usage.

10. **Native multi-agent support**: Evolve from external agent control toward native
    agent instances with per-agent workspaces and configuration-driven topology.

11. **Skills system**: A directory-based skill system for modular prompt composition
    and community extensibility.

12. **API key pooling**: Round-robin across multiple API keys per search provider for
    higher throughput.

---

## 8. Long-Term Architecture Evolution

### Phase 1: Reliability Hardening (Near-term)

Focus on making the existing system more robust:

- Hard iteration caps on the tool loop
- Error classification for provider calls
- Proactive context management (summarization before overflow)
- Pre-emptive token refresh for OAuth providers
- Health probe endpoints for container deployment

These changes work within the current architecture and improve reliability immediately.

### Phase 2: Persistent Intelligence (Medium-term)

Add memory and context persistence:

- MEMORY.md + daily notes for long-term agent memory
- Layered system prompt with mtime-based caching
- Bootstrap file system (modular prompt composition)
- Conversation summarization for history compression
- Agent-directed memory curation

These changes give gistclaw persistent context across sessions, making it significantly
more useful as a long-running assistant.

### Phase 3: Platform Evolution (Longer-term)

Evolve from single-channel controller to multi-platform system:

- Channel capability interfaces for multi-platform support
- Factory + auto-registration for channels
- Per-channel worker queues with rate limiting
- MessageBus decoupling (channels from agent logic)
- Code-block-aware message splitting

These changes would be relevant if gistclaw expands beyond Telegram.

### Phase 4: Agent Sophistication (Long-term)

Evolve from external agent control to native agent capabilities:

- Formal Tool interface with dual-channel results
- Tool discovery with TTL-based promotion
- Native multi-agent with per-agent workspaces
- Configuration-driven agent topology
- LLM fallback chains with cooldown tracking
- Model routing (light vs heavy) based on message complexity
- Skills system for modular extensibility

These changes represent a fundamental evolution of gistclaw's architecture, moving it
from a controller for external agents to a platform that hosts its own agent capabilities.

### Guiding Principles for Evolution

Drawing from picoclaw's philosophy:

1. **Extend, don't rewrite**: Each phase should build on the previous architecture, not
   replace it. Picoclaw demonstrates that legacy and new patterns can coexist (dual config
   systems, auto-migration between formats).

2. **Configuration over code**: Favor config-driven extensibility. Each new capability
   should be toggleable without recompilation where possible.

3. **Safety by default**: New capabilities should be restrictive by default and explicitly
   opted into. Follow picoclaw's deny-by-default shell execution model.

4. **Independent guard mechanisms**: Don't rely on a single safety mechanism. Layer
   multiple independent guards (iteration caps + doom loop detection + context compression
   + error classification).

5. **Graceful degradation**: New subsystems should tolerate partial failures. If a memory
   file is corrupted, fall back to no memory rather than crashing. If a fallback provider
   fails, try the next one.

6. **Inspectability**: Prefer storage formats that humans can read and debug. JSONL and
   Markdown are preferable to opaque binary formats for debugging agent behavior.

---

*Analysis completed. This document captures architectural ideas inspired by picoclaw's
design philosophy, intended to guide the future evolution of gistclaw. No code was copied
or ported. All ideas are conceptual and require independent implementation.*
