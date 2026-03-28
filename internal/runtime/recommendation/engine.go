package recommendation

import (
	"fmt"
	"slices"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
)

type Mode string

const (
	ModeDirect      Mode = "direct"
	ModeDelegate    Mode = "delegate"
	ModeParallelize Mode = "parallelize"
)

type Decision struct {
	Mode           Mode
	Rationale      string
	Confidence     float64
	SuggestedKinds []model.DelegationKind
}

type Input struct {
	Objective    string
	Agent        model.AgentProfile
	Specialists  map[string]model.AgentProfile
	VisibleTools []model.ToolSpec
}

type Engine struct{}

func (Engine) Recommend(input Input) Decision {
	objective := normalize(input.Objective)
	if objective == "" {
		return Decision{
			Mode:       ModeDirect,
			Rationale:  "no specialist trigger detected; execute directly",
			Confidence: 0.35,
		}
	}

	toolFamilies := effectiveToolFamilies(input.Agent, input.VisibleTools)
	hasConnectorCapability := slices.Contains(toolFamilies, model.ToolFamilyConnectorCapability)
	hasRuntimeCapability := slices.Contains(toolFamilies, model.ToolFamilyRuntimeCapability)

	researchSignal := containsAny(objective, "research", "latest", "look up", "search", "investigate", "find sources", "summarize findings", "docs")
	writeSignal := containsAny(objective, "fix", "implement", "edit", "change", "modify", "refactor", "patch", "write code")
	reviewSignal := containsAny(objective, "review", "audit", "inspect the diff", "code review")
	verifySignal := containsAny(objective, "verify", "test", "validate", "confirm", "check that")
	boundedAction := containsAny(objective, "list", "show", "status", "lookup", "resolve", "send", "message", "contacts", "transfer", "route", "handoff")
	connectorAction := containsAny(objective, "telegram", "whatsapp", "zalo", "email", "contact", "contacts", "message")

	kinds := make([]model.DelegationKind, 0, 4)
	if researchSignal && canDelegate(input.Agent, input.Specialists, model.DelegationKindResearch) {
		kinds = append(kinds, model.DelegationKindResearch)
	}
	if writeSignal && canDelegate(input.Agent, input.Specialists, model.DelegationKindWrite) {
		kinds = append(kinds, model.DelegationKindWrite)
	}
	if reviewSignal && canDelegate(input.Agent, input.Specialists, model.DelegationKindReview) {
		kinds = append(kinds, model.DelegationKindReview)
	}
	if verifySignal && canDelegate(input.Agent, input.Specialists, model.DelegationKindVerify) {
		kinds = append(kinds, model.DelegationKindVerify)
	}

	if boundedAction && connectorAction && (hasConnectorCapability || hasRuntimeCapability) {
		return Decision{
			Mode:       ModeDirect,
			Rationale:  "bounded connector action with no specialist advantage; execute directly",
			Confidence: 0.9,
		}
	}
	if boundedAction && !researchSignal && !writeSignal && !reviewSignal && !verifySignal {
		return Decision{
			Mode:       ModeDirect,
			Rationale:  "bounded local action with no specialist advantage; execute directly",
			Confidence: 0.8,
		}
	}
	if shouldParallelize(kinds) {
		return Decision{
			Mode:           ModeParallelize,
			Rationale:      fmt.Sprintf("independent %s work detected; parallel specialists are available", joinKinds(kinds)),
			Confidence:     0.82,
			SuggestedKinds: kinds,
		}
	}
	if len(kinds) > 0 {
		return Decision{
			Mode:           ModeDelegate,
			Rationale:      fmt.Sprintf("%s-heavy objective with a matching specialist available", kinds[0]),
			Confidence:     0.78,
			SuggestedKinds: kinds[:1],
		}
	}
	return Decision{
		Mode:       ModeDirect,
		Rationale:  "no clear specialist advantage detected; execute directly",
		Confidence: 0.55,
	}
}

func effectiveToolFamilies(agent model.AgentProfile, tools []model.ToolSpec) []model.ToolFamily {
	if len(tools) == 0 {
		return append([]model.ToolFamily(nil), agent.ToolFamilies...)
	}
	seen := make(map[model.ToolFamily]bool, len(tools))
	families := make([]model.ToolFamily, 0, len(tools))
	for _, spec := range tools {
		if spec.Family == "" || seen[spec.Family] {
			continue
		}
		seen[spec.Family] = true
		families = append(families, spec.Family)
	}
	if len(families) == 0 {
		return append([]model.ToolFamily(nil), agent.ToolFamilies...)
	}
	return families
}

func canDelegate(agent model.AgentProfile, specialists map[string]model.AgentProfile, kind model.DelegationKind) bool {
	return slices.Contains(agent.DelegationKinds, kind) && hasSpecialistForKind(specialists, kind)
}

func hasSpecialistForKind(specialists map[string]model.AgentProfile, kind model.DelegationKind) bool {
	targetProfile := baseProfileForKind(kind)
	for _, specialist := range specialists {
		if specialist.BaseProfile == targetProfile {
			return true
		}
	}
	return false
}

func baseProfileForKind(kind model.DelegationKind) model.BaseProfile {
	switch kind {
	case model.DelegationKindResearch:
		return model.BaseProfileResearch
	case model.DelegationKindWrite:
		return model.BaseProfileWrite
	case model.DelegationKindReview:
		return model.BaseProfileReview
	case model.DelegationKindVerify:
		return model.BaseProfileVerify
	default:
		return ""
	}
}

func shouldParallelize(kinds []model.DelegationKind) bool {
	if len(kinds) < 2 {
		return false
	}
	for _, kind := range kinds {
		if kind == model.DelegationKindWrite {
			return false
		}
	}
	return true
}

func joinKinds(kinds []model.DelegationKind) string {
	items := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		items = append(items, string(kind))
	}
	return strings.Join(items, " and ")
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func normalize(text string) string {
	return strings.ToLower(strings.TrimSpace(text))
}
