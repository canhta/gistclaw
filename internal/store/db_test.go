package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
)

func TestDB_OpenAndPragmas(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	var journalMode string
	err = db.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("querying journal_mode: %v", err)
	}
	t.Logf("journal_mode: %s", journalMode)

	var fk int
	err = db.db.QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if err != nil {
		t.Fatalf("querying foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("expected foreign_keys=1, got %d", fk)
	}

	var bt int
	err = db.db.QueryRow("PRAGMA busy_timeout").Scan(&bt)
	if err != nil {
		t.Fatalf("querying busy_timeout: %v", err)
	}
	if bt != 5000 {
		t.Fatalf("expected busy_timeout=5000, got %d", bt)
	}
}

func TestDB_OpenAndPragmas_FileDB(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.db"

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	var journalMode string
	err = db.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("querying journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("expected journal_mode=wal, got %q", journalMode)
	}
}

func TestDB_ConcurrentReadWrite(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/concurrent.db"

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	_, err = db.db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		err := db.Tx(context.Background(), func(tx *sql.Tx) error {
			_, err := tx.Exec("INSERT INTO test (val) VALUES ('hello')")
			return err
		})
		if err != nil {
			errCh <- err
		}
	}()
	go func() {
		defer wg.Done()
		err := db.Tx(context.Background(), func(tx *sql.Tx) error {
			rows, err := tx.Query("SELECT count(*) FROM test")
			if err != nil {
				return err
			}
			defer rows.Close()
			return nil
		})
		if err != nil {
			errCh <- err
		}
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent operation failed: %v", err)
	}
}

func TestDB_Tx_RollsBackOnError(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	_, err = db.db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	testErr := fmt.Errorf("intentional error")
	err = db.Tx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test (val) VALUES ('should_not_persist')")
		if err != nil {
			return err
		}
		return testErr
	})
	if err != testErr {
		t.Fatalf("expected testErr, got %v", err)
	}

	var count int
	err = db.db.QueryRow("SELECT count(*) FROM test").Scan(&count)
	if err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 rows after rollback, got %d", count)
	}
}
