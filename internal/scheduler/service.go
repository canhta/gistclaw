// internal/scheduler/service.go
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/adhocore/gronx"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/agent"
	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/store"
)

// Job represents a scheduled task.
type Job struct {
	ID             string
	Kind           string // "at" | "every" | "cron" | "in"
	Target         agent.Kind
	Prompt         string
	Schedule       string
	NextRunAt      time.Time
	LastRunAt      *time.Time
	Enabled        bool
	DeleteAfterRun bool
	CreatedAt      time.Time
}

// JobTarget is implemented by app.App (wired in Plan 8).
type JobTarget interface {
	RunAgentTask(ctx context.Context, kind agent.Kind, prompt string) error
	SendChat(ctx context.Context, chatID int64, text string) error
}

// Service runs the scheduler: fires jobs on schedule, manages CRUD.
type Service struct {
	store          *store.Store
	target         JobTarget
	tuning         config.Tuning
	operatorChatID int64
}

// NewService creates a new scheduler Service.
func NewService(s *store.Store, target JobTarget, tuning config.Tuning, operatorChatID int64) *Service {
	return &Service{store: s, target: target, tuning: tuning, operatorChatID: operatorChatID}
}

// Run starts the scheduler loop. Blocks until ctx is cancelled.
func (s *Service) Run(ctx context.Context) error {
	if err := s.handleMissedJobs(ctx); err != nil {
		log.Warn().Err(err).Msg("scheduler: handleMissedJobs error (non-fatal)")
	}

	tick := s.tuning.SchedulerTick
	if tick == 0 {
		tick = time.Second
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			if err := s.tick(ctx, now); err != nil {
				log.Error().Err(err).Msg("scheduler: tick error")
			}
		}
	}
}

func (s *Service) tick(ctx context.Context, now time.Time) error {
	jobs, err := s.store.ListEnabledJobsDueBefore(now)
	if err != nil {
		return fmt.Errorf("scheduler: ListEnabledJobsDueBefore: %w", err)
	}
	for _, row := range jobs {
		s.fireJob(ctx, row, now)
	}
	return nil
}

func (s *Service) fireJob(ctx context.Context, row store.JobRow, now time.Time) {
	j := rowToJob(row)

	var fireErr error
	if j.Target == agent.KindChat {
		fireErr = s.target.SendChat(ctx, s.operatorChatID, j.Prompt)
	} else {
		fireErr = s.target.RunAgentTask(ctx, j.Target, j.Prompt)
	}

	if fireErr != nil {
		log.Warn().Str("job_id", j.ID).Err(fireErr).Msg("scheduler: job fire error; skipped")
		_ = s.target.SendChat(ctx, s.operatorChatID, "Scheduled job skipped: agent busy.")
		return
	}

	if j.DeleteAfterRun {
		_ = s.store.UpdateJobField(j.ID, "enabled", 0)
		_ = s.store.DeleteJob(j.ID)
		return
	}

	nextRun, err := s.advanceNextRun(j, now)
	if err != nil {
		log.Error().Str("job_id", j.ID).Err(err).Msg("scheduler: advanceNextRun failed")
		return
	}
	_ = s.store.UpdateJobAfterRun(j.ID, now, nextRun)
}

func (s *Service) advanceNextRun(j Job, now time.Time) (time.Time, error) {
	switch j.Kind {
	case "every":
		secs, err := strconv.ParseInt(j.Schedule, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid 'every' schedule %q: %w", j.Schedule, err)
		}
		return now.Add(time.Duration(secs) * time.Second), nil
	case "cron":
		next, err := gronx.NextTick(j.Schedule, false)
		if err != nil {
			return time.Time{}, fmt.Errorf("gronx NextTick(%q): %w", j.Schedule, err)
		}
		return next, nil
	default:
		return now, nil
	}
}

func (s *Service) handleMissedJobs(ctx context.Context) error {
	now := time.Now().UTC()
	jobs, err := s.store.ListEnabledJobsDueBefore(now)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		return nil
	}

	limit := s.tuning.MissedJobsFireLimit
	if limit == 0 {
		limit = 5
	}

	fired := 0
	for _, row := range jobs {
		j := rowToJob(row)
		if fired < limit {
			s.fireJob(ctx, row, now)
			fired++
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(500 * time.Millisecond):
			}
		} else {
			next, err := s.advanceNextRun(j, now)
			if err != nil {
				log.Warn().Str("job_id", j.ID).Err(err).Msg("scheduler: missed job advance failed; disabling")
				_ = s.store.UpdateJobField(j.ID, "enabled", 0)
				continue
			}
			_ = s.store.UpdateJobAfterRun(j.ID, now, next)
			log.Warn().Str("job_id", j.ID).Time("next_run_at", next).
				Msg("scheduler: missed job not fired (over limit); next_run_at advanced")
		}
	}
	return nil
}

// CreateJob validates and inserts a new job. Assigns a new UUID v4 ID.
func (s *Service) CreateJob(j Job) error {
	if err := validateJob(j); err != nil {
		return err
	}
	j.ID = uuid.New().String()
	j.CreatedAt = time.Now().UTC()
	if j.Kind == "at" || j.Kind == "in" {
		j.DeleteAfterRun = true
	}

	nextRun, err := s.computeInitialNextRun(j)
	if err != nil {
		return err
	}
	j.NextRunAt = nextRun
	j.Enabled = true

	return s.store.InsertJob(jobToRow(j))
}

// ListJobs returns all jobs in the store.
func (s *Service) ListJobs() ([]Job, error) {
	rows, err := s.store.ListAllJobs()
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, len(rows))
	for i, r := range rows {
		jobs[i] = rowToJob(r)
	}
	return jobs, nil
}

// UpdateJob updates specific fields of an existing job by ID.
func (s *Service) UpdateJob(id string, fields map[string]any) error {
	for k, v := range fields {
		switch k {
		case "enabled":
			val := 0
			switch b := v.(type) {
			case bool:
				if b {
					val = 1
				}
			case float64:
				if b != 0 {
					val = 1
				}
			}
			if err := s.store.UpdateJobField(id, "enabled", val); err != nil {
				return fmt.Errorf("UpdateJob: %w", err)
			}
		default:
			return fmt.Errorf("UpdateJob: field %q not supported", k)
		}
	}
	return nil
}

// DeleteJob deletes a job by ID.
func (s *Service) DeleteJob(id string) error {
	return s.store.DeleteJob(id)
}

func (s *Service) computeInitialNextRun(j Job) (time.Time, error) {
	now := time.Now().UTC()
	switch j.Kind {
	case "at":
		t, err := time.Parse(time.RFC3339, j.Schedule)
		if err != nil {
			return time.Time{}, fmt.Errorf("'at' schedule must be RFC3339: %w", err)
		}
		return t.UTC(), nil
	case "every":
		secs, err := strconv.ParseInt(j.Schedule, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("'every' schedule must be seconds string: %w", err)
		}
		return now.Add(time.Duration(secs) * time.Second), nil
	case "cron":
		next, err := gronx.NextTick(j.Schedule, false)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid cron expression %q: %w", j.Schedule, err)
		}
		return next, nil
	case "in":
		secs, err := strconv.ParseInt(j.Schedule, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("'in' schedule must be integer seconds: %w", err)
		}
		if secs < 0 {
			return time.Time{}, fmt.Errorf("'in' schedule must be non-negative, got %d", secs)
		}
		return now.Add(time.Duration(secs) * time.Second), nil
	default:
		return time.Time{}, fmt.Errorf("unknown job kind %q", j.Kind)
	}
}

func validateJob(j Job) error {
	switch j.Kind {
	case "at", "every", "cron", "in":
	default:
		return fmt.Errorf("invalid job kind %q; must be 'at', 'every', 'cron', or 'in'", j.Kind)
	}
	switch j.Target {
	case agent.KindOpenCode, agent.KindClaudeCode, agent.KindChat, agent.KindGateway:
		// valid
	default:
		return fmt.Errorf("invalid target %v; must be opencode, claudecode, chat, or gateway", j.Target)
	}
	if j.Prompt == "" {
		return fmt.Errorf("job prompt must not be empty")
	}
	if j.Schedule == "" {
		return fmt.Errorf("job schedule must not be empty")
	}
	if j.Kind == "cron" {
		if !gronx.IsValid(j.Schedule) {
			return fmt.Errorf("invalid cron expression: %q", j.Schedule)
		}
	}
	return nil
}

// Tools returns the four scheduler control tools for the gateway's System tool category.
func (s *Service) Tools() []providers.Tool {
	return []providers.Tool{
		{
			Name:        "schedule_job",
			Description: "Create a scheduled job. kind: 'at' (one-time, RFC3339 schedule), 'every' (recurring, schedule in seconds), 'cron' (cron expression), or 'in' (one-shot delay, schedule = seconds from now e.g. '300'). target: 'opencode', 'claudecode', 'chat', or 'gateway' (runs the full LLM+tools loop).",
			InputSchema: scheduleJobSchema(),
		},
		{
			Name:        "list_jobs",
			Description: "List all scheduled jobs with their IDs, kinds, targets, schedules, and enabled status.",
			InputSchema: emptySchema(),
		},
		{
			Name:        "update_job",
			Description: "Update a scheduled job. Currently supports toggling enabled status.",
			InputSchema: updateJobSchema(),
		},
		{
			Name:        "delete_job",
			Description: "Delete a scheduled job by ID.",
			InputSchema: deleteJobSchema(),
		},
	}
}

func scheduleJobSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kind": map[string]any{
				"type":        "string",
				"enum":        []string{"at", "every", "cron", "in"},
				"description": "'at' = one-time (RFC3339 schedule), 'every' = recurring (seconds), 'cron' = cron expression, 'in' = one-shot delay (schedule = seconds from now, e.g. '300' = 5 minutes)",
			},
			"target": map[string]any{
				"type":        "string",
				"enum":        []string{"opencode", "claudecode", "chat", "gateway"},
				"description": "Which agent or channel to target: 'gateway' runs the full LLM+tools chat loop",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The prompt/task to send when the job fires",
			},
			"schedule": map[string]any{
				"type":        "string",
				"description": "RFC3339 datetime (at), seconds string e.g. '3600' (every), cron expression e.g. '0 9 * * 1-5' (cron), or seconds from now e.g. '300' (in)",
			},
		},
		"required": []string{"kind", "target", "prompt", "schedule"},
	}
}

func updateJobSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "Job ID (from list_jobs)",
			},
			"enabled": map[string]any{
				"type":        "boolean",
				"description": "Enable or disable the job",
			},
		},
		"required": []string{"id"},
	}
}

func deleteJobSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "Job ID to delete",
			},
		},
		"required": []string{"id"},
	}
}

func emptySchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

// --- conversion helpers ---

func rowToJob(r store.JobRow) Job {
	kind, err := agent.KindFromString(r.Target)
	if err != nil {
		log.Warn().Str("job_id", r.ID).Str("target", r.Target).Err(err).Msg("scheduler: unknown target in store")
	}
	return Job{
		ID:             r.ID,
		Kind:           r.Kind,
		Target:         kind,
		Prompt:         r.Prompt,
		Schedule:       r.Schedule,
		NextRunAt:      r.NextRunAt,
		LastRunAt:      r.LastRunAt,
		Enabled:        r.Enabled,
		DeleteAfterRun: r.DeleteAfterRun,
		CreatedAt:      r.CreatedAt,
	}
}

func jobToRow(j Job) store.JobRow {
	return store.JobRow{
		ID:             j.ID,
		Kind:           j.Kind,
		Target:         j.Target.String(),
		Prompt:         j.Prompt,
		Schedule:       j.Schedule,
		NextRunAt:      j.NextRunAt,
		LastRunAt:      j.LastRunAt,
		Enabled:        j.Enabled,
		DeleteAfterRun: j.DeleteAfterRun,
		CreatedAt:      j.CreatedAt,
	}
}

// JobsToJSON serialises a slice of Jobs for tool results.
func JobsToJSON(jobs []Job) string {
	type item struct {
		ID        string `json:"id"`
		Kind      string `json:"kind"`
		Target    string `json:"target"`
		Prompt    string `json:"prompt"`
		Schedule  string `json:"schedule"`
		NextRunAt string `json:"next_run_at"`
		Enabled   bool   `json:"enabled"`
	}
	items := make([]item, len(jobs))
	for i, j := range jobs {
		items[i] = item{
			ID:        j.ID,
			Kind:      j.Kind,
			Target:    j.Target.String(),
			Prompt:    j.Prompt,
			Schedule:  j.Schedule,
			NextRunAt: j.NextRunAt.Format(time.RFC3339),
			Enabled:   j.Enabled,
		}
	}
	b, _ := json.MarshalIndent(items, "", "  ")
	return string(b)
}
