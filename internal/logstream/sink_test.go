package logstream

import (
	"context"
	"testing"
	"time"
)

func TestSinkSnapshotFiltersAndCapsWindow(t *testing.T) {
	t.Parallel()

	sink := New(5)
	sink.Append("web", "info", "request method=GET path=/api/work")
	sink.Append("scheduler", "warn", "missed occurrence schedule_id=daily-digest")
	sink.Append("runtime", "error", "provider call failed")

	snapshot := sink.Snapshot(Query{
		Query:  "failed",
		Level:  LevelError,
		Source: "all",
		Limit:  50,
	})

	if snapshot.BufferedEntries != 3 {
		t.Fatalf("buffered_entries = %d, want 3", snapshot.BufferedEntries)
	}
	if len(snapshot.Sources) != 3 {
		t.Fatalf("expected 3 sources, got %d", len(snapshot.Sources))
	}
	if len(snapshot.Entries) != 1 {
		t.Fatalf("expected 1 filtered entry, got %d", len(snapshot.Entries))
	}
	if snapshot.Entries[0].Source != "runtime" || snapshot.Entries[0].Level != LevelError {
		t.Fatalf("unexpected filtered entry %+v", snapshot.Entries[0])
	}
}

func TestSinkWriteParsesTimestampedLoggerLines(t *testing.T) {
	t.Parallel()

	sink := New(5)
	if _, err := sink.Write([]byte("2026/03/29 10:00:00 web info request method=GET path=/api/work\n")); err != nil {
		t.Fatalf("write log line: %v", err)
	}

	snapshot := sink.Snapshot(Query{Level: LevelAll, Source: "all", Limit: 10})
	if len(snapshot.Entries) != 1 {
		t.Fatalf("expected 1 parsed entry, got %d", len(snapshot.Entries))
	}

	entry := snapshot.Entries[0]
	if entry.Source != "web" {
		t.Fatalf("source = %q, want %q", entry.Source, "web")
	}
	if entry.Level != LevelInfo {
		t.Fatalf("level = %q, want %q", entry.Level, LevelInfo)
	}
	if entry.Message != "request method=GET path=/api/work" {
		t.Fatalf("message = %q", entry.Message)
	}
}

func TestSinkSubscribeMatchesFilters(t *testing.T) {
	t.Parallel()

	sink := New(5)
	sub := sink.Subscribe(Query{Source: "web", Level: LevelWarn, Limit: 10})
	defer sink.Unsubscribe(sub)

	sink.Append("runtime", "warn", "provider backoff")
	sink.Append("web", "info", "request method=GET path=/api/work")
	if err := sink.Emit(context.Background(), "web", "warn", "panic method=GET path=/api/debug err=boom"); err != nil {
		t.Fatalf("emit log entry: %v", err)
	}

	select {
	case entry := <-sub:
		if entry.Source != "web" || entry.Level != LevelWarn {
			t.Fatalf("unexpected subscribed entry %+v", entry)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive matching subscribed entry")
	}
}
