package runtime

import (
	"fmt"

	"go.yaml.in/yaml/v4"
)

// TeamSpec is the in-memory representation of a validated team.yaml file.
type TeamSpec struct {
	Name             string              `yaml:"name"`
	Agents           []AgentSpec         `yaml:"agents"`
	CapabilityFlags  map[string][]string `yaml:"capability_flags"`
	HandoffEdges     []HandoffEdge       `yaml:"handoff_edges"`
}

// AgentSpec declares one agent in the team.
type AgentSpec struct {
	ID       string `yaml:"id"`
	SoulFile string `yaml:"soul_file"`
}

// HandoffEdge declares a permitted delegation path between two agents.
type HandoffEdge struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// LoadTeamSpec parses and validates a YAML-encoded team specification.
// It returns a descriptive error naming any missing required field, and
// an error naming any agent ID referenced in handoff_edges that is not
// declared in the agents list.
func LoadTeamSpec(data []byte) (*TeamSpec, error) {
	var spec TeamSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("team: parse yaml: %w", err)
	}

	// Validate required fields after unmarshal by checking struct zero values.
	// A missing YAML key leaves the field at its zero value (nil slice/map).
	// We distinguish "present but empty" from "absent" by checking whether
	// the key existed at all in the raw document via a targeted nil check.
	if spec.Agents == nil {
		return nil, fmt.Errorf("team: required field %q is missing", "agents")
	}
	if spec.CapabilityFlags == nil {
		return nil, fmt.Errorf("team: required field %q is missing", "capability_flags")
	}
	if spec.HandoffEdges == nil {
		return nil, fmt.Errorf("team: required field %q is missing", "handoff_edges")
	}

	// Build a set of declared agent IDs for edge validation.
	agentIDs := make(map[string]bool, len(spec.Agents))
	for _, a := range spec.Agents {
		agentIDs[a.ID] = true
	}

	for _, edge := range spec.HandoffEdges {
		if !agentIDs[edge.From] {
			return nil, fmt.Errorf("team: handoff_edge references undeclared agent %q (from)", edge.From)
		}
		if !agentIDs[edge.To] {
			return nil, fmt.Errorf("team: handoff_edge references undeclared agent %q (to)", edge.To)
		}
	}

	return &spec, nil
}
