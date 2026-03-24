package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronExpr holds the parsed field sets for a 5-field cron expression.
// Fields: minute hour day-of-month month day-of-week
// Each field is represented as a set of matching integers.
type CronExpr struct {
	minutes  []bool // [0..59]
	hours    []bool // [0..23]
	mdays    []bool // [1..31]
	months   []bool // [1..12]
	weekdays []bool // [0..6] Sunday=0
}

// ParseCron parses a standard 5-field cron expression.
// Supports: * (any), N (literal), */N (step), N-M (range), comma lists.
func ParseCron(expr string) (CronExpr, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return CronExpr{}, fmt.Errorf("cron: expected 5 fields, got %d in %q", len(fields), expr)
	}

	var c CronExpr
	var err error
	if c.minutes, err = parseField(fields[0], 0, 59); err != nil {
		return CronExpr{}, fmt.Errorf("cron: minute: %w", err)
	}
	if c.hours, err = parseField(fields[1], 0, 23); err != nil {
		return CronExpr{}, fmt.Errorf("cron: hour: %w", err)
	}
	if c.mdays, err = parseField(fields[2], 1, 31); err != nil {
		return CronExpr{}, fmt.Errorf("cron: day-of-month: %w", err)
	}
	if c.months, err = parseField(fields[3], 1, 12); err != nil {
		return CronExpr{}, fmt.Errorf("cron: month: %w", err)
	}
	if c.weekdays, err = parseField(fields[4], 0, 6); err != nil {
		return CronExpr{}, fmt.Errorf("cron: day-of-week: %w", err)
	}
	return c, nil
}

// Matches reports whether the cron expression fires at the given time.
// Seconds and sub-minute precision are ignored.
func (c CronExpr) Matches(t time.Time) bool {
	return c.minutes[t.Minute()] &&
		c.hours[t.Hour()] &&
		c.mdays[t.Day()] &&
		c.months[int(t.Month())] &&
		c.weekdays[int(t.Weekday())]
}

// ── Field parser ───────────────────────────────────────────────────────────────

// parseField returns a boolean slice [0..max] where slot i is true if the
// cron field expression covers value i.
func parseField(field string, min, max int) ([]bool, error) {
	bits := make([]bool, max+1)

	for _, part := range strings.Split(field, ",") {
		if err := applyPart(bits, part, min, max); err != nil {
			return nil, err
		}
	}
	return bits, nil
}

func applyPart(bits []bool, part string, min, max int) error {
	// */step
	if strings.HasPrefix(part, "*/") {
		step, err := atoi(part[2:])
		if err != nil {
			return fmt.Errorf("invalid step %q", part)
		}
		if step <= 0 {
			return fmt.Errorf("step must be > 0, got %d", step)
		}
		for i := min; i <= max; i++ {
			if (i-min)%step == 0 {
				bits[i] = true
			}
		}
		return nil
	}

	// *
	if part == "*" {
		for i := min; i <= max; i++ {
			bits[i] = true
		}
		return nil
	}

	// N-M range
	if idx := strings.Index(part, "-"); idx >= 0 {
		lo, err := atoi(part[:idx])
		if err != nil {
			return fmt.Errorf("invalid range start in %q", part)
		}
		hi, err := atoi(part[idx+1:])
		if err != nil {
			return fmt.Errorf("invalid range end in %q", part)
		}
		if lo < min || hi > max || lo > hi {
			return fmt.Errorf("range %d-%d out of bounds [%d,%d]", lo, hi, min, max)
		}
		for i := lo; i <= hi; i++ {
			bits[i] = true
		}
		return nil
	}

	// literal N
	n, err := atoi(part)
	if err != nil {
		return fmt.Errorf("invalid value %q", part)
	}
	if n < min || n > max {
		return fmt.Errorf("value %d out of range [%d,%d]", n, min, max)
	}
	bits[n] = true
	return nil
}

func atoi(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number", s)
	}
	return n, nil
}
