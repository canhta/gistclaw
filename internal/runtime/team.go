package runtime

import (
	"context"
	"fmt"

	"github.com/canhta/gistclaw/internal/teams"
)

func (r *Runtime) SetTeamDir(teamDir string) {
	r.teamDir = teamDir
}

func (r *Runtime) TeamConfig() (teams.Config, error) {
	if r.teamDir == "" {
		return teams.Config{}, fmt.Errorf("runtime: team dir not configured")
	}
	cfg, err := teams.LoadConfig(r.teamDir)
	if err != nil {
		return teams.Config{}, fmt.Errorf("runtime: load team config: %w", err)
	}
	return cfg, nil
}

func (r *Runtime) UpdateTeam(_ context.Context, cfg teams.Config) error {
	if r.teamDir == "" {
		return fmt.Errorf("runtime: team dir not configured")
	}
	if err := teams.WriteConfig(r.teamDir, cfg); err != nil {
		return fmt.Errorf("runtime: write team config: %w", err)
	}
	snapshot, err := teams.LoadExecutionSnapshot(r.teamDir)
	if err != nil {
		return fmt.Errorf("runtime: reload team snapshot: %w", err)
	}
	if err := r.SetDefaultExecutionSnapshot(snapshot); err != nil {
		return fmt.Errorf("runtime: refresh default execution snapshot: %w", err)
	}
	return nil
}
