package memory

import (
	"context"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

func setupMemoryDB(t *testing.T) (*store.DB, *conversations.ConversationStore) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cs := conversations.NewConversationStore(db)
	return db, cs
}

func TestMemory_WriteFactPersistsWithProvenance(t *testing.T) {
	db, cs := setupMemoryDB(t)
	s := NewStore(db, cs)
	ctx := context.Background()
	const projectID = "proj-alpha"

	err := s.WriteFact(ctx, model.MemoryItem{
		ProjectID:  projectID,
		ID:         "mem-1",
		AgentID:    "agent-a",
		Scope:      "local",
		Content:    "Go uses goroutines for concurrency",
		Source:     "model",
		Provenance: "run-123:turn-5",
		Confidence: 0.9,
		DedupeKey:  "go-concurrency",
	})
	if err != nil {
		t.Fatalf("WriteFact failed: %v", err)
	}

	items, err := s.Search(ctx, model.MemoryQuery{
		ProjectID: projectID,
		AgentID:   "agent-a",
		Scope:     "local",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Provenance != "run-123:turn-5" {
		t.Fatalf("expected provenance %q, got %q", "run-123:turn-5", items[0].Provenance)
	}
}

func TestMemory_WriteFactDoesNotTriggerPromotion(t *testing.T) {
	db, cs := setupMemoryDB(t)
	s := NewStore(db, cs)
	ctx := context.Background()
	const projectID = "proj-alpha"

	key := conversations.ConversationKey{
		ConnectorID: "cli", AccountID: "local", ExternalID: "conv-mem", ThreadID: "main",
	}
	conv, err := cs.Resolve(ctx, key)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	err = s.WriteFact(ctx, model.MemoryItem{
		ProjectID: projectID,
		ID:        "mem-2",
		AgentID:   "agent-a",
		Scope:     "local",
		Content:   "test fact",
		Source:    "model",
	})
	if err != nil {
		t.Fatalf("WriteFact failed: %v", err)
	}

	events, err := cs.ListEvents(ctx, conv.ID, 100)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	for _, evt := range events {
		if evt.Kind == "memory_promoted" {
			t.Fatal("WriteFact should NOT trigger promotion events")
		}
	}
}

func TestMemory_PromoteEmitsJournalEvent(t *testing.T) {
	db, cs := setupMemoryDB(t)
	s := NewStore(db, cs)
	ctx := context.Background()
	const projectID = "proj-alpha"

	key := conversations.ConversationKey{
		ConnectorID: "cli", AccountID: "local", ExternalID: "conv-promo", ThreadID: "main",
	}
	conv, err := cs.Resolve(ctx, key)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	err = s.PromoteCandidate(ctx, model.MemoryCandidate{
		ProjectID:      projectID,
		AgentID:        "agent-a",
		Scope:          "local",
		Content:        "promoted fact",
		Provenance:     "run-456:turn-2",
		Confidence:     0.95,
		DedupeKey:      "promo-1",
		ConversationID: conv.ID,
	})
	if err != nil {
		t.Fatalf("PromoteCandidate failed: %v", err)
	}

	events, err := cs.ListEvents(ctx, conv.ID, 100)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	foundPromo := false
	for _, evt := range events {
		if evt.Kind == "memory_promoted" {
			foundPromo = true
			break
		}
	}
	if !foundPromo {
		t.Fatal("expected memory_promoted event in journal")
	}
}

func TestMemory_PromoteCandidateUpsertsByProjectAndDedupeKey(t *testing.T) {
	db, cs := setupMemoryDB(t)
	s := NewStore(db, cs)
	ctx := context.Background()
	const projectID = "proj-alpha"

	candidate := model.MemoryCandidate{
		ProjectID:  projectID,
		AgentID:    "assistant",
		Scope:      "team",
		Content:    "the repo uses pnpm workspaces.",
		Provenance: "explicit_memory_request",
		Confidence: 0.95,
		DedupeKey:  "explicit_memory:pnpm",
	}

	if err := s.PromoteCandidate(ctx, candidate); err != nil {
		t.Fatalf("first PromoteCandidate failed: %v", err)
	}

	candidate.Content = "the repo uses pnpm workspaces and turbo tasks."
	if err := s.PromoteCandidate(ctx, candidate); err != nil {
		t.Fatalf("second PromoteCandidate failed: %v", err)
	}

	items, err := s.Search(ctx, model.MemoryQuery{
		ProjectID: projectID,
		AgentID:   "assistant",
		Scope:     "team",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 deduped memory item, got %d", len(items))
	}
	if got := items[0].Content; got != "the repo uses pnpm workspaces and turbo tasks." {
		t.Fatalf("expected latest promoted content, got %q", got)
	}
}

func TestMemory_PromoteCandidatePreservesHumanEditForSameDedupeKey(t *testing.T) {
	db, cs := setupMemoryDB(t)
	s := NewStore(db, cs)
	ctx := context.Background()
	const projectID = "proj-alpha"

	candidate := model.MemoryCandidate{
		ProjectID:  projectID,
		AgentID:    "assistant",
		Scope:      "team",
		Content:    "the repo uses pnpm workspaces.",
		Provenance: "explicit_memory_request",
		Confidence: 0.95,
		DedupeKey:  "explicit_memory:pnpm",
	}

	if err := s.PromoteCandidate(ctx, candidate); err != nil {
		t.Fatalf("first PromoteCandidate failed: %v", err)
	}

	items, err := s.Search(ctx, model.MemoryQuery{
		ProjectID: projectID,
		AgentID:   "assistant",
		Scope:     "team",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 memory item after first promotion, got %d", len(items))
	}

	if err := s.Edit(ctx, projectID, items[0].ID, "use pnpm workspaces and keep lockfile changes isolated."); err != nil {
		t.Fatalf("Edit failed: %v", err)
	}

	candidate.Content = "the repo uses pnpm workspaces and turbo tasks."
	if err := s.PromoteCandidate(ctx, candidate); err != nil {
		t.Fatalf("second PromoteCandidate failed: %v", err)
	}

	items, err = s.Search(ctx, model.MemoryQuery{
		ProjectID: projectID,
		AgentID:   "assistant",
		Scope:     "team",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 memory item after human edit, got %d", len(items))
	}
	if got := items[0].Source; got != "human" {
		t.Fatalf("expected human source to remain authoritative, got %q", got)
	}
	if got := items[0].Content; got != "use pnpm workspaces and keep lockfile changes isolated." {
		t.Fatalf("expected human-edited content to remain, got %q", got)
	}
}

func TestMemory_HumanEditOutranksModelFact(t *testing.T) {
	db, cs := setupMemoryDB(t)
	s := NewStore(db, cs)
	ctx := context.Background()
	const projectID = "proj-alpha"

	err := s.WriteFact(ctx, model.MemoryItem{
		ProjectID: projectID,
		ID:        "mem-rank",
		AgentID:   "agent-a",
		Scope:     "local",
		Content:   "model says X",
		Source:    "model",
	})
	if err != nil {
		t.Fatalf("WriteFact failed: %v", err)
	}

	err = s.UpdateFact(ctx, model.MemoryItem{
		ProjectID: projectID,
		ID:        "mem-rank",
		AgentID:   "agent-a",
		Scope:     "local",
		Content:   "human says Y",
		Source:    "human",
	})
	if err != nil {
		t.Fatalf("UpdateFact failed: %v", err)
	}

	items, err := s.Search(ctx, model.MemoryQuery{ProjectID: projectID, AgentID: "agent-a", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Source != "human" {
		t.Fatalf("expected source %q, got %q", "human", items[0].Source)
	}
	if items[0].Content != "human says Y" {
		t.Fatalf("expected %q, got %q", "human says Y", items[0].Content)
	}
}

func TestMemory_ModelCannotOverwriteHumanFact(t *testing.T) {
	db, cs := setupMemoryDB(t)
	s := NewStore(db, cs)
	ctx := context.Background()
	const projectID = "proj-alpha"

	err := s.WriteFact(ctx, model.MemoryItem{
		ProjectID: projectID,
		ID:        "mem-human",
		AgentID:   "agent-a",
		Scope:     "local",
		Content:   "human truth",
		Source:    "human",
	})
	if err != nil {
		t.Fatalf("WriteFact failed: %v", err)
	}

	err = s.UpdateFact(ctx, model.MemoryItem{
		ProjectID: projectID,
		ID:        "mem-human",
		AgentID:   "agent-a",
		Scope:     "local",
		Content:   "model override",
		Source:    "model",
	})
	if err != nil {
		t.Fatalf("UpdateFact returned error: %v", err)
	}

	items, err := s.Search(ctx, model.MemoryQuery{ProjectID: projectID, AgentID: "agent-a", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Source != "human" {
		t.Fatalf("expected source still %q, got %q", "human", items[0].Source)
	}
	if items[0].Content != "human truth" {
		t.Fatalf("expected content still %q, got %q", "human truth", items[0].Content)
	}
}

func TestMemory_UpsertWorkingSummaryUsesRunID(t *testing.T) {
	db, cs := setupMemoryDB(t)
	s := NewStore(db, cs)
	ctx := context.Background()
	const projectID = "proj-alpha"

	ref, err := s.UpsertWorkingSummary(ctx, "run-summary", "conv-summary", projectID)
	if err != nil {
		t.Fatalf("UpsertWorkingSummary failed: %v", err)
	}
	if ref.RunID != "run-summary" {
		t.Fatalf("expected summary RunID %q, got %q", "run-summary", ref.RunID)
	}

	var count int
	err = db.RawDB().QueryRow(
		"SELECT count(*) FROM run_summaries WHERE run_id = 'run-summary' AND project_id = ?",
		projectID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count run summaries: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 run summary for run-summary, got %d", count)
	}
}

func TestMemory_LoadContextUsesRunScopedSummaryAndScopedFacts(t *testing.T) {
	db, cs := setupMemoryDB(t)
	s := NewStore(db, cs)
	ctx := context.Background()
	const projectID = "proj-alpha"

	for _, item := range []model.MemoryItem{
		{ProjectID: projectID, ID: "mem-a", AgentID: "agent-a", Scope: "local", Content: "keep me", Source: "model"},
		{ProjectID: projectID, ID: "mem-b", AgentID: "agent-b", Scope: "local", Content: "ignore me", Source: "model"},
		{ProjectID: projectID, ID: "mem-team", AgentID: "agent-a", Scope: "team", Content: "other scope", Source: "model"},
		{ProjectID: "proj-beta", ID: "mem-other-project", AgentID: "agent-a", Scope: "local", Content: "other project", Source: "model"},
	} {
		if err := s.WriteFact(ctx, item); err != nil {
			t.Fatalf("WriteFact %s failed: %v", item.ID, err)
		}
	}

	if _, err := s.UpsertWorkingSummary(ctx, "run-context", "conv-context", projectID); err != nil {
		t.Fatalf("UpsertWorkingSummary failed: %v", err)
	}

	contextView, err := s.LoadContext(ctx, "run-context", projectID, "agent-a", "local", 10)
	if err != nil {
		t.Fatalf("LoadContext failed: %v", err)
	}
	if contextView.Summary.RunID != "run-context" {
		t.Fatalf("expected run-scoped summary, got %q", contextView.Summary.RunID)
	}
	if len(contextView.Items) != 1 {
		t.Fatalf("expected 1 scoped memory item, got %d", len(contextView.Items))
	}
	if contextView.Items[0].ID != "mem-a" {
		t.Fatalf("expected mem-a, got %q", contextView.Items[0].ID)
	}
}

func TestMemory_SearchEmptyStoreReturnsEmpty(t *testing.T) {
	db, cs := setupMemoryDB(t)
	s := NewStore(db, cs)
	ctx := context.Background()

	items, err := s.Search(ctx, model.MemoryQuery{ProjectID: "proj-alpha", AgentID: "nobody", Limit: 10})
	if err != nil {
		t.Fatalf("Search on empty store should not error, got: %v", err)
	}
	if items == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestMemory_SearchScopesFactsToProject(t *testing.T) {
	db, cs := setupMemoryDB(t)
	s := NewStore(db, cs)
	ctx := context.Background()

	for _, item := range []model.MemoryItem{
		{ProjectID: "proj-alpha", ID: "mem-alpha", AgentID: "agent-a", Scope: "local", Content: "alpha fact", Source: "model"},
		{ProjectID: "proj-beta", ID: "mem-beta", AgentID: "agent-a", Scope: "local", Content: "beta fact", Source: "model"},
	} {
		if err := s.WriteFact(ctx, item); err != nil {
			t.Fatalf("WriteFact %s failed: %v", item.ID, err)
		}
	}

	items, err := s.Search(ctx, model.MemoryQuery{
		ProjectID: "proj-alpha",
		AgentID:   "agent-a",
		Scope:     "local",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 project-scoped fact, got %d", len(items))
	}
	if items[0].ID != "mem-alpha" {
		t.Fatalf("expected mem-alpha, got %q", items[0].ID)
	}
}
