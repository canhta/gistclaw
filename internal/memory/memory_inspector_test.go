package memory

import (
	"context"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

func setupInspectorStore(t *testing.T) *Store {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cs := conversations.NewConversationStore(db)
	return NewStore(db, cs)
}

func writeFact(t *testing.T, s *Store, agentID, scope, content, source string) string {
	t.Helper()
	item := model.MemoryItem{
		AgentID: agentID,
		Scope:   scope,
		Content: content,
		Source:  source,
	}
	if err := s.WriteFact(context.Background(), item); err != nil {
		t.Fatalf("WriteFact: %v", err)
	}
	// Find the id just written by content.
	items, err := s.Filter(context.Background(), MemoryFilter{AgentID: agentID})
	if err != nil {
		t.Fatalf("Filter after write: %v", err)
	}
	for _, it := range items {
		if it.Content == content {
			return it.ID
		}
	}
	t.Fatalf("could not find written fact with content %q", content)
	return ""
}

func TestInspector_ForgetExcludesFromList(t *testing.T) {
	s := setupInspectorStore(t)
	ctx := context.Background()

	id := writeFact(t, s, "coordinator", "local", "remember this", "model")

	if err := s.Forget(ctx, id); err != nil {
		t.Fatalf("Forget: %v", err)
	}

	items, err := s.Filter(ctx, MemoryFilter{})
	if err != nil {
		t.Fatalf("Filter after forget: %v", err)
	}
	for _, it := range items {
		if it.ID == id {
			t.Fatal("forgotten fact must not appear in Filter results")
		}
	}
}

func TestInspector_EditUpdatesValue(t *testing.T) {
	s := setupInspectorStore(t)
	ctx := context.Background()

	id := writeFact(t, s, "coordinator", "local", "original content", "model")

	if err := s.Edit(ctx, id, "updated content"); err != nil {
		t.Fatalf("Edit: %v", err)
	}

	items, err := s.Filter(ctx, MemoryFilter{AgentID: "coordinator"})
	if err != nil {
		t.Fatalf("Filter after edit: %v", err)
	}
	var found bool
	for _, it := range items {
		if it.ID == id {
			found = true
			if it.Content != "updated content" {
				t.Errorf("expected updated content, got %q", it.Content)
			}
			if it.Source != "human" {
				t.Errorf("expected source=human after edit, got %q", it.Source)
			}
		}
	}
	if !found {
		t.Fatal("edited fact not found in Filter results")
	}
}

func TestInspector_HumanEditOutranksModel(t *testing.T) {
	s := setupInspectorStore(t)
	ctx := context.Background()

	id := writeFact(t, s, "coordinator", "local", "initial", "model")

	// Human edit applied after model write.
	if err := s.Edit(ctx, id, "human value"); err != nil {
		t.Fatalf("Edit: %v", err)
	}

	// UpdateFact with model source must not overwrite the human edit.
	if err := s.UpdateFact(ctx, model.MemoryItem{
		ID:      id,
		AgentID: "coordinator",
		Scope:   "local",
		Content: "model tries to overwrite",
		Source:  "model",
	}); err != nil {
		t.Fatalf("UpdateFact: %v", err)
	}

	items, err := s.Filter(ctx, MemoryFilter{AgentID: "coordinator"})
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	for _, it := range items {
		if it.ID == id {
			if it.Content != "human value" {
				t.Errorf("human value should outrank model: got %q", it.Content)
			}
			return
		}
	}
	t.Fatal("fact not found")
}

func TestInspector_FilterByScope_Local(t *testing.T) {
	s := setupInspectorStore(t)
	ctx := context.Background()

	writeFact(t, s, "agent1", "local", "local fact", "model")
	writeFact(t, s, "agent1", "team", "team fact", "model")

	items, err := s.Filter(ctx, MemoryFilter{Scope: "local"})
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	for _, it := range items {
		if it.Scope != "local" {
			t.Errorf("expected only local facts, got scope=%q", it.Scope)
		}
	}
	if len(items) == 0 {
		t.Fatal("expected at least one local fact")
	}
}

func TestInspector_FilterByScope_Team(t *testing.T) {
	s := setupInspectorStore(t)
	ctx := context.Background()

	writeFact(t, s, "agent1", "local", "local fact", "model")
	writeFact(t, s, "agent1", "team", "team fact", "model")

	items, err := s.Filter(ctx, MemoryFilter{Scope: "team"})
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	for _, it := range items {
		if it.Scope != "team" {
			t.Errorf("expected only team facts, got scope=%q", it.Scope)
		}
	}
	if len(items) == 0 {
		t.Fatal("expected at least one team fact")
	}
}

func TestInspector_FilterByAgentID(t *testing.T) {
	s := setupInspectorStore(t)
	ctx := context.Background()

	writeFact(t, s, "agent-a", "local", "fact from a", "model")
	writeFact(t, s, "agent-b", "local", "fact from b", "model")

	items, err := s.Filter(ctx, MemoryFilter{AgentID: "agent-a"})
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	for _, it := range items {
		if it.AgentID != "agent-a" {
			t.Errorf("expected only agent-a facts, got agent_id=%q", it.AgentID)
		}
	}
	if len(items) == 0 {
		t.Fatal("expected at least one fact from agent-a")
	}
}

func TestInspector_WrittenFactPreservesScope(t *testing.T) {
	s := setupInspectorStore(t)
	ctx := context.Background()

	// Write with scope="team" — store must not reclassify it.
	if err := s.WriteFact(ctx, model.MemoryItem{
		AgentID: "coordinator",
		Scope:   "team",
		Content: "scoped fact",
		Source:  "model",
	}); err != nil {
		t.Fatalf("WriteFact: %v", err)
	}

	items, err := s.Filter(ctx, MemoryFilter{Scope: "team"})
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	var found bool
	for _, it := range items {
		if it.Content == "scoped fact" {
			found = true
			if it.Scope != "team" {
				t.Errorf("scope was reclassified: expected team, got %q", it.Scope)
			}
		}
	}
	if !found {
		t.Fatal("team-scoped fact not found in team filter")
	}
}
