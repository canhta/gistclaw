// Package telegram provides a Telegram DM connector using long polling.
// No webhooks are used — only getUpdates with a 30-second timeout.
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	controlconnector "github.com/canhta/gistclaw/internal/connectors/control"
)

// UpdateHandler is called for each dispatched Telegram update.
type UpdateHandler func(context.Context, Update)

// Bot manages the Telegram long-poll loop.
type Bot struct {
	token   string
	handler UpdateHandler
	// apiBase can be overridden in tests to point at a mock server.
	apiBase       string
	client        *http.Client
	commandSpecs  []controlconnector.CommandSpec
	onPollSuccess func(time.Time)
	onPollError   func(time.Time, error)
}

// NewBot creates a Bot. If token is empty, Start returns immediately without polling.
// handler may be nil; if set it is called for each received update.
func NewBot(token string, handler UpdateHandler) *Bot {
	return &Bot{
		token:   token,
		handler: handler,
		apiBase: "https://api.telegram.org/bot",
		client:  &http.Client{Timeout: 40 * time.Second},
	}
}

// Start runs the long-poll loop until ctx is cancelled.
// If no token is configured, Start returns immediately with nil.
func (b *Bot) Start(ctx context.Context) error {
	if b.token == "" {
		return nil
	}
	if err := b.publishCommands(ctx); err != nil {
		log.Printf("telegram: setMyCommands warning: %v", err)
	}

	var offset int64
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		updates, err := b.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if b.onPollError != nil {
				b.onPollError(time.Now().UTC(), err)
			}
			log.Printf("telegram: getUpdates error: %v", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
			continue
		}
		if b.onPollSuccess != nil {
			b.onPollSuccess(time.Now().UTC())
		}

		for _, upd := range updates {
			if upd.UpdateID >= offset {
				offset = upd.UpdateID + 1
			}
			if b.handler != nil {
				b.handler(ctx, upd)
			}
		}
	}
}

func (b *Bot) publishCommands(ctx context.Context) error {
	if len(b.commandSpecs) == 0 {
		return nil
	}

	body, err := json.Marshal(b.commandSpecs)
	if err != nil {
		return fmt.Errorf("telegram: marshal commands: %w", err)
	}

	form := url.Values{}
	form.Set("commands", string(body))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s%s/setMyCommands", b.apiBase, b.token),
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return fmt.Errorf("telegram: build setMyCommands request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: setMyCommands: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("telegram: read setMyCommands body: %w", err)
	}

	var result struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return fmt.Errorf("telegram: parse setMyCommands response: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("telegram: setMyCommands returned ok=false")
	}
	return nil
}

func (b *Bot) getUpdates(ctx context.Context, offset int64) ([]Update, error) {
	url := fmt.Sprintf("%s%s/getUpdates?timeout=30&offset=%d", b.apiBase, b.token, offset)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("telegram: build request: %w", err)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram: getUpdates: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("telegram: read body: %w", err)
	}

	var result struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("telegram: parse response: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram: api returned ok=false")
	}
	return result.Result, nil
}

func (b *Bot) answerCallbackQuery(ctx context.Context, callbackQueryID string) error {
	if strings.TrimSpace(callbackQueryID) == "" {
		return nil
	}

	form := url.Values{}
	form.Set("callback_query_id", callbackQueryID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s%s/answerCallbackQuery", b.apiBase, b.token),
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return fmt.Errorf("telegram: build answerCallbackQuery request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: answerCallbackQuery: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("telegram: read answerCallbackQuery body: %w", err)
	}

	var result struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return fmt.Errorf("telegram: parse answerCallbackQuery response: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("telegram: answerCallbackQuery returned ok=false")
	}
	return nil
}

// ── Telegram types ────────────────────────────────────────────────────────────

// Update is a Telegram update object.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

// Message is a Telegram message object.
type Message struct {
	MessageID int64  `json:"message_id"`
	From      User   `json:"from"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}

// Chat is a Telegram chat object.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"` // "private", "group", "supergroup", "channel"
}

type User struct {
	ID           int64  `json:"id"`
	LanguageCode string `json:"language_code"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}
