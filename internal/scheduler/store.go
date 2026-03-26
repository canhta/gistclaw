package scheduler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/model"
	storepkg "github.com/canhta/gistclaw/internal/store"
)

type rowScanner interface {
	Scan(dest ...any) error
}

type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Store struct {
	db  *storepkg.DB
	now func() time.Time
}

func NewStore(db *storepkg.DB) *Store {
	return &Store{
		db: db,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *Store) CreateSchedule(ctx context.Context, in CreateScheduleInput) (Schedule, error) {
	if strings.TrimSpace(in.Name) == "" {
		return Schedule{}, fmt.Errorf("scheduler: create schedule: name required")
	}
	if strings.TrimSpace(in.Objective) == "" {
		return Schedule{}, fmt.Errorf("scheduler: create schedule: objective required")
	}
	if strings.TrimSpace(in.WorkspaceRoot) == "" {
		return Schedule{}, fmt.Errorf("scheduler: create schedule: workspace_root required")
	}
	if err := ValidateSpec(in.Spec); err != nil {
		return Schedule{}, fmt.Errorf("scheduler: create schedule: %w", err)
	}

	id := strings.TrimSpace(in.ID)
	if id == "" {
		var err error
		id, err = generateSchedulerID()
		if err != nil {
			return Schedule{}, fmt.Errorf("scheduler: create schedule: %w", err)
		}
	}

	now := s.now().UTC()
	nextRunAt := time.Time{}
	if in.Enabled {
		var err error
		nextRunAt, err = ComputeNextRun(in.Spec, now)
		if err != nil {
			return Schedule{}, fmt.Errorf("scheduler: create schedule: compute next run: %w", err)
		}
	}

	_, err := s.db.RawDB().ExecContext(ctx,
		`INSERT INTO schedules
		 (id, name, objective, workspace_root, schedule_kind, schedule_at, schedule_every_seconds,
		  schedule_cron_expr, timezone, enabled, next_run_at, last_run_at, last_status, last_error,
		  consecutive_failures, schedule_error_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, '', '', 0, 0, ?, ?)`,
		id,
		strings.TrimSpace(in.Name),
		strings.TrimSpace(in.Objective),
		strings.TrimSpace(in.WorkspaceRoot),
		in.Spec.Kind,
		strings.TrimSpace(in.Spec.At),
		in.Spec.EverySeconds,
		strings.TrimSpace(in.Spec.CronExpr),
		strings.TrimSpace(in.Spec.Timezone),
		boolToInt(in.Enabled),
		nullableTime(nextRunAt),
		now,
		now,
	)
	if err != nil {
		return Schedule{}, fmt.Errorf("scheduler: create schedule: %w", err)
	}

	return s.LoadSchedule(ctx, id)
}

func (s *Store) LoadSchedule(ctx context.Context, id string) (Schedule, error) {
	schedule, err := loadScheduleRow(ctx, s.db.RawDB(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			return Schedule{}, err
		}
		return Schedule{}, fmt.Errorf("scheduler: load schedule: %w", err)
	}
	return schedule, nil
}

func (s *Store) ListSchedules(ctx context.Context) ([]Schedule, error) {
	rows, err := s.db.RawDB().QueryContext(ctx,
		`SELECT id, name, objective, workspace_root, schedule_kind, schedule_at, schedule_every_seconds,
		        schedule_cron_expr, timezone, enabled, next_run_at, last_run_at, last_status, last_error,
		        consecutive_failures, schedule_error_count, created_at, updated_at
		   FROM schedules
		  ORDER BY created_at ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("scheduler: list schedules: %w", err)
	}
	defer rows.Close()

	var schedules []Schedule
	for rows.Next() {
		schedule, err := scanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("scheduler: list schedules: %w", err)
		}
		schedules = append(schedules, schedule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scheduler: list schedules: %w", err)
	}

	return schedules, nil
}

func (s *Store) ListDueSchedules(ctx context.Context, now time.Time, limit int) ([]Schedule, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.RawDB().QueryContext(ctx,
		`SELECT id, name, objective, workspace_root, schedule_kind, schedule_at, schedule_every_seconds,
		        schedule_cron_expr, timezone, enabled, next_run_at, last_run_at, last_status, last_error,
		        consecutive_failures, schedule_error_count, created_at, updated_at
		   FROM schedules
		  WHERE enabled = 1
		    AND next_run_at IS NOT NULL
		    AND next_run_at <= ?
		  ORDER BY next_run_at ASC, id ASC
		  LIMIT ?`,
		now.UTC(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("scheduler: list due schedules: %w", err)
	}
	defer rows.Close()

	var schedules []Schedule
	for rows.Next() {
		schedule, err := scanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("scheduler: list due schedules: %w", err)
		}
		schedules = append(schedules, schedule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scheduler: list due schedules: %w", err)
	}

	return schedules, nil
}

func (s *Store) ClaimDueOccurrence(ctx context.Context, scheduleID string, now time.Time) (*ClaimedOccurrence, error) {
	now = now.UTC()

	var claimed *ClaimedOccurrence
	err := s.db.Tx(ctx, func(tx *sql.Tx) error {
		schedule, err := loadScheduleRow(ctx, tx, scheduleID)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return fmt.Errorf("scheduler: claim due occurrence: load schedule: %w", err)
		}
		if !schedule.Enabled || schedule.NextRunAt.IsZero() || schedule.NextRunAt.After(now) {
			return nil
		}

		slotAt := schedule.NextRunAt.UTC()
		nextRunAt, err := ComputeNextRun(schedule.Spec, now.Add(time.Nanosecond))
		if err != nil {
			return fmt.Errorf("scheduler: claim due occurrence: compute next run: %w", err)
		}

		hasPreviousActive, err := hasPreviousActiveOccurrence(ctx, tx, schedule.ID, slotAt)
		if err != nil {
			return fmt.Errorf("scheduler: claim due occurrence: load active occurrence: %w", err)
		}

		occurrenceID, err := generateSchedulerID()
		if err != nil {
			return fmt.Errorf("scheduler: claim due occurrence: %w", err)
		}
		occurrence := Occurrence{
			ID:         occurrenceID,
			ScheduleID: schedule.ID,
			SlotAt:     slotAt,
			ThreadID:   slotAt.Format(time.RFC3339Nano),
			Status:     OccurrenceDispatching,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if hasPreviousActive {
			occurrence.Status = OccurrenceSkipped
			occurrence.SkipReason = "previous_occurrence_active"
		}

		_, err = tx.ExecContext(ctx,
			`INSERT INTO schedule_occurrences
			 (id, schedule_id, slot_at, thread_id, status, skip_reason, run_id, conversation_id, error,
			  started_at, finished_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, '', '', '', NULL, NULL, ?, ?)`,
			occurrence.ID,
			occurrence.ScheduleID,
			occurrence.SlotAt,
			occurrence.ThreadID,
			occurrence.Status,
			occurrence.SkipReason,
			occurrence.CreatedAt,
			occurrence.UpdatedAt,
		)
		if err != nil {
			if !storepkg.IsSQLiteConstraintUnique(err) {
				return fmt.Errorf("scheduler: claim due occurrence: insert occurrence: %w", err)
			}
			if err := updateScheduleNextRun(ctx, tx, schedule.ID, nextRunAt, now); err != nil {
				return fmt.Errorf("scheduler: claim due occurrence: advance duplicate slot: %w", err)
			}
			return nil
		}

		if err := updateScheduleNextRun(ctx, tx, schedule.ID, nextRunAt, now); err != nil {
			return fmt.Errorf("scheduler: claim due occurrence: advance schedule: %w", err)
		}

		if occurrence.Status == OccurrenceDispatching {
			schedule.NextRunAt = nextRunAt
			claimed = &ClaimedOccurrence{
				Schedule:   schedule,
				Occurrence: occurrence,
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return claimed, nil
}

func (s *Store) SetScheduleEnabled(ctx context.Context, scheduleID string, enabled bool, now time.Time) (Schedule, error) {
	schedule, err := s.LoadSchedule(ctx, scheduleID)
	if err != nil {
		return Schedule{}, err
	}

	nextRunAt := time.Time{}
	if enabled {
		nextRunAt, err = ComputeNextRun(schedule.Spec, now.UTC())
		if err != nil {
			return Schedule{}, fmt.Errorf("scheduler: set schedule enabled: %w", err)
		}
	}

	_, err = s.db.RawDB().ExecContext(ctx,
		"UPDATE schedules SET enabled = ?, next_run_at = ?, updated_at = ? WHERE id = ?",
		boolToInt(enabled),
		nullableTime(nextRunAt),
		now.UTC(),
		scheduleID,
	)
	if err != nil {
		return Schedule{}, fmt.Errorf("scheduler: set schedule enabled: %w", err)
	}

	return s.LoadSchedule(ctx, scheduleID)
}

func (s *Store) DeleteSchedule(ctx context.Context, scheduleID string) error {
	if _, err := s.db.RawDB().ExecContext(ctx, "DELETE FROM schedules WHERE id = ?", scheduleID); err != nil {
		return fmt.Errorf("scheduler: delete schedule: %w", err)
	}
	return nil
}

func (s *Store) ClaimManualOccurrence(ctx context.Context, scheduleID string, slotAt time.Time) (*ClaimedOccurrence, error) {
	slotAt = slotAt.UTC()

	var claimed *ClaimedOccurrence
	err := s.db.Tx(ctx, func(tx *sql.Tx) error {
		schedule, err := loadScheduleRow(ctx, tx, scheduleID)
		if err != nil {
			return fmt.Errorf("scheduler: claim manual occurrence: load schedule: %w", err)
		}

		hasPreviousActive, err := hasPreviousActiveOccurrence(ctx, tx, schedule.ID, slotAt)
		if err != nil {
			return fmt.Errorf("scheduler: claim manual occurrence: load active occurrence: %w", err)
		}

		occurrenceID, err := generateSchedulerID()
		if err != nil {
			return fmt.Errorf("scheduler: claim manual occurrence: %w", err)
		}
		occurrence := Occurrence{
			ID:         occurrenceID,
			ScheduleID: schedule.ID,
			SlotAt:     slotAt,
			ThreadID:   slotAt.Format(time.RFC3339Nano),
			Status:     OccurrenceDispatching,
			CreatedAt:  slotAt,
			UpdatedAt:  slotAt,
		}
		if hasPreviousActive {
			occurrence.Status = OccurrenceSkipped
			occurrence.SkipReason = "previous_occurrence_active"
		}

		_, err = tx.ExecContext(ctx,
			`INSERT INTO schedule_occurrences
			 (id, schedule_id, slot_at, thread_id, status, skip_reason, run_id, conversation_id, error,
			  started_at, finished_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, '', '', '', NULL, NULL, ?, ?)`,
			occurrence.ID,
			occurrence.ScheduleID,
			occurrence.SlotAt,
			occurrence.ThreadID,
			occurrence.Status,
			occurrence.SkipReason,
			occurrence.CreatedAt,
			occurrence.UpdatedAt,
		)
		if err != nil {
			if storepkg.IsSQLiteConstraintUnique(err) {
				return nil
			}
			return fmt.Errorf("scheduler: claim manual occurrence: insert occurrence: %w", err)
		}

		if occurrence.Status == OccurrenceDispatching {
			claimed = &ClaimedOccurrence{
				Schedule:   schedule,
				Occurrence: occurrence,
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func (s *Store) ListOpenOccurrences(ctx context.Context) ([]Occurrence, error) {
	rows, err := s.db.RawDB().QueryContext(ctx,
		`SELECT id, schedule_id, slot_at, thread_id, status, skip_reason, run_id, conversation_id, error,
		        started_at, finished_at, created_at, updated_at
		   FROM schedule_occurrences
		  WHERE status IN (?, ?, ?)
		  ORDER BY created_at ASC, id ASC`,
		OccurrenceDispatching,
		OccurrenceActive,
		OccurrenceNeedsApproval,
	)
	if err != nil {
		return nil, fmt.Errorf("scheduler: list open occurrences: %w", err)
	}
	defer rows.Close()

	var occurrences []Occurrence
	for rows.Next() {
		occurrence, err := scanOccurrence(rows)
		if err != nil {
			return nil, fmt.Errorf("scheduler: list open occurrences: %w", err)
		}
		occurrences = append(occurrences, occurrence)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scheduler: list open occurrences: %w", err)
	}
	return occurrences, nil
}

func (s *Store) RecoverRunFromReceipt(ctx context.Context, occurrence Occurrence) (model.Run, bool, error) {
	var runID string
	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT run_id
		   FROM inbound_receipts
		  WHERE connector_id = 'schedule'
		    AND account_id = 'local'
		    AND thread_id = ?
		    AND source_message_id = ?
		  ORDER BY created_at DESC, id DESC
		  LIMIT 1`,
		occurrence.ThreadID,
		occurrence.ID,
	).Scan(&runID)
	if err == sql.ErrNoRows {
		return model.Run{}, false, nil
	}
	if err != nil {
		return model.Run{}, false, fmt.Errorf("scheduler: recover run from receipt: %w", err)
	}

	run, err := s.LoadRun(ctx, runID)
	if err != nil {
		return model.Run{}, false, err
	}
	return run, true, nil
}

func (s *Store) LoadRun(ctx context.Context, runID string) (model.Run, error) {
	var run model.Run
	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT id, conversation_id, status
		   FROM runs
		  WHERE id = ?`,
		runID,
	).Scan(&run.ID, &run.ConversationID, &run.Status)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.Run{}, err
		}
		return model.Run{}, fmt.Errorf("scheduler: load run: %w", err)
	}
	return run, nil
}

func (s *Store) MarkOccurrenceAccepted(ctx context.Context, occurrenceID string, run model.Run, startedAt time.Time) error {
	return s.db.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE schedule_occurrences
			    SET run_id = ?, conversation_id = ?, status = ?, started_at = ?, updated_at = ?
			  WHERE id = ?`,
			run.ID,
			run.ConversationID,
			OccurrenceActive,
			startedAt.UTC(),
			startedAt.UTC(),
			occurrenceID,
		)
		if err != nil {
			return fmt.Errorf("scheduler: mark occurrence accepted: %w", err)
		}
		return nil
	})
}

func (s *Store) MarkOccurrenceFailed(ctx context.Context, occurrenceID, errorText string, finishedAt time.Time) error {
	return s.db.Tx(ctx, func(tx *sql.Tx) error {
		occurrence, err := loadOccurrenceRow(ctx, tx, occurrenceID)
		if err != nil {
			return fmt.Errorf("scheduler: mark occurrence failed: load occurrence: %w", err)
		}
		schedule, err := loadScheduleRow(ctx, tx, occurrence.ScheduleID)
		if err != nil {
			return fmt.Errorf("scheduler: mark occurrence failed: load schedule: %w", err)
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE schedule_occurrences
			    SET status = ?, error = ?, finished_at = ?, updated_at = ?
			  WHERE id = ?`,
			OccurrenceFailed,
			errorText,
			finishedAt.UTC(),
			finishedAt.UTC(),
			occurrenceID,
		); err != nil {
			return fmt.Errorf("scheduler: mark occurrence failed: update occurrence: %w", err)
		}

		consecutiveFailures := 1
		if !schedule.LastRunAt.IsZero() && schedule.LastStatus == OccurrenceFailed {
			consecutiveFailures = schedule.ConsecutiveFailures + 1
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE schedules
			    SET last_run_at = ?, last_status = ?, last_error = ?, consecutive_failures = ?, updated_at = ?
			  WHERE id = ?`,
			occurrence.SlotAt.UTC(),
			OccurrenceFailed,
			errorText,
			consecutiveFailures,
			finishedAt.UTC(),
			schedule.ID,
		); err != nil {
			return fmt.Errorf("scheduler: mark occurrence failed: update schedule: %w", err)
		}

		return nil
	})
}

func (s *Store) SyncOccurrenceWithRun(ctx context.Context, occurrenceID string, run model.Run, now time.Time) error {
	return s.db.Tx(ctx, func(tx *sql.Tx) error {
		occurrence, err := loadOccurrenceRow(ctx, tx, occurrenceID)
		if err != nil {
			return fmt.Errorf("scheduler: sync occurrence with run: load occurrence: %w", err)
		}
		schedule, err := loadScheduleRow(ctx, tx, occurrence.ScheduleID)
		if err != nil {
			return fmt.Errorf("scheduler: sync occurrence with run: load schedule: %w", err)
		}

		targetStatus, err := occurrenceStatusFromRun(run.Status)
		if err != nil {
			return err
		}
		if targetStatus == occurrence.Status {
			return nil
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE schedule_occurrences
			    SET status = ?, updated_at = ?, finished_at = CASE
			            WHEN ? IN (?, ?, ?) THEN ?
			            ELSE finished_at
			        END
			  WHERE id = ?`,
			targetStatus,
			now.UTC(),
			targetStatus,
			OccurrenceCompleted,
			OccurrenceFailed,
			OccurrenceInterrupted,
			nullableTime(now),
			occurrenceID,
		); err != nil {
			return fmt.Errorf("scheduler: sync occurrence with run: update occurrence: %w", err)
		}

		switch targetStatus {
		case OccurrenceNeedsApproval:
			if _, err := tx.ExecContext(ctx,
				`UPDATE schedules
				    SET last_run_at = ?, last_status = ?, last_error = '', updated_at = ?
				  WHERE id = ?`,
				occurrence.SlotAt.UTC(),
				OccurrenceNeedsApproval,
				now.UTC(),
				schedule.ID,
			); err != nil {
				return fmt.Errorf("scheduler: sync occurrence with run: update approval summary: %w", err)
			}
		case OccurrenceCompleted, OccurrenceInterrupted:
			if _, err := tx.ExecContext(ctx,
				`UPDATE schedules
				    SET last_run_at = ?, last_status = ?, last_error = '', consecutive_failures = 0, updated_at = ?
				  WHERE id = ?`,
				occurrence.SlotAt.UTC(),
				targetStatus,
				now.UTC(),
				schedule.ID,
			); err != nil {
				return fmt.Errorf("scheduler: sync occurrence with run: update terminal summary: %w", err)
			}
		case OccurrenceFailed:
			consecutiveFailures := 1
			if !schedule.LastRunAt.IsZero() && schedule.LastStatus == OccurrenceFailed {
				consecutiveFailures = schedule.ConsecutiveFailures + 1
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE schedules
				    SET last_run_at = ?, last_status = ?, last_error = '', consecutive_failures = ?, updated_at = ?
				  WHERE id = ?`,
				occurrence.SlotAt.UTC(),
				OccurrenceFailed,
				consecutiveFailures,
				now.UTC(),
				schedule.ID,
			); err != nil {
				return fmt.Errorf("scheduler: sync occurrence with run: update failed summary: %w", err)
			}
		}

		return nil
	})
}

func (s *Store) RepairMissingNextRunAt(ctx context.Context, now time.Time) error {
	rows, err := s.db.RawDB().QueryContext(ctx,
		`SELECT id, name, objective, workspace_root, schedule_kind, schedule_at, schedule_every_seconds,
		        schedule_cron_expr, timezone, enabled, next_run_at, last_run_at, last_status, last_error,
		        consecutive_failures, schedule_error_count, created_at, updated_at
		   FROM schedules
		  WHERE enabled = 1 AND next_run_at IS NULL`,
	)
	if err != nil {
		return fmt.Errorf("scheduler: repair missing next run: %w", err)
	}
	defer rows.Close()

	var schedules []Schedule
	for rows.Next() {
		schedule, err := scanSchedule(rows)
		if err != nil {
			return fmt.Errorf("scheduler: repair missing next run: %w", err)
		}
		schedules = append(schedules, schedule)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("scheduler: repair missing next run: %w", err)
	}

	for _, schedule := range schedules {
		nextRunAt, err := ComputeNextRun(schedule.Spec, now.UTC())
		if err != nil {
			return fmt.Errorf("scheduler: repair missing next run for %s: %w", schedule.ID, err)
		}
		if _, err := s.db.RawDB().ExecContext(ctx,
			"UPDATE schedules SET next_run_at = ?, updated_at = ? WHERE id = ?",
			nullableTime(nextRunAt),
			now.UTC(),
			schedule.ID,
		); err != nil {
			return fmt.Errorf("scheduler: repair missing next run for %s: %w", schedule.ID, err)
		}
	}

	return nil
}

func loadScheduleRow(ctx context.Context, q queryRower, id string) (Schedule, error) {
	return scanSchedule(q.QueryRowContext(ctx,
		`SELECT id, name, objective, workspace_root, schedule_kind, schedule_at, schedule_every_seconds,
		        schedule_cron_expr, timezone, enabled, next_run_at, last_run_at, last_status, last_error,
		        consecutive_failures, schedule_error_count, created_at, updated_at
		   FROM schedules
		  WHERE id = ?`,
		id,
	))
}

func scanSchedule(scanner rowScanner) (Schedule, error) {
	var schedule Schedule
	var kind string
	var enabled int
	var nextRunAt sql.NullTime
	var lastRunAt sql.NullTime

	err := scanner.Scan(
		&schedule.ID,
		&schedule.Name,
		&schedule.Objective,
		&schedule.WorkspaceRoot,
		&kind,
		&schedule.Spec.At,
		&schedule.Spec.EverySeconds,
		&schedule.Spec.CronExpr,
		&schedule.Spec.Timezone,
		&enabled,
		&nextRunAt,
		&lastRunAt,
		&schedule.LastStatus,
		&schedule.LastError,
		&schedule.ConsecutiveFailures,
		&schedule.ScheduleErrorCount,
		&schedule.CreatedAt,
		&schedule.UpdatedAt,
	)
	if err != nil {
		return Schedule{}, err
	}

	schedule.Spec.Kind = ScheduleKind(kind)
	schedule.Enabled = enabled == 1
	if nextRunAt.Valid {
		schedule.NextRunAt = nextRunAt.Time.UTC()
	}
	if lastRunAt.Valid {
		schedule.LastRunAt = lastRunAt.Time.UTC()
	}
	schedule.CreatedAt = schedule.CreatedAt.UTC()
	schedule.UpdatedAt = schedule.UpdatedAt.UTC()

	return schedule, nil
}

func scanOccurrence(scanner rowScanner) (Occurrence, error) {
	var occurrence Occurrence
	var startedAt sql.NullTime
	var finishedAt sql.NullTime

	err := scanner.Scan(
		&occurrence.ID,
		&occurrence.ScheduleID,
		&occurrence.SlotAt,
		&occurrence.ThreadID,
		&occurrence.Status,
		&occurrence.SkipReason,
		&occurrence.RunID,
		&occurrence.ConversationID,
		&occurrence.Error,
		&startedAt,
		&finishedAt,
		&occurrence.CreatedAt,
		&occurrence.UpdatedAt,
	)
	if err != nil {
		return Occurrence{}, err
	}

	occurrence.SlotAt = occurrence.SlotAt.UTC()
	if startedAt.Valid {
		occurrence.StartedAt = startedAt.Time.UTC()
	}
	if finishedAt.Valid {
		occurrence.FinishedAt = finishedAt.Time.UTC()
	}
	occurrence.CreatedAt = occurrence.CreatedAt.UTC()
	occurrence.UpdatedAt = occurrence.UpdatedAt.UTC()
	return occurrence, nil
}

func loadOccurrenceRow(ctx context.Context, q queryRower, id string) (Occurrence, error) {
	return scanOccurrence(q.QueryRowContext(ctx,
		`SELECT id, schedule_id, slot_at, thread_id, status, skip_reason, run_id, conversation_id, error,
		        started_at, finished_at, created_at, updated_at
		   FROM schedule_occurrences
		  WHERE id = ?`,
		id,
	))
}

func hasPreviousActiveOccurrence(ctx context.Context, tx *sql.Tx, scheduleID string, slotAt time.Time) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(1)
		   FROM schedule_occurrences
		  WHERE schedule_id = ?
		    AND slot_at < ?
		    AND status IN (?, ?, ?)`,
		scheduleID,
		slotAt.UTC(),
		OccurrenceDispatching,
		OccurrenceActive,
		OccurrenceNeedsApproval,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func updateScheduleNextRun(ctx context.Context, tx *sql.Tx, scheduleID string, nextRunAt, now time.Time) error {
	_, err := tx.ExecContext(ctx,
		"UPDATE schedules SET next_run_at = ?, updated_at = ? WHERE id = ?",
		nullableTime(nextRunAt),
		now.UTC(),
		scheduleID,
	)
	return err
}

func occurrenceStatusFromRun(status model.RunStatus) (OccurrenceStatus, error) {
	switch status {
	case model.RunStatusActive, model.RunStatusPending:
		return OccurrenceActive, nil
	case model.RunStatusNeedsApproval:
		return OccurrenceNeedsApproval, nil
	case model.RunStatusCompleted:
		return OccurrenceCompleted, nil
	case model.RunStatusInterrupted:
		return OccurrenceInterrupted, nil
	case model.RunStatusFailed:
		return OccurrenceFailed, nil
	default:
		return "", fmt.Errorf("scheduler: unsupported run status %q", status)
	}
}

func nullableTime(ts time.Time) any {
	if ts.IsZero() {
		return nil
	}
	return ts.UTC()
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func generateSchedulerID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("scheduler: generate id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
