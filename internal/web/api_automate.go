package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/scheduler"
)

const automateDispatchGracePeriod = 30 * time.Second

type automateResponse struct {
	Summary           automateSummaryResponse      `json:"summary"`
	Health            automateHealthResponse       `json:"health"`
	Schedules         []automateScheduleResponse   `json:"schedules"`
	OpenOccurrences   []automateOccurrenceResponse `json:"open_occurrences"`
	RecentOccurrences []automateOccurrenceResponse `json:"recent_occurrences"`
}

type automateSummaryResponse struct {
	TotalSchedules    int    `json:"total_schedules"`
	EnabledSchedules  int    `json:"enabled_schedules"`
	DueSchedules      int    `json:"due_schedules"`
	ActiveOccurrences int    `json:"active_occurrences"`
	NextWakeAtLabel   string `json:"next_wake_at_label"`
}

type automateHealthResponse struct {
	InvalidSchedules int `json:"invalid_schedules"`
	StuckDispatching int `json:"stuck_dispatching"`
	MissingNextRun   int `json:"missing_next_run"`
}

type automateScheduleResponse struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Objective           string `json:"objective"`
	Kind                string `json:"kind"`
	KindLabel           string `json:"kind_label"`
	CadenceLabel        string `json:"cadence_label"`
	Enabled             bool   `json:"enabled"`
	EnabledLabel        string `json:"enabled_label"`
	StatusLabel         string `json:"status_label"`
	StatusClass         string `json:"status_class"`
	NextRunAtLabel      string `json:"next_run_at_label"`
	LastRunAtLabel      string `json:"last_run_at_label"`
	LastError           string `json:"last_error"`
	ProjectID           string `json:"project_id"`
	CWD                 string `json:"cwd"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	ScheduleErrorCount  int    `json:"schedule_error_count"`
}

type automateOccurrenceResponse struct {
	ID             string `json:"id"`
	ScheduleID     string `json:"schedule_id"`
	ScheduleName   string `json:"schedule_name"`
	Status         string `json:"status"`
	StatusLabel    string `json:"status_label"`
	StatusClass    string `json:"status_class"`
	SlotAtLabel    string `json:"slot_at_label"`
	UpdatedAtLabel string `json:"updated_at_label"`
	RunID          string `json:"run_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	Error          string `json:"error,omitempty"`
	SkipReason     string `json:"skip_reason,omitempty"`
}

type automateCreateRequest struct {
	Name       string `json:"name"`
	Objective  string `json:"objective"`
	Kind       string `json:"kind"`
	AnchorAt   string `json:"anchor_at"`
	EveryHours int    `json:"every_hours"`
	CronExpr   string `json:"cron_expr"`
	Timezone   string `json:"timezone"`
}

type automateScheduleMutationResponse struct {
	Schedule automateScheduleResponse `json:"schedule"`
}

type automateRunResponse struct {
	Schedule   automateScheduleResponse   `json:"schedule"`
	Occurrence automateOccurrenceResponse `json:"occurrence"`
}

type automateOccurrenceRow struct {
	ID             string
	ScheduleID     string
	ScheduleName   string
	Status         string
	RunID          string
	ConversationID string
	Error          string
	SkipReason     string
	SlotAt         time.Time
	UpdatedAt      time.Time
}

func (s *Server) handleAutomateIndex(w http.ResponseWriter, r *http.Request) {
	if s.schedules == nil {
		http.Error(w, "scheduler is not configured", http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	schedules, err := s.schedules.ListSchedules(ctx)
	if err != nil {
		http.Error(w, "failed to load schedules", http.StatusInternalServerError)
		return
	}
	status, err := s.schedules.ScheduleStatus(ctx)
	if err != nil {
		http.Error(w, "failed to load scheduler status", http.StatusInternalServerError)
		return
	}
	health, err := scheduler.NewStore(s.db).Health(ctx, time.Now().UTC(), automateDispatchGracePeriod)
	if err != nil {
		http.Error(w, "failed to load scheduler health", http.StatusInternalServerError)
		return
	}
	openOccurrences, err := s.loadAutomateOccurrences(ctx, []scheduler.OccurrenceStatus{
		scheduler.OccurrenceDispatching,
		scheduler.OccurrenceActive,
		scheduler.OccurrenceNeedsApproval,
	}, 12)
	if err != nil {
		http.Error(w, "failed to load open schedule work", http.StatusInternalServerError)
		return
	}
	recentOccurrences, err := s.loadAutomateOccurrences(ctx, []scheduler.OccurrenceStatus{
		scheduler.OccurrenceFailed,
		scheduler.OccurrenceInterrupted,
		scheduler.OccurrenceCompleted,
		scheduler.OccurrenceSkipped,
	}, 12)
	if err != nil {
		http.Error(w, "failed to load recent schedule work", http.StatusInternalServerError)
		return
	}

	resp := automateResponse{
		Summary: automateSummaryResponse{
			TotalSchedules:    status.TotalSchedules,
			EnabledSchedules:  status.EnabledSchedules,
			DueSchedules:      status.DueSchedules,
			ActiveOccurrences: status.ActiveOccurrences,
			NextWakeAtLabel:   automateNextWakeLabel(status.NextWakeAt),
		},
		Health: automateHealthResponse{
			InvalidSchedules: health.InvalidSchedules,
			StuckDispatching: health.StuckDispatching,
			MissingNextRun:   health.MissingNextRun,
		},
		Schedules:         make([]automateScheduleResponse, 0, len(schedules)),
		OpenOccurrences:   make([]automateOccurrenceResponse, 0, len(openOccurrences)),
		RecentOccurrences: make([]automateOccurrenceResponse, 0, len(recentOccurrences)),
	}
	for _, item := range schedules {
		resp.Schedules = append(resp.Schedules, buildAutomateScheduleResponse(item))
	}
	for _, item := range openOccurrences {
		resp.OpenOccurrences = append(resp.OpenOccurrences, buildAutomateOccurrenceResponse(item))
	}
	for _, item := range recentOccurrences {
		resp.RecentOccurrences = append(resp.RecentOccurrences, buildAutomateOccurrenceResponse(item))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAutomateCreate(w http.ResponseWriter, r *http.Request) {
	if s.schedules == nil {
		http.Error(w, "scheduler is not configured", http.StatusServiceUnavailable)
		return
	}

	var req automateCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	input, err := buildAutomateCreateInput(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.fillAutomateProjectDefaults(r.Context(), &input); err != nil {
		http.Error(w, "failed to resolve active project: "+err.Error(), http.StatusBadRequest)
		return
	}

	schedule, err := s.schedules.CreateSchedule(r.Context(), input)
	if err != nil {
		http.Error(w, "failed to create schedule: "+err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, automateScheduleMutationResponse{
		Schedule: buildAutomateScheduleResponse(schedule),
	})
}

func (s *Server) fillAutomateProjectDefaults(ctx context.Context, input *scheduler.CreateScheduleInput) error {
	if input == nil || (strings.TrimSpace(input.ProjectID) != "" && strings.TrimSpace(input.CWD) != "") {
		return nil
	}

	project, err := runtime.ActiveProject(ctx, s.db)
	if err != nil {
		return err
	}
	if strings.TrimSpace(input.ProjectID) == "" {
		input.ProjectID = project.ID
	}
	if strings.TrimSpace(input.CWD) == "" {
		input.CWD = project.PrimaryPath
	}
	return nil
}

func (s *Server) handleAutomateEnable(w http.ResponseWriter, r *http.Request) {
	s.handleAutomateScheduleMutation(w, r, func(ctx context.Context, scheduleID string) (scheduler.Schedule, error) {
		return s.schedules.EnableSchedule(ctx, scheduleID)
	})
}

func (s *Server) handleAutomateDisable(w http.ResponseWriter, r *http.Request) {
	s.handleAutomateScheduleMutation(w, r, func(ctx context.Context, scheduleID string) (scheduler.Schedule, error) {
		return s.schedules.DisableSchedule(ctx, scheduleID)
	})
}

func (s *Server) handleAutomateRun(w http.ResponseWriter, r *http.Request) {
	if s.schedules == nil {
		http.Error(w, "scheduler is not configured", http.StatusServiceUnavailable)
		return
	}

	scheduleID := strings.TrimSpace(r.PathValue("id"))
	if scheduleID == "" {
		http.NotFound(w, r)
		return
	}

	claimed, err := s.schedules.RunScheduleNow(r.Context(), scheduleID)
	if err != nil {
		http.Error(w, "failed to launch schedule: "+err.Error(), http.StatusBadRequest)
		return
	}
	if claimed == nil {
		http.Error(w, "schedule already has active work", http.StatusConflict)
		return
	}

	occurrence := automateOccurrenceRowFromClaim(*claimed)
	if refreshed, err := s.loadAutomateOccurrenceByID(r.Context(), claimed.Occurrence.ID); err == nil {
		occurrence = refreshed
	}

	writeJSON(w, http.StatusOK, automateRunResponse{
		Schedule:   buildAutomateScheduleResponse(claimed.Schedule),
		Occurrence: buildAutomateOccurrenceResponse(occurrence),
	})
}

func (s *Server) handleAutomateScheduleMutation(
	w http.ResponseWriter,
	r *http.Request,
	mutate func(context.Context, string) (scheduler.Schedule, error),
) {
	if s.schedules == nil {
		http.Error(w, "scheduler is not configured", http.StatusServiceUnavailable)
		return
	}

	scheduleID := strings.TrimSpace(r.PathValue("id"))
	if scheduleID == "" {
		http.NotFound(w, r)
		return
	}

	schedule, err := mutate(r.Context(), scheduleID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		http.Error(w, "failed to update schedule: "+err.Error(), status)
		return
	}

	writeJSON(w, http.StatusOK, automateScheduleMutationResponse{
		Schedule: buildAutomateScheduleResponse(schedule),
	})
}

func buildAutomateCreateInput(req automateCreateRequest) (scheduler.CreateScheduleInput, error) {
	input := scheduler.CreateScheduleInput{
		Name:      strings.TrimSpace(req.Name),
		Objective: strings.TrimSpace(req.Objective),
		Enabled:   true,
	}

	switch scheduler.ScheduleKind(strings.TrimSpace(req.Kind)) {
	case scheduler.ScheduleKindAt:
		input.Spec = scheduler.ScheduleSpec{
			Kind: scheduler.ScheduleKindAt,
			At:   strings.TrimSpace(req.AnchorAt),
		}
	case scheduler.ScheduleKindEvery:
		if req.EveryHours <= 0 {
			return scheduler.CreateScheduleInput{}, fmt.Errorf("every_hours must be greater than zero")
		}
		input.Spec = scheduler.ScheduleSpec{
			Kind:         scheduler.ScheduleKindEvery,
			At:           strings.TrimSpace(req.AnchorAt),
			EverySeconds: int64((time.Duration(req.EveryHours) * time.Hour) / time.Second),
		}
	case scheduler.ScheduleKindCron:
		input.Spec = scheduler.ScheduleSpec{
			Kind:     scheduler.ScheduleKindCron,
			CronExpr: strings.TrimSpace(req.CronExpr),
			Timezone: strings.TrimSpace(req.Timezone),
		}
	default:
		return scheduler.CreateScheduleInput{}, fmt.Errorf("kind must be one of at, every, or cron")
	}

	return input, nil
}

func buildAutomateScheduleResponse(item scheduler.Schedule) automateScheduleResponse {
	statusLabel, statusClass := automateScheduleStatus(item)
	return automateScheduleResponse{
		ID:                  item.ID,
		Name:                item.Name,
		Objective:           item.Objective,
		Kind:                string(item.Spec.Kind),
		KindLabel:           automateKindLabel(item.Spec.Kind),
		CadenceLabel:        automateCadenceLabel(item.Spec),
		Enabled:             item.Enabled,
		EnabledLabel:        automateEnabledLabel(item.Enabled),
		StatusLabel:         statusLabel,
		StatusClass:         statusClass,
		NextRunAtLabel:      automateScheduleTimeLabel(item.NextRunAt, "Waiting for the next wake"),
		LastRunAtLabel:      automateScheduleTimeLabel(item.LastRunAt, "No executions recorded"),
		LastError:           strings.TrimSpace(item.LastError),
		ProjectID:           item.ProjectID,
		CWD:                 item.CWD,
		ConsecutiveFailures: item.ConsecutiveFailures,
		ScheduleErrorCount:  item.ScheduleErrorCount,
	}
}

func buildAutomateOccurrenceResponse(item automateOccurrenceRow) automateOccurrenceResponse {
	return automateOccurrenceResponse{
		ID:             item.ID,
		ScheduleID:     item.ScheduleID,
		ScheduleName:   item.ScheduleName,
		Status:         item.Status,
		StatusLabel:    humanizeWebLabel(item.Status),
		StatusClass:    automateOccurrenceStatusClass(item.Status),
		SlotAtLabel:    formatRunCompactTimestamp(item.SlotAt),
		UpdatedAtLabel: formatRunCompactTimestamp(item.UpdatedAt),
		RunID:          item.RunID,
		ConversationID: item.ConversationID,
		Error:          item.Error,
		SkipReason:     item.SkipReason,
	}
}

func automateOccurrenceRowFromClaim(claimed scheduler.ClaimedOccurrence) automateOccurrenceRow {
	return automateOccurrenceRow{
		ID:             claimed.Occurrence.ID,
		ScheduleID:     claimed.Schedule.ID,
		ScheduleName:   claimed.Schedule.Name,
		Status:         string(claimed.Occurrence.Status),
		RunID:          claimed.Occurrence.RunID,
		ConversationID: claimed.Occurrence.ConversationID,
		Error:          claimed.Occurrence.Error,
		SkipReason:     claimed.Occurrence.SkipReason,
		SlotAt:         claimed.Occurrence.SlotAt,
		UpdatedAt:      claimed.Occurrence.UpdatedAt,
	}
}

func (s *Server) loadAutomateOccurrences(
	ctx context.Context,
	statuses []scheduler.OccurrenceStatus,
	limit int,
) ([]automateOccurrenceRow, error) {
	if limit <= 0 {
		limit = 12
	}
	if len(statuses) == 0 {
		return nil, nil
	}

	args := make([]any, 0, len(statuses)+1)
	placeholders := make([]string, 0, len(statuses))
	for _, status := range statuses {
		placeholders = append(placeholders, "?")
		args = append(args, status)
	}
	args = append(args, limit)

	query := `
		SELECT o.id, o.schedule_id, s.name, o.status, COALESCE(o.run_id, ''), COALESCE(o.conversation_id, ''),
		       COALESCE(o.error, ''), COALESCE(o.skip_reason, ''), o.slot_at, o.updated_at
		FROM schedule_occurrences o
		JOIN schedules s ON s.id = o.schedule_id
		WHERE o.status IN (` + strings.Join(placeholders, ", ") + `)
		ORDER BY o.updated_at DESC, o.id DESC
		LIMIT ?`

	rows, err := s.db.RawDB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query automate occurrences: %w", err)
	}
	defer rows.Close()

	resp := make([]automateOccurrenceRow, 0, limit)
	for rows.Next() {
		var item automateOccurrenceRow
		if err := rows.Scan(
			&item.ID,
			&item.ScheduleID,
			&item.ScheduleName,
			&item.Status,
			&item.RunID,
			&item.ConversationID,
			&item.Error,
			&item.SkipReason,
			&item.SlotAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan automate occurrence: %w", err)
		}
		resp = append(resp, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate automate occurrences: %w", err)
	}
	return resp, nil
}

func (s *Server) loadAutomateOccurrenceByID(ctx context.Context, occurrenceID string) (automateOccurrenceRow, error) {
	row := s.db.RawDB().QueryRowContext(
		ctx,
		`SELECT o.id, o.schedule_id, s.name, o.status, COALESCE(o.run_id, ''), COALESCE(o.conversation_id, ''),
		        COALESCE(o.error, ''), COALESCE(o.skip_reason, ''), o.slot_at, o.updated_at
		   FROM schedule_occurrences o
		   JOIN schedules s ON s.id = o.schedule_id
		  WHERE o.id = ?`,
		occurrenceID,
	)

	var item automateOccurrenceRow
	if err := row.Scan(
		&item.ID,
		&item.ScheduleID,
		&item.ScheduleName,
		&item.Status,
		&item.RunID,
		&item.ConversationID,
		&item.Error,
		&item.SkipReason,
		&item.SlotAt,
		&item.UpdatedAt,
	); err != nil {
		return automateOccurrenceRow{}, err
	}
	return item, nil
}

func automateNextWakeLabel(ts time.Time) string {
	if ts.IsZero() {
		return "No wake scheduled"
	}
	return formatRunCompactTimestamp(ts)
}

func automateScheduleTimeLabel(ts time.Time, fallback string) string {
	if ts.IsZero() {
		return fallback
	}
	return formatRunCompactTimestamp(ts)
}

func automateKindLabel(kind scheduler.ScheduleKind) string {
	switch kind {
	case scheduler.ScheduleKindAt:
		return "Once"
	case scheduler.ScheduleKindEvery:
		return "Every"
	case scheduler.ScheduleKindCron:
		return "Cron"
	default:
		return humanizeWebLabel(string(kind))
	}
}

func automateCadenceLabel(spec scheduler.ScheduleSpec) string {
	switch spec.Kind {
	case scheduler.ScheduleKindAt:
		return "Runs once at " + formatScheduleAnchor(spec.At)
	case scheduler.ScheduleKindEvery:
		return fmt.Sprintf("Every %s from %s", formatScheduleInterval(spec.EverySeconds), formatScheduleAnchor(spec.At))
	case scheduler.ScheduleKindCron:
		if strings.TrimSpace(spec.Timezone) == "" {
			return "Cron: " + strings.TrimSpace(spec.CronExpr)
		}
		return fmt.Sprintf("Cron: %s (%s)", strings.TrimSpace(spec.CronExpr), strings.TrimSpace(spec.Timezone))
	default:
		return "Schedule details unavailable"
	}
}

func formatScheduleAnchor(raw string) string {
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	return ts.UTC().Format("2006-01-02 15:04 UTC")
}

func formatScheduleInterval(seconds int64) string {
	switch {
	case seconds <= 0:
		return "custom cadence"
	case seconds%(24*60*60) == 0:
		return fmt.Sprintf("%dd", seconds/(24*60*60))
	case seconds%(60*60) == 0:
		return fmt.Sprintf("%dh", seconds/(60*60))
	case seconds%60 == 0:
		return fmt.Sprintf("%dm", seconds/60)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}

func automateEnabledLabel(enabled bool) string {
	if enabled {
		return "Enabled"
	}
	return "Paused"
}

func automateScheduleStatus(item scheduler.Schedule) (string, string) {
	switch {
	case !item.Enabled:
		return "Paused", "is-muted"
	case item.ConsecutiveFailures > 0 || item.LastStatus == scheduler.OccurrenceFailed:
		return "Needs repair", "is-error"
	case item.LastStatus == scheduler.OccurrenceNeedsApproval:
		return "Waiting for approval", "is-approval"
	case item.LastStatus == scheduler.OccurrenceActive || item.LastStatus == scheduler.OccurrenceDispatching:
		return "In flight", "is-active"
	default:
		return "Healthy", "is-active"
	}
}

func automateOccurrenceStatusClass(status string) string {
	switch scheduler.OccurrenceStatus(status) {
	case scheduler.OccurrenceFailed, scheduler.OccurrenceInterrupted:
		return "is-error"
	case scheduler.OccurrenceNeedsApproval:
		return "is-approval"
	case scheduler.OccurrenceActive, scheduler.OccurrenceCompleted:
		return "is-active"
	default:
		return "is-muted"
	}
}
