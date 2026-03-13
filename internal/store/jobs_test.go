// internal/store/jobs_test.go
package store_test

import (
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/store"
)

func TestJobCRUD(t *testing.T) {
	s := newTestStore(t) // helper already defined in sqlite_test.go

	now := time.Now().UTC().Truncate(time.Second)

	row := store.JobRow{
		ID:             "job-001",
		Kind:           "every",
		Target:         "opencode",
		Prompt:         "run tests",
		Schedule:       "3600",
		NextRunAt:      now.Add(time.Hour),
		LastRunAt:      nil,
		Enabled:        true,
		DeleteAfterRun: false,
		CreatedAt:      now,
	}

	// Insert
	if err := s.InsertJob(row); err != nil {
		t.Fatalf("InsertJob: %v", err)
	}

	// ListAllJobs
	rows, err := s.ListAllJobs()
	if err != nil {
		t.Fatalf("ListAllJobs: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 job, got %d", len(rows))
	}
	if rows[0].ID != "job-001" {
		t.Errorf("ID: got %q, want job-001", rows[0].ID)
	}

	// ListEnabledJobsDueBefore — not yet due
	due, err := s.ListEnabledJobsDueBefore(now)
	if err != nil {
		t.Fatalf("ListEnabledJobsDueBefore: %v", err)
	}
	if len(due) != 0 {
		t.Errorf("expected 0 due jobs, got %d", len(due))
	}

	// ListEnabledJobsDueBefore — past due
	due, err = s.ListEnabledJobsDueBefore(now.Add(2 * time.Hour))
	if err != nil {
		t.Fatalf("ListEnabledJobsDueBefore: %v", err)
	}
	if len(due) != 1 {
		t.Errorf("expected 1 due job, got %d", len(due))
	}

	// UpdateJobAfterRun
	lastRun := now.Add(time.Hour)
	nextRun := now.Add(2 * time.Hour)
	if err := s.UpdateJobAfterRun("job-001", lastRun, nextRun); err != nil {
		t.Fatalf("UpdateJobAfterRun: %v", err)
	}
	rows, _ = s.ListAllJobs()
	if rows[0].LastRunAt == nil || !rows[0].LastRunAt.Equal(lastRun) {
		t.Errorf("LastRunAt: got %v, want %v", rows[0].LastRunAt, lastRun)
	}
	if !rows[0].NextRunAt.Equal(nextRun) {
		t.Errorf("NextRunAt: got %v, want %v", rows[0].NextRunAt, nextRun)
	}

	// UpdateJobField — disable
	if err := s.UpdateJobField("job-001", "enabled", 0); err != nil {
		t.Fatalf("UpdateJobField: %v", err)
	}
	rows, _ = s.ListAllJobs()
	if rows[0].Enabled {
		t.Errorf("expected enabled=false after UpdateJobField")
	}

	// DeleteJob
	if err := s.DeleteJob("job-001"); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
	rows, _ = s.ListAllJobs()
	if len(rows) != 0 {
		t.Errorf("expected 0 rows after DeleteJob, got %d", len(rows))
	}
}

func TestInsertJob_DuplicateID(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()
	row := store.JobRow{
		ID:        "dup-001",
		Kind:      "at",
		Target:    "chat",
		Prompt:    "hello",
		Schedule:  now.Format(time.RFC3339),
		NextRunAt: now,
		Enabled:   true,
		CreatedAt: now,
	}
	if err := s.InsertJob(row); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := s.InsertJob(row); err == nil {
		t.Fatal("expected error on duplicate insert, got nil")
	}
}

func TestListEnabledJobsDueBefore_DisabledNotReturned(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()
	row := store.JobRow{
		ID:        "disabled-001",
		Kind:      "every",
		Target:    "opencode",
		Prompt:    "x",
		Schedule:  "60",
		NextRunAt: now.Add(-time.Minute), // past due
		Enabled:   false,                 // but disabled
		CreatedAt: now,
	}
	if err := s.InsertJob(row); err != nil {
		t.Fatalf("InsertJob: %v", err)
	}
	due, err := s.ListEnabledJobsDueBefore(now.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListEnabledJobsDueBefore: %v", err)
	}
	if len(due) != 0 {
		t.Errorf("disabled job should not appear; got %d", len(due))
	}
}
