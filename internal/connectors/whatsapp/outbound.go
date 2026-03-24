// Package whatsapp implements a model.Connector for the WhatsApp Business Cloud API.
// Inbound messages arrive via Meta webhooks (Start is a no-op for the outbound dispatcher).
// Outbound messages are delivered via the Meta Graph API Messages endpoint.
package whatsapp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

const (
	defaultAPIBase  = "https://graph.facebook.com"
	defaultAPIVer   = "v17.0"
)

// outboundAllowedKinds mirrors the same filter used by the Telegram connector.
var outboundAllowedKinds = map[string]bool{
	"run_started":     true,
	"run_blocked":     true,
	"run_completed":   true,
	"run_interrupted": true,
	"approval_needed": true,
}

// OutboundDispatcher delivers outbound notifications via the WhatsApp Cloud API.
// It implements model.Connector.
type OutboundDispatcher struct {
	connectorID   string
	phoneNumberID string // Meta WhatsApp Business phone number ID
	accessToken   string // Meta page or system user access token
	apiBase       string // Base URL — overridable for tests
	db            *store.DB
	cs            *conversations.ConversationStore
	client        *http.Client
}

// NewOutboundDispatcher creates a dispatcher for the given Meta phoneNumberID and accessToken.
func NewOutboundDispatcher(phoneNumberID, accessToken string, db *store.DB, cs *conversations.ConversationStore) *OutboundDispatcher {
	return newWithBaseURL(phoneNumberID, accessToken, db, cs, defaultAPIBase)
}

func newWithBaseURL(phoneNumberID, accessToken string, db *store.DB, cs *conversations.ConversationStore, apiBase string) *OutboundDispatcher {
	return &OutboundDispatcher{
		connectorID:   "whatsapp",
		phoneNumberID: phoneNumberID,
		accessToken:   accessToken,
		apiBase:       apiBase,
		db:            db,
		cs:            cs,
		client:        &http.Client{},
	}
}

func (d *OutboundDispatcher) ID() string { return d.connectorID }

// Start is a no-op for WhatsApp. Inbound messages arrive via a Meta webhook
// registered separately in the web layer; this connector only handles outbound.
func (d *OutboundDispatcher) Start(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// Notify records an outbound intent and attempts immediate delivery via the
// WhatsApp Cloud API. The dedupeKey prevents re-delivery.
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

	text := buildMessage(delta)
	intentID := generateID()

	_, err := d.db.RawDB().ExecContext(ctx,
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', 0, datetime('now'))`,
		intentID, delta.RunID, d.connectorID, chatID, text, dedupeKey,
	)
	if err != nil {
		return fmt.Errorf("whatsapp: write intent: %w", err)
	}

	return d.deliver(ctx, intentID, chatID, text)
}

// Drain delivers all pending or retrying outbound intents from a prior session.
func (d *OutboundDispatcher) Drain(ctx context.Context) error {
	rows, err := d.db.RawDB().QueryContext(ctx,
		`SELECT id, chat_id, message_text FROM outbound_intents
		 WHERE status IN ('pending', 'retrying') AND connector_id = ?`,
		d.connectorID)
	if err != nil {
		return fmt.Errorf("whatsapp: drain query: %w", err)
	}
	defer rows.Close()

	type intent struct{ id, chatID, text string }
	var intents []intent
	for rows.Next() {
		var it intent
		if err := rows.Scan(&it.id, &it.chatID, &it.text); err != nil {
			return fmt.Errorf("whatsapp: drain scan: %w", err)
		}
		intents = append(intents, it)
	}
	_ = rows.Close()

	for _, it := range intents {
		_ = d.deliver(ctx, it.id, it.chatID, it.text)
	}
	return nil
}

// deliver POSTs a WhatsApp text message and updates the intent status.
func (d *OutboundDispatcher) deliver(ctx context.Context, intentID, to, text string) error {
	endpoint := fmt.Sprintf("%s/%s/%s/messages", d.apiBase, defaultAPIVer, d.phoneNumberID)

	payload, err := json.Marshal(map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "text",
		"text":              map[string]string{"body": text},
	})
	if err != nil {
		return fmt.Errorf("whatsapp: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("whatsapp: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.accessToken)

	resp, err := d.client.Do(req)
	if err != nil {
		d.markStatus(ctx, intentID, "retrying")
		return fmt.Errorf("whatsapp: send: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		d.markStatus(ctx, intentID, "retrying")
		return fmt.Errorf("whatsapp: API status %d: %s", resp.StatusCode, body)
	}

	d.markStatus(ctx, intentID, "delivered")
	return nil
}

func (d *OutboundDispatcher) markStatus(ctx context.Context, intentID, status string) {
	_, _ = d.db.RawDB().ExecContext(ctx,
		"UPDATE outbound_intents SET status=?, last_attempt_at=datetime('now') WHERE id=?",
		status, intentID,
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
	case "approval_needed":
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

// Compile-time check.
var _ model.Connector = (*OutboundDispatcher)(nil)

// Ensure time import is used.
var _ = time.Now
