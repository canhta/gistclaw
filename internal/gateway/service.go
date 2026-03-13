// internal/gateway/service.go
package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/channel"
	"github.com/canhta/gistclaw/internal/config"
	convpkg "github.com/canhta/gistclaw/internal/conversation"
	"github.com/canhta/gistclaw/internal/hitl"
	"github.com/canhta/gistclaw/internal/infra"
	"github.com/canhta/gistclaw/internal/mcp"
	mempkg "github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/scheduler"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

// ocService abstracts opencode.Service to avoid circular imports in tests.
type ocService interface {
	SubmitTask(ctx context.Context, chatID int64, prompt string) error
	SubmitTaskWithResult(ctx context.Context, chatID int64, prompt string) (string, error)
	Stop(ctx context.Context) error
	IsAlive(ctx context.Context) bool
}

// ccService abstracts claudecode.Service.
type ccService interface {
	SubmitTask(ctx context.Context, chatID int64, prompt string) error
	SubmitTaskWithResult(ctx context.Context, chatID int64, prompt string) (string, error)
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
	ch          channel.Channel
	hitl        hitlService
	opencode    ocService
	claudecode  ccService
	llm         providers.LLMProvider
	search      tools.SearchProvider // may be nil
	fetcher     tools.WebFetcher
	mcp         mcp.Manager // interface (not *mcp.Manager)
	sched       *scheduler.Service
	st          *store.Store
	guard       *infra.CostGuard // tracks daily LLM spend; read by buildStatus; may be nil
	memory      mempkg.Engine    // nil-safe; provides LoadContext, AppendFact, AppendNote
	conv        convpkg.Manager  // conversation history manager
	startTime   time.Time        // set in NewService; used by buildStatus for Uptime line
	cfg         config.Config
	lifetimeCtx context.Context //nolint:containedctx
}

// NewService creates a new gateway Service.
// guard is *infra.CostGuard (tracks daily LLM spend); pass nil in unit tests.
// startTime should be time.Now() at the call site.
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
	guard *infra.CostGuard,
	mem mempkg.Engine,
	conv convpkg.Manager,
	startTime time.Time,
	cfg config.Config,
) *Service {
	return &Service{
		ch:          ch,
		hitl:        h,
		opencode:    oc,
		claudecode:  cc,
		llm:         llm,
		search:      search,
		fetcher:     fetcher,
		mcp:         m,
		sched:       sched,
		st:          st,
		guard:       guard,
		memory:      mem,
		conv:        conv,
		startTime:   startTime,
		cfg:         cfg,
		lifetimeCtx: context.Background(),
	}
}

// Run starts the gateway message loop. Blocks until ctx is cancelled.
func (s *Service) Run(ctx context.Context) error {
	s.lifetimeCtx = ctx

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

// sendFinal persists the assistant response to history and sends it to the user.
// Used by all successful exit paths in handlePlainChat so saves are never missed.
func (s *Service) sendFinal(ctx context.Context, chatID int64, content string) {
	if s.conv != nil {
		if err := s.conv.Save(chatID, "assistant", content); err != nil {
			log.Warn().Err(err).Msg("gateway: failed to save assistant message")
		}
	}
	_ = s.ch.SendMessage(ctx, chatID, content)
}
