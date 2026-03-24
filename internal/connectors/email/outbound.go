// Package email implements a model.Connector for SMTP email delivery.
// Inbound messages are not supported (Start is a no-op).
// Outbound notifications are delivered via net/smtp.SendMail.
package email

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/smtp"
	"time"

	deliveryconnector "github.com/canhta/gistclaw/internal/connectors/delivery"
	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

// outboundAllowedKinds mirrors the filter used by the Telegram and WhatsApp connectors.
var outboundAllowedKinds = map[string]bool{
	"run_started":     true,
	"run_blocked":     true,
	"run_completed":   true,
	"run_interrupted": true,
	"approval_needed": true,
}

// SMTPConfig holds the connection settings for the outgoing mail server.
type SMTPConfig struct {
	Addr     string // host:port, e.g. "smtp.example.com:587"
	From     string // envelope sender address
	Username string // SMTP AUTH username (empty = no auth)
	Password string // SMTP AUTH password
}

// senderFunc is the signature of smtp.SendMail — injectable for tests.
type senderFunc func(addr, from string, to []string, msg []byte) error

// OutboundDispatcher delivers outbound notifications via SMTP.
// It implements model.Connector.
type OutboundDispatcher struct {
	connectorID string
	cfg         SMTPConfig
	db          *store.DB
	cs          *conversations.ConversationStore
	sender      senderFunc
	maxAttempts int
	retryDelay  time.Duration
}

// NewOutboundDispatcher creates a dispatcher using the given SMTP configuration.
func NewOutboundDispatcher(cfg SMTPConfig, db *store.DB, cs *conversations.ConversationStore) *OutboundDispatcher {
	return &OutboundDispatcher{
		connectorID: "email",
		cfg:         cfg,
		db:          db,
		cs:          cs,
		sender:      defaultSender,
		maxAttempts: 5,
		retryDelay:  2 * time.Second,
	}
}

func defaultSender(addr, from string, to []string, msg []byte) error {
	return smtp.SendMail(addr, nil, from, to, msg)
}

func (d *OutboundDispatcher) ID() string { return d.connectorID }

// Start is a no-op for email. Inbound email is out of scope; this connector
// only handles outbound notifications.
func (d *OutboundDispatcher) Start(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// Notify records an outbound intent and attempts immediate SMTP delivery.
// The dedupeKey prevents re-delivery of the same event.
func (d *OutboundDispatcher) Notify(ctx context.Context, chatID string, delta model.ReplayDelta, dedupeKey string) error {
	if !outboundAllowedKinds[delta.Kind] {
		return nil
	}

	var existing int
	_ = d.db.RawDB().QueryRowContext(ctx,
		"SELECT count(*) FROM outbound_intents WHERE dedupe_key = ?", dedupeKey,
	).Scan(&existing)
	if existing > 0 {
		return nil
	}

	subject, body := buildEmail(delta)
	text := subject + "\n" + body
	intentID := generateID()

	_, err := d.db.RawDB().ExecContext(ctx,
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', 0, datetime('now'))`,
		intentID, delta.RunID, d.connectorID, chatID, text, dedupeKey,
	)
	if err != nil {
		return fmt.Errorf("email: write intent: %w", err)
	}

	return d.deliverWithRetry(ctx, intentID, chatID, subject, body, delta.Kind)
}

// Drain delivers all pending or retrying outbound intents from a prior session.
func (d *OutboundDispatcher) Drain(ctx context.Context) error {
	rows, err := d.db.RawDB().QueryContext(ctx,
		`SELECT id, chat_id, message_text FROM outbound_intents
		 WHERE status IN ('pending', 'retrying') AND connector_id = ?`,
		d.connectorID)
	if err != nil {
		return fmt.Errorf("email: drain query: %w", err)
	}
	defer rows.Close()

	type intent struct{ id, chatID, text string }
	var intents []intent
	for rows.Next() {
		var it intent
		if err := rows.Scan(&it.id, &it.chatID, &it.text); err != nil {
			return fmt.Errorf("email: drain scan: %w", err)
		}
		intents = append(intents, it)
	}
	_ = rows.Close()

	for _, it := range intents {
		// text stored as "subject\nbody" — split on first newline.
		subject, body := splitSubjectBody(it.text)
		_ = d.deliverWithRetry(ctx, it.id, it.chatID, subject, body, "")
	}
	return nil
}

func (d *OutboundDispatcher) deliverWithRetry(ctx context.Context, intentID, to, subject, body, eventKind string) error {
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

		if err := d.deliverOnce(to, subject, body); err != nil {
			lastErr = err
			d.markStatus(ctx, intentID, "retrying", attempt)
			continue
		}

		d.markStatus(ctx, intentID, "delivered", attempt)
		return nil
	}

	d.markStatus(ctx, intentID, "terminal", maxAttempts)
	_ = deliveryconnector.AppendDeliveryFailedEvent(
		ctx,
		d.db,
		d.cs,
		intentID,
		d.connectorID,
		to,
		eventKind,
		lastErr,
	)
	return lastErr
}

// deliverOnce builds an RFC 2822 message and sends it via SMTP once.
func (d *OutboundDispatcher) deliverOnce(to, subject, body string) error {
	msg := buildRawMessage(d.cfg.From, to, subject, body)
	if err := d.sender(d.cfg.Addr, d.cfg.From, []string{to}, msg); err != nil {
		return fmt.Errorf("email: send: %w", err)
	}
	return nil
}

func (d *OutboundDispatcher) markStatus(ctx context.Context, intentID, status string, attempts int) {
	_, _ = d.db.RawDB().ExecContext(ctx,
		"UPDATE outbound_intents SET status=?, attempts=?, last_attempt_at=datetime('now') WHERE id=?",
		status, attempts, intentID,
	)
}

// buildRawMessage constructs a minimal RFC 2822 email message.
func buildRawMessage(from, to, subject, body string) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\n", from)
	fmt.Fprintf(&buf, "To: %s\r\n", to)
	fmt.Fprintf(&buf, "Subject: %s\r\n", subject)
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: text/plain; charset=utf-8\r\n")
	fmt.Fprintf(&buf, "\r\n")
	fmt.Fprintf(&buf, "%s\r\n", body)
	return buf.Bytes()
}

func buildEmail(delta model.ReplayDelta) (subject, body string) {
	switch delta.Kind {
	case "run_started":
		return fmt.Sprintf("[GistClaw] Run %s started", delta.RunID),
			fmt.Sprintf("Run %s has started.", delta.RunID)
	case "run_blocked":
		return fmt.Sprintf("[GistClaw] Run %s blocked", delta.RunID),
			fmt.Sprintf("Run %s is blocked and needs attention.", delta.RunID)
	case "run_completed":
		return fmt.Sprintf("[GistClaw] Run %s completed", delta.RunID),
			fmt.Sprintf("Run %s has completed successfully.", delta.RunID)
	case "run_interrupted":
		return fmt.Sprintf("[GistClaw] Run %s interrupted", delta.RunID),
			fmt.Sprintf("Run %s was interrupted.", delta.RunID)
	case "approval_needed":
		return fmt.Sprintf("[GistClaw] Run %s needs approval", delta.RunID),
			fmt.Sprintf("Run %s needs your approval. Visit the web UI to review.", delta.RunID)
	default:
		return fmt.Sprintf("[GistClaw] Run %s: %s", delta.RunID, delta.Kind),
			fmt.Sprintf("Run %s: %s", delta.RunID, delta.Kind)
	}
}

// splitSubjectBody splits the stored "subject\nbody" text back into parts.
func splitSubjectBody(text string) (subject, body string) {
	for i, c := range text {
		if c == '\n' {
			return text[:i], text[i+1:]
		}
	}
	return text, ""
}

func generateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// Ensure time import is used (time.Time used in future extensions).
var _ = time.Now

// Compile-time interface check.
var _ model.Connector = (*OutboundDispatcher)(nil)
