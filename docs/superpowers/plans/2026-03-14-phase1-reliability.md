# Phase 1 Reliability Hardening Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete Phase 1 reliability hardening with three targeted improvements: Codex token refresh buffer (C), configurable LLM retry attempts (D), and context-window error recovery with message compression (B).

**Architecture:** C and D are trivial constant/config changes. B adds a new `ErrKindContextWindow` error kind and a pure `compressMessages` function (in its own file for testability), then threads the compression path into `chatWithRetry` via a pointer-receiver signature change.

**Tech Stack:** Go, zerolog, `github.com/caarlos0/env` for config parsing.

**Spec:** `docs/superpowers/specs/2026-03-14-phase1-reliability-design.md`

---

## Chunk 1: Task C + D (trivial changes)

### Task 1: Codex token refresh buffer

**Files:**
- Modify: `internal/providers/codex/codex.go` (add constant, replace 2 literals)
- Create: `internal/providers/codex/token_refresh_test.go` (internal package test)

`codex_test.go` is `package codex_test` (external), so `ensureToken` (unexported) is
not accessible there. We add a separate internal test file `token_refresh_test.go` with
`package codex` instead of adding to the existing external test file.

- [ ] **Step 1: Write the failing test**

Create `internal/providers/codex/token_refresh_test.go`:

```go
// internal/providers/codex/token_refresh_test.go
package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/store"
)

func newTestStoreCodex(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestEnsureToken_RefreshesWithinBuffer verifies that ensureToken treats a token
// expiring within tokenRefreshBuffer (5 min) as stale and triggers a refresh.
// Before this fix the buffer was 30s, so a token with 3min remaining was treated
// as valid and returned as-is.
func TestEnsureToken_RefreshesWithinBuffer(t *testing.T) {
	refreshCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshCalled = true
		expiry := time.Now().Add(1 * time.Hour).Unix()
		// Minimal OAuth2 token response accepted by golang.org/x/oauth2.
		fmt.Fprintf(w, `{"access_token":"new-token","token_type":"Bearer","expires_in":3600,"expiry":%d}`, expiry)
	}))
	defer srv.Close()

	s := newTestStoreCodex(t)
	p := NewWithURLs(s, srv.URL, "http://unused-chat")

	// Store a token expiring in 3 minutes — inside the 5-minute buffer.
	// The old 30s buffer would have accepted this; the new 5m buffer must not.
	tok := StoredToken{
		AccessToken:  "old-token",
		RefreshToken: "some-refresh-token",
		ExpiresAt:    time.Now().Add(3 * time.Minute),
	}
	raw, err := json.Marshal(tok)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := s.SetProviderCredentials("codex", string(raw)); err != nil {
		t.Fatalf("SetProviderCredentials: %v", err)
	}

	result, err := p.ensureToken(context.Background())
	if err != nil {
		t.Fatalf("ensureToken: %v", err)
	}
	if !refreshCalled {
		t.Error("token refresh endpoint was NOT called; token within 5-min buffer should trigger refresh")
	}
	if result.AccessToken == "old-token" {
		t.Error("ensureToken returned stale token; expected refreshed token")
	}
}

// TestEnsureToken_ValidTokenNotRefreshed verifies that a token with >5 min remaining
// is returned from cache without calling the refresh endpoint.
func TestEnsureToken_ValidTokenNotRefreshed(t *testing.T) {
	refreshCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshCalled = true
		w.WriteHeader(http.StatusInternalServerError) // should never be reached
	}))
	defer srv.Close()

	s := newTestStoreCodex(t)
	p := NewWithURLs(s, srv.URL, "http://unused-chat")

	tok := StoredToken{
		AccessToken:  "valid-token",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(10 * time.Minute), // well beyond 5-min buffer
	}
	raw, _ := json.Marshal(tok)
	_ = s.SetProviderCredentials("codex", string(raw))

	result, err := p.ensureToken(context.Background())
	if err != nil {
		t.Fatalf("ensureToken: %v", err)
	}
	if refreshCalled {
		t.Error("refresh endpoint was called unexpectedly for a valid token")
	}
	if result.AccessToken != "valid-token" {
		t.Errorf("expected valid-token; got %q", result.AccessToken)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /Users/canh/Projects/Claw/gistclaw
go test ./internal/providers/codex/... -run TestEnsureToken_ -v
```

Expected: `TestEnsureToken_RefreshesWithinBuffer` FAIL (refresh not called — 30s buffer passes 3min token); `TestEnsureToken_ValidTokenNotRefreshed` PASS (already works with 30s buffer).

- [ ] **Step 3: Add the named constant and replace both literal occurrences**

In `internal/providers/codex/codex.go`, add after the existing `const` block (after `codexAudience`):

```go
// tokenRefreshBuffer is how early we treat a token as expired.
// 5 minutes provides a safety margin for slow VPS networks.
const tokenRefreshBuffer = 5 * time.Minute
```

Replace the **in-memory cache check** (line ~117):
```go
// before:
if p.token != nil && time.Now().Before(p.token.ExpiresAt.Add(-30*time.Second)) {
// after:
if p.token != nil && time.Now().Before(p.token.ExpiresAt.Add(-tokenRefreshBuffer)) {
```

Replace the **store-loaded check** (line ~131):
```go
// before:
} else if time.Now().Before(tok.ExpiresAt.Add(-30 * time.Second)) {
// after:
} else if time.Now().Before(tok.ExpiresAt.Add(-tokenRefreshBuffer)) {
```

Verify no other `-30*time.Second` or `30*time.Second` literals remain in `ensureToken`.

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/providers/codex/... -v
```

Expected: all PASS including the two new `TestEnsureToken_` tests.

- [ ] **Step 5: Commit**

```bash
git add internal/providers/codex/codex.go internal/providers/codex/token_refresh_test.go
git commit -m "fix(codex): increase token refresh buffer from 30s to 5min"
```

---

### Task 2: Configurable LLM retry attempts

**Files:**
- Modify: `internal/config/config.go` (add field to Tuning)
- Modify: `sample.env` (add commented entry)
- Modify: `internal/gateway/loop.go` (replace hardcoded const)
- Modify: `internal/gateway/service_test.go` (add test + extend newServiceFull)

- [ ] **Step 1: Write the failing test**

Add to `internal/gateway/service_test.go`:

```go
// TestGateway_Retry_ConfigurableAttempts verifies that LLMRetryAttempts=2 limits
// total calls to 2 (not the default 3) on repeated retryable errors.
func TestGateway_Retry_ConfigurableAttempts(t *testing.T) {
	ch := newMockChannel()
	serverErr := errors.New("503 Service Unavailable")
	llm := &mockLLM{
		errs: []error{serverErr, serverErr, serverErr},
	}
	svc := newServiceFull(t, ch, llm, nil, 10*time.Millisecond, 2 /* retryAttempts */)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "hello"}
	time.Sleep(300 * time.Millisecond)

	// With LLMRetryAttempts=2, expect exactly 2 calls (not the default 3).
	if calls := llm.calls(); calls != 2 {
		t.Errorf("expected exactly 2 LLM calls with LLMRetryAttempts=2; got %d", calls)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./internal/gateway/... -run TestGateway_Retry_ConfigurableAttempts -v
```

Expected: compile error — `newServiceFull` does not yet accept a 6th argument.

- [ ] **Step 3: Add the config field**

In `internal/config/config.go`, inside the `Tuning` struct after `LLMRetryDelay`:

```go
LLMRetryAttempts        int           `env:"TUNING_LLM_RETRY_ATTEMPTS"`
```

No `envDefault` tag — zero value means "use default of 3", handled in the call site.

- [ ] **Step 4: Extend `newServiceFull` to accept `retryAttempts`**

In `internal/gateway/service_test.go`, update `newServiceFull` signature and body to add the new parameter:

```go
// before:
func newServiceFull(t *testing.T, ch channel.Channel, llm providers.LLMProvider, mem mempkg.Engine, retryDelay time.Duration) *gateway.Service {

// after:
func newServiceFull(t *testing.T, ch channel.Channel, llm providers.LLMProvider, mem mempkg.Engine, retryDelay time.Duration, retryAttempts int) *gateway.Service {
```

Inside the body, add `LLMRetryAttempts: retryAttempts` to the `Tuning` struct literal.

Update **all existing callers** of `newServiceFull` to pass `0` as the last argument
(zero means "use default 3", preserving current behaviour). Search for all calls:

```bash
grep -n "newServiceFull" internal/gateway/service_test.go
```

Typical callers: `newService` helper and any test that calls `newServiceFull` directly.
Update each one to append `, 0`.

Also update `newService` which wraps `newServiceFull`:

```go
// before:
func newService(t *testing.T, ch channel.Channel, llm providers.LLMProvider) *gateway.Service {
    ...
    return newServiceFull(t, ch, llm, mem, 10*time.Millisecond)
}

// after:
func newService(t *testing.T, ch channel.Channel, llm providers.LLMProvider) *gateway.Service {
    ...
    return newServiceFull(t, ch, llm, mem, 10*time.Millisecond, 0)
}
```

- [ ] **Step 5: Update `chatWithRetry` to use the config field**

In `internal/gateway/loop.go`, inside `chatWithRetry`, find and replace the hardcoded constant:

```go
// before:
const maxAttempts = 3

// after:
maxAttempts := s.cfg.Tuning.LLMRetryAttempts
if maxAttempts <= 0 {
    maxAttempts = 3
}
```

- [ ] **Step 6: Document in sample.env**

Append at the end of `sample.env` (after the `TUNING_MCP_CALL_TIMEOUT` block):

```
# Max retries for transient LLM errors (5xx, timeouts). 0 or unset = 3.
# TUNING_LLM_RETRY_ATTEMPTS=3
```

- [ ] **Step 7: Run the new test**

```bash
go test ./internal/gateway/... -run TestGateway_Retry_ConfigurableAttempts -v
```

Expected: PASS

- [ ] **Step 8: Run the full gateway and config test suites**

```bash
go test ./internal/gateway/... ./internal/config/... -v
```

Expected: all PASS. Existing `TestGateway_Retry_ExhaustsAndErrors` still expects 3 calls
— it uses `newServiceFull(..., 0)` which defaults to 3.

- [ ] **Step 9: Commit**

```bash
git add internal/config/config.go internal/gateway/loop.go internal/gateway/service_test.go sample.env
git commit -m "feat(config): make LLM retry attempts configurable via TUNING_LLM_RETRY_ATTEMPTS"
```

---

## Chunk 2: Task B — Context window error recovery

### Task 3: Add ErrKindContextWindow to error classification

**Files:**
- Modify: `internal/providers/errors.go`
- Modify: `internal/providers/errors_test.go`

`ErrKindContextWindow` is **appended** as the 4th constant after the existing three iota
values — it is not inserted between them. Insertion order in the `ClassifyError` function
body is separate from iota order.

- [ ] **Step 1: Write the failing tests**

Add to `internal/providers/errors_test.go`:

```go
func TestClassifyError_ContextWindow(t *testing.T) {
	ctxWindow := []error{
		errors.New("context_length_exceeded"),
		errors.New("openai: 400 context_length_exceeded"),
		errors.New("maximum context length is 128000 tokens"),
		errors.New("too many tokens in your prompt"),
		errors.New("reduce the length of your messages"),
		errors.New("tokens in your prompt: 130000"),
		// 500 + context pattern must be context-window, NOT retryable.
		errors.New("500 context_length_exceeded"),
	}
	for _, err := range ctxWindow {
		if got := providers.ClassifyError(err); got != providers.ErrKindContextWindow {
			t.Errorf("ClassifyError(%q) = %v; want ErrKindContextWindow", err, got)
		}
	}
}

// TestClassifyError_ContextWindow_NoOverlapRateLimit ensures "too many requests"
// stays ErrKindRateLimit — not confused with "too many tokens".
func TestClassifyError_ContextWindow_NoOverlapRateLimit(t *testing.T) {
	rl := errors.New("429 too many requests")
	if got := providers.ClassifyError(rl); got != providers.ErrKindRateLimit {
		t.Errorf("ClassifyError(%q) = %v; want ErrKindRateLimit", rl, got)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/providers/... -run TestClassifyError_ContextWindow -v
```

Expected: FAIL — `providers.ErrKindContextWindow` undefined.

- [ ] **Step 3: Add the new error kind and detection**

Replace the contents of `internal/providers/errors.go` with:

```go
// internal/providers/errors.go
package providers

import (
	"context"
	"errors"
	"strings"
)

// ErrKind classifies an LLM provider error into a retry strategy.
type ErrKind int

const (
	// ErrKindTerminal means fail fast — don't retry (4xx except 429, format errors).
	ErrKindTerminal ErrKind = iota
	// ErrKindRetryable means retry with exponential backoff (5xx, timeout).
	ErrKindRetryable
	// ErrKindRateLimit means retry with backoff and notify the user (429).
	ErrKindRateLimit
	// ErrKindContextWindow means the context limit was exceeded — compress history and retry once.
	// Appended last so existing iota values are unchanged.
	ErrKindContextWindow
)

// ClassifyError maps an LLM provider error to a retry strategy.
// Uses string matching because each provider wraps HTTP errors differently;
// this keeps classification provider-agnostic without requiring type assertions.
func ClassifyError(err error) ErrKind {
	if err == nil {
		return ErrKindTerminal
	}

	// context.DeadlineExceeded / context.Canceled → retryable timeout
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrKindRetryable
	}

	msg := err.Error()
	lower := strings.ToLower(msg)

	// Context window exceeded — checked BEFORE the 5xx block so that error strings
	// like "500 context_length_exceeded" are classified as context-window, not retryable.
	if strings.Contains(lower, "context_length_exceeded") ||
		strings.Contains(lower, "maximum context length") ||
		strings.Contains(lower, "too many tokens") ||
		strings.Contains(lower, "reduce the length") ||
		strings.Contains(lower, "tokens in your prompt") {
		return ErrKindContextWindow
	}

	// 429 rate limit
	if strings.Contains(msg, "429") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "too many requests") {
		return ErrKindRateLimit
	}

	// 5xx server errors
	for _, code := range []string{"500", "502", "503", "504"} {
		if strings.Contains(msg, code) {
			return ErrKindRetryable
		}
	}

	// Timeout / deadline keywords (some SDKs surface these in message text)
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline") {
		return ErrKindRetryable
	}

	return ErrKindTerminal
}
```

- [ ] **Step 4: Run all provider tests**

```bash
go test ./internal/providers/... -v
```

Expected: all PASS including the new context-window tests.

- [ ] **Step 5: Commit**

```bash
git add internal/providers/errors.go internal/providers/errors_test.go
git commit -m "feat(providers): add ErrKindContextWindow for context-limit errors"
```

---

### Task 4: Implement compressMessages

**Files:**
- Create: `internal/gateway/compress.go` (package `gateway`, unexported function)
- Create: `internal/gateway/compress_test.go` (package `gateway` — internal, for unexported access)

`compressMessages` signature: `func compressMessages(msgs *[]providers.Message) bool`
Takes a pointer so it can replace the slice in-place. Returns `true` if any messages
were removed, `false` if the slice is unchanged.

- [ ] **Step 1: Write the failing tests**

Create `internal/gateway/compress_test.go`:

```go
// internal/gateway/compress_test.go
package gateway

import (
	"testing"

	"github.com/canhta/gistclaw/internal/providers"
)

func msg(role, content string) providers.Message {
	return providers.Message{Role: role, Content: content}
}

// TestCompressMessages_NoOp_FewTurns: ≤4 non-system messages → no-op.
func TestCompressMessages_NoOp_FewTurns(t *testing.T) {
	msgs := []providers.Message{
		msg("system", "sys"),
		msg("user", "u1"),
		msg("assistant", "a1"),
		msg("tool", "t1"),
		msg("user", "u2"),
	}
	original := make([]providers.Message, len(msgs))
	copy(original, msgs)
	if compressMessages(&msgs) {
		t.Error("expected no-op (false) for ≤4 turns")
	}
	if len(msgs) != len(original) {
		t.Errorf("len changed: got %d; want %d", len(msgs), len(original))
	}
}

// TestCompressMessages_NoOp_NoDyads: candidates have no assistant+tool pairs.
func TestCompressMessages_NoOp_NoDyads(t *testing.T) {
	// 6 non-system messages but candidates are all user messages.
	msgs := []providers.Message{
		msg("system", "sys"),
		msg("user", "c1"),
		msg("user", "c2"),
		msg("user", "tail1"),
		msg("user", "tail2"),
		msg("user", "tail3"),
		msg("user", "tail4"),
	}
	if compressMessages(&msgs) {
		t.Error("expected no-op (false) when no dyads in candidates")
	}
}

// TestCompressMessages_NoOp_OneDyad: floor(1/2)=0 → no-op.
func TestCompressMessages_NoOp_OneDyad(t *testing.T) {
	msgs := []providers.Message{
		msg("system", "sys"),
		msg("assistant", "a0"), msg("tool", "t0"), // 1 dyad in candidates
		msg("user", "tail1"),                       // tail (last 4)
		msg("assistant", "a1"), msg("tool", "t1"),
		msg("user", "tail2"),
	}
	if compressMessages(&msgs) {
		t.Error("expected no-op (false) for 1 dyad where floor(1/2)=0")
	}
}

// TestCompressMessages_DropsDyads: 2 dyads → drop floor(2/2)=1 oldest.
func TestCompressMessages_DropsDyads(t *testing.T) {
	msgs := []providers.Message{
		msg("system", "sys"),
		// candidates (first len(turns)-4 = 4):
		msg("assistant", "drop-a"), msg("tool", "drop-t"), // dyad1 oldest — dropped
		msg("assistant", "keep-a"), msg("tool", "keep-t"), // dyad2 — kept
		// tail (last 4):
		msg("user", "tail-u1"),
		msg("assistant", "tail-a"), msg("tool", "tail-t"),
		msg("user", "tail-u2"),
	}
	before := len(msgs)
	if !compressMessages(&msgs) {
		t.Fatal("expected compression (true)")
	}
	if len(msgs) != before-2 {
		t.Errorf("expected len=%d; got %d", before-2, len(msgs))
	}
	for _, m := range msgs {
		if m.Content == "drop-a" || m.Content == "drop-t" {
			t.Errorf("dropped dyad content %q still present", m.Content)
		}
	}
	found := false
	for _, m := range msgs {
		if m.Content == "keep-a" {
			found = true
		}
	}
	if !found {
		t.Error("kept dyad (keep-a) was incorrectly dropped")
	}
}

// TestCompressMessages_UserMessagesNeverDropped: user in candidates always survives.
func TestCompressMessages_UserMessagesNeverDropped(t *testing.T) {
	msgs := []providers.Message{
		msg("system", "sys"),
		// candidates (8 messages = 2 user anchors + 4 dyads → 2 dropped):
		msg("user", "must-keep"),
		msg("assistant", "a1"), msg("tool", "t1"), // dyad1 — dropped
		msg("assistant", "a2"), msg("tool", "t2"), // dyad2 — dropped
		msg("assistant", "a3"), msg("tool", "t3"), // dyad3 — kept
		msg("assistant", "a4"), msg("tool", "t4"), // dyad4 — kept
		// tail (last 4):
		msg("user", "tail1"),
		msg("assistant", "tail-a"), msg("tool", "tail-t"),
		msg("user", "tail2"),
	}
	if !compressMessages(&msgs) {
		t.Fatal("expected compression")
	}
	for _, m := range msgs {
		if m.Content == "must-keep" {
			return
		}
	}
	t.Error("user message in candidates was incorrectly dropped")
}

// TestCompressMessages_SystemMessagesAlwaysKept.
func TestCompressMessages_SystemMessagesAlwaysKept(t *testing.T) {
	msgs := []providers.Message{
		msg("system", "sys1"),
		msg("system", "sys2"),
		msg("assistant", "a1"), msg("tool", "t1"),
		msg("assistant", "a2"), msg("tool", "t2"),
		msg("assistant", "a3"), msg("tool", "t3"),
		msg("assistant", "a4"), msg("tool", "t4"),
		msg("user", "tail1"),
		msg("assistant", "tail-a"), msg("tool", "tail-t"),
		msg("user", "tail2"),
	}
	if !compressMessages(&msgs) {
		t.Fatal("expected compression")
	}
	sysCount := 0
	for _, m := range msgs {
		if m.Role == "system" {
			sysCount++
		}
	}
	if sysCount != 2 {
		t.Errorf("expected 2 system messages; got %d", sysCount)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/gateway/... -run TestCompressMessages -v
```

Expected: compile error — `compressMessages` undefined.

- [ ] **Step 3: Implement compressMessages**

Create `internal/gateway/compress.go`:

```go
// internal/gateway/compress.go
package gateway

import "github.com/canhta/gistclaw/internal/providers"

// compressMessages reduces the message history in-place when a context-window
// limit is hit. It drops the oldest floor(N/2) assistant+tool dyads from the
// candidate window (all non-system turns except the last 4).
//
// Returns true if at least one dyad was dropped; false if unchanged (no-op).
// Callers MUST NOT retry when false is returned — nothing was compressed.
//
// Rules:
//   - system (role "system"): always kept, placed first in output.
//   - user messages: always kept — dropping them corrupts strict-alternation APIs.
//   - assistant+tool consecutive pair ("dyad"): the only droppable unit.
//   - tail: the last 4 non-system messages are always preserved.
//   - drop = len(dyads)/2 (Go floor division); if drop==0 → no-op, return false.
func compressMessages(msgs *[]providers.Message) bool {
	var system, turns []providers.Message
	for _, m := range *msgs {
		if m.Role == "system" {
			system = append(system, m)
		} else {
			turns = append(turns, m)
		}
	}

	if len(turns) <= 4 {
		return false
	}

	tailStart := len(turns) - 4
	candidates := turns[:tailStart]
	tail := turns[tailStart:]

	// Collect consecutive assistant+tool dyads from candidates.
	type dyad struct{ start, end int }
	var dyads []dyad
	for i := 0; i < len(candidates)-1; i++ {
		if candidates[i].Role == "assistant" && candidates[i+1].Role == "tool" {
			dyads = append(dyads, dyad{i, i + 1})
			i++ // advance past the tool message
		}
	}

	drop := len(dyads) / 2 // floor division; covers dyads==0 and dyads==1 cases
	if drop == 0 {
		return false
	}

	// Mark indices of messages to remove (oldest dyads first).
	dropIdx := make(map[int]bool)
	for _, d := range dyads[:drop] {
		dropIdx[d.start] = true
		dropIdx[d.end] = true
	}

	result := make([]providers.Message, 0, len(*msgs)-drop*2)
	result = append(result, system...)
	for i, m := range candidates {
		if !dropIdx[i] {
			result = append(result, m)
		}
	}
	result = append(result, tail...)
	*msgs = result
	return true
}
```

- [ ] **Step 4: Run compress tests**

```bash
go test ./internal/gateway/... -run TestCompressMessages -v
```

Expected: all PASS

- [ ] **Step 5: Run full gateway suite**

```bash
go test ./internal/gateway/... -v
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/gateway/compress.go internal/gateway/compress_test.go
git commit -m "feat(gateway): add compressMessages for context-window history recovery"
```

---

### Task 5: Wire context-window recovery into chatWithRetry

**Files:**
- Modify: `internal/gateway/loop.go`
- Modify: `internal/gateway/service_test.go`

`chatWithRetry` currently takes `msgs []providers.Message`. We change it to
`msgs *[]providers.Message` so `compressMessages` can modify the slice in-place.
There are **exactly three call sites** in `handlePlainChat`:
1. The max-iterations forced final answer call (~line 78)
2. The main tool loop call (~line 89)
3. The doom-loop guard forced final answer call (~line 129)

All three must be updated to pass `&msgs`.

- [ ] **Step 1: Write the failing integration tests**

The context-window tests need a conversation with enough history to trigger dyad dropping.
We pre-seed the SQLite store with assistant+tool history pairs so that when
`handlePlainChat` calls `conv.Load`, the messages slice has >4 non-system turns.

Add a helper and tests to `internal/gateway/service_test.go`:

```go
// newServiceWithSeededHistory creates a service with pre-seeded conversation history.
// It saves `pairs` assistant+tool pairs for chatID 42 in the store, so that
// handlePlainChat will load them as history and have enough messages for compression.
func newServiceWithSeededHistory(t *testing.T, ch channel.Channel, llm providers.LLMProvider, pairs int) *gateway.Service {
	t.Helper()
	s := newTestStore(t)
	// Seed pairs*2 messages (assistant+tool) for chatID=42.
	for i := 0; i < pairs; i++ {
		if err := s.SaveMessage(42, "assistant", fmt.Sprintf("tool-call-%d", i)); err != nil {
			t.Fatalf("seed SaveMessage assistant: %v", err)
		}
		if err := s.SaveMessage(42, "tool", fmt.Sprintf("tool-result-%d", i)); err != nil {
			t.Fatalf("seed SaveMessage tool: %v", err)
		}
	}
	sched := newTestScheduler(t, s)
	cfg := config.Config{
		AllowedUserIDs: []int64{42},
		Tuning: config.Tuning{
			SchedulerTick:           time.Second,
			MissedJobsFireLimit:     5,
			MaxIterations:           20,
			LLMRetryDelay:           10 * time.Millisecond,
			ConversationWindowTurns: 20,
			LLMRetryAttempts:        0, // default 3
		},
	}
	conv := conversation.NewManager(s, cfg.Tuning.ConversationWindowTurns, 0)
	return gateway.NewService(
		ch,
		&mockApprover{},
		&mockOCService{isAlive: false},
		&mockCCService{isAlive: false},
		llm,
		&mockSearch{},
		&mockFetcher{},
		mcp.NewMCPManager(nil, config.Tuning{}),
		sched,
		s,
		nil,
		nil,
		conv,
		time.Now(),
		cfg,
	)
}

// TestGateway_ContextWindow_CompressesAndRetries verifies that a context-window
// error triggers history compression and a single retry that succeeds.
// Pre-seeds 4 assistant+tool pairs so compressMessages has dyads to drop.
// Total LLM calls: 2 (1 context error + 1 successful retry).
func TestGateway_ContextWindow_CompressesAndRetries(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{
		errs: []error{
			errors.New("context_length_exceeded"),
			nil, // retry succeeds
		},
		responses: []*providers.LLMResponse{
			nil,
			{Content: "compressed answer"},
		},
	}
	// 4 pairs = 8 history messages + 1 user = 9 non-system turns → compressMessages runs.
	svc := newServiceWithSeededHistory(t, ch, llm, 4)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "hello"}
	time.Sleep(300 * time.Millisecond)

	if calls := llm.calls(); calls != 2 {
		t.Errorf("expected 2 LLM calls (context error + retry); got %d", calls)
	}
	msgs := ch.sentMessages()
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "compressed answer") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'compressed answer' after retry; got: %v", msgs)
	}
}

// TestGateway_ContextWindow_NoOpPropagates verifies that when compression is a
// no-op (≤4 turns), the error propagates without retry. Total LLM calls: 1.
func TestGateway_ContextWindow_NoOpPropagates(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{
		errs: []error{errors.New("context_length_exceeded")},
	}
	// newService has no history → ≤4 turns → compression is no-op.
	svc := newService(t, ch, llm)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "hello"}
	time.Sleep(200 * time.Millisecond)

	if calls := llm.calls(); calls != 1 {
		t.Errorf("expected exactly 1 LLM call when compression is no-op; got %d", calls)
	}
	msgs := ch.sentMessages()
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "⚠️") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error message to user; got: %v", msgs)
	}
}

// TestGateway_ContextWindow_RetryAlsoFails verifies that if both the call and
// the retry fail with context errors, the user gets an error message.
// Uses 4 seeded pairs so compression runs. Total LLM calls: 2.
func TestGateway_ContextWindow_RetryAlsoFails(t *testing.T) {
	ch := newMockChannel()
	ctxErr := errors.New("context_length_exceeded")
	llm := &mockLLM{
		errs: []error{ctxErr, ctxErr},
	}
	svc := newServiceWithSeededHistory(t, ch, llm, 4)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "hello"}
	time.Sleep(300 * time.Millisecond)

	if calls := llm.calls(); calls != 2 {
		t.Errorf("expected 2 LLM calls (compress + retry fails); got %d", calls)
	}
	msgs := ch.sentMessages()
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "⚠️") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error message to user after retry failure; got: %v", msgs)
	}
}
```

Also add `"fmt"` to the imports in `service_test.go` if not already present (needed for `fmt.Sprintf` in `newServiceWithSeededHistory`).

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/gateway/... -run TestGateway_ContextWindow -v
```

Expected: FAIL — `ErrKindContextWindow` not handled in `chatWithRetry` yet, so all context-window errors propagate as terminal after 0 retries... or compile error if `newServiceWithSeededHistory` references `s.SaveMessage` which must match the actual store API.

Verify `store.Store.SaveMessage` exists:
```bash
grep -n "func.*SaveMessage" internal/store/sqlite.go
```

- [ ] **Step 3: Update chatWithRetry signature**

In `internal/gateway/loop.go`, change the function signature from:
```go
func (s *Service) chatWithRetry(ctx context.Context, chatID int64, msgs []providers.Message, tools []providers.Tool) (*providers.LLMResponse, error) {
```
to:
```go
func (s *Service) chatWithRetry(ctx context.Context, chatID int64, msgs *[]providers.Message, tools []providers.Tool) (*providers.LLMResponse, error) {
```

Inside `chatWithRetry`, replace the `s.llm.Chat` call to dereference the pointer:
```go
// before:
resp, err := s.llm.Chat(ctx, msgs, tools)
// after:
resp, err := s.llm.Chat(ctx, *msgs, tools)
```

- [ ] **Step 4: Add the ErrKindContextWindow branch**

Inside `chatWithRetry`, add the new case in the switch statement, **before** the
`ErrKindTerminal` return. Place it after the `ErrKindRateLimit` fallthrough block:

```go
case providers.ErrKindContextWindow:
    before := len(*msgs)
    if !compressMessages(msgs) {
        // No dyads dropped (≤4 turns or floor(dyads/2)==0) — propagate immediately.
        return nil, err
    }
    after := len(*msgs)
    log.Warn().
        Int("before", before).
        Int("after", after).
        Msg("gateway: context window exceeded; compressed history, retrying once")
    // One dedicated retry, orthogonal to the maxAttempts loop.
    resp2, err2 := s.llm.Chat(ctx, *msgs, tools)
    if err2 != nil {
        return nil, err2
    }
    return resp2, nil
```

- [ ] **Step 5: Update the doc comment on chatWithRetry**

Replace the existing comment with:

```go
// chatWithRetry calls the LLM with retry logic for transient errors.
//
// Four behaviours based on ClassifyError:
//   - ErrKindContextWindow: compress history (drop oldest floor(N/2) dyads),
//     then retry exactly once. Orthogonal to the maxAttempts loop.
//   - ErrKindRetryable (5xx, timeout): up to maxAttempts retries with exponential backoff.
//   - ErrKindRateLimit (429): same backoff, plus a one-time user notification.
//   - ErrKindTerminal (4xx, format errors): fail immediately, no retry.
//
// msgs is a pointer so compressMessages can modify the slice in-place; all three
// call sites in handlePlainChat pass &msgs.
```

- [ ] **Step 6: Update all three call sites in handlePlainChat**

In `handlePlainChat`, find each call to `chatWithRetry` and add `&`:

**Call site 1** — max-iterations forced final answer:
```go
// before:
finalResp, ferr := s.chatWithRetry(ctx, chatID, msgs, nil)
// after:
finalResp, ferr := s.chatWithRetry(ctx, chatID, &msgs, nil)
```

**Call site 2** — main tool loop:
```go
// before:
resp, err := s.chatWithRetry(ctx, chatID, msgs, toolRegistry)
// after:
resp, err := s.chatWithRetry(ctx, chatID, &msgs, toolRegistry)
```

**Call site 3** — doom-loop guard forced final answer:
```go
// before:
finalResp, ferr := s.chatWithRetry(ctx, chatID, msgs, nil)
// after:
finalResp, ferr := s.chatWithRetry(ctx, chatID, &msgs, nil)
```

Confirm there are no other `s.chatWithRetry` calls in the file:
```bash
grep -n "chatWithRetry" internal/gateway/loop.go
```
Expected: exactly 4 lines — 1 definition + 3 call sites.

- [ ] **Step 7: Run the new tests**

```bash
go test ./internal/gateway/... -run TestGateway_ContextWindow -v
```

Expected: all PASS

- [ ] **Step 8: Run the full gateway suite**

```bash
go test ./internal/gateway/... -v
```

Expected: all PASS — existing retry tests unaffected (they use `ErrKindRetryable` /
`ErrKindRateLimit` / `ErrKindTerminal` paths which are unchanged).

- [ ] **Step 9: Build everything**

```bash
go build ./...
```

Expected: no errors

- [ ] **Step 10: Run all tests**

```bash
go test ./...
```

Expected: all PASS

- [ ] **Step 11: Commit**

```bash
git add internal/gateway/loop.go internal/gateway/service_test.go
git commit -m "feat(gateway): recover from context-window errors via history compression"
```
