// internal/store/sqlite_test.go
package store_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenCreatesSchema(t *testing.T) {
	s := newTestStore(t)
	// If schema was applied, we can query each table without error.
	tables := []string{"sessions", "hitl_pending", "cost_daily", "channel_state", "provider_credentials", "jobs"}
	for _, table := range tables {
		if err := s.Ping(table); err != nil {
			t.Errorf("table %q not accessible: %v", table, err)
		}
	}
}

func TestPurgeSessionsOlderThan(t *testing.T) {
	s := newTestStore(t)

	// Insert two sessions: one old, one recent.
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := s.InsertSession("old-id", "opencode", "active", "prompt", oldTime); err != nil {
		t.Fatalf("InsertSession old: %v", err)
	}
	if err := s.InsertSession("new-id", "opencode", "active", "prompt", time.Now()); err != nil {
		t.Fatalf("InsertSession new: %v", err)
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	if err := s.PurgeSessions(cutoff); err != nil {
		t.Fatalf("PurgeSessions: %v", err)
	}

	remaining, err := s.CountSessions()
	if err != nil {
		t.Fatalf("CountSessions: %v", err)
	}
	if remaining != 1 {
		t.Errorf("after purge: got %d sessions, want 1", remaining)
	}
}

func TestChannelStateUpdateID(t *testing.T) {
	s := newTestStore(t)

	// Initially no record — GetLastUpdateID returns 0.
	id, err := s.GetLastUpdateID("telegram:123")
	if err != nil {
		t.Fatalf("GetLastUpdateID: %v", err)
	}
	if id != 0 {
		t.Errorf("initial update ID: got %d, want 0", id)
	}

	if err := s.SetLastUpdateID("telegram:123", 42); err != nil {
		t.Fatalf("SetLastUpdateID: %v", err)
	}

	id, err = s.GetLastUpdateID("telegram:123")
	if err != nil {
		t.Fatalf("GetLastUpdateID after set: %v", err)
	}
	if id != 42 {
		t.Errorf("after set: got %d, want 42", id)
	}
}

func TestProviderCredentials(t *testing.T) {
	s := newTestStore(t)

	_, err := s.GetProviderCredentials("codex")
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound for missing credential, got %v", err)
	}

	if err := s.SetProviderCredentials("codex", `{"access_token":"tok"}`); err != nil {
		t.Fatalf("SetProviderCredentials: %v", err)
	}

	data, err := s.GetProviderCredentials("codex")
	if err != nil {
		t.Fatalf("GetProviderCredentials after set: %v", err)
	}
	if data != `{"access_token":"tok"}` {
		t.Errorf("unexpected credential data: %q", data)
	}
}

func TestCostDailyUpsert(t *testing.T) {
	s := newTestStore(t)

	date := "2026-03-12"
	if err := s.UpsertCostDaily(date, 1.5); err != nil {
		t.Fatalf("UpsertCostDaily: %v", err)
	}
	if err := s.UpsertCostDaily(date, 0.5); err != nil {
		t.Fatalf("UpsertCostDaily second call: %v", err)
	}
	total, err := s.GetCostDaily(date)
	if err != nil {
		t.Fatalf("GetCostDaily: %v", err)
	}
	// Each call sets the total (not adds), so last write wins.
	if total != 0.5 {
		t.Errorf("cost daily: got %v, want 0.5", total)
	}
}

func TestHITLPending(t *testing.T) {
	s := newTestStore(t)

	if err := s.InsertHITLPending("perm_001", "opencode", "write_file"); err != nil {
		t.Fatalf("InsertHITLPending: %v", err)
	}

	pending, err := s.ListPendingHITL()
	if err != nil {
		t.Fatalf("ListPendingHITL: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "perm_001" {
		t.Errorf("ListPendingHITL: got %v", pending)
	}

	if err := s.ResolveHITL("perm_001", "auto_rejected"); err != nil {
		t.Fatalf("ResolveHITL: %v", err)
	}

	pending, err = s.ListPendingHITL()
	if err != nil {
		t.Fatalf("ListPendingHITL after resolve: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after resolve, got %d", len(pending))
	}
}

func TestPurgeOldFiles(_ *testing.T) {
	// Ensure test binary does not leave temp files.
	os.RemoveAll("./test.db")
}
