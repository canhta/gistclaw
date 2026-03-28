package threadstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/canhta/gistclaw/internal/store"
)

type Summary struct {
	ConnectorID        string
	AccountID          string
	ThreadID           string
	ThreadType         string
	Title              string
	Subtitle           string
	LastMessagePreview string
	LastMessageAt      time.Time
	Metadata           map[string]string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Filter struct {
	ConnectorID string
	AccountID   string
	Limit       int
}

type Store struct {
	db *store.DB
}

func New(db *store.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Upsert(ctx context.Context, summary Summary) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("threadstate: db is required")
	}
	if summary.ConnectorID == "" {
		return fmt.Errorf("threadstate: connector id is required")
	}
	if summary.ThreadID == "" {
		return fmt.Errorf("threadstate: thread id is required")
	}

	metadataJSON, err := json.Marshal(summary.Metadata)
	if err != nil {
		return fmt.Errorf("threadstate: encode metadata: %w", err)
	}

	lastMessageAt := any(nil)
	if !summary.LastMessageAt.IsZero() {
		lastMessageAt = summary.LastMessageAt.UTC()
	}

	_, err = s.db.RawDB().ExecContext(ctx, `
		INSERT INTO connector_threads (
			connector_id, account_id, thread_id, thread_type, title, subtitle,
			last_message_preview, last_message_at, metadata_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(connector_id, account_id, thread_id) DO UPDATE SET
			thread_type = excluded.thread_type,
			title = CASE WHEN excluded.title <> '' THEN excluded.title ELSE connector_threads.title END,
			subtitle = CASE WHEN excluded.subtitle <> '' THEN excluded.subtitle ELSE connector_threads.subtitle END,
			last_message_preview = CASE
				WHEN excluded.last_message_preview <> '' THEN excluded.last_message_preview
				ELSE connector_threads.last_message_preview
			END,
			last_message_at = COALESCE(excluded.last_message_at, connector_threads.last_message_at),
			metadata_json = excluded.metadata_json,
			updated_at = datetime('now')
	`, summary.ConnectorID, summary.AccountID, summary.ThreadID, summary.ThreadType, summary.Title, summary.Subtitle, summary.LastMessagePreview, lastMessageAt, string(metadataJSON))
	if err != nil {
		return fmt.Errorf("threadstate: upsert summary: %w", err)
	}
	return nil
}

func (s *Store) List(ctx context.Context, filter Filter) ([]Summary, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("threadstate: db is required")
	}
	if filter.ConnectorID == "" {
		return nil, fmt.Errorf("threadstate: connector id is required")
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.RawDB().QueryContext(ctx, `
		SELECT connector_id, account_id, thread_id, thread_type, title, subtitle,
		       last_message_preview, last_message_at, metadata_json, created_at, updated_at
		  FROM connector_threads
		 WHERE connector_id = ? AND account_id = ?
		 ORDER BY COALESCE(last_message_at, updated_at) DESC, thread_id ASC
		 LIMIT ?`,
		filter.ConnectorID, filter.AccountID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("threadstate: list summaries: %w", err)
	}
	defer rows.Close()

	summaries := make([]Summary, 0, limit)
	for rows.Next() {
		var summary Summary
		var lastMessageAt sql.NullTime
		var metadataJSON string
		if err := rows.Scan(
			&summary.ConnectorID,
			&summary.AccountID,
			&summary.ThreadID,
			&summary.ThreadType,
			&summary.Title,
			&summary.Subtitle,
			&summary.LastMessagePreview,
			&lastMessageAt,
			&metadataJSON,
			&summary.CreatedAt,
			&summary.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("threadstate: scan summary: %w", err)
		}
		if lastMessageAt.Valid {
			summary.LastMessageAt = lastMessageAt.Time.UTC()
		}
		if metadataJSON != "" {
			if err := json.Unmarshal([]byte(metadataJSON), &summary.Metadata); err != nil {
				return nil, fmt.Errorf("threadstate: decode metadata: %w", err)
			}
		}
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("threadstate: iterate summaries: %w", err)
	}
	return summaries, nil
}
