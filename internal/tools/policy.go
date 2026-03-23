package tools

import "github.com/canhta/gistclaw/internal/model"

type ToolProfile string

type Policy struct {
	Profile   ToolProfile
	Overrides map[string]model.DecisionMode
}

func (p *Policy) Decide(agent model.AgentProfile, _ model.RunProfile, spec model.ToolSpec) model.ToolDecision {
	if p.Overrides != nil {
		if mode, ok := p.Overrides[spec.Name]; ok {
			return model.ToolDecision{Mode: mode, Reason: "override"}
		}
	}

	if spec.Risk == model.RiskLow {
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
		return model.ToolDecision{
			Mode:   model.DecisionAsk,
			Reason: "workspace_write requires approval for risky tools",
		}
	case "operator_facing", "elevated":
		if spec.Risk == model.RiskHigh {
			return model.ToolDecision{
				Mode:   model.DecisionAsk,
				Reason: "high risk requires approval",
			}
		}
		return model.ToolDecision{Mode: model.DecisionAllow, Reason: "elevated profile"}
	default:
		return model.ToolDecision{Mode: model.DecisionAllow, Reason: "default allow"}
	}
}
