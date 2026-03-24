package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var ErrDiskFull = fmt.Errorf("store: disk full")

// sqliteCodeErr is the interface satisfied by modernc.org/sqlite's Error type.
type sqliteCodeErr interface {
	Code() int
}

// IsSQLiteFull reports whether err (or any error in its chain) is a SQLite
// SQLITE_FULL error (code 13).
func IsSQLiteFull(err error) bool {
	if err == nil {
		return false
	}
	var ce sqliteCodeErr
	return errors.As(err, &ce) && ce.Code() == 13 // SQLITE_FULL
}

// IsSQLiteConstraintUnique reports whether err (or any wrapped error in its
// chain) is a SQLite uniqueness constraint violation.
func IsSQLiteConstraintUnique(err error) bool {
	if err == nil {
		return false
	}
	var ce sqliteCodeErr
	if !errors.As(err, &ce) {
		return false
	}
	switch ce.Code() {
	case 19, 1555, 2067:
		return true
	default:
		return false
	}
}

type DB struct {
	db *sql.DB
}

func Open(path string) (*DB, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("store: mkdir %q: %w", filepath.Dir(path), err)
		}
	}

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
		if IsSQLiteFull(err) {
			return fmt.Errorf("store: commit tx: %w: %w", ErrDiskFull, err)
		}
		return fmt.Errorf("store: commit tx: %w", err)
	}

	return nil
}

func (d *DB) Close() error {
	return d.db.Close()
}
