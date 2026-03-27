package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestPrepareStartRun_UsesActiveProjectPrimaryPathWhenCWDOmitted(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	projectID := generateID()
	primaryPath := "/tmp/project-alpha"
	if _, err := db.RawDB().Exec(
		`INSERT INTO projects (id, name, primary_path, roots_json, policy_json, source, created_at, last_used_at)
		 VALUES (?, ?, ?, '{}', '{}', 'operator', datetime('now'), datetime('now'))`,
		projectID, "project-alpha", primaryPath,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := db.RawDB().Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES ('active_project_id', ?, datetime('now'))`,
		projectID,
	); err != nil {
		t.Fatalf("seed active_project_id: %v", err)
	}

	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})

	cmd, err := rt.prepareStartRun(context.Background(), "", StartRun{
		ConversationID: "conv-project",
		AgentID:        "assistant",
		Objective:      "review the repo",
	})
	if err != nil {
		t.Fatalf("prepareStartRun: %v", err)
	}
	if cmd.ProjectID != projectID {
		t.Fatalf("project id = %q, want %q", cmd.ProjectID, projectID)
	}
	if cmd.CWD != primaryPath {
		t.Fatalf("cwd = %q, want %q", cmd.CWD, primaryPath)
	}
}

func TestActivateProjectPath_RegistersAndActivatesProjectWithoutCreatingRepo(t *testing.T) {
	db, _, _, _ := setupRunTestDeps(t)
	primaryPath := filepath.Join(t.TempDir(), "project-alpha", "nested")

	project, err := ActivateProjectPath(context.Background(), db, primaryPath, "seo-test", "operator")
	if err != nil {
		t.Fatalf("ActivateProjectPath: %v", err)
	}
	if project.ID == "" {
		t.Fatal("expected activated project id to be set")
	}
	if project.PrimaryPath != primaryPath {
		t.Fatalf("primary_path = %q, want %q", project.PrimaryPath, primaryPath)
	}

	active, err := ActiveProject(context.Background(), db)
	if err != nil {
		t.Fatalf("ActiveProject: %v", err)
	}
	if active.ID != project.ID {
		t.Fatalf("expected active project %q, got %q", project.ID, active.ID)
	}
	if active.PrimaryPath != primaryPath {
		t.Fatalf("active primary_path = %q, want %q", active.PrimaryPath, primaryPath)
	}

	projects, err := ListProjects(context.Background(), db)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Name != "seo-test" {
		t.Fatalf("expected project name %q, got %q", "seo-test", projects[0].Name)
	}
	if _, err := os.Stat(primaryPath); !os.IsNotExist(err) {
		t.Fatalf("expected activation to keep %q as metadata only, stat err = %v", primaryPath, err)
	}
}

func TestRegisterProjectPath_ReusesExistingPrimaryPath(t *testing.T) {
	db, _, _, _ := setupRunTestDeps(t)

	first, err := RegisterProjectPath(context.Background(), db, "/tmp/project-alpha", "alpha", "starter")
	if err != nil {
		t.Fatalf("first RegisterProjectPath: %v", err)
	}
	second, err := RegisterProjectPath(context.Background(), db, "/tmp/project-alpha", "override", "operator")
	if err != nil {
		t.Fatalf("second RegisterProjectPath: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected duplicate primary_path registration to reuse %q, got %q", first.ID, second.ID)
	}
}

func TestActiveProject_ReturnsEmptyWithoutActivation(t *testing.T) {
	db, _, _, _ := setupRunTestDeps(t)

	project, err := ActiveProject(context.Background(), db)
	if err != nil {
		t.Fatalf("ActiveProject: %v", err)
	}
	if project != (model.Project{}) {
		t.Fatalf("expected no active project, got %#v", project)
	}
}

func TestSetActiveProject_RejectsUnknownProject(t *testing.T) {
	db, _, _, _ := setupRunTestDeps(t)

	if err := SetActiveProject(context.Background(), db, "missing-project"); err == nil {
		t.Fatal("expected SetActiveProject to reject unknown project")
	}
}

func TestActiveProjectTeamProfile_DefaultsToDefault(t *testing.T) {
	db, _, _, _ := setupRunTestDeps(t)
	primaryPath := filepath.Join(t.TempDir(), "project-alpha")

	project, err := ActivateProjectPath(context.Background(), db, primaryPath, "alpha", "operator")
	if err != nil {
		t.Fatalf("ActivateProjectPath: %v", err)
	}
	if err := SetActiveProject(context.Background(), db, project.ID); err != nil {
		t.Fatalf("SetActiveProject: %v", err)
	}

	profile, err := ActiveProjectTeamProfile(context.Background(), db)
	if err != nil {
		t.Fatalf("ActiveProjectTeamProfile: %v", err)
	}
	if profile != "default" {
		t.Fatalf("expected default profile, got %q", profile)
	}
}

func TestSetActiveProjectTeamProfile_PersistsPerProject(t *testing.T) {
	db, _, _, _ := setupRunTestDeps(t)
	alphaPath := filepath.Join(t.TempDir(), "project-alpha")
	betaPath := filepath.Join(t.TempDir(), "project-beta")

	alpha, err := ActivateProjectPath(context.Background(), db, alphaPath, "alpha", "operator")
	if err != nil {
		t.Fatalf("ActivateProjectPath alpha: %v", err)
	}
	beta, err := ActivateProjectPath(context.Background(), db, betaPath, "beta", "operator")
	if err != nil {
		t.Fatalf("ActivateProjectPath beta: %v", err)
	}

	if err := SetActiveProjectTeamProfile(context.Background(), db, alpha.ID, "review"); err != nil {
		t.Fatalf("SetActiveProjectTeamProfile alpha: %v", err)
	}
	if err := SetActiveProjectTeamProfile(context.Background(), db, beta.ID, "ops"); err != nil {
		t.Fatalf("SetActiveProjectTeamProfile beta: %v", err)
	}

	if err := SetActiveProject(context.Background(), db, alpha.ID); err != nil {
		t.Fatalf("SetActiveProject alpha: %v", err)
	}
	alphaProfile, err := ActiveProjectTeamProfile(context.Background(), db)
	if err != nil {
		t.Fatalf("ActiveProjectTeamProfile alpha: %v", err)
	}
	if alphaProfile != "review" {
		t.Fatalf("expected alpha profile %q, got %q", "review", alphaProfile)
	}

	if err := SetActiveProject(context.Background(), db, beta.ID); err != nil {
		t.Fatalf("SetActiveProject beta: %v", err)
	}
	betaProfile, err := ActiveProjectTeamProfile(context.Background(), db)
	if err != nil {
		t.Fatalf("ActiveProjectTeamProfile beta: %v", err)
	}
	if betaProfile != "ops" {
		t.Fatalf("expected beta profile %q, got %q", "ops", betaProfile)
	}
}
