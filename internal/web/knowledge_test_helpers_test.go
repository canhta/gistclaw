package web

import (
	"context"
	"testing"

	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
)

func memoryFilterArg(projectID, agentID, scope string) memory.MemoryFilter {
	return memory.MemoryFilter{ProjectID: projectID, AgentID: agentID, Scope: scope}
}

func seedMemoryFact(t *testing.T, h *serverHarness, agentID, scope, content, source string) string {
	return seedMemoryFactInProject(t, h, h.activeProjectID, agentID, scope, content, source)
}

func seedMemoryFactInProject(t *testing.T, h *serverHarness, projectID, agentID, scope, content, source string) string {
	t.Helper()
	item := model.MemoryItem{
		ProjectID: projectID,
		AgentID:   agentID,
		Scope:     scope,
		Content:   content,
		Source:    source,
	}
	if err := h.rt.Memory().WriteFact(context.Background(), item); err != nil {
		t.Fatalf("seedMemoryFact: %v", err)
	}

	facts, err := h.rt.Memory().Filter(context.Background(), memoryFilterArg(projectID, agentID, ""))
	if err != nil {
		t.Fatalf("seedMemoryFact filter: %v", err)
	}
	for _, fact := range facts {
		if fact.Content == content {
			return fact.ID
		}
	}
	t.Fatalf("could not find seeded fact with content %q", content)
	return ""
}
