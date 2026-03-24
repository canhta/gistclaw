package delivery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

func AppendDeliveryFailedEvent(
	ctx context.Context,
	db *store.DB,
	cs *conversations.ConversationStore,
	intentID string,
	connectorID string,
	chatID string,
	eventKind string,
	deliveryErr error,
) error {
	if db == nil || cs == nil || deliveryErr == nil {
		return nil
	}

	var runID string
	var conversationID string
	err := db.RawDB().QueryRowContext(ctx,
		`SELECT oi.run_id, COALESCE(r.conversation_id, '')
		 FROM outbound_intents oi
		 LEFT JOIN runs r ON r.id = oi.run_id
		 WHERE oi.id = ?`,
		intentID,
	).Scan(&runID, &conversationID)
	if err != nil {
		return fmt.Errorf("delivery: load failure context: %w", err)
	}
	if conversationID == "" || runID == "" {
		return nil
	}

	payload, err := json.Marshal(map[string]any{
		"intent_id":    intentID,
		"chat_id":      chatID,
		"event_kind":   eventKind,
		"connector_id": connectorID,
		"error":        deliveryErr.Error(),
	})
	if err != nil {
		return fmt.Errorf("delivery: marshal delivery_failed payload: %w", err)
	}

	if err := cs.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "delivery_failed",
		PayloadJSON:    payload,
		CreatedAt:      time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("delivery: append delivery_failed: %w", err)
	}
	return nil
}

func generateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
