package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
)

// RunStarter is the minimal runtime interface required by Dispatcher.
// Defined in the consuming package per the interfaces-in-consuming-package rule.
type RunStarter interface {
	Start(ctx context.Context, req runtime.StartRun) (model.Run, error)
}

// Dispatcher reads enabled schedules from the database, evaluates their cron
// expressions on each tick, and calls rt.Start() when a schedule is due.
//
//	Tick (every minute)
//	  │
//	  ├── query enabled schedules
//	  │
//	  └── for each schedule
//	        ├── parse cron expression
//	        ├── check Matches(now) AND last_run_at is null or > 1 minute ago
//	        └── if due: cs.Resolve → rt.Start → update last_run_at
type Dispatcher struct {
	db            *store.DB
	cs            *conversations.ConversationStore
	rt            RunStarter
	workspaceRoot string
}

// NewDispatcher creates a Dispatcher. workspaceRoot is forwarded to each StartRun.
func NewDispatcher(db *store.DB, cs *conversations.ConversationStore, rt RunStarter, workspaceRoot string) *Dispatcher {
	return &Dispatcher{db: db, cs: cs, rt: rt, workspaceRoot: workspaceRoot}
}

// Run ticks every minute, evaluating and firing due schedules until ctx is
// cancelled. Returns ctx.Err() on cancellation.
func (d *Dispatcher) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Fire once on startup for any schedules due right now.
	_ = d.tickAt(ctx, time.Now().UTC())

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case t := <-ticker.C:
			_ = d.tickAt(ctx, t.UTC())
		}
	}
}

// tickAt evaluates all enabled schedules against t and fires those that are due.
// Exposed for testing; production code calls it via Run.
func (d *Dispatcher) tickAt(ctx context.Context, now time.Time) error {
	rows, err := d.db.RawDB().QueryContext(ctx,
		`SELECT id, agent_id, objective, cron_expr, last_run_at
		 FROM schedules WHERE enabled = 1`,
	)
	if err != nil {
		return fmt.Errorf("scheduler: query schedules: %w", err)
	}
	defer rows.Close()

	type schedRow struct {
		id        string
		agentID   string
		objective string
		cronExpr  string
		lastRunAt *time.Time
	}
	var schedules []schedRow

	for rows.Next() {
		var s schedRow
		var lastRunNT sql.NullTime
		if err := rows.Scan(&s.id, &s.agentID, &s.objective, &s.cronExpr, &lastRunNT); err != nil {
			return fmt.Errorf("scheduler: scan row: %w", err)
		}
		if lastRunNT.Valid {
			t := lastRunNT.Time.UTC()
			s.lastRunAt = &t
		}
		schedules = append(schedules, s)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, s := range schedules {
		cron, err := ParseCron(s.cronExpr)
		if err != nil {
			// Bad cron expression — skip silently but don't fail the whole tick.
			continue
		}
		if !cron.Matches(now) {
			continue
		}
		// Don't double-fire within the same minute.
		if s.lastRunAt != nil && now.Sub(*s.lastRunAt) < time.Minute {
			continue
		}
		if err := d.fire(ctx, s.id, s.agentID, s.objective, now); err != nil {
			// Fire failure is non-fatal — log and continue to other schedules.
			fmt.Printf("scheduler: fire %s: %v\n", s.id, err)
		}
	}
	return nil
}

func (d *Dispatcher) fire(ctx context.Context, schedID, agentID, objective string, now time.Time) error {
	conv, err := d.cs.Resolve(ctx, conversations.ConversationKey{
		ConnectorID: "scheduler",
		AccountID:   "system",
		ExternalID:  schedID,
		ThreadID:    "main",
	})
	if err != nil {
		return fmt.Errorf("resolve conversation: %w", err)
	}

	if _, err := d.rt.Start(ctx, runtime.StartRun{
		ConversationID: conv.ID,
		AgentID:        agentID,
		Objective:      objective,
		WorkspaceRoot:  d.workspaceRoot,
		AccountID:      "system",
	}); err != nil {
		return fmt.Errorf("start run: %w", err)
	}

	_, err = d.db.RawDB().ExecContext(ctx,
		`UPDATE schedules SET last_run_at = ?, updated_at = datetime('now') WHERE id = ?`,
		now.Format("2006-01-02 15:04:05"), schedID,
	)
	return err
}
