package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestPrepareStartRun_UsesActiveWorkspaceWhenWorkspaceRootOmitted(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	if _, err := db.RawDB().Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES ('workspace_root', '/tmp/project-alpha', datetime('now'))`,
	); err != nil {
		t.Fatalf("seed workspace_root: %v", err)
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
	if cmd.WorkspaceRoot != "/tmp/project-alpha" {
		t.Fatalf("expected omitted workspace_root to resolve to active workspace, got %q", cmd.WorkspaceRoot)
	}
}

func TestActivateWorkspace_RegistersAndActivatesProject(t *testing.T) {
	db, _, _, _ := setupRunTestDeps(t)
	workspaceRoot := filepath.Join(t.TempDir(), "project-alpha")

	project, err := ActivateWorkspace(context.Background(), db, workspaceRoot, "seo-test", "operator")
	if err != nil {
		t.Fatalf("ActivateWorkspace: %v", err)
	}
	if project.ID == "" {
		t.Fatal("expected activated project id to be set")
	}

	active, err := ActiveProject(context.Background(), db)
	if err != nil {
		t.Fatalf("ActiveProject: %v", err)
	}
	if active.ID != project.ID {
		t.Fatalf("expected active project %q, got %q", project.ID, active.ID)
	}
	if active.WorkspaceRoot != workspaceRoot {
		t.Fatalf("expected active workspace root %q, got %q", workspaceRoot, active.WorkspaceRoot)
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
	if _, err := os.Stat(filepath.Join(workspaceRoot, ".git")); err != nil {
		t.Fatalf("expected activated workspace to be initialized as a git repo: %v", err)
	}
}

func TestRegisterProject_ReusesExistingWorkspace(t *testing.T) {
	db, _, _, _ := setupRunTestDeps(t)

	first, err := RegisterProject(context.Background(), db, "/tmp/project-alpha", "alpha", "starter")
	if err != nil {
		t.Fatalf("first RegisterProject: %v", err)
	}
	second, err := RegisterProject(context.Background(), db, "/tmp/project-alpha", "override", "operator")
	if err != nil {
		t.Fatalf("second RegisterProject: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected duplicate workspace registration to reuse %q, got %q", first.ID, second.ID)
	}
}

func TestActiveProject_FallsBackToLegacyWorkspaceSetting(t *testing.T) {
	db, _, _, _ := setupRunTestDeps(t)
	if _, err := db.RawDB().Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES ('workspace_root', '/tmp/legacy-project', datetime('now'))`,
	); err != nil {
		t.Fatalf("seed workspace_root: %v", err)
	}

	project, err := ActiveProject(context.Background(), db)
	if err != nil {
		t.Fatalf("ActiveProject: %v", err)
	}
	if project.ID != "" {
		t.Fatalf("expected legacy fallback to have no stored project id, got %q", project.ID)
	}
	if project.Name != "legacy-project" {
		t.Fatalf("expected legacy fallback name %q, got %q", "legacy-project", project.Name)
	}
}

func TestListProjects_FallsBackToLegacyWorkspaceSetting(t *testing.T) {
	db, _, _, _ := setupRunTestDeps(t)
	if _, err := db.RawDB().Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES ('workspace_root', '/tmp/legacy-project', datetime('now'))`,
	); err != nil {
		t.Fatalf("seed workspace_root: %v", err)
	}

	projects, err := ListProjects(context.Background(), db)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 legacy project fallback, got %d", len(projects))
	}
	if projects[0].WorkspaceRoot != "/tmp/legacy-project" {
		t.Fatalf("expected legacy workspace root %q, got %q", "/tmp/legacy-project", projects[0].WorkspaceRoot)
	}
}

func TestSetActiveProject_RejectsUnknownProject(t *testing.T) {
	db, _, _, _ := setupRunTestDeps(t)

	if err := SetActiveProject(context.Background(), db, "missing-project"); err == nil {
		t.Fatal("expected SetActiveProject to reject unknown project")
	}
}
