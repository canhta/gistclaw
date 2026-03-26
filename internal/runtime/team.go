package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/teams"
)

func (r *Runtime) SetTeamDir(teamDir string) {
	r.teamDir = teamDir
}

func (r *Runtime) resolveTeamDir(ctx context.Context) string {
	project, err := ActiveProject(ctx, r.store)
	if err == nil && project.WorkspaceRoot != "" {
		teamDir := filepath.Join(project.WorkspaceRoot, ".gistclaw", "teams", "default")
		if _, statErr := os.Stat(filepath.Join(teamDir, "team.yaml")); statErr == nil {
			return teamDir
		}
	}
	return r.teamDir
}

func (r *Runtime) executionSnapshotForContext(ctx context.Context) (model.ExecutionSnapshot, []byte, error) {
	teamDir := r.resolveTeamDir(ctx)
	if teamDir == "" {
		return r.defaultSnapshot, append([]byte(nil), r.defaultSnapshotJSON...), nil
	}

	snapshot, err := teams.LoadExecutionSnapshot(teamDir)
	if err != nil {
		return r.defaultSnapshot, append([]byte(nil), r.defaultSnapshotJSON...), nil
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return model.ExecutionSnapshot{}, nil, fmt.Errorf("runtime: marshal execution snapshot: %w", err)
	}
	return snapshot, raw, nil
}

func (r *Runtime) TeamConfig(ctx context.Context) (teams.Config, error) {
	teamDir := r.resolveTeamDir(ctx)
	if teamDir == "" {
		return teams.Config{}, fmt.Errorf("runtime: team dir not configured")
	}
	cfg, err := teams.LoadConfig(teamDir)
	if err != nil {
		return teams.Config{}, fmt.Errorf("runtime: load team config: %w", err)
	}
	return cfg, nil
}

func (r *Runtime) UpdateTeam(ctx context.Context, cfg teams.Config) error {
	teamDir := r.resolveTeamDir(ctx)
	if teamDir == "" {
		return fmt.Errorf("runtime: team dir not configured")
	}
	if err := teams.WriteConfig(teamDir, cfg); err != nil {
		return fmt.Errorf("runtime: write team config: %w", err)
	}
	snapshot, err := teams.LoadExecutionSnapshot(teamDir)
	if err != nil {
		return fmt.Errorf("runtime: reload team snapshot: %w", err)
	}
	if err := r.SetDefaultExecutionSnapshot(snapshot); err != nil {
		return fmt.Errorf("runtime: refresh default execution snapshot: %w", err)
	}
	return nil
}
