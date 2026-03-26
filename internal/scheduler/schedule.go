package scheduler

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

var cronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

func ValidateSpec(spec ScheduleSpec) error {
	switch spec.Kind {
	case ScheduleKindAt:
		if spec.EverySeconds != 0 {
			return fmt.Errorf("scheduler: at schedules do not accept intervals")
		}
		if strings.TrimSpace(spec.CronExpr) != "" {
			return fmt.Errorf("scheduler: at schedules do not accept cron expressions")
		}
		if strings.TrimSpace(spec.Timezone) != "" {
			return fmt.Errorf("scheduler: at schedules do not accept timezones")
		}
		if _, err := parseScheduleTimestamp(spec.At); err != nil {
			return err
		}
		return nil
	case ScheduleKindEvery:
		if spec.EverySeconds <= 0 {
			return fmt.Errorf("scheduler: every schedules require a positive interval")
		}
		if strings.TrimSpace(spec.CronExpr) != "" {
			return fmt.Errorf("scheduler: every schedules do not accept cron expressions")
		}
		if strings.TrimSpace(spec.Timezone) != "" {
			return fmt.Errorf("scheduler: every schedules do not accept timezones")
		}
		if _, err := parseScheduleTimestamp(spec.At); err != nil {
			return err
		}
		return nil
	case ScheduleKindCron:
		if strings.TrimSpace(spec.CronExpr) == "" {
			return fmt.Errorf("scheduler: cron schedules require an expression")
		}
		if strings.TrimSpace(spec.At) != "" {
			return fmt.Errorf("scheduler: cron schedules do not accept anchor timestamps")
		}
		if spec.EverySeconds != 0 {
			return fmt.Errorf("scheduler: cron schedules do not accept intervals")
		}
		if _, err := loadScheduleLocation(spec.Timezone); err != nil {
			return err
		}
		if _, err := cronParser.Parse(spec.CronExpr); err != nil {
			return fmt.Errorf("scheduler: parse cron expression: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("scheduler: unsupported schedule kind %q", spec.Kind)
	}
}

func ComputeNextRun(spec ScheduleSpec, now time.Time) (time.Time, error) {
	if err := ValidateSpec(spec); err != nil {
		return time.Time{}, err
	}

	now = now.UTC()

	switch spec.Kind {
	case ScheduleKindAt:
		at, err := parseScheduleTimestamp(spec.At)
		if err != nil {
			return time.Time{}, err
		}
		if at.Before(now) {
			return time.Time{}, nil
		}
		return at, nil
	case ScheduleKindEvery:
		anchor, err := parseScheduleTimestamp(spec.At)
		if err != nil {
			return time.Time{}, err
		}
		if !anchor.Before(now) {
			return anchor, nil
		}

		interval := time.Duration(spec.EverySeconds) * time.Second
		steps := now.Sub(anchor) / interval
		next := anchor.Add(steps * interval)
		if next.Before(now) {
			next = next.Add(interval)
		}
		return next.UTC(), nil
	case ScheduleKindCron:
		location, err := loadScheduleLocation(spec.Timezone)
		if err != nil {
			return time.Time{}, err
		}
		schedule, err := cronParser.Parse(spec.CronExpr)
		if err != nil {
			return time.Time{}, fmt.Errorf("scheduler: parse cron expression: %w", err)
		}
		next := schedule.Next(now.In(location).Add(-time.Nanosecond))
		return next.UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("scheduler: unsupported schedule kind %q", spec.Kind)
	}
}

func parseScheduleTimestamp(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("scheduler: timestamp is required")
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		ts, err := time.Parse(layout, raw)
		if err == nil {
			return ts.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("scheduler: timestamp must be RFC3339 with offset")
}

func loadScheduleLocation(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return time.UTC, nil
	}

	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("scheduler: load timezone %q: %w", name, err)
	}

	return location, nil
}
