package web

import (
	"strings"

	"github.com/canhta/gistclaw/internal/model"
)

func normalizeAgentLinks(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(values))
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		items = append(items, value)
	}
	return items
}

func normalizeToolFamilies(values []string) []model.ToolFamily {
	seen := make(map[string]bool, len(values))
	items := make([]model.ToolFamily, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		items = append(items, model.ToolFamily(value))
	}
	return items
}

func normalizeDelegationKinds(values []string) []model.DelegationKind {
	seen := make(map[string]bool, len(values))
	items := make([]model.DelegationKind, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		items = append(items, model.DelegationKind(value))
	}
	return items
}
