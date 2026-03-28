package store

import (
	"database/sql"
	"testing"
)

func TestMigrate_ExpectedMigrationFiles(t *testing.T) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	want := []string{"001_init.sql"}
	if len(entries) != len(want) {
		t.Fatalf("expected %d migration files, got %d", len(want), len(entries))
	}
	for i, name := range want {
		if entries[i].Name() != name {
			t.Errorf("migration[%d]: expected %q, got %q", i, name, entries[i].Name())
		}
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
	if ver != 1 {
		t.Fatalf("expected schema version 1, got %d", ver)
	}

	tables := []string{
		"conversations", "events", "runs", "sessions", "session_messages",
		"session_bindings", "inbound_receipts", "tool_calls", "approvals", "receipts",
		"memory_items", "outbound_intents", "settings", "connector_threads", "run_summaries",
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
		"idx_sessions_conversation_id_status",
		"idx_session_messages_session_id_created_at",
		"idx_session_bindings_conversation_id_thread_id_status",
		"idx_session_bindings_session_id_status_created_at",
		"idx_inbound_receipts_conversation_source_message",
		"idx_approvals_run_id_status",
		"idx_memory_items_project_id_agent_id_scope",
		"idx_connector_threads_connector_account_last_message_at",
		"idx_run_summaries_project_id_run_id",
		"idx_runs_session_id_status_updated_at",
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
	if ver != 1 {
		t.Fatalf("expected schema version 1, got %d", ver)
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

func TestMigrateCreatesSessionRuntimeTables(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	for _, table := range []string{
		"conversations",
		"events",
		"runs",
		"sessions",
		"session_messages",
		"session_bindings",
		"inbound_receipts",
		"tool_calls",
		"approvals",
		"receipts",
	} {
		var name string
		err := db.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			table,
		).Scan(&name)
		if err != nil {
			t.Fatalf("expected table %q to exist: %v", table, err)
		}
	}

}

func TestMigrate_UsesHostExecutionSchema(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	assertTableHasColumns(t, db, "projects", "primary_path", "roots_json", "policy_json")
	assertTableHasColumns(t, db, "runs", "cwd", "authority_json")
	assertTableHasColumns(t, db, "approvals", "binding_json")
	assertTableHasColumns(t, db, "schedules", "project_id", "cwd", "authority_json")

	for _, key := range []string{"storage_root", "approval_mode", "host_access_mode"} {
		if hasSettingKey(t, db, key) {
			t.Fatalf("expected settings key %q to be config-owned and absent from migration defaults", key)
		}
	}
}

func hasSettingKey(t *testing.T, db *DB, key string) bool {
	t.Helper()

	var count int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM settings WHERE key = ?`, key).Scan(&count); err != nil {
		t.Fatalf("count settings %q: %v", key, err)
	}
	return count > 0
}

func assertTableHasColumns(t *testing.T, db *DB, tableName string, columns ...string) {
	t.Helper()
	have := tableColumns(t, db, tableName)
	for _, column := range columns {
		if !have[column] {
			t.Fatalf("expected table %q to contain column %q", tableName, column)
		}
	}
}

func assertTableOmitsColumns(t *testing.T, db *DB, tableName string, columns ...string) {
	t.Helper()
	have := tableColumns(t, db, tableName)
	for _, column := range columns {
		if have[column] {
			t.Fatalf("expected table %q to omit column %q", tableName, column)
		}
	}
}

func tableColumns(t *testing.T, db *DB, tableName string) map[string]bool {
	t.Helper()

	rows, err := db.db.Query("PRAGMA table_info(" + tableName + ")")
	if err != nil {
		t.Fatalf("table info %q: %v", tableName, err)
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var (
			cid          int
			name         string
			columnType   string
			notNull      int
			defaultValue sql.NullString
			primaryKey   int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan table info %q: %v", tableName, err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table info rows %q: %v", tableName, err)
	}
	return columns
}
