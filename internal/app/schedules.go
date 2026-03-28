package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/scheduler"
)

type schedulerRuntimeDispatcher struct {
	runtime *runtime.Runtime
}

func (d schedulerRuntimeDispatcher) DispatchScheduled(ctx context.Context, cmd scheduler.DispatchCommand) (model.Run, error) {
	frontAgentID := strings.TrimSpace(cmd.FrontAgentID)
	if frontAgentID == "" {
		var err error
		frontAgentID, err = d.runtime.FrontAgentID(ctx)
		if err != nil {
			return model.Run{}, err
		}
	}
	return d.runtime.ReceiveInboundMessageAsync(ctx, runtime.InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: cmd.ConversationKey.ConnectorID,
			AccountID:   cmd.ConversationKey.AccountID,
			ExternalID:  cmd.ConversationKey.ExternalID,
			ThreadID:    cmd.ConversationKey.ThreadID,
		},
		FrontAgentID:    frontAgentID,
		Body:            cmd.Body,
		SourceMessageID: cmd.SourceMessageID,
		ProjectID:       cmd.ProjectID,
		CWD:             cmd.CWD,
	})
}

func (a *App) CreateSchedule(ctx context.Context, in scheduler.CreateScheduleInput) (scheduler.Schedule, error) {
	if a.scheduler == nil {
		return scheduler.Schedule{}, fmt.Errorf("scheduler is not configured")
	}

	projectID, cwd, err := a.resolveScheduleProject(ctx, in.ProjectID, in.CWD)
	if err != nil {
		return scheduler.Schedule{}, err
	}
	in.ProjectID = projectID
	in.CWD = cwd
	return a.scheduler.CreateSchedule(ctx, in)
}

func (a *App) UpdateSchedule(ctx context.Context, scheduleID string, patch scheduler.UpdateScheduleInput) (scheduler.Schedule, error) {
	if a.scheduler == nil {
		return scheduler.Schedule{}, fmt.Errorf("scheduler is not configured")
	}
	if patch.ProjectID != nil || patch.CWD != nil {
		projectID := ""
		if patch.ProjectID != nil {
			projectID = *patch.ProjectID
		}
		cwd := ""
		if patch.CWD != nil {
			cwd = *patch.CWD
		}
		resolvedProjectID, resolvedCWD, err := a.resolveScheduleProject(ctx, projectID, cwd)
		if err != nil {
			return scheduler.Schedule{}, err
		}
		if patch.ProjectID != nil || resolvedProjectID != "" {
			patch.ProjectID = &resolvedProjectID
		}
		patch.CWD = &resolvedCWD
	}
	return a.scheduler.UpdateSchedule(ctx, scheduleID, patch)
}

func (a *App) ListSchedules(ctx context.Context) ([]scheduler.Schedule, error) {
	if a.scheduler == nil {
		return nil, fmt.Errorf("scheduler is not configured")
	}
	return a.scheduler.ListSchedules(ctx)
}

func (a *App) LoadSchedule(ctx context.Context, scheduleID string) (scheduler.Schedule, error) {
	if a.scheduler == nil {
		return scheduler.Schedule{}, fmt.Errorf("scheduler is not configured")
	}
	return a.scheduler.LoadSchedule(ctx, scheduleID)
}

func (a *App) EnableSchedule(ctx context.Context, scheduleID string) (scheduler.Schedule, error) {
	if a.scheduler == nil {
		return scheduler.Schedule{}, fmt.Errorf("scheduler is not configured")
	}
	return a.scheduler.EnableSchedule(ctx, scheduleID)
}

func (a *App) DisableSchedule(ctx context.Context, scheduleID string) (scheduler.Schedule, error) {
	if a.scheduler == nil {
		return scheduler.Schedule{}, fmt.Errorf("scheduler is not configured")
	}
	return a.scheduler.DisableSchedule(ctx, scheduleID)
}

func (a *App) DeleteSchedule(ctx context.Context, scheduleID string) error {
	if a.scheduler == nil {
		return fmt.Errorf("scheduler is not configured")
	}
	return a.scheduler.DeleteSchedule(ctx, scheduleID)
}

func (a *App) ScheduleStatus(ctx context.Context) (scheduler.StatusSummary, error) {
	if a.scheduler == nil {
		return scheduler.StatusSummary{}, fmt.Errorf("scheduler is not configured")
	}
	status, err := a.scheduler.Status(ctx)
	if err != nil {
		return scheduler.StatusSummary{}, err
	}
	status.Enabled = true
	return status, nil
}

func (a *App) RunScheduleNow(ctx context.Context, scheduleID string) (*scheduler.ClaimedOccurrence, error) {
	if a.scheduler == nil {
		return nil, fmt.Errorf("scheduler is not configured")
	}
	return a.scheduler.RunNow(ctx, scheduleID)
}

func (a *App) resolveScheduleProject(ctx context.Context, projectID, cwd string) (string, string, error) {
	projectID = strings.TrimSpace(projectID)
	cwd = strings.TrimSpace(cwd)
	if projectID != "" && cwd != "" {
		return projectID, cwd, nil
	}
	if projectID != "" {
		project, err := runtime.ActiveProject(ctx, a.db)
		if err != nil {
			return "", "", err
		}
		if project.ID == projectID && strings.TrimSpace(project.PrimaryPath) != "" {
			return project.ID, project.PrimaryPath, nil
		}
		return projectID, cwd, nil
	}
	if cwd != "" {
		project, err := runtime.RegisterProjectPath(ctx, a.db, cwd, "", "scheduler")
		if err != nil {
			return "", "", err
		}
		return project.ID, project.PrimaryPath, nil
	}
	project, err := runtime.ActiveProject(ctx, a.db)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(project.PrimaryPath) == "" {
		return "", "", fmt.Errorf("schedule cwd is required")
	}
	return project.ID, project.PrimaryPath, nil
}
