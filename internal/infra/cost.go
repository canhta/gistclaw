// internal/infra/cost.go
package infra

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/store"
)

// Notifier is a minimal interface for sending text messages.
// Implemented by channel.Channel (or a thin adapter); kept as a local interface
// to avoid an import cycle with internal/channel.
type Notifier interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}

// CostGuard tracks daily LLM spend and sends soft-stop notifications at 80% and 100%.
// It is safe for concurrent use.
//
// Concurrency design:
//   - cents (atomic.Int64) is incremented lock-free; CurrentUSD() reads it without a lock.
//   - mu guards only the mutable state that cannot be atomic: today string, notified flags,
//     and the SQLite write (which must use the post-increment total).
type CostGuard struct {
	cents       atomic.Int64 // spend in integer micro-dollars (1e6 per USD); lock-free
	mu          sync.Mutex   // guards today, notified80, notified100, and SQLite write
	limitUSD    float64
	store       *store.Store
	notifier    Notifier // may be nil in tests
	operatorID  int64    // chat ID to notify; 0 = no notification
	today       string   // current date string "YYYY-MM-DD"; reset daily
	notified80  bool
	notified100 bool
}

// NewCostGuard creates a CostGuard. notifier may be nil (no Telegram notifications sent).
// operatorChatID is the chat ID for supervisor notifications.
func NewCostGuard(s *store.Store, limitUSD float64, notifier Notifier) *CostGuard {
	return &CostGuard{
		store:    s,
		limitUSD: limitUSD,
		notifier: notifier,
		today:    todayUTC(),
	}
}

// WithOperator sets the operator chat ID for notifications. Returns g for chaining.
func (g *CostGuard) WithOperator(chatID int64) *CostGuard {
	g.operatorID = chatID
	return g
}

// Track adds usd to the daily spend and triggers notifications if thresholds are crossed.
// A zero usd value is a valid no-op (providers with opaque billing).
func (g *CostGuard) Track(ctx context.Context, usd float64) error {
	if usd == 0 {
		return nil
	}

	// Increment the atomic counter lock-free first.
	microDollars := int64(math.Round(usd * 1e6))
	newTotal := g.cents.Add(microDollars)
	totalUSD := float64(newTotal) / 1e6

	// Lock only for: daily reset check, notification flag check, and SQLite write.
	g.mu.Lock()
	defer g.mu.Unlock()

	// Daily reset: if the date has changed, reset counter and notification flags.
	if today := todayUTC(); today != g.today {
		g.today = today
		g.cents.Store(0)
		newTotal = 0
		totalUSD = 0
		g.notified80 = false
		g.notified100 = false
	}

	// Persist to SQLite (uses totalUSD computed after any reset).
	if err := g.store.UpsertCostDaily(g.today, totalUSD); err != nil {
		log.Error().Err(err).Msg("infra/cost: failed to persist daily cost")
	}

	// Threshold notifications (send once per day per threshold).
	if g.notifier != nil && g.operatorID != 0 {
		pct := totalUSD / g.limitUSD * 100
		if pct >= 100 && !g.notified100 {
			g.notified100 = true
			msg := fmt.Sprintf("⚠️ Daily limit reached ($%.2f / $%.2f). Current session will finish cleanly.", totalUSD, g.limitUSD)
			go g.notifier.SendMessage(ctx, g.operatorID, msg) //nolint:errcheck
		} else if pct >= 80 && !g.notified80 {
			g.notified80 = true
			msg := fmt.Sprintf("⚠️ 80%% of daily cost used ($%.2f / $%.2f).", totalUSD, g.limitUSD)
			go g.notifier.SendMessage(ctx, g.operatorID, msg) //nolint:errcheck
		}
	}

	return nil
}

// CurrentUSD returns the current daily spend in USD.
// Reads the atomic counter lock-free — safe for concurrent calls.
func (g *CostGuard) CurrentUSD() float64 {
	return float64(g.cents.Load()) / 1e6
}

// LimitUSD returns the configured daily limit.
func (g *CostGuard) LimitUSD() float64 { return g.limitUSD }

func todayUTC() string {
	return time.Now().UTC().Format("2006-01-02")
}
