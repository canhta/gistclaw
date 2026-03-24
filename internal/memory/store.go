package memory

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
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

type ContextView struct {
	Summary model.SummaryRef
	Items   []model.MemoryItem
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
	return s.Forget(ctx, memoryID)
}

// Forget soft-deletes a memory item by setting forgotten_at to now.
// Forgotten items are excluded from all Filter and Search results.
func (s *Store) Forget(ctx context.Context, memoryID string) error {
	_, err := s.db.RawDB().ExecContext(ctx,
		"UPDATE memory_items SET forgotten_at = datetime('now') WHERE id = ?",
		memoryID,
	)
	if err != nil {
		return fmt.Errorf("memory: forget: %w", err)
	}
	return nil
}

// Edit updates a fact's content and records source=human, replacing any prior source.
// Human edits are permanent: a subsequent model UpdateFact will not overwrite them.
func (s *Store) Edit(ctx context.Context, memoryID, newContent string) error {
	_, err := s.db.RawDB().ExecContext(ctx,
		`UPDATE memory_items
		 SET content = ?, source = 'human', updated_at = datetime('now')
		 WHERE id = ?`,
		newContent, memoryID,
	)
	if err != nil {
		return fmt.Errorf("memory: edit: %w", err)
	}
	return nil
}

// MemoryFilter controls which facts are returned by Filter.
// Zero-value fields are treated as "no constraint".
type MemoryFilter struct {
	Scope   string
	AgentID string
}

// Filter returns all non-forgotten facts matching the given filter.
func (s *Store) Filter(ctx context.Context, f MemoryFilter) ([]model.MemoryItem, error) {
	q := `SELECT id, agent_id, scope, content, source, COALESCE(provenance, ''),
		COALESCE(confidence, 0), COALESCE(dedupe_key, ''), created_at, updated_at
		FROM memory_items
		WHERE forgotten_at IS NULL`
	args := make([]any, 0)

	if f.AgentID != "" {
		q += " AND agent_id = ?"
		args = append(args, f.AgentID)
	}
	if f.Scope != "" {
		q += " AND scope = ?"
		args = append(args, f.Scope)
	}
	q += " ORDER BY updated_at DESC"

	rows, err := s.db.RawDB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: filter: %w", err)
	}
	defer rows.Close()

	items := make([]model.MemoryItem, 0)
	for rows.Next() {
		var item model.MemoryItem
		if err := rows.Scan(
			&item.ID, &item.AgentID, &item.Scope, &item.Content, &item.Source,
			&item.Provenance, &item.Confidence, &item.DedupeKey,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("memory: filter scan: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Search(ctx context.Context, query model.MemoryQuery) ([]model.MemoryItem, error) {
	sqlQuery := `SELECT id, agent_id, scope, content, source, COALESCE(provenance, ''),
		COALESCE(confidence, 0), COALESCE(dedupe_key, ''), created_at, updated_at
		FROM memory_items WHERE forgotten_at IS NULL`
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

func (s *Store) UpsertWorkingSummary(ctx context.Context, runID, conversationID string) (model.SummaryRef, error) {
	id := memGenerateID()
	now := time.Now().UTC()
	summary := fmt.Sprintf("Summary of conversation %s for run %s generated at %s", conversationID, runID, now.Format(time.RFC3339))

	ref := model.SummaryRef{
		ID:         id,
		RunID:      runID,
		Content:    summary,
		TokenCount: len(summary) / 4,
	}

	payload, err := json.Marshal(map[string]any{
		"summary_id":  ref.ID,
		"run_id":      ref.RunID,
		"content":     ref.Content,
		"token_count": ref.TokenCount,
	})
	if err != nil {
		return model.SummaryRef{}, fmt.Errorf("memory: summarize payload: %w", err)
	}

	if err := s.convStore.AppendEvent(ctx, model.Event{
		ID:             memGenerateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "summary_upserted",
		PayloadJSON:    payload,
		CreatedAt:      now,
	}); err != nil {
		return model.SummaryRef{}, fmt.Errorf("memory: summarize: %w", err)
	}

	return ref, nil
}

func (s *Store) LoadContext(ctx context.Context, runID, agentID, scope string, limit int) (ContextView, error) {
	view := ContextView{
		Items: make([]model.MemoryItem, 0),
	}

	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT id, run_id, content, token_count
		 FROM run_summaries
		 WHERE run_id = ?`,
		runID,
	).Scan(
		&view.Summary.ID,
		&view.Summary.RunID,
		&view.Summary.Content,
		&view.Summary.TokenCount,
	)
	if err != nil && err != sql.ErrNoRows {
		return ContextView{}, fmt.Errorf("memory: load summary: %w", err)
	}

	items, err := s.Search(ctx, model.MemoryQuery{
		AgentID: agentID,
		Scope:   scope,
		Limit:   limit,
	})
	if err != nil {
		return ContextView{}, fmt.Errorf("memory: load scoped facts: %w", err)
	}
	view.Items = items

	return view, nil
}

func memGenerateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
