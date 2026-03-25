package teams

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/canhta/gistclaw/internal/model"
	"go.yaml.in/yaml/v4"
)

type soulSpec struct {
	ToolPosture string `yaml:"tool_posture"`
}

func LoadExecutionSnapshot(teamDir string) (model.ExecutionSnapshot, error) {
	if teamDir == "" {
		return model.ExecutionSnapshot{}, nil
	}

	teamYAMLPath := filepath.Join(teamDir, "team.yaml")
	data, err := os.ReadFile(teamYAMLPath)
	if err != nil {
		return model.ExecutionSnapshot{}, fmt.Errorf("team: read team.yaml: %w", err)
	}
	spec, err := LoadSpec(data)
	if err != nil {
		return model.ExecutionSnapshot{}, err
	}

	snapshot := model.ExecutionSnapshot{
		TeamID: spec.Name,
		Agents: make(map[string]model.AgentProfile, len(spec.Agents)),
	}
	for _, agent := range spec.Agents {
		soulPath := filepath.Join(teamDir, agent.SoulFile)
		soulData, err := os.ReadFile(soulPath)
		if err != nil {
			return model.ExecutionSnapshot{}, fmt.Errorf("team: read soul %q: %w", agent.SoulFile, err)
		}
		profile, err := buildAgentProfile(agent, soulData)
		if err != nil {
			return model.ExecutionSnapshot{}, fmt.Errorf("team: agent %q: %w", agent.ID, err)
		}
		snapshot.Agents[agent.ID] = profile
	}

	return snapshot, nil
}

func buildAgentProfile(agent AgentSpec, soulData []byte) (model.AgentProfile, error) {
	var soul soulSpec
	if err := yaml.Unmarshal(soulData, &soul); err != nil {
		return model.AgentProfile{}, fmt.Errorf("parse soul yaml: %w", err)
	}
	if soul.ToolPosture == "" {
		return model.AgentProfile{}, fmt.Errorf("tool_posture is required")
	}

	capability, err := toolPostureCapability(soul.ToolPosture)
	if err != nil {
		return model.AgentProfile{}, err
	}

	capabilities := []model.AgentCapability{capability}
	if len(agent.CanSpawn) > 0 {
		capabilities = append(capabilities, model.CapSpawn)
	}

	return model.AgentProfile{
		AgentID:      agent.ID,
		Capabilities: capabilities,
		ToolProfile:  soul.ToolPosture,
	}, nil
}

func toolPostureCapability(toolPosture string) (model.AgentCapability, error) {
	switch toolPosture {
	case "workspace_write":
		return model.CapWorkspaceWrite, nil
	case "operator_facing":
		return model.CapOperatorFacing, nil
	case "read_heavy":
		return model.CapReadHeavy, nil
	case "propose_only":
		return model.CapProposeOnly, nil
	default:
		return "", fmt.Errorf("unknown tool_posture %q", toolPosture)
	}
}
