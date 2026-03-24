package telegram

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

// outboundAllowedKinds is the set of event kinds that trigger an outbound message.
var outboundAllowedKinds = map[string]bool{
	"run_started":     true,
	"run_blocked":     true,
	"run_completed":   true,
	"run_interrupted": true,
	"approval_needed": true,
}

// OutboundDispatcher writes outbound_intents and delivers them via sendMessage.
// It implements model.Connector for the Telegram channel.
type OutboundDispatcher struct {
	connectorID string
	bot         *Bot
	db          *store.DB
	cs          *conversations.ConversationStore
	maxAttempts int
	retryDelay  time.Duration
}

// ID returns the connector identifier stored in outbound_intents.connector_id.
func (d *OutboundDispatcher) ID() string { return d.connectorID }

// Start runs the bot's long-poll loop until ctx is cancelled.
func (d *OutboundDispatcher) Start(ctx context.Context) error { return d.bot.Start(ctx) }

// NewOutboundDispatcher creates a dispatcher. token is the Telegram bot token.
func NewOutboundDispatcher(token string, db *store.DB, cs *conversations.ConversationStore) *OutboundDispatcher {
	return &OutboundDispatcher{
		connectorID: "telegram",
		bot:         NewBot(token, nil),
		db:          db,
		cs:          cs,
		maxAttempts: 5,
		retryDelay:  2 * time.Second,
	}
}

// Notify records an outbound intent for an event and attempts immediate delivery.
// The dedupeKey prevents re-delivery if the same key is seen again.
// If the event kind is not in the allowed set, Notify is a no-op.
func (d *OutboundDispatcher) Notify(ctx context.Context, chatID string, delta model.ReplayDelta, dedupeKey string) error {
	if !outboundAllowedKinds[delta.Kind] {
		return nil
	}

	// Check dedupe.
	var existing int
	_ = d.db.RawDB().QueryRowContext(ctx,
		"SELECT count(*) FROM outbound_intents WHERE dedupe_key = ?", dedupeKey,
	).Scan(&existing)
	if existing > 0 {
		return nil
	}

	text := buildMessage(delta)
	intentID := generateIntentID()

	_, err := d.db.RawDB().ExecContext(ctx,
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', 0, datetime('now'))`,
		intentID, delta.RunID, d.connectorID, chatID, text, dedupeKey,
	)
	if err != nil {
		return fmt.Errorf("telegram: write intent: %w", err)
	}

	return d.deliverWithRetry(ctx, intentID, chatID, text, delta.Kind)
}

// Drain delivers all pending/retrying intents from a previous session.
func (d *OutboundDispatcher) Drain(ctx context.Context) error {
	rows, err := d.db.RawDB().QueryContext(ctx,
		`SELECT id, chat_id, message_text, run_id FROM outbound_intents
		 WHERE status IN ('pending', 'retrying') AND connector_id = ?`,
		d.connectorID)
	if err != nil {
		return fmt.Errorf("telegram: drain query: %w", err)
	}
	defer rows.Close()

	type intent struct {
		id, chatID, text, runID string
	}
	var intents []intent
	for rows.Next() {
		var it intent
		if err := rows.Scan(&it.id, &it.chatID, &it.text, &it.runID); err != nil {
			return fmt.Errorf("telegram: drain scan: %w", err)
		}
		intents = append(intents, it)
	}
	_ = rows.Close()

	for _, it := range intents {
		_ = d.deliverWithRetry(ctx, it.id, it.chatID, it.text, "")
	}
	return nil
}

// deliverWithRetry attempts delivery up to maxAttempts times with retryDelay between each.
func (d *OutboundDispatcher) deliverWithRetry(ctx context.Context, intentID, chatID, text, eventKind string) error {
	var lastErr error
	for attempt := 1; attempt <= d.maxAttempts; attempt++ {
		if attempt > 1 && d.retryDelay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d.retryDelay):
			}
		}

		if err := d.sendMessage(ctx, chatID, text); err != nil {
			lastErr = err
			// Mark retrying.
			_, _ = d.db.RawDB().ExecContext(ctx,
				"UPDATE outbound_intents SET status='retrying', attempts=?, last_attempt_at=datetime('now') WHERE id=?",
				attempt, intentID,
			)
			continue
		}

		// Success.
		_, _ = d.db.RawDB().ExecContext(ctx,
			"UPDATE outbound_intents SET status='delivered', attempts=?, last_attempt_at=datetime('now') WHERE id=?",
			attempt, intentID,
		)
		return nil
	}

	// All attempts failed — mark terminal.
	_, _ = d.db.RawDB().ExecContext(ctx,
		"UPDATE outbound_intents SET status='terminal', attempts=?, last_attempt_at=datetime('now') WHERE id=?",
		d.maxAttempts, intentID,
	)

	// Append delivery_failed journal event.
	_ = d.appendDeliveryFailedEvent(ctx, intentID, chatID, eventKind, lastErr)

	return lastErr
}

func (d *OutboundDispatcher) appendDeliveryFailedEvent(
	ctx context.Context,
	intentID string,
	chatID string,
	eventKind string,
	lastErr error,
) error {
	var runID string
	var conversationID string
	err := d.db.RawDB().QueryRowContext(ctx,
		`SELECT oi.run_id, COALESCE(r.conversation_id, '')
		 FROM outbound_intents oi
		 LEFT JOIN runs r ON r.id = oi.run_id
		 WHERE oi.id = ?`,
		intentID,
	).Scan(&runID, &conversationID)
	if err != nil {
		return fmt.Errorf("telegram: load delivery failure context: %w", err)
	}
	if conversationID == "" || runID == "" {
		return nil
	}

	payload, err := json.Marshal(map[string]any{
		"intent_id":    intentID,
		"chat_id":      chatID,
		"event_kind":   eventKind,
		"connector_id": d.connectorID,
		"error":        lastErr.Error(),
	})
	if err != nil {
		return fmt.Errorf("telegram: marshal delivery_failed payload: %w", err)
	}

	if err := d.cs.AppendEvent(ctx, model.Event{
		ID:             generateIntentID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "delivery_failed",
		PayloadJSON:    payload,
		CreatedAt:      time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("telegram: append delivery_failed: %w", err)
	}
	return nil
}

func (d *OutboundDispatcher) sendMessage(ctx context.Context, chatID, text string) error {
	apiURL := fmt.Sprintf("%s%s/sendMessage", d.bot.apiBase, d.bot.token)

	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("telegram: build sendMessage: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := d.bot.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: sendMessage: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram: sendMessage status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(body, &result); err != nil || !result.OK {
		return fmt.Errorf("telegram: sendMessage not ok: %s", body)
	}
	return nil
}

func buildMessage(delta model.ReplayDelta) string {
	switch delta.Kind {
	case "run_started":
		return fmt.Sprintf("Run %s started.", delta.RunID)
	case "run_blocked":
		return fmt.Sprintf("Run %s is blocked and needs attention.", delta.RunID)
	case "run_completed":
		return fmt.Sprintf("Run %s completed.", delta.RunID)
	case "run_interrupted":
		return fmt.Sprintf("Run %s was interrupted.", delta.RunID)
	case "approval_needed":
		return fmt.Sprintf("Run %s needs approval. Visit the web UI to review.", delta.RunID)
	default:
		return fmt.Sprintf("Run %s: %s", delta.RunID, delta.Kind)
	}
}

func generateIntentID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
