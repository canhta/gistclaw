package zalopersonal

import (
	"context"
	"testing"

	"github.com/canhta/gistclaw/internal/store"
)

func TestStoredCredentialsLoadSaveClear(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("missing credentials returns false", func(t *testing.T) {
		t.Parallel()

		db, err := store.Open(":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if err := store.Migrate(db); err != nil {
			t.Fatalf("migrate db: %v", err)
		}

		got, ok, err := LoadStoredCredentials(ctx, db)
		if err != nil {
			t.Fatalf("LoadStoredCredentials: %v", err)
		}
		if ok {
			t.Fatalf("expected no stored credentials, got %+v", got)
		}
		if got != (StoredCredentials{}) {
			t.Fatalf("expected zero credentials, got %+v", got)
		}
	})

	t.Run("save then load round trips credentials", func(t *testing.T) {
		t.Parallel()

		db, err := store.Open(":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if err := store.Migrate(db); err != nil {
			t.Fatalf("migrate db: %v", err)
		}

		want := StoredCredentials{
			AccountID:   "123456789",
			DisplayName: "Canh",
			IMEI:        "imei-123",
			Cookie:      "cookie=value",
			UserAgent:   "Mozilla/5.0",
			Language:    "vi",
		}
		if err := SaveStoredCredentials(ctx, db, want); err != nil {
			t.Fatalf("SaveStoredCredentials: %v", err)
		}

		got, ok, err := LoadStoredCredentials(ctx, db)
		if err != nil {
			t.Fatalf("LoadStoredCredentials: %v", err)
		}
		if !ok {
			t.Fatal("expected stored credentials to exist")
		}
		if got != want {
			t.Fatalf("expected %+v, got %+v", want, got)
		}
	})

	t.Run("clear removes stored credentials", func(t *testing.T) {
		t.Parallel()

		db, err := store.Open(":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if err := store.Migrate(db); err != nil {
			t.Fatalf("migrate db: %v", err)
		}

		if err := SaveStoredCredentials(ctx, db, StoredCredentials{
			AccountID: "123456789",
			IMEI:      "imei-123",
			Cookie:    "cookie=value",
			UserAgent: "Mozilla/5.0",
		}); err != nil {
			t.Fatalf("SaveStoredCredentials: %v", err)
		}
		if err := ClearStoredCredentials(ctx, db); err != nil {
			t.Fatalf("ClearStoredCredentials: %v", err)
		}

		got, ok, err := LoadStoredCredentials(ctx, db)
		if err != nil {
			t.Fatalf("LoadStoredCredentials: %v", err)
		}
		if ok {
			t.Fatalf("expected cleared credentials, got %+v", got)
		}
	})
}
