package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

func TestService_RunOnceDispatchesDueOccurrenceAndPersistsRunLink(t *testing.T) {
	t.Parallel()

	storeNow := mustParseRFC3339(t, "2026-03-26T01:00:00Z")
	runNow := mustParseRFC3339(t, "2026-03-26T02:05:00Z")
	s := newTestStore(t, storeNow)
	ctx := context.Background()
	schedule := mustCreateSchedule(t, ctx, s, "sched-dispatch")

	fake := &fakeDispatcher{
		dispatch: func(_ context.Context, cmd DispatchCommand) (model.Run, error) {
			if cmd.ConversationKey.ConnectorID != "schedule" {
				t.Fatalf("connector_id = %q, want %q", cmd.ConversationKey.ConnectorID, "schedule")
			}
			if cmd.ConversationKey.AccountID != "local" {
				t.Fatalf("account_id = %q, want %q", cmd.ConversationKey.AccountID, "local")
			}
			if cmd.ConversationKey.ExternalID != "job:"+schedule.ID {
				t.Fatalf("external_id = %q, want %q", cmd.ConversationKey.ExternalID, "job:"+schedule.ID)
			}
			if cmd.ConversationKey.ThreadID != "2026-03-26T02:00:00Z" {
				t.Fatalf("thread_id = %q, want %q", cmd.ConversationKey.ThreadID, "2026-03-26T02:00:00Z")
			}
			if cmd.FrontAgentID != "assistant" {
				t.Fatalf("front_agent_id = %q, want %q", cmd.FrontAgentID, "assistant")
			}
			if cmd.Body != schedule.Objective {
				t.Fatalf("body = %q, want %q", cmd.Body, schedule.Objective)
			}
			if cmd.CWD != schedule.CWD {
				t.Fatalf("cwd = %q, want %q", cmd.CWD, schedule.CWD)
			}
			if cmd.SourceMessageID == "" {
				t.Fatal("source_message_id is empty")
			}
			return model.Run{ID: "run-1", ConversationID: "conv-1", Status: model.RunStatusActive}, nil
		},
	}

	service := NewService(s, fake)
	service.clock = func() time.Time { return runNow }

	if err := service.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if fake.calls != 1 {
		t.Fatalf("dispatcher call count = %d, want 1", fake.calls)
	}

	occurrence := loadLatestOccurrence(t, s, schedule.ID)
	if occurrence.Status != OccurrenceActive {
		t.Fatalf("occurrence status = %q, want %q", occurrence.Status, OccurrenceActive)
	}
	if occurrence.RunID != "run-1" || occurrence.ConversationID != "conv-1" {
		t.Fatalf("occurrence run link = (%q, %q), want (%q, %q)", occurrence.RunID, occurrence.ConversationID, "run-1", "conv-1")
	}
	if occurrence.StartedAt.IsZero() {
		t.Fatal("occurrence started_at is zero")
	}
}

func TestService_RunOnceRecoversReceiptAfterDispatchError(t *testing.T) {
	t.Parallel()

	storeNow := mustParseRFC3339(t, "2026-03-26T01:00:00Z")
	runNow := mustParseRFC3339(t, "2026-03-26T02:05:00Z")
	s := newTestStore(t, storeNow)
	ctx := context.Background()
	schedule := mustCreateSchedule(t, ctx, s, "sched-recover")

	fake := &fakeDispatcher{
		dispatch: func(ctx context.Context, cmd DispatchCommand) (model.Run, error) {
			mustInsertRun(t, s, "run-recovered", "conv-recovered", model.RunStatusActive, runNow)
			mustInsertInboundReceipt(t, ctx, s, "conv-recovered", cmd.ConversationKey.ThreadID, cmd.SourceMessageID, "run-recovered", runNow)
			return model.Run{}, errors.New("dispatch crashed after accept")
		},
	}

	service := NewService(s, fake)
	service.clock = func() time.Time { return runNow }

	if err := service.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}

	occurrence := loadLatestOccurrence(t, s, schedule.ID)
	if occurrence.Status != OccurrenceActive {
		t.Fatalf("occurrence status = %q, want %q", occurrence.Status, OccurrenceActive)
	}
	if occurrence.RunID != "run-recovered" || occurrence.ConversationID != "conv-recovered" {
		t.Fatalf("occurrence run link = (%q, %q), want (%q, %q)", occurrence.RunID, occurrence.ConversationID, "run-recovered", "conv-recovered")
	}
}

func TestService_RepairMarksStaleDispatchingOccurrenceFailed(t *testing.T) {
	t.Parallel()

	storeNow := mustParseRFC3339(t, "2026-03-26T01:00:00Z")
	repairNow := mustParseRFC3339(t, "2026-03-26T02:10:00Z")
	s := newTestStore(t, storeNow)
	ctx := context.Background()
	schedule := mustCreateSchedule(t, ctx, s, "sched-stale")

	slot := mustParseRFC3339(t, "2026-03-26T02:00:00Z")
	mustInsertOccurrence(t, ctx, s, Occurrence{
		ID:         "occ-stale",
		ScheduleID: schedule.ID,
		SlotAt:     slot,
		ThreadID:   slot.Format(time.RFC3339Nano),
		Status:     OccurrenceDispatching,
		CreatedAt:  mustParseRFC3339(t, "2026-03-26T02:00:01Z"),
		UpdatedAt:  mustParseRFC3339(t, "2026-03-26T02:00:01Z"),
	})

	service := NewService(s, &fakeDispatcher{})
	service.clock = func() time.Time { return repairNow }
	service.dispatchGracePeriod = 5 * time.Minute

	if err := service.Repair(ctx); err != nil {
		t.Fatalf("Repair returned error: %v", err)
	}

	occurrence := loadOccurrenceByID(t, s, "occ-stale")
	if occurrence.Status != OccurrenceFailed {
		t.Fatalf("occurrence status = %q, want %q", occurrence.Status, OccurrenceFailed)
	}
	if occurrence.Error == "" {
		t.Fatal("occurrence error is empty")
	}

	summary := loadScheduleSummary(t, s, schedule.ID)
	if summary.LastStatus != OccurrenceFailed {
		t.Fatalf("schedule last_status = %q, want %q", summary.LastStatus, OccurrenceFailed)
	}
	if summary.ConsecutiveFailures != 1 {
		t.Fatalf("schedule consecutive_failures = %d, want 1", summary.ConsecutiveFailures)
	}
}

func TestService_RunOnceReconcilesOpenOccurrencesFromRuns(t *testing.T) {
	t.Parallel()

	storeNow := mustParseRFC3339(t, "2026-03-26T01:00:00Z")
	runNow := mustParseRFC3339(t, "2026-03-26T02:15:00Z")
	s := newTestStore(t, storeNow)
	ctx := context.Background()
	schedule := mustCreateSchedule(t, ctx, s, "sched-reconcile")

	slot := mustParseRFC3339(t, "2026-03-26T02:00:00Z")
	mustInsertOccurrence(t, ctx, s, Occurrence{
		ID:             "occ-active",
		ScheduleID:     schedule.ID,
		SlotAt:         slot,
		ThreadID:       slot.Format(time.RFC3339Nano),
		Status:         OccurrenceActive,
		RunID:          "run-complete",
		ConversationID: "conv-complete",
		StartedAt:      slot,
		CreatedAt:      slot,
		UpdatedAt:      slot,
	})
	mustInsertRun(t, s, "run-complete", "conv-complete", model.RunStatusCompleted, runNow)

	service := NewService(s, &fakeDispatcher{})
	service.clock = func() time.Time { return runNow }

	if err := service.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}

	occurrence := loadOccurrenceByID(t, s, "occ-active")
	if occurrence.Status != OccurrenceCompleted {
		t.Fatalf("occurrence status = %q, want %q", occurrence.Status, OccurrenceCompleted)
	}
	if occurrence.FinishedAt.IsZero() {
		t.Fatal("occurrence finished_at is zero")
	}

	summary := loadScheduleSummary(t, s, schedule.ID)
	if summary.LastStatus != OccurrenceCompleted {
		t.Fatalf("schedule last_status = %q, want %q", summary.LastStatus, OccurrenceCompleted)
	}
	if !summary.LastRunAt.Equal(slot) {
		t.Fatalf("schedule last_run_at = %s, want %s", summary.LastRunAt.Format(time.RFC3339), slot.Format(time.RFC3339))
	}
}

func TestService_RepairRecomputesMissingNextRunAtForEnabledSchedules(t *testing.T) {
	t.Parallel()

	storeNow := mustParseRFC3339(t, "2026-03-26T01:00:00Z")
	repairNow := mustParseRFC3339(t, "2026-03-26T01:30:00Z")
	s := newTestStore(t, storeNow)
	ctx := context.Background()
	schedule := mustCreateSchedule(t, ctx, s, "sched-missing-next")

	if _, err := s.db.RawDB().ExecContext(ctx,
		"UPDATE schedules SET next_run_at = NULL, updated_at = ? WHERE id = ?",
		repairNow,
		schedule.ID,
	); err != nil {
		t.Fatalf("clear next_run_at: %v", err)
	}

	service := NewService(s, &fakeDispatcher{})
	service.clock = func() time.Time { return repairNow }

	if err := service.Repair(ctx); err != nil {
		t.Fatalf("Repair returned error: %v", err)
	}

	nextRun := loadScheduleNextRunAt(t, s.db, schedule.ID)
	wantNext := mustParseRFC3339(t, "2026-03-26T02:00:00Z")
	if !nextRun.Equal(wantNext) {
		t.Fatalf("next_run_at = %s, want %s", nextRun.Format(time.RFC3339), wantNext.Format(time.RFC3339))
	}
}

func TestService_ScheduleLifecycleOperations(t *testing.T) {
	t.Parallel()

	now := mustParseRFC3339(t, "2026-03-26T01:00:00Z")
	s := newTestStore(t, now)
	service := NewService(s, &fakeDispatcher{})
	service.clock = func() time.Time { return now }

	created, err := service.CreateSchedule(context.Background(), CreateScheduleInput{
		ID:        "sched-manage",
		Name:      "Hourly check",
		Objective: "Inspect repository state",
		CWD:       "/tmp/repo",
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

	listed, err := service.ListSchedules(context.Background())
	if err != nil {
		t.Fatalf("ListSchedules returned error: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("ListSchedules returned %#v, want schedule %q", listed, created.ID)
	}

	loaded, err := service.LoadSchedule(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("LoadSchedule returned error: %v", err)
	}
	if loaded.ID != created.ID {
		t.Fatalf("LoadSchedule returned ID %q, want %q", loaded.ID, created.ID)
	}

	status, err := service.Status(context.Background())
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if status.TotalSchedules != 1 || status.EnabledSchedules != 1 {
		t.Fatalf("Status returned totals (%d, %d), want (1, 1)", status.TotalSchedules, status.EnabledSchedules)
	}

	name := "Updated hourly check"
	objective := "Inspect repository health with the updated objective"
	spec := schedulerSpecEvery(t, "2026-03-26T10:00:00+07:00", 2*time.Hour)
	updated, err := service.UpdateSchedule(context.Background(), created.ID, UpdateScheduleInput{
		Name:      &name,
		Objective: &objective,
		Spec:      &spec,
	})
	if err != nil {
		t.Fatalf("UpdateSchedule returned error: %v", err)
	}
	if updated.Name != name || updated.Objective != objective {
		t.Fatalf("UpdateSchedule returned (%q, %q), want (%q, %q)", updated.Name, updated.Objective, name, objective)
	}
	if updated.Spec.EverySeconds != int64((2*time.Hour)/time.Second) {
		t.Fatalf("UpdateSchedule returned every_seconds %d, want %d", updated.Spec.EverySeconds, int64((2*time.Hour)/time.Second))
	}

	disabled, err := service.DisableSchedule(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("DisableSchedule returned error: %v", err)
	}
	if disabled.Enabled {
		t.Fatal("DisableSchedule returned enabled schedule")
	}

	enabled, err := service.EnableSchedule(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("EnableSchedule returned error: %v", err)
	}
	if !enabled.Enabled {
		t.Fatal("EnableSchedule returned disabled schedule")
	}

	if err := service.DeleteSchedule(context.Background(), created.ID); err != nil {
		t.Fatalf("DeleteSchedule returned error: %v", err)
	}

	listed, err = service.ListSchedules(context.Background())
	if err != nil {
		t.Fatalf("ListSchedules after delete returned error: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("ListSchedules returned %d schedules after delete, want 0", len(listed))
	}
}

func TestService_RunNowDispatchesImmediateOccurrence(t *testing.T) {
	t.Parallel()

	storeNow := mustParseRFC3339(t, "2026-03-26T01:00:00Z")
	runNow := mustParseRFC3339(t, "2026-03-26T05:45:00Z")
	s := newTestStore(t, storeNow)
	ctx := context.Background()
	schedule := mustCreateSchedule(t, ctx, s, "sched-run-now")

	fake := &fakeDispatcher{
		dispatch: func(_ context.Context, cmd DispatchCommand) (model.Run, error) {
			if cmd.ConversationKey.ThreadID != runNow.Format(time.RFC3339Nano) {
				t.Fatalf("thread_id = %q, want %q", cmd.ConversationKey.ThreadID, runNow.Format(time.RFC3339Nano))
			}
			return model.Run{ID: "run-now", ConversationID: "conv-now", Status: model.RunStatusActive}, nil
		},
	}

	service := NewService(s, fake)
	service.clock = func() time.Time { return runNow }

	claimed, err := service.RunNow(ctx, schedule.ID)
	if err != nil {
		t.Fatalf("RunNow returned error: %v", err)
	}
	if claimed == nil {
		t.Fatal("RunNow returned nil claim")
	}

	occurrence := loadOccurrenceByID(t, s, claimed.Occurrence.ID)
	if occurrence.Status != OccurrenceActive {
		t.Fatalf("occurrence status = %q, want %q", occurrence.Status, OccurrenceActive)
	}
	if !occurrence.SlotAt.Equal(runNow) {
		t.Fatalf("occurrence slot_at = %s, want %s", occurrence.SlotAt.Format(time.RFC3339), runNow.Format(time.RFC3339))
	}
	if occurrence.RunID != "run-now" || occurrence.ConversationID != "conv-now" {
		t.Fatalf("occurrence run link = (%q, %q), want (%q, %q)", occurrence.RunID, occurrence.ConversationID, "run-now", "conv-now")
	}
}

func TestService_RunOnceHonorsDueLimitForOverdueSchedules(t *testing.T) {
	t.Parallel()

	storeNow := mustParseRFC3339(t, "2026-03-26T01:00:00Z")
	runNow := mustParseRFC3339(t, "2026-03-26T05:05:00Z")
	s := newTestStore(t, storeNow)
	ctx := context.Background()
	first := mustCreateSchedule(t, ctx, s, "sched-due-limit-first")
	second := mustCreateSchedule(t, ctx, s, "sched-due-limit-second")

	firstDue := mustParseRFC3339(t, "2026-03-26T02:00:00Z")
	secondDue := mustParseRFC3339(t, "2026-03-26T03:00:00Z")
	mustSetScheduleTiming(t, s.db, first.ID, true, firstDue)
	mustSetScheduleTiming(t, s.db, second.ID, true, secondDue)

	fake := &fakeDispatcher{
		dispatch: func(_ context.Context, cmd DispatchCommand) (model.Run, error) {
			return model.Run{ID: "run-" + cmd.ConversationKey.ExternalID, ConversationID: "conv", Status: model.RunStatusActive}, nil
		},
	}
	service := NewService(s, fake)
	service.clock = func() time.Time { return runNow }
	service.dueLimit = 1

	if err := service.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if fake.calls != 1 {
		t.Fatalf("dispatcher call count = %d, want 1", fake.calls)
	}

	if count := countOccurrencesForSchedule(t, s, first.ID); count != 1 {
		t.Fatalf("first schedule occurrence count = %d, want 1", count)
	}
	if count := countOccurrencesForSchedule(t, s, second.ID); count != 0 {
		t.Fatalf("second schedule occurrence count = %d, want 0", count)
	}
}

func TestService_RunNowSkipsDuplicateWhilePreviousOccurrenceIsOpen(t *testing.T) {
	t.Parallel()

	storeNow := mustParseRFC3339(t, "2026-03-26T01:00:00Z")
	firstRunNow := mustParseRFC3339(t, "2026-03-26T05:45:00Z")
	secondRunNow := mustParseRFC3339(t, "2026-03-26T05:45:01Z")
	s := newTestStore(t, storeNow)
	ctx := context.Background()
	schedule := mustCreateSchedule(t, ctx, s, "sched-run-now-skip")

	fake := &fakeDispatcher{
		dispatch: func(_ context.Context, cmd DispatchCommand) (model.Run, error) {
			return model.Run{ID: "run-" + cmd.SourceMessageID, ConversationID: "conv-now", Status: model.RunStatusActive}, nil
		},
	}
	service := NewService(s, fake)
	service.clock = func() time.Time { return firstRunNow }

	first, err := service.RunNow(ctx, schedule.ID)
	if err != nil {
		t.Fatalf("first RunNow returned error: %v", err)
	}
	if first == nil {
		t.Fatal("first RunNow returned nil claim")
	}

	service.clock = func() time.Time { return secondRunNow }
	second, err := service.RunNow(ctx, schedule.ID)
	if err != nil {
		t.Fatalf("second RunNow returned error: %v", err)
	}
	if second != nil {
		t.Fatalf("second RunNow returned claim %#v, want nil skipped duplicate", second)
	}
	if fake.calls != 1 {
		t.Fatalf("dispatcher call count = %d, want 1", fake.calls)
	}
}

func TestService_NextWakeDelayUsesNearestScheduledRunWithClamp(t *testing.T) {
	t.Parallel()

	storeNow := mustParseRFC3339(t, "2026-03-26T01:00:00Z")
	now := mustParseRFC3339(t, "2026-03-26T03:30:00Z")
	s := newTestStore(t, storeNow)
	ctx := context.Background()

	service := NewService(s, &fakeDispatcher{})
	service.clock = func() time.Time { return now }
	service.wakeInterval = 30 * time.Second
	service.minWakeInterval = time.Second

	t.Run("no schedules uses fallback", func(t *testing.T) {
		delay, err := service.nextWakeDelay(ctx, now)
		if err != nil {
			t.Fatalf("nextWakeDelay returned error: %v", err)
		}
		if delay != 30*time.Second {
			t.Fatalf("delay = %s, want %s", delay, 30*time.Second)
		}
	})

	t.Run("future wake uses nearest schedule", func(t *testing.T) {
		schedule := mustCreateSchedule(t, ctx, s, "sched-next-wake")
		nextWake := now.Add(5 * time.Second)
		mustSetScheduleTiming(t, s.db, schedule.ID, true, nextWake)

		delay, err := service.nextWakeDelay(ctx, now)
		if err != nil {
			t.Fatalf("nextWakeDelay returned error: %v", err)
		}
		if delay != 5*time.Second {
			t.Fatalf("delay = %s, want %s", delay, 5*time.Second)
		}
	})

	t.Run("overdue wake clamps to minimum", func(t *testing.T) {
		schedule := mustCreateSchedule(t, ctx, s, "sched-next-wake-overdue")
		mustSetScheduleTiming(t, s.db, schedule.ID, true, now.Add(-10*time.Second))

		delay, err := service.nextWakeDelay(ctx, now)
		if err != nil {
			t.Fatalf("nextWakeDelay returned error: %v", err)
		}
		if delay != time.Second {
			t.Fatalf("delay = %s, want %s", delay, time.Second)
		}
	})
}

func schedulerSpecEvery(t *testing.T, at string, every time.Duration) ScheduleSpec {
	t.Helper()
	return ScheduleSpec{
		Kind:         ScheduleKindEvery,
		At:           at,
		EverySeconds: int64(every / time.Second),
	}
}

type fakeDispatcher struct {
	calls    int
	dispatch func(ctx context.Context, cmd DispatchCommand) (model.Run, error)
}

func (f *fakeDispatcher) DispatchScheduled(ctx context.Context, cmd DispatchCommand) (model.Run, error) {
	f.calls++
	if f.dispatch == nil {
		return model.Run{}, nil
	}
	return f.dispatch(ctx, cmd)
}

func loadLatestOccurrence(t *testing.T, s *Store, scheduleID string) Occurrence {
	t.Helper()

	occurrence, err := scanOccurrence(s.db.RawDB().QueryRow(
		`SELECT id, schedule_id, slot_at, thread_id, status, skip_reason, run_id, conversation_id, error,
		        started_at, finished_at, created_at, updated_at
		   FROM schedule_occurrences
		  WHERE schedule_id = ?
		  ORDER BY created_at DESC, id DESC
		  LIMIT 1`,
		scheduleID,
	))
	if err != nil {
		t.Fatalf("load latest occurrence: %v", err)
	}
	return occurrence
}

func loadOccurrenceByID(t *testing.T, s *Store, occurrenceID string) Occurrence {
	t.Helper()

	occurrence, err := loadOccurrenceRow(context.Background(), s.db.RawDB(), occurrenceID)
	if err != nil {
		t.Fatalf("load occurrence by id: %v", err)
	}
	return occurrence
}

func mustInsertRun(t *testing.T, s *Store, runID, conversationID string, status model.RunStatus, now time.Time) {
	t.Helper()

	if _, err := s.db.RawDB().Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, status, created_at, updated_at)
		 VALUES (?, ?, 'assistant', ?, ?, ?)`,
		runID,
		conversationID,
		status,
		now.UTC(),
		now.UTC(),
	); err != nil {
		t.Fatalf("insert run: %v", err)
	}
}

func mustInsertInboundReceipt(t *testing.T, ctx context.Context, s *Store, conversationID, threadID, sourceMessageID, runID string, now time.Time) {
	t.Helper()

	if _, err := s.db.RawDB().ExecContext(ctx,
		`INSERT INTO inbound_receipts
		 (id, conversation_id, connector_id, account_id, thread_id, source_message_id, run_id, session_id, session_message_id, created_at)
		 VALUES (?, ?, 'schedule', 'local', ?, ?, ?, 'sess-schedule', '', ?)`,
		"receipt-"+runID,
		conversationID,
		threadID,
		sourceMessageID,
		runID,
		now.UTC(),
	); err != nil {
		t.Fatalf("insert inbound receipt: %v", err)
	}
}

func mustInsertOccurrence(t *testing.T, ctx context.Context, s *Store, occurrence Occurrence) {
	t.Helper()

	if _, err := s.db.RawDB().ExecContext(ctx,
		`INSERT INTO schedule_occurrences
		 (id, schedule_id, slot_at, thread_id, status, skip_reason, run_id, conversation_id, error,
		  started_at, finished_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		occurrence.ID,
		occurrence.ScheduleID,
		occurrence.SlotAt.UTC(),
		occurrence.ThreadID,
		occurrence.Status,
		occurrence.SkipReason,
		occurrence.RunID,
		occurrence.ConversationID,
		occurrence.Error,
		nullableTime(occurrence.StartedAt),
		nullableTime(occurrence.FinishedAt),
		occurrence.CreatedAt.UTC(),
		occurrence.UpdatedAt.UTC(),
	); err != nil {
		t.Fatalf("insert occurrence: %v", err)
	}
}

func loadScheduleSummary(t *testing.T, s *Store, scheduleID string) Schedule {
	t.Helper()

	schedule, err := s.LoadSchedule(context.Background(), scheduleID)
	if err != nil {
		t.Fatalf("LoadSchedule(%q): %v", scheduleID, err)
	}
	return schedule
}

func countOccurrencesForSchedule(t *testing.T, s *Store, scheduleID string) int {
	t.Helper()

	var count int
	if err := s.db.RawDB().QueryRow(
		`SELECT COUNT(1)
		   FROM schedule_occurrences
		  WHERE schedule_id = ?`,
		scheduleID,
	).Scan(&count); err != nil {
		t.Fatalf("count occurrences for %q: %v", scheduleID, err)
	}
	return count
}
