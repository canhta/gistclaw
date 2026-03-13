// internal/gateway/router.go
package gateway

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

const helpFallback = "Tèo đây! Tao có thể làm:\n" +
	"/oc <task>  — chạy OpenCode\n" +
	"/cc <task>  — chạy Claude Code\n" +
	"/status     — xem trạng thái\n" +
	"/stop       — dừng agent\n" +
	"Chat thường: hỏi gì cũng được, tao có web search, memory, scheduler."

// handle processes a single inbound message.
func (s *Service) handle(ctx context.Context, msg channel.InboundMessage) {
	// User ID whitelist check
	if !s.isAllowed(msg.ChatID) {
		log.Debug().Int64("chat_id", msg.ChatID).Msg("gateway: message from disallowed user; dropped")
		return
	}

	// HITL callback
	if msg.CallbackData != "" {
		s.handleCallback(ctx, msg)
		return
	}

	// Command routing
	text := strings.TrimSpace(msg.Text)
	switch {
	case text == "/oc":
		_ = s.ch.SendMessage(ctx, msg.ChatID, "Usage: /oc <task> — e.g. /oc build the auth module")
	case text == "/cc":
		_ = s.ch.SendMessage(ctx, msg.ChatID, "Usage: /cc <task> — e.g. /cc refactor the service layer")
	case strings.HasPrefix(text, "/oc "):
		prompt := strings.TrimPrefix(text, "/oc ")
		if err := s.opencode.SubmitTask(ctx, msg.ChatID, prompt); err != nil {
			log.Error().Err(err).Msg("gateway: opencode.SubmitTask")
			_ = s.ch.SendMessage(ctx, msg.ChatID, "⚠️ OpenCode error: "+err.Error())
		}
	case strings.HasPrefix(text, "/cc "):
		prompt := strings.TrimPrefix(text, "/cc ")
		if err := s.claudecode.SubmitTask(ctx, msg.ChatID, prompt); err != nil {
			log.Error().Err(err).Msg("gateway: claudecode.SubmitTask")
			_ = s.ch.SendMessage(ctx, msg.ChatID, "⚠️ ClaudeCode error: "+err.Error())
		}
	case text == "/start", text == "/help":
		s.handleHelp(ctx, msg.ChatID)
	case text == "/stop":
		_ = s.opencode.Stop(ctx)
		_ = s.claudecode.Stop(ctx)
		_ = s.ch.SendMessage(ctx, msg.ChatID, "⏹ Stopped.")
	case text == "/status":
		_ = s.ch.SendMessage(ctx, msg.ChatID, s.buildStatus(ctx))
	default:
		// Plain chat
		s.handlePlainChat(ctx, msg.ChatID, text)
	}
}

// isAllowed checks if the chatID is in the allowed list.
func (s *Service) isAllowed(chatID int64) bool {
	for _, id := range s.cfg.AllowedUserIDs {
		if id == chatID {
			return true
		}
	}
	return false
}

// handleCallback dispatches HITL callback queries.
// All "hitl:" prefixed callbacks are forwarded to hitl.Service via Deliver,
// which routes them to either the permission or question handler inside the
// HITL event loop. This avoids a second ch.Receive() call in hitl.Service
// (which would open a duplicate Telegram long-poll and cause 409 errors).
func (s *Service) handleCallback(ctx context.Context, msg channel.InboundMessage) {
	data := msg.CallbackData
	if !strings.HasPrefix(data, "hitl:") {
		log.Warn().Str("callback_data", data).Msg("gateway: unknown callback prefix; ignored")
		return
	}
	log.Debug().Str("callback_data", data).Msg("gateway: delivering HITL callback")
	s.hitl.Deliver(msg)
	// suppress unused ctx warning
	_ = ctx
}

// buildStatus formats the /status response.
func (s *Service) buildStatus(ctx context.Context) string {
	var sb strings.Builder
	sb.WriteString("GistClaw status (UTC)\n")

	// Uptime
	uptime := time.Since(s.startTime).Round(time.Minute)
	fmt.Fprintf(&sb, "Uptime: %s\n", formatDuration(uptime))

	ocStatus := "idle"
	if s.opencode.IsAlive(ctx) {
		ocStatus = "running"
	}
	fmt.Fprintf(&sb, "OpenCode: %s\n", ocStatus)

	ccStatus := "idle"
	if s.claudecode.IsAlive(ctx) {
		ccStatus = "running"
	}
	fmt.Fprintf(&sb, "ClaudeCode: %s\n", ccStatus)

	// HITL pending count (query live from store)
	hitlCount := 0
	if pending, err := s.st.ListPendingHITL(); err == nil {
		hitlCount = len(pending)
	}
	fmt.Fprintf(&sb, "HITL pending: %d\n", hitlCount)

	// Scheduled jobs — summary line + one row per job
	jobs, _ := s.sched.ListJobs()
	activeCount := 0
	disabledCount := 0
	for _, j := range jobs {
		if j.Enabled {
			activeCount++
		} else {
			disabledCount++
		}
	}
	if disabledCount > 0 {
		fmt.Fprintf(&sb, "Scheduled jobs: %d active, %d disabled\n", activeCount, disabledCount)
	} else {
		fmt.Fprintf(&sb, "Scheduled jobs: %d active\n", activeCount)
	}

	// Sort: enabled first (by next_run_at ascending), disabled last.
	sort.Slice(jobs, func(i, k int) bool {
		if jobs[i].Enabled != jobs[k].Enabled {
			return jobs[i].Enabled // enabled before disabled
		}
		return jobs[i].NextRunAt.Before(jobs[k].NextRunAt)
	})

	for _, j := range jobs {
		idPrefix := j.ID
		if len(idPrefix) > 8 {
			idPrefix = idPrefix[:8]
		}
		prompt := j.Prompt
		if len([]rune(prompt)) > 40 {
			runes := []rune(prompt)
			prompt = string(runes[:40]) + "…"
		}
		enabled := "✓"
		if !j.Enabled {
			enabled = "✗"
		}
		var when string
		if !j.Enabled {
			when = "disabled"
		} else {
			diff := time.Until(j.NextRunAt).Round(time.Minute)
			if diff < 0 {
				when = "overdue"
			} else {
				when = "next in " + formatDuration(diff)
			}
		}
		fmt.Fprintf(&sb, "  [%s] %s %s → %s: %q %s %s\n",
			idPrefix, j.Kind, j.Schedule, j.Target.String(), prompt, enabled, when)
	}

	// Daily cost (from infra.CostGuard)
	if s.guard != nil {
		currentUSD := s.guard.CurrentUSD()
		limitUSD := s.guard.LimitUSD()
		pct := 0.0
		if limitUSD > 0 {
			pct = currentUSD / limitUSD * 100
		}
		fmt.Fprintf(&sb, "Daily cost: $%.2f / $%.2f (%.0f%%)\n", currentUSD, limitUSD, pct)
	}

	// MCP servers — sorted for deterministic output
	servers := s.mcp.ServerStatus()
	if len(servers) > 0 {
		names := make([]string, 0, len(servers))
		for name := range servers {
			names = append(names, name)
		}
		sort.Strings(names)
		sb.WriteString("MCP servers:")
		for _, name := range names {
			if servers[name] {
				fmt.Fprintf(&sb, " %s ✓", name)
			} else {
				fmt.Fprintf(&sb, " %s ✗ (failed)", name)
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatDuration formats a duration in a human-readable "Xh Ym" or "Ym" style.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

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
