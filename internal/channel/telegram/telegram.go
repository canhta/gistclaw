// internal/channel/telegram/telegram.go
package telegram

import (
	"context"
	"errors"
	"fmt"
	"time"

	telego "github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoapi"
	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/channel"
	"github.com/canhta/gistclaw/internal/store"
)

const longPollTimeout = 30 // seconds passed to Telegram getUpdates

// TelegramChannel implements channel.Channel using Telegram Bot API long-polling.
// It deduplicates updates via store.GetLastUpdateID / store.SetLastUpdateID keyed
// by "telegram:<botUsername>".
type TelegramChannel struct {
	bot      *telego.Bot
	store    *store.Store
	stateKey string // "telegram:<botUsername>"
}

// NewTelegramChannel creates a TelegramChannel that connects to the real Telegram API.
// Returns an error if the token is rejected by Telegram on the getMe call.
func NewTelegramChannel(token string, s *store.Store) (*TelegramChannel, error) {
	return newWithOptions(token, s, nil)
}

// NewTelegramChannelWithBaseURL creates a TelegramChannel that connects to baseURL
// instead of https://api.telegram.org. Used in tests with httptest.Server.
func NewTelegramChannelWithBaseURL(token string, s *store.Store, baseURL string) (*TelegramChannel, error) {
	return newWithOptions(token, s, &baseURL)
}

func newWithOptions(token string, s *store.Store, baseURL *string) (*TelegramChannel, error) {
	opts := []telego.BotOption{}
	if baseURL != nil {
		opts = append(opts, telego.WithAPIServer(*baseURL))
	}

	bot, err := telego.NewBot(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("telegram: create bot: %w", err)
	}

	me, err := bot.GetMe(context.Background())
	if err != nil {
		return nil, fmt.Errorf("telegram: getMe: %w", err)
	}

	stateKey := fmt.Sprintf("telegram:%s", me.Username)
	return &TelegramChannel{
		bot:      bot,
		store:    s,
		stateKey: stateKey,
	}, nil
}

// Name returns the platform identifier.
func (t *TelegramChannel) Name() string { return "telegram" }

// Receive starts long-polling and returns a channel of inbound messages.
// Runs until ctx is cancelled. The returned channel is closed on exit.
// Duplicate update IDs (tracked in SQLite) are silently dropped.
func (t *TelegramChannel) Receive(ctx context.Context) (<-chan channel.InboundMessage, error) {
	out := make(chan channel.InboundMessage, 64)

	go func() {
		defer close(out)
		var offset int64

		// Load the last seen update ID from SQLite to resume correctly after restart.
		// lastPersisted tracks the highest update ID written to SQLite; used for dedup
		// without a per-update SQLite read in the hot loop.
		var lastPersisted int64
		if last, err := t.store.GetLastUpdateID(t.stateKey); err == nil && last > 0 {
			offset = last + 1
			lastPersisted = last
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			updates, err := t.bot.GetUpdates(ctx, &telego.GetUpdatesParams{
				Offset:  int(offset),
				Timeout: longPollTimeout,
			})
			if err != nil {
				if ctx.Err() != nil {
					return // context cancelled during poll — clean exit
				}
				log.Warn().Err(err).Str("channel", t.stateKey).Msg("telegram: GetUpdates error; retrying in 1s")
				select {
				case <-time.After(time.Second):
				case <-ctx.Done():
					return
				}
				continue
			}

			for _, u := range updates {
				updateID := int64(u.UpdateID)
				offset = updateID + 1

				// Dedup: skip updates we've already persisted (handles replay on restart).
				if updateID <= lastPersisted {
					continue
				}
				lastPersisted = updateID
				if err := t.store.SetLastUpdateID(t.stateKey, updateID); err != nil {
					log.Warn().Err(err).Msg("telegram: failed to persist update ID")
				}

				msg := extractMessage(u)
				if msg == nil {
					continue
				}

				select {
				case out <- *msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

// extractMessage converts a telego.Update into a channel.InboundMessage.
// Returns nil if the update carries no relevant content.
func extractMessage(u telego.Update) *channel.InboundMessage {
	if cb := u.CallbackQuery; cb != nil {
		chatID := int64(0)
		if cb.Message != nil {
			chatID = cb.Message.GetChat().ID
		}
		// cb.From is a value type (User), not a pointer; always present for callback queries.
		userID := cb.From.ID
		return &channel.InboundMessage{
			ID:           fmt.Sprintf("%d", u.UpdateID),
			ChatID:       chatID,
			UserID:       userID,
			CallbackData: cb.Data,
		}
	}
	if m := u.Message; m != nil {
		var userID int64
		if m.From != nil { // m.From is nil for channel posts and some forwarded messages
			userID = m.From.ID
		}
		return &channel.InboundMessage{
			ID:     fmt.Sprintf("%d", u.UpdateID),
			ChatID: m.Chat.ID,
			UserID: userID,
			Text:   m.Text,
		}
	}
	return nil
}

// SendMessage sends text to chatID. Long messages are split at line boundaries
// with code-block fence healing; see SplitMessage for details.
func (t *TelegramChannel) SendMessage(ctx context.Context, chatID int64, text string) error {
	chunks := SplitMessage(text, telegramLimit)
	for _, chunk := range chunks {
		if err := t.sendWithRetry(ctx, func() error {
			_, err := t.bot.SendMessage(ctx, &telego.SendMessageParams{
				ChatID: telego.ChatID{ID: chatID},
				Text:   chunk,
			})
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

// SendKeyboard sends a message with an inline keyboard.
// Translates channel.KeyboardPayload → telego.InlineKeyboardMarkup.
// Each channel.ButtonRow becomes one row of inline buttons.
func (t *TelegramChannel) SendKeyboard(ctx context.Context, chatID int64, payload channel.KeyboardPayload) error {
	markup := buildInlineKeyboard(payload)
	return t.sendWithRetry(ctx, func() error {
		_, err := t.bot.SendMessage(ctx, &telego.SendMessageParams{
			ChatID:      telego.ChatID{ID: chatID},
			Text:        payload.Text,
			ReplyMarkup: markup,
		})
		return err
	})
}

// Ping calls GetMe to verify that the Telegram API is reachable.
// Implements infra.Pinger.
func (t *TelegramChannel) Ping(ctx context.Context) error {
	_, err := t.bot.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("telegram: ping (getMe): %w", err)
	}
	return nil
}

// SendTyping sends a "typing" chat action to chatID.
func (t *TelegramChannel) SendTyping(ctx context.Context, chatID int64) error {
	return t.sendWithRetry(ctx, func() error {
		return t.bot.SendChatAction(ctx, &telego.SendChatActionParams{
			ChatID: telego.ChatID{ID: chatID},
			Action: telego.ChatActionTyping,
		})
	})
}

// buildInlineKeyboard translates a channel.KeyboardPayload into a telego inline keyboard.
func buildInlineKeyboard(payload channel.KeyboardPayload) *telego.InlineKeyboardMarkup {
	rows := make([][]telego.InlineKeyboardButton, len(payload.Rows))
	for i, row := range payload.Rows {
		buttons := make([]telego.InlineKeyboardButton, len(row))
		for j, btn := range row {
			buttons[j] = telego.InlineKeyboardButton{
				Text:         btn.Label,
				CallbackData: btn.CallbackData,
			}
		}
		rows[i] = buttons
	}
	return &telego.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// sendWithRetry executes fn with Telegram-specific retry logic:
//   - 429 Too Many Requests: read RetryAfter, sleep, retry (unlimited).
//   - 5xx server errors: 3 retries at 500ms / 1s / 2s.
//   - Network errors (non-*telegoapi.Error): treated same as 5xx — 3 retries at 500ms / 1s / 2s.
//   - 403 Forbidden (blocked): log WARN, return nil (no retry, no error).
func (t *TelegramChannel) sendWithRetry(ctx context.Context, fn func() error) error {
	const maxRetries = 3
	delays := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second}

	for attempt := 0; ; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Try to extract telegoapi.Error details via errors.As (telego wraps the error).
		var tErr *telegoapi.Error
		if errors.As(err, &tErr) {
			switch {
			case tErr.ErrorCode == 403:
				// Bot was blocked by user — log and silently drop.
				log.Warn().Int("code", tErr.ErrorCode).Msg("telegram: bot blocked by user; dropping message")
				return nil
			case tErr.ErrorCode == 429:
				// Rate limit — honour RetryAfter.
				var retryAfter time.Duration
				if tErr.Parameters != nil && tErr.Parameters.RetryAfter > 0 {
					retryAfter = time.Duration(tErr.Parameters.RetryAfter) * time.Second
				} else {
					retryAfter = 5 * time.Second
				}
				log.Warn().Dur("retry_after", retryAfter).Msg("telegram: rate limited (429); waiting")
				select {
				case <-time.After(retryAfter):
				case <-ctx.Done():
					return ctx.Err()
				}
				continue // retry without consuming the attempt counter
			case tErr.ErrorCode >= 500:
				if attempt >= maxRetries {
					return fmt.Errorf("telegram: 5xx after %d retries: %w", maxRetries, err)
				}
				delay := delays[attempt]
				log.Warn().Int("attempt", attempt+1).Dur("delay", delay).Msg("telegram: 5xx; retrying")
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return ctx.Err()
				}
				continue
			}
			// Other telego errors (e.g. 4xx client errors except 403/429) are non-retriable.
			return err
		} else {
			// Network / transport error — treat same as 5xx per §9.21.
			if attempt >= maxRetries {
				return fmt.Errorf("telegram: network error after %d retries: %w", maxRetries, err)
			}
			delay := delays[attempt]
			log.Warn().Int("attempt", attempt+1).Dur("delay", delay).Err(err).Msg("telegram: network error; retrying")
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}
	}
}
