package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/teams"
)

func (r *Runtime) SetTeamDir(teamDir string) {
	r.teamDir = teamDir
}

func (r *Runtime) TeamConfigPath(ctx context.Context) (string, error) {
	teamDir, err := r.resolveTeamDir(ctx)
	if err != nil {
		return "", fmt.Errorf("runtime: resolve team dir: %w", err)
	}
	if teamDir == "" {
		return "", fmt.Errorf("runtime: team dir not configured")
	}
	return filepath.Join(teamDir, "team.yaml"), nil
}

func (r *Runtime) resolveTeamDir(ctx context.Context) (string, error) {
	projectTeamDir, err := r.activeProjectTeamDir(ctx)
	if err != nil {
		return "", err
	}
	if projectTeamDir != "" {
		return projectTeamDir, nil
	}
	return r.teamDir, nil
}

func (r *Runtime) activeProjectTeamDir(ctx context.Context) (string, error) {
	project, err := ActiveProject(ctx, r.store)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(project.ID) == "" || strings.TrimSpace(r.storageRoot) == "" {
		return "", nil
	}
	profile, err := ActiveProjectTeamProfile(ctx, r.store)
	if err != nil {
		return "", err
	}
	return filepath.Join(r.storageRoot, "projects", project.ID, "teams", profile), nil
}

func (r *Runtime) resolveReadableTeamDir(ctx context.Context) (string, error) {
	teamDir, err := r.resolveTeamDir(ctx)
	if err != nil {
		return "", err
	}
	if teamDir == "" {
		return "", nil
	}
	if _, statErr := os.Stat(filepath.Join(teamDir, "team.yaml")); statErr == nil {
		return teamDir, nil
	}
	if teamDir != "" {
		profile, profileErr := ActiveProjectTeamProfile(ctx, r.store)
		if profileErr != nil {
			return "", profileErr
		}
		if profile == teams.DefaultProfileName && r.teamDir != "" {
			if _, statErr := os.Stat(filepath.Join(r.teamDir, "team.yaml")); statErr == nil {
				return r.teamDir, nil
			}
		}
	}
	if teamDir != r.teamDir {
		return teamDir, nil
	}
	return r.teamDir, nil
}

func (r *Runtime) executionSnapshotForContext(ctx context.Context) (model.ExecutionSnapshot, []byte, error) {
	teamDir, err := r.resolveReadableTeamDir(ctx)
	if err != nil || teamDir == "" {
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

func (r *Runtime) FrontAgentID(ctx context.Context) (string, error) {
	snapshot, _, err := r.executionSnapshotForContext(ctx)
	if err != nil {
		return "", err
	}
	return frontAgentIDFromSnapshot(snapshot)
}

func frontAgentIDFromSnapshot(snapshot model.ExecutionSnapshot) (string, error) {
	frontAgentID := strings.TrimSpace(snapshot.FrontAgentID)
	if frontAgentID == "" {
		return "", fmt.Errorf("runtime: front agent is not configured")
	}
	if len(snapshot.Agents) > 0 {
		if _, ok := snapshot.Agents[frontAgentID]; !ok {
			return "", fmt.Errorf("runtime: front agent %q is not present in execution snapshot", frontAgentID)
		}
	}
	return frontAgentID, nil
}

func (r *Runtime) TeamConfig(ctx context.Context) (teams.Config, error) {
	teamDir, err := r.resolveReadableTeamDir(ctx)
	if err != nil {
		return teams.Config{}, fmt.Errorf("runtime: resolve team dir: %w", err)
	}
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
	teamDir, err := r.resolveTeamDir(ctx)
	if err != nil {
		return fmt.Errorf("runtime: resolve team dir: %w", err)
	}
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
