package telegram

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	deliveryconnector "github.com/canhta/gistclaw/internal/connectors/delivery"
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
	draftMu     sync.Mutex
	drafts      map[string]draftState
	health      *healthState
}

type draftState struct {
	chatID    string
	text      string
	dirty     bool
	completed bool
}

// ID returns the connector identifier stored in outbound_intents.connector_id.
func (d *OutboundDispatcher) ID() string { return d.connectorID }

// Start runs the bot's long-poll loop until ctx is cancelled.
func (d *OutboundDispatcher) Start(ctx context.Context) error { return d.bot.Start(ctx) }

// Send records and delivers a connector-owned outbound message that is not
// attached to a run lifecycle event, such as a native control-command reply.
func (d *OutboundDispatcher) Send(ctx context.Context, chatID, text string) error {
	intentID, err := d.enqueueIntent(ctx, "", chatID, text, nil, "")
	if err != nil {
		return err
	}
	return d.deliverWithRetry(ctx, intentID, chatID, text, nil, "connector_command")
}

// NewOutboundDispatcher creates a dispatcher. token is the Telegram bot token.
func NewOutboundDispatcher(token string, db *store.DB, cs *conversations.ConversationStore, health ...*healthState) *OutboundDispatcher {
	var state *healthState
	if len(health) > 0 {
		state = health[0]
	}
	return &OutboundDispatcher{
		connectorID: "telegram",
		bot:         NewBot(token, nil),
		db:          db,
		cs:          cs,
		maxAttempts: 5,
		retryDelay:  2 * time.Second,
		drafts:      make(map[string]draftState),
		health:      state,
	}
}

// Notify records an outbound intent for an event and attempts immediate delivery.
// The dedupeKey prevents re-delivery if the same key is seen again.
// If the event kind is not in the allowed set, Notify is a no-op.
func (d *OutboundDispatcher) Notify(ctx context.Context, chatID string, delta model.ReplayDelta, dedupeKey string) error {
	if delta.Kind == "turn_delta" || delta.Kind == "turn_completed" {
		return d.recordDraft(chatID, delta)
	}

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
	intentID, err := d.enqueueIntent(ctx, delta.RunID, chatID, text, nil, dedupeKey)
	if err != nil {
		return err
	}

	return d.deliverWithRetry(ctx, intentID, chatID, text, nil, delta.Kind)
}

func (d *OutboundDispatcher) recordDraft(chatID string, delta model.ReplayDelta) error {
	if chatID == "" || delta.RunID == "" {
		return nil
	}

	var payload struct {
		Text    string `json:"text"`
		Content string `json:"content"`
	}
	if len(delta.PayloadJSON) > 0 {
		if err := json.Unmarshal(delta.PayloadJSON, &payload); err != nil {
			return fmt.Errorf("telegram: decode draft payload: %w", err)
		}
	}

	d.draftMu.Lock()
	defer d.draftMu.Unlock()

	state := d.drafts[delta.RunID]
	state.chatID = chatID
	switch delta.Kind {
	case "turn_delta":
		if payload.Text == "" {
			return nil
		}
		state.text += payload.Text
	case "turn_completed":
		if payload.Content != "" {
			state.text = payload.Content
		}
		state.completed = true
	}
	if state.text == "" {
		return nil
	}
	state.dirty = true
	d.drafts[delta.RunID] = state
	return nil
}

// Drain delivers all pending/retrying intents from a previous session.
func (d *OutboundDispatcher) Drain(ctx context.Context) error {
	rows, err := d.db.RawDB().QueryContext(ctx,
		`SELECT id, chat_id, message_text, COALESCE(metadata_json, x'7B7D'), run_id FROM outbound_intents
		 WHERE status IN ('pending', 'retrying') AND connector_id = ?`,
		d.connectorID)
	if err != nil {
		return fmt.Errorf("telegram: drain query: %w", err)
	}
	defer rows.Close()

	type intent struct {
		id, chatID, text, runID string
		metadataJSON            []byte
	}
	var intents []intent
	for rows.Next() {
		var it intent
		if err := rows.Scan(&it.id, &it.chatID, &it.text, &it.metadataJSON, &it.runID); err != nil {
			return fmt.Errorf("telegram: drain scan: %w", err)
		}
		intents = append(intents, it)
	}
	_ = rows.Close()

	for _, it := range intents {
		_ = d.deliverWithRetry(ctx, it.id, it.chatID, it.text, it.metadataJSON, "")
	}
	return nil
}

func (d *OutboundDispatcher) FlushDrafts(ctx context.Context) error {
	type pendingDraft struct {
		runID     string
		chatID    string
		text      string
		completed bool
	}

	d.draftMu.Lock()
	pending := make([]pendingDraft, 0, len(d.drafts))
	for runID, state := range d.drafts {
		if !state.dirty || state.chatID == "" || state.text == "" {
			continue
		}
		pending = append(pending, pendingDraft{
			runID:     runID,
			chatID:    state.chatID,
			text:      state.text,
			completed: state.completed,
		})
	}
	d.draftMu.Unlock()

	for _, draft := range pending {
		if err := d.sendMessageDraft(ctx, draft.chatID, draftIDForRun(draft.runID), draft.text); err != nil {
			return err
		}

		d.draftMu.Lock()
		state, ok := d.drafts[draft.runID]
		if ok && state.text == draft.text {
			if draft.completed {
				delete(d.drafts, draft.runID)
			} else {
				state.dirty = false
				d.drafts[draft.runID] = state
			}
		}
		d.draftMu.Unlock()
	}

	return nil
}

func (d *OutboundDispatcher) enqueueIntent(ctx context.Context, runID, chatID, text string, metadataJSON []byte, dedupeKey string) (string, error) {
	intentID := generateIntentID()
	_, err := d.db.RawDB().ExecContext(ctx,
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, metadata_json, dedupe_key, status, attempts, created_at)
		 VALUES (?, NULLIF(?, ''), ?, ?, ?, COALESCE(?, '{}'), NULLIF(?, ''), 'pending', 0, datetime('now'))`,
		intentID, runID, d.connectorID, chatID, text, metadataJSON, dedupeKey,
	)
	if err != nil {
		return "", fmt.Errorf("telegram: write intent: %w", err)
	}
	return intentID, nil
}

// deliverWithRetry attempts delivery up to maxAttempts times with retryDelay between each.
func (d *OutboundDispatcher) deliverWithRetry(ctx context.Context, intentID, chatID, text string, metadataJSON []byte, eventKind string) error {
	var lastErr error
	for attempt := 1; attempt <= d.maxAttempts; attempt++ {
		if attempt > 1 && d.retryDelay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d.retryDelay):
			}
		}

		if err := d.sendMessage(ctx, chatID, text, metadataJSON); err != nil {
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
		if d.health != nil {
			d.health.markDrainSuccess(time.Now().UTC())
		}
		return nil
	}

	// All attempts failed — mark terminal.
	_, _ = d.db.RawDB().ExecContext(ctx,
		"UPDATE outbound_intents SET status='terminal', attempts=?, last_attempt_at=datetime('now') WHERE id=?",
		d.maxAttempts, intentID,
	)

	// Append delivery_failed journal event.
	_ = deliveryconnector.AppendDeliveryFailedEvent(
		ctx,
		d.db,
		d.cs,
		intentID,
		d.connectorID,
		chatID,
		eventKind,
		lastErr,
	)
	if d.health != nil && lastErr != nil {
		d.health.markFailure(time.Now().UTC(), "delivery: "+lastErr.Error())
	}

	return lastErr
}

func (d *OutboundDispatcher) sendMessage(ctx context.Context, chatID, text string, metadataJSON []byte) error {
	apiURL := fmt.Sprintf("%s%s/sendMessage", d.bot.apiBase, d.bot.token)

	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	replyMarkup, err := telegramReplyMarkup(metadataJSON)
	if err != nil {
		return err
	}
	if replyMarkup != "" {
		form.Set("reply_markup", replyMarkup)
	}

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

func telegramReplyMarkup(metadataJSON []byte) (string, error) {
	if len(metadataJSON) == 0 {
		return "", nil
	}
	var metadata model.OutboundIntentMetadata
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return "", fmt.Errorf("telegram: decode outbound metadata: %w", err)
	}
	if len(metadata.ActionButtons) == 0 {
		return "", nil
	}

	row := make([]map[string]string, 0, len(metadata.ActionButtons))
	for _, button := range metadata.ActionButtons {
		if strings.TrimSpace(button.Label) == "" || strings.TrimSpace(button.Value) == "" {
			continue
		}
		row = append(row, map[string]string{
			"text":          button.Label,
			"callback_data": button.Value,
		})
	}
	if len(row) == 0 {
		return "", nil
	}
	payload, err := json.Marshal(map[string]any{
		"inline_keyboard": []any{row},
	})
	if err != nil {
		return "", fmt.Errorf("telegram: marshal reply markup: %w", err)
	}
	return string(payload), nil
}

func (d *OutboundDispatcher) sendMessageDraft(ctx context.Context, chatID string, draftID int64, text string) error {
	apiURL := fmt.Sprintf("%s%s/sendMessageDraft", d.bot.apiBase, d.bot.token)

	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("draft_id", strconv.FormatInt(draftID, 10))
	form.Set("text", text)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("telegram: build sendMessageDraft: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := d.bot.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: sendMessageDraft: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram: sendMessageDraft status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(body, &result); err != nil || !result.OK {
		return fmt.Errorf("telegram: sendMessageDraft not ok: %s", body)
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
	default:
		return fmt.Sprintf("Run %s: %s", delta.RunID, delta.Kind)
	}
}

func generateIntentID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func draftIDForRun(runID string) int64 {
	sum := crc32.ChecksumIEEE([]byte(runID))
	if sum == 0 {
		sum = 1
	}
	return int64(sum)
}
