package app

import (
	"context"
	"testing"

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
