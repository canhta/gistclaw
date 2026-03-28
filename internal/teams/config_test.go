package teams

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestLoadConfig_CombinesTeamAndSoulFields(t *testing.T) {
	dir := t.TempDir()
	writeTeamFile(t, filepath.Join(dir, "team.yaml"), `
name: Repo Task Team
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    base_profile: operator
    tool_families: [repo_read, connector_capability, delegate]
    allow_tools: [connector_directory_list]
    deny_tools: [repo_write]
    delegation_kinds: [research, write]
    specialties: [routing, messaging]
    can_message: [patcher]
    specialist_summary_visibility: basic
  - id: patcher
    soul_file: patcher.soul.yaml
    base_profile: write
    tool_families: [repo_read, repo_write]
    can_message: [assistant]
`)
	writeTeamFile(t, filepath.Join(dir, "assistant.soul.yaml"), "role: front assistant\ntone: direct\n")
	writeTeamFile(t, filepath.Join(dir, "patcher.soul.yaml"), "role: patcher\n")

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

	assistant := cfg.Agents[0]
	if assistant.Role != "front assistant" {
		t.Fatalf("expected assistant role, got %q", assistant.Role)
	}
	if assistant.BaseProfile != model.BaseProfileOperator {
		t.Fatalf("expected operator base profile, got %q", assistant.BaseProfile)
	}
	if len(assistant.ToolFamilies) != 3 {
		t.Fatalf("expected 3 tool families, got %#v", assistant.ToolFamilies)
	}
	if len(assistant.AllowTools) != 1 || assistant.AllowTools[0] != "connector_directory_list" {
		t.Fatalf("expected allow_tools to round-trip, got %#v", assistant.AllowTools)
	}
	if len(assistant.DenyTools) != 1 || assistant.DenyTools[0] != "repo_write" {
		t.Fatalf("expected deny_tools to round-trip, got %#v", assistant.DenyTools)
	}
	if len(assistant.DelegationKinds) != 2 {
		t.Fatalf("expected delegation kinds to round-trip, got %#v", assistant.DelegationKinds)
	}
	if len(assistant.Specialties) != 2 || assistant.Specialties[0] != "routing" || assistant.Specialties[1] != "messaging" {
		t.Fatalf("expected specialties to round-trip, got %#v", assistant.Specialties)
	}
	if assistant.SpecialistSummaryVisibility != model.SpecialistSummaryBasic {
		t.Fatalf("expected basic specialist summary visibility, got %q", assistant.SpecialistSummaryVisibility)
	}
	if got := assistant.Soul.Extra["tone"]; got != "direct" {
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
    base_profile: operator
    tool_families: [repo_read, delegate]
    allow_tools: [connector_directory_list]
    deny_tools: [repo_write]
    delegation_kinds: [research]
    can_message: [patcher]
    specialist_summary_visibility: basic
  - id: patcher
    soul_file: patcher.soul.yaml
    base_profile: write
    tool_families: [repo_read, repo_write]
    can_message: [assistant]
`)
	writeTeamFile(t, filepath.Join(dir, "assistant.soul.yaml"), "role: front assistant\ntone: direct\n")
	writeTeamFile(t, filepath.Join(dir, "patcher.soul.yaml"), "role: patcher\nnotes: edits files\n")

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.Name = "Platform Crew"
	cfg.FrontAgent = "patcher"
	cfg.Agents[0].Role = "adaptive assistant"
	cfg.Agents[0].BaseProfile = model.BaseProfileResearch
	cfg.Agents[0].ToolFamilies = []model.ToolFamily{
		model.ToolFamilyRepoRead,
		model.ToolFamilyWebRead,
	}
	cfg.Agents[0].AllowTools = []string{"web_search"}
	cfg.Agents[0].DenyTools = []string{"repo_write"}
	cfg.Agents[0].DelegationKinds = []model.DelegationKind{model.DelegationKindReview}
	cfg.Agents[0].CanMessage = []string{"patcher"}
	cfg.Agents[0].SpecialistSummaryVisibility = model.SpecialistSummaryFull

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
	if reloaded.Agents[0].Role != "adaptive assistant" {
		t.Fatalf("expected updated role, got %q", reloaded.Agents[0].Role)
	}
	if reloaded.Agents[0].BaseProfile != model.BaseProfileResearch {
		t.Fatalf("expected updated base profile, got %q", reloaded.Agents[0].BaseProfile)
	}
	if len(reloaded.Agents[0].ToolFamilies) != 2 {
		t.Fatalf("expected updated tool families, got %#v", reloaded.Agents[0].ToolFamilies)
	}
	if reloaded.Agents[0].SpecialistSummaryVisibility != model.SpecialistSummaryFull {
		t.Fatalf("expected updated specialist summary visibility, got %q", reloaded.Agents[0].SpecialistSummaryVisibility)
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
    base_profile: operator
    tool_families: [repo_read, delegate]
    allow_tools: [connector_directory_list]
    can_message: [patcher]
  - id: patcher
    soul_file: patcher.soul.yaml
    base_profile: write
    tool_families: [repo_read, repo_write]
    can_message: [assistant]
`)
	writeTeamFile(t, filepath.Join(dir, "assistant.soul.yaml"), "role: front assistant\ntone: direct\n")
	writeTeamFile(t, filepath.Join(dir, "patcher.soul.yaml"), "role: patcher\n")

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.FrontAgent = "assistant"
	cfg.Agents = []AgentConfig{
		{
			ID:                          cfg.Agents[0].ID,
			SoulFile:                    cfg.Agents[0].SoulFile,
			Role:                        cfg.Agents[0].Role,
			BaseProfile:                 cfg.Agents[0].BaseProfile,
			ToolFamilies:                cfg.Agents[0].ToolFamilies,
			AllowTools:                  append([]string(nil), cfg.Agents[0].AllowTools...),
			DenyTools:                   append([]string(nil), cfg.Agents[0].DenyTools...),
			DelegationKinds:             []model.DelegationKind{model.DelegationKindResearch},
			CanMessage:                  []string{"researcher"},
			SpecialistSummaryVisibility: model.SpecialistSummaryBasic,
			Soul:                        cfg.Agents[0].Soul,
		},
		{
			ID:          "researcher",
			SoulFile:    "researcher.soul.yaml",
			Role:        "research specialist",
			BaseProfile: model.BaseProfileResearch,
			ToolFamilies: []model.ToolFamily{
				model.ToolFamilyRepoRead,
				model.ToolFamilyWebRead,
			},
			CanMessage:                  []string{"assistant"},
			SpecialistSummaryVisibility: model.SpecialistSummaryBasic,
			Soul: SoulSpec{
				Role: "research specialist",
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
    base_profile: operator
    tool_families: [repo_read, connector_capability, delegate]
    allow_tools: [connector_directory_list]
    deny_tools: [repo_write]
    delegation_kinds: [research, write]
    specialties: [routing, messaging]
    can_message: [patcher]
    specialist_summary_visibility: basic
  - id: patcher
    soul_file: patcher.soul.yaml
    base_profile: write
    tool_families: [repo_read, repo_write]
    can_message: [assistant]
`)
	writeTeamFile(t, filepath.Join(dir, "assistant.soul.yaml"), "role: front assistant\ntone: direct\n")
	writeTeamFile(t, filepath.Join(dir, "patcher.soul.yaml"), "role: patcher\nnotes: edits files\n")

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
		"role: front assistant",
		"base_profile: operator",
		"tool_families:",
		"delegation_kinds:",
		"specialties:",
		"routing",
		"messaging",
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
	if reloaded.Agents[0].Role != "front assistant" || reloaded.Agents[0].BaseProfile != model.BaseProfileOperator {
		t.Fatalf("expected editable fields to round-trip, got %+v", reloaded.Agents[0])
	}
	if len(reloaded.Agents[0].Specialties) != 2 || reloaded.Agents[0].Specialties[0] != "routing" {
		t.Fatalf("expected specialties to round-trip, got %#v", reloaded.Agents[0].Specialties)
	}
	if got := reloaded.Agents[0].Soul.Extra["tone"]; got != "direct" {
		t.Fatalf("expected soul extra tone to round-trip, got %#v", got)
	}
}

func TestLoadEditableYAML_RejectsRemovedTeamFields(t *testing.T) {
	raw := []byte(`
name: Repo Task Team
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    role: front assistant
    base_profile: operator
    tool_families: [repo_read]
    tool_posture: operator_facing
    can_spawn: [patcher]
	`)

	if _, err := LoadEditableYAML(raw); err == nil {
		t.Fatal("expected removed team fields to be rejected")
	}
}
