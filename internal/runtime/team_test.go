package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestRuntime_TeamConfigUsesActiveProjectWorkspace(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})
	ctx := context.Background()

	alphaRoot := t.TempDir()
	writeRuntimeTeamDir(t, alphaRoot, "Alpha Team")
	alphaProject, err := ActivateWorkspace(ctx, db, alphaRoot, "alpha", "operator")
	if err != nil {
		t.Fatalf("activate alpha project: %v", err)
	}
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

func TestRuntime_UpdateTeamWritesIntoActiveProjectWorkspace(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})
	ctx := context.Background()

	workspaceRoot := t.TempDir()
	writeRuntimeTeamDir(t, workspaceRoot, "Alpha Team")
	project, err := ActivateWorkspace(ctx, db, workspaceRoot, "alpha", "operator")
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
		WorkspaceRoot:  workspaceRoot,
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

func TestRuntime_TeamConfigFallsBackToConfiguredTeamDirWhenWorkspaceCopyMissing(t *testing.T) {
	db, cs, mem, reg := setupRunTestDeps(t)
	rt := New(db, cs, reg, mem, NewMockProvider(nil, nil), &model.NoopEventSink{})
	ctx := context.Background()

	fallbackRoot := t.TempDir()
	writeRuntimeTeamDir(t, fallbackRoot, "Fallback Team")
	fallbackTeamDir := filepath.Join(fallbackRoot, ".gistclaw", "teams", "default")
	rt.SetTeamDir(fallbackTeamDir)

	workspaceRoot := t.TempDir()
	project, err := ActivateWorkspace(ctx, db, workspaceRoot, "workspace-without-team-copy", "operator")
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

func writeRuntimeTeamDir(t *testing.T, workspaceRoot, name string) {
	t.Helper()

	teamDir := filepath.Join(workspaceRoot, ".gistclaw", "teams", "default")
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
