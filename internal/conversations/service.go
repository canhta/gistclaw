package conversations

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

var ErrConversationBusy = fmt.Errorf("conversation: competing root run active")

type ConversationStore struct {
	db *store.DB
}

func NewConversationStore(db *store.DB) *ConversationStore {
	return &ConversationStore{db: db}
}

func (s *ConversationStore) Resolve(ctx context.Context, key ConversationKey) (model.Conversation, error) {
	normalized := key.Normalize()

	var conv model.Conversation
	err := s.db.RawDB().QueryRowContext(ctx,
		"SELECT id, key, created_at FROM conversations WHERE key = ?",
		normalized,
	).Scan(&conv.ID, &conv.Key, &conv.CreatedAt)
	if err == nil {
		return conv, nil
	}
	if err != sql.ErrNoRows {
		return model.Conversation{}, fmt.Errorf("resolve conversation: %w", err)
	}

	id := generateID()
	now := time.Now().UTC()
	_, err = s.db.RawDB().ExecContext(ctx,
		"INSERT INTO conversations (id, key, created_at) VALUES (?, ?, ?) ON CONFLICT(key) DO NOTHING",
		id, normalized, now,
	)
	if err != nil {
		return model.Conversation{}, fmt.Errorf("create conversation: %w", err)
	}

	err = s.db.RawDB().QueryRowContext(ctx,
		"SELECT id, key, created_at FROM conversations WHERE key = ?",
		normalized,
	).Scan(&conv.ID, &conv.Key, &conv.CreatedAt)
	if err != nil {
		return model.Conversation{}, fmt.Errorf("re-read conversation: %w", err)
	}

	return conv, nil
}

func (s *ConversationStore) AppendEvent(ctx context.Context, evt model.Event) error {
	return s.db.Tx(ctx, func(tx *sql.Tx) error {
		if evt.CreatedAt.IsZero() {
			evt.CreatedAt = time.Now().UTC()
		}

		_, err := tx.ExecContext(ctx,
			`INSERT INTO events (id, conversation_id, run_id, parent_run_id, kind, payload_json, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			evt.ID, evt.ConversationID, evt.RunID, evt.ParentRunID, evt.Kind, evt.PayloadJSON, evt.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("append event: %w", err)
		}

		switch evt.Kind {
		case "run_started":
			_, err = tx.ExecContext(ctx,
				"UPDATE runs SET status = 'active', updated_at = ? WHERE id = ?",
				evt.CreatedAt, evt.RunID,
			)
		case "run_completed":
			_, err = tx.ExecContext(ctx,
				"UPDATE runs SET status = 'completed', updated_at = ? WHERE id = ?",
				evt.CreatedAt, evt.RunID,
			)
		case "run_interrupted":
			_, err = tx.ExecContext(ctx,
				"UPDATE runs SET status = 'interrupted', updated_at = ? WHERE id = ?",
				evt.CreatedAt, evt.RunID,
			)
		case "run_failed":
			_, err = tx.ExecContext(ctx,
				"UPDATE runs SET status = 'failed', updated_at = ? WHERE id = ?",
				evt.CreatedAt, evt.RunID,
			)
		case "budget_exhausted":
			_, err = tx.ExecContext(ctx,
				"UPDATE runs SET updated_at = ? WHERE id = ?",
				evt.CreatedAt, evt.RunID,
			)
		}
		if err != nil {
			return fmt.Errorf("update projection: %w", err)
		}

		return nil
	})
}

func (s *ConversationStore) ListEvents(ctx context.Context, conversationID string, limit int) ([]model.Event, error) {
	rows, err := s.db.RawDB().QueryContext(ctx,
		`SELECT id, conversation_id, COALESCE(run_id, ''), COALESCE(parent_run_id, ''), kind,
		 COALESCE(payload_json, x''), created_at
		 FROM events
		 WHERE conversation_id = ?
		 ORDER BY created_at ASC
		 LIMIT ?`,
		conversationID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	events := make([]model.Event, 0)
	for rows.Next() {
		var event model.Event
		if err := rows.Scan(
			&event.ID,
			&event.ConversationID,
			&event.RunID,
			&event.ParentRunID,
			&event.Kind,
			&event.PayloadJSON,
			&event.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

func (s *ConversationStore) ActiveRootRun(ctx context.Context, conversationID string) (model.RunRef, error) {
	var ref model.RunRef
	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT id, status
		 FROM runs
		 WHERE conversation_id = ? AND parent_run_id IS NULL AND status = 'active'
		 ORDER BY created_at ASC
		 LIMIT 1`,
		conversationID,
	).Scan(&ref.ID, &ref.Status)
	if err == sql.ErrNoRows {
		return model.RunRef{}, nil
	}
	if err != nil {
		return model.RunRef{}, fmt.Errorf("active root run: %w", err)
	}
	return ref, nil
}

func (s *ConversationStore) DB() *store.DB {
	return s.db
}

func generateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
