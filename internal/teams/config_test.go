package teams

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_CombinesTeamAndSoulFields(t *testing.T) {
	dir := t.TempDir()
	writeTeamFile(t, filepath.Join(dir, "team.yaml"), `
name: Repo Task Team
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
	writeTeamFile(t, filepath.Join(dir, "assistant.soul.yaml"), "role: coordinator\ntool_posture: operator_facing\ntone: direct\n")
	writeTeamFile(t, filepath.Join(dir, "patcher.soul.yaml"), "role: patcher\ntool_posture: workspace_write\n")

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Name != "Repo Task Team" {
		t.Fatalf("expected team name, got %q", cfg.Name)
	}
	if cfg.FrontAgent != "assistant" {
		t.Fatalf("expected front agent assistant, got %q", cfg.FrontAgent)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(cfg.Agents))
	}
	if cfg.Agents[0].Role != "coordinator" {
		t.Fatalf("expected assistant role, got %q", cfg.Agents[0].Role)
	}
	if cfg.Agents[0].ToolPosture != "operator_facing" {
		t.Fatalf("expected assistant posture, got %q", cfg.Agents[0].ToolPosture)
	}
	if got := cfg.Agents[0].Soul.Extra["tone"]; got != "direct" {
		t.Fatalf("expected assistant tone to be preserved, got %#v", got)
	}
}

func TestWriteConfig_PreservesSoulFieldsWhileUpdatingEditableValues(t *testing.T) {
	dir := t.TempDir()
	writeTeamFile(t, filepath.Join(dir, "team.yaml"), `
name: Repo Task Team
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
	writeTeamFile(t, filepath.Join(dir, "assistant.soul.yaml"), "role: coordinator\ntool_posture: operator_facing\ntone: direct\n")
	writeTeamFile(t, filepath.Join(dir, "patcher.soul.yaml"), "role: patcher\ntool_posture: workspace_write\nnotes: edits files\n")

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.Name = "Platform Crew"
	cfg.FrontAgent = "patcher"
	cfg.Agents[0].Role = "front reviewer"
	cfg.Agents[0].ToolPosture = "read_heavy"
	cfg.Agents[0].CanSpawn = []string{"patcher"}
	cfg.Agents[0].CanMessage = []string{"patcher"}

	if err := WriteConfig(dir, cfg); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	reloaded, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if reloaded.Name != "Platform Crew" {
		t.Fatalf("expected updated team name, got %q", reloaded.Name)
	}
	if reloaded.FrontAgent != "patcher" {
		t.Fatalf("expected updated front agent, got %q", reloaded.FrontAgent)
	}
	if reloaded.Agents[0].Role != "front reviewer" {
		t.Fatalf("expected updated role, got %q", reloaded.Agents[0].Role)
	}
	if reloaded.Agents[0].ToolPosture != "read_heavy" {
		t.Fatalf("expected updated posture, got %q", reloaded.Agents[0].ToolPosture)
	}

	assistantSoul, err := os.ReadFile(filepath.Join(dir, "assistant.soul.yaml"))
	if err != nil {
		t.Fatalf("read assistant soul: %v", err)
	}
	if !strings.Contains(string(assistantSoul), "tone: direct") {
		t.Fatalf("expected assistant soul extras to be preserved, got:\n%s", assistantSoul)
	}
}
