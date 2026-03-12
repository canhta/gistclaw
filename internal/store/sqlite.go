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
