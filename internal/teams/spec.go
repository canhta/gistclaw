package teams

import (
	"fmt"

	"go.yaml.in/yaml/v4"
)

// Spec is the in-memory representation of a validated team.yaml file.
type Spec struct {
	Name       string      `yaml:"name"`
	FrontAgent string      `yaml:"front_agent"`
	Agents     []AgentSpec `yaml:"agents"`
}

// AgentSpec declares one agent in the team along with the sessions it may
// spawn and the peers it may message directly.
type AgentSpec struct {
	ID         string   `yaml:"id"`
	SoulFile   string   `yaml:"soul_file"`
	CanSpawn   []string `yaml:"can_spawn"`
	CanMessage []string `yaml:"can_message"`
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
		if agentIDs[agent.ID] {
			return fmt.Errorf("team: duplicate agent %q", agent.ID)
		}
		agentIDs[agent.ID] = true
	}

	if !agentIDs[spec.FrontAgent] {
		return fmt.Errorf("team: front_agent %q is not declared in agents", spec.FrontAgent)
	}

	for _, agent := range spec.Agents {
		for _, target := range agent.CanSpawn {
			if !agentIDs[target] {
				return fmt.Errorf("team: agent %q can_spawn references undeclared agent %q", agent.ID, target)
			}
		}
		for _, target := range agent.CanMessage {
			if !agentIDs[target] {
				return fmt.Errorf("team: agent %q can_message references undeclared agent %q", agent.ID, target)
			}
		}
	}

	return nil
}
