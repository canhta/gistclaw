package store

import "testing"

func TestMigrateCreatesAuthTablesAndIndexes(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	for _, table := range []string{"auth_devices", "auth_sessions"} {
		var name string
		if err := db.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			table,
		).Scan(&name); err != nil {
			t.Fatalf("expected table %q to exist: %v", table, err)
		}
	}

	for _, index := range []string{
		"idx_auth_devices_last_seen_at",
		"idx_auth_sessions_token_hash",
		"idx_auth_sessions_device_id_revoked_expires_at",
		"idx_auth_sessions_expires_at_revoked_at",
	} {
		var name string
		if err := db.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='index' AND name=?",
			index,
		).Scan(&name); err != nil {
			t.Fatalf("expected index %q to exist: %v", index, err)
		}
	}
}

func TestMigrateRecreatesAuthTablesForExistingSchemaVersion(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("first Migrate failed: %v", err)
	}

	if _, err := db.db.Exec(`DROP TABLE auth_sessions`); err != nil {
		t.Fatalf("drop auth_sessions: %v", err)
	}
	if _, err := db.db.Exec(`DROP TABLE auth_devices`); err != nil {
		t.Fatalf("drop auth_devices: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate failed: %v", err)
	}

	for _, table := range []string{"auth_devices", "auth_sessions"} {
		var name string
		if err := db.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			table,
		).Scan(&name); err != nil {
			t.Fatalf("expected table %q to be recreated: %v", table, err)
		}
	}
}
