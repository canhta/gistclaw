package zalopersonal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	deliveryconnector "github.com/canhta/gistclaw/internal/connectors/delivery"
	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

var outboundAllowedKinds = map[string]bool{
	"run_started":        true,
	"run_blocked":        true,
	"run_completed":      true,
	"run_interrupted":    true,
	"approval_requested": true,
}

type TextSender interface {
	SendText(ctx context.Context, chatID, text string) error
}

type OutboundDispatcher struct {
	connectorID string
	sender      TextSender
	db          *store.DB
	cs          *conversations.ConversationStore
	maxAttempts int
	retryDelay  time.Duration
}

func NewOutboundDispatcher(sender TextSender, db *store.DB, cs *conversations.ConversationStore) *OutboundDispatcher {
	return &OutboundDispatcher{
		connectorID: "zalo_personal",
		sender:      sender,
		db:          db,
		cs:          cs,
		maxAttempts: 5,
		retryDelay:  2 * time.Second,
	}
}

func (d *OutboundDispatcher) ID() string {
	return d.connectorID
}

func (d *OutboundDispatcher) Send(ctx context.Context, chatID, text string) error {
	intentID, err := d.enqueueIntent(ctx, "", chatID, text, "")
	if err != nil {
		return err
	}
	return d.deliverWithRetry(ctx, intentID, chatID, text, "connector_command")
}

func (d *OutboundDispatcher) Notify(ctx context.Context, chatID string, delta model.ReplayDelta, dedupeKey string) error {
	if !outboundAllowedKinds[delta.Kind] {
		return nil
	}

	var existing int
	_ = d.db.RawDB().QueryRowContext(ctx,
		"SELECT count(*) FROM outbound_intents WHERE dedupe_key = ?",
		dedupeKey,
	).Scan(&existing)
	if existing > 0 {
		return nil
	}

	text := buildMessage(delta)
	intentID, err := d.enqueueIntent(ctx, delta.RunID, chatID, text, dedupeKey)
	if err != nil {
		return err
	}
	return d.deliverWithRetry(ctx, intentID, chatID, text, delta.Kind)
}

func (d *OutboundDispatcher) Drain(ctx context.Context) error {
	rows, err := d.db.RawDB().QueryContext(ctx,
		`SELECT id, chat_id, message_text FROM outbound_intents
		 WHERE status IN ('pending', 'retrying') AND connector_id = ?`,
		d.connectorID,
	)
	if err != nil {
		return fmt.Errorf("zalo personal outbound: drain query: %w", err)
	}
	defer rows.Close()

	type intent struct {
		id, chatID, text string
	}
	intents := make([]intent, 0)
	for rows.Next() {
		var item intent
		if err := rows.Scan(&item.id, &item.chatID, &item.text); err != nil {
			return fmt.Errorf("zalo personal outbound: drain scan: %w", err)
		}
		intents = append(intents, item)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("zalo personal outbound: drain rows: %w", err)
	}

	for _, item := range intents {
		if err := d.deliverWithRetry(ctx, item.id, item.chatID, item.text, ""); err != nil {
			return err
		}
	}
	return nil
}

func (d *OutboundDispatcher) enqueueIntent(ctx context.Context, runID, chatID, text, dedupeKey string) (string, error) {
	intentID := generateID()
	if _, err := d.db.RawDB().ExecContext(ctx,
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at)
		 VALUES (?, NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), 'pending', 0, datetime('now'))`,
		intentID, runID, d.connectorID, chatID, text, dedupeKey,
	); err != nil {
		return "", fmt.Errorf("zalo personal outbound: write intent: %w", err)
	}
	return intentID, nil
}

func (d *OutboundDispatcher) deliverWithRetry(ctx context.Context, intentID, chatID, text, eventKind string) error {
	maxAttempts := d.maxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 && d.retryDelay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d.retryDelay):
			}
		}

		if err := d.sender.SendText(ctx, chatID, text); err != nil {
			lastErr = err
			d.markStatus(ctx, intentID, "retrying", attempt)
			continue
		}

		d.markStatus(ctx, intentID, "delivered", attempt)
		return nil
	}

	d.markStatus(ctx, intentID, "terminal", maxAttempts)
	_ = deliveryconnector.AppendDeliveryFailedEvent(ctx, d.db, d.cs, intentID, d.connectorID, chatID, eventKind, lastErr)
	return lastErr
}

func (d *OutboundDispatcher) markStatus(ctx context.Context, intentID, status string, attempts int) {
	_, _ = d.db.RawDB().ExecContext(ctx,
		"UPDATE outbound_intents SET status=?, attempts=?, last_attempt_at=datetime('now') WHERE id=?",
		status, attempts, intentID,
	)
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
	case "approval_requested":
		return fmt.Sprintf("Run %s needs approval. Visit the web UI to review.", delta.RunID)
	default:
		return fmt.Sprintf("Run %s: %s", delta.RunID, delta.Kind)
	}
}

func generateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
