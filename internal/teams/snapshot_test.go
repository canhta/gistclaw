package teams

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestLoadExecutionSnapshot_BuildsAgentProfilesFromAdaptiveTeamConfig(t *testing.T) {
	dir := t.TempDir()
	writeTeamFile(t, filepath.Join(dir, "team.yaml"), `
name: default
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    base_profile: operator
    tool_families: [repo_read, connector_capability, delegate]
    allow_tools: [connector_directory_list]
    deny_tools: [repo_write]
    delegation_kinds: [research, write]
    can_message: [patcher]
    specialist_summary_visibility: basic
  - id: patcher
    soul_file: patcher.soul.yaml
    base_profile: write
    tool_families: [repo_read, repo_write]
    can_message: [assistant]
    specialist_summary_visibility: none
`)
	writeTeamFile(t, filepath.Join(dir, "assistant.soul.yaml"), "role: front assistant\ndecision_policy:\n  - prefer direct capability execution\n")
	writeTeamFile(t, filepath.Join(dir, "patcher.soul.yaml"), "role: write specialist\n")

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
	if assistant.BaseProfile != model.BaseProfileOperator {
		t.Fatalf("expected assistant operator base profile, got %q", assistant.BaseProfile)
	}
	if len(assistant.ToolFamilies) != 3 {
		t.Fatalf("expected assistant tool families, got %#v", assistant.ToolFamilies)
	}
	if len(assistant.AllowTools) != 1 || assistant.AllowTools[0] != "connector_directory_list" {
		t.Fatalf("expected assistant allow_tools, got %#v", assistant.AllowTools)
	}
	if len(assistant.DenyTools) != 1 || assistant.DenyTools[0] != "repo_write" {
		t.Fatalf("expected assistant deny_tools, got %#v", assistant.DenyTools)
	}
	if len(assistant.DelegationKinds) != 2 {
		t.Fatalf("expected assistant delegation kinds, got %#v", assistant.DelegationKinds)
	}
	if assistant.SpecialistSummaryVisibility != model.SpecialistSummaryBasic {
		t.Fatalf("expected assistant basic specialist summary visibility, got %q", assistant.SpecialistSummaryVisibility)
	}
	if !strings.Contains(assistant.Instructions, "prefer direct capability execution") {
		t.Fatalf("expected assistant instructions to contain adaptive rule, got %q", assistant.Instructions)
	}
	if len(assistant.CanMessage) != 1 || assistant.CanMessage[0] != "patcher" {
		t.Fatalf("expected assistant can_message [patcher], got %#v", assistant.CanMessage)
	}

	patcher := snapshot.Agents["patcher"]
	if patcher.BaseProfile != model.BaseProfileWrite {
		t.Fatalf("expected patcher write base profile, got %q", patcher.BaseProfile)
	}
	if len(patcher.ToolFamilies) != 2 {
		t.Fatalf("expected patcher tool families, got %#v", patcher.ToolFamilies)
	}
	if patcher.SpecialistSummaryVisibility != model.SpecialistSummaryNone {
		t.Fatalf("expected patcher specialist summary visibility none, got %q", patcher.SpecialistSummaryVisibility)
	}
	if len(patcher.CanMessage) != 1 || patcher.CanMessage[0] != "assistant" {
		t.Fatalf("expected patcher can_message [assistant], got %#v", patcher.CanMessage)
	}
}

func TestLoadExecutionSnapshot_RejectsUnknownAdaptiveFields(t *testing.T) {
	dir := t.TempDir()
	writeTeamFile(t, filepath.Join(dir, "team.yaml"), `
name: default
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    base_profile: operator
    tool_families: [dangerous_mode]
    can_message: []
`)
	writeTeamFile(t, filepath.Join(dir, "assistant.soul.yaml"), "role: front assistant\n")

	if _, err := LoadExecutionSnapshot(dir); err == nil {
		t.Fatal("expected unknown tool family to fail")
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
