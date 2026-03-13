# Extending GistClaw

This document explains how to add new capabilities to GistClaw without modifying
existing services. Three extension patterns are supported by the architecture:

1. [Adding a new channel](#1-adding-a-new-channel)
2. [Adding a new LLM provider](#2-adding-a-new-llm-provider)
3. [Adding a new agent kind](#3-adding-a-new-agent-kind)

---

## 1. Adding a new channel

A "channel" is a chat platform adapter. The v1 implementation is Telegram. Adding
Discord, Slack, or any other platform follows this pattern.

1. Create `internal/channel/<name>/<name>.go` implementing `channel.Channel`:

   ```go
   package <name>

   import (
       "context"
       "github.com/canhta/gistclaw/internal/channel"
   )

   type Channel struct { /* platform-specific fields */ }

   func New(cfg Config) (*Channel, error) { /* ... */ }

   func (c *Channel) Receive(ctx context.Context) (<-chan channel.InboundMessage, error) { /* ... */ }
   func (c *Channel) SendMessage(ctx context.Context, chatID int64, text string) error  { /* ... */ }
   func (c *Channel) SendKeyboard(ctx context.Context, chatID int64, payload channel.KeyboardPayload) error { /* ... */ }
   func (c *Channel) SendTyping(ctx context.Context, chatID int64) error                { /* ... */ }
   func (c *Channel) Name() string { return "<name>" }
   ```

   The five methods are the complete contract. Platform-specific types (SDKs, API clients)
   are confined to this package — nothing else imports them.

2. Add any required env vars to `internal/config/config.go` and `sample.env`.
   Example: `DISCORD_BOT_TOKEN`, `DISCORD_CHANNEL_ID`.

3. Add a case to the channel factory in `internal/app/app.go` (`NewApp`):

   ```go
   // internal/app/app.go — inside NewApp
   var ch channel.Channel
   switch cfg.Channel {
   case "telegram":
       ch, err = telegram.New(cfg)
   case "<name>":
       ch, err = <name>.New(cfg)
   default:
       return nil, fmt.Errorf("unknown channel: %s", cfg.Channel)
   }
   ```

   Set `CHANNEL=<name>` in `.env` to activate the new adapter.

4. `gateway.Service` does not change — it only uses `channel.Channel`. HITL does not
   change — it only constructs `channel.KeyboardPayload` (no platform types). The
   `channel_state` table in SQLite is keyed by channel ID, so dedup works for all
   channel implementations without schema changes.

**What you do NOT need to change:** `gateway`, `hitl`, `opencode`, `claudecode`,
`scheduler`, `store`, any provider, any tool.

---

## 2. Adding a new LLM provider

A "provider" implements the `providers.LLMProvider` interface. Adding a new model
backend (e.g., Anthropic direct API, a local Ollama server) follows this pattern.

1. Create `internal/providers/<name>/<name>.go` implementing `providers.LLMProvider`:

   ```go
   package <name>

   import (
       "context"
       "github.com/canhta/gistclaw/internal/providers"
   )

   type Provider struct { /* API client, config */ }

   func New(cfg Config) (*Provider, error) { /* ... */ }

   func (p *Provider) Chat(ctx context.Context, messages []providers.Message, tools []providers.Tool) (*providers.LLMResponse, error) {
       // Call the backend API
       // Populate Usage.TotalCostUSD:
       //   - exact value if the API returns token prices (like openai-key)
       //   - 0.0 if billing is opaque (like copilot, codex-oauth)
       // A zero TotalCostUSD is valid — it does not trigger cost soft-stop thresholds.
       return &providers.LLMResponse{
           Content: "...",
           Usage: providers.Usage{
               PromptTokens:     123,
               CompletionTokens: 45,
               TotalCostUSD:     0.001, // or 0.0 if unknown
           },
       }, nil
   }

   func (p *Provider) Name() string { return "<name>" }
   ```

2. Add a case to the factory in `internal/providers/llm.go`:

   ```go
   // internal/providers/llm.go — inside New(cfg)
   switch cfg.LLMProvider {
   case "openai-key":
       return openai.New(cfg)
   case "copilot":
       return copilot.New(cfg)
   case "codex-oauth":
       return codex.New(cfg)
   case "<name>":
       return <name>.New(cfg)
   default:
       return nil, fmt.Errorf("unknown LLM_PROVIDER: %s", cfg.LLMProvider)
   }
   ```

3. Add `LLM_PROVIDER=<name>` documentation to `sample.env` (commented out) and `README.md`.

4. `providers.NewTrackingProvider` wraps the new provider automatically — no cost wiring needed.
   `gateway.Service` and `scheduler.Service` both receive the decorated provider via `app.NewApp`.

**What you do NOT need to change:** `gateway`, `hitl`, `opencode`, `claudecode`,
`scheduler`, `store`, any channel, any tool.

---

## 3. Adding a new agent kind

An "agent kind" identifies which agent service handles a job dispatched by the scheduler
or triggered by the gateway. The typed enum lives in `internal/agent/kind.go`.

1. Add a constant to `internal/agent/kind.go`:

   ```go
   // internal/agent/kind.go
   package agent

   type Kind int

   const (
       // KindUnknown is a sentinel; explicit -1 so zero-value Kind is not treated as KindOpenCode.
       KindUnknown    Kind = -1
       KindOpenCode   Kind = 0
       KindClaudeCode Kind = 1
       KindChat       Kind = 2
       KindNewAgent   Kind = 3 // add here
   )
   ```

2. Add a case to `String()` in the same file:

   ```go
   func (k Kind) String() string {
       switch k {
       case KindOpenCode:   return "opencode"
       case KindClaudeCode: return "claudecode"
       case KindChat:       return "chat"
       case KindNewAgent:   return "newagent"
       default:             return fmt.Sprintf("unknown(%d)", int(k))
       }
   }
   ```

3. Add a case to `KindFromString` (used when scanning from the SQLite `jobs.target` column):

   ```go
   func KindFromString(s string) (Kind, error) {
       switch s {
       case "opencode":   return KindOpenCode, nil
       case "claudecode": return KindClaudeCode, nil
       case "chat":       return KindChat, nil
       case "newagent":   return KindNewAgent, nil
       default:           return KindUnknown, fmt.Errorf("agent: unknown kind %q", s)
       }
   }
   ```

4. Add a case to the `JobTarget` implementation in `internal/app/app.go`:

   ```go
   // internal/app/app.go — appJobTarget.RunAgentTask
   func (t *appJobTarget) RunAgentTask(ctx context.Context, kind agent.Kind, prompt string) error {
       switch kind {
       case agent.KindOpenCode:
           return t.opencode.SubmitTask(ctx, t.operatorChatID, prompt)
       case agent.KindClaudeCode:
           return t.claudecode.SubmitTask(ctx, t.operatorChatID, prompt)
       case agent.KindChat:
           return t.gateway.SendChat(ctx, t.operatorChatID, prompt)
       case agent.KindNewAgent:
           return t.newagent.SubmitTask(ctx, t.operatorChatID, prompt)
       default:
           return fmt.Errorf("unknown agent kind: %v", kind)
       }
   }
   ```

**What you do NOT need to change:** `scheduler.Service` does not change — it dispatches
via `JobTarget` and knows nothing about concrete agent types. `gateway.Service` does not
change. No database schema changes — `jobs.target` stores the string representation.
