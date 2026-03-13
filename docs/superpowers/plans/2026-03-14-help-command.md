# LLM-Generated /start and /help Commands Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `/start` and `/help` commands that send a one-time LLM-generated capability summary in Tèo's own voice, cached in memory for the process lifetime, with a hardcoded fallback if the LLM fails.

**Architecture:** Two new command cases in `router.go` both call a `handleHelp` method on `Service`. `handleHelp` uses `sync.Once` to call the LLM exactly once, building the prompt from the live SOUL.md system context and a hardcoded tool/command list. The result is cached in `Service.cachedHelp`; on LLM failure the hardcoded fallback is sent instead (and cached as empty so fallback is used for the rest of the process lifetime).

**Tech Stack:** Go 1.25, `sync.Once`, `github.com/rs/zerolog/log`, existing `providers.LLMProvider` interface, existing `memory.Engine` interface.

**Spec:** `docs/superpowers/specs/2026-03-14-help-command-design.md`

---

## Chunk 1: Fields, fallback constant, and handleHelp method

### Task 1: Add `helpOnce` and `cachedHelp` fields to `Service`

**Files:**
- Modify: `internal/gateway/service.go`

- [ ] **Step 1: Add the two new fields to the `Service` struct**

  In `internal/gateway/service.go`, add `"sync"` to the stdlib import group (alongside `"context"`, `"fmt"`, `"time"`):

  ```go
  import (
      "context"
      "fmt"
      "sync"
      "time"
      ...
  )
  ```

  Then add to the `Service` struct after the `lifetimeCtx` field:

  ```go
  helpOnce   sync.Once
  cachedHelp string
  ```

- [ ] **Step 2: Verify the file compiles**

  ```bash
  go build ./internal/gateway/...
  ```

  Expected: no errors.

---

### Task 2: Add `helpFallback` const and `/start`+`/help` routing in `router.go`

**Files:**
- Modify: `internal/gateway/router.go`

- [ ] **Step 1: Add the fallback constant at package level**

  At the top of `internal/gateway/router.go`, after the `package gateway` line and imports, add:

  ```go
  const helpFallback = "Tèo đây! Tao có thể làm:\n" +
      "/oc <task>  — chạy OpenCode\n" +
      "/cc <task>  — chạy Claude Code\n" +
      "/status     — xem trạng thái\n" +
      "/stop       — dừng agent\n" +
      "Chat thường: hỏi gì cũng được, tao có web search, memory, scheduler."
  ```

- [ ] **Step 2: Add `/start` and `/help` cases to the `switch` in `handle`**

  In the `switch` block in `handle`, add before the `case text == "/stop":` line:

  ```go
  case text == "/start", text == "/help":
      s.handleHelp(ctx, msg.ChatID)
  ```

- [ ] **Step 3: Add the `handleHelp` method**

  Append to `internal/gateway/router.go`:

  ```go
  // handleHelp generates (or returns cached) LLM capability summary for /start and /help.
  // Uses sync.Once to call the LLM at most once per process lifetime.
  // Falls back to helpFallback if s.llm is nil or the LLM call fails.
  func (s *Service) handleHelp(ctx context.Context, chatID int64) {
      if s.llm == nil {
          _ = s.ch.SendMessage(ctx, chatID, helpFallback)
          return
      }

      s.helpOnce.Do(func() {
          var msgs []providers.Message

          // System context from SOUL.md + memory (same as plain chat).
          if s.memory != nil {
              if sysContent := s.memory.LoadContext(); sysContent != "" {
                  msgs = append(msgs, providers.Message{Role: "system", Content: sysContent})
              }
          }

          // Inject tool/command list and ask the LLM to describe capabilities.
          const toolList = "Available commands and tools:\n" +
              "- /oc <task>: submit a coding task to OpenCode agent\n" +
              "- /cc <task>: submit a coding task to Claude Code agent\n" +
              "- /stop: stop the currently running agent\n" +
              "- /status: show bot uptime, agent status, cost, scheduled jobs\n" +
              "- web_search: search the web (Brave Search) — Tèo calls this automatically when needed\n" +
              "- web_fetch: fetch and read a URL — Tèo calls this automatically when needed\n" +
              "- remember / note: Tèo saves facts and notes to memory automatically during conversations\n" +
              "- schedule_job / list_jobs / delete_job: manage cron-based scheduled tasks\n" +
              "- spawn_agent / run_parallel / chain_agents: orchestrate multiple AI agents\n\n" +
              "Describe what you can do for the user. Use your own voice and personality. Be concise."

          msgs = append(msgs, providers.Message{Role: "user", Content: toolList})

          resp, err := s.llm.Chat(ctx, msgs, nil)
          if err != nil {
              log.Warn().Err(err).Msg("gateway: handleHelp LLM error; using fallback")
              return
          }
          s.cachedHelp = resp.Content
      })

      if s.cachedHelp == "" {
          _ = s.ch.SendMessage(ctx, chatID, helpFallback)
          return
      }
      _ = s.ch.SendMessage(ctx, chatID, s.cachedHelp)
  }
  ```

  Make sure `providers` is imported in `router.go`. It is **not** currently imported — add it to the internal imports group:
  ```go
  import (
      "context"
      "fmt"
      "sort"
      "strings"
      "time"

      "github.com/rs/zerolog/log"

      "github.com/canhta/gistclaw/internal/channel"
      "github.com/canhta/gistclaw/internal/providers"
  )
  ```

- [ ] **Step 4: Verify the file compiles**

  ```bash
  go build ./internal/gateway/...
  ```

  Expected: no errors.

---

## Chunk 1: Tests (write first — red phase)

### Task 1: Write failing tests for handleHelp

**Files:**
- Modify: `internal/gateway/service_test.go`

All three tests follow the established gateway test pattern:
1. Create service with `newService(t, ch, llm)`.
2. Start `svc.Run(ctx)` in a goroutine.
3. Inject message via `ch.inbound <-`.
4. `time.Sleep(150 * time.Millisecond)` to let the loop process it.
5. Assert on `ch.sentMessages()` and `llm.calls()`.

- [ ] **Step 1: Write `TestHandleHelpLLMSuccess`**

  Append to `internal/gateway/service_test.go`:

  ```go
  func TestHandleHelpLLMSuccess(t *testing.T) {
      ch := newMockChannel()
      llm := &mockLLM{
          responses: []*providers.LLMResponse{
              {Content: "mocked help text"},
          },
      }
      svc := newService(t, ch, llm)

      ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
      defer cancel()
      go svc.Run(ctx) //nolint:errcheck

      // First call: /start — should trigger LLM and send cached response.
      ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "/start"}
      time.Sleep(150 * time.Millisecond)

      msgs := ch.sentMessages()
      if len(msgs) == 0 {
          t.Fatal("expected a message after /start, got none")
      }
      if msgs[len(msgs)-1] != "mocked help text" {
          t.Errorf("/start reply = %q, want %q", msgs[len(msgs)-1], "mocked help text")
      }

      // Second call: /help — should use cache, not call LLM again.
      ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "/help"}
      time.Sleep(150 * time.Millisecond)

      msgs = ch.sentMessages()
      if msgs[len(msgs)-1] != "mocked help text" {
          t.Errorf("/help reply = %q, want %q", msgs[len(msgs)-1], "mocked help text")
      }
      if llm.calls() != 1 {
          t.Errorf("LLM called %d times, want 1 (cache hit on /help)", llm.calls())
      }
  }
  ```

- [ ] **Step 2: Write `TestHandleHelpLLMFailure`**

  ```go
  func TestHandleHelpLLMFailure(t *testing.T) {
      ch := newMockChannel()
      llm := &mockLLM{
          errs: []error{errors.New("llm down")},
      }
      svc := newService(t, ch, llm)

      ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
      defer cancel()
      go svc.Run(ctx) //nolint:errcheck

      ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "/help"}
      time.Sleep(150 * time.Millisecond)

      msgs := ch.sentMessages()
      if len(msgs) == 0 {
          t.Fatal("expected a fallback message, got none")
      }
      if !strings.Contains(msgs[len(msgs)-1], "/oc") {
          t.Errorf("fallback message = %q, want it to contain /oc", msgs[len(msgs)-1])
      }
      if llm.calls() != 1 {
          t.Errorf("LLM called %d times, want exactly 1", llm.calls())
      }
  }
  ```

- [ ] **Step 3: Write `TestHandleHelpNilLLM`**

  ```go
  func TestHandleHelpNilLLM(t *testing.T) {
      ch := newMockChannel()
      svc := newService(t, ch, nil) // nil LLM

      ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
      defer cancel()
      go svc.Run(ctx) //nolint:errcheck

      ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "/help"}
      time.Sleep(150 * time.Millisecond)

      msgs := ch.sentMessages()
      if len(msgs) == 0 {
          t.Fatal("expected a fallback message, got none")
      }
      if !strings.Contains(msgs[len(msgs)-1], "/oc") {
          t.Errorf("fallback message = %q, want it to contain /oc", msgs[len(msgs)-1])
      }
  }
  ```

- [ ] **Step 4: Run all three tests to confirm they all fail**

  ```bash
  go test ./internal/gateway/... -run "TestHandleHelp" -v
  ```

  Expected: all FAIL — `/start` and `/help` are unrecognised commands, plain-chat fallback fires instead of help text.

---

## Chunk 2: Implementation (green phase)

### Task 2: Add `helpOnce` and `cachedHelp` fields to `Service`

**Files:**
- Modify: `internal/gateway/service.go`

- [ ] **Step 1: Add `"sync"` to the stdlib import group and add the two new fields to the `Service` struct**

  In `internal/gateway/service.go`, add `"sync"` to the stdlib import group (alongside `"context"`, `"fmt"`, `"time"`):

  ```go
  import (
      "context"
      "fmt"
      "sync"
      "time"
      ...
  )
  ```

  Then add to the `Service` struct after the `lifetimeCtx` field:

  ```go
  helpOnce   sync.Once
  cachedHelp string
  ```

- [ ] **Step 2: Verify the file compiles**

  ```bash
  go build ./internal/gateway/...
  ```

  Expected: no errors.

---

### Task 3: Add `helpFallback` const, `/start`+`/help` routing, and `handleHelp` in `router.go`

**Files:**
- Modify: `internal/gateway/router.go`

- [ ] **Step 1: Add the fallback constant at package level**

  At the top of `internal/gateway/router.go`, after the `package gateway` line and imports, add:

  ```go
  const helpFallback = "Tèo đây! Tao có thể làm:\n" +
      "/oc <task>  — chạy OpenCode\n" +
      "/cc <task>  — chạy Claude Code\n" +
      "/status     — xem trạng thái\n" +
      "/stop       — dừng agent\n" +
      "Chat thường: hỏi gì cũng được, tao có web search, memory, scheduler."
  ```

- [ ] **Step 2: Add `/start` and `/help` cases to the `switch` in `handle`**

  In the `switch` block in `handle`, add before the `case text == "/stop":` line:

  ```go
  case text == "/start", text == "/help":
      s.handleHelp(ctx, msg.ChatID)
  ```

- [ ] **Step 3: Update the imports in `router.go` to include `providers`**

  `providers` is **not** currently imported in `router.go`. Update the import block to:

  ```go
  import (
      "context"
      "fmt"
      "sort"
      "strings"
      "time"

      "github.com/rs/zerolog/log"

      "github.com/canhta/gistclaw/internal/channel"
      "github.com/canhta/gistclaw/internal/providers"
  )
  ```

- [ ] **Step 4: Add the `handleHelp` method**

  Append to `internal/gateway/router.go`:

  ```go
  // handleHelp generates (or returns cached) LLM capability summary for /start and /help.
  // Uses sync.Once to call the LLM at most once per process lifetime.
  // Falls back to helpFallback if s.llm is nil or the LLM call fails.
  func (s *Service) handleHelp(ctx context.Context, chatID int64) {
      if s.llm == nil {
          _ = s.ch.SendMessage(ctx, chatID, helpFallback)
          return
      }

      s.helpOnce.Do(func() {
          var msgs []providers.Message

          // System context from SOUL.md + memory (same as plain chat).
          if s.memory != nil {
              if sysContent := s.memory.LoadContext(); sysContent != "" {
                  msgs = append(msgs, providers.Message{Role: "system", Content: sysContent})
              }
          }

          // Inject tool/command list and ask the LLM to describe capabilities.
          const toolList = "Available commands and tools:\n" +
              "- /oc <task>: submit a coding task to OpenCode agent\n" +
              "- /cc <task>: submit a coding task to Claude Code agent\n" +
              "- /stop: stop the currently running agent\n" +
              "- /status: show bot uptime, agent status, cost, scheduled jobs\n" +
              "- web_search: search the web (Brave Search) — Tèo calls this automatically when needed\n" +
              "- web_fetch: fetch and read a URL — Tèo calls this automatically when needed\n" +
              "- remember / note: Tèo saves facts and notes to memory automatically during conversations\n" +
              "- schedule_job / list_jobs / delete_job: manage cron-based scheduled tasks\n" +
              "- spawn_agent / run_parallel / chain_agents: orchestrate multiple AI agents\n\n" +
              "Describe what you can do for the user. Use your own voice and personality. Be concise."

          msgs = append(msgs, providers.Message{Role: "user", Content: toolList})

          resp, err := s.llm.Chat(ctx, msgs, nil)
          if err != nil {
              log.Warn().Err(err).Msg("gateway: handleHelp LLM error; using fallback")
              return
          }
          s.cachedHelp = resp.Content
      })

      if s.cachedHelp == "" {
          _ = s.ch.SendMessage(ctx, chatID, helpFallback)
          return
      }
      _ = s.ch.SendMessage(ctx, chatID, s.cachedHelp)
  }
  ```

- [ ] **Step 5: Run the three new tests — expect green**

  ```bash
  go test ./internal/gateway/... -run "TestHandleHelp" -v
  ```

  Expected: all three PASS.

- [ ] **Step 6: Run the full test suite**

  ```bash
  go test ./...
  ```

  Expected: all packages pass.

- [ ] **Step 7: Run the linter**

  ```bash
  make lint
  ```

  Expected: no new lint errors.

- [ ] **Step 8: Commit**

  ```bash
  git add internal/gateway/service.go internal/gateway/router.go internal/gateway/service_test.go
  git commit -m "feat(gateway): add /start and /help with LLM-generated capability summary"
  ```

---

## Chunk 3: Build and deploy

### Task 4: Build and restart the bot

- [ ] **Step 1: Build the binary**

  ```bash
  go build -o /tmp/gistclaw ./cmd/gistclaw
  ```

  Expected: no errors.

- [ ] **Step 2: Restart the running bot**

  ```bash
  pkill -f '/tmp/gistclaw' || true
  sleep 1
  cd /home/ubuntu/projects/gistclaw && env $(cat .env | grep -v '^#' | xargs) nohup /tmp/gistclaw > /tmp/gistclaw.log 2>&1 &
  echo "PID=$!"
  ```

- [ ] **Step 3: Confirm the bot is running**

  ```bash
  sleep 2 && tail -5 /tmp/gistclaw.log
  ```

  Expected: log line with `"message":"opencode: server already running, skipping spawn"` or similar startup info. No `ERROR` lines.

- [ ] **Step 4: Push to main**

  ```bash
  git push origin main
  ```
