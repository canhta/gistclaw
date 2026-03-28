package tools

import "github.com/canhta/gistclaw/internal/model"

type Policy struct {
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

	if containsString(agent.DenyTools, spec.Name) {
		return model.ToolDecision{Mode: model.DecisionDeny, Reason: "tool denied by agent policy"}
	}
	if spec.Family == "" {
		return model.ToolDecision{Mode: model.DecisionDeny, Reason: "tool family is required"}
	}
	if spec.RequiresExplicitAllow && !containsString(agent.AllowTools, spec.Name) {
		return model.ToolDecision{Mode: model.DecisionDeny, Reason: "raw session spawning requires explicit allow"}
	}
	if spec.Family == model.ToolFamilyDelegate && len(agent.DelegationKinds) == 0 {
		return model.ToolDecision{Mode: model.DecisionDeny, Reason: "delegation kinds required"}
	}

	if !allowsToolFamily(agent.BaseProfile, spec.Family) && !containsString(agent.AllowTools, spec.Name) {
		return model.ToolDecision{Mode: model.DecisionDeny, Reason: "tool family denied by base profile"}
	}

	if isReadEffect(effect) {
		return model.ToolDecision{Mode: model.DecisionAllow, Reason: "allowed read tool"}
	}
	if spec.Approval == "required" {
		return model.ToolDecision{Mode: model.DecisionAsk, Reason: "tool requires approval"}
	}
	if spec.Approval == "maybe" && isMutatingEffect(effect) {
		return model.ToolDecision{Mode: model.DecisionAsk, Reason: "mutating tool requires approval"}
	}
	return model.ToolDecision{Mode: model.DecisionAllow, Reason: "tool allowed by adaptive policy"}
}

func allowsToolFamily(profile model.BaseProfile, family model.ToolFamily) bool {
	switch profile {
	case model.BaseProfileOperator:
		return family == model.ToolFamilyRepoRead ||
			family == model.ToolFamilyRuntimeCapability ||
			family == model.ToolFamilyConnectorCapability ||
			family == model.ToolFamilyWebRead ||
			family == model.ToolFamilyDelegate
	case model.BaseProfileResearch:
		return family == model.ToolFamilyRepoRead || family == model.ToolFamilyWebRead
	case model.BaseProfileWrite:
		return family == model.ToolFamilyRepoRead || family == model.ToolFamilyRepoWrite
	case model.BaseProfileReview:
		return family == model.ToolFamilyRepoRead || family == model.ToolFamilyDiffReview
	case model.BaseProfileVerify:
		return family == model.ToolFamilyRepoRead || family == model.ToolFamilyVerification
	default:
		return false
	}
}

func isMutatingEffect(effect string) bool {
	return !isReadEffect(effect)
}
