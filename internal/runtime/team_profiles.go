package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/teams"
)

func ActiveProjectTeamProfile(ctx context.Context, db *store.DB) (string, error) {
	if db == nil {
		return teams.DefaultProfileName, nil
	}

	project, err := ActiveProject(ctx, db)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(project.ID) == "" {
		return teams.DefaultProfileName, nil
	}
	return ProjectTeamProfile(ctx, db, project.ID)
}

func ProjectTeamProfile(ctx context.Context, db *store.DB, projectID string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return teams.DefaultProfileName, nil
	}

	profile, err := lookupProjectSetting(ctx, db, teamProfileSettingKey(projectID))
	if err != nil {
		return "", err
	}
	if profile == "" {
		return teams.DefaultProfileName, nil
	}

	normalized, err := teams.NormalizeProfileName(profile)
	if err != nil {
		return "", fmt.Errorf("runtime: invalid team profile for project %q: %w", projectID, err)
	}
	return normalized, nil
}

func SetActiveProjectTeamProfile(ctx context.Context, db *store.DB, projectID, profile string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return fmt.Errorf("runtime: project id is required")
	}
	if _, err := loadProjectByID(ctx, db, projectID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("runtime: unknown project %q", projectID)
		}
		return fmt.Errorf("runtime: load project for profile update: %w", err)
	}

	normalized, err := teams.NormalizeProfileName(profile)
	if err != nil {
		return fmt.Errorf("runtime: invalid team profile: %w", err)
	}
	return upsertProjectSetting(ctx, db, teamProfileSettingKey(projectID), normalized)
}

func teamProfileSettingKey(projectID string) string {
	return "project." + projectID + ".team_profile"
}

func (r *Runtime) ActiveTeamProfile(ctx context.Context) (string, error) {
	return ActiveProjectTeamProfile(ctx, r.store)
}

func (r *Runtime) ListTeamProfiles(ctx context.Context) ([]teams.Profile, error) {
	workspaceRoot, err := r.activeProjectWorkspaceRoot(ctx)
	if err != nil {
		return nil, err
	}
	return teams.ListProfiles(workspaceRoot)
}

func (r *Runtime) SelectTeamProfile(ctx context.Context, profile string) error {
	project, err := ActiveProject(ctx, r.store)
	if err != nil {
		return err
	}
	if project.ID == "" {
		return fmt.Errorf("runtime: active project is required")
	}
	return SetActiveProjectTeamProfile(ctx, r.store, project.ID, profile)
}

func (r *Runtime) CreateTeamProfile(ctx context.Context, profile string) error {
	workspaceRoot, err := r.activeProjectWorkspaceRoot(ctx)
	if err != nil {
		return err
	}
	return teams.CreateProfile(workspaceRoot, profile)
}

func (r *Runtime) CloneTeamProfile(ctx context.Context, sourceProfile, newProfile string) error {
	workspaceRoot, err := r.activeProjectWorkspaceRoot(ctx)
	if err != nil {
		return err
	}
	sourceProfile, err = teams.NormalizeProfileName(sourceProfile)
	if err != nil {
		return fmt.Errorf("runtime: invalid source team profile: %w", err)
	}
	sourceDir, err := r.cloneSourceDir(ctx, sourceProfile, workspaceRoot)
	if err != nil {
		return err
	}
	return teams.CloneProfileFromDir(workspaceRoot, sourceDir, newProfile)
}

func (r *Runtime) DeleteTeamProfile(ctx context.Context, profile string) error {
	activeProfile, err := r.ActiveTeamProfile(ctx)
	if err != nil {
		return err
	}
	profile, err = teams.NormalizeProfileName(profile)
	if err != nil {
		return fmt.Errorf("runtime: invalid team profile: %w", err)
	}
	if profile == activeProfile {
		return fmt.Errorf("runtime: choose another active profile before deleting %s", profile)
	}

	workspaceRoot, err := r.activeProjectWorkspaceRoot(ctx)
	if err != nil {
		return err
	}
	return teams.DeleteProfile(workspaceRoot, profile)
}

func (r *Runtime) activeProjectWorkspaceRoot(ctx context.Context) (string, error) {
	project, err := ActiveProject(ctx, r.store)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(project.WorkspaceRoot) == "" {
		return "", fmt.Errorf("runtime: active project workspace not configured")
	}
	return project.WorkspaceRoot, nil
}

func (r *Runtime) cloneSourceDir(ctx context.Context, profile, workspaceRoot string) (string, error) {
	sourceDir := teams.ProfileDir(workspaceRoot, profile)
	if _, err := os.Stat(filepath.Join(sourceDir, "team.yaml")); err == nil {
		return sourceDir, nil
	}
	if profile == teams.DefaultProfileName && r.teamDir != "" {
		if _, err := os.Stat(filepath.Join(r.teamDir, "team.yaml")); err == nil {
			return r.teamDir, nil
		}
	}
	return "", fmt.Errorf("runtime: team profile %q not found", profile)
}
