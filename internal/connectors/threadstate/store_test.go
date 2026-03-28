package threadstate

import (
	"context"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/store"
)

func setupDB(t *testing.T) *store.DB {
	t.Helper()

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestStore_UpsertAndList(t *testing.T) {
	t.Parallel()

	db := setupDB(t)
	s := New(db)
	ctx := context.Background()
	newer := time.Unix(1_700_000_100, 0).UTC()
	older := time.Unix(1_700_000_000, 0).UTC()

	for _, summary := range []Summary{
		{
			ConnectorID:        "zalo_personal",
			AccountID:          "acc-1",
			ThreadID:           "user-2",
			ThreadType:         "contact",
			Title:              "Bao",
			LastMessagePreview: "older",
			LastMessageAt:      older,
		},
		{
			ConnectorID:        "zalo_personal",
			AccountID:          "acc-1",
			ThreadID:           "user-1",
			ThreadType:         "contact",
			Title:              "An",
			Subtitle:           "user-1",
			LastMessagePreview: "newer",
			LastMessageAt:      newer,
			Metadata:           map[string]string{"avatar": "https://example.com/an.png"},
		},
	} {
		if err := s.Upsert(ctx, summary); err != nil {
			t.Fatalf("Upsert(%s): %v", summary.ThreadID, err)
		}
	}

	got, err := s.List(ctx, Filter{
		ConnectorID: "zalo_personal",
		AccountID:   "acc-1",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 summaries, got %+v", got)
	}
	if got[0].ThreadID != "user-1" || got[0].LastMessagePreview != "newer" {
		t.Fatalf("expected newest thread first, got %+v", got)
	}
	if got[0].Metadata["avatar"] != "https://example.com/an.png" {
		t.Fatalf("expected metadata to round-trip, got %+v", got[0].Metadata)
	}
}

func TestStore_UpsertPreservesExistingNonEmptyFields(t *testing.T) {
	t.Parallel()

	db := setupDB(t)
	s := New(db)
	ctx := context.Background()
	firstAt := time.Unix(1_700_000_000, 0).UTC()
	secondAt := firstAt.Add(5 * time.Minute)

	if err := s.Upsert(ctx, Summary{
		ConnectorID:        "zalo_personal",
		AccountID:          "acc-1",
		ThreadID:           "user-1",
		ThreadType:         "contact",
		Title:              "Mẹ",
		Subtitle:           "user-1",
		LastMessagePreview: "alo",
		LastMessageAt:      firstAt,
		Metadata:           map[string]string{"avatar": "a"},
	}); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}
	if err := s.Upsert(ctx, Summary{
		ConnectorID:   "zalo_personal",
		AccountID:     "acc-1",
		ThreadID:      "user-1",
		ThreadType:    "contact",
		LastMessageAt: secondAt,
		Metadata:      map[string]string{"avatar": "b", "pinned": "true"},
	}); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	got, err := s.List(ctx, Filter{
		ConnectorID: "zalo_personal",
		AccountID:   "acc-1",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 summary, got %+v", got)
	}
	if got[0].Title != "Mẹ" || got[0].Subtitle != "user-1" || got[0].LastMessagePreview != "alo" {
		t.Fatalf("expected non-empty fields to be preserved, got %+v", got[0])
	}
	if !got[0].LastMessageAt.Equal(secondAt) {
		t.Fatalf("expected last message time to update, got %v", got[0].LastMessageAt)
	}
	if got[0].Metadata["avatar"] != "b" || got[0].Metadata["pinned"] != "true" {
		t.Fatalf("expected metadata to be replaced by latest snapshot, got %+v", got[0].Metadata)
	}
}
