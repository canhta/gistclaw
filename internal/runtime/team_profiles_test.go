package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/teams"
)

func TestRuntime_TeamConfigUsesActiveProfileForProject(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	rt := New(db, cs, reg, nil, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})
	ctx := context.Background()
	storageRoot := t.TempDir()
	rt.SetStorageRoot(storageRoot)

	workspaceRoot := t.TempDir()
	project, err := ActivateProjectPath(ctx, db, workspaceRoot, "alpha", "operator")
	if err != nil {
		t.Fatalf("activate project: %v", err)
	}
	writeRuntimeStoredTeamProfile(t, storageRoot, project.ID, "default", "Default Team")
	writeRuntimeStoredTeamProfile(t, storageRoot, project.ID, "review", "Review Team")
	if err := SetActiveProject(ctx, db, project.ID); err != nil {
		t.Fatalf("set active project: %v", err)
	}
	if err := SetActiveProjectTeamProfile(ctx, db, project.ID, "review"); err != nil {
		t.Fatalf("SetActiveProjectTeamProfile: %v", err)
	}

	cfg, err := rt.TeamConfig(ctx)
	if err != nil {
		t.Fatalf("TeamConfig: %v", err)
	}
	if cfg.Name != "Review Team" {
		t.Fatalf("expected active profile team name %q, got %q", "Review Team", cfg.Name)
	}
}

func TestRuntime_ChangingActiveProfileOnlyAffectsFutureRuns(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	rt := New(db, cs, reg, nil, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})
	ctx := context.Background()
	storageRoot := t.TempDir()
	rt.SetStorageRoot(storageRoot)

	workspaceRoot := t.TempDir()
	project, err := ActivateProjectPath(ctx, db, workspaceRoot, "alpha", "operator")
	if err != nil {
		t.Fatalf("activate project: %v", err)
	}
	writeRuntimeStoredTeamProfile(t, storageRoot, project.ID, "default", "Default Team")
	writeRuntimeStoredTeamProfile(t, storageRoot, project.ID, "review", "Review Team")
	if err := SetActiveProject(ctx, db, project.ID); err != nil {
		t.Fatalf("set active project: %v", err)
	}

	first, err := rt.prepareStartRun(ctx, "", StartRun{
		ConversationID: "conv-team-default",
		AgentID:        "assistant",
		Objective:      "use default team",
		CWD:            workspaceRoot,
	})
	if err != nil {
		t.Fatalf("prepareStartRun default: %v", err)
	}

	if err := SetActiveProjectTeamProfile(ctx, db, project.ID, "review"); err != nil {
		t.Fatalf("SetActiveProjectTeamProfile: %v", err)
	}

	second, err := rt.prepareStartRun(ctx, "", StartRun{
		ConversationID: "conv-team-review",
		AgentID:        "assistant",
		Objective:      "use review team",
		CWD:            workspaceRoot,
	})
	if err != nil {
		t.Fatalf("prepareStartRun review: %v", err)
	}

	firstSnapshot, err := decodeExecutionSnapshot(first.ExecutionSnapshotJSON)
	if err != nil {
		t.Fatalf("decodeExecutionSnapshot first: %v", err)
	}
	secondSnapshot, err := decodeExecutionSnapshot(second.ExecutionSnapshotJSON)
	if err != nil {
		t.Fatalf("decodeExecutionSnapshot second: %v", err)
	}
	if firstSnapshot.TeamID != "Default Team" {
		t.Fatalf("expected first snapshot team id %q, got %q", "Default Team", firstSnapshot.TeamID)
	}
	if secondSnapshot.TeamID != "Review Team" {
		t.Fatalf("expected second snapshot team id %q, got %q", "Review Team", secondSnapshot.TeamID)
	}
}

func TestRuntime_TeamProfileManagementMethods(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	rt := New(db, cs, reg, nil, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})
	ctx := context.Background()
	storageRoot := t.TempDir()
	rt.SetStorageRoot(storageRoot)

	writeRuntimeGlobalTeamProfile(t, storageRoot, "default", "Fallback Team")
	rt.SetTeamDir(filepath.Join(storageRoot, "teams", "default"))

	workspaceRoot := t.TempDir()
	project, err := ActivateProjectPath(ctx, db, workspaceRoot, "alpha", "operator")
	if err != nil {
		t.Fatalf("activate project: %v", err)
	}
	if err := SetActiveProject(ctx, db, project.ID); err != nil {
		t.Fatalf("set active project: %v", err)
	}

	activeProfile, err := rt.ActiveTeamProfile(ctx)
	if err != nil {
		t.Fatalf("ActiveTeamProfile default: %v", err)
	}
	if activeProfile != teams.DefaultProfileName {
		t.Fatalf("expected default active profile, got %q", activeProfile)
	}

	if err := rt.CreateTeamProfile(ctx, "review"); err != nil {
		t.Fatalf("CreateTeamProfile: %v", err)
	}
	if err := rt.CloneTeamProfile(ctx, teams.DefaultProfileName, "ops"); err != nil {
		t.Fatalf("CloneTeamProfile: %v", err)
	}

	profiles, err := rt.ListTeamProfiles(ctx)
	if err != nil {
		t.Fatalf("ListTeamProfiles: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 storage-managed profiles, got %d", len(profiles))
	}

	cloned, err := teams.LoadConfig(filepath.Join(storageRoot, "projects", project.ID, "teams", "ops"))
	if err != nil {
		t.Fatalf("LoadConfig cloned profile: %v", err)
	}
	if cloned.Name != "Fallback Team" {
		t.Fatalf("expected cloned profile team name %q, got %q", "Fallback Team", cloned.Name)
	}

	if err := rt.SelectTeamProfile(ctx, "review"); err != nil {
		t.Fatalf("SelectTeamProfile: %v", err)
	}
	activeProfile, err = rt.ActiveTeamProfile(ctx)
	if err != nil {
		t.Fatalf("ActiveTeamProfile review: %v", err)
	}
	if activeProfile != "review" {
		t.Fatalf("expected selected profile %q, got %q", "review", activeProfile)
	}

	if err := rt.DeleteTeamProfile(ctx, "ops"); err != nil {
		t.Fatalf("DeleteTeamProfile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(storageRoot, "projects", project.ID, "teams", "ops")); !os.IsNotExist(err) {
		t.Fatalf("expected deleted profile dir to be removed, err=%v", err)
	}
}

func TestRuntime_DeleteTeamProfileRejectsActiveProfile(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	rt := New(db, cs, reg, nil, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})
	ctx := context.Background()
	rt.SetStorageRoot(t.TempDir())

	workspaceRoot := t.TempDir()
	project, err := ActivateProjectPath(ctx, db, workspaceRoot, "alpha", "operator")
	if err != nil {
		t.Fatalf("activate project: %v", err)
	}
	if err := SetActiveProject(ctx, db, project.ID); err != nil {
		t.Fatalf("set active project: %v", err)
	}

	if err := rt.DeleteTeamProfile(ctx, teams.DefaultProfileName); err == nil {
		t.Fatal("expected DeleteTeamProfile to reject deleting the active profile")
	}
}

func TestRuntime_SelectTeamProfileRejectsUnknownProfile(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	rt := New(db, cs, reg, nil, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})
	ctx := context.Background()
	storageRoot := t.TempDir()
	rt.SetStorageRoot(storageRoot)

	writeRuntimeGlobalTeamProfile(t, storageRoot, "default", "Fallback Team")
	rt.SetTeamDir(filepath.Join(storageRoot, "teams", "default"))

	workspaceRoot := t.TempDir()
	project, err := ActivateProjectPath(ctx, db, workspaceRoot, "alpha", "operator")
	if err != nil {
		t.Fatalf("activate project: %v", err)
	}
	if err := SetActiveProject(ctx, db, project.ID); err != nil {
		t.Fatalf("set active project: %v", err)
	}

	if err := rt.SelectTeamProfile(ctx, "missing"); err == nil {
		t.Fatal("expected SelectTeamProfile to reject an unknown profile")
	}

	activeProfile, err := rt.ActiveTeamProfile(ctx)
	if err != nil {
		t.Fatalf("ActiveTeamProfile: %v", err)
	}
	if activeProfile != teams.DefaultProfileName {
		t.Fatalf("active profile = %q, want %q", activeProfile, teams.DefaultProfileName)
	}
}

func writeRuntimeStoredTeamProfile(t *testing.T, storageRoot, projectID, profile, name string) {
	t.Helper()

	teamDir := filepath.Join(storageRoot, "projects", projectID, "teams", profile)
	writeRuntimeTeamSpec(t, teamDir, name)
}

func writeRuntimeGlobalTeamProfile(t *testing.T, storageRoot, profile, name string) {
	t.Helper()

	teamDir := filepath.Join(storageRoot, "teams", profile)
	writeRuntimeTeamSpec(t, teamDir, name)
}

func writeRuntimeTeamSpec(t *testing.T, teamDir, name string) {
	t.Helper()

	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime team dir: %v", err)
	}
	teamSpec := "name: " + name + "\nfront_agent: assistant\nagents:\n  - id: assistant\n    soul_file: assistant.soul.yaml\n    base_profile: operator\n    tool_families: [repo_read, delegate]\n    delegation_kinds: [research]\n    can_message: []\n"
	if err := os.WriteFile(filepath.Join(teamDir, "team.yaml"), []byte(teamSpec), 0o644); err != nil {
		t.Fatalf("write runtime team yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamDir, "assistant.soul.yaml"), []byte("role: front assistant\n"), 0o644); err != nil {
		t.Fatalf("write runtime soul: %v", err)
	}
}
