package app

import (
	"context"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/scheduler"
)

func TestApp_ScheduleLifecycleMethods(t *testing.T) {
	cfg := Config{
		DatabasePath:  ":memory:",
		StateDir:      t.TempDir(),
		WorkspaceRoot: t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
	}

	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	t.Cleanup(func() { _ = app.Stop() })

	created, err := app.CreateSchedule(context.Background(), scheduler.CreateScheduleInput{
		ID:        "sched-app",
		Name:      "Daily review",
		Objective: "Inspect repository status",
		Spec: scheduler.ScheduleSpec{
			Kind: scheduler.ScheduleKindAt,
			At:   "2030-01-01T00:00:00Z",
		},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateSchedule returned error: %v", err)
	}
	if created.WorkspaceRoot != app.cfg.WorkspaceRoot {
		t.Fatalf("CreateSchedule workspace_root = %q, want %q", created.WorkspaceRoot, app.cfg.WorkspaceRoot)
	}

	listed, err := app.ListSchedules(context.Background())
	if err != nil {
		t.Fatalf("ListSchedules returned error: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("ListSchedules returned %#v, want schedule %q", listed, created.ID)
	}

	loaded, err := app.LoadSchedule(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("LoadSchedule returned error: %v", err)
	}
	if loaded.ID != created.ID {
		t.Fatalf("LoadSchedule returned ID %q, want %q", loaded.ID, created.ID)
	}

	name := "Updated daily review"
	objective := "Inspect repository status with updated wording"
	spec := scheduler.ScheduleSpec{
		Kind:         scheduler.ScheduleKindEvery,
		At:           "2030-01-01T08:00:00Z",
		EverySeconds: 7200,
	}
	updated, err := app.UpdateSchedule(context.Background(), created.ID, scheduler.UpdateScheduleInput{
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
	if updated.Spec.Kind != scheduler.ScheduleKindEvery {
		t.Fatalf("UpdateSchedule returned kind %q, want %q", updated.Spec.Kind, scheduler.ScheduleKindEvery)
	}

	disabled, err := app.DisableSchedule(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("DisableSchedule returned error: %v", err)
	}
	if disabled.Enabled {
		t.Fatal("DisableSchedule returned enabled schedule")
	}

	enabled, err := app.EnableSchedule(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("EnableSchedule returned error: %v", err)
	}
	if !enabled.Enabled {
		t.Fatal("EnableSchedule returned disabled schedule")
	}

	if err := app.DeleteSchedule(context.Background(), created.ID); err != nil {
		t.Fatalf("DeleteSchedule returned error: %v", err)
	}

	listed, err = app.ListSchedules(context.Background())
	if err != nil {
		t.Fatalf("ListSchedules after delete returned error: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("ListSchedules returned %d schedules after delete, want 0", len(listed))
	}
}

func TestApp_ScheduleStatusReturnsNextWakeAndCounts(t *testing.T) {
	cfg := Config{
		DatabasePath:  ":memory:",
		StateDir:      t.TempDir(),
		WorkspaceRoot: t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
	}

	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	t.Cleanup(func() { _ = app.Stop() })

	ctx := context.Background()
	dueSchedule, err := app.CreateSchedule(ctx, scheduler.CreateScheduleInput{
		ID:        "sched-due",
		Name:      "Due review",
		Objective: "Inspect repository status",
		Spec: scheduler.ScheduleSpec{
			Kind: scheduler.ScheduleKindAt,
			At:   "2030-01-01T00:00:00Z",
		},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateSchedule due returned error: %v", err)
	}
	failedSchedule, err := app.CreateSchedule(ctx, scheduler.CreateScheduleInput{
		ID:        "sched-failed",
		Name:      "Failed review",
		Objective: "Inspect repository status after a failure",
		Spec: scheduler.ScheduleSpec{
			Kind: scheduler.ScheduleKindAt,
			At:   "2030-01-02T00:00:00Z",
		},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateSchedule failed returned error: %v", err)
	}
	disabledSchedule, err := app.CreateSchedule(ctx, scheduler.CreateScheduleInput{
		ID:        "sched-disabled",
		Name:      "Disabled review",
		Objective: "Inspect repository status later",
		Spec: scheduler.ScheduleSpec{
			Kind: scheduler.ScheduleKindAt,
			At:   "2030-01-03T00:00:00Z",
		},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateSchedule disabled returned error: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	dueAt := now.Add(-1 * time.Minute)
	failedAt := now.Add(-2 * time.Hour)
	futureAt := now.Add(2 * time.Hour)
	if _, err := app.db.RawDB().ExecContext(ctx,
		`UPDATE schedules
		    SET enabled = 1, next_run_at = ?, updated_at = ?
		  WHERE id = ?`,
		dueAt,
		now,
		dueSchedule.ID,
	); err != nil {
		t.Fatalf("update due schedule timing: %v", err)
	}
	if _, err := app.db.RawDB().ExecContext(ctx,
		`UPDATE schedules
		    SET enabled = 1, next_run_at = ?, last_run_at = ?, last_status = ?, last_error = ?, consecutive_failures = ?, updated_at = ?
		  WHERE id = ?`,
		futureAt,
		failedAt,
		scheduler.OccurrenceFailed,
		"dispatch boom",
		2,
		now,
		failedSchedule.ID,
	); err != nil {
		t.Fatalf("update failed schedule summary: %v", err)
	}
	if _, err := app.db.RawDB().ExecContext(ctx,
		`UPDATE schedules
		    SET enabled = 0, next_run_at = NULL, updated_at = ?
		  WHERE id = ?`,
		now,
		disabledSchedule.ID,
	); err != nil {
		t.Fatalf("disable schedule directly: %v", err)
	}
	if _, err := app.db.RawDB().ExecContext(ctx,
		`INSERT INTO schedule_occurrences
		 (id, schedule_id, slot_at, thread_id, status, skip_reason, run_id, conversation_id, error, started_at, finished_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, '', ?, ?, '', ?, NULL, ?, ?)`,
		"occ-active",
		dueSchedule.ID,
		dueAt,
		dueAt.Format(time.RFC3339Nano),
		scheduler.OccurrenceActive,
		"run-active",
		"conv-active",
		now,
		now,
		now,
	); err != nil {
		t.Fatalf("insert active occurrence: %v", err)
	}

	status, err := app.ScheduleStatus(ctx)
	if err != nil {
		t.Fatalf("ScheduleStatus returned error: %v", err)
	}
	if !status.Enabled {
		t.Fatal("ScheduleStatus returned disabled scheduler, want enabled")
	}
	if status.TotalSchedules != 3 {
		t.Fatalf("ScheduleStatus returned total_schedules %d, want 3", status.TotalSchedules)
	}
	if status.EnabledSchedules != 2 {
		t.Fatalf("ScheduleStatus returned enabled_schedules %d, want 2", status.EnabledSchedules)
	}
	if status.DueSchedules != 1 {
		t.Fatalf("ScheduleStatus returned due_schedules %d, want 1", status.DueSchedules)
	}
	if status.ActiveOccurrences != 1 {
		t.Fatalf("ScheduleStatus returned active_occurrences %d, want 1", status.ActiveOccurrences)
	}
	if !status.NextWakeAt.Equal(dueAt) {
		t.Fatalf("ScheduleStatus returned next_wake_at %s, want %s", status.NextWakeAt.Format(time.RFC3339), dueAt.Format(time.RFC3339))
	}
	if status.LastFailure == nil {
		t.Fatal("ScheduleStatus returned nil last failure")
	}
	if status.LastFailure.ScheduleID != failedSchedule.ID {
		t.Fatalf("ScheduleStatus returned last failure schedule %q, want %q", status.LastFailure.ScheduleID, failedSchedule.ID)
	}
	if status.LastFailure.Error != "dispatch boom" {
		t.Fatalf("ScheduleStatus returned last failure error %q, want %q", status.LastFailure.Error, "dispatch boom")
	}
}
