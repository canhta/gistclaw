package store

import "testing"

func TestMigrate_ExpectedMigrationFiles(t *testing.T) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	want := []string{"001_init.sql", "003_scheduler.sql"}
	if len(entries) != len(want) {
		t.Fatalf("expected %d migration files, got %d", len(want), len(entries))
	}
	for i, name := range want {
		if entries[i].Name() != name {
			t.Errorf("migration[%d]: expected %q, got %q", i, name, entries[i].Name())
		}
	}
}

func TestMigration003_SchedulesTableExists(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// schedules table must exist and accept a zero-row SELECT.
	rows, err := db.db.Query("SELECT id FROM schedules LIMIT 0")
	if err != nil {
		t.Fatalf("schedules table missing after migration 003: %v", err)
	}
	rows.Close()

	// Schema version must be at least 3.
	ver, err := SchemaVersion(db)
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if ver < 3 {
		t.Fatalf("expected schema version >= 3, got %d", ver)
	}
}

func TestMigrate_FreshDB(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	err = Migrate(db)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	ver, err := SchemaVersion(db)
	if err != nil {
		t.Fatalf("SchemaVersion failed: %v", err)
	}
	if ver != 3 {
		t.Fatalf("expected schema version 3, got %d", ver)
	}

	tables := []string{
		"conversations", "events", "runs", "delegations", "tool_calls",
		"approvals", "receipts", "memory_items", "outbound_intents",
		"settings", "run_summaries",
	}
	for _, table := range tables {
		var name string
		err := db.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}

	indexes := []string{
		"idx_events_run_id_created_at",
		"idx_runs_conversation_id_status",
		"idx_delegations_parent_run_id_status",
		"idx_approvals_run_id_status",
		"idx_memory_items_agent_id_scope",
	}
	for _, index := range indexes {
		var name string
		err := db.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='index' AND name=?",
			index,
		).Scan(&name)
		if err != nil {
			t.Errorf("index %q not found: %v", index, err)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("first Migrate failed: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate failed: %v", err)
	}

	ver, err := SchemaVersion(db)
	if err != nil {
		t.Fatalf("SchemaVersion failed: %v", err)
	}
	if ver != 3 {
		t.Fatalf("expected schema version 3, got %d", ver)
	}
}

func TestMigrate_WALEnabled(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/wal_test.db"

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	var journalMode string
	err = db.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("querying journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("expected WAL mode, got %q", journalMode)
	}
}

func TestMigrate_EnforcesSingleActiveRootRunPerConversation(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	_, err = db.db.Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
		 VALUES ('run-1', 'conv-1', 'agent-a', 'active', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert first active root run: %v", err)
	}

	_, err = db.db.Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
		 VALUES ('run-2', 'conv-1', 'agent-b', 'active', datetime('now'), datetime('now'))`,
	)
	if err == nil {
		t.Fatal("expected unique constraint for second active root run, got nil")
	}
}
