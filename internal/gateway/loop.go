// internal/gateway/loop.go
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/providers"
	toolpkg "github.com/canhta/gistclaw/internal/tools"
)

// handlePlainChat runs the multi-round LLM tool loop for non-command messages.
func (s *Service) handlePlainChat(ctx context.Context, chatID int64, text string) {
	_ = s.ch.SendTyping(ctx, chatID)

	var msgs []providers.Message

	// 1. Composed system prompt: SOUL + MEMORY.md + today's notes (all managed by memory.Engine).
	//    Combined into one system message so the LLM sees a single coherent prompt.
	if s.memory != nil {
		if sysContent := s.memory.LoadContext(); sysContent != "" {
			msgs = append(msgs, providers.Message{Role: "system", Content: sysContent})
		}
	}

	// 1b. Current UTC time — injected so the LLM can compute correct timestamps
	//     for scheduled jobs and any other time-aware reasoning.
	msgs = append(msgs, providers.Message{
		Role:    "system",
		Content: "Current UTC time: " + time.Now().UTC().Format(time.RFC3339),
	})

	// 2. Conversation history — run optional summarization, then load history.
	if s.conv != nil {
		if err := s.conv.MaybeSummarize(ctx, chatID, s.llm); err != nil {
			log.Warn().Err(err).Msg("gateway: conversation summarize failed")
		}
		history, err := s.conv.Load(chatID)
		if err != nil {
			log.Warn().Err(err).Msg("gateway: failed to load conversation history")
		} else {
			msgs = append(msgs, history...)
		}
	}

	// 3. Current user message.
	msgs = append(msgs, providers.Message{Role: "user", Content: text})

	// Persist user message. Non-fatal — history is best-effort.
	if s.conv != nil {
		if err := s.conv.Save(chatID, "user", text); err != nil {
			log.Warn().Err(err).Msg("gateway: failed to save user message")
		}
	}

	engine := s.buildToolEngine()
	toolRegistry := engine.Definitions()

	const doomLoopMax = 3
	type callSig struct{ name, input string }
	lastCalls := make([]callSig, 0, doomLoopMax)

	var assistantReply string

	for iteration := 0; ; iteration++ {
		// MaxIterations hard cap: 0 means unlimited (default for zero-value config in tests).
		if s.cfg.Tuning.MaxIterations > 0 && iteration >= s.cfg.Tuning.MaxIterations {
			log.Warn().Int("iterations", iteration).Msg("gateway: max iterations reached; forcing final answer")
			msgs = append(msgs, providers.Message{
				Role:    "system",
				Content: "[Maximum tool iterations reached. Provide your final answer with what you know so far.]",
			})
			finalResp, ferr := s.chatWithRetry(ctx, chatID, &msgs, nil)
			if ferr != nil {
				_ = s.ch.SendMessage(ctx, chatID, "⚠️ LLM error after max iterations: "+ferr.Error())
				return
			}
			assistantReply = finalResp.Content
			s.sendFinal(ctx, chatID, assistantReply)
			s.autoCurateMemory(text, assistantReply)
			return
		}

		resp, err := s.chatWithRetry(ctx, chatID, &msgs, toolRegistry)
		if err != nil {
			log.Error().Err(err).Msg("gateway: LLM Chat error")
			_ = s.ch.SendMessage(ctx, chatID, "⚠️ LLM error: "+err.Error())
			return
		}

		// No tool call → send final answer and exit loop
		if resp.ToolCall == nil {
			assistantReply = resp.Content
			s.sendFinal(ctx, chatID, assistantReply)
			s.autoCurateMemory(text, assistantReply)
			return
		}

		tc := resp.ToolCall

		// Doom-loop guard: track consecutive identical tool calls using InputJSON directly
		sig := callSig{name: tc.Name, input: tc.InputJSON}
		if len(lastCalls) >= doomLoopMax {
			lastCalls = lastCalls[1:]
		}
		lastCalls = append(lastCalls, sig)

		if len(lastCalls) == doomLoopMax {
			identical := true
			for _, c := range lastCalls[1:] {
				if c != lastCalls[0] {
					identical = false
					break
				}
			}
			if identical {
				log.Warn().Str("tool", tc.Name).Msg("gateway: doom-loop detected; forcing final answer")
				// Inject guard message and call LLM one final time without tools
				msgs = append(msgs, providers.Message{
					Role:       "tool",
					Content:    "[Tool call loop detected. Provide your best answer now.]",
					ToolCallID: tc.ID,
				})
				finalResp, ferr := s.chatWithRetry(ctx, chatID, &msgs, nil)
				if ferr != nil {
					_ = s.ch.SendMessage(ctx, chatID, "⚠️ LLM error after doom-loop guard: "+ferr.Error())
					return
				}
				assistantReply = finalResp.Content
				s.sendFinal(ctx, chatID, assistantReply)
				s.autoCurateMemory(text, assistantReply)
				return
			}
		}

		// Execute tool call — unmarshal InputJSON into map[string]any first
		var input map[string]any
		if tc.InputJSON != "" {
			if err := json.Unmarshal([]byte(tc.InputJSON), &input); err != nil {
				log.Warn().Str("tool", tc.Name).Err(err).Msg("gateway: failed to unmarshal tool InputJSON")
			}
		}
		toolResult := engine.Execute(ctx, tc.Name, input).ForLLM

		// Append assistant message representing the tool call, then tool result
		msgs = append(msgs,
			providers.Message{
				Role:       "assistant",
				Content:    tc.InputJSON, // store the raw tool call input as content
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
			},
			providers.Message{
				Role:       "tool",
				Content:    truncateToolResult(toolResult),
				ToolCallID: tc.ID,
			},
		)
	}
}

// chatWithRetry calls the LLM with retry logic for transient errors.
//
// Four behaviours based on ClassifyError:
//   - ErrKindContextWindow: compress history (drop oldest floor(N/2) dyads, or
//     fall back to dropping oldest plain conversation turns), then retry
//     exactly once. Orthogonal to the maxAttempts loop.
//   - ErrKindRetryable (5xx, timeout): up to maxAttempts retries with exponential backoff.
//   - ErrKindRateLimit (429): same backoff, plus a one-time user notification.
//   - ErrKindTerminal (4xx, format errors): fail immediately, no retry.
//
// msgs is a pointer so compressMessages can modify the slice in-place; all three
// call sites in handlePlainChat pass &msgs.
func (s *Service) chatWithRetry(ctx context.Context, chatID int64, msgs *[]providers.Message, tools []providers.Tool) (*providers.LLMResponse, error) {
	maxAttempts := s.cfg.Tuning.LLMRetryAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	delay := s.cfg.Tuning.LLMRetryDelay
	if delay <= 0 {
		delay = time.Second
	}

	rateLimitNotified := false
	compressedOnce := false
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		resp, err := s.llm.Chat(ctx, *msgs, tools)
		if err == nil {
			return resp, nil
		}

		switch providers.ClassifyError(err) {
		case providers.ErrKindContextWindow:
			if compressedOnce {
				return nil, err
			}
			before := len(*msgs)
			if !compressMessages(msgs) {
				return nil, err
			}
			compressedOnce = true
			after := len(*msgs)
			log.Warn().
				Int("before", before).
				Int("after", after).
				Msg("gateway: context window exceeded; compressed history, retrying once")
			resp2, err2 := s.llm.Chat(ctx, *msgs, tools)
			if err2 == nil {
				return resp2, nil
			}
			err = err2
			switch providers.ClassifyError(err) {
			case providers.ErrKindContextWindow:
				return nil, err
			case providers.ErrKindTerminal:
				return nil, err
			case providers.ErrKindRateLimit:
				if !rateLimitNotified {
					log.Warn().Err(err).Int("attempt", attempt+1).Msg("gateway: LLM rate limited; retrying")
					notifyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_ = s.ch.SendMessage(notifyCtx, chatID, "⚠️ LLM rate limited; retrying…")
					cancel()
					rateLimitNotified = true
				}
				fallthrough
			case providers.ErrKindRetryable:
				log.Warn().Err(err).Int("attempt", attempt+1).Dur("backoff", delay).Msg("gateway: transient LLM error; retrying")
				lastErr = err
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}
				delay *= 2
			}

		case providers.ErrKindTerminal:
			return nil, err

		case providers.ErrKindRateLimit:
			if !rateLimitNotified {
				log.Warn().Err(err).Int("attempt", attempt+1).Msg("gateway: LLM rate limited; retrying")
				notifyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = s.ch.SendMessage(notifyCtx, chatID, "⚠️ LLM rate limited; retrying…")
				cancel()
				rateLimitNotified = true
			}
			fallthrough

		case providers.ErrKindRetryable:
			log.Warn().Err(err).Int("attempt", attempt+1).Dur("backoff", delay).Msg("gateway: transient LLM error; retrying")
			lastErr = err
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
		}
	}

	return nil, lastErr
}

// buildToolEngine constructs the ToolEngine with all registered tools for this request.
func (s *Service) buildToolEngine() *toolpkg.ToolEngine {
	e := toolpkg.NewToolEngine()

	// Web tools.
	e.Register(toolpkg.NewWebSearchTool(s.search))
	e.Register(toolpkg.NewWebFetchTool(s.fetcher))

	// Memory tools.
	if s.memory != nil {
		e.Register(toolpkg.NewRememberTool(s.memory))
		e.Register(toolpkg.NewNoteTool(s.memory))
		e.Register(toolpkg.NewCurateMemoryTool(s.memory, s.llm))
	}

	// Scheduler tools.
	for _, t := range toolpkg.NewSchedulerTools(s.sched) {
		e.Register(t)
	}

	// MCP tools (dynamic, discovered at build time).
	for _, t := range toolpkg.NewMCPTools(s.mcp) {
		e.Register(t)
	}

	// Agent orchestration tools.
	e.Register(toolpkg.NewSpawnAgentTool(s.opencode, s.claudecode, s.cfg.OperatorChatID(), s.lifetimeCtx))
	e.Register(toolpkg.NewRunParallelTool(s.opencode, s.claudecode, s.cfg.OperatorChatID(), s.lifetimeCtx))
	e.Register(toolpkg.NewChainAgentsTool(s.opencode, s.claudecode, s.cfg.OperatorChatID()))

	return e
}

// autoCurateMemory asynchronously evaluates whether anything from the exchange should be remembered.
func (s *Service) autoCurateMemory(userText, reply string) {
	if s.memory == nil {
		return
	}
	go func(userText, reply string) {
		bgCtx, cancel := context.WithTimeout(s.lifetimeCtx, 10*time.Second)
		defer cancel()
		prompt := fmt.Sprintf(
			"Given this exchange, should anything be remembered long-term?\nUser: %s\nAssistant: %s\n"+
				"Reply ONLY with JSON: {\"remember\":false} or {\"remember\":true,\"kind\":\"fact|note\",\"content\":\"...\"}",
			userText, reply)
		resp, err := s.llm.Chat(bgCtx, []providers.Message{{Role: "user", Content: prompt}}, nil)
		if err != nil {
			return
		}
		if resp == nil || resp.Content == "" {
			return
		}
		var result struct {
			Remember bool   `json:"remember"`
			Kind     string `json:"kind"`
			Content  string `json:"content"`
		}
		if json.Unmarshal([]byte(resp.Content), &result) == nil && result.Remember {
			switch result.Kind {
			case "fact":
				_ = s.memory.AppendFact(result.Content)
			case "note":
				_ = s.memory.AppendNote(result.Content)
			}
		}
	}(userText, reply)
}

// --- helpers ---

// truncateToolResult truncates a tool result to 50KB / 2000 lines, whichever is smaller.
func truncateToolResult(s string) string {
	const maxBytes = 50 * 1024
	const maxLines = 2000

	lines := strings.SplitN(s, "\n", maxLines+1)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		s = strings.Join(lines, "\n") + "\n[truncated: exceeded 2000 lines]"
	}

	if len(s) > maxBytes {
		s = s[:maxBytes] + "\n[truncated: exceeded 50KB]"
	}

	return s
}
