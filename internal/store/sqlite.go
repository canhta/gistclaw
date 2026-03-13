// internal/store/sqlite.go
package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("store: not found")

// HITLRecord is a row from hitl_pending.
type HITLRecord struct {
	ID       string
	Agent    string
	ToolName string
	Status   string
}

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path, enables WAL mode,
// and applies the schema.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("store: open %q: %w", path, err)
	}
	// Single writer; avoid "database is locked" under concurrent reads.
	db.SetMaxOpenConns(1)

	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return nil, fmt.Errorf("store: read schema: %w", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		return nil, fmt.Errorf("store: apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }

// Ping verifies a table exists by selecting from it.
func (s *Store) Ping(table string) error {
	// table name is only ever passed from internal code, not user input — safe.
	_, err := s.db.Exec("SELECT 1 FROM " + table + " LIMIT 0")
	return err
}

// PurgeStartup runs all startup purge operations:
//   - sessions older than sessionTTL
//   - hitl_pending records older than hitlTTL
//   - cost_daily rows older than costTTL
func (s *Store) PurgeStartup(sessionTTL, hitlTTL, costTTL time.Duration) error {
	if err := s.PurgeSessions(time.Now().Add(-sessionTTL)); err != nil {
		return err
	}
	if _, err := s.db.Exec(
		`DELETE FROM hitl_pending WHERE created_at < ?`,
		time.Now().Add(-hitlTTL).UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("store: purge hitl_pending: %w", err)
	}
	if _, err := s.db.Exec(
		`DELETE FROM cost_daily WHERE date < ?`,
		time.Now().Add(-costTTL).UTC().Format("2006-01-02"),
	); err != nil {
		return fmt.Errorf("store: purge cost_daily: %w", err)
	}
	return nil
}

// PurgeSessions deletes sessions with created_at before cutoff.
func (s *Store) PurgeSessions(cutoff time.Time) error {
	_, err := s.db.Exec(
		`DELETE FROM sessions WHERE created_at < ?`,
		cutoff.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("store: purge sessions: %w", err)
	}
	return nil
}

// InsertSession inserts a new session record.
func (s *Store) InsertSession(id, agent, status, prompt string, createdAt time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, agent, status, prompt, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, agent, status, prompt, createdAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("store: insert session %q: %w", id, err)
	}
	return nil
}

// CountSessions returns the total number of session rows.
func (s *Store) CountSessions() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n)
	return n, err
}

// GetLastUpdateID returns the last seen update_id for a channel, or 0 if none.
func (s *Store) GetLastUpdateID(channelID string) (int64, error) {
	var id int64
	err := s.db.QueryRow(
		`SELECT last_update_id FROM channel_state WHERE channel_id = ?`, channelID,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// SetLastUpdateID upserts the last_update_id for a channel.
func (s *Store) SetLastUpdateID(channelID string, updateID int64) error {
	_, err := s.db.Exec(
		`INSERT INTO channel_state (channel_id, last_update_id) VALUES (?, ?)
		 ON CONFLICT(channel_id) DO UPDATE SET last_update_id = excluded.last_update_id`,
		channelID, updateID,
	)
	if err != nil {
		return fmt.Errorf("store: set last update ID for %q: %w", channelID, err)
	}
	return nil
}

// GetProviderCredentials returns the stored credential JSON for provider.
// Returns ErrNotFound if no credential is stored.
func (s *Store) GetProviderCredentials(provider string) (string, error) {
	var data string
	err := s.db.QueryRow(
		`SELECT data FROM provider_credentials WHERE provider = ?`, provider,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return data, err
}

// SetProviderCredentials upserts the credential JSON for provider.
func (s *Store) SetProviderCredentials(provider, data string) error {
	_, err := s.db.Exec(
		`INSERT INTO provider_credentials (provider, data, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(provider) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`,
		provider, data,
	)
	if err != nil {
		return fmt.Errorf("store: set provider credentials for %q: %w", provider, err)
	}
	return nil
}

// UpsertCostDaily sets (not adds) the total_usd for date.
func (s *Store) UpsertCostDaily(date string, totalUSD float64) error {
	_, err := s.db.Exec(
		`INSERT INTO cost_daily (date, total_usd) VALUES (?, ?)
		 ON CONFLICT(date) DO UPDATE SET total_usd = excluded.total_usd`,
		date, totalUSD,
	)
	return err
}

// GetCostDaily returns the total_usd for date, or 0 if no row exists.
func (s *Store) GetCostDaily(date string) (float64, error) {
	var total float64
	err := s.db.QueryRow(`SELECT total_usd FROM cost_daily WHERE date = ?`, date).Scan(&total)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return total, err
}

// InsertHITLPending inserts a new pending HITL record with status "pending".
func (s *Store) InsertHITLPending(id, agent, toolName string) error {
	_, err := s.db.Exec(
		`INSERT INTO hitl_pending (id, agent, tool_name, status) VALUES (?, ?, ?, 'pending')`,
		id, agent, toolName,
	)
	if err != nil {
		return fmt.Errorf("store: insert HITL pending %q: %w", id, err)
	}
	return nil
}

// ListPendingHITL returns all hitl_pending rows with status "pending".
func (s *Store) ListPendingHITL() ([]HITLRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, agent, COALESCE(tool_name,'') FROM hitl_pending WHERE status = 'pending'`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	var out []HITLRecord
	for rows.Next() {
		var r HITLRecord
		r.Status = "pending"
		if err := rows.Scan(&r.ID, &r.Agent, &r.ToolName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ResolveHITL updates the status of a hitl_pending record.
// Returns ErrNotFound if no record exists with the given id.
func (s *Store) ResolveHITL(id, status string) error {
	res, err := s.db.Exec(
		`UPDATE hitl_pending SET status = ?, resolved_at = datetime('now') WHERE id = ?`,
		status, id,
	)
	if err != nil {
		return fmt.Errorf("store: resolve HITL %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("store: resolve HITL %q: %w", id, ErrNotFound)
	}
	return nil
}

// HistoryMessage is a persisted plain-chat turn (user or assistant only).
// Tool call/result messages are excluded — they are execution artifacts, not conversational memory.
type HistoryMessage struct {
	Role    string // "user" or "assistant"
	Content string
}

// SaveMessage persists a single conversation turn for the given chatID.
// Errors are non-fatal at the call site — history persistence is best-effort.
func (s *Store) SaveMessage(chatID int64, role, content string) error {
	_, err := s.db.Exec(
		`INSERT INTO messages (chat_id, role, content) VALUES (?, ?, ?)`,
		chatID, role, content,
	)
	if err != nil {
		return fmt.Errorf("store: save message: %w", err)
	}
	return nil
}

// GetHistory returns up to limit messages for chatID in chronological order
// (oldest first, ready for direct injection into a message slice).
// The caller should pass ConversationWindowTurns*2 as limit.
func (s *Store) GetHistory(chatID int64, limit int) ([]HistoryMessage, error) {
	rows, err := s.db.Query(
		`SELECT role, content FROM messages
		 WHERE chat_id = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT ?`,
		chatID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("store: get history: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []HistoryMessage
	for rows.Next() {
		var m HistoryMessage
		if err := rows.Scan(&m.Role, &m.Content); err != nil {
			return nil, fmt.Errorf("store: get history scan: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: get history: %w", err)
	}

	// Query returns newest-first; reverse to inject oldest-first.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// JobRow mirrors the jobs table. Used by scheduler.Service to avoid a circular import.
type JobRow struct {
	ID             string
	Kind           string // "at" | "every" | "cron"
	Target         string // agent.Kind.String(): "opencode" | "claudecode" | "chat"
	Prompt         string
	Schedule       string
	NextRunAt      time.Time
	LastRunAt      *time.Time
	Enabled        bool
	DeleteAfterRun bool
	CreatedAt      time.Time
}

// InsertJob inserts a new job row. Returns error on duplicate ID.
func (s *Store) InsertJob(j JobRow) error {
	_, err := s.db.Exec(`
		INSERT INTO jobs (id, kind, target, prompt, schedule, next_run_at, last_run_at,
		                  enabled, delete_after_run, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.ID, j.Kind, j.Target, j.Prompt, j.Schedule,
		j.NextRunAt.UTC().Format(time.RFC3339),
		nullableTime(j.LastRunAt),
		boolToInt(j.Enabled),
		boolToInt(j.DeleteAfterRun),
		j.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("store: insert job %q: %w", j.ID, err)
	}
	return nil
}

// ListEnabledJobsDueBefore returns all enabled jobs with next_run_at <= t.
func (s *Store) ListEnabledJobsDueBefore(t time.Time) ([]JobRow, error) {
	rows, err := s.db.Query(`
		SELECT id, kind, target, prompt, schedule, next_run_at, last_run_at,
		       enabled, delete_after_run, created_at
		FROM jobs
		WHERE enabled = 1 AND next_run_at <= ?`,
		t.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("store: list enabled jobs due before: %w", err)
	}
	defer rows.Close()
	result, err := scanJobRows(rows)
	if err != nil {
		return nil, fmt.Errorf("store: list enabled jobs due before: %w", err)
	}
	return result, nil
}

// ListAllJobs returns all job rows regardless of status.
func (s *Store) ListAllJobs() ([]JobRow, error) {
	rows, err := s.db.Query(`
		SELECT id, kind, target, prompt, schedule, next_run_at, last_run_at,
		       enabled, delete_after_run, created_at
		FROM jobs ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list all jobs: %w", err)
	}
	defer rows.Close()
	result, err := scanJobRows(rows)
	if err != nil {
		return nil, fmt.Errorf("store: list all jobs: %w", err)
	}
	return result, nil
}

// UpdateJobAfterRun sets last_run_at and next_run_at for the given job.
func (s *Store) UpdateJobAfterRun(id string, lastRunAt time.Time, nextRunAt time.Time) error {
	res, err := s.db.Exec(`
		UPDATE jobs SET last_run_at = ?, next_run_at = ? WHERE id = ?`,
		lastRunAt.UTC().Format(time.RFC3339),
		nextRunAt.UTC().Format(time.RFC3339),
		id,
	)
	if err != nil {
		return fmt.Errorf("store: update job after run %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("store: update job after run %q: %w", id, ErrNotFound)
	}
	return nil
}

// DeleteJob removes a job by ID.
func (s *Store) DeleteJob(id string) error {
	res, err := s.db.Exec(`DELETE FROM jobs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete job %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("store: delete job %q: %w", id, ErrNotFound)
	}
	return nil
}

// UpdateJobField sets a single column value for the given job ID.
// field must be a valid column name; value must be SQLite-compatible.
func (s *Store) UpdateJobField(id string, field string, value any) error {
	// Allowlist to prevent SQL injection from LLM-supplied field names.
	allowed := map[string]bool{
		"enabled": true, "next_run_at": true, "delete_after_run": true,
	}
	if !allowed[field] {
		return fmt.Errorf("store: UpdateJobField: field %q not allowed", field)
	}
	result, err := s.db.Exec(fmt.Sprintf(`UPDATE jobs SET %s = ? WHERE id = ?`, field), value, id)
	if err != nil {
		return fmt.Errorf("store: update job field %q on %q: %w", field, id, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: UpdateJobField: rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountMessages returns the number of message rows for chatID.
func (s *Store) CountMessages(chatID int64) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE chat_id = ?`, chatID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: count messages: %w", err)
	}
	return count, nil
}

// ReplaceHistory deletes all message rows for chatID and inserts rows in a single
// transaction. rows is ordered oldest-first. Passing nil or empty rows deletes all.
func (s *Store) ReplaceHistory(chatID int64, rows []HistoryMessage) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: replace history: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck — no-op after Commit

	if _, err := tx.Exec(`DELETE FROM messages WHERE chat_id = ?`, chatID); err != nil {
		return fmt.Errorf("store: replace history: delete: %w", err)
	}
	for _, row := range rows {
		if _, err := tx.Exec(
			`INSERT INTO messages (chat_id, role, content) VALUES (?, ?, ?)`,
			chatID, row.Role, row.Content,
		); err != nil {
			return fmt.Errorf("store: replace history: insert: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: replace history: commit: %w", err)
	}
	return nil
}

// --- helpers ---

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func scanJobRows(rows *sql.Rows) ([]JobRow, error) {
	var result []JobRow
	for rows.Next() {
		var r JobRow
		var nextRunAt, createdAt string
		var lastRunAt *string
		var enabled, deleteAfterRun int
		if err := rows.Scan(
			&r.ID, &r.Kind, &r.Target, &r.Prompt, &r.Schedule,
			&nextRunAt, &lastRunAt,
			&enabled, &deleteAfterRun, &createdAt,
		); err != nil {
			return nil, err
		}
		r.Enabled = enabled == 1
		r.DeleteAfterRun = deleteAfterRun == 1
		if t, err := time.Parse(time.RFC3339, nextRunAt); err == nil {
			r.NextRunAt = t
		}
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			r.CreatedAt = t
		}
		if lastRunAt != nil {
			if t, err := time.Parse(time.RFC3339, *lastRunAt); err == nil {
				r.LastRunAt = &t
			}
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
