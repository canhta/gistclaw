package scheduler

import (
	"testing"
	"time"
)

func t0(minute, hour, mday, month, wday int) time.Time {
	// year 2025, a known reference; wday is ignored in construction —
	// we pick a date that has the right weekday.
	return time.Date(2025, time.Month(month), mday, hour, minute, 0, 0, time.UTC)
}

func TestParseCron_InvalidFieldCount(t *testing.T) {
	for _, expr := range []string{"* * * *", "* * * * * *", "", "bad"} {
		if _, err := ParseCron(expr); err == nil {
			t.Errorf("ParseCron(%q): expected error, got nil", expr)
		}
	}
}

func TestParseCron_InvalidValues(t *testing.T) {
	cases := []string{
		"60 * * * *",  // minute out of range
		"* 24 * * *",  // hour out of range
		"* * 32 * *",  // day out of range
		"* * * 13 *",  // month out of range
		"* * * * 7",   // weekday out of range (0-6)
		"abc * * * *", // non-numeric
	}
	for _, expr := range cases {
		if _, err := ParseCron(expr); err == nil {
			t.Errorf("ParseCron(%q): expected error, got nil", expr)
		}
	}
}

func TestCronMatches_EveryMinute(t *testing.T) {
	c, err := ParseCron("* * * * *")
	if err != nil {
		t.Fatalf("ParseCron: %v", err)
	}
	for _, ts := range []time.Time{
		t0(0, 0, 1, 1, 0),
		t0(30, 12, 15, 6, 3),
		t0(59, 23, 31, 12, 6),
	} {
		if !c.Matches(ts) {
			t.Errorf("expected * * * * * to match %v", ts)
		}
	}
}

func TestCronMatches_ExactTime(t *testing.T) {
	c, err := ParseCron("30 9 * * *")
	if err != nil {
		t.Fatalf("ParseCron: %v", err)
	}
	if !c.Matches(t0(30, 9, 1, 1, 0)) {
		t.Error("expected match at 09:30")
	}
	if c.Matches(t0(31, 9, 1, 1, 0)) {
		t.Error("expected no match at 09:31")
	}
	if c.Matches(t0(30, 10, 1, 1, 0)) {
		t.Error("expected no match at 10:30")
	}
}

func TestCronMatches_Step(t *testing.T) {
	c, err := ParseCron("*/15 * * * *")
	if err != nil {
		t.Fatalf("ParseCron: %v", err)
	}
	for _, min := range []int{0, 15, 30, 45} {
		if !c.Matches(t0(min, 0, 1, 1, 0)) {
			t.Errorf("expected */15 to match minute %d", min)
		}
	}
	for _, min := range []int{1, 14, 16, 29, 31} {
		if c.Matches(t0(min, 0, 1, 1, 0)) {
			t.Errorf("expected */15 NOT to match minute %d", min)
		}
	}
}

func TestCronMatches_Range(t *testing.T) {
	c, err := ParseCron("* 9-17 * * *")
	if err != nil {
		t.Fatalf("ParseCron: %v", err)
	}
	for _, hour := range []int{9, 12, 17} {
		if !c.Matches(t0(0, hour, 1, 1, 0)) {
			t.Errorf("expected 9-17 range to match hour %d", hour)
		}
	}
	for _, hour := range []int{8, 18, 0} {
		if c.Matches(t0(0, hour, 1, 1, 0)) {
			t.Errorf("expected 9-17 range NOT to match hour %d", hour)
		}
	}
}

func TestCronMatches_CommaList(t *testing.T) {
	c, err := ParseCron("0 8,12,18 * * *")
	if err != nil {
		t.Fatalf("ParseCron: %v", err)
	}
	for _, hour := range []int{8, 12, 18} {
		if !c.Matches(t0(0, hour, 1, 1, 0)) {
			t.Errorf("expected comma list to match hour %d", hour)
		}
	}
	if c.Matches(t0(0, 9, 1, 1, 0)) {
		t.Error("expected comma list NOT to match hour 9")
	}
}

func TestCronMatches_Weekday(t *testing.T) {
	// Monday = 1 in standard cron (0=Sunday, 1=Monday, ..., 6=Saturday)
	// 2025-03-17 is a Monday.
	c, err := ParseCron("0 9 * * 1")
	if err != nil {
		t.Fatalf("ParseCron: %v", err)
	}
	monday := time.Date(2025, 3, 17, 9, 0, 0, 0, time.UTC) // Monday
	tuesday := time.Date(2025, 3, 18, 9, 0, 0, 0, time.UTC)
	if !c.Matches(monday) {
		t.Error("expected match on Monday")
	}
	if c.Matches(tuesday) {
		t.Error("expected no match on Tuesday")
	}
}
