package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

var ErrDiskFull = fmt.Errorf("store: disk full")

type DB struct {
	db *sql.DB
}

func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open %q: %w", path, err)
	}

	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("store: pragma %q: %w", pragma, err)
		}
	}

	return &DB{db: db}, nil
}

func (d *DB) RawDB() *sql.DB {
	return d.db
}

func (d *DB) Tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin tx: %w", err)
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit tx: %w", err)
	}

	return nil
}

func (d *DB) Close() error {
	return d.db.Close()
}
