package app

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/replay"
	"github.com/canhta/gistclaw/internal/runtime"
)

type Status struct {
	ActiveRuns       int
	InterruptedRuns  int
	PendingApprovals int
}

func (a *App) RunTask(ctx context.Context, objective string) (model.Run, error) {
	if strings.TrimSpace(objective) == "" {
		return model.Run{}, fmt.Errorf("objective is required")
	}

	conv, err := a.convStore.Resolve(ctx, conversations.ConversationKey{
		ConnectorID: "cli",
		AccountID:   "local",
		ExternalID:  "default",
		ThreadID:    "main",
	})
	if err != nil {
		return model.Run{}, fmt.Errorf("resolve cli conversation: %w", err)
	}

	return a.runtime.Start(ctx, runtime.StartRun{
		ConversationID: conv.ID,
		AgentID:        "cli-operator",
		Objective:      objective,
		WorkspaceRoot:  a.cfg.WorkspaceRoot,
		AccountID:      "local",
	})
}

func (a *App) InspectStatus(ctx context.Context) (Status, error) {
	var status Status
	queries := []struct {
		dest  *int
		query string
	}{
		{&status.ActiveRuns, "SELECT count(*) FROM runs WHERE status = 'active'"},
		{&status.InterruptedRuns, "SELECT count(*) FROM runs WHERE status = 'interrupted'"},
		{&status.PendingApprovals, "SELECT count(*) FROM approvals WHERE status = 'pending'"},
	}

	for _, item := range queries {
		if err := a.db.RawDB().QueryRowContext(ctx, item.query).Scan(item.dest); err != nil {
			return Status{}, err
		}
	}

	return status, nil
}

func (a *App) ListRuns(ctx context.Context) ([]model.Run, error) {
	rows, err := a.db.RawDB().QueryContext(ctx,
		`SELECT id, conversation_id, agent_id, COALESCE(team_id, ''), COALESCE(parent_run_id, ''),
		        COALESCE(objective, ''), COALESCE(workspace_root, ''), status,
		        input_tokens, output_tokens, created_at, updated_at
		 FROM runs
		 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	runs := make([]model.Run, 0)
	for rows.Next() {
		var run model.Run
		var status string
		if err := rows.Scan(
			&run.ID,
			&run.ConversationID,
			&run.AgentID,
			&run.TeamID,
			&run.ParentRunID,
			&run.Objective,
			&run.WorkspaceRoot,
			&status,
			&run.InputTokens,
			&run.OutputTokens,
			&run.CreatedAt,
			&run.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		run.Status = model.RunStatus(status)
		runs = append(runs, run)
	}

	return runs, rows.Err()
}

func (a *App) LoadReplay(ctx context.Context, runID string) (replay.RunReplay, error) {
	return a.replay.LoadRun(ctx, runID)
}

func (a *App) AdminToken(ctx context.Context) (string, error) {
	var token string
	err := a.db.RawDB().QueryRowContext(ctx,
		"SELECT value FROM settings WHERE key = 'admin_token'",
	).Scan(&token)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("admin token is not set")
	}
	if err != nil {
		return "", fmt.Errorf("load admin token: %w", err)
	}
	return token, nil
}
