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
