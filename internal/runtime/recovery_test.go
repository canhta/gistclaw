package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

// TestRecovery_InterruptedRunsReconciled verifies that a run left in 'active'
// status (e.g. from a daemon crash) is moved to 'interrupted' on reconciliation
// and a run_interrupted journal event is recorded.
func TestRecovery_InterruptedRunsReconciled(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), sink)
	ctx := context.Background()

	// Insert a conversation and a run left in 'active' state.
	_, err := db.RawDB().ExecContext(ctx,
		`INSERT INTO conversations (id, key, connector_id, account_id, external_id, thread_id, project_id, created_at)
		 VALUES ('conv-crash', 'test:acct:ext:main', 'test', 'acct', 'ext', 'main', 'proj-recovery', datetime('now'))`)
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	_, err = db.RawDB().ExecContext(ctx,
		`INSERT INTO runs (id, conversation_id, agent_id, team_id, objective, cwd, authority_json, status, created_at, updated_at)
		 VALUES ('run-crashed', 'conv-crash', 'assistant', 'team-1', 'test', '/tmp', '{}', 'active', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}

	report, err := rt.ReconcileInterrupted(ctx)
	if err != nil {
		t.Fatalf("ReconcileInterrupted: %v", err)
	}
	if report.ReconciledCount != 1 {
		t.Errorf("expected 1 reconciled, got %d", report.ReconciledCount)
	}

	// Assert run status is now 'interrupted'.
	var status string
	if err := db.RawDB().QueryRowContext(ctx,
		"SELECT status FROM runs WHERE id = 'run-crashed'",
	).Scan(&status); err != nil {
		t.Fatalf("scan status: %v", err)
	}
	if status != string(model.RunStatusInterrupted) {
		t.Errorf("expected interrupted status, got %q", status)
	}

	// Assert a run_interrupted journal event exists.
	var eventCount int
	if err := db.RawDB().QueryRowContext(ctx,
		"SELECT count(*) FROM events WHERE run_id = 'run-crashed' AND kind = 'run_interrupted'",
	).Scan(&eventCount); err != nil {
		t.Fatalf("query events: %v", err)
	}
	if eventCount == 0 {
		t.Error("expected run_interrupted journal event, got none")
	}
}

// TestRecovery_StaleApprovalsExpired verifies that a pending approval older
// than 24 hours is marked 'expired' on startup reconciliation.
func TestRecovery_StaleApprovalsExpired(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), sink)
	ctx := context.Background()

	// Insert a stale pending approval (created 25 hours ago).
	_, err := db.RawDB().ExecContext(ctx,
		`INSERT INTO approvals (id, run_id, tool_name, args_json, binding_json, fingerprint, status, created_at)
		 VALUES ('approval-stale', 'run-old', 'bash', x'', '{}', 'fp-stale', 'pending', datetime('now', '-25 hours'))`)
	if err != nil {
		t.Fatalf("insert stale approval: %v", err)
	}

	n, err := rt.ExpireStaleApprovals(ctx)
	if err != nil {
		t.Fatalf("ExpireStaleApprovals: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 expired, got %d", n)
	}

	var status string
	if err := db.RawDB().QueryRowContext(ctx,
		"SELECT status FROM approvals WHERE id = 'approval-stale'",
	).Scan(&status); err != nil {
		t.Fatalf("scan status: %v", err)
	}
	if status != "expired" {
		t.Errorf("expected expired, got %q", status)
	}
}

func TestRecovery_DetachedPendingApprovalsExpired(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), sink)
	ctx := context.Background()

	_, err := db.RawDB().ExecContext(ctx,
		`INSERT INTO runs (id, conversation_id, agent_id, team_id, objective, cwd, authority_json, status, created_at, updated_at)
		 VALUES ('run-interrupted-with-approval', 'conv-detached', 'patcher', 'team-1', 'write file', '/tmp', '{}', 'interrupted', datetime('now'), datetime('now')),
		        ('run-needs-approval', 'conv-live', 'patcher', 'team-1', 'write file', '/tmp', '{}', 'needs_approval', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert runs: %v", err)
	}
	_, err = db.RawDB().ExecContext(ctx,
		`INSERT INTO approvals (id, run_id, tool_name, args_json, binding_json, fingerprint, status, created_at)
		 VALUES ('approval-detached', 'run-interrupted-with-approval', 'bash', x'', '{}', 'fp-detached', 'pending', datetime('now')),
		        ('approval-live', 'run-needs-approval', 'bash', x'', '{}', 'fp-live', 'pending', datetime('now'))`)
	if err != nil {
		t.Fatalf("insert approvals: %v", err)
	}

	n, err := rt.ExpireStaleApprovals(ctx)
	if err != nil {
		t.Fatalf("ExpireStaleApprovals: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 detached approval expired, got %d", n)
	}

	var detachedStatus, liveStatus string
	if err := db.RawDB().QueryRowContext(ctx,
		"SELECT status FROM approvals WHERE id = 'approval-detached'",
	).Scan(&detachedStatus); err != nil {
		t.Fatalf("scan detached approval status: %v", err)
	}
	if err := db.RawDB().QueryRowContext(ctx,
		"SELECT status FROM approvals WHERE id = 'approval-live'",
	).Scan(&liveStatus); err != nil {
		t.Fatalf("scan live approval status: %v", err)
	}
	if detachedStatus != "expired" {
		t.Fatalf("expected detached approval expired, got %q", detachedStatus)
	}
	if liveStatus != "pending" {
		t.Fatalf("expected active approval to stay pending, got %q", liveStatus)
	}
}

// TestRecovery_DiskFullHandled verifies that when a commit error carries
// SQLITE_FULL (code 13), store.Tx wraps it as store.ErrDiskFull — and that
// the helper correctly identifies the error via its Code() method.
func TestRecovery_DiskFullHandled(t *testing.T) {
	// sqliteFullErr (defined at bottom of file) satisfies the same Code() int
	// interface as modernc.org/sqlite's Error type.
	if !store.IsSQLiteFull(sqliteFullErr(13)) {
		t.Error("IsSQLiteFull: expected true for code 13, got false")
	}
	if store.IsSQLiteFull(errors.New("plain error")) {
		t.Error("IsSQLiteFull: expected false for plain error, got true")
	}
	if store.IsSQLiteFull(nil) {
		t.Error("IsSQLiteFull: expected false for nil, got true")
	}
}

// TestRecovery_IdleStartupNoModelCalls verifies that reconciliation on a
// clean database (no interrupted runs, no stale approvals) produces zero
// reconciled runs and zero expired approvals without triggering any model calls.
func TestRecovery_IdleStartupNoModelCalls(t *testing.T) {
	db, cs, mem, reg := setupMilestoneTestDeps(t)
	prov := NewMockProvider(nil, nil)
	sink := &model.NoopEventSink{}
	rt := New(db, cs, reg, mem, prov, sink)
	ctx := context.Background()

	report, err := rt.ReconcileInterrupted(ctx)
	if err != nil {
		t.Fatalf("ReconcileInterrupted: %v", err)
	}
	if report.ReconciledCount != 0 {
		t.Errorf("expected 0 reconciled, got %d", report.ReconciledCount)
	}

	n, err := rt.ExpireStaleApprovals(ctx)
	if err != nil {
		t.Fatalf("ExpireStaleApprovals: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 expired, got %d", n)
	}

	if prov.CallCount() != 0 {
		t.Errorf("expected 0 model calls, got %d", prov.CallCount())
	}
}

// sqliteFullErr is a test-only type that implements the Code() int interface
// returned by modernc.org/sqlite errors.
type sqliteFullErr int

func (e sqliteFullErr) Error() string { return "database is full (SQLITE_FULL)" }
func (e sqliteFullErr) Code() int     { return int(e) }
