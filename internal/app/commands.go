package app

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	authpkg "github.com/canhta/gistclaw/internal/auth"
	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/replay"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

type Status struct {
	ActiveRuns       int
	InterruptedRuns  int
	PendingApprovals int
	Storage          store.HealthReport
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

	health, err := store.LoadHealth(a.cfg.DatabasePath, time.Now().UTC())
	if err != nil {
		return Status{}, fmt.Errorf("load storage health: %w", err)
	}
	status.Storage = health

	return status, nil
}

func (a *App) ListRuns(ctx context.Context) ([]model.Run, error) {
	rows, err := a.db.RawDB().QueryContext(ctx,
		`SELECT id, conversation_id, agent_id, COALESCE(team_id, ''), COALESCE(parent_run_id, ''),
		        COALESCE(objective, ''), COALESCE(cwd, ''), status,
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
			&run.CWD,
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

func (a *App) SetPassword(ctx context.Context, password string, now time.Time) error {
	if err := authpkg.SetPassword(ctx, a.db, password, now); err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	return nil
}

func (a *App) ConnectorHealth(context.Context) ([]model.ConnectorHealthSnapshot, error) {
	return collectConnectorHealthSnapshots(a.connectors), nil
}

func ConfiguredConnectorHealth(ctx context.Context, cfg Config, db *store.DB) ([]model.ConnectorHealthSnapshot, error) {
	if db == nil {
		return nil, fmt.Errorf("connector health: db is required")
	}

	cs := conversations.NewConversationStore(db)
	mem := memory.NewStore(db, cs)
	rt := runtime.New(db, cs, tools.NewRegistry(), mem, nil, nil)
	app := &App{
		connectors: buildConnectors(cfg, db, cs, rt, nil),
	}

	snapshots, err := app.ConnectorHealth(ctx)
	if err != nil {
		return nil, err
	}

	persisted, err := loadRecentConnectorHealthSnapshots(ctx, db, time.Now().UTC(), connectorHealthSnapshotMaxAge)
	if err != nil {
		return nil, err
	}
	connectorsByID := make(map[string]model.Connector, len(app.connectors))
	for _, connector := range app.connectors {
		meta := connector.Metadata()
		if meta.ID == "" {
			continue
		}
		connectorsByID[meta.ID] = connector
	}
	for i := range snapshots {
		if snapshot, ok := persisted[snapshots[i].ConnectorID]; ok {
			snapshots[i] = snapshot
		}
		connector := connectorsByID[snapshots[i].ConnectorID]
		if snapshot, ok, err := fallbackConfiguredConnectorHealth(ctx, connector, snapshots[i]); err != nil {
			return nil, err
		} else if ok {
			snapshots[i] = snapshot
		}
	}
	return snapshots, nil
}

func fallbackConfiguredConnectorHealth(
	ctx context.Context,
	connector model.Connector,
	snapshot model.ConnectorHealthSnapshot,
) (model.ConnectorHealthSnapshot, bool, error) {
	reporter, ok := connector.(model.ConnectorConfiguredHealthReporter)
	if !ok {
		return model.ConnectorHealthSnapshot{}, false, nil
	}
	fallback, ok, err := reporter.ConfiguredConnectorHealth(ctx, snapshot)
	if err != nil {
		return model.ConnectorHealthSnapshot{}, false, fmt.Errorf("connector health: configured readiness for %s: %w", snapshot.ConnectorID, err)
	}
	if !ok {
		return model.ConnectorHealthSnapshot{}, false, nil
	}
	if strings.TrimSpace(fallback.ConnectorID) == "" {
		fallback.ConnectorID = snapshot.ConnectorID
	}
	return fallback, true, nil
}

func collectConnectorHealthSnapshots(connectors []model.Connector) []model.ConnectorHealthSnapshot {
	snapshots := make([]model.ConnectorHealthSnapshot, 0, len(connectors))
	for _, connector := range connectors {
		reporter, ok := connector.(connectorHealthReporter)
		if !ok {
			continue
		}
		snapshots = append(snapshots, reporter.ConnectorHealthSnapshot())
	}
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].ConnectorID < snapshots[j].ConnectorID
	})
	return snapshots
}
