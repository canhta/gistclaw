package memory

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
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

type SearchPageQuery struct {
	ProjectID string
	AgentID   string
	Scope     string
	Keyword   string
	Limit     int
	Cursor    string
	Direction string
}

type SearchPage struct {
	Items      []model.MemoryItem
	NextCursor string
	PrevCursor string
	HasNext    bool
	HasPrev    bool
}

func NewStore(db *store.DB, cs *conversations.ConversationStore) *Store {
	return &Store{db: db, convStore: cs}
}

func (s *Store) WriteFact(ctx context.Context, item model.MemoryItem) error {
	if strings.TrimSpace(item.ProjectID) == "" {
		return fmt.Errorf("memory: write fact: project_id required")
	}
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
		 (id, project_id, agent_id, scope, content, source, provenance, confidence, dedupe_key, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.ProjectID, item.AgentID, item.Scope, item.Content, item.Source, item.Provenance,
		item.Confidence, item.DedupeKey, item.CreatedAt, item.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("memory: write fact: %w", err)
	}
	return nil
}

func (s *Store) UpdateFact(ctx context.Context, item model.MemoryItem) error {
	if strings.TrimSpace(item.ProjectID) == "" {
		return fmt.Errorf("memory: update fact: project_id required")
	}
	var existingSource string
	err := s.db.RawDB().QueryRowContext(ctx,
		"SELECT source FROM memory_items WHERE id = ? AND project_id = ?",
		item.ID, item.ProjectID,
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
		 WHERE id = ? AND project_id = ?`,
		item.Content, item.Source, item.Provenance, item.Confidence, item.ID, item.ProjectID,
	)
	if err != nil {
		return fmt.Errorf("memory: update fact: %w", err)
	}
	return nil
}

func (s *Store) ForgetFact(ctx context.Context, projectID, memoryID string) error {
	return s.Forget(ctx, projectID, memoryID)
}

// Forget soft-deletes a memory item by setting forgotten_at to now.
// Forgotten items are excluded from all Filter and Search results.
func (s *Store) Forget(ctx context.Context, projectID, memoryID string) error {
	if strings.TrimSpace(projectID) == "" {
		return fmt.Errorf("memory: forget: project_id required")
	}
	res, err := s.db.RawDB().ExecContext(ctx,
		"UPDATE memory_items SET forgotten_at = datetime('now') WHERE id = ? AND project_id = ?",
		memoryID, projectID,
	)
	if err != nil {
		return fmt.Errorf("memory: forget: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Edit updates a fact's content and records source=human, replacing any prior source.
// Human edits are permanent: a subsequent model UpdateFact will not overwrite them.
func (s *Store) Edit(ctx context.Context, projectID, memoryID, newContent string) error {
	if strings.TrimSpace(projectID) == "" {
		return fmt.Errorf("memory: edit: project_id required")
	}
	res, err := s.db.RawDB().ExecContext(ctx,
		`UPDATE memory_items
		 SET content = ?, source = 'human', updated_at = datetime('now')
		 WHERE id = ? AND project_id = ?`,
		newContent, memoryID, projectID,
	)
	if err != nil {
		return fmt.Errorf("memory: edit: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// MemoryFilter controls which facts are returned by Filter.
// Zero-value fields are treated as "no constraint".
type MemoryFilter struct {
	ProjectID string
	Scope     string
	AgentID   string
}

// GetByID returns a single non-forgotten memory item by its ID.
// Returns sql.ErrNoRows if the item does not exist or has been forgotten.
func (s *Store) GetByID(ctx context.Context, projectID, id string) (model.MemoryItem, error) {
	if strings.TrimSpace(projectID) == "" {
		return model.MemoryItem{}, fmt.Errorf("memory: get by id: project_id required")
	}
	var item model.MemoryItem
	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT id, project_id, agent_id, scope, content, source, COALESCE(provenance, ''),
		 COALESCE(confidence, 0), COALESCE(dedupe_key, ''), created_at, updated_at
		 FROM memory_items
		 WHERE id = ? AND project_id = ? AND forgotten_at IS NULL`,
		id, projectID,
	).Scan(
		&item.ID, &item.ProjectID, &item.AgentID, &item.Scope, &item.Content, &item.Source,
		&item.Provenance, &item.Confidence, &item.DedupeKey,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return model.MemoryItem{}, fmt.Errorf("memory: get by id: %w", err)
	}
	return item, nil
}

func (s *Store) getByDedupeKey(ctx context.Context, projectID, dedupeKey string) (model.MemoryItem, bool, error) {
	if strings.TrimSpace(projectID) == "" {
		return model.MemoryItem{}, false, fmt.Errorf("memory: get by dedupe key: project_id required")
	}
	if strings.TrimSpace(dedupeKey) == "" {
		return model.MemoryItem{}, false, fmt.Errorf("memory: get by dedupe key: dedupe_key required")
	}

	var item model.MemoryItem
	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT id, project_id, agent_id, scope, content, source, COALESCE(provenance, ''),
		 COALESCE(confidence, 0), COALESCE(dedupe_key, ''), created_at, updated_at
		 FROM memory_items
		 WHERE project_id = ? AND dedupe_key = ? AND forgotten_at IS NULL
		 ORDER BY updated_at DESC
		 LIMIT 1`,
		projectID, dedupeKey,
	).Scan(
		&item.ID, &item.ProjectID, &item.AgentID, &item.Scope, &item.Content, &item.Source,
		&item.Provenance, &item.Confidence, &item.DedupeKey,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return model.MemoryItem{}, false, nil
	}
	if err != nil {
		return model.MemoryItem{}, false, fmt.Errorf("memory: get by dedupe key: %w", err)
	}
	return item, true, nil
}

// Filter returns all non-forgotten facts matching the given filter.
func (s *Store) Filter(ctx context.Context, f MemoryFilter) ([]model.MemoryItem, error) {
	if strings.TrimSpace(f.ProjectID) == "" {
		return nil, fmt.Errorf("memory: filter: project_id required")
	}

	q := `SELECT id, project_id, agent_id, scope, content, source, COALESCE(provenance, ''),
		COALESCE(confidence, 0), COALESCE(dedupe_key, ''), created_at, updated_at
		FROM memory_items
		WHERE forgotten_at IS NULL AND project_id = ?`
	args := make([]any, 0, 3)
	args = append(args, f.ProjectID)

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
			&item.ID, &item.ProjectID, &item.AgentID, &item.Scope, &item.Content, &item.Source,
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
	page, err := s.SearchPage(ctx, SearchPageQuery{
		ProjectID: query.ProjectID,
		AgentID:   query.AgentID,
		Scope:     query.Scope,
		Keyword:   query.Keyword,
		Limit:     query.Limit,
	})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *Store) SearchPage(ctx context.Context, query SearchPageQuery) (SearchPage, error) {
	if strings.TrimSpace(query.ProjectID) == "" {
		return SearchPage{}, fmt.Errorf("memory: search page: project_id required")
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}

	cursor, hasCursor := parseMemoryCursor(query.Cursor)
	direction := query.Direction
	if direction != "prev" {
		direction = "next"
	}

	sqlQuery := `SELECT id, project_id, agent_id, scope, content, source, COALESCE(provenance, ''),
		COALESCE(confidence, 0), COALESCE(dedupe_key, ''), created_at, updated_at, updated_at
		FROM memory_items WHERE forgotten_at IS NULL AND project_id = ?`
	args := make([]any, 0, 9)
	args = append(args, query.ProjectID)

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
	if hasCursor {
		if direction == "prev" {
			sqlQuery += " AND (updated_at > ? OR (updated_at = ? AND id > ?))"
		} else {
			sqlQuery += " AND (updated_at < ? OR (updated_at = ? AND id < ?))"
		}
		args = append(args, cursor.UpdatedAt, cursor.UpdatedAt, cursor.ID)
	}

	if direction == "prev" {
		sqlQuery += " ORDER BY updated_at ASC, id ASC"
	} else {
		sqlQuery += " ORDER BY updated_at DESC, id DESC"
	}
	sqlQuery += " LIMIT ?"
	args = append(args, limit+1)

	rows, err := s.db.RawDB().QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return SearchPage{}, fmt.Errorf("memory: search page: %w", err)
	}
	defer rows.Close()

	pageRows := make([]memoryPageRow, 0, limit+1)
	for rows.Next() {
		var item model.MemoryItem
		var updatedAt string
		if err := rows.Scan(
			&item.ID,
			&item.ProjectID,
			&item.AgentID,
			&item.Scope,
			&item.Content,
			&item.Source,
			&item.Provenance,
			&item.Confidence,
			&item.DedupeKey,
			&item.CreatedAt,
			&item.UpdatedAt,
			&updatedAt,
		); err != nil {
			return SearchPage{}, fmt.Errorf("memory: scan page: %w", err)
		}
		pageRows = append(pageRows, memoryPageRow{Item: item, UpdatedAt: updatedAt})
	}
	if err := rows.Err(); err != nil {
		return SearchPage{}, fmt.Errorf("memory: iterate page: %w", err)
	}

	return finalizeMemoryPage(direction, hasCursor, limit, pageRows), nil
}

func (s *Store) UpsertWorkingSummary(ctx context.Context, runID, conversationID, projectID string) (model.SummaryRef, error) {
	if strings.TrimSpace(projectID) == "" {
		return model.SummaryRef{}, fmt.Errorf("memory: summarize: project_id required")
	}
	id := memGenerateID()
	now := time.Now().UTC()
	summary := fmt.Sprintf("Summary of conversation %s for run %s generated at %s", conversationID, runID, now.Format(time.RFC3339))

	ref := model.SummaryRef{
		ID:         id,
		ProjectID:  projectID,
		RunID:      runID,
		Content:    summary,
		TokenCount: len(summary) / 4,
	}

	payload, err := json.Marshal(map[string]any{
		"project_id":  ref.ProjectID,
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

func (s *Store) LoadContext(ctx context.Context, runID, projectID, agentID, scope string, limit int) (ContextView, error) {
	if strings.TrimSpace(projectID) == "" {
		return ContextView{}, fmt.Errorf("memory: load context: project_id required")
	}
	view := ContextView{
		Items: make([]model.MemoryItem, 0),
	}

	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT id, project_id, run_id, content, token_count
		 FROM run_summaries
		 WHERE run_id = ? AND project_id = ?`,
		runID, projectID,
	).Scan(
		&view.Summary.ID,
		&view.Summary.ProjectID,
		&view.Summary.RunID,
		&view.Summary.Content,
		&view.Summary.TokenCount,
	)
	if err != nil && err != sql.ErrNoRows {
		return ContextView{}, fmt.Errorf("memory: load summary: %w", err)
	}

	items, err := s.Search(ctx, model.MemoryQuery{
		ProjectID: projectID,
		AgentID:   agentID,
		Scope:     scope,
		Limit:     limit,
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

type memoryCursor struct {
	UpdatedAt string
	ID        string
}

type memoryPageRow struct {
	Item      model.MemoryItem
	UpdatedAt string
}

func parseMemoryCursor(raw string) (memoryCursor, bool) {
	if raw == "" {
		return memoryCursor{}, false
	}
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return memoryCursor{}, false
	}
	return memoryCursor{UpdatedAt: parts[0], ID: parts[1]}, true
}

func encodeMemoryCursor(updatedAt, id string) string {
	if updatedAt == "" || id == "" {
		return ""
	}
	return updatedAt + "|" + id
}

func finalizeMemoryPage(direction string, hasCursor bool, limit int, rows []memoryPageRow) SearchPage {
	hasExtra := len(rows) > limit
	if hasExtra {
		rows = rows[:limit]
	}
	if direction == "prev" {
		for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
			rows[left], rows[right] = rows[right], rows[left]
		}
	}

	page := SearchPage{
		Items: make([]model.MemoryItem, 0, len(rows)),
	}
	for _, row := range rows {
		page.Items = append(page.Items, row.Item)
	}
	if len(rows) > 0 {
		first := rows[0]
		last := rows[len(rows)-1]
		page.PrevCursor = encodeMemoryCursor(first.UpdatedAt, first.Item.ID)
		page.NextCursor = encodeMemoryCursor(last.UpdatedAt, last.Item.ID)
	}

	switch direction {
	case "prev":
		page.HasPrev = hasExtra
		page.HasNext = hasCursor
	default:
		page.HasPrev = hasCursor
		page.HasNext = hasExtra
	}

	return page
}
