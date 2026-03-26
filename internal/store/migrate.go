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
			workspace_root TEXT NOT NULL UNIQUE,
			source TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			last_used_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_projects_last_used_at ON projects(last_used_at, created_at)`,
	} {
		if _, err := db.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: ensure project schema: %w", err)
		}
	}

	hasProjectID, err := tableHasColumn(db, "runs", "project_id")
	if err != nil {
		return err
	}
	if !hasProjectID {
		if _, err := db.db.Exec(`ALTER TABLE runs ADD COLUMN project_id TEXT`); err != nil {
			return fmt.Errorf("migrate: add runs.project_id: %w", err)
		}
	}
	if _, err := db.db.Exec(`CREATE INDEX IF NOT EXISTS idx_runs_project_id_status_updated_at ON runs(project_id, status, updated_at)`); err != nil {
		return fmt.Errorf("migrate: create runs.project_id index: %w", err)
	}
	if _, err := db.db.Exec(`UPDATE runs
		SET project_id = (
			SELECT projects.id
			FROM projects
			WHERE projects.workspace_root = runs.workspace_root
		)
		WHERE COALESCE(project_id, '') = ''
		  AND COALESCE(workspace_root, '') != ''`); err != nil {
		return fmt.Errorf("migrate: backfill runs.project_id: %w", err)
	}

	return nil
}

func tableHasColumn(db *DB, tableName, columnName string) (bool, error) {
	rows, err := db.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false, fmt.Errorf("migrate: table info %s: %w", tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("migrate: scan table info %s: %w", tableName, err)
		}
		if name == columnName {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("migrate: table info rows %s: %w", tableName, err)
	}
	return false, nil
}
