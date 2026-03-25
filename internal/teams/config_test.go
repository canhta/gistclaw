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

func TestWriteConfig_CreatesNewSoulFilesAndRemovesDeletedOnes(t *testing.T) {
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
	cfg.FrontAgent = "assistant"
	cfg.Agents = []AgentConfig{
		{
			ID:          cfg.Agents[0].ID,
			SoulFile:    cfg.Agents[0].SoulFile,
			Role:        cfg.Agents[0].Role,
			ToolPosture: cfg.Agents[0].ToolPosture,
			CanSpawn:    []string{"researcher"},
			CanMessage:  []string{"researcher"},
			Soul:        cfg.Agents[0].Soul,
		},
		{
			ID:          "researcher",
			SoulFile:    "researcher.soul.yaml",
			Role:        "researcher",
			ToolPosture: "read_heavy",
			CanSpawn:    nil,
			CanMessage:  []string{"assistant"},
			Soul: SoulSpec{
				Role:        "researcher",
				ToolPosture: "read_heavy",
				Extra: map[string]any{
					"notes": "finds references",
				},
			},
		},
	}

	if err := WriteConfig(dir, cfg); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "researcher.soul.yaml")); err != nil {
		t.Fatalf("expected new soul file to be created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "patcher.soul.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected deleted agent soul file to be removed, err=%v", err)
	}
}

func TestExportAndImportEditableYAML_RoundTripsConfig(t *testing.T) {
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

	raw, err := ExportEditableYAML(cfg)
	if err != nil {
		t.Fatalf("ExportEditableYAML: %v", err)
	}
	for _, want := range []string{
		"name: Repo Task Team",
		"role: coordinator",
		"tool_posture: operator_facing",
		"soul_file: assistant.soul.yaml",
		"soul_extra:",
	} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("expected export to contain %q:\n%s", want, raw)
		}
	}

	reloaded, err := LoadEditableYAML(raw)
	if err != nil {
		t.Fatalf("LoadEditableYAML: %v", err)
	}
	if reloaded.Name != cfg.Name || reloaded.FrontAgent != cfg.FrontAgent {
		t.Fatalf("expected round-trip metadata, got %+v", reloaded)
	}
	if len(reloaded.Agents) != len(cfg.Agents) {
		t.Fatalf("expected %d agents, got %d", len(cfg.Agents), len(reloaded.Agents))
	}
	if reloaded.Agents[0].Role != "coordinator" || reloaded.Agents[0].ToolPosture != "operator_facing" {
		t.Fatalf("expected editable fields to round-trip, got %+v", reloaded.Agents[0])
	}
	if got := reloaded.Agents[0].Soul.Extra["tone"]; got != "direct" {
		t.Fatalf("expected soul extra tone to round-trip, got %#v", got)
	}
}
