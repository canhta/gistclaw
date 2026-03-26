package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/store"
)

func TestSchedulerSchema_CreatesSchedulesAndOccurrencesTables(t *testing.T) {
	db := openTestDB(t)

	names := loadTableNames(t, db, "schedules", "schedule_occurrences")
	if !names["schedules"] || !names["schedule_occurrences"] {
		t.Fatalf("missing scheduler tables: %#v", names)
	}
}

func TestSchedulerTypes_ExposeSupportedKindsAndStatuses(t *testing.T) {
	kinds := []ScheduleKind{ScheduleKindAt, ScheduleKindEvery, ScheduleKindCron}
	if len(kinds) != 3 {
		t.Fatalf("expected 3 schedule kinds, got %d", len(kinds))
	}

	statuses := []OccurrenceStatus{
		OccurrenceDispatching,
		OccurrenceActive,
		OccurrenceNeedsApproval,
		OccurrenceCompleted,
		OccurrenceFailed,
		OccurrenceInterrupted,
		OccurrenceSkipped,
	}
	if len(statuses) != 7 {
		t.Fatalf("expected 7 occurrence statuses, got %d", len(statuses))
	}
}

func TestStore_CreateScheduleNormalizesSummaryFields(t *testing.T) {
	t.Parallel()

	now := mustParseRFC3339(t, "2026-03-26T01:00:00Z")
	s := newTestStore(t, now)

	got, err := s.CreateSchedule(context.Background(), CreateScheduleInput{
		ID:            "sched-create",
		Name:          "Morning sync",
		Objective:     "Check the repo state",
		WorkspaceRoot: "/tmp/repo",
		Spec: ScheduleSpec{
			Kind:         ScheduleKindEvery,
			At:           "2026-03-26T09:00:00+07:00",
			EverySeconds: 3600,
		},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateSchedule returned error: %v", err)
	}

	if got.ID != "sched-create" {
		t.Fatalf("CreateSchedule returned ID %q, want %q", got.ID, "sched-create")
	}
	if !got.Enabled {
		t.Fatal("CreateSchedule returned disabled schedule, want enabled")
	}
	if got.LastStatus != "" || got.LastError != "" {
		t.Fatalf("CreateSchedule returned unexpected summary fields: status=%q error=%q", got.LastStatus, got.LastError)
	}
	if got.ConsecutiveFailures != 0 || got.ScheduleErrorCount != 0 {
		t.Fatalf("CreateSchedule returned non-zero counters: failures=%d schedule_errors=%d", got.ConsecutiveFailures, got.ScheduleErrorCount)
	}

	wantNext := mustParseRFC3339(t, "2026-03-26T02:00:00Z")
	if !got.NextRunAt.Equal(wantNext) {
		t.Fatalf("CreateSchedule returned next_run_at %s, want %s", got.NextRunAt.Format(time.RFC3339), wantNext.Format(time.RFC3339))
	}
}

func TestStore_ListDueSchedulesOrdersByNextRunAt(t *testing.T) {
	t.Parallel()

	now := mustParseRFC3339(t, "2026-03-26T03:30:00Z")
	s := newTestStore(t, now)
	ctx := context.Background()

	first := mustCreateSchedule(t, ctx, s, "sched-first")
	second := mustCreateSchedule(t, ctx, s, "sched-second")
	third := mustCreateSchedule(t, ctx, s, "sched-third")
	disabled := mustCreateSchedule(t, ctx, s, "sched-disabled")

	mustSetScheduleTiming(t, s.db, first.ID, true, mustParseRFC3339(t, "2026-03-26T02:00:00Z"))
	mustSetScheduleTiming(t, s.db, second.ID, true, mustParseRFC3339(t, "2026-03-26T03:00:00Z"))
	mustSetScheduleTiming(t, s.db, third.ID, true, mustParseRFC3339(t, "2026-03-26T04:00:00Z"))
	mustSetScheduleTiming(t, s.db, disabled.ID, false, mustParseRFC3339(t, "2026-03-26T01:00:00Z"))

	got, err := s.ListDueSchedules(ctx, now, 10)
	if err != nil {
		t.Fatalf("ListDueSchedules returned error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("ListDueSchedules returned %d schedules, want 2", len(got))
	}
	if got[0].ID != first.ID || got[1].ID != second.ID {
		t.Fatalf("ListDueSchedules returned IDs [%s %s], want [%s %s]", got[0].ID, got[1].ID, first.ID, second.ID)
	}
}

func TestStore_ClaimDueOccurrenceCreatesSingleDispatchingRow(t *testing.T) {
	t.Parallel()

	storeNow := mustParseRFC3339(t, "2026-03-26T01:00:00Z")
	claimNow := mustParseRFC3339(t, "2026-03-26T02:05:00Z")
	s := newTestStore(t, storeNow)
	ctx := context.Background()

	schedule := mustCreateSchedule(t, ctx, s, "sched-claim")

	claimed, err := s.ClaimDueOccurrence(ctx, schedule.ID, claimNow)
	if err != nil {
		t.Fatalf("ClaimDueOccurrence returned error: %v", err)
	}
	if claimed == nil {
		t.Fatal("ClaimDueOccurrence returned nil claim for due schedule")
	}
	if claimed.Occurrence.Status != OccurrenceDispatching {
		t.Fatalf("ClaimDueOccurrence returned status %q, want %q", claimed.Occurrence.Status, OccurrenceDispatching)
	}

	wantSlot := mustParseRFC3339(t, "2026-03-26T02:00:00Z")
	if !claimed.Occurrence.SlotAt.Equal(wantSlot) {
		t.Fatalf("ClaimDueOccurrence returned slot %s, want %s", claimed.Occurrence.SlotAt.Format(time.RFC3339), wantSlot.Format(time.RFC3339))
	}
	if claimed.Occurrence.ThreadID != wantSlot.Format(time.RFC3339Nano) {
		t.Fatalf("ClaimDueOccurrence returned thread_id %q, want %q", claimed.Occurrence.ThreadID, wantSlot.Format(time.RFC3339Nano))
	}

	status, skipReason, count := loadOccurrenceStatusRow(t, s.db, schedule.ID, wantSlot)
	if count != 1 {
		t.Fatalf("occurrence row count = %d, want 1", count)
	}
	if status != string(OccurrenceDispatching) || skipReason != "" {
		t.Fatalf("occurrence row = (%q, %q), want (%q, %q)", status, skipReason, OccurrenceDispatching, "")
	}

	nextRun := loadScheduleNextRunAt(t, s.db, schedule.ID)
	wantNext := mustParseRFC3339(t, "2026-03-26T03:00:00Z")
	if !nextRun.Equal(wantNext) {
		t.Fatalf("next_run_at = %s, want %s", nextRun.Format(time.RFC3339), wantNext.Format(time.RFC3339))
	}
}

func TestStore_ClaimDueOccurrenceSkipsWhenPreviousOccurrenceActive(t *testing.T) {
	t.Parallel()

	storeNow := mustParseRFC3339(t, "2026-03-26T02:30:00Z")
	claimNow := mustParseRFC3339(t, "2026-03-26T03:05:00Z")
	s := newTestStore(t, storeNow)
	ctx := context.Background()

	schedule := mustCreateSchedule(t, ctx, s, "sched-skip")
	previousSlot := mustParseRFC3339(t, "2026-03-26T02:00:00Z")
	if _, err := s.db.RawDB().ExecContext(ctx,
		`INSERT INTO schedule_occurrences
		 (id, schedule_id, slot_at, thread_id, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"occ-active", schedule.ID, previousSlot, previousSlot.Format(time.RFC3339Nano), OccurrenceActive, claimNow, claimNow,
	); err != nil {
		t.Fatalf("seed active occurrence: %v", err)
	}

	claimed, err := s.ClaimDueOccurrence(ctx, schedule.ID, claimNow)
	if err != nil {
		t.Fatalf("ClaimDueOccurrence returned error: %v", err)
	}
	if claimed != nil {
		t.Fatalf("ClaimDueOccurrence returned claim %#v, want nil for skipped slot", claimed)
	}

	slot := mustParseRFC3339(t, "2026-03-26T03:00:00Z")
	status, skipReason, count := loadOccurrenceStatusRow(t, s.db, schedule.ID, slot)
	if count != 1 {
		t.Fatalf("occurrence row count = %d, want 1", count)
	}
	if status != string(OccurrenceSkipped) || skipReason != "previous_occurrence_active" {
		t.Fatalf("occurrence row = (%q, %q), want (%q, %q)", status, skipReason, OccurrenceSkipped, "previous_occurrence_active")
	}

	nextRun := loadScheduleNextRunAt(t, s.db, schedule.ID)
	wantNext := mustParseRFC3339(t, "2026-03-26T04:00:00Z")
	if !nextRun.Equal(wantNext) {
		t.Fatalf("next_run_at = %s, want %s", nextRun.Format(time.RFC3339), wantNext.Format(time.RFC3339))
	}
}

func TestStore_ClaimDueOccurrenceIsIdempotentPerSlot(t *testing.T) {
	t.Parallel()

	storeNow := mustParseRFC3339(t, "2026-03-26T01:00:00Z")
	claimNow := mustParseRFC3339(t, "2026-03-26T02:05:00Z")
	s := newTestStore(t, storeNow)
	ctx := context.Background()

	schedule := mustCreateSchedule(t, ctx, s, "sched-idempotent")
	first, err := s.ClaimDueOccurrence(ctx, schedule.ID, claimNow)
	if err != nil {
		t.Fatalf("first ClaimDueOccurrence returned error: %v", err)
	}
	if first == nil {
		t.Fatal("first ClaimDueOccurrence returned nil claim")
	}

	slot := mustParseRFC3339(t, "2026-03-26T02:00:00Z")
	if _, err := s.db.RawDB().ExecContext(ctx,
		"UPDATE schedules SET next_run_at = ?, updated_at = ? WHERE id = ?",
		slot, claimNow, schedule.ID,
	); err != nil {
		t.Fatalf("reset next_run_at: %v", err)
	}

	second, err := s.ClaimDueOccurrence(ctx, schedule.ID, claimNow)
	if err != nil {
		t.Fatalf("second ClaimDueOccurrence returned error: %v", err)
	}
	if second != nil {
		t.Fatalf("second ClaimDueOccurrence returned claim %#v, want nil duplicate slot", second)
	}

	status, _, count := loadOccurrenceStatusRow(t, s.db, schedule.ID, slot)
	if count != 1 {
		t.Fatalf("occurrence row count = %d, want 1", count)
	}
	if status != string(OccurrenceDispatching) {
		t.Fatalf("occurrence status = %q, want %q", status, OccurrenceDispatching)
	}

	nextRun := loadScheduleNextRunAt(t, s.db, schedule.ID)
	wantNext := mustParseRFC3339(t, "2026-03-26T03:00:00Z")
	if !nextRun.Equal(wantNext) {
		t.Fatalf("next_run_at = %s, want %s", nextRun.Format(time.RFC3339), wantNext.Format(time.RFC3339))
	}
}

func TestStore_ClaimDueOccurrenceAdvancesRecurringScheduleToNextFutureSlot(t *testing.T) {
	t.Parallel()

	storeNow := mustParseRFC3339(t, "2026-03-26T01:00:00Z")
	claimNow := mustParseRFC3339(t, "2026-03-26T06:05:00Z")
	s := newTestStore(t, storeNow)
	ctx := context.Background()

	schedule := mustCreateSchedule(t, ctx, s, "sched-overdue")
	overdueSlot := mustParseRFC3339(t, "2026-03-26T02:00:00Z")
	if _, err := s.db.RawDB().ExecContext(ctx,
		"UPDATE schedules SET next_run_at = ?, updated_at = ? WHERE id = ?",
		overdueSlot,
		claimNow,
		schedule.ID,
	); err != nil {
		t.Fatalf("set overdue next_run_at: %v", err)
	}

	claimed, err := s.ClaimDueOccurrence(ctx, schedule.ID, claimNow)
	if err != nil {
		t.Fatalf("ClaimDueOccurrence returned error: %v", err)
	}
	if claimed == nil {
		t.Fatal("ClaimDueOccurrence returned nil claim for overdue schedule")
	}

	nextRun := loadScheduleNextRunAt(t, s.db, schedule.ID)
	wantNext := mustParseRFC3339(t, "2026-03-26T07:00:00Z")
	if !nextRun.Equal(wantNext) {
		t.Fatalf("next_run_at = %s, want %s", nextRun.Format(time.RFC3339), wantNext.Format(time.RFC3339))
	}
}

func openTestDB(t *testing.T) *store.DB {
	t.Helper()

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	return db
}

func newTestStore(t *testing.T, now time.Time) *Store {
	t.Helper()

	s := NewStore(openTestDB(t))
	s.now = func() time.Time { return now.UTC() }
	return s
}

func mustCreateSchedule(t *testing.T, ctx context.Context, s *Store, id string) Schedule {
	t.Helper()

	schedule, err := s.CreateSchedule(ctx, CreateScheduleInput{
		ID:            id,
		Name:          id,
		Objective:     "Check repository health",
		WorkspaceRoot: "/tmp/repo",
		Spec: ScheduleSpec{
			Kind:         ScheduleKindEvery,
			At:           "2026-03-26T09:00:00+07:00",
			EverySeconds: 3600,
		},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateSchedule(%q): %v", id, err)
	}

	return schedule
}

func mustSetScheduleTiming(t *testing.T, db *store.DB, id string, enabled bool, nextRunAt time.Time) {
	t.Helper()

	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	if _, err := db.RawDB().Exec(
		"UPDATE schedules SET enabled = ?, next_run_at = ?, updated_at = ? WHERE id = ?",
		enabledInt, nextRunAt.UTC(), nextRunAt.UTC(), id,
	); err != nil {
		t.Fatalf("set schedule timing: %v", err)
	}
}

func loadTableNames(t *testing.T, db *store.DB, names ...string) map[string]bool {
	t.Helper()

	out := make(map[string]bool, len(names))
	for _, name := range names {
		var found string
		err := db.RawDB().QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			name,
		).Scan(&found)
		if err == nil {
			out[name] = true
		}
	}

	return out
}

func loadOccurrenceStatusRow(t *testing.T, db *store.DB, scheduleID string, slotAt time.Time) (string, string, int) {
	t.Helper()

	var status string
	var skipReason string
	err := db.RawDB().QueryRow(
		`SELECT status, skip_reason
		 FROM schedule_occurrences
		 WHERE schedule_id = ? AND slot_at = ?`,
		scheduleID, slotAt.UTC(),
	).Scan(&status, &skipReason)
	if err != nil {
		return "", "", 0
	}
	return status, skipReason, 1
}

func loadScheduleNextRunAt(t *testing.T, db *store.DB, scheduleID string) time.Time {
	t.Helper()

	var nextRunAt time.Time
	if err := db.RawDB().QueryRow(
		"SELECT next_run_at FROM schedules WHERE id = ?",
		scheduleID,
	).Scan(&nextRunAt); err != nil {
		t.Fatalf("load next_run_at: %v", err)
	}
	return nextRunAt.UTC()
}
