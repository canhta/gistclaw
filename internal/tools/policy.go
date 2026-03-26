package tools

import "github.com/canhta/gistclaw/internal/model"

type ToolProfile string

type Policy struct {
	Profile   ToolProfile
	Overrides map[string]model.DecisionMode
}

func (p *Policy) Decide(agent model.AgentProfile, _ model.RunProfile, spec model.ToolSpec) model.ToolDecision {
	return p.decide(agent, spec, classifyToolSpec(spec))
}

func (p *Policy) DecideCall(agent model.AgentProfile, _ model.RunProfile, spec model.ToolSpec, inputJSON []byte) model.ToolDecision {
	return p.decide(agent, spec, classifyToolCall(spec, inputJSON))
}

func (p *Policy) decide(agent model.AgentProfile, spec model.ToolSpec, effect string) model.ToolDecision {
	if p.Overrides != nil {
		if mode, ok := p.Overrides[spec.Name]; ok {
			return model.ToolDecision{Mode: mode, Reason: "override"}
		}
	}

	switch spec.Name {
	case "session_spawn":
		if !hasCapability(agent.Capabilities, model.CapSpawn) {
			return model.ToolDecision{Mode: model.DecisionDeny, Reason: "spawn capability required"}
		}
	case "workspace_apply":
		if !hasCapability(agent.Capabilities, model.CapWorkspaceWrite) {
			return model.ToolDecision{Mode: model.DecisionDeny, Reason: "workspace_write capability required"}
		}
	}

	if isReadEffect(effect) {
		return model.ToolDecision{Mode: model.DecisionAllow, Reason: "low risk tool"}
	}

	profile := string(p.Profile)
	if profile == "" {
		profile = agent.ToolProfile
	}

	switch profile {
	case "read_only", "read_heavy", "propose_only":
		return model.ToolDecision{
			Mode:   model.DecisionDeny,
			Reason: "profile denies risky tools",
		}
	case "workspace_write":
		if (spec.Name == "shell_exec" || spec.Name == "coder_exec") && effect == effectExecWrite {
			return model.ToolDecision{
				Mode:   model.DecisionAsk,
				Reason: "workspace_write requires approval for mutating shell commands",
			}
		}
		return model.ToolDecision{
			Mode:   model.DecisionAllow,
			Reason: "workspace_write allows workspace mutations",
		}
	case "operator_facing", "elevated":
		if (spec.Name == "shell_exec" || spec.Name == "coder_exec") && effect == effectExecWrite {
			return model.ToolDecision{
				Mode:   model.DecisionAsk,
				Reason: "mutating shell commands require approval",
			}
		}
		return model.ToolDecision{Mode: model.DecisionAllow, Reason: "elevated profile"}
	default:
		return model.ToolDecision{Mode: model.DecisionAllow, Reason: "default allow"}
	}
}

func hasCapability(capabilities []model.AgentCapability, target model.AgentCapability) bool {
	for _, capability := range capabilities {
		if capability == target {
			return true
		}
	}
	return false
}
