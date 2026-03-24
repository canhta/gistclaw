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
	"time"
)

// UpdateHandler is called for each dispatched Telegram update.
type UpdateHandler func(Update)

// Bot manages the Telegram long-poll loop.
type Bot struct {
	token   string
	handler UpdateHandler
	// apiBase can be overridden in tests to point at a mock server.
	apiBase string
	client  *http.Client
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
			log.Printf("telegram: getUpdates error: %v", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
			continue
		}

		for _, upd := range updates {
			if upd.UpdateID >= offset {
				offset = upd.UpdateID + 1
			}
			if b.handler != nil {
				b.handler(upd)
			}
		}
	}
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

// ── Telegram types ────────────────────────────────────────────────────────────

// Update is a Telegram update object.
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

// Message is a Telegram message object.
type Message struct {
	MessageID int64  `json:"message_id"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}

// Chat is a Telegram chat object.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"` // "private", "group", "supergroup", "channel"
}
