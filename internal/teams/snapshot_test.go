package teams

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestLoadExecutionSnapshot_BuildsAgentProfilesFromTeamAndSoulFiles(t *testing.T) {
	dir := t.TempDir()
	writeTeamFile(t, filepath.Join(dir, "team.yaml"), `
name: default
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    can_spawn: [patcher]
    can_message: [patcher]
  - id: patcher
    soul_file: patcher.soul.yaml
    can_spawn: []
    can_message: [assistant]
`)
	writeTeamFile(t, filepath.Join(dir, "assistant.soul.yaml"), "tool_posture: operator_facing\n")
	writeTeamFile(t, filepath.Join(dir, "patcher.soul.yaml"), "tool_posture: workspace_write\n")

	snapshot, err := LoadExecutionSnapshot(dir)
	if err != nil {
		t.Fatalf("LoadExecutionSnapshot: %v", err)
	}
	if snapshot.TeamID != "default" {
		t.Fatalf("expected team id default, got %q", snapshot.TeamID)
	}
	if len(snapshot.Agents) != 2 {
		t.Fatalf("expected 2 agent profiles, got %d", len(snapshot.Agents))
	}

	assistant, ok := snapshot.Agents["assistant"]
	if !ok {
		t.Fatal("expected assistant profile")
	}
	if assistant.ToolProfile != "operator_facing" {
		t.Fatalf("expected assistant operator_facing, got %q", assistant.ToolProfile)
	}
	if !hasCapability(assistant.Capabilities, model.CapOperatorFacing) {
		t.Fatalf("expected assistant operator_facing capability, got %+v", assistant.Capabilities)
	}
	if !hasCapability(assistant.Capabilities, model.CapSpawn) {
		t.Fatalf("expected assistant spawn capability, got %+v", assistant.Capabilities)
	}

	patcher := snapshot.Agents["patcher"]
	if patcher.ToolProfile != "workspace_write" {
		t.Fatalf("expected patcher workspace_write, got %q", patcher.ToolProfile)
	}
	if !hasCapability(patcher.Capabilities, model.CapWorkspaceWrite) {
		t.Fatalf("expected patcher workspace_write capability, got %+v", patcher.Capabilities)
	}
}

func TestLoadExecutionSnapshot_RejectsUnknownToolPosture(t *testing.T) {
	dir := t.TempDir()
	writeTeamFile(t, filepath.Join(dir, "team.yaml"), `
name: default
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    can_spawn: []
    can_message: []
`)
	writeTeamFile(t, filepath.Join(dir, "assistant.soul.yaml"), "tool_posture: dangerous_mode\n")

	if _, err := LoadExecutionSnapshot(dir); err == nil {
		t.Fatal("expected unknown tool posture to fail")
	}
}

func writeTeamFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func hasCapability(capabilities []model.AgentCapability, want model.AgentCapability) bool {
	for _, capability := range capabilities {
		if capability == want {
			return true
		}
	}
	return false
}
