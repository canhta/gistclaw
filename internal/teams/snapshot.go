package teams

import (
	"fmt"
	"sort"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
)

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

func buildAgentProfile(agent AgentConfig) (model.AgentProfile, error) {
	if agent.BaseProfile == "" {
		return model.AgentProfile{}, fmt.Errorf("base_profile is required")
	}

	return model.AgentProfile{
		AgentID:                     agent.ID,
		Role:                        agent.Role,
		Instructions:                renderSoulInstructions(agent.Soul.Extra),
		BaseProfile:                 agent.BaseProfile,
		ToolFamilies:                append([]model.ToolFamily(nil), agent.ToolFamilies...),
		AllowTools:                  append([]string(nil), agent.AllowTools...),
		DenyTools:                   append([]string(nil), agent.DenyTools...),
		DelegationKinds:             append([]model.DelegationKind(nil), agent.DelegationKinds...),
		SpecialistSummaryVisibility: agent.SpecialistSummaryVisibility,
		CanMessage:                  append([]string(nil), agent.CanMessage...),
	}, nil
}

func renderSoulInstructions(extra map[string]any) string {
	if len(extra) == 0 {
		return ""
	}

	keys := make([]string, 0, len(extra))
	for key := range extra {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, key := range keys {
		if i > 0 {
			b.WriteString("\n")
		}
		writeSoulField(&b, key, extra[key], 0)
	}
	return strings.TrimSpace(b.String())
}

func writeSoulField(b *strings.Builder, key string, value any, indent int) {
	prefix := strings.Repeat("  ", indent)
	switch typed := value.(type) {
	case string:
		line := strings.TrimSpace(typed)
		if line == "" {
			b.WriteString(prefix + key + ":\n")
			return
		}
		b.WriteString(prefix + key + ": " + line + "\n")
	case []any:
		b.WriteString(prefix + key + ":\n")
		for _, item := range typed {
			writeSoulListItem(b, item, indent+1)
		}
	case []string:
		b.WriteString(prefix + key + ":\n")
		for _, item := range typed {
			writeSoulListItem(b, item, indent+1)
		}
	case map[string]any:
		b.WriteString(prefix + key + ":\n")
		writeSoulMap(b, typed, indent+1)
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			b.WriteString(prefix + key + ":\n")
			return
		}
		b.WriteString(prefix + key + ": " + text + "\n")
	}
}

func writeSoulMap(b *strings.Builder, values map[string]any, indent int) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		writeSoulField(b, key, values[key], indent)
	}
}

func writeSoulListItem(b *strings.Builder, value any, indent int) {
	prefix := strings.Repeat("  ", indent)
	switch typed := value.(type) {
	case string:
		line := strings.TrimSpace(typed)
		b.WriteString(prefix + "- " + line + "\n")
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			b.WriteString(prefix + "-\n")
			return
		}
		firstKey := keys[0]
		switch firstValue := typed[firstKey].(type) {
		case string:
			b.WriteString(prefix + "- " + firstKey + ": " + strings.TrimSpace(firstValue) + "\n")
		default:
			b.WriteString(prefix + "- " + firstKey + ":\n")
			writeSoulField(b, firstKey, firstValue, indent+1)
		}
		for _, key := range keys[1:] {
			writeSoulField(b, key, typed[key], indent+1)
		}
	case []any:
		b.WriteString(prefix + "-\n")
		for _, item := range typed {
			writeSoulListItem(b, item, indent+1)
		}
	default:
		b.WriteString(prefix + "- " + strings.TrimSpace(fmt.Sprint(value)) + "\n")
	}
}
