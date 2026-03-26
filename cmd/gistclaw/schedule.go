package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/app"
	"github.com/canhta/gistclaw/internal/scheduler"
)

const scheduleUsage = `Usage: gistclaw schedule <subcommand> [options]

Subcommands:
  add       Create a schedule
  update    Update a schedule
  list      List schedules
  show      Show one schedule
  run       Trigger a schedule immediately
  enable    Enable a schedule
  disable   Disable a schedule
  delete    Delete a schedule

Add flags:
  --name TEXT
  --objective TEXT
  --workspace-root PATH
  --at RFC3339
  --every DURATION --start-at RFC3339
  --cron EXPR [--timezone IANA]
  --disabled
`

func runSchedule(configPath string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, scheduleUsage)
		return 1
	}

	application, err := loadApp(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "bootstrap app: %v\n", err)
		return 1
	}
	defer func() { _ = application.Stop() }()

	switch args[0] {
	case "add":
		input, err := parseScheduleAddArgs(args[1:])
		if err != nil {
			fmt.Fprintf(stderr, "schedule add: %v\n", err)
			return 1
		}
		schedule, err := application.CreateSchedule(context.Background(), input)
		if err != nil {
			fmt.Fprintf(stderr, "schedule add failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "schedule_id: %s\nenabled: %t\nkind: %s\nnext_run_at: %s\n",
			schedule.ID, schedule.Enabled, schedule.Spec.Kind, formatTime(schedule.NextRunAt))
		return 0
	case "update":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "Usage: gistclaw schedule update <schedule_id> [options]")
			return 1
		}
		patch, err := parseScheduleUpdateArgs(args[2:])
		if err != nil {
			fmt.Fprintf(stderr, "schedule update: %v\n", err)
			return 1
		}
		schedule, err := application.UpdateSchedule(context.Background(), args[1], patch)
		if err != nil {
			fmt.Fprintf(stderr, "schedule update failed: %v\n", err)
			return 1
		}
		printSchedule(stdout, schedule)
		return 0
	case "list":
		schedules, err := application.ListSchedules(context.Background())
		if err != nil {
			fmt.Fprintf(stderr, "schedule list failed: %v\n", err)
			return 1
		}
		for _, schedule := range schedules {
			fmt.Fprintf(stdout, "%s %t %s %s %s\n",
				schedule.ID, schedule.Enabled, schedule.Spec.Kind, formatTime(schedule.NextRunAt), schedule.Name)
		}
		return 0
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "Usage: gistclaw schedule show <schedule_id>")
			return 1
		}
		schedule, err := application.LoadSchedule(context.Background(), args[1])
		if err != nil {
			fmt.Fprintf(stderr, "schedule show failed: %v\n", err)
			return 1
		}
		printSchedule(stdout, schedule)
		return 0
	case "run":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "Usage: gistclaw schedule run <schedule_id>")
			return 1
		}
		claimed, err := application.RunScheduleNow(context.Background(), args[1])
		if err != nil {
			fmt.Fprintf(stderr, "schedule run failed: %v\n", err)
			return 1
		}
		if claimed == nil {
			fmt.Fprintln(stdout, "status: skipped")
			return 0
		}
		fmt.Fprintf(stdout, "occurrence_id: %s\nslot_at: %s\n", claimed.Occurrence.ID, formatTime(claimed.Occurrence.SlotAt))
		return 0
	case "enable":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "Usage: gistclaw schedule enable <schedule_id>")
			return 1
		}
		schedule, err := application.EnableSchedule(context.Background(), args[1])
		if err != nil {
			fmt.Fprintf(stderr, "schedule enable failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "schedule_id: %s\nenabled: %t\nnext_run_at: %s\n", schedule.ID, schedule.Enabled, formatTime(schedule.NextRunAt))
		return 0
	case "disable":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "Usage: gistclaw schedule disable <schedule_id>")
			return 1
		}
		schedule, err := application.DisableSchedule(context.Background(), args[1])
		if err != nil {
			fmt.Fprintf(stderr, "schedule disable failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "schedule_id: %s\nenabled: %t\n", schedule.ID, schedule.Enabled)
		return 0
	case "delete":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "Usage: gistclaw schedule delete <schedule_id>")
			return 1
		}
		if err := application.DeleteSchedule(context.Background(), args[1]); err != nil {
			fmt.Fprintf(stderr, "schedule delete failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "schedule_id: %s\ndeleted: true\n", args[1])
		return 0
	default:
		fmt.Fprintf(stderr, "unknown schedule subcommand: %s\n\n%s", args[0], scheduleUsage)
		return 1
	}
}

func parseScheduleAddArgs(args []string) (scheduler.CreateScheduleInput, error) {
	var input scheduler.CreateScheduleInput
	enabled := true
	var flags scheduleSpecFlags

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			value, err := scheduleFlagValue(args, &i)
			if err != nil {
				return scheduler.CreateScheduleInput{}, err
			}
			input.Name = value
		case "--objective":
			value, err := scheduleFlagValue(args, &i)
			if err != nil {
				return scheduler.CreateScheduleInput{}, err
			}
			input.Objective = value
		case "--workspace-root":
			value, err := scheduleFlagValue(args, &i)
			if err != nil {
				return scheduler.CreateScheduleInput{}, err
			}
			input.WorkspaceRoot = value
		case "--at":
			value, err := scheduleFlagValue(args, &i)
			if err != nil {
				return scheduler.CreateScheduleInput{}, err
			}
			flags.at = value
		case "--every":
			value, err := scheduleFlagValue(args, &i)
			if err != nil {
				return scheduler.CreateScheduleInput{}, err
			}
			flags.every = value
		case "--start-at":
			value, err := scheduleFlagValue(args, &i)
			if err != nil {
				return scheduler.CreateScheduleInput{}, err
			}
			flags.startAt = value
		case "--cron":
			value, err := scheduleFlagValue(args, &i)
			if err != nil {
				return scheduler.CreateScheduleInput{}, err
			}
			flags.cron = value
		case "--timezone":
			value, err := scheduleFlagValue(args, &i)
			if err != nil {
				return scheduler.CreateScheduleInput{}, err
			}
			flags.timezone = value
		case "--disabled":
			enabled = false
		default:
			return scheduler.CreateScheduleInput{}, fmt.Errorf("unknown flag %q", args[i])
		}
	}

	if strings.TrimSpace(input.Name) == "" {
		return scheduler.CreateScheduleInput{}, fmt.Errorf("--name is required")
	}
	if strings.TrimSpace(input.Objective) == "" {
		return scheduler.CreateScheduleInput{}, fmt.Errorf("--objective is required")
	}

	spec, _, err := flags.build(true)
	if err != nil {
		return scheduler.CreateScheduleInput{}, err
	}

	input.Spec = spec
	input.Enabled = enabled
	return input, nil
}

func parseScheduleUpdateArgs(args []string) (scheduler.UpdateScheduleInput, error) {
	var patch scheduler.UpdateScheduleInput
	var flags scheduleSpecFlags

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			value, err := scheduleFlagValue(args, &i)
			if err != nil {
				return scheduler.UpdateScheduleInput{}, err
			}
			patch.Name = stringPtr(value)
		case "--objective":
			value, err := scheduleFlagValue(args, &i)
			if err != nil {
				return scheduler.UpdateScheduleInput{}, err
			}
			patch.Objective = stringPtr(value)
		case "--workspace-root":
			value, err := scheduleFlagValue(args, &i)
			if err != nil {
				return scheduler.UpdateScheduleInput{}, err
			}
			patch.WorkspaceRoot = stringPtr(value)
		case "--at":
			value, err := scheduleFlagValue(args, &i)
			if err != nil {
				return scheduler.UpdateScheduleInput{}, err
			}
			flags.at = value
		case "--every":
			value, err := scheduleFlagValue(args, &i)
			if err != nil {
				return scheduler.UpdateScheduleInput{}, err
			}
			flags.every = value
		case "--start-at":
			value, err := scheduleFlagValue(args, &i)
			if err != nil {
				return scheduler.UpdateScheduleInput{}, err
			}
			flags.startAt = value
		case "--cron":
			value, err := scheduleFlagValue(args, &i)
			if err != nil {
				return scheduler.UpdateScheduleInput{}, err
			}
			flags.cron = value
		case "--timezone":
			value, err := scheduleFlagValue(args, &i)
			if err != nil {
				return scheduler.UpdateScheduleInput{}, err
			}
			flags.timezone = value
		default:
			return scheduler.UpdateScheduleInput{}, fmt.Errorf("unknown flag %q", args[i])
		}
	}

	spec, ok, err := flags.build(false)
	if err != nil {
		return scheduler.UpdateScheduleInput{}, err
	}
	if ok {
		patch.Spec = &spec
	}
	if patch.Name == nil && patch.Objective == nil && patch.WorkspaceRoot == nil && patch.Spec == nil {
		return scheduler.UpdateScheduleInput{}, fmt.Errorf("at least one update flag is required")
	}
	return patch, nil
}

type scheduleSpecFlags struct {
	at       string
	every    string
	startAt  string
	cron     string
	timezone string
}

func (f scheduleSpecFlags) build(required bool) (scheduler.ScheduleSpec, bool, error) {
	kindCount := 0
	if strings.TrimSpace(f.at) != "" {
		kindCount++
	}
	if strings.TrimSpace(f.every) != "" {
		kindCount++
	}
	if strings.TrimSpace(f.cron) != "" {
		kindCount++
	}

	if kindCount == 0 {
		if strings.TrimSpace(f.startAt) != "" || strings.TrimSpace(f.timezone) != "" {
			return scheduler.ScheduleSpec{}, false, fmt.Errorf("--start-at requires --every and --timezone requires --cron")
		}
		if required {
			return scheduler.ScheduleSpec{}, false, fmt.Errorf("exactly one of --at, --every, or --cron is required")
		}
		return scheduler.ScheduleSpec{}, false, nil
	}
	if kindCount != 1 {
		return scheduler.ScheduleSpec{}, false, fmt.Errorf("exactly one of --at, --every, or --cron is required")
	}

	switch {
	case strings.TrimSpace(f.at) != "":
		if strings.TrimSpace(f.startAt) != "" || strings.TrimSpace(f.timezone) != "" {
			return scheduler.ScheduleSpec{}, false, fmt.Errorf("--at does not accept --start-at or --timezone")
		}
		return scheduler.ScheduleSpec{Kind: scheduler.ScheduleKindAt, At: f.at}, true, nil
	case strings.TrimSpace(f.every) != "":
		duration, err := time.ParseDuration(f.every)
		if err != nil {
			return scheduler.ScheduleSpec{}, false, fmt.Errorf("parse --every: %w", err)
		}
		if duration <= 0 || duration%time.Second != 0 {
			return scheduler.ScheduleSpec{}, false, fmt.Errorf("--every must be a positive whole-second duration")
		}
		if strings.TrimSpace(f.startAt) == "" {
			return scheduler.ScheduleSpec{}, false, fmt.Errorf("--start-at is required with --every")
		}
		if strings.TrimSpace(f.timezone) != "" {
			return scheduler.ScheduleSpec{}, false, fmt.Errorf("--every does not accept --timezone")
		}
		return scheduler.ScheduleSpec{
			Kind:         scheduler.ScheduleKindEvery,
			At:           f.startAt,
			EverySeconds: int64(duration / time.Second),
		}, true, nil
	default:
		if strings.TrimSpace(f.startAt) != "" {
			return scheduler.ScheduleSpec{}, false, fmt.Errorf("--cron does not accept --start-at")
		}
		return scheduler.ScheduleSpec{
			Kind:     scheduler.ScheduleKindCron,
			CronExpr: f.cron,
			Timezone: f.timezone,
		}, true, nil
	}
}

func scheduleFlagValue(args []string, idx *int) (string, error) {
	if *idx+1 >= len(args) {
		return "", fmt.Errorf("missing value for %s", args[*idx])
	}
	*idx = *idx + 1
	return args[*idx], nil
}

func printSchedule(w io.Writer, schedule scheduler.Schedule) {
	fmt.Fprintf(w, "schedule_id: %s\n", schedule.ID)
	fmt.Fprintf(w, "name: %s\n", schedule.Name)
	fmt.Fprintf(w, "objective: %s\n", schedule.Objective)
	fmt.Fprintf(w, "workspace_root: %s\n", schedule.WorkspaceRoot)
	fmt.Fprintf(w, "enabled: %t\n", schedule.Enabled)
	fmt.Fprintf(w, "kind: %s\n", schedule.Spec.Kind)
	fmt.Fprintf(w, "at: %s\n", schedule.Spec.At)
	fmt.Fprintf(w, "every_seconds: %d\n", schedule.Spec.EverySeconds)
	fmt.Fprintf(w, "cron: %s\n", schedule.Spec.CronExpr)
	fmt.Fprintf(w, "timezone: %s\n", schedule.Spec.Timezone)
	fmt.Fprintf(w, "next_run_at: %s\n", formatTime(schedule.NextRunAt))
	fmt.Fprintf(w, "last_run_at: %s\n", formatTime(schedule.LastRunAt))
	fmt.Fprintf(w, "last_status: %s\n", schedule.LastStatus)
	fmt.Fprintf(w, "last_error: %s\n", schedule.LastError)
}

func formatTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}

func loadScheduleApp(configPath string) (*app.App, error) {
	return loadApp(configPath)
}

func stringPtr(value string) *string {
	return &value
}
