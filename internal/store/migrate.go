package store

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func Migrate(db *DB) error {
	_, err := db.db.Exec(`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return fmt.Errorf("migrate: create settings: %w", err)
	}

	currentVersion, err := SchemaVersion(db)
	if err != nil {
		currentVersion = 0
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("migrate: read dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		parts := strings.SplitN(entry.Name(), "_", 2)
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		if version <= currentVersion {
			continue
		}

		sqlBytes, err := migrationFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", entry.Name(), err)
		}

		if _, err := db.db.Exec(string(sqlBytes)); err != nil {
			return fmt.Errorf("migrate: exec %s: %w", entry.Name(), err)
		}

		_, err = db.db.Exec(
			`INSERT INTO settings (key, value, updated_at)
			 VALUES ('schema_version', ?, datetime('now'))
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			strconv.Itoa(version),
		)
		if err != nil {
			return fmt.Errorf("migrate: update version: %w", err)
		}

		currentVersion = version
	}

	if err := ensureProjectSchema(db); err != nil {
		return err
	}
	if err := ensureAuthSchema(db); err != nil {
		return err
	}

	return nil
}

func SchemaVersion(db *DB) (int, error) {
	var value string
	err := db.db.QueryRow("SELECT value FROM settings WHERE key = 'schema_version'").Scan(&value)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	version, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("migrate: parse version %q: %w", value, err)
	}
	return version, nil
}

func ensureProjectSchema(db *DB) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			primary_path TEXT NOT NULL DEFAULT '',
			roots_json BLOB NOT NULL DEFAULT '[]',
			policy_json BLOB NOT NULL DEFAULT '{}',
			source TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			last_used_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_projects_last_used_at ON projects(last_used_at, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_project_id_status_updated_at ON runs(project_id, status, updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_items_project_id_agent_id_scope ON memory_items(project_id, agent_id, scope)`,
		`CREATE INDEX IF NOT EXISTS idx_run_summaries_project_id_run_id ON run_summaries(project_id, run_id)`,
	} {
		if _, err := db.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: ensure project schema: %w", err)
		}
	}

	return nil
}


func ensureAuthSchema(db *DB) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS auth_devices (
			id TEXT PRIMARY KEY,
			token_hash TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			browser TEXT NOT NULL DEFAULT '',
			platform TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			last_seen_at DATETIME NOT NULL DEFAULT (datetime('now')),
			last_ip TEXT NOT NULL DEFAULT '',
			last_user_agent TEXT NOT NULL DEFAULT '',
			blocked_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS auth_sessions (
			id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			last_seen_at DATETIME NOT NULL DEFAULT (datetime('now')),
			expires_at DATETIME NOT NULL,
			revoked_at DATETIME,
			revoke_reason TEXT NOT NULL DEFAULT '',
			FOREIGN KEY (device_id) REFERENCES auth_devices(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_auth_devices_last_seen_at ON auth_devices(last_seen_at)`,
		`CREATE INDEX IF NOT EXISTS idx_auth_sessions_token_hash ON auth_sessions(token_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_auth_sessions_device_id_revoked_expires_at ON auth_sessions(device_id, revoked_at, expires_at, last_seen_at)`,
		`CREATE INDEX IF NOT EXISTS idx_auth_sessions_expires_at_revoked_at ON auth_sessions(expires_at, revoked_at)`,
	} {
		if _, err := db.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: ensure auth schema: %w", err)
		}
	}
	return nil
}
