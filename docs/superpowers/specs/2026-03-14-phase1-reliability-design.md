# Phase 1 Reliability Hardening — Design Spec

**Date:** 2026-03-14
**Scope:** Three targeted improvements to complete Phase 1 of the picoclaw-inspired architecture plan.

---

## Overview

Three small, independent reliability improvements to gistclaw's LLM interaction layer:

1. **Context window error recovery** — reactive compression when context limit is hit
2. **Codex token refresh buffer** — increase pre-expiry buffer from 30s to 5min
3. **Configurable retry attempts** — make `chatWithRetry` max attempts configurable

---

## B — Context Window Error Recovery

### Problem

`chatWithRetry` currently treats context-window errors as `ErrKindTerminal` (fail immediately).
A single long conversation or a large tool result can push message history over the
provider's context limit, causing the loop to return an error with no recovery.
Proactive summarization (`MaybeSummarize`) helps, but does not cover all cases.

### Design

**`providers/errors.go`** — add a fourth error kind:

```go
ErrKindContextWindow // context limit exceeded — compress and retry
```

Detect via string patterns (case-insensitive): `"context_length_exceeded"`,
`"maximum context length"`, `"too many tokens"`, `"reduce the length"`,
`"tokens in your prompt"`.

**Insertion order in `ClassifyError`:** The context-window check must be inserted
**before** the existing 5xx numeric check. Some providers surface context-limit errors
with an HTTP 500 status (e.g., `"500 too many tokens"`); if the 5xx check ran first,
these would be misclassified as `ErrKindRetryable` and never reach the compression path.

Update `chatWithRetry`'s doc comment to list the four behaviours including the new
`ErrKindContextWindow` branch.

**`gateway/loop.go` — `chatWithRetry` signature change:**

```go
func (s *Service) chatWithRetry(
    ctx context.Context,
    chatID int64,
    msgs *[]providers.Message,   // pointer — compression modifies in place
    tools []providers.Tool,
) (*providers.LLMResponse, error)
```

All **three** call sites in `handlePlainChat` are updated to pass `&msgs`:
- max-iterations forced final answer
- normal tool loop
- doom-loop guard forced final answer

**Compression logic — `compressMessages(msgs *[]providers.Message) bool`:**

Returns `true` if any messages were dropped, `false` if it was a no-op.

Message anatomy in the conversation slice:
- `role:"system"` — injected system prompt and timestamps, always kept.
- `role:"user"` — user turns, always kept (dropping user turns without their
  paired assistant response corrupts strict-alternation APIs).
- `role:"assistant"` + immediately following `role:"tool"` — a dyad produced by one
  tool call iteration; the only droppable unit.

Algorithm:

1. Walk `*msgs` and separate into two buckets:
   - **system**: all `role:"system"` messages — always kept.
   - **turns**: all remaining messages in original order (user, assistant, tool).
2. If `len(turns) <= 4`: return `false` (no-op — nothing safe to drop).
3. **Tail**: the last 4 messages from `turns` — always kept verbatim.
4. **Candidates**: `turns[:len(turns)-4]`.
5. Walk candidates left-to-right and collect dyads: when an `assistant` message is
   immediately followed by a `tool` message, group them as one dyad. All other
   messages (user, or unpaired assistant) are non-droppable anchors — they stay.
6. Compute `drop = floor(len(dyads) / 2)` (Go integer division: `len(dyads) / 2`).
   If `drop == 0` (covers both `len(dyads) == 0` and `len(dyads) == 1`),
   return `false` — nothing would be removed, no point retrying.
7. Rebuild `*msgs` as: system messages + surviving candidates + tail.
8. Return `true`.

**Retry policy (in `chatWithRetry`):**

Context-window handling is **orthogonal to the `maxAttempts` loop** — it is a separate
single-shot path, not one of the attempt slots:

```
for attempt := 0; attempt < maxAttempts; attempt++ {
    resp, err := s.llm.Chat(ctx, *msgs, tools)
    if err == nil { return resp, nil }

    switch ClassifyError(err) {
    case ErrKindContextWindow:
        if !compressMessages(msgs) {
            // No dyads to drop — can't help. Propagate immediately.
            return nil, err
        }
        // capture before = len(*msgs) before compressMessages; after = len(*msgs) after it returns true
        log.Warn().Int("before", before).Int("after", after).Msg("gateway: context window exceeded; compressed history")
        // One dedicated retry outside the attempt loop.
        resp2, err2 := s.llm.Chat(ctx, *msgs, tools)
        if err2 != nil {
            return nil, err2  // propagate whatever error (including another ErrKindContextWindow)
        }
        return resp2, nil

    case ErrKindTerminal:
        return nil, err
    case ErrKindRateLimit:
        // notify + fallthrough
    case ErrKindRetryable:
        // backoff + continue
    }
}
```

This means: if `maxAttempts = 1` and a context-window error occurs, there will be
exactly 2 total LLM calls (1 from the loop + 1 from the dedicated retry). The
`maxAttempts` loop governs retryable/rate-limit errors only.

### Acceptance criteria

- A conversation that exceeds the context limit triggers compression + one dedicated retry.
- System messages are never dropped.
- User messages are never dropped.
- The last 4 non-system/non-user turns are preserved.
- assistant+tool dyads are always dropped as a unit.
- If `len(turns) <= 4`, or `drop == 0` (no dyads, or only 1 dyad where floor(1/2)=0),
  compression is a no-op and the error propagates immediately without retry.
- `drop count = floor(len(dyads) / 2)` — explicitly floor division.
- A warning log entry appears with message count before and after compression.
- If the dedicated retry fails, its error propagates as-is.
- All three call sites in `handlePlainChat` pass `&msgs`.
- `chatWithRetry`'s doc comment lists all four error-kind behaviours.

---

## C — Codex Token Refresh Buffer

### Problem

Codex checks `time.Now().Before(tok.ExpiresAt.Add(-30 * time.Second))`. On a slow VPS
with network latency, a token that is 25 seconds from expiry will be considered valid,
but the subsequent API call may arrive after expiry, causing an auth failure.

### Design

Add a named constant in `providers/codex/codex.go`:

```go
const tokenRefreshBuffer = 5 * time.Minute
```

Replace every occurrence of a hard-coded short expiry buffer (the 30-second literal) in
`ensureToken` with `-tokenRefreshBuffer`. There are two semantic check sites:
1. The in-memory token cache check (validates the already-loaded `p.token`).
2. The freshly-loaded store token check (validates what was just read from SQLite).

Both must use `tokenRefreshBuffer` so behaviour is consistent regardless of whether
the token came from cache or disk.

No interface changes. No config changes needed — 5 minutes is a safe universal default.

### Acceptance criteria

- `ensureToken` refreshes when the token has ≤ 5 minutes remaining.
- Both expiry check sites (cache and store) use the named constant `tokenRefreshBuffer`.
- No literal time durations remain for expiry checking in `ensureToken`.

---

## D — Configurable Retry Attempts

### Problem

`chatWithRetry` has `const maxAttempts = 3` hardcoded. There is no way to tune this
for different VPS environments or provider reliability characteristics.

### Design

**`config/config.go`** — add to `Tuning` struct:

```go
LLMRetryAttempts int `yaml:"llm_retry_attempts" env:"TUNING_LLM_RETRY_ATTEMPTS"`
```

**`gateway/loop.go`** — replace `const maxAttempts = 3`:

```go
maxAttempts := s.cfg.Tuning.LLMRetryAttempts
if maxAttempts <= 0 {
    maxAttempts = 3
}
```

Default: `0` → treated as `3` (backward compatible, zero-value config works in tests).

**`sample.env`** — add after the existing `TUNING_` block:

```
# TUNING_LLM_RETRY_ATTEMPTS=3  # max retries for transient LLM errors; 0 or unset = 3
```

The comment value `3` matches the effective default so operators see the actual
in-use value without needing to read the source.

### Acceptance criteria

- Setting `TUNING_LLM_RETRY_ATTEMPTS=5` results in up to 5 total attempts.
- Unset or `0` behaves identically to the current hardcoded 3.
- `sample.env` uses the `TUNING_` prefix consistent with other tuning vars and
  shows `3` as the example value with a note that 0/unset also defaults to 3.

---

## Implementation Order

1. **C** (trivial, no dependencies) — change constant in `codex.go`
2. **D** (trivial, config only) — add field + update `chatWithRetry`
3. **B** (moderate) — add error kind + compression logic + signature change

Each can be done and tested independently.
