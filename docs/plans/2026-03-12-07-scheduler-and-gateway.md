# GistClaw Plan 7: Scheduler & Gateway

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build `internal/scheduler/service.go` (1-second ticker, job CRUD, gronx cron, `Tools()`) and `internal/gateway/service.go` (channel-agnostic controller, plain-chat tool loop with doom-loop guard, command routing).

**Architecture:** Three tasks in dependency order. Task 1 adds job CRUD methods to the existing SQLite store. Task 2 builds `scheduler.Service` on top of the store with a `JobTarget` interface and `Tools()` method exposing four LLM-callable tools. Task 3 builds `gateway.Service`, which wires channel, HITL, both agent services, LLM, search, web fetch, MCP, and scheduler into a single controller that routes commands and runs a multi-round plain-chat tool loop. No new external dependencies beyond `gronx` and `uuid` (already in the dependency budget).

**Tech Stack:** Go 1.25, `github.com/adhocore/gronx`, `github.com/google/uuid`, `modernc.org/sqlite` (already present), `github.com/rs/zerolog` (already present), all stdlib (`time`, `context`, `encoding/json`, `strings`, `fmt`).

**Design reference:** `docs/plans/design.md` §7, §9.15, §9.23, §11

**Depends on:** Plans 1–6

---

## Execution order

```
Task 1  internal/store additions (job CRUD methods on *store.Store)
Task 2  internal/scheduler/service.go
Task 3  internal/gateway/service.go
```

Task 1 has no dependencies beyond the existing store package.
Task 2 depends on Task 1 (store job methods) and existing packages: `internal/agent`, `internal/providers`, `internal/config`.
Task 3 depends on Task 2 (scheduler.Service) and existing packages: `internal/channel`, `internal/hitl`, `internal/opencode`, `internal/claudecode`, `internal/providers`, `internal/tools`, `internal/mcp`, `internal/store`, `internal/config`.

---

## Task 1: `internal/store` additions — job CRUD methods

**Files:**
- Modify: `internal/store/sqlite.go`
- Create: `internal/store/jobs_test.go`

The `jobs` table already exists in `schema.sql` from Plan 2. This task adds Go methods to `*store.Store` to insert, query, update, and delete jobs.

> **Note:** The `scheduler.Job` type is defined in `internal/scheduler/service.go` (Task 2).
> To avoid a circular import, the store methods accept `scheduler.Job` from the `scheduler` package.
> However, because `store` must not import `scheduler`, pass job fields as flat parameters or define
> a `store.JobRow` mirror struct in the store package. Use a mirror struct: `store.JobRow`. The
> scheduler package converts between `Job` and `store.JobRow`.

### Step 1: Write the failing test

```go
// internal/store/jobs_test.go
package store_test

import (
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/store"
)

func TestJobCRUD(t *testing.T) {
	s := newTestStore(t) // helper already used in existing store tests; creates temp DB

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
```

### Step 2: Run to verify it fails

```bash
go test ./internal/store/... -run TestJobCRUD -v
```

Expected: compile error — `store.JobRow`, `InsertJob`, `ListAllJobs`, `ListEnabledJobsDueBefore`, `UpdateJobAfterRun`, `UpdateJobField`, `DeleteJob` undefined.

### Step 3: Add `JobRow` struct and CRUD methods to `internal/store/sqlite.go`

Add the following to `internal/store/sqlite.go` (after existing store code):

```go
// JobRow mirrors the jobs table. Used by scheduler.Service to avoid a circular import.
type JobRow struct {
	ID             string
	Kind           string     // "at" | "every" | "cron"
	Target         string     // agent.Kind.String(): "opencode" | "claudecode" | "chat"
	Prompt         string
	Schedule       string
	NextRunAt      time.Time
	LastRunAt      *time.Time
	Enabled        bool
	DeleteAfterRun bool
	CreatedAt      time.Time
}

// InsertJob inserts a new job row. Returns error on duplicate ID.
func (s *Store) InsertJob(j JobRow) error {
	_, err := s.db.Exec(`
		INSERT INTO jobs (id, kind, target, prompt, schedule, next_run_at, last_run_at,
		                  enabled, delete_after_run, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.ID, j.Kind, j.Target, j.Prompt, j.Schedule,
		j.NextRunAt.UTC().Format(time.RFC3339),
		nullableTime(j.LastRunAt),
		boolToInt(j.Enabled),
		boolToInt(j.DeleteAfterRun),
		j.CreatedAt.UTC().Format(time.RFC3339),
	)
	return err
}

// ListEnabledJobsDueBefore returns all enabled jobs with next_run_at <= t.
func (s *Store) ListEnabledJobsDueBefore(t time.Time) ([]JobRow, error) {
	rows, err := s.db.Query(`
		SELECT id, kind, target, prompt, schedule, next_run_at, last_run_at,
		       enabled, delete_after_run, created_at
		FROM jobs
		WHERE enabled = 1 AND next_run_at <= ?`,
		t.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanJobRows(rows)
}

// ListAllJobs returns all job rows regardless of status.
func (s *Store) ListAllJobs() ([]JobRow, error) {
	rows, err := s.db.Query(`
		SELECT id, kind, target, prompt, schedule, next_run_at, last_run_at,
		       enabled, delete_after_run, created_at
		FROM jobs ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanJobRows(rows)
}

// UpdateJobAfterRun sets last_run_at and next_run_at for the given job.
func (s *Store) UpdateJobAfterRun(id string, lastRunAt time.Time, nextRunAt time.Time) error {
	_, err := s.db.Exec(`
		UPDATE jobs SET last_run_at = ?, next_run_at = ? WHERE id = ?`,
		lastRunAt.UTC().Format(time.RFC3339),
		nextRunAt.UTC().Format(time.RFC3339),
		id,
	)
	return err
}

// DeleteJob removes a job by ID.
func (s *Store) DeleteJob(id string) error {
	_, err := s.db.Exec(`DELETE FROM jobs WHERE id = ?`, id)
	return err
}

// UpdateJobField sets a single column value for the given job ID.
// field must be a valid column name; value must be SQLite-compatible.
func (s *Store) UpdateJobField(id string, field string, value any) error {
	// Allowlist to prevent SQL injection from LLM-supplied field names.
	allowed := map[string]bool{
		"enabled": true, "next_run_at": true, "delete_after_run": true,
	}
	if !allowed[field] {
		return fmt.Errorf("store: UpdateJobField: field %q not allowed", field)
	}
	_, err := s.db.Exec(fmt.Sprintf(`UPDATE jobs SET %s = ? WHERE id = ?`, field), value, id)
	return err
}

// --- helpers ---

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func scanJobRows(rows *sql.Rows) ([]JobRow, error) {
	var result []JobRow
	for rows.Next() {
		var r JobRow
		var nextRunAt, createdAt string
		var lastRunAt *string
		var enabled, deleteAfterRun int
		if err := rows.Scan(
			&r.ID, &r.Kind, &r.Target, &r.Prompt, &r.Schedule,
			&nextRunAt, &lastRunAt,
			&enabled, &deleteAfterRun, &createdAt,
		); err != nil {
			return nil, err
		}
		r.Enabled = enabled == 1
		r.DeleteAfterRun = deleteAfterRun == 1
		if t, err := time.Parse(time.RFC3339, nextRunAt); err == nil {
			r.NextRunAt = t
		}
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			r.CreatedAt = t
		}
		if lastRunAt != nil {
			if t, err := time.Parse(time.RFC3339, *lastRunAt); err == nil {
				r.LastRunAt = &t
			}
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
```

> **Note:** `nullableTime` and `boolToInt` may already exist in the file from earlier plans.
> If so, skip re-adding them and adjust the helpers accordingly.
> Check for `sql.Rows` import — add `"database/sql"` if not already present (it likely is).

### Step 4: Run tests to verify they pass

```bash
go test ./internal/store/... -run "TestJobCRUD|TestInsertJob_DuplicateID|TestListEnabledJobsDueBefore_DisabledNotReturned" -v
```

Expected: all three tests PASS.

### Step 5: Run full store test suite to check no regressions

```bash
go test ./internal/store/... -v
```

Expected: all tests PASS.

### Step 6: Commit

```bash
git add internal/store/sqlite.go internal/store/jobs_test.go
git commit -m "feat(store): add job CRUD methods (InsertJob, ListAllJobs, ListEnabledJobsDueBefore, UpdateJobAfterRun, DeleteJob, UpdateJobField)"
```

---

## Task 2: `internal/scheduler/service.go`

**Files:**
- Create: `internal/scheduler/service.go`
- Create: `internal/scheduler/service_test.go`

### Add dependency

```bash
go get github.com/adhocore/gronx
go get github.com/google/uuid
```

Verify both appear in `go.mod`:
```bash
grep -E "gronx|google/uuid" go.mod
```

### Step 1: Write the failing tests

```go
// internal/scheduler/service_test.go
package scheduler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/agent"
	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/scheduler"
	"github.com/canhta/gistclaw/internal/store"
)

// --- mock JobTarget ---

type mockTarget struct {
	mu      sync.Mutex
	calls   []targetCall
	chatIDs []int64
}

type targetCall struct {
	Kind   agent.Kind
	Prompt string
}

func (m *mockTarget) RunAgentTask(_ context.Context, kind agent.Kind, prompt string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, targetCall{Kind: kind, Prompt: prompt})
	return nil
}

func (m *mockTarget) SendChat(_ context.Context, chatID int64, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chatIDs = append(m.chatIDs, chatID)
	return nil
}

func (m *mockTarget) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// --- helpers ---

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
	if err != nil {
		t.Fatalf("newTestStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func defaultTuning() config.Tuning {
	return config.Tuning{
		SchedulerTick:       100 * time.Millisecond, // fast for tests
		MissedJobsFireLimit: 5,
	}
}

// --- tests ---

// TestScheduler_EveryJob verifies an "every" job fires when its next_run_at is reached.
func TestScheduler_EveryJob(t *testing.T) {
	s := newTestStore(t)
	target := &mockTarget{}
	tuning := defaultTuning()
	svc := scheduler.NewService(s, target, tuning)

	now := time.Now().UTC()
	row := store.JobRow{
		ID:        "every-001",
		Kind:      "every",
		Target:    "opencode",
		Prompt:    "run tests",
		Schedule:  "1", // 1 second
		NextRunAt: now.Add(-time.Second), // already due
		Enabled:   true,
		CreatedAt: now,
	}
	if err := s.InsertJob(row); err != nil {
		t.Fatalf("InsertJob: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	time.Sleep(300 * time.Millisecond)
	if target.callCount() < 1 {
		t.Errorf("expected >=1 RunAgentTask call, got %d", target.callCount())
	}
}

// TestScheduler_AtJob verifies an "at" job fires once then is disabled and deleted.
func TestScheduler_AtJob(t *testing.T) {
	s := newTestStore(t)
	target := &mockTarget{}
	svc := scheduler.NewService(s, target, defaultTuning())

	now := time.Now().UTC()
	row := store.JobRow{
		ID:             "at-001",
		Kind:           "at",
		Target:         "opencode",
		Prompt:         "deploy",
		Schedule:       now.Add(-time.Second).Format(time.RFC3339),
		NextRunAt:      now.Add(-time.Second),
		Enabled:        true,
		DeleteAfterRun: true,
		CreatedAt:      now,
	}
	if err := s.InsertJob(row); err != nil {
		t.Fatalf("InsertJob: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	time.Sleep(300 * time.Millisecond)
	if target.callCount() != 1 {
		t.Errorf("expected exactly 1 RunAgentTask call, got %d", target.callCount())
	}

	// Job must be deleted from store after firing
	rows, err := s.ListAllJobs()
	if err != nil {
		t.Fatalf("ListAllJobs: %v", err)
	}
	for _, r := range rows {
		if r.ID == "at-001" {
			t.Errorf("'at' job should be deleted after firing")
		}
	}
}

// TestScheduler_CronJob verifies a cron job's next_run_at advances correctly via gronx.
func TestScheduler_CronJob(t *testing.T) {
	s := newTestStore(t)
	target := &mockTarget{}
	svc := scheduler.NewService(s, target, defaultTuning())

	now := time.Now().UTC()
	row := store.JobRow{
		ID:        "cron-001",
		Kind:      "cron",
		Target:    "claudecode",
		Prompt:    "check status",
		Schedule:  "* * * * *", // every minute
		NextRunAt: now.Add(-time.Second), // overdue
		Enabled:   true,
		CreatedAt: now,
	}
	if err := s.InsertJob(row); err != nil {
		t.Fatalf("InsertJob: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	time.Sleep(300 * time.Millisecond)
	if target.callCount() < 1 {
		t.Errorf("expected >=1 RunAgentTask call, got %d", target.callCount())
	}

	// After firing, next_run_at must be in the future
	rows, err := s.ListAllJobs()
	if err != nil {
		t.Fatalf("ListAllJobs: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected cron job to remain; got 0 rows")
	}
	if !rows[0].NextRunAt.After(now) {
		t.Errorf("expected next_run_at in future, got %v", rows[0].NextRunAt)
	}
}

// TestScheduler_MissedJobsOnStartup verifies overdue jobs are fired immediately (up to limit)
// and remaining overdue jobs have their next_run_at advanced.
func TestScheduler_MissedJobsOnStartup(t *testing.T) {
	s := newTestStore(t)
	target := &mockTarget{}
	tuning := config.Tuning{
		SchedulerTick:       100 * time.Millisecond,
		MissedJobsFireLimit: 2, // fire only 2 immediately; rest advance
	}
	svc := scheduler.NewService(s, target, tuning)

	now := time.Now().UTC()
	for i := range 5 {
		row := store.JobRow{
			ID:        fmt.Sprintf("missed-%02d", i),
			Kind:      "every",
			Target:    "opencode",
			Prompt:    fmt.Sprintf("job %d", i),
			Schedule:  "3600",
			NextRunAt: now.Add(-time.Hour), // all overdue
			Enabled:   true,
			CreatedAt: now,
		}
		if err := s.InsertJob(row); err != nil {
			t.Fatalf("InsertJob: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	// Wait for startup to complete (2 fires at 500ms stagger + some margin)
	time.Sleep(1500 * time.Millisecond)

	// Exactly MissedJobsFireLimit=2 jobs should have fired on startup
	if count := target.callCount(); count < 2 {
		t.Errorf("expected >=2 startup fires, got %d", count)
	}

	// All remaining jobs must have next_run_at in the future
	rows, err := s.ListAllJobs()
	if err != nil {
		t.Fatalf("ListAllJobs: %v", err)
	}
	for _, r := range rows {
		if r.NextRunAt.Before(now) && r.Enabled {
			t.Errorf("job %s still has past next_run_at %v", r.ID, r.NextRunAt)
		}
	}
}

// TestScheduler_KindChat verifies a KindChat job calls SendChat (not RunAgentTask).
func TestScheduler_KindChat(t *testing.T) {
	s := newTestStore(t)
	target := &mockTarget{}
	svc := scheduler.NewService(s, target, defaultTuning())

	now := time.Now().UTC()
	row := store.JobRow{
		ID:        "chat-001",
		Kind:      "every",
		Target:    "chat",
		Prompt:    "daily summary",
		Schedule:  "3600",
		NextRunAt: now.Add(-time.Second),
		Enabled:   true,
		CreatedAt: now,
	}
	if err := s.InsertJob(row); err != nil {
		t.Fatalf("InsertJob: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	time.Sleep(300 * time.Millisecond)
	if target.callCount() != 0 {
		t.Errorf("expected 0 RunAgentTask calls for KindChat, got %d", target.callCount())
	}
}

// TestScheduler_Tools verifies Tools() returns exactly 4 tools with correct names and schemas.
func TestScheduler_Tools(t *testing.T) {
	s := newTestStore(t)
	svc := scheduler.NewService(s, &mockTarget{}, defaultTuning())

	tools := svc.Tools()
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}

	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
		// Each tool must have a non-empty description
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
		// InputSchema must be valid JSON object
		var schema map[string]any
		b, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Errorf("tool %q: InputSchema marshal error: %v", tool.Name, err)
			continue
		}
		if err := json.Unmarshal(b, &schema); err != nil {
			t.Errorf("tool %q: InputSchema is not a JSON object: %v", tool.Name, err)
		}
	}

	for _, want := range []string{"schedule_job", "list_jobs", "update_job", "delete_job"} {
		if !names[want] {
			t.Errorf("missing tool %q", want)
		}
	}
}

// TestScheduler_CRUD verifies CreateJob / ListJobs / UpdateJob / DeleteJob service methods.
func TestScheduler_CRUD(t *testing.T) {
	s := newTestStore(t)
	svc := scheduler.NewService(s, &mockTarget{}, defaultTuning())

	j := scheduler.Job{
		Kind:     "every",
		Target:   agent.KindOpenCode,
		Prompt:   "test",
		Schedule: "60",
	}

	if err := svc.CreateJob(j); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	jobs, err := svc.ListJobs()
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	id := jobs[0].ID
	if id == "" {
		t.Fatal("expected non-empty job ID")
	}

	if err := svc.UpdateJob(id, map[string]any{"enabled": false}); err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}
	jobs, _ = svc.ListJobs()
	if jobs[0].Enabled {
		t.Errorf("expected enabled=false after UpdateJob")
	}

	if err := svc.DeleteJob(id); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
	jobs, _ = svc.ListJobs()
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs after DeleteJob, got %d", len(jobs))
	}
}

// TestScheduler_CreateJob_InvalidKind verifies CreateJob rejects unknown job kinds.
func TestScheduler_CreateJob_InvalidKind(t *testing.T) {
	s := newTestStore(t)
	svc := scheduler.NewService(s, &mockTarget{}, defaultTuning())
	j := scheduler.Job{
		Kind:     "invalid",
		Target:   agent.KindOpenCode,
		Prompt:   "x",
		Schedule: "60",
	}
	if err := svc.CreateJob(j); err == nil {
		t.Error("expected error for invalid Kind, got nil")
	}
}

// TestScheduler_CreateJob_InvalidCronExpr verifies gronx cron validation on CreateJob.
func TestScheduler_CreateJob_InvalidCronExpr(t *testing.T) {
	s := newTestStore(t)
	svc := scheduler.NewService(s, &mockTarget{}, defaultTuning())
	j := scheduler.Job{
		Kind:     "cron",
		Target:   agent.KindOpenCode,
		Prompt:   "x",
		Schedule: "not a cron expression",
	}
	if err := svc.CreateJob(j); err == nil {
		t.Error("expected error for invalid cron expression, got nil")
	}
}
```

### Step 2: Run to verify tests fail

```bash
go test ./internal/scheduler/... -v
```

Expected: compile error — package `internal/scheduler` does not exist.

### Step 3: Add dependencies

```bash
go get github.com/adhocore/gronx
go get github.com/google/uuid
```

### Step 4: Implement `internal/scheduler/service.go`

```go
// internal/scheduler/service.go
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/adhocore/gronx"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/agent"
	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/store"
)

// Job represents a scheduled task.
type Job struct {
	ID             string
	Kind           string     // "at" | "every" | "cron"
	Target         agent.Kind // typed enum; stored as string in SQLite
	Prompt         string
	Schedule       string     // RFC3339 (at) | seconds string (every) | cron expr (cron)
	NextRunAt      time.Time
	LastRunAt      *time.Time
	Enabled        bool
	DeleteAfterRun bool       // always true for "at" jobs
	CreatedAt      time.Time
}

// JobTarget is implemented by app.App (wired in Plan 8).
type JobTarget interface {
	RunAgentTask(ctx context.Context, kind agent.Kind, prompt string) error
	SendChat(ctx context.Context, chatID int64, text string) error
}

// Service runs the scheduler: fires jobs on schedule, manages CRUD.
type Service struct {
	store  *store.Store
	target JobTarget
	tuning config.Tuning
}

// NewService creates a new scheduler Service.
func NewService(s *store.Store, target JobTarget, tuning config.Tuning) *Service {
	return &Service{store: s, target: target, tuning: tuning}
}

// Run starts the scheduler loop. Blocks until ctx is cancelled.
// On startup, fires missed jobs immediately (up to MissedJobsFireLimit) and
// advances next_run_at for the rest.
func (s *Service) Run(ctx context.Context) error {
	if err := s.handleMissedJobs(ctx); err != nil {
		log.Warn().Err(err).Msg("scheduler: handleMissedJobs error (non-fatal)")
	}

	tick := time.Duration(s.tuning.SchedulerTick)
	if tick == 0 {
		tick = time.Second
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			if err := s.tick(ctx, now); err != nil {
				log.Error().Err(err).Msg("scheduler: tick error")
			}
		}
	}
}

// tick queries due jobs and fires each one.
func (s *Service) tick(ctx context.Context, now time.Time) error {
	jobs, err := s.store.ListEnabledJobsDueBefore(now)
	if err != nil {
		return fmt.Errorf("scheduler: ListEnabledJobsDueBefore: %w", err)
	}
	for _, row := range jobs {
		s.fireJob(ctx, row, now)
	}
	return nil
}

// fireJob executes a single job row, then updates store state.
func (s *Service) fireJob(ctx context.Context, row store.JobRow, now time.Time) {
	j := rowToJob(row)

	var fireErr error
	if j.Target == agent.KindChat {
		// chatID for scheduled chat is determined by convention: encode chatID in prompt as
		// "chatID:<n>:<text>" or fall back to 0 (operator). For v1 simplicity, use 0.
		fireErr = s.target.SendChat(ctx, 0, j.Prompt)
	} else {
		fireErr = s.target.RunAgentTask(ctx, j.Target, j.Prompt)
	}

	if fireErr != nil {
		log.Warn().Str("job_id", j.ID).Err(fireErr).Msg("scheduler: job fire error; skipped")
		// Notify operator via SendChat — best-effort; ignore error.
		_ = s.target.SendChat(ctx, 0, "⏰ Scheduled job skipped: agent busy.")
		return
	}

	lastRun := now
	if j.DeleteAfterRun {
		// "at" jobs: disable + delete
		_ = s.store.UpdateJobField(j.ID, "enabled", 0)
		_ = s.store.DeleteJob(j.ID)
		return
	}

	nextRun, err := s.advanceNextRun(j, now)
	if err != nil {
		log.Error().Str("job_id", j.ID).Err(err).Msg("scheduler: advanceNextRun failed")
		return
	}
	_ = s.store.UpdateJobAfterRun(j.ID, lastRun, nextRun)
}

// advanceNextRun computes the next run time for a job after it fires.
func (s *Service) advanceNextRun(j Job, now time.Time) (time.Time, error) {
	switch j.Kind {
	case "every":
		secs, err := strconv.ParseInt(j.Schedule, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid 'every' schedule %q: %w", j.Schedule, err)
		}
		return now.Add(time.Duration(secs) * time.Second), nil
	case "cron":
		gx := gronx.New()
		next, err := gx.NextTick(j.Schedule, false)
		if err != nil {
			return time.Time{}, fmt.Errorf("gronx NextTick(%q): %w", j.Schedule, err)
		}
		return next, nil
	default:
		return now, nil // "at" jobs never reach this branch
	}
}

// handleMissedJobs fires up to MissedJobsFireLimit overdue jobs immediately,
// advances next_run_at for the rest to the next valid future time.
func (s *Service) handleMissedJobs(ctx context.Context) error {
	now := time.Now().UTC()
	jobs, err := s.store.ListEnabledJobsDueBefore(now)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		return nil
	}

	limit := s.tuning.MissedJobsFireLimit
	if limit == 0 {
		limit = 5
	}

	fired := 0
	for _, row := range jobs {
		j := rowToJob(row)
		if fired < limit {
			s.fireJob(ctx, row, now)
			fired++
			// Stagger startup fires 500ms apart.
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(500 * time.Millisecond):
			}
		} else {
			// Advance to next future time rather than fire immediately.
			next, err := s.advanceNextRun(j, now)
			if err != nil {
				log.Warn().Str("job_id", j.ID).Err(err).Msg("scheduler: missed job advance failed; disabling")
				_ = s.store.UpdateJobField(j.ID, "enabled", 0)
				continue
			}
			_ = s.store.UpdateJobAfterRun(j.ID, now, next)
			log.Warn().Str("job_id", j.ID).Time("next_run_at", next).
				Msg("scheduler: missed job not fired (over limit); next_run_at advanced")
		}
	}
	return nil
}

// --- CRUD methods (called by gateway tool loop) ---

// CreateJob validates and inserts a new job. Assigns a new ULID-style ID (uuid v4).
func (s *Service) CreateJob(j Job) error {
	if err := validateJob(j); err != nil {
		return err
	}
	j.ID = uuid.New().String()
	j.CreatedAt = time.Now().UTC()
	if j.Kind == "at" {
		j.DeleteAfterRun = true
	}

	nextRun, err := s.computeInitialNextRun(j)
	if err != nil {
		return err
	}
	j.NextRunAt = nextRun
	j.Enabled = true

	return s.store.InsertJob(jobToRow(j))
}

// ListJobs returns all jobs in the store.
func (s *Service) ListJobs() ([]Job, error) {
	rows, err := s.store.ListAllJobs()
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, len(rows))
	for i, r := range rows {
		jobs[i] = rowToJob(r)
	}
	return jobs, nil
}

// UpdateJob updates specific fields of an existing job by ID.
// Supported fields: "enabled" (bool).
func (s *Service) UpdateJob(id string, fields map[string]any) error {
	for k, v := range fields {
		switch k {
		case "enabled":
			val := 0
			switch b := v.(type) {
			case bool:
				if b {
					val = 1
				}
			case float64:
				if b != 0 {
					val = 1
				}
			}
			if err := s.store.UpdateJobField(id, "enabled", val); err != nil {
				return fmt.Errorf("UpdateJob: %w", err)
			}
		default:
			return fmt.Errorf("UpdateJob: field %q not supported", k)
		}
	}
	return nil
}

// DeleteJob deletes a job by ID.
func (s *Service) DeleteJob(id string) error {
	return s.store.DeleteJob(id)
}

// computeInitialNextRun sets the first next_run_at based on job kind.
func (s *Service) computeInitialNextRun(j Job) (time.Time, error) {
	now := time.Now().UTC()
	switch j.Kind {
	case "at":
		t, err := time.Parse(time.RFC3339, j.Schedule)
		if err != nil {
			return time.Time{}, fmt.Errorf("'at' schedule must be RFC3339: %w", err)
		}
		return t.UTC(), nil
	case "every":
		secs, err := strconv.ParseInt(j.Schedule, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("'every' schedule must be seconds string: %w", err)
		}
		return now.Add(time.Duration(secs) * time.Second), nil
	case "cron":
		gx := gronx.New()
		next, err := gx.NextTick(j.Schedule, false)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid cron expression %q: %w", j.Schedule, err)
		}
		return next, nil
	default:
		return time.Time{}, fmt.Errorf("unknown job kind %q", j.Kind)
	}
}

// validateJob checks the job struct before inserting.
func validateJob(j Job) error {
	switch j.Kind {
	case "at", "every", "cron":
	default:
		return fmt.Errorf("invalid job kind %q; must be 'at', 'every', or 'cron'", j.Kind)
	}
	if j.Prompt == "" {
		return fmt.Errorf("job prompt must not be empty")
	}
	if j.Schedule == "" {
		return fmt.Errorf("job schedule must not be empty")
	}
	if j.Kind == "cron" {
		gx := gronx.New()
		if !gx.IsValid(j.Schedule) {
			return fmt.Errorf("invalid cron expression: %q", j.Schedule)
		}
	}
	return nil
}

// --- Tools() ---

// Tools returns the four scheduler control tools for the gateway's System tool category.
func (s *Service) Tools() []providers.Tool {
	return []providers.Tool{
		{
			Name:        "schedule_job",
			Description: "Create a scheduled job. kind: 'at' (one-time, RFC3339 schedule), 'every' (recurring, schedule in seconds), or 'cron' (cron expression). target: 'opencode', 'claudecode', or 'chat'.",
			InputSchema: scheduleJobSchema(),
		},
		{
			Name:        "list_jobs",
			Description: "List all scheduled jobs with their IDs, kinds, targets, schedules, and enabled status.",
			InputSchema: emptySchema(),
		},
		{
			Name:        "update_job",
			Description: "Update a scheduled job. Currently supports toggling enabled status.",
			InputSchema: updateJobSchema(),
		},
		{
			Name:        "delete_job",
			Description: "Delete a scheduled job by ID.",
			InputSchema: deleteJobSchema(),
		},
	}
}

func scheduleJobSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kind": map[string]any{
				"type":        "string",
				"enum":        []string{"at", "every", "cron"},
				"description": "'at' = one-time (RFC3339 schedule), 'every' = recurring (seconds), 'cron' = cron expression",
			},
			"target": map[string]any{
				"type":        "string",
				"enum":        []string{"opencode", "claudecode", "chat"},
				"description": "Which agent (or chat channel) to target",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The prompt/task to send when the job fires",
			},
			"schedule": map[string]any{
				"type":        "string",
				"description": "RFC3339 datetime (at), seconds string e.g. '3600' (every), or cron expression e.g. '0 9 * * 1-5' (cron)",
			},
		},
		"required": []string{"kind", "target", "prompt", "schedule"},
	}
}

func updateJobSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "Job ID (from list_jobs)",
			},
			"enabled": map[string]any{
				"type":        "boolean",
				"description": "Enable or disable the job",
			},
		},
		"required": []string{"id"},
	}
}

func deleteJobSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "Job ID to delete",
			},
		},
		"required": []string{"id"},
	}
}

func emptySchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

// --- conversion helpers ---

func rowToJob(r store.JobRow) Job {
	kind, _ := agent.KindFromString(r.Target)
	return Job{
		ID:             r.ID,
		Kind:           r.Kind,
		Target:         kind,
		Prompt:         r.Prompt,
		Schedule:       r.Schedule,
		NextRunAt:      r.NextRunAt,
		LastRunAt:      r.LastRunAt,
		Enabled:        r.Enabled,
		DeleteAfterRun: r.DeleteAfterRun,
		CreatedAt:      r.CreatedAt,
	}
}

func jobToRow(j Job) store.JobRow {
	return store.JobRow{
		ID:             j.ID,
		Kind:           j.Kind,
		Target:         j.Target.String(),
		Prompt:         j.Prompt,
		Schedule:       j.Schedule,
		NextRunAt:      j.NextRunAt,
		LastRunAt:      j.LastRunAt,
		Enabled:        j.Enabled,
		DeleteAfterRun: j.DeleteAfterRun,
		CreatedAt:      j.CreatedAt,
	}
}

// jobsToJSON serialises a slice of Jobs for tool results.
func jobsToJSON(jobs []Job) string {
	type item struct {
		ID        string `json:"id"`
		Kind      string `json:"kind"`
		Target    string `json:"target"`
		Prompt    string `json:"prompt"`
		Schedule  string `json:"schedule"`
		NextRunAt string `json:"next_run_at"`
		Enabled   bool   `json:"enabled"`
	}
	items := make([]item, len(jobs))
	for i, j := range jobs {
		items[i] = item{
			ID:        j.ID,
			Kind:      j.Kind,
			Target:    j.Target.String(),
			Prompt:    j.Prompt,
			Schedule:  j.Schedule,
			NextRunAt: j.NextRunAt.Format(time.RFC3339),
			Enabled:   j.Enabled,
		}
	}
	b, _ := json.MarshalIndent(items, "", "  ")
	return string(b)
}
```

> **Important implementation note — `store.New` signature:**
> Check the actual `store.New` constructor signature from Plan 2. Adjust `newTestStore` in the test if
> the constructor takes a path string differently (e.g. `store.New(":memory:")` or
> `store.New(cfg)` — match whatever Plan 2 implemented).

### Step 5: Run tests

```bash
go test ./internal/scheduler/... -v
```

Expected: all tests PASS.

### Step 6: Run full test suite

```bash
go test ./... 
```

Expected: all tests PASS. No regressions.

### Step 7: Commit

```bash
git add internal/scheduler/ go.mod go.sum
git commit -m "feat(scheduler): add scheduler.Service with Job CRUD, ticker loop, gronx cron, and Tools()"
```

---

## Task 3: `internal/gateway/service.go`

**Files:**
- Create: `internal/gateway/service.go`
- Create: `internal/gateway/service_test.go`

### Step 1: Write the failing tests

```go
// internal/gateway/service_test.go
package gateway_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/agent"
	"github.com/canhta/gistclaw/internal/channel"
	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/gateway"
	"github.com/canhta/gistclaw/internal/hitl"
	"github.com/canhta/gistclaw/internal/mcp"
	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/scheduler"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

// ============================================================
// Mock implementations
// ============================================================

// --- mock channel ---

type mockChannel struct {
	mu       sync.Mutex
	inbound  chan channel.InboundMessage
	sent     []string
	typings  []int64
	name     string
}

func newMockChannel() *mockChannel {
	return &mockChannel{
		inbound: make(chan channel.InboundMessage, 10),
		name:    "mock",
	}
}

func (m *mockChannel) Receive(_ context.Context) (<-chan channel.InboundMessage, error) {
	return m.inbound, nil
}

func (m *mockChannel) SendMessage(_ context.Context, _ int64, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, text)
	return nil
}

func (m *mockChannel) SendKeyboard(_ context.Context, _ int64, _ channel.KeyboardPayload) error {
	return nil
}

func (m *mockChannel) SendTyping(_ context.Context, chatID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.typings = append(m.typings, chatID)
	return nil
}

func (m *mockChannel) Name() string { return m.name }

func (m *mockChannel) sentMessages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.sent))
	copy(cp, m.sent)
	return cp
}

// --- mock hitl.Approver ---

type mockApprover struct{}

func (m *mockApprover) RequestPermission(_ context.Context, _ hitl.PermissionRequest) error {
	return nil
}

func (m *mockApprover) RequestQuestion(_ context.Context, _ hitl.QuestionRequest) error {
	return nil
}

// --- mock opencode.Service ---

type mockOCService struct {
	mu       sync.Mutex
	tasks    []string
	stopped  bool
	isAlive  bool
}

func (m *mockOCService) Run(_ context.Context) error       { return nil }
func (m *mockOCService) IsAlive(_ context.Context) bool    { m.mu.Lock(); defer m.mu.Unlock(); return m.isAlive }
func (m *mockOCService) Stop(_ context.Context) error      { m.mu.Lock(); defer m.mu.Unlock(); m.stopped = true; return nil }
func (m *mockOCService) SubmitTask(_ context.Context, _ int64, prompt string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks = append(m.tasks, prompt)
	return nil
}

// --- mock claudecode.Service ---

type mockCCService struct {
	mu      sync.Mutex
	tasks   []string
	stopped bool
	isAlive bool
}

func (m *mockCCService) Run(_ context.Context) error       { return nil }
func (m *mockCCService) IsAlive(_ context.Context) bool    { m.mu.Lock(); defer m.mu.Unlock(); return m.isAlive }
func (m *mockCCService) Stop(_ context.Context) error      { m.mu.Lock(); defer m.mu.Unlock(); m.stopped = true; return nil }
func (m *mockCCService) SubmitTask(_ context.Context, _ int64, prompt string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks = append(m.tasks, prompt)
	return nil
}

// --- mock LLMProvider ---

type mockLLM struct {
	mu        sync.Mutex
	responses []*providers.LLMResponse
	callCount int
}

func (m *mockLLM) Name() string { return "mock" }

func (m *mockLLM) Chat(_ context.Context, _ []providers.Message, _ []providers.Tool) (*providers.LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.callCount < len(m.responses) {
		resp := m.responses[m.callCount]
		m.callCount++
		return resp, nil
	}
	return &providers.LLMResponse{Content: "fallback answer"}, nil
}

func (m *mockLLM) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// --- mock SearchProvider ---

type mockSearch struct{}

func (m *mockSearch) Search(_ context.Context, query string, _ int) ([]tools.SearchResult, error) {
	return []tools.SearchResult{{Title: "result", URL: "https://example.com", Snippet: "snippet for " + query}}, nil
}

// --- mock WebFetcher ---

type mockFetcher struct{}

func (m *mockFetcher) Fetch(_ context.Context, url string) (string, error) {
	return "content of " + url, nil
}

// --- helpers ---

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
	if err != nil {
		t.Fatalf("newTestStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func newTestScheduler(t *testing.T, s *store.Store) *scheduler.Service {
	t.Helper()
	return scheduler.NewService(s, &noopJobTarget{}, config.Tuning{
		SchedulerTick:       time.Second,
		MissedJobsFireLimit: 5,
	})
}

type noopJobTarget struct{}

func (n *noopJobTarget) RunAgentTask(_ context.Context, _ agent.Kind, _ string) error { return nil }
func (n *noopJobTarget) SendChat(_ context.Context, _ int64, _ string) error          { return nil }

func newService(t *testing.T, ch channel.Channel, llm providers.LLMProvider) *gateway.Service {
	t.Helper()
	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{
		AllowedUserIDs: []int64{42},
		Tuning: config.Tuning{
			SchedulerTick:       time.Second,
			MissedJobsFireLimit: 5,
		},
	}
	return gateway.NewService(
		ch,
		&mockApprover{},
		&mockOCService{isAlive: false},
		&mockCCService{isAlive: false},
		llm,
		&mockSearch{},
		&mockFetcher{},
		mcp.NewManager(nil), // empty MCP manager
		sched,
		s,
		cfg,
	)
}

// ============================================================
// Tests
// ============================================================

// TestGateway_AllowedUserCheck verifies messages from disallowed users are silently dropped.
func TestGateway_AllowedUserCheck(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{}
	svc := newService(t, ch, llm)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	go svc.Run(ctx) //nolint:errcheck

	// Disallowed user
	ch.inbound <- channel.InboundMessage{ChatID: 99, UserID: 99, Text: "hello"}
	time.Sleep(150 * time.Millisecond)

	if msgs := ch.sentMessages(); len(msgs) != 0 {
		t.Errorf("expected 0 messages for disallowed user, got %d: %v", len(msgs), msgs)
	}
	if llm.calls() != 0 {
		t.Errorf("expected 0 LLM calls for disallowed user, got %d", llm.calls())
	}
}

// TestGateway_OCCommand verifies /oc routes to opencode.SubmitTask.
func TestGateway_OCCommand(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{}
	oc := &mockOCService{}
	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{AllowedUserIDs: []int64{42}}
	svc := gateway.NewService(ch, &mockApprover{}, oc, &mockCCService{}, llm,
		&mockSearch{}, &mockFetcher{}, mcp.NewManager(nil), sched, s, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "/oc build the auth module"}
	time.Sleep(150 * time.Millisecond)

	oc.mu.Lock()
	tasks := oc.tasks
	oc.mu.Unlock()
	if len(tasks) != 1 || tasks[0] != "build the auth module" {
		t.Errorf("expected SubmitTask(\"build the auth module\"), got %v", tasks)
	}
}

// TestGateway_CCCommand verifies /cc routes to claudecode.SubmitTask.
func TestGateway_CCCommand(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{}
	cc := &mockCCService{}
	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{AllowedUserIDs: []int64{42}}
	svc := gateway.NewService(ch, &mockApprover{}, &mockOCService{}, cc, llm,
		&mockSearch{}, &mockFetcher{}, mcp.NewManager(nil), sched, s, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "/cc refactor service layer"}
	time.Sleep(150 * time.Millisecond)

	cc.mu.Lock()
	tasks := cc.tasks
	cc.mu.Unlock()
	if len(tasks) != 1 || tasks[0] != "refactor service layer" {
		t.Errorf("expected SubmitTask(\"refactor service layer\"), got %v", tasks)
	}
}

// TestGateway_StopCommand verifies /stop calls Stop on both agent services and sends confirmation.
func TestGateway_StopCommand(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{}
	oc := &mockOCService{}
	cc := &mockCCService{}
	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{AllowedUserIDs: []int64{42}}
	svc := gateway.NewService(ch, &mockApprover{}, oc, cc, llm,
		&mockSearch{}, &mockFetcher{}, mcp.NewManager(nil), sched, s, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "/stop"}
	time.Sleep(150 * time.Millisecond)

	oc.mu.Lock()
	ocStopped := oc.stopped
	oc.mu.Unlock()
	cc.mu.Lock()
	ccStopped := cc.stopped
	cc.mu.Unlock()

	if !ocStopped {
		t.Error("expected opencode.Stop() to be called")
	}
	if !ccStopped {
		t.Error("expected claudecode.Stop() to be called")
	}
	msgs := ch.sentMessages()
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "Stopped") || strings.Contains(m, "⏹") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected stop confirmation message, got %v", msgs)
	}
}

// TestGateway_StatusCommand verifies /status sends a status message.
func TestGateway_StatusCommand(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{}
	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{AllowedUserIDs: []int64{42}}
	svc := gateway.NewService(ch, &mockApprover{}, &mockOCService{}, &mockCCService{}, llm,
		&mockSearch{}, &mockFetcher{}, mcp.NewManager(nil), sched, s, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "/status"}
	time.Sleep(150 * time.Millisecond)

	msgs := ch.sentMessages()
	if len(msgs) == 0 {
		t.Fatal("expected /status response, got none")
	}
	// Status must contain expected sections
	status := strings.Join(msgs, "\n")
	for _, want := range []string{"OpenCode", "ClaudeCode", "HITL", "Scheduled"} {
		if !strings.Contains(status, want) {
			t.Errorf("/status missing %q; got:\n%s", want, status)
		}
	}
}

// TestGateway_PlainChat_DirectAnswer verifies a plain chat message that needs no tools returns LLM answer.
func TestGateway_PlainChat_DirectAnswer(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{
		responses: []*providers.LLMResponse{
			{Content: "Go was released in 2009.", ToolCall: nil, Usage: providers.Usage{}},
		},
	}
	svc := newService(t, ch, llm)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "when was Go released?"}
	time.Sleep(300 * time.Millisecond)

	msgs := ch.sentMessages()
	if len(msgs) == 0 {
		t.Fatal("expected plain chat response, got none")
	}
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "2009") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected LLM answer to contain '2009'; got: %v", msgs)
	}
	if llm.calls() != 1 {
		t.Errorf("expected 1 LLM call, got %d", llm.calls())
	}
}

// TestGateway_PlainChat_ToolLoop verifies a plain chat message that calls web_search then answers.
func TestGateway_PlainChat_ToolLoop(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{
		responses: []*providers.LLMResponse{
			{
				Content: "",
				ToolCall: &providers.ToolCall{
					ID:    "call-1",
					Name:  "web_search",
					Input: map[string]any{"query": "latest Go release", "count": 5},
				},
				Usage: providers.Usage{},
			},
			{Content: "The latest Go version is 1.25.", ToolCall: nil, Usage: providers.Usage{}},
		},
	}
	svc := newService(t, ch, llm)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "what is the latest Go version?"}
	time.Sleep(300 * time.Millisecond)

	if llm.calls() != 2 {
		t.Errorf("expected 2 LLM calls (tool + answer), got %d", llm.calls())
	}
	msgs := ch.sentMessages()
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "1.25") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected final answer to contain '1.25'; got: %v", msgs)
	}
}

// TestGateway_DoomLoopGuard verifies the doom-loop guard:
// LLM returns the same tool call 3 times → forced final answer on call 4.
// Total LLM calls must be exactly 4.
func TestGateway_DoomLoopGuard(t *testing.T) {
	ch := newMockChannel()

	sameToolCall := &providers.ToolCall{
		ID:    "call-dup",
		Name:  "web_search",
		Input: map[string]any{"query": "Go programming", "count": 5},
	}
	finalAnswer := "Here is the answer after detecting tool loop."

	llm := &mockLLM{
		responses: []*providers.LLMResponse{
			// calls 1, 2, 3: same tool call (doom-loop)
			{Content: "", ToolCall: sameToolCall, Usage: providers.Usage{}},
			{Content: "", ToolCall: sameToolCall, Usage: providers.Usage{}},
			{Content: "", ToolCall: sameToolCall, Usage: providers.Usage{}},
			// call 4: forced final answer (LLM called without tools after guard triggers)
			{Content: finalAnswer, ToolCall: nil, Usage: providers.Usage{}},
		},
	}
	svc := newService(t, ch, llm)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "tell me about Go"}
	time.Sleep(600 * time.Millisecond)

	// Must have made exactly 4 LLM calls
	if calls := llm.calls(); calls != 4 {
		t.Errorf("doom-loop: expected 4 LLM calls, got %d", calls)
	}

	// Final message sent to user must be the forced final answer
	msgs := ch.sentMessages()
	found := false
	for _, m := range msgs {
		if strings.Contains(m, finalAnswer) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected forced final answer to be sent; got: %v", msgs)
	}
}

// TestGateway_HITLCallback verifies callback data "hitl:<id>:<action>" is dispatched to HITL handling.
// This test verifies gateway does NOT forward callback to LLM.
func TestGateway_HITLCallback(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{}
	svc := newService(t, ch, llm)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{
		ChatID:       42,
		UserID:       42,
		Text:         "",
		CallbackData: "hitl:permission_abc123:once",
	}
	time.Sleep(150 * time.Millisecond)

	// LLM should NOT be called for HITL callbacks
	if llm.calls() != 0 {
		t.Errorf("expected 0 LLM calls for HITL callback, got %d", llm.calls())
	}
}

// TestGateway_ScheduleJobTool verifies the schedule_job tool creates a job via scheduler.
func TestGateway_ScheduleJobTool(t *testing.T) {
	ch := newMockChannel()

	llm := &mockLLM{
		responses: []*providers.LLMResponse{
			{
				Content: "",
				ToolCall: &providers.ToolCall{
					ID:   "call-sched",
					Name: "schedule_job",
					Input: map[string]any{
						"kind":     "every",
						"target":   "opencode",
						"prompt":   "run tests",
						"schedule": "3600",
					},
				},
				Usage: providers.Usage{},
			},
			{Content: "Job scheduled successfully.", ToolCall: nil, Usage: providers.Usage{}},
		},
	}
	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{AllowedUserIDs: []int64{42}}
	svc := gateway.NewService(ch, &mockApprover{}, &mockOCService{}, &mockCCService{}, llm,
		&mockSearch{}, &mockFetcher{}, mcp.NewManager(nil), sched, s, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "schedule a job to run tests every hour"}
	time.Sleep(300 * time.Millisecond)

	// Job must appear in store
	rows, err := s.ListAllJobs()
	if err != nil {
		t.Fatalf("ListAllJobs: %v", err)
	}
	if len(rows) == 0 {
		t.Error("expected schedule_job tool to create a job in store; got none")
	}
}

// TestGateway_LLMError verifies gateway sends an error message if LLM returns an error.
func TestGateway_LLMError(t *testing.T) {
	ch := newMockChannel()
	llm := &mockLLM{}
	// Inject a failing LLM via custom implementation
	failLLM := &failingLLM{err: errors.New("LLM unavailable")}
	s := newTestStore(t)
	sched := newTestScheduler(t, s)
	cfg := config.Config{AllowedUserIDs: []int64{42}}
	svc := gateway.NewService(ch, &mockApprover{}, &mockOCService{}, &mockCCService{}, failLLM,
		&mockSearch{}, &mockFetcher{}, mcp.NewManager(nil), sched, s, cfg)
	_ = llm // unused; suppress warning

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go svc.Run(ctx) //nolint:errcheck

	ch.inbound <- channel.InboundMessage{ChatID: 42, UserID: 42, Text: "anything"}
	time.Sleep(150 * time.Millisecond)

	msgs := ch.sentMessages()
	if len(msgs) == 0 {
		t.Fatal("expected error message to be sent, got none")
	}
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "⚠️") || strings.Contains(m, "error") || strings.Contains(m, "Error") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ⚠️ error message; got: %v", msgs)
	}
}

type failingLLM struct {
	err error
}

func (f *failingLLM) Name() string { return "failing" }
func (f *failingLLM) Chat(_ context.Context, _ []providers.Message, _ []providers.Tool) (*providers.LLMResponse, error) {
	return nil, f.err
}
```

### Step 2: Run to verify tests fail

```bash
go test ./internal/gateway/... -v
```

Expected: compile error — package `internal/gateway` does not exist.

### Step 3: Implement `internal/gateway/service.go`

> **Before implementing:** check exact method signatures from previous plans:
> - `opencode.Service` interface from Plan 6 (`internal/opencode/service.go`)
> - `claudecode.Service` interface from Plan 6 (`internal/claudecode/service.go`)
> - `hitl.Approver` interface from Plan 5 (`internal/hitl/types.go` or `service.go`)
> - `mcp.MCPManager.GetAllTools()` return type from Plan 4
> - `tools.SearchProvider.Search()` return type from Plan 4
> - `tools.WebFetcher.Fetch()` signature from Plan 4
> - `providers.Tool`, `providers.Message`, `providers.ToolCall` from Plan 3
> - `channel.Channel` interface from Plan 5
> - `config.Config` struct from Plan 1
>
> Adjust import paths and method calls if the actual signatures differ from what is assumed here.

```go
// internal/gateway/service.go
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/channel"
	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/hitl"
	"github.com/canhta/gistclaw/internal/mcp"
	"github.com/canhta/gistclaw/internal/providers"
	"github.com/canhta/gistclaw/internal/scheduler"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

// ocService abstracts opencode.Service to avoid circular imports in tests.
type ocService interface {
	SubmitTask(ctx context.Context, chatID int64, prompt string) error
	Stop(ctx context.Context) error
	IsAlive(ctx context.Context) bool
}

// ccService abstracts claudecode.Service.
type ccService interface {
	SubmitTask(ctx context.Context, chatID int64, prompt string) error
	Stop(ctx context.Context) error
	IsAlive(ctx context.Context) bool
}

// Service is the channel-agnostic gateway controller.
type Service struct {
	ch         channel.Channel
	hitl       hitl.Approver
	opencode   ocService
	claudecode ccService
	llm        providers.LLMProvider
	search     tools.SearchProvider   // may be nil
	fetcher    tools.WebFetcher
	mcp        *mcp.Manager           // adjust type name to match Plan 4 impl
	sched      *scheduler.Service
	store      *store.Store
	cfg        config.Config
}

// NewService creates a new gateway Service.
func NewService(
	ch channel.Channel,
	h hitl.Approver,
	oc ocService,
	cc ccService,
	llm providers.LLMProvider,
	search tools.SearchProvider,
	fetcher tools.WebFetcher,
	m *mcp.Manager,
	sched *scheduler.Service,
	st *store.Store,
	cfg config.Config,
) *Service {
	return &Service{
		ch:         ch,
		hitl:       h,
		opencode:   oc,
		claudecode: cc,
		llm:        llm,
		search:     search,
		fetcher:    fetcher,
		mcp:        m,
		sched:      sched,
		store:      st,
		cfg:        cfg,
	}
}

// Run starts the gateway message loop. Blocks until ctx is cancelled.
func (s *Service) Run(ctx context.Context) error {
	msgs, err := s.ch.Receive(ctx)
	if err != nil {
		return fmt.Errorf("gateway: Receive: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			s.handle(ctx, msg)
		}
	}
}

// handle processes a single inbound message.
func (s *Service) handle(ctx context.Context, msg channel.InboundMessage) {
	// User ID whitelist check
	if !s.isAllowed(msg.ChatID) {
		log.Debug().Int64("chat_id", msg.ChatID).Msg("gateway: message from disallowed user; dropped")
		return
	}

	// HITL callback
	if msg.CallbackData != "" {
		s.handleCallback(ctx, msg)
		return
	}

	// Command routing
	text := strings.TrimSpace(msg.Text)
	switch {
	case strings.HasPrefix(text, "/oc "):
		prompt := strings.TrimPrefix(text, "/oc ")
		if err := s.opencode.SubmitTask(ctx, msg.ChatID, prompt); err != nil {
			log.Error().Err(err).Msg("gateway: opencode.SubmitTask")
			_ = s.ch.SendMessage(ctx, msg.ChatID, "⚠️ OpenCode error: "+err.Error())
		}
	case strings.HasPrefix(text, "/cc "):
		prompt := strings.TrimPrefix(text, "/cc ")
		if err := s.claudecode.SubmitTask(ctx, msg.ChatID, prompt); err != nil {
			log.Error().Err(err).Msg("gateway: claudecode.SubmitTask")
			_ = s.ch.SendMessage(ctx, msg.ChatID, "⚠️ ClaudeCode error: "+err.Error())
		}
	case text == "/stop":
		_ = s.opencode.Stop(ctx)
		_ = s.claudecode.Stop(ctx)
		_ = s.ch.SendMessage(ctx, msg.ChatID, "⏹ Stopped.")
	case text == "/status":
		_ = s.ch.SendMessage(ctx, msg.ChatID, s.buildStatus(ctx))
	default:
		// Plain chat
		s.handlePlainChat(ctx, msg.ChatID, text)
	}
}

// isAllowed checks if the chatID is in the allowed list.
func (s *Service) isAllowed(chatID int64) bool {
	for _, id := range s.cfg.AllowedUserIDs {
		if id == chatID {
			return true
		}
	}
	return false
}

// handleCallback dispatches HITL callback queries.
// Expected format: "hitl:<id>:<action>"
func (s *Service) handleCallback(ctx context.Context, msg channel.InboundMessage) {
	data := msg.CallbackData
	if !strings.HasPrefix(data, "hitl:") {
		log.Warn().Str("callback_data", data).Msg("gateway: unknown callback prefix; ignored")
		return
	}
	// Parse: hitl:<id>:<action>
	parts := strings.SplitN(strings.TrimPrefix(data, "hitl:"), ":", 2)
	if len(parts) != 2 {
		log.Warn().Str("callback_data", data).Msg("gateway: malformed hitl callback; ignored")
		return
	}
	id, action := parts[0], parts[1]
	log.Debug().Str("hitl_id", id).Str("action", action).Msg("gateway: HITL callback received")
	// HITL resolution is handled by hitl.Service internally via the pending map.
	// Gateway signals by resolving the pending request through the HITL approver.
	// The hitl.Service stores a map of id → decision channel; resolution is via a separate
	// method. For v1, we use a type assertion to access the concrete *hitl.Service Resolve method
	// if available. If the Approver does not expose Resolve, log and drop.
	type resolver interface {
		Resolve(id string, action string) error
	}
	if r, ok := s.hitl.(resolver); ok {
		if err := r.Resolve(id, action); err != nil {
			log.Warn().Str("hitl_id", id).Err(err).Msg("gateway: HITL Resolve failed")
		}
	} else {
		log.Debug().Str("hitl_id", id).Msg("gateway: HITL approver does not implement Resolve; callback dropped")
	}
}

// handlePlainChat runs the multi-round LLM tool loop for non-command messages.
func (s *Service) handlePlainChat(ctx context.Context, chatID int64, text string) {
	_ = s.ch.SendTyping(ctx, chatID)

	msgs := []providers.Message{
		{Role: "user", Content: text},
	}
	toolRegistry := s.buildToolRegistry()

	const doomLoopMax = 3
	type callSig struct{ name, input string }
	lastCalls := make([]callSig, 0, doomLoopMax)
	identicalCount := 0

	for {
		resp, err := s.llm.Chat(ctx, msgs, toolRegistry)
		if err != nil {
			log.Error().Err(err).Msg("gateway: LLM Chat error")
			_ = s.ch.SendMessage(ctx, chatID, "⚠️ LLM error: "+err.Error())
			return
		}

		// No tool call → send final answer and exit loop
		if resp.ToolCall == nil {
			_ = s.ch.SendMessage(ctx, chatID, resp.Content)
			return
		}

		tc := resp.ToolCall

		// Doom-loop guard: track consecutive identical tool calls
		inputJSON, _ := json.Marshal(tc.Input)
		sig := callSig{name: tc.Name, input: string(inputJSON)}
		if len(lastCalls) >= doomLoopMax {
			lastCalls = lastCalls[1:]
		}
		lastCalls = append(lastCalls, sig)

		if len(lastCalls) == doomLoopMax {
			identical := true
			for _, c := range lastCalls[1:] {
				if c != lastCalls[0] {
					identical = false
					break
				}
			}
			if identical {
				identicalCount++
				if identicalCount >= 1 {
					log.Warn().Str("tool", tc.Name).Msg("gateway: doom-loop detected; forcing final answer")
					// Inject guard message and call LLM one final time without tools
					msgs = append(msgs, providers.Message{
						Role:    "tool",
						Content: "[Tool call loop detected. Provide your best answer now.]",
						ToolCallID: tc.ID,
					})
					finalResp, ferr := s.llm.Chat(ctx, msgs, nil)
					if ferr != nil {
						_ = s.ch.SendMessage(ctx, chatID, "⚠️ LLM error after doom-loop guard: "+ferr.Error())
						return
					}
					_ = s.ch.SendMessage(ctx, chatID, finalResp.Content)
					return
				}
			} else {
				identicalCount = 0
			}
		}

		// Execute tool call
		toolResult := s.executeTool(ctx, tc)

		// Append assistant message with tool call, then tool result
		msgs = append(msgs,
			providers.Message{
				Role:       "assistant",
				ToolCall:   tc,
			},
			providers.Message{
				Role:       "tool",
				Content:    truncateToolResult(toolResult),
				ToolCallID: tc.ID,
			},
		)
	}
}

// executeTool dispatches a tool call and returns the result as a string.
func (s *Service) executeTool(ctx context.Context, tc *providers.ToolCall) string {
	switch tc.Name {
	case "web_search":
		return s.execWebSearch(ctx, tc.Input)
	case "web_fetch":
		return s.execWebFetch(ctx, tc.Input)
	case "schedule_job":
		return s.execScheduleJob(tc.Input)
	case "list_jobs":
		return s.execListJobs()
	case "update_job":
		return s.execUpdateJob(tc.Input)
	case "delete_job":
		return s.execDeleteJob(tc.Input)
	default:
		// MCP tool: format is "{server}__{tool}" (double underscore)
		if strings.Contains(tc.Name, "__") {
			return s.execMCPTool(ctx, tc)
		}
		return fmt.Sprintf("unknown tool: %q", tc.Name)
	}
}

func (s *Service) execWebSearch(ctx context.Context, input map[string]any) string {
	if s.search == nil {
		return "web_search is not configured (no search API key)"
	}
	query, _ := input["query"].(string)
	count := 5
	if c, ok := input["count"].(float64); ok {
		count = int(c)
	}
	results, err := s.search.Search(ctx, query, count)
	if err != nil {
		return "Search failed: " + err.Error()
	}
	var sb strings.Builder
	for i, r := range results {
		fmt.Fprintf(&sb, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet)
	}
	return sb.String()
}

func (s *Service) execWebFetch(ctx context.Context, input map[string]any) string {
	url, _ := input["url"].(string)
	if url == "" {
		return "web_fetch: 'url' parameter required"
	}
	content, err := s.fetcher.Fetch(ctx, url)
	if err != nil {
		return "web_fetch error: " + err.Error()
	}
	return content
}

func (s *Service) execScheduleJob(input map[string]any) string {
	kindStr, _ := input["kind"].(string)
	targetStr, _ := input["target"].(string)
	prompt, _ := input["prompt"].(string)
	schedule, _ := input["schedule"].(string)

	targetKind, err := agentKindFromString(targetStr)
	if err != nil {
		return "schedule_job error: invalid target: " + err.Error()
	}

	j := scheduler.Job{
		Kind:     kindStr,
		Target:   targetKind,
		Prompt:   prompt,
		Schedule: schedule,
	}
	if err := s.sched.CreateJob(j); err != nil {
		return "schedule_job error: " + err.Error()
	}
	return `{"status":"ok","message":"Job scheduled successfully."}`
}

func (s *Service) execListJobs() string {
	jobs, err := s.sched.ListJobs()
	if err != nil {
		return "list_jobs error: " + err.Error()
	}
	if len(jobs) == 0 {
		return "[]"
	}
	type item struct {
		ID        string `json:"id"`
		Kind      string `json:"kind"`
		Target    string `json:"target"`
		Prompt    string `json:"prompt"`
		Schedule  string `json:"schedule"`
		NextRunAt string `json:"next_run_at"`
		Enabled   bool   `json:"enabled"`
	}
	items := make([]item, len(jobs))
	for i, j := range jobs {
		items[i] = item{
			ID:        j.ID,
			Kind:      j.Kind,
			Target:    j.Target.String(),
			Prompt:    j.Prompt,
			Schedule:  j.Schedule,
			NextRunAt: j.NextRunAt.Format(time.RFC3339),
			Enabled:   j.Enabled,
		}
	}
	b, _ := json.MarshalIndent(items, "", "  ")
	return string(b)
}

func (s *Service) execUpdateJob(input map[string]any) string {
	id, _ := input["id"].(string)
	if id == "" {
		return "update_job error: 'id' parameter required"
	}
	fields := make(map[string]any)
	if v, ok := input["enabled"]; ok {
		fields["enabled"] = v
	}
	if err := s.sched.UpdateJob(id, fields); err != nil {
		return "update_job error: " + err.Error()
	}
	return `{"status":"ok"}`
}

func (s *Service) execDeleteJob(input map[string]any) string {
	id, _ := input["id"].(string)
	if id == "" {
		return "delete_job error: 'id' parameter required"
	}
	if err := s.sched.DeleteJob(id); err != nil {
		return "delete_job error: " + err.Error()
	}
	return `{"status":"ok"}`
}

func (s *Service) execMCPTool(ctx context.Context, tc *providers.ToolCall) string {
	result, err := s.mcp.CallTool(ctx, tc.Name, tc.Input)
	if err != nil {
		return "MCP tool error: " + err.Error()
	}
	return result
}

// buildToolRegistry assembles the three-category tool registry.
func (s *Service) buildToolRegistry() []providers.Tool {
	var registry []providers.Tool

	// Core — always present if configured
	if s.search != nil {
		registry = append(registry, webSearchTool())
	}
	registry = append(registry, webFetchTool())

	// Agent — MCP-derived, namespaced {server}__{tool}
	for _, t := range s.mcp.GetAllTools() {
		registry = append(registry, providers.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}

	// System — scheduler control
	registry = append(registry, s.sched.Tools()...)

	return registry
}

// buildStatus formats the /status response.
func (s *Service) buildStatus(ctx context.Context) string {
	var sb strings.Builder
	sb.WriteString("GistClaw status\n")

	ocStatus := "idle"
	if s.opencode.IsAlive(ctx) {
		ocStatus = "running"
	}
	fmt.Fprintf(&sb, "OpenCode: %s\n", ocStatus)

	ccStatus := "idle"
	if s.claudecode.IsAlive(ctx) {
		ccStatus = "running"
	}
	fmt.Fprintf(&sb, "ClaudeCode: %s\n", ccStatus)

	// HITL pending count
	hitlCount := 0 // would query store in full impl
	fmt.Fprintf(&sb, "HITL pending: %d\n", hitlCount)

	// Scheduled jobs
	jobs, _ := s.sched.ListJobs()
	activeCount := 0
	var nextRun time.Time
	for _, j := range jobs {
		if j.Enabled {
			activeCount++
			if nextRun.IsZero() || j.NextRunAt.Before(nextRun) {
				nextRun = j.NextRunAt
			}
		}
	}
	if activeCount > 0 && !nextRun.IsZero() {
		diff := time.Until(nextRun).Round(time.Minute)
		fmt.Fprintf(&sb, "Scheduled jobs: %d active (next: in %v)\n", activeCount, diff)
	} else {
		fmt.Fprintf(&sb, "Scheduled jobs: %d active\n", activeCount)
	}

	// MCP servers
	servers := s.mcp.ServerStatus()
	if len(servers) > 0 {
		sb.WriteString("MCP servers:")
		for name, ok := range servers {
			if ok {
				fmt.Fprintf(&sb, " %s ✓", name)
			} else {
				fmt.Fprintf(&sb, " %s ✗ (failed)", name)
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// --- tool definitions ---

func webSearchTool() providers.Tool {
	return providers.Tool{
		Name:        "web_search",
		Description: "Search the web for current information. Returns ranked results with titles, URLs, and snippets.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The search query",
				},
				"count": map[string]any{
					"type":        "integer",
					"description": "Number of results (default 5, max 10)",
					"default":     5,
				},
			},
			"required": []string{"query"},
		},
	}
}

func webFetchTool() providers.Tool {
	return providers.Tool{
		Name:        "web_fetch",
		Description: "Fetch and extract readable content from a URL. Returns markdown text. Truncated at 50KB / 2000 lines.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "The URL to fetch",
				},
			},
			"required": []string{"url"},
		},
	}
}

// --- helpers ---

// truncateToolResult truncates a tool result to 50KB / 2000 lines, whichever is smaller.
func truncateToolResult(s string) string {
	const maxBytes = 50 * 1024
	const maxLines = 2000

	lines := strings.SplitN(s, "\n", maxLines+1)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		s = strings.Join(lines, "\n") + "\n[truncated: exceeded 2000 lines]"
	}

	if len(s) > maxBytes {
		s = s[:maxBytes] + "\n[truncated: exceeded 50KB]"
	}

	return s
}

// agentKindFromString converts an LLM-supplied string to agent.Kind.
// Imported indirectly to avoid circular dependency.
func agentKindFromString(s string) (agentKind, error) {
	// Use the agent package via scheduler.Job.Target field.
	// Import agent directly.
	return agentKindFromStringImpl(s)
}
```

> **Important:** The `agentKindFromString` helper needs to import `internal/agent`. Add the import
> and replace the stub at the bottom with a direct call:
>
> ```go
> import "github.com/canhta/gistclaw/internal/agent"
>
> // In execScheduleJob:
> targetKind, err := agent.KindFromString(targetStr)
> ```
>
> Remove the `agentKindFromString` wrapper and `agentKind` type entirely — they are
> scaffolding artifacts. Import `agent` directly.
>
> Similarly, check `mcp.Manager` type name (it may be `*mcp.MCPManager` from Plan 4 —
> adjust the field and constructor parameter type accordingly).
> Check `mcp.Manager.GetAllTools()` return type and `mcp.Manager.CallTool()` signature.
> Check `mcp.Manager.ServerStatus()` — if it does not exist, return an empty map or add it to Plan 4.
>
> Check `providers.Message` struct fields — particularly `ToolCall` and `ToolCallID` —
> against what Plan 3 actually implemented. Adjust accordingly.

### Step 4: Clean up the implementation

After writing the initial file, verify it compiles:

```bash
go build ./internal/gateway/...
```

Fix any type mismatches by checking actual interface definitions from Plans 3–6:

```bash
# Check actual types
grep -n "type Message struct" internal/providers/llm.go
grep -n "type ToolCall struct" internal/providers/llm.go
grep -n "type Tool struct" internal/providers/llm.go
grep -n "type Manager struct\|type MCPManager struct" internal/mcp/manager.go
grep -n "func.*GetAllTools\|func.*CallTool\|func.*ServerStatus" internal/mcp/manager.go
grep -n "type SearchResult struct" internal/tools/search.go
grep -n "type WebFetcher interface\|Fetch(" internal/tools/fetch.go
```

Adjust the implementation to match actual types. Common fixes:
- `mcp.Manager` → `mcp.MCPManager` (or whatever Plan 4 named it)
- `providers.Message.ToolCall` → may be `ToolCallID string` + separate ToolCall field
- `tools.SearchResult` fields (`Title`, `URL`, `Snippet`) → check actual field names
- `hitl.Approver` → check actual interface method signatures

### Step 5: Run tests

```bash
go test ./internal/gateway/... -v
```

Expected: all tests PASS.

### Step 6: Run full test suite

```bash
go test ./...
```

Expected: all tests PASS.

### Step 7: Commit

```bash
git add internal/gateway/
git commit -m "feat(gateway): add gateway.Service with command routing, plain-chat tool loop, doom-loop guard, and HITL callback dispatch"
```

---

## Final verification

After all three tasks are complete, run the full test suite one final time:

```bash
go test ./... -count=1
```

And verify the binary still builds:

```bash
go build ./...
```

Expected: zero errors, zero test failures.

---

Plan 7 complete. Next: Plan 8 (App Wiring).
