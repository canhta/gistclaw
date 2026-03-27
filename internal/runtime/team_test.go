package runtime

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/teams"
)

func TestRuntime_TeamConfigUsesActiveProjectStoredProfile(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})
	ctx := context.Background()
	storageRoot := t.TempDir()
	rt.SetStorageRoot(storageRoot)

	alphaRoot := t.TempDir()
	alphaProject, err := ActivateProjectPath(ctx, db, alphaRoot, "alpha", "operator")
	if err != nil {
		t.Fatalf("activate alpha project: %v", err)
	}
	writeRuntimeStoredTeamProfile(t, storageRoot, alphaProject.ID, teams.DefaultProfileName, "Alpha Team")
	if err := SetActiveProject(ctx, db, alphaProject.ID); err != nil {
		t.Fatalf("set active project: %v", err)
	}

	cfg, err := rt.TeamConfig(ctx)
	if err != nil {
		t.Fatalf("TeamConfig: %v", err)
	}
	if cfg.Name != "Alpha Team" {
		t.Fatalf("expected active project team name %q, got %q", "Alpha Team", cfg.Name)
	}
}

func TestRuntime_UpdateTeamWritesIntoActiveProjectStorage(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})
	ctx := context.Background()
	storageRoot := t.TempDir()
	rt.SetStorageRoot(storageRoot)

	workspaceRoot := t.TempDir()
	project, err := ActivateProjectPath(ctx, db, workspaceRoot, "alpha", "operator")
	if err != nil {
		t.Fatalf("activate project: %v", err)
	}
	writeRuntimeStoredTeamProfile(t, storageRoot, project.ID, teams.DefaultProfileName, "Alpha Team")
	if err := SetActiveProject(ctx, db, project.ID); err != nil {
		t.Fatalf("set active project: %v", err)
	}

	cfg, err := rt.TeamConfig(ctx)
	if err != nil {
		t.Fatalf("TeamConfig: %v", err)
	}
	cfg.Name = "Updated Team"
	if err := rt.UpdateTeam(ctx, cfg); err != nil {
		t.Fatalf("UpdateTeam: %v", err)
	}

	reloaded, err := rt.TeamConfig(ctx)
	if err != nil {
		t.Fatalf("TeamConfig reload: %v", err)
	}
	if reloaded.Name != "Updated Team" {
		t.Fatalf("expected updated team name, got %q", reloaded.Name)
	}

	cmd, err := rt.prepareStartRun(ctx, "", StartRun{
		ConversationID: "conv-team",
		AgentID:        "assistant",
		Objective:      "use updated team",
		CWD:            workspaceRoot,
	})
	if err != nil {
		t.Fatalf("prepareStartRun: %v", err)
	}
	snapshot, err := decodeExecutionSnapshot(cmd.ExecutionSnapshotJSON)
	if err != nil {
		t.Fatalf("decodeExecutionSnapshot: %v", err)
	}
	if snapshot.TeamID != "Updated Team" {
		t.Fatalf("expected refreshed snapshot team id %q, got %q", "Updated Team", snapshot.TeamID)
	}
}

func TestRuntime_TeamConfigFallsBackToConfiguredTeamDirWhenStoredProfileMissing(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})
	ctx := context.Background()
	storageRoot := t.TempDir()
	rt.SetStorageRoot(storageRoot)

	writeRuntimeGlobalTeamProfile(t, storageRoot, teams.DefaultProfileName, "Fallback Team")
	fallbackTeamDir := filepath.Join(storageRoot, "teams", "default")
	rt.SetTeamDir(fallbackTeamDir)

	workspaceRoot := t.TempDir()
	project, err := ActivateProjectPath(ctx, db, workspaceRoot, "workspace-without-team-copy", "operator")
	if err != nil {
		t.Fatalf("activate project: %v", err)
	}
	if err := SetActiveProject(ctx, db, project.ID); err != nil {
		t.Fatalf("set active project: %v", err)
	}

	cfg, err := rt.TeamConfig(ctx)
	if err != nil {
		t.Fatalf("TeamConfig: %v", err)
	}
	if cfg.Name != "Fallback Team" {
		t.Fatalf("expected fallback team name %q, got %q", "Fallback Team", cfg.Name)
	}
}
