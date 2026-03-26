package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
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
