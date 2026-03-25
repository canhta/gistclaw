package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

func ActiveProject(ctx context.Context, db *store.DB) (model.Project, error) {
	if db == nil {
		return model.Project{}, nil
	}

	activeProjectID, err := lookupProjectSetting(ctx, db, "active_project_id")
	if err != nil {
		return model.Project{}, err
	}
	if activeProjectID != "" {
		project, err := loadProjectByID(ctx, db, activeProjectID)
		if err == nil {
			return project, nil
		}
		if err != sql.ErrNoRows {
			return model.Project{}, fmt.Errorf("runtime: load active project: %w", err)
		}
	}

	workspaceRoot, err := lookupProjectSetting(ctx, db, "workspace_root")
	if err != nil {
		return model.Project{}, err
	}
	if workspaceRoot == "" {
		return model.Project{}, nil
	}

	project, err := loadProjectByWorkspaceRoot(ctx, db, workspaceRoot)
	if err == nil {
		return project, nil
	}
	if err != sql.ErrNoRows {
		return model.Project{}, fmt.Errorf("runtime: load active project by workspace: %w", err)
	}

	return model.Project{
		Name:          projectNameFromWorkspace(workspaceRoot),
		WorkspaceRoot: workspaceRoot,
	}, nil
}

func ListProjects(ctx context.Context, db *store.DB) ([]model.Project, error) {
	rows, err := db.RawDB().QueryContext(ctx,
		`SELECT id, name, workspace_root, source, created_at, last_used_at
		 FROM projects
		 ORDER BY last_used_at DESC, created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("runtime: list projects: %w", err)
	}
	defer rows.Close()

	var projects []model.Project
	for rows.Next() {
		var project model.Project
		if err := rows.Scan(
			&project.ID,
			&project.Name,
			&project.WorkspaceRoot,
			&project.Source,
			&project.CreatedAt,
			&project.LastUsedAt,
		); err != nil {
			return nil, fmt.Errorf("runtime: scan project: %w", err)
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("runtime: iterate projects: %w", err)
	}
	if len(projects) > 0 {
		return projects, nil
	}

	legacyWorkspace, err := lookupProjectSetting(ctx, db, "workspace_root")
	if err != nil {
		return nil, err
	}
	if legacyWorkspace == "" {
		return nil, nil
	}
	return []model.Project{{
		Name:          projectNameFromWorkspace(legacyWorkspace),
		WorkspaceRoot: legacyWorkspace,
	}}, nil
}

func ActivateWorkspace(ctx context.Context, db *store.DB, workspaceRoot, name, source string) (model.Project, error) {
	project, err := RegisterProject(ctx, db, workspaceRoot, name, source)
	if err != nil {
		return model.Project{}, err
	}
	if err := SetActiveProject(ctx, db, project.ID); err != nil {
		return model.Project{}, err
	}
	return project, nil
}

func RegisterProject(ctx context.Context, db *store.DB, workspaceRoot, name, source string) (model.Project, error) {
	workspaceRoot = normalizeWorkspaceRoot(workspaceRoot)
	if workspaceRoot == "" {
		return model.Project{}, fmt.Errorf("runtime: workspace_root is required")
	}
	if name == "" {
		name = projectNameFromWorkspace(workspaceRoot)
	}
	if source == "" {
		source = "operator"
	}

	project, err := loadProjectByWorkspaceRoot(ctx, db, workspaceRoot)
	if err == nil {
		return project, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return model.Project{}, fmt.Errorf("runtime: load project by workspace: %w", err)
	}

	projectID := generateID()
	if _, err := db.RawDB().ExecContext(ctx,
		`INSERT INTO projects (id, name, workspace_root, source, created_at, last_used_at)
		 VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		projectID, name, workspaceRoot, source,
	); err != nil {
		return model.Project{}, fmt.Errorf("runtime: insert project: %w", err)
	}

	project, err = loadProjectByID(ctx, db, projectID)
	if err != nil {
		return model.Project{}, fmt.Errorf("runtime: reload project: %w", err)
	}
	return project, nil
}

func SetActiveProject(ctx context.Context, db *store.DB, projectID string) error {
	project, err := loadProjectByID(ctx, db, projectID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("runtime: unknown project %q", projectID)
		}
		return fmt.Errorf("runtime: load project for activation: %w", err)
	}

	if err := upsertProjectSetting(ctx, db, "active_project_id", project.ID); err != nil {
		return err
	}
	if err := upsertProjectSetting(ctx, db, "workspace_root", project.WorkspaceRoot); err != nil {
		return err
	}
	if _, err := db.RawDB().ExecContext(ctx,
		`UPDATE projects SET last_used_at = datetime('now') WHERE id = ?`,
		project.ID,
	); err != nil {
		return fmt.Errorf("runtime: touch active project: %w", err)
	}
	return nil
}

func lookupProjectSetting(ctx context.Context, db *store.DB, key string) (string, error) {
	var value string
	err := db.RawDB().QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("runtime: lookup setting %q: %w", key, err)
	}
	return value, nil
}

func upsertProjectSetting(ctx context.Context, db *store.DB, key, value string) error {
	if _, err := db.RawDB().ExecContext(ctx,
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value,
	); err != nil {
		return fmt.Errorf("runtime: update setting %q: %w", key, err)
	}
	return nil
}

func loadProjectByID(ctx context.Context, db *store.DB, projectID string) (model.Project, error) {
	var project model.Project
	err := db.RawDB().QueryRowContext(ctx,
		`SELECT id, name, workspace_root, source, created_at, last_used_at
		 FROM projects
		 WHERE id = ?`,
		projectID,
	).Scan(
		&project.ID,
		&project.Name,
		&project.WorkspaceRoot,
		&project.Source,
		&project.CreatedAt,
		&project.LastUsedAt,
	)
	if err != nil {
		return model.Project{}, err
	}
	return project, nil
}

func loadProjectByWorkspaceRoot(ctx context.Context, db *store.DB, workspaceRoot string) (model.Project, error) {
	var project model.Project
	err := db.RawDB().QueryRowContext(ctx,
		`SELECT id, name, workspace_root, source, created_at, last_used_at
		 FROM projects
		 WHERE workspace_root = ?`,
		normalizeWorkspaceRoot(workspaceRoot),
	).Scan(
		&project.ID,
		&project.Name,
		&project.WorkspaceRoot,
		&project.Source,
		&project.CreatedAt,
		&project.LastUsedAt,
	)
	if err != nil {
		return model.Project{}, err
	}
	return project, nil
}

func normalizeWorkspaceRoot(workspaceRoot string) string {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return ""
	}
	return filepath.Clean(workspaceRoot)
}

func projectNameFromWorkspace(workspaceRoot string) string {
	name := filepath.Base(normalizeWorkspaceRoot(workspaceRoot))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "project"
	}
	return name
}
