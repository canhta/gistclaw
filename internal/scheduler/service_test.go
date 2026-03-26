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
			if cmd.WorkspaceRoot != schedule.WorkspaceRoot {
				t.Fatalf("workspace_root = %q, want %q", cmd.WorkspaceRoot, schedule.WorkspaceRoot)
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
		ID:            "sched-manage",
		Name:          "Hourly check",
		Objective:     "Inspect repository state",
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

	listed, err := service.ListSchedules(context.Background())
	if err != nil {
		t.Fatalf("ListSchedules returned error: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("ListSchedules returned %#v, want schedule %q", listed, created.ID)
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
