// internal/gateway/service.go
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/agent"
	"github.com/canhta/gistclaw/internal/channel"
	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/hitl"
	"github.com/canhta/gistclaw/internal/infra"
	"github.com/canhta/gistclaw/internal/mcp"
	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/scheduler"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

// ocService abstracts opencode.Service to avoid circular imports in tests.
type ocService interface {
	SubmitTask(ctx context.Context, chatID int64, prompt string) error
	Stop(ctx context.Context) error
	IsAlive(ctx context.Context) bool
}

// ccService abstracts claudecode.Service.
type ccService interface {
	SubmitTask(ctx context.Context, chatID int64, prompt string) error
	Stop(ctx context.Context) error
	IsAlive(ctx context.Context) bool
}

// hitlService is the subset of hitl.Service used by gateway.
// It extends hitl.Approver with Resolve, which handles keyboard callback replies.
type hitlService interface {
	hitl.Approver
	// Resolve delivers a keyboard button press to the waiting HITL handler.
	// id is the permission/question ID; action is one of "once", "always", "reject", "stop".
	Resolve(id string, action string) error
}

// Service is the channel-agnostic gateway controller.
type Service struct {
	ch         channel.Channel
	hitl       hitlService
	opencode   ocService
	claudecode ccService
	llm        providers.LLMProvider
	search     tools.SearchProvider // may be nil
	fetcher    tools.WebFetcher
	mcp        mcp.Manager // interface (not *mcp.Manager)
	sched      *scheduler.Service
	store      *store.Store
	costGuard  *infra.CostGuard  // tracks daily LLM spend; read by buildStatus; may be nil
	soul       *infra.SOULLoader // nil-safe; injected as system prompt in plain chat
	startTime  time.Time         // set in NewService; used by buildStatus for Uptime line
	cfg        config.Config
}

// NewService creates a new gateway Service.
// costGuard is *infra.CostGuard (tracks daily LLM spend); pass nil in unit tests.
// startTime should be time.Now() at the call site (typically app.New in Plan 8).
func NewService(
	ch channel.Channel,
	h hitlService,
	oc ocService,
	cc ccService,
	llm providers.LLMProvider,
	search tools.SearchProvider,
	fetcher tools.WebFetcher,
	m mcp.Manager,
	sched *scheduler.Service,
	st *store.Store,
	costGuard *infra.CostGuard,
	soul *infra.SOULLoader,
	startTime time.Time,
	cfg config.Config,
) *Service {
	return &Service{
		ch:         ch,
		hitl:       h,
		opencode:   oc,
		claudecode: cc,
		llm:        llm,
		search:     search,
		fetcher:    fetcher,
		mcp:        m,
		sched:      sched,
		store:      st,
		costGuard:  costGuard,
		soul:       soul,
		startTime:  startTime,
		cfg:        cfg,
	}
}

// Run starts the gateway message loop. Blocks until ctx is cancelled.
func (s *Service) Run(ctx context.Context) error {
	msgs, err := s.ch.Receive(ctx)
	if err != nil {
		return fmt.Errorf("gateway: Receive: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			s.handle(ctx, msg)
		}
	}
}

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
// Expected format: "hitl:<id>:<action>"
func (s *Service) handleCallback(ctx context.Context, msg channel.InboundMessage) {
	data := msg.CallbackData
	if !strings.HasPrefix(data, "hitl:") {
		log.Warn().Str("callback_data", data).Msg("gateway: unknown callback prefix; ignored")
		return
	}
	// Parse: hitl:<id>:<action>
	parts := strings.SplitN(strings.TrimPrefix(data, "hitl:"), ":", 2)
	if len(parts) != 2 {
		log.Warn().Str("callback_data", data).Msg("gateway: malformed hitl callback; ignored")
		return
	}
	id, action := parts[0], parts[1]
	log.Debug().Str("hitl_id", id).Str("action", action).Msg("gateway: HITL callback received")
	if err := s.hitl.Resolve(id, action); err != nil {
		log.Warn().Str("hitl_id", id).Err(err).Msg("gateway: HITL Resolve failed")
	}
	// suppress unused ctx warning
	_ = ctx
}

// handlePlainChat runs the multi-round LLM tool loop for non-command messages.
func (s *Service) handlePlainChat(ctx context.Context, chatID int64, text string) {
	_ = s.ch.SendTyping(ctx, chatID)

	var msgs []providers.Message
	if s.soul != nil {
		if content, err := s.soul.Load(); err != nil {
			log.Warn().Err(err).Msg("gateway: SOUL load failed; proceeding without system prompt")
		} else if content != "" {
			msgs = append(msgs, providers.Message{Role: "system", Content: content})
		}
	}
	msgs = append(msgs, providers.Message{Role: "user", Content: text})

	toolRegistry := s.buildToolRegistry()

	const doomLoopMax = 3
	type callSig struct{ name, input string }
	lastCalls := make([]callSig, 0, doomLoopMax)

	for iteration := 0; ; iteration++ {
		// MaxIterations hard cap: 0 means unlimited (default for zero-value config in tests).
		if s.cfg.Tuning.MaxIterations > 0 && iteration >= s.cfg.Tuning.MaxIterations {
			log.Warn().Int("iterations", iteration).Msg("gateway: max iterations reached; forcing final answer")
			msgs = append(msgs, providers.Message{
				Role:    "system",
				Content: "[Maximum tool iterations reached. Provide your final answer with what you know so far.]",
			})
			finalResp, ferr := s.llm.Chat(ctx, msgs, nil)
			if ferr != nil {
				_ = s.ch.SendMessage(ctx, chatID, "⚠️ LLM error after max iterations: "+ferr.Error())
				return
			}
			_ = s.ch.SendMessage(ctx, chatID, finalResp.Content)
			return
		}

		resp, err := s.llm.Chat(ctx, msgs, toolRegistry)
		if err != nil {
			log.Error().Err(err).Msg("gateway: LLM Chat error")
			_ = s.ch.SendMessage(ctx, chatID, "⚠️ LLM error: "+err.Error())
			return
		}

		// No tool call → send final answer and exit loop
		if resp.ToolCall == nil {
			_ = s.ch.SendMessage(ctx, chatID, resp.Content)
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
				finalResp, ferr := s.llm.Chat(ctx, msgs, nil)
				if ferr != nil {
					_ = s.ch.SendMessage(ctx, chatID, "⚠️ LLM error after doom-loop guard: "+ferr.Error())
					return
				}
				_ = s.ch.SendMessage(ctx, chatID, finalResp.Content)
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
		toolResult := s.executeToolWithInput(ctx, tc, input)

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

// executeToolWithInput dispatches a tool call given already-unmarshaled input.
func (s *Service) executeToolWithInput(ctx context.Context, tc *providers.ToolCall, input map[string]any) string {
	switch tc.Name {
	case "web_search":
		return s.execWebSearch(ctx, input)
	case "web_fetch":
		return s.execWebFetch(ctx, input)
	case "schedule_job":
		return s.execScheduleJob(input)
	case "list_jobs":
		return s.execListJobs()
	case "update_job":
		return s.execUpdateJob(input)
	case "delete_job":
		return s.execDeleteJob(input)
	default:
		// MCP tool: format is "{server}__{tool}" (double underscore)
		if strings.Contains(tc.Name, "__") {
			return s.execMCPTool(ctx, tc.Name, input)
		}
		return fmt.Sprintf("unknown tool: %q", tc.Name)
	}
}

func (s *Service) execWebSearch(ctx context.Context, input map[string]any) string {
	if s.search == nil {
		return "web_search is not configured (no search API key)"
	}
	query, _ := input["query"].(string)
	count := 5
	if c, ok := input["count"].(float64); ok {
		count = int(c)
	}
	results, err := s.search.Search(ctx, query, count)
	if err != nil {
		return "Search failed: " + err.Error()
	}
	var sb strings.Builder
	for i, r := range results {
		fmt.Fprintf(&sb, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet)
	}
	return sb.String()
}

func (s *Service) execWebFetch(ctx context.Context, input map[string]any) string {
	url, _ := input["url"].(string)
	if url == "" {
		return "web_fetch: 'url' parameter required"
	}
	content, err := s.fetcher.Fetch(ctx, url)
	if err != nil {
		return "web_fetch error: " + err.Error()
	}
	return content
}

func (s *Service) execScheduleJob(input map[string]any) string {
	kindStr, _ := input["kind"].(string)
	targetStr, _ := input["target"].(string)
	prompt, _ := input["prompt"].(string)
	schedule, _ := input["schedule"].(string)

	targetKind, err := agent.KindFromString(targetStr)
	if err != nil {
		return "schedule_job error: invalid target: " + err.Error()
	}

	j := scheduler.Job{
		Kind:     kindStr,
		Target:   targetKind,
		Prompt:   prompt,
		Schedule: schedule,
	}

	if err := s.sched.CreateJob(j); err != nil {
		return "schedule_job error: " + err.Error()
	}
	return `{"status":"ok","message":"Job scheduled successfully."}`
}

func (s *Service) execListJobs() string {
	jobs, err := s.sched.ListJobs()
	if err != nil {
		return "list_jobs error: " + err.Error()
	}
	if len(jobs) == 0 {
		return "[]"
	}
	return scheduler.JobsToJSON(jobs)
}

func (s *Service) execUpdateJob(input map[string]any) string {
	id, _ := input["id"].(string)
	if id == "" {
		return "update_job error: 'id' parameter required"
	}
	fields := make(map[string]any)
	if v, ok := input["enabled"]; ok {
		fields["enabled"] = v
	}
	if err := s.sched.UpdateJob(id, fields); err != nil {
		return "update_job error: " + err.Error()
	}
	return `{"status":"ok"}`
}

func (s *Service) execDeleteJob(input map[string]any) string {
	id, _ := input["id"].(string)
	if id == "" {
		return "delete_job error: 'id' parameter required"
	}
	if err := s.sched.DeleteJob(id); err != nil {
		return "delete_job error: " + err.Error()
	}
	return `{"status":"ok"}`
}

func (s *Service) execMCPTool(ctx context.Context, toolName string, input map[string]any) string {
	result, err := s.mcp.CallTool(ctx, toolName, input)
	if err != nil {
		return "MCP tool error: " + err.Error()
	}
	return result
}

// buildToolRegistry assembles the three-category tool registry.
func (s *Service) buildToolRegistry() []providers.Tool {
	var registry []providers.Tool

	// Core — always present if configured
	if s.search != nil {
		registry = append(registry, webSearchTool())
	}
	registry = append(registry, webFetchTool())

	// Agent — MCP-derived, namespaced {server}__{tool}
	for _, t := range s.mcp.GetAllTools() {
		registry = append(registry, providers.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}

	// System — scheduler control
	registry = append(registry, s.sched.Tools()...)

	return registry
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
	if pending, err := s.store.ListPendingHITL(); err == nil {
		hitlCount = len(pending)
	}
	fmt.Fprintf(&sb, "HITL pending: %d\n", hitlCount)

	// Scheduled jobs
	jobs, _ := s.sched.ListJobs()
	activeCount := 0
	var nextRun time.Time
	for _, j := range jobs {
		if j.Enabled {
			activeCount++
			if nextRun.IsZero() || j.NextRunAt.Before(nextRun) {
				nextRun = j.NextRunAt
			}
		}
	}
	if activeCount > 0 && !nextRun.IsZero() {
		diff := time.Until(nextRun).Round(time.Minute)
		fmt.Fprintf(&sb, "Scheduled jobs: %d active  (next: in %s)\n", activeCount, formatDuration(diff))
	} else {
		fmt.Fprintf(&sb, "Scheduled jobs: %d active\n", activeCount)
	}

	// Daily cost (from infra.CostGuard)
	if s.costGuard != nil {
		currentUSD := s.costGuard.CurrentUSD()
		limitUSD := s.costGuard.LimitUSD()
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

// --- tool definitions ---

func webSearchTool() providers.Tool {
	return providers.Tool{
		Name:        "web_search",
		Description: "Search the web for current information. Returns ranked results with titles, URLs, and snippets.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The search query",
				},
				"count": map[string]any{
					"type":        "integer",
					"description": "Number of results (default 5, max 10)",
					"default":     5,
				},
			},
			"required": []string{"query"},
		},
	}
}

func webFetchTool() providers.Tool {
	return providers.Tool{
		Name:        "web_fetch",
		Description: "Fetch and extract readable content from a URL. Returns markdown text. Truncated at 50KB / 2000 lines.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "The URL to fetch",
				},
			},
			"required": []string{"url"},
		},
	}
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
