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
	return d.runtime.ReceiveInboundMessageAsync(ctx, runtime.InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: cmd.ConversationKey.ConnectorID,
			AccountID:   cmd.ConversationKey.AccountID,
			ExternalID:  cmd.ConversationKey.ExternalID,
			ThreadID:    cmd.ConversationKey.ThreadID,
		},
		FrontAgentID:    cmd.FrontAgentID,
		Body:            cmd.Body,
		SourceMessageID: cmd.SourceMessageID,
		WorkspaceRoot:   cmd.WorkspaceRoot,
	})
}

func (a *App) CreateSchedule(ctx context.Context, in scheduler.CreateScheduleInput) (scheduler.Schedule, error) {
	if a.scheduler == nil {
		return scheduler.Schedule{}, fmt.Errorf("scheduler is not configured")
	}

	workspaceRoot, err := a.resolveScheduleWorkspace(ctx, in.WorkspaceRoot)
	if err != nil {
		return scheduler.Schedule{}, err
	}
	in.WorkspaceRoot = workspaceRoot
	return a.scheduler.CreateSchedule(ctx, in)
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

func (a *App) RunScheduleNow(ctx context.Context, scheduleID string) (*scheduler.ClaimedOccurrence, error) {
	if a.scheduler == nil {
		return nil, fmt.Errorf("scheduler is not configured")
	}
	return a.scheduler.RunNow(ctx, scheduleID)
}

func (a *App) resolveScheduleWorkspace(ctx context.Context, workspaceRoot string) (string, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot != "" {
		project, err := runtime.RegisterProject(ctx, a.db, workspaceRoot, "", "scheduler")
		if err != nil {
			return "", err
		}
		return project.WorkspaceRoot, nil
	}

	project, err := runtime.ActiveProject(ctx, a.db)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(project.WorkspaceRoot) == "" {
		return "", fmt.Errorf("schedule workspace is required")
	}

	project, err = runtime.RegisterProject(ctx, a.db, project.WorkspaceRoot, project.Name, "scheduler")
	if err != nil {
		return "", err
	}
	return project.WorkspaceRoot, nil
}
