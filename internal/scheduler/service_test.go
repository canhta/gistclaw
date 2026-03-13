// internal/scheduler/service_test.go
package scheduler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
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
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
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

// operatorChatID used in tests
const testOperatorChatID int64 = 42

// --- tests ---

// TestScheduler_EveryJob verifies an "every" job fires when its next_run_at is reached.
func TestScheduler_EveryJob(t *testing.T) {
	s := newTestStore(t)
	target := &mockTarget{}
	tuning := defaultTuning()
	svc := scheduler.NewService(s, target, tuning, testOperatorChatID)

	now := time.Now().UTC()
	row := store.JobRow{
		ID:        "every-001",
		Kind:      "every",
		Target:    "opencode",
		Prompt:    "run tests",
		Schedule:  "1",                   // 1 second
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

// TestScheduler_AtJob verifies an "at" job fires once then is deleted.
func TestScheduler_AtJob(t *testing.T) {
	s := newTestStore(t)
	target := &mockTarget{}
	svc := scheduler.NewService(s, target, defaultTuning(), testOperatorChatID)

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

// TestScheduler_CronJob verifies a cron job fires and next_run_at advances.
func TestScheduler_CronJob(t *testing.T) {
	s := newTestStore(t)
	target := &mockTarget{}
	svc := scheduler.NewService(s, target, defaultTuning(), testOperatorChatID)

	now := time.Now().UTC()
	row := store.JobRow{
		ID:        "cron-001",
		Kind:      "cron",
		Target:    "claudecode",
		Prompt:    "check status",
		Schedule:  "* * * * *",           // every minute
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

// TestScheduler_MissedJobsOnStartup verifies overdue jobs are fired immediately (up to limit).
func TestScheduler_MissedJobsOnStartup(t *testing.T) {
	s := newTestStore(t)
	target := &mockTarget{}
	tuning := config.Tuning{
		SchedulerTick:       100 * time.Millisecond,
		MissedJobsFireLimit: 2, // fire only 2 immediately; rest advance
	}
	svc := scheduler.NewService(s, target, tuning, testOperatorChatID)

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
	svc := scheduler.NewService(s, target, defaultTuning(), testOperatorChatID)

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
	target.mu.Lock()
	chatIDs := target.chatIDs
	target.mu.Unlock()
	if len(chatIDs) < 1 {
		t.Errorf("expected >=1 SendChat call for KindChat job, got 0")
	}
}

// TestScheduler_Tools verifies Tools() returns exactly 4 tools with correct names.
func TestScheduler_Tools(t *testing.T) {
	s := newTestStore(t)
	svc := scheduler.NewService(s, &mockTarget{}, defaultTuning(), testOperatorChatID)

	tools := svc.Tools()
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}

	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
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

// TestScheduler_CRUD verifies CreateJob / ListJobs / UpdateJob / DeleteJob.
func TestScheduler_CRUD(t *testing.T) {
	s := newTestStore(t)
	svc := scheduler.NewService(s, &mockTarget{}, defaultTuning(), testOperatorChatID)

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
	svc := scheduler.NewService(s, &mockTarget{}, defaultTuning(), testOperatorChatID)
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
	svc := scheduler.NewService(s, &mockTarget{}, defaultTuning(), testOperatorChatID)
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

// TestSchedulerKindIn_NextRunAt verifies kind="in" computes next_run_at as now+schedule seconds,
// sets delete_after_run=true, and that CreateJob accepts it.
func TestSchedulerKindIn_NextRunAt(t *testing.T) {
	s := newTestStore(t)
	svc := scheduler.NewService(s, &mockTarget{}, defaultTuning(), testOperatorChatID)

	before := time.Now().UTC()
	j := scheduler.Job{
		Kind:     "in",
		Target:   agent.KindGateway,
		Prompt:   "search for gold price",
		Schedule: "300", // 5 minutes
	}
	if err := svc.CreateJob(j); err != nil {
		t.Fatalf("CreateJob kind=in: %v", err)
	}
	after := time.Now().UTC()

	jobs, err := svc.ListJobs()
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	got := jobs[0]

	// next_run_at must be within [before+300s, after+300s] (±2s slack)
	wantLow := before.Add(300 * time.Second).Add(-2 * time.Second)
	wantHigh := after.Add(300 * time.Second).Add(2 * time.Second)
	if got.NextRunAt.Before(wantLow) || got.NextRunAt.After(wantHigh) {
		t.Errorf("kind=in NextRunAt=%v; want between %v and %v", got.NextRunAt, wantLow, wantHigh)
	}

	// delete_after_run must be true (one-shot)
	if !got.DeleteAfterRun {
		t.Errorf("kind=in: expected DeleteAfterRun=true, got false")
	}
}

// TestSchedulerKindIn_Fires verifies a kind="in" job fires exactly once and is deleted.
func TestSchedulerKindIn_Fires(t *testing.T) {
	s := newTestStore(t)
	target := &mockTarget{}
	svc := scheduler.NewService(s, target, defaultTuning(), testOperatorChatID)

	now := time.Now().UTC()
	row := store.JobRow{
		ID:             "in-001",
		Kind:           "in",
		Target:         "gateway",
		Prompt:         "check prices",
		Schedule:       "300",
		NextRunAt:      now.Add(-time.Second), // already due
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

	// RunAgentTask must be called exactly once
	if count := target.callCount(); count != 1 {
		t.Errorf("kind=in: expected exactly 1 RunAgentTask call, got %d", count)
	}

	// Job must be deleted from store
	rows, err := s.ListAllJobs()
	if err != nil {
		t.Fatalf("ListAllJobs: %v", err)
	}
	for _, r := range rows {
		if r.ID == "in-001" {
			t.Errorf("kind=in job should be deleted after firing; still present")
		}
	}
}

// TestSchedulerKindIn_InvalidSchedule verifies a non-integer schedule string is rejected.
func TestSchedulerKindIn_InvalidSchedule(t *testing.T) {
	s := newTestStore(t)
	svc := scheduler.NewService(s, &mockTarget{}, defaultTuning(), testOperatorChatID)
	j := scheduler.Job{
		Kind:     "in",
		Target:   agent.KindGateway,
		Prompt:   "x",
		Schedule: "not-a-number",
	}
	if err := svc.CreateJob(j); err == nil {
		t.Error("expected error for non-integer kind=in schedule, got nil")
	}
}

// TestScheduleJob_GatewayTargetValidation verifies that CreateJob accepts target=KindGateway
// and that the stored job has target="gateway".
func TestScheduleJob_GatewayTargetValidation(t *testing.T) {
	s := newTestStore(t)
	svc := scheduler.NewService(s, &mockTarget{}, defaultTuning(), testOperatorChatID)

	j := scheduler.Job{
		Kind:     "in",
		Target:   agent.KindGateway,
		Prompt:   "get gold prices",
		Schedule: "300",
	}
	if err := svc.CreateJob(j); err != nil {
		t.Fatalf("CreateJob with KindGateway: %v", err)
	}

	jobs, err := svc.ListJobs()
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Target != agent.KindGateway {
		t.Errorf("stored job Target = %v, want KindGateway (%v)", jobs[0].Target, agent.KindGateway)
	}
}
