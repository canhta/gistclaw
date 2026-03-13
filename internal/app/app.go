// internal/app/app.go
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"

	"github.com/canhta/gistclaw/internal/agent"
	"github.com/canhta/gistclaw/internal/channel"
	tgchan "github.com/canhta/gistclaw/internal/channel/telegram"
	"github.com/canhta/gistclaw/internal/claudecode"
	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/gateway"
	"github.com/canhta/gistclaw/internal/hitl"
	"github.com/canhta/gistclaw/internal/infra"
	"github.com/canhta/gistclaw/internal/mcp"
	"github.com/canhta/gistclaw/internal/opencode"
	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/providers/factory"
	"github.com/canhta/gistclaw/internal/scheduler"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

// App is the composition root. It holds the dependency graph and owns the
// supervision policy in Run.
//
// Fields that do not require network connections at construction time are
// initialised in NewApp. The Telegram channel (which calls GetMe on
// construction) is built inside Run after the fast-exit ctx check, so that
// tests using a fake token can construct an App without making real HTTP calls.
type App struct {
	cfg    config.Config
	store  *store.Store
	soul   *infra.SOULLoader
	rawLLM providers.LLMProvider
	mcp    mcp.Manager
}

// NewApp constructs the App. It opens the SQLite database, validates
// configuration, and builds all services that do not require network
// connections (LLM provider, MCP manager). Network-bound construction
// (Telegram bot GetMe handshake) is deferred to Run.
func NewApp(cfg config.Config) (*App, error) {
	// Open the store.
	s, err := store.Open(cfg.SQLitePath)
	if err != nil {
		return nil, fmt.Errorf("app: open store: %w", err)
	}

	// Purge stale records on startup (non-fatal).
	if err := s.PurgeStartup(
		cfg.Tuning.SessionTTL,
		cfg.Tuning.HITLTimeout*2,
		cfg.Tuning.CostHistoryTTL,
	); err != nil {
		log.Warn().Err(err).Msg("app: PurgeStartup failed (non-fatal)")
	}

	// SOULLoader — lazy file reads; does not fail if SOUL.md is missing.
	soul := infra.NewSOULLoader(cfg.SoulPath)

	// Build LLM provider (validates API key format; no network call).
	rawLLM, err := factory.New(cfg, s)
	if err != nil {
		return nil, fmt.Errorf("app: build LLM provider: %w", err)
	}

	// Build MCP manager (parses config; no network call at construction).
	mcpConfigs, err := mcp.LoadMCPConfig(cfg.MCPConfigPath)
	if err != nil {
		return nil, fmt.Errorf("app: load MCP config: %w", err)
	}
	mcpManager := mcp.NewMCPManager(mcpConfigs, cfg.Tuning)

	return &App{
		cfg:    cfg,
		store:  s,
		soul:   soul,
		rawLLM: rawLLM,
		mcp:    mcpManager,
	}, nil
}

// Run wires the remaining network-bound services and starts all supervised
// goroutines. Returns nil on clean shutdown (ctx cancelled), or a non-nil
// error if the critical gateway service reaches PermanentFailure.
func (a *App) Run(ctx context.Context) error {
	// Fast exit on already-cancelled context.
	if ctx.Err() != nil {
		return nil
	}

	cfg := a.cfg
	s := a.store
	defer func() {
		if err := a.mcp.Close(); err != nil {
			log.Warn().Err(err).Msg("app: mcp close error")
		}
		if err := a.store.Close(); err != nil {
			log.Warn().Err(err).Msg("app: store close error")
		}
	}()

	// --- Build Telegram channel (makes HTTP GetMe call) ---
	tgCh, err := tgchan.NewTelegramChannel(cfg.TelegramToken, s)
	if err != nil {
		return fmt.Errorf("app: build telegram channel: %w", err)
	}
	var ch channel.Channel = tgCh

	// --- Build cost guard with channel as notifier (constructed once, correctly) ---
	costGuard := infra.NewCostGuard(s, cfg.DailyLimitUSD, ch).
		WithOperator(cfg.OperatorChatID())

	// Wrap LLM with cost tracking.
	trackedLLM := providers.NewTrackingProvider(a.rawLLM, costGuard)

	// --- Build tools ---
	fetcher := tools.NewWebFetcher()
	search := tools.NewSearchProvider(cfg) // nil if no search key configured

	// --- Build HITL service ---
	hitlSvc := hitl.NewService(ch, s, cfg.Tuning, nil)

	// --- Build opencode service ---
	ocCfg := opencode.Config{
		Port:           cfg.OpenCodePort,
		Dir:            cfg.OpenCodeDir,
		StartupTimeout: opencode.DefaultStartupTimeout,
	}
	ocAdapter := &costTrackerAdapter{g: costGuard}
	ocSvc := opencode.New(ocCfg, ch, hitlSvc, ocAdapter, a.soul)

	// --- Build claudecode service ---
	ccCfg := claudecode.Config{
		Dir:            cfg.ClaudeDir,
		HookServerAddr: cfg.HookServerAddr,
	}
	ccAdapter := &costTrackerAdapter{g: costGuard}
	ccSvc := claudecode.New(ccCfg, "claude", ch, hitlSvc, ccAdapter, a.soul)

	// --- Build heartbeat ---
	heartbeat := infra.NewHeartbeat(ch, []infra.AgentHealthChecker{ocSvc, ccSvc}, cfg.OperatorChatID())

	// --- Build scheduler ---
	jobTarget := &appJobTarget{oc: ocSvc, cc: ccSvc, ch: ch, cfg: cfg}
	schedSvc := scheduler.NewService(s, jobTarget, cfg.Tuning, cfg.OperatorChatID())

	// --- Build gateway ---
	gatewaySvc := gateway.NewService(
		ch,
		hitlSvc,
		ocSvc,
		ccSvc,
		trackedLLM,
		search,
		fetcher,
		a.mcp,
		schedSvc,
		s,
		costGuard,
		time.Now(),
		cfg,
	)

	logger := log.Logger

	eg, egCtx := errgroup.WithContext(ctx)

	// nonCritical wraps a service with restart supervision. On PermanentFailure
	// it notifies the operator and returns nil so the errgroup is NOT cancelled.
	// ALL goroutines use egCtx so that a gateway PermanentFailure (which cancels
	// egCtx) also stops non-critical goroutines.
	nonCritical := func(name string, maxAttempts int, window time.Duration, svcFn func(context.Context) error) func() error {
		return func() error {
			err := WithRestart(logger, name, maxAttempts, window, svcFn)(egCtx) // ← egCtx, not ctx
			if err != nil {
				// PermanentFailure — log and notify operator; keep running.
				log.Error().Str("service", name).Err(err).Msg("app: non-critical service permanently failed")
				msg := FormatDegradedMsg(name)
				notifyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = ch.SendMessage(notifyCtx, cfg.OperatorChatID(), msg)
				// Drain HITL if hitl service died.
				if name == "hitl" {
					hitlSvc.DrainPending()
				}
				return nil // absorb — do not cancel errgroup
			}
			return nil
		}
	}

	// OpenCode — non-critical; 5 attempts / 30s window
	eg.Go(nonCritical("opencode", 5, 30*time.Second, func(ctx context.Context) error {
		return ocSvc.Run(ctx)
	}))

	// ClaudeCode — non-critical; 5 attempts / 30s window
	eg.Go(nonCritical("claudecode", 5, 30*time.Second, func(ctx context.Context) error {
		return ccSvc.Run(ctx)
	}))

	// HITL — non-critical; 10 attempts / 10s window
	eg.Go(nonCritical("hitl", 10, 10*time.Second, func(ctx context.Context) error {
		return hitlSvc.Run(ctx)
	}))

	// Scheduler — non-critical; 5 attempts / 30s window
	eg.Go(nonCritical("scheduler", 5, 30*time.Second, func(ctx context.Context) error {
		return schedSvc.Run(ctx)
	}))

	// Heartbeat ticker loop — runs until context cancelled.
	eg.Go(func() error {
		ticker := time.NewTicker(cfg.Tuning.HeartbeatTier2Every)
		defer ticker.Stop()
		for {
			select {
			case <-egCtx.Done():
				return nil
			case <-ticker.C:
				heartbeat.CheckAgents(egCtx)
			}
		}
	})

	// Gateway — critical: PermanentFailure propagates to errgroup, cancelling all.
	eg.Go(func() error {
		return WithRestart(logger, "gateway", 0, 0, func(ctx context.Context) error {
			return gatewaySvc.Run(ctx)
		})(egCtx)
	})

	return eg.Wait()
}

// FormatDegradedMsg returns the operator notification message for the named
// non-critical service entering degraded (PermanentFailure) mode.
func FormatDegradedMsg(name string) string {
	switch name {
	case "opencode":
		return "⚠️ OpenCode has permanently failed and is now degraded. /oc commands will not work until restart."
	case "claudecode":
		return "⚠️ ClaudeCode has permanently failed and is now degraded. /cc commands will not work until restart."
	case "hitl":
		return "⚠️ HITL service has permanently failed. Human-in-the-loop approvals will be auto-rejected."
	case "scheduler":
		return "⚠️ Scheduler has permanently failed. Scheduled jobs will not run until restart."
	default:
		return fmt.Sprintf("⚠️ Service %q has permanently failed and is now degraded.", name)
	}
}

// costTrackerAdapter adapts *infra.CostGuard (Track(ctx, usd)) to the narrow
// costTracker interface used by opencode and claudecode (Track(usd)).
type costTrackerAdapter struct {
	g *infra.CostGuard
}

// Track implements the narrow costTracker interface (no ctx, no error return).
// Cost tracking is best-effort — errors from the underlying store are non-fatal
// and silently discarded to avoid disrupting agent task execution. A background
// context is used because there is no request-scoped context available at this
// call site.
func (a *costTrackerAdapter) Track(usd float64) {
	_ = a.g.Track(context.Background(), usd)
}

// appJobTarget implements scheduler.JobTarget by routing to the appropriate
// agent service or chat channel.
type appJobTarget struct {
	oc  opencode.Service
	cc  claudecode.Service
	ch  channel.Channel
	cfg config.Config
}

func (t *appJobTarget) RunAgentTask(ctx context.Context, kind agent.Kind, prompt string) error {
	operatorChatID := t.cfg.OperatorChatID()
	switch kind {
	case agent.KindOpenCode:
		return t.oc.SubmitTask(ctx, operatorChatID, prompt)
	case agent.KindClaudeCode:
		return t.cc.SubmitTask(ctx, operatorChatID, prompt)
	default:
		return fmt.Errorf("app: unknown agent kind %v", kind)
	}
}

func (t *appJobTarget) SendChat(ctx context.Context, chatID int64, text string) error {
	return t.ch.SendMessage(ctx, chatID, text)
}
