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

	err := s.WriteFact(ctx, model.MemoryItem{
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
		AgentID: "agent-a",
		Scope:   "local",
		Limit:   10,
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

	key := conversations.ConversationKey{
		ConnectorID: "cli", AccountID: "local", ExternalID: "conv-mem", ThreadID: "main",
	}
	conv, err := cs.Resolve(ctx, key)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	err = s.WriteFact(ctx, model.MemoryItem{
		ID:      "mem-2",
		AgentID: "agent-a",
		Scope:   "local",
		Content: "test fact",
		Source:  "model",
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

	key := conversations.ConversationKey{
		ConnectorID: "cli", AccountID: "local", ExternalID: "conv-promo", ThreadID: "main",
	}
	conv, err := cs.Resolve(ctx, key)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	err = s.PromoteCandidate(ctx, model.MemoryCandidate{
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

func TestMemory_HumanEditOutranksModelFact(t *testing.T) {
	db, cs := setupMemoryDB(t)
	s := NewStore(db, cs)
	ctx := context.Background()

	err := s.WriteFact(ctx, model.MemoryItem{
		ID:      "mem-rank",
		AgentID: "agent-a",
		Scope:   "local",
		Content: "model says X",
		Source:  "model",
	})
	if err != nil {
		t.Fatalf("WriteFact failed: %v", err)
	}

	err = s.UpdateFact(ctx, model.MemoryItem{
		ID:      "mem-rank",
		AgentID: "agent-a",
		Scope:   "local",
		Content: "human says Y",
		Source:  "human",
	})
	if err != nil {
		t.Fatalf("UpdateFact failed: %v", err)
	}

	items, err := s.Search(ctx, model.MemoryQuery{AgentID: "agent-a", Limit: 10})
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

	err := s.WriteFact(ctx, model.MemoryItem{
		ID:      "mem-human",
		AgentID: "agent-a",
		Scope:   "local",
		Content: "human truth",
		Source:  "human",
	})
	if err != nil {
		t.Fatalf("WriteFact failed: %v", err)
	}

	err = s.UpdateFact(ctx, model.MemoryItem{
		ID:      "mem-human",
		AgentID: "agent-a",
		Scope:   "local",
		Content: "model override",
		Source:  "model",
	})
	if err != nil {
		t.Fatalf("UpdateFact returned error: %v", err)
	}

	items, err := s.Search(ctx, model.MemoryQuery{AgentID: "agent-a", Limit: 10})
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

func TestMemory_SearchEmptyStoreReturnsEmpty(t *testing.T) {
	db, cs := setupMemoryDB(t)
	s := NewStore(db, cs)
	ctx := context.Background()

	items, err := s.Search(ctx, model.MemoryQuery{AgentID: "nobody", Limit: 10})
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
