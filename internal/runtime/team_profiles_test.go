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
	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})
	ctx := context.Background()

	workspaceRoot := t.TempDir()
	writeRuntimeTeamProfile(t, workspaceRoot, "default", "Default Team")
	writeRuntimeTeamProfile(t, workspaceRoot, "review", "Review Team")

	project, err := ActivateWorkspace(ctx, db, workspaceRoot, "alpha", "operator")
	if err != nil {
		t.Fatalf("activate project: %v", err)
	}
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
	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})
	ctx := context.Background()

	workspaceRoot := t.TempDir()
	writeRuntimeTeamProfile(t, workspaceRoot, "default", "Default Team")
	writeRuntimeTeamProfile(t, workspaceRoot, "review", "Review Team")

	project, err := ActivateWorkspace(ctx, db, workspaceRoot, "alpha", "operator")
	if err != nil {
		t.Fatalf("activate project: %v", err)
	}
	if err := SetActiveProject(ctx, db, project.ID); err != nil {
		t.Fatalf("set active project: %v", err)
	}

	first, err := rt.prepareStartRun(ctx, "", StartRun{
		ConversationID: "conv-team-default",
		AgentID:        "assistant",
		Objective:      "use default team",
		WorkspaceRoot:  workspaceRoot,
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
		WorkspaceRoot:  workspaceRoot,
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
	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})
	ctx := context.Background()

	fallbackRoot := t.TempDir()
	writeRuntimeTeamProfile(t, fallbackRoot, "default", "Fallback Team")
	rt.SetTeamDir(filepath.Join(fallbackRoot, ".gistclaw", "teams", "default"))

	workspaceRoot := t.TempDir()
	project, err := ActivateWorkspace(ctx, db, workspaceRoot, "alpha", "operator")
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
		t.Fatalf("expected 2 workspace-managed profiles, got %d", len(profiles))
	}

	cloned, err := teams.LoadConfig(teams.ProfileDir(workspaceRoot, "ops"))
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
	if _, err := os.Stat(teams.ProfileDir(workspaceRoot, "ops")); !os.IsNotExist(err) {
		t.Fatalf("expected deleted profile dir to be removed, err=%v", err)
	}
}

func TestRuntime_DeleteTeamProfileRejectsActiveProfile(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})
	ctx := context.Background()

	workspaceRoot := t.TempDir()
	writeRuntimeTeamProfile(t, workspaceRoot, "default", "Default Team")

	project, err := ActivateWorkspace(ctx, db, workspaceRoot, "alpha", "operator")
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

func writeRuntimeTeamProfile(t *testing.T, workspaceRoot, profile, name string) {
	t.Helper()

	teamDir := filepath.Join(workspaceRoot, ".gistclaw", "teams", profile)
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime team dir: %v", err)
	}
	teamSpec := "name: " + name + "\nfront_agent: assistant\nagents:\n  - id: assistant\n    soul_file: assistant.soul.yaml\n    role: coordinator\n    tool_posture: read_heavy\n"
	if err := os.WriteFile(filepath.Join(teamDir, "team.yaml"), []byte(teamSpec), 0o644); err != nil {
		t.Fatalf("write runtime team yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamDir, "assistant.soul.yaml"), []byte("role: coordinator\ntool_posture: read_heavy\n"), 0o644); err != nil {
		t.Fatalf("write runtime soul: %v", err)
	}
}
