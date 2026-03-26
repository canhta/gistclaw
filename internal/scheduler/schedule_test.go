package scheduler

import (
	"testing"
	"time"
)

func TestComputeNextRun_AtScheduleFiresOnce(t *testing.T) {
	t.Parallel()

	now := mustParseRFC3339(t, "2026-03-26T10:00:00Z")

	tests := []struct {
		name string
		spec ScheduleSpec
		want time.Time
	}{
		{
			name: "future slot remains scheduled",
			spec: ScheduleSpec{
				Kind: ScheduleKindAt,
				At:   "2026-03-26T17:30:00+07:00",
			},
			want: mustParseRFC3339(t, "2026-03-26T10:30:00Z"),
		},
		{
			name: "expired slot has no next run",
			spec: ScheduleSpec{
				Kind: ScheduleKindAt,
				At:   "2026-03-26T15:30:00+07:00",
			},
			want: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ComputeNextRun(tt.spec, now)
			if err != nil {
				t.Fatalf("ComputeNextRun returned error: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("ComputeNextRun returned %s, want %s", got.Format(time.RFC3339), tt.want.Format(time.RFC3339))
			}
		})
	}
}

func TestComputeNextRun_EveryScheduleAnchorsToScheduledSlot(t *testing.T) {
	t.Parallel()

	spec := ScheduleSpec{
		Kind:         ScheduleKindEvery,
		At:           "2026-03-26T09:00:00+07:00",
		EverySeconds: int64((2 * time.Hour) / time.Second),
	}

	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "before anchor returns anchor",
			now:  mustParseRFC3339(t, "2026-03-26T01:30:00Z"),
			want: mustParseRFC3339(t, "2026-03-26T02:00:00Z"),
		},
		{
			name: "between slots returns next scheduled slot",
			now:  mustParseRFC3339(t, "2026-03-26T04:45:00Z"),
			want: mustParseRFC3339(t, "2026-03-26T06:00:00Z"),
		},
		{
			name: "exact slot stays on that slot",
			now:  mustParseRFC3339(t, "2026-03-26T06:00:00Z"),
			want: mustParseRFC3339(t, "2026-03-26T06:00:00Z"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ComputeNextRun(spec, tt.now)
			if err != nil {
				t.Fatalf("ComputeNextRun returned error: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("ComputeNextRun returned %s, want %s", got.Format(time.RFC3339), tt.want.Format(time.RFC3339))
			}
		})
	}
}

func TestComputeNextRun_CronScheduleUsesTimezone(t *testing.T) {
	t.Parallel()

	spec := ScheduleSpec{
		Kind:     ScheduleKindCron,
		CronExpr: "30 9 * * *",
		Timezone: "Asia/Ho_Chi_Minh",
	}
	now := mustParseRFC3339(t, "2026-03-26T01:00:00Z")

	got, err := ComputeNextRun(spec, now)
	if err != nil {
		t.Fatalf("ComputeNextRun returned error: %v", err)
	}

	want := mustParseRFC3339(t, "2026-03-26T02:30:00Z")
	if !got.Equal(want) {
		t.Fatalf("ComputeNextRun returned %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestComputeNextRun_CronScheduleRejectsInvalidExpression(t *testing.T) {
	t.Parallel()

	spec := ScheduleSpec{
		Kind:     ScheduleKindCron,
		CronExpr: "not a cron spec",
	}

	if _, err := ComputeNextRun(spec, time.Now().UTC()); err == nil {
		t.Fatal("ComputeNextRun returned nil error for invalid cron expression")
	}
}

func TestValidateSpec_RejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec ScheduleSpec
	}{
		{
			name: "unsupported kind",
			spec: ScheduleSpec{Kind: "weekly"},
		},
		{
			name: "at missing offset timestamp",
			spec: ScheduleSpec{
				Kind: ScheduleKindAt,
				At:   "2026-03-26T09:00:00",
			},
		},
		{
			name: "every missing anchor",
			spec: ScheduleSpec{
				Kind:         ScheduleKindEvery,
				EverySeconds: 3600,
			},
		},
		{
			name: "cron invalid timezone",
			spec: ScheduleSpec{
				Kind:     ScheduleKindCron,
				CronExpr: "0 9 * * *",
				Timezone: "Mars/Base",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateSpec(tt.spec); err == nil {
				t.Fatal("ValidateSpec returned nil error")
			}
		})
	}
}

func mustParseRFC3339(t *testing.T, raw string) time.Time {
	t.Helper()

	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("Parse(%q): %v", raw, err)
	}

	return ts.UTC()
}
