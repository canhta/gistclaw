package recommendation

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/canhta/gistclaw/internal/model"
)

type Mode string

const (
	ModeDirect      Mode = "direct"
	ModeDelegate    Mode = "delegate"
	ModeParallelize Mode = "parallelize"
)

type Decision struct {
	Mode            Mode
	Rationale       string
	Confidence      float64
	SuggestedKinds  []model.DelegationKind
	RankedToolNames []string
}

type Input struct {
	Objective    string
	Agent        model.AgentProfile
	Specialists  map[string]model.AgentProfile
	VisibleTools []model.ToolSpec
}

type ObjectiveDescriptor struct {
	Text     string
	Intents  []model.ToolIntent
	Keywords []string
}

type Engine struct{}

func AnalyzeObjective(text string) ObjectiveDescriptor {
	normalized := normalize(text)
	return ObjectiveDescriptor{
		Text:     normalized,
		Intents:  detectObjectiveIntents(normalized),
		Keywords: objectiveKeywords(normalized),
	}
}

func (Engine) Recommend(input Input) Decision {
	objective := AnalyzeObjective(input.Objective)
	if objective.Text == "" {
		return Decision{
			Mode:       ModeDirect,
			Rationale:  "no specialist trigger detected; execute directly",
			Confidence: 0.35,
		}
	}

	directIntents := directObjectiveIntents(objective.Intents)
	delegationKinds := suggestedDelegationKinds(objective.Intents, input.Agent, input.Specialists)
	rankedTools := rankToolsForObjective(directIntents, input.VisibleTools)

	if len(directIntents) > 0 && len(rankedTools) > 0 && len(delegationKinds) == 0 {
		return Decision{
			Mode:            ModeDirect,
			Rationale:       "bounded connector or local task with matching capabilities; execute directly",
			Confidence:      0.88,
			RankedToolNames: rankedTools,
		}
	}
	if shouldParallelize(delegationKinds) {
		return Decision{
			Mode:           ModeParallelize,
			Rationale:      fmt.Sprintf("independent %s work detected; parallel specialists are available", joinKinds(delegationKinds)),
			Confidence:     0.82,
			SuggestedKinds: delegationKinds,
		}
	}
	if len(delegationKinds) > 0 {
		return Decision{
			Mode:            ModeDelegate,
			Rationale:       fmt.Sprintf("%s-heavy objective with a matching specialist available", delegationKinds[0]),
			Confidence:      0.78,
			SuggestedKinds:  delegationKinds[:1],
			RankedToolNames: rankedTools,
		}
	}
	if len(rankedTools) > 0 {
		return Decision{
			Mode:            ModeDirect,
			Rationale:       "local capabilities can complete the bounded task directly",
			Confidence:      0.72,
			RankedToolNames: rankedTools,
		}
	}
	return Decision{
		Mode:       ModeDirect,
		Rationale:  "no clear specialist advantage detected; execute directly",
		Confidence: 0.55,
	}
}

func suggestedDelegationKinds(
	intents []model.ToolIntent,
	agent model.AgentProfile,
	specialists map[string]model.AgentProfile,
) []model.DelegationKind {
	candidates := make([]model.DelegationKind, 0, 4)
	add := func(kind model.DelegationKind) {
		if containsDelegationKind(candidates, kind) {
			return
		}
		if !containsDelegationKind(agent.DelegationKinds, kind) {
			return
		}
		if !hasSpecialistForKind(specialists, kind) {
			return
		}
		candidates = append(candidates, kind)
	}
	for _, intent := range intents {
		switch intent {
		case model.ToolIntentResearchRead:
			add(model.DelegationKindResearch)
		case model.ToolIntentWrite:
			add(model.DelegationKindWrite)
		case model.ToolIntentReview:
			add(model.DelegationKindReview)
		case model.ToolIntentVerify:
			add(model.DelegationKindVerify)
		}
	}
	return candidates
}

func directObjectiveIntents(intents []model.ToolIntent) []model.ToolIntent {
	direct := make([]model.ToolIntent, 0, len(intents))
	for _, intent := range intents {
		switch intent {
		case model.ToolIntentInboxList,
			model.ToolIntentInboxUpdate,
			model.ToolIntentDirectoryList,
			model.ToolIntentTargetResolve,
			model.ToolIntentMessageSend,
			model.ToolIntentStatusRead:
			if !containsIntent(direct, intent) {
				direct = append(direct, intent)
			}
		}
	}
	return direct
}

func rankToolsForObjective(intents []model.ToolIntent, visibleTools []model.ToolSpec) []string {
	if len(intents) == 0 || len(visibleTools) == 0 {
		return nil
	}

	ranked := make([]string, 0, len(visibleTools))
	seen := make(map[string]bool, len(visibleTools))
	for _, intent := range intents {
		matches := matchingToolsForIntent(intent, visibleTools)
		sort.Slice(matches, func(i, j int) bool {
			if matches[i].score != matches[j].score {
				return matches[i].score > matches[j].score
			}
			return matches[i].name < matches[j].name
		})
		for _, match := range matches {
			if seen[match.name] {
				continue
			}
			seen[match.name] = true
			ranked = append(ranked, match.name)
		}
	}
	return ranked
}

type toolMatch struct {
	name  string
	score int
}

func matchingToolsForIntent(intent model.ToolIntent, visibleTools []model.ToolSpec) []toolMatch {
	matches := make([]toolMatch, 0, len(visibleTools))
	for _, spec := range visibleTools {
		if !containsIntent(spec.Intents, intent) {
			continue
		}
		score := 1
		if len(spec.Intents) == 1 {
			score = 2
		}
		matches = append(matches, toolMatch{name: spec.Name, score: score})
	}
	return matches
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

func containsDelegationKind(values []model.DelegationKind, want model.DelegationKind) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsIntent(values []model.ToolIntent, want model.ToolIntent) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func detectObjectiveIntents(text string) []model.ToolIntent {
	if text == "" {
		return nil
	}
	intents := make([]model.ToolIntent, 0, 6)
	add := func(intent model.ToolIntent) {
		if !containsIntent(intents, intent) {
			intents = append(intents, intent)
		}
	}

	if containsAny(text, "research", "search", "look up", "investigate", "find sources", "summarize findings", "latest") {
		add(model.ToolIntentResearchRead)
	}
	if containsAny(text, "fix", "implement", "edit", "change", "modify", "refactor", "patch", "write code") {
		add(model.ToolIntentWrite)
	}
	if containsAny(text, "review", "audit", "inspect the diff", "code review") {
		add(model.ToolIntentReview)
	}
	if containsAny(text, "verify", "test", "validate", "confirm", "check that") {
		add(model.ToolIntentVerify)
	}
	if containsAny(text, "mark read", "mark unread", "pin", "unpin", "archive", "unarchive", "hide", "unhide", "đánh dấu", "đã đọc", "ghim", "bỏ ghim", "lưu trữ", "bỏ lưu trữ", "ẩn", "bỏ ẩn") {
		add(model.ToolIntentInboxUpdate)
	}
	if containsAny(text, "unread", "inbox", "conversation", "thread", "chat", "chưa đọc", "hộp thư", "cuộc trò chuyện", "tin nhắn gần đây") {
		add(model.ToolIntentInboxList)
	}
	if containsAny(text, "list", "show", "contacts", "groups", "directory") {
		add(model.ToolIntentDirectoryList)
	}
	if containsAny(text, "resolve", "find ", "lookup", "person", "someone", "target", "name") {
		add(model.ToolIntentTargetResolve)
	}
	if containsAny(text, "send", "message", "transfer", "route", "forward", "relay", "handoff") {
		add(model.ToolIntentMessageSend)
	}
	if containsAny(text, "status", "health", "connected") {
		add(model.ToolIntentStatusRead)
	}
	return intents
}

func objectiveKeywords(text string) []string {
	if text == "" {
		return nil
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	keywords := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.ToLower(part))
		if len(part) < 3 || stopword(part) || seen[part] {
			continue
		}
		seen[part] = true
		keywords = append(keywords, part)
	}
	return keywords
}

func stopword(value string) bool {
	switch value {
	case "the", "and", "for", "with", "from", "that", "this", "your", "our", "into", "about", "latest":
		return true
	default:
		return false
	}
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
