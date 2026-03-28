package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/scheduler"
)

func TestAutomateAPIListsSchedulesAndExecutionLanes(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	dispatcher := &automateTestDispatcher{run: model.Run{
		ID:             "run-automate-now",
		ConversationID: "conv-automate-now",
		Status:         model.RunStatusActive,
	}}
	service := scheduler.NewService(scheduler.NewStore(h.db), dispatcher)
	h.rawServer.schedules = automateScheduleServiceAdapter{service: service}

	ctx := context.Background()
	everySchedule, err := service.CreateSchedule(ctx, scheduler.CreateScheduleInput{
		ID:        "sched-review",
		Name:      "Repo review",
		Objective: "Audit the repository every two hours.",
		ProjectID: h.activeProjectID,
		CWD:       h.workspaceRoot,
		Enabled:   true,
		Spec: scheduler.ScheduleSpec{
			Kind:         scheduler.ScheduleKindEvery,
			At:           time.Now().UTC().Add(-4 * time.Hour).Format(time.RFC3339),
			EverySeconds: int64((2 * time.Hour) / time.Second),
		},
	})
	if err != nil {
		t.Fatalf("create every schedule: %v", err)
	}
	cronSchedule, err := service.CreateSchedule(ctx, scheduler.CreateScheduleInput{
		ID:        "sched-report",
		Name:      "Morning report",
		Objective: "Summarize overnight activity.",
		ProjectID: h.activeProjectID,
		CWD:       h.workspaceRoot,
		Enabled:   true,
		Spec: scheduler.ScheduleSpec{
			Kind:     scheduler.ScheduleKindCron,
			CronExpr: "0 9 * * *",
			Timezone: "Asia/Ho_Chi_Minh",
		},
	})
	if err != nil {
		t.Fatalf("create cron schedule: %v", err)
	}
	if _, err := service.DisableSchedule(ctx, cronSchedule.ID); err != nil {
		t.Fatalf("disable cron schedule: %v", err)
	}

	h.insertScheduleOccurrence(t, "occ-open", everySchedule.ID, scheduler.OccurrenceNeedsApproval, "run-open", "conv-open", "", "", time.Now().UTC().Add(-5*time.Minute))
	h.insertScheduleOccurrence(t, "occ-done", everySchedule.ID, scheduler.OccurrenceCompleted, "run-done", "conv-done", "", "", time.Now().UTC().Add(-2*time.Hour))
	h.insertScheduleOccurrence(t, "occ-failed", cronSchedule.ID, scheduler.OccurrenceFailed, "run-failed", "conv-failed", "dispatch boom", "", time.Now().UTC().Add(-90*time.Minute))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/automate", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Summary struct {
			TotalSchedules    int    `json:"total_schedules"`
			EnabledSchedules  int    `json:"enabled_schedules"`
			DueSchedules      int    `json:"due_schedules"`
			ActiveOccurrences int    `json:"active_occurrences"`
			NextWakeAtLabel   string `json:"next_wake_at_label"`
		} `json:"summary"`
		Health struct {
			InvalidSchedules int `json:"invalid_schedules"`
			StuckDispatching int `json:"stuck_dispatching"`
			MissingNextRun   int `json:"missing_next_run"`
		} `json:"health"`
		Schedules []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			Kind           string `json:"kind"`
			CadenceLabel   string `json:"cadence_label"`
			Enabled        bool   `json:"enabled"`
			StatusLabel    string `json:"status_label"`
			LastError      string `json:"last_error"`
			NextRunAtLabel string `json:"next_run_at_label"`
		} `json:"schedules"`
		OpenOccurrences []struct {
			ID           string `json:"id"`
			ScheduleID   string `json:"schedule_id"`
			ScheduleName string `json:"schedule_name"`
			Status       string `json:"status"`
			RunID        string `json:"run_id"`
		} `json:"open_occurrences"`
		RecentOccurrences []struct {
			ID           string `json:"id"`
			ScheduleID   string `json:"schedule_id"`
			ScheduleName string `json:"schedule_name"`
			Status       string `json:"status"`
			Error        string `json:"error"`
		} `json:"recent_occurrences"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Summary.TotalSchedules != 2 || resp.Summary.EnabledSchedules != 1 {
		t.Fatalf("unexpected summary %+v", resp.Summary)
	}
	if resp.Summary.ActiveOccurrences != 1 {
		t.Fatalf("unexpected active occurrence count %+v", resp.Summary)
	}
	if resp.Summary.NextWakeAtLabel == "" {
		t.Fatalf("expected next wake label in summary %+v", resp.Summary)
	}
	if resp.Health.InvalidSchedules != 0 || resp.Health.StuckDispatching != 0 || resp.Health.MissingNextRun != 0 {
		t.Fatalf("unexpected health %+v", resp.Health)
	}
	if len(resp.Schedules) != 2 {
		t.Fatalf("expected 2 schedules, got %d", len(resp.Schedules))
	}
	if resp.Schedules[0].ID != everySchedule.ID || resp.Schedules[0].CadenceLabel == "" || resp.Schedules[0].NextRunAtLabel == "" {
		t.Fatalf("unexpected first schedule %+v", resp.Schedules[0])
	}
	if resp.Schedules[1].ID != cronSchedule.ID || resp.Schedules[1].Enabled || resp.Schedules[1].Kind != string(scheduler.ScheduleKindCron) {
		t.Fatalf("unexpected second schedule %+v", resp.Schedules[1])
	}
	if len(resp.OpenOccurrences) != 1 || resp.OpenOccurrences[0].ID != "occ-open" || resp.OpenOccurrences[0].ScheduleName != "Repo review" {
		t.Fatalf("unexpected open occurrences %+v", resp.OpenOccurrences)
	}
	if len(resp.RecentOccurrences) < 2 {
		t.Fatalf("expected recent occurrences, got %+v", resp.RecentOccurrences)
	}
	if resp.RecentOccurrences[0].ID != "occ-failed" || resp.RecentOccurrences[0].Error != "dispatch boom" {
		t.Fatalf("unexpected latest recent occurrence %+v", resp.RecentOccurrences[0])
	}
}

func TestAutomateAPIActionsFlowThroughScheduler(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	dispatcher := &automateTestDispatcher{run: model.Run{
		ID:             "run-dispatched",
		ConversationID: "conv-dispatched",
		Status:         model.RunStatusActive,
	}}
	service := scheduler.NewService(scheduler.NewStore(h.db), dispatcher)
	h.rawServer.schedules = automateScheduleServiceAdapter{service: service}

	createRR := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/automate", strings.NewReader(`{
		"name":"Nightly repo sweep",
		"objective":"Inspect open pull requests and summarize blockers.",
		"kind":"every",
		"anchor_at":"2026-03-28T20:00:00Z",
		"every_hours":6
	}`))
	createReq.Header.Set("Authorization", "Bearer "+h.adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	h.server.ServeHTTP(createRR, createReq)

	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", createRR.Code, createRR.Body.String())
	}

	var createResp struct {
		Schedule struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
			Kind    string `json:"kind"`
		} `json:"schedule"`
	}
	if err := json.Unmarshal(createRR.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.Schedule.ID == "" || createResp.Schedule.Name != "Nightly repo sweep" || !createResp.Schedule.Enabled {
		t.Fatalf("unexpected create response %+v", createResp.Schedule)
	}

	disableRR := httptest.NewRecorder()
	disableReq := httptest.NewRequest(http.MethodPost, "/api/automate/"+createResp.Schedule.ID+"/disable", nil)
	disableReq.Header.Set("Authorization", "Bearer "+h.adminToken)
	h.server.ServeHTTP(disableRR, disableReq)
	if disableRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", disableRR.Code, disableRR.Body.String())
	}

	enableRR := httptest.NewRecorder()
	enableReq := httptest.NewRequest(http.MethodPost, "/api/automate/"+createResp.Schedule.ID+"/enable", nil)
	enableReq.Header.Set("Authorization", "Bearer "+h.adminToken)
	h.server.ServeHTTP(enableRR, enableReq)
	if enableRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", enableRR.Code, enableRR.Body.String())
	}

	runRR := httptest.NewRecorder()
	runReq := httptest.NewRequest(http.MethodPost, "/api/automate/"+createResp.Schedule.ID+"/run", nil)
	runReq.Header.Set("Authorization", "Bearer "+h.adminToken)
	h.server.ServeHTTP(runRR, runReq)
	if runRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", runRR.Code, runRR.Body.String())
	}

	var runResp struct {
		Schedule struct {
			ID string `json:"id"`
		} `json:"schedule"`
		Occurrence struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			RunID  string `json:"run_id"`
		} `json:"occurrence"`
	}
	if err := json.Unmarshal(runRR.Body.Bytes(), &runResp); err != nil {
		t.Fatalf("decode run response: %v", err)
	}
	if runResp.Schedule.ID != createResp.Schedule.ID || runResp.Occurrence.RunID != "run-dispatched" || runResp.Occurrence.Status != string(scheduler.OccurrenceActive) {
		t.Fatalf("unexpected run response %+v", runResp)
	}
	if len(dispatcher.commands) != 1 || dispatcher.commands[0].Body != "Inspect open pull requests and summarize blockers." {
		t.Fatalf("unexpected dispatcher commands %+v", dispatcher.commands)
	}
}

func (h *serverHarness) insertScheduleOccurrence(
	t *testing.T,
	occurrenceID, scheduleID string,
	status scheduler.OccurrenceStatus,
	runID, conversationID, errorText, skipReason string,
	updatedAt time.Time,
) {
	t.Helper()

	if _, err := h.db.RawDB().Exec(
		`INSERT INTO schedule_occurrences
		 (id, schedule_id, slot_at, thread_id, status, skip_reason, run_id, conversation_id, error, started_at, finished_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		occurrenceID,
		scheduleID,
		updatedAt.UTC(),
		"thread-"+occurrenceID,
		status,
		skipReason,
		runID,
		conversationID,
		errorText,
		automateNullableTime(updatedAt.UTC()),
		automateNullableTime(updatedAt.UTC()),
		updatedAt.UTC(),
		updatedAt.UTC(),
	); err != nil {
		t.Fatalf("insert schedule occurrence: %v", err)
	}
}

func automateNullableTime(ts time.Time) any {
	if ts.IsZero() {
		return nil
	}
	return ts.UTC()
}

type automateScheduleServiceAdapter struct {
	service *scheduler.Service
}

func (a automateScheduleServiceAdapter) CreateSchedule(ctx context.Context, in scheduler.CreateScheduleInput) (scheduler.Schedule, error) {
	return a.service.CreateSchedule(ctx, in)
}

func (a automateScheduleServiceAdapter) UpdateSchedule(ctx context.Context, scheduleID string, patch scheduler.UpdateScheduleInput) (scheduler.Schedule, error) {
	return a.service.UpdateSchedule(ctx, scheduleID, patch)
}

func (a automateScheduleServiceAdapter) ListSchedules(ctx context.Context) ([]scheduler.Schedule, error) {
	return a.service.ListSchedules(ctx)
}

func (a automateScheduleServiceAdapter) LoadSchedule(ctx context.Context, scheduleID string) (scheduler.Schedule, error) {
	return a.service.LoadSchedule(ctx, scheduleID)
}

func (a automateScheduleServiceAdapter) EnableSchedule(ctx context.Context, scheduleID string) (scheduler.Schedule, error) {
	return a.service.EnableSchedule(ctx, scheduleID)
}

func (a automateScheduleServiceAdapter) DisableSchedule(ctx context.Context, scheduleID string) (scheduler.Schedule, error) {
	return a.service.DisableSchedule(ctx, scheduleID)
}

func (a automateScheduleServiceAdapter) DeleteSchedule(ctx context.Context, scheduleID string) error {
	return a.service.DeleteSchedule(ctx, scheduleID)
}

func (a automateScheduleServiceAdapter) ScheduleStatus(ctx context.Context) (scheduler.StatusSummary, error) {
	return a.service.Status(ctx)
}

func (a automateScheduleServiceAdapter) RunScheduleNow(ctx context.Context, scheduleID string) (*scheduler.ClaimedOccurrence, error) {
	return a.service.RunNow(ctx, scheduleID)
}

type automateTestDispatcher struct {
	run      model.Run
	commands []scheduler.DispatchCommand
}

func (d *automateTestDispatcher) DispatchScheduled(ctx context.Context, cmd scheduler.DispatchCommand) (model.Run, error) {
	d.commands = append(d.commands, cmd)
	return d.run, nil
}
