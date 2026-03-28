package web

import (
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/teams"
)

func TestAddTeamMemberDefaultsToResearchProfile(t *testing.T) {
	cfg := teams.Config{
		Name:       "default",
		FrontAgent: "assistant",
		Agents: []teams.AgentConfig{
			{ID: "assistant"},
			{ID: "patcher"},
			{ID: "reviewer"},
		},
	}

	updated := addTeamMember(cfg)
	if len(updated.Agents) != 4 {
		t.Fatalf("expected 4 agents after add, got %d", len(updated.Agents))
	}
	added := updated.Agents[3]
	if added.ID != "agent_1" {
		t.Fatalf("expected new agent id %q, got %q", "agent_1", added.ID)
	}
	if added.BaseProfile != model.BaseProfileResearch {
		t.Fatalf("expected new agent base profile %q, got %q", model.BaseProfileResearch, added.BaseProfile)
	}
	if len(added.ToolFamilies) != 2 || added.ToolFamilies[0] != model.ToolFamilyRepoRead || added.ToolFamilies[1] != model.ToolFamilyWebRead {
		t.Fatalf("expected research tool families [repo_read web_read], got %#v", added.ToolFamilies)
	}
	if added.Role != "research specialist" {
		t.Fatalf("expected role %q, got %q", "research specialist", added.Role)
	}
	if added.Soul.Role != "research specialist" {
		t.Fatalf("expected soul role %q, got %q", "research specialist", added.Soul.Role)
	}
}

func TestBuildBaseProfileOptionsOmitsGenericSpecialist(t *testing.T) {
	options := buildBaseProfileOptions(model.BaseProfileResearch)
	for _, option := range options {
		if option.Value == "specialist" {
			t.Fatalf("expected generic specialist option to be removed, got %#v", options)
		}
	}
}
