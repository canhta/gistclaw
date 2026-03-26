package scheduler

import "time"

type ScheduleKind string

const (
	ScheduleKindAt    ScheduleKind = "at"
	ScheduleKindEvery ScheduleKind = "every"
	ScheduleKindCron  ScheduleKind = "cron"
)

type OccurrenceStatus string

const (
	OccurrenceDispatching   OccurrenceStatus = "dispatching"
	OccurrenceActive        OccurrenceStatus = "active"
	OccurrenceNeedsApproval OccurrenceStatus = "needs_approval"
	OccurrenceCompleted     OccurrenceStatus = "completed"
	OccurrenceFailed        OccurrenceStatus = "failed"
	OccurrenceInterrupted   OccurrenceStatus = "interrupted"
	OccurrenceSkipped       OccurrenceStatus = "skipped"
)

type ScheduleSpec struct {
	Kind         ScheduleKind
	At           string
	EverySeconds int64
	CronExpr     string
	Timezone     string
}

type CreateScheduleInput struct {
	ID            string
	Name          string
	Objective     string
	WorkspaceRoot string
	Spec          ScheduleSpec
	Enabled       bool
}

type UpdateScheduleInput struct {
	Name          *string
	Objective     *string
	WorkspaceRoot *string
	Spec          *ScheduleSpec
}

type StatusSummary struct {
	Enabled           bool
	TotalSchedules    int
	EnabledSchedules  int
	DueSchedules      int
	ActiveOccurrences int
	NextWakeAt        time.Time
	LastFailure       *FailureSummary
}

type FailureSummary struct {
	ScheduleID string
	Name       string
	Error      string
	FailedAt   time.Time
}

type HealthSummary struct {
	InvalidSchedules int
	StuckDispatching int
	MissingNextRun   int
}

type Schedule struct {
	ID                  string
	Name                string
	Objective           string
	WorkspaceRoot       string
	Spec                ScheduleSpec
	Enabled             bool
	NextRunAt           time.Time
	LastRunAt           time.Time
	LastStatus          OccurrenceStatus
	LastError           string
	ConsecutiveFailures int
	ScheduleErrorCount  int
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type ClaimedOccurrence struct {
	Schedule   Schedule
	Occurrence Occurrence
}

type Occurrence struct {
	ID             string
	ScheduleID     string
	SlotAt         time.Time
	ThreadID       string
	Status         OccurrenceStatus
	SkipReason     string
	RunID          string
	ConversationID string
	Error          string
	StartedAt      time.Time
	FinishedAt     time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
