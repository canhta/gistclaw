package authority

import "github.com/canhta/gistclaw/internal/model"

func Decide(env Envelope, intent Intent) model.ToolDecision {
	if env.ApprovalMode == "" {
		env.ApprovalMode = ApprovalModePrompt
	}
	if env.HostAccessMode == "" {
		env.HostAccessMode = HostAccessModeStandard
	}

	for _, class := range intent.Sensitive {
		if IsSensitiveClass(class) && env.HostAccessMode != HostAccessModeElevated {
			return model.ToolDecision{
				Mode:   model.DecisionDeny,
				Reason: "sensitive access requires elevated host mode",
			}
		}
	}

	if intent.Mutating || intent.Network || len(intent.WriteRoots) > 0 {
		if env.ApprovalMode == ApprovalModeAutoApprove {
			return model.ToolDecision{
				Mode:   model.DecisionAllow,
				Reason: "auto approve mode enabled",
			}
		}
		return model.ToolDecision{
			Mode:   model.DecisionAsk,
			Reason: "mutating access requires approval",
		}
	}

	return model.ToolDecision{
		Mode:   model.DecisionAllow,
		Reason: "read-only access allowed",
	}
}
