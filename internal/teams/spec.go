package teams

import (
	"fmt"

	"github.com/canhta/gistclaw/internal/model"
	"go.yaml.in/yaml/v4"
)

// Spec is the in-memory representation of a validated team.yaml file.
type Spec struct {
	Name       string      `yaml:"name"`
	FrontAgent string      `yaml:"front_agent"`
	Agents     []AgentSpec `yaml:"agents"`
}

// AgentSpec declares one agent in the team along with its adaptive execution
// policy and the peers it may message directly.
type AgentSpec struct {
	ID                          string                            `yaml:"id"`
	SoulFile                    string                            `yaml:"soul_file"`
	BaseProfile                 model.BaseProfile                 `yaml:"base_profile"`
	ToolFamilies                []model.ToolFamily                `yaml:"tool_families"`
	AllowTools                  []string                          `yaml:"allow_tools,omitempty"`
	DenyTools                   []string                          `yaml:"deny_tools,omitempty"`
	DelegationKinds             []model.DelegationKind            `yaml:"delegation_kinds,omitempty"`
	CanMessage                  []string                          `yaml:"can_message"`
	SpecialistSummaryVisibility model.SpecialistSummaryVisibility `yaml:"specialist_summary_visibility,omitempty"`
	LegacyToolPosture           string                            `yaml:"tool_posture,omitempty"`
	LegacyCanSpawn              []string                          `yaml:"can_spawn,omitempty"`
}

// LoadSpec parses and validates a YAML-encoded team specification.
func LoadSpec(data []byte) (*Spec, error) {
	var spec Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("team: parse yaml: %w", err)
	}
	if err := validateSpec(&spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

func validateSpec(spec *Spec) error {
	if spec.FrontAgent == "" {
		return fmt.Errorf("team: required field %q is missing", "front_agent")
	}
	if spec.Agents == nil {
		return fmt.Errorf("team: required field %q is missing", "agents")
	}

	agentIDs := make(map[string]bool, len(spec.Agents))
	for i, agent := range spec.Agents {
		if agent.ID == "" {
			return fmt.Errorf("team: agent at index %d is missing %q", i, "id")
		}
		if agent.SoulFile == "" {
			return fmt.Errorf("team: agent %q is missing %q", agent.ID, "soul_file")
		}
		if agent.LegacyToolPosture != "" {
			return fmt.Errorf("team: agent %q uses legacy field %q", agent.ID, "tool_posture")
		}
		if len(agent.LegacyCanSpawn) > 0 {
			return fmt.Errorf("team: agent %q uses legacy field %q", agent.ID, "can_spawn")
		}
		if agent.BaseProfile == "" {
			return fmt.Errorf("team: agent %q is missing %q", agent.ID, "base_profile")
		}
		if !model.IsValidBaseProfile(string(agent.BaseProfile)) {
			return fmt.Errorf("team: agent %q has invalid base_profile %q", agent.ID, agent.BaseProfile)
		}
		if len(agent.ToolFamilies) == 0 {
			return fmt.Errorf("team: agent %q is missing %q", agent.ID, "tool_families")
		}
		for _, family := range agent.ToolFamilies {
			if !model.IsValidToolFamily(string(family)) {
				return fmt.Errorf("team: agent %q has invalid tool_families entry %q", agent.ID, family)
			}
		}
		for _, kind := range agent.DelegationKinds {
			if !model.IsValidDelegationKind(string(kind)) {
				return fmt.Errorf("team: agent %q has invalid delegation_kinds entry %q", agent.ID, kind)
			}
		}
		if agent.SpecialistSummaryVisibility != "" &&
			!model.IsValidSpecialistSummaryVisibility(string(agent.SpecialistSummaryVisibility)) {
			return fmt.Errorf(
				"team: agent %q has invalid specialist_summary_visibility %q",
				agent.ID,
				agent.SpecialistSummaryVisibility,
			)
		}
		if agentIDs[agent.ID] {
			return fmt.Errorf("team: duplicate agent %q", agent.ID)
		}
		agentIDs[agent.ID] = true
	}

	if !agentIDs[spec.FrontAgent] {
		return fmt.Errorf("team: front_agent %q is not declared in agents", spec.FrontAgent)
	}

	for _, agent := range spec.Agents {
		for _, target := range agent.CanMessage {
			if !agentIDs[target] {
				return fmt.Errorf("team: agent %q can_message references undeclared agent %q", agent.ID, target)
			}
		}
	}

	return nil
}
