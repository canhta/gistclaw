package teams

import (
	"fmt"

	"github.com/canhta/gistclaw/internal/model"
)

type soulSpec struct {
	ToolPosture string `yaml:"tool_posture"`
}

func LoadExecutionSnapshot(teamDir string) (model.ExecutionSnapshot, error) {
	if teamDir == "" {
		return model.ExecutionSnapshot{}, nil
	}
	cfg, err := LoadConfig(teamDir)
	if err != nil {
		return model.ExecutionSnapshot{}, err
	}
	return cfg.Snapshot()
}

func buildAgentProfile(agent AgentSpec, soul soulSpec) (model.AgentProfile, error) {
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
		CanSpawn:     append([]string(nil), agent.CanSpawn...),
		CanMessage:   append([]string(nil), agent.CanMessage...),
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
