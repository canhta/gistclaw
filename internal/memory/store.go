package memory

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

type Store struct {
	db        *store.DB
	convStore *conversations.ConversationStore
}

func NewStore(db *store.DB, cs *conversations.ConversationStore) *Store {
	return &Store{db: db, convStore: cs}
}

func (s *Store) WriteFact(ctx context.Context, item model.MemoryItem) error {
	if item.ID == "" {
		item.ID = memGenerateID()
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = item.CreatedAt
	}

	_, err := s.db.RawDB().ExecContext(ctx,
		`INSERT INTO memory_items
		 (id, agent_id, scope, content, source, provenance, confidence, dedupe_key, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.AgentID, item.Scope, item.Content, item.Source, item.Provenance,
		item.Confidence, item.DedupeKey, item.CreatedAt, item.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("memory: write fact: %w", err)
	}
	return nil
}

func (s *Store) UpdateFact(ctx context.Context, item model.MemoryItem) error {
	var existingSource string
	err := s.db.RawDB().QueryRowContext(ctx,
		"SELECT source FROM memory_items WHERE id = ?",
		item.ID,
	).Scan(&existingSource)
	if err == sql.ErrNoRows {
		return s.WriteFact(ctx, item)
	}
	if err != nil {
		return fmt.Errorf("memory: check existing: %w", err)
	}

	if existingSource == "human" && item.Source == "model" {
		return nil
	}

	_, err = s.db.RawDB().ExecContext(ctx,
		`UPDATE memory_items
		 SET content = ?, source = ?, provenance = ?, confidence = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		item.Content, item.Source, item.Provenance, item.Confidence, item.ID,
	)
	if err != nil {
		return fmt.Errorf("memory: update fact: %w", err)
	}
	return nil
}

func (s *Store) ForgetFact(ctx context.Context, memoryID string) error {
	_, err := s.db.RawDB().ExecContext(ctx,
		"DELETE FROM memory_items WHERE id = ?",
		memoryID,
	)
	if err != nil {
		return fmt.Errorf("memory: forget: %w", err)
	}
	return nil
}

func (s *Store) Search(ctx context.Context, query model.MemoryQuery) ([]model.MemoryItem, error) {
	sqlQuery := `SELECT id, agent_id, scope, content, source, COALESCE(provenance, ''),
		COALESCE(confidence, 0), COALESCE(dedupe_key, ''), created_at, updated_at
		FROM memory_items WHERE 1=1`
	args := make([]any, 0)

	if query.AgentID != "" {
		sqlQuery += " AND agent_id = ?"
		args = append(args, query.AgentID)
	}
	if query.Scope != "" {
		sqlQuery += " AND scope = ?"
		args = append(args, query.Scope)
	}
	if query.Keyword != "" {
		sqlQuery += " AND content LIKE ?"
		args = append(args, "%"+query.Keyword+"%")
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	sqlQuery += " ORDER BY updated_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.RawDB().QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: search: %w", err)
	}
	defer rows.Close()

	items := make([]model.MemoryItem, 0)
	for rows.Next() {
		var item model.MemoryItem
		if err := rows.Scan(
			&item.ID,
			&item.AgentID,
			&item.Scope,
			&item.Content,
			&item.Source,
			&item.Provenance,
			&item.Confidence,
			&item.DedupeKey,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("memory: scan: %w", err)
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (s *Store) SummarizeConversation(ctx context.Context, conversationID string) (model.SummaryRef, error) {
	id := memGenerateID()
	now := time.Now().UTC()
	summary := fmt.Sprintf("Summary of conversation %s generated at %s", conversationID, now.Format(time.RFC3339))

	_, err := s.db.RawDB().ExecContext(ctx,
		`INSERT INTO run_summaries (id, run_id, content, token_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(run_id) DO UPDATE SET content = excluded.content, updated_at = excluded.updated_at`,
		id, conversationID, summary, len(summary)/4, now, now,
	)
	if err != nil {
		return model.SummaryRef{}, fmt.Errorf("memory: summarize: %w", err)
	}

	return model.SummaryRef{
		ID:         id,
		RunID:      conversationID,
		Content:    summary,
		TokenCount: len(summary) / 4,
	}, nil
}

func memGenerateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
