package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	sqlite "modernc.org/sqlite"

	"github.com/canhta/gistclaw/internal/store"
)

// sqliteBackuper is the interface exposed by modernc.org/sqlite's conn type.
type sqliteBackuper interface {
	NewBackup(dstURI string) (*sqlite.Backup, error)
}

// runBackup copies the SQLite database at srcPath to a timestamped .db.bak
// file in the same directory using the SQLite online backup API.
func runBackup(args []string, stdout, stderr io.Writer) int {
	srcPath, err := parseFlag(args, "--db")
	if err != nil || srcPath == "" {
		fmt.Fprintln(stderr, "Usage: gistclaw backup --db <path>")
		return 1
	}

	db, err := store.Open(srcPath)
	if err != nil {
		fmt.Fprintf(stderr, "backup: open source db: %v\n", err)
		return 1
	}
	defer db.Close()

	// WAL checkpoint before backup to flush any pending WAL data.
	if _, err := db.RawDB().Exec("PRAGMA wal_checkpoint(FULL)"); err != nil {
		fmt.Fprintf(stderr, "backup: wal_checkpoint: %v\n", err)
		return 1
	}

	// Build destination path: insert timestamp before .db extension.
	dstPath := store.BackupPathForTime(srcPath, time.Now().UTC())

	// Use SQLite online backup API via conn.Raw.
	rawConn, err := db.RawDB().Conn(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "backup: get conn: %v\n", err)
		return 1
	}
	defer rawConn.Close()

	if err := rawConn.Raw(func(driverConn any) error {
		bp, ok := driverConn.(sqliteBackuper)
		if !ok {
			return fmt.Errorf("driver does not support backup API")
		}
		bck, err := bp.NewBackup(dstPath)
		if err != nil {
			return fmt.Errorf("new backup: %w", err)
		}
		for more := true; more; {
			more, err = bck.Step(-1)
			if err != nil {
				return fmt.Errorf("backup step: %w", err)
			}
		}
		return bck.Finish()
	}); err != nil {
		fmt.Fprintf(stderr, "backup: %v\n", err)
		return 1
	}

	absDst, err := filepath.Abs(dstPath)
	if err != nil {
		absDst = dstPath
	}
	fmt.Fprintln(stdout, absDst)
	return 0
}

// parseFlag extracts a named flag value (--name value or --name=value) from args.
func parseFlag(args []string, name string) (string, error) {
	for i := 0; i < len(args); i++ {
		// --flag=value form: no lookahead needed.
		if len(args[i]) > len(name)+1 && args[i][:len(name)+1] == name+"=" {
			return args[i][len(name)+1:], nil
		}
		// --flag value form: requires a following element.
		if args[i] == name {
			if i+1 < len(args) {
				return args[i+1], nil
			}
			return "", nil
		}
	}
	return "", nil
}
