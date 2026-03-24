package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
)

// stubStarter records Start calls made to it.
type stubStarter struct {
	calls []runtime.StartRun
}

func (s *stubStarter) Start(_ context.Context, req runtime.StartRun) (model.Run, error) {
	s.calls = append(s.calls, req)
	return model.Run{ID: "sched-run"}, nil
}

func setupSchedulerDB(t *testing.T) (*store.DB, *conversations.ConversationStore) {
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
	return db, cs
}

func insertSchedule(t *testing.T, db *store.DB, id, agentID, objective, cronExpr string, enabled bool) {
	t.Helper()
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err := db.RawDB().Exec(
		`INSERT INTO schedules (id, team_id, agent_id, objective, cron_expr, enabled)
		 VALUES (?, 'team-1', ?, ?, ?, ?)`,
		id, agentID, objective, cronExpr, enabledInt,
	)
	if err != nil {
		t.Fatalf("insert schedule: %v", err)
	}
}

func TestDispatcher_FiresDueSchedule(t *testing.T) {
	db, cs := setupSchedulerDB(t)
	starter := &stubStarter{}
	d := NewDispatcher(db, cs, starter, "")

	// Insert a schedule that matches every minute.
	insertSchedule(t, db, "sched-1", "coordinator", "run the checks", "* * * * *", true)

	now := time.Now().UTC()
	if err := d.tickAt(context.Background(), now); err != nil {
		t.Fatalf("tickAt: %v", err)
	}

	if len(starter.calls) != 1 {
		t.Fatalf("expected 1 Start call, got %d", len(starter.calls))
	}
	call := starter.calls[0]
	if call.Objective != "run the checks" {
		t.Errorf("Objective: got %q, want %q", call.Objective, "run the checks")
	}
	if call.AgentID != "coordinator" {
		t.Errorf("AgentID: got %q, want %q", call.AgentID, "coordinator")
	}
	if call.ConversationID == "" {
		t.Error("ConversationID must be non-empty")
	}
}

func TestDispatcher_DisabledScheduleIsSkipped(t *testing.T) {
	db, cs := setupSchedulerDB(t)
	starter := &stubStarter{}
	d := NewDispatcher(db, cs, starter, "")

	insertSchedule(t, db, "sched-disabled", "coordinator", "should not run", "* * * * *", false)

	if err := d.tickAt(context.Background(), time.Now().UTC()); err != nil {
		t.Fatalf("tickAt: %v", err)
	}
	if len(starter.calls) != 0 {
		t.Fatalf("expected 0 Start calls for disabled schedule, got %d", len(starter.calls))
	}
}

func TestDispatcher_ScheduleNotMatchingCurrentTimeIsSkipped(t *testing.T) {
	db, cs := setupSchedulerDB(t)
	starter := &stubStarter{}
	d := NewDispatcher(db, cs, starter, "")

	// Schedule that only fires at 00:00 on Jan 1.
	insertSchedule(t, db, "sched-jan1", "coordinator", "new year", "0 0 1 1 *", true)

	// Tick at a time that definitely doesn't match.
	notJan1 := time.Date(2025, 6, 15, 12, 30, 0, 0, time.UTC)
	if err := d.tickAt(context.Background(), notJan1); err != nil {
		t.Fatalf("tickAt: %v", err)
	}
	if len(starter.calls) != 0 {
		t.Fatalf("expected 0 Start calls, got %d", len(starter.calls))
	}
}

func TestDispatcher_DoesNotDoubleFire(t *testing.T) {
	db, cs := setupSchedulerDB(t)
	starter := &stubStarter{}
	d := NewDispatcher(db, cs, starter, "")

	insertSchedule(t, db, "sched-once", "coordinator", "once only", "* * * * *", true)

	now := time.Now().UTC()

	if err := d.tickAt(context.Background(), now); err != nil {
		t.Fatalf("first tickAt: %v", err)
	}
	// Tick again at the same minute — should not fire again.
	if err := d.tickAt(context.Background(), now.Add(5*time.Second)); err != nil {
		t.Fatalf("second tickAt: %v", err)
	}

	if len(starter.calls) != 1 {
		t.Fatalf("expected 1 Start call total (no double-fire), got %d", len(starter.calls))
	}
}

func TestDispatcher_RunRespectsCancellation(t *testing.T) {
	db, cs := setupSchedulerDB(t)
	starter := &stubStarter{}
	d := NewDispatcher(db, cs, starter, "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := d.Run(ctx)
	if err == nil {
		t.Fatal("expected Run to return an error when context is cancelled")
	}
}
