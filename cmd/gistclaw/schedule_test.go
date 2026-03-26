package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/scheduler"
)

func TestRun_ScheduleWithoutSubcommandShowsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"schedule"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit for missing schedule subcommand")
	}
	if !strings.Contains(stderr.String(), "Usage: gistclaw schedule") {
		t.Fatalf("expected schedule usage in stderr, got:\n%s", stderr.String())
	}
}

func TestParseScheduleAddArgs_AtSchedule(t *testing.T) {
	input, err := parseScheduleAddArgs([]string{
		"--name", "Daily review",
		"--objective", "Inspect repository status",
		"--at", "2030-01-01T00:00:00Z",
		"--workspace-root", "/tmp/repo",
	})
	if err != nil {
		t.Fatalf("parseScheduleAddArgs returned error: %v", err)
	}

	if input.Name != "Daily review" {
		t.Fatalf("name = %q, want %q", input.Name, "Daily review")
	}
	if input.Objective != "Inspect repository status" {
		t.Fatalf("objective = %q, want %q", input.Objective, "Inspect repository status")
	}
	if input.WorkspaceRoot != "/tmp/repo" {
		t.Fatalf("workspace_root = %q, want %q", input.WorkspaceRoot, "/tmp/repo")
	}
	if input.Spec.Kind != scheduler.ScheduleKindAt || input.Spec.At != "2030-01-01T00:00:00Z" {
		t.Fatalf("spec = %#v, want at schedule", input.Spec)
	}
	if !input.Enabled {
		t.Fatal("expected schedules to default to enabled")
	}
}

func TestParseScheduleAddArgs_EveryScheduleRequiresStartAt(t *testing.T) {
	_, err := parseScheduleAddArgs([]string{
		"--name", "Hourly review",
		"--objective", "Inspect repository status",
		"--every", "1h",
	})
	if err == nil {
		t.Fatal("expected parseScheduleAddArgs to reject missing --start-at for --every")
	}
}

func TestParseScheduleUpdateArgs_EveryScheduleRequiresStartAt(t *testing.T) {
	_, err := parseScheduleUpdateArgs([]string{
		"--name", "Updated review",
		"--every", "2h",
	})
	if err == nil {
		t.Fatal("expected parseScheduleUpdateArgs to reject missing --start-at for --every")
	}
}

func TestRun_ScheduleLifecycleCommands(t *testing.T) {
	cfgPath, _ := writeCLIConfig(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{
		"schedule",
		"--config", cfgPath,
		"add",
		"--name", "Daily review",
		"--objective", "Inspect repository status",
		"--at", "2030-01-01T00:00:00Z",
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("schedule add failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	scheduleID := fieldValue(t, stdout.String(), "schedule_id")

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"schedule", "--config", cfgPath, "list"}, &stdout, &stderr); code != 0 {
		t.Fatalf("schedule list failed with code %d:\n%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), scheduleID) || !strings.Contains(stdout.String(), "Daily review") {
		t.Fatalf("schedule list missing schedule:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"schedule", "--config", cfgPath, "show", scheduleID}, &stdout, &stderr); code != 0 {
		t.Fatalf("schedule show failed with code %d:\n%s", code, stderr.String())
	}
	for _, want := range []string{
		"schedule_id: " + scheduleID,
		"name: Daily review",
		"objective: Inspect repository status",
		"kind: at",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("schedule show missing %q:\n%s", want, stdout.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{
		"schedule",
		"--config", cfgPath,
		"update", scheduleID,
		"--name", "Updated review",
		"--objective", "Inspect repository status after the update",
		"--every", "2h",
		"--start-at", "2030-01-01T08:00:00Z",
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("schedule update failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "name: Updated review") || !strings.Contains(stdout.String(), "kind: every") {
		t.Fatalf("schedule update output missing updated fields:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"schedule", "--config", cfgPath, "disable", scheduleID}, &stdout, &stderr); code != 0 {
		t.Fatalf("schedule disable failed with code %d:\n%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "enabled: false") {
		t.Fatalf("schedule disable output missing disabled state:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"schedule", "--config", cfgPath, "enable", scheduleID}, &stdout, &stderr); code != 0 {
		t.Fatalf("schedule enable failed with code %d:\n%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "enabled: true") {
		t.Fatalf("schedule enable output missing enabled state:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"schedule", "--config", cfgPath, "delete", scheduleID}, &stdout, &stderr); code != 0 {
		t.Fatalf("schedule delete failed with code %d:\n%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "deleted: true") {
		t.Fatalf("schedule delete output missing delete marker:\n%s", stdout.String())
	}
}
