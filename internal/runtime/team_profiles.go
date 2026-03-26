package runtime

import (
	"context"
	"database/sql"
	"fmt"
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
