package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

var ErrBudgetExhausted = fmt.Errorf("runtime: budget exhausted")
var ErrDailyCap = fmt.Errorf("runtime: daily cost cap exceeded")

type StartRun struct {
	ConversationID        string
	AgentID               string
	TeamID                string
	Objective             string
	WorkspaceRoot         string
	AccountID             string
	ExecutionSnapshotJSON []byte
}

type ContinueRun struct {
	RunID string
	Input string
}

type DelegateRun struct {
	ParentRunID   string
	TargetAgentID string
	Objective     string
}

type ResumeRun struct {
	RunID string
}

type ReconcileReport struct {
	ReconciledCount int
	RunIDs          []string
}

type BudgetGuard struct {
	db              *store.DB
	PerRunTokenCap  int
	DailyCostCapUSD float64
}

func (b *BudgetGuard) BeforeTurn(_ context.Context, run model.RunProfile) error {
	totalTokens := run.InputTokens + run.OutputTokens
	if b.PerRunTokenCap > 0 && totalTokens >= b.PerRunTokenCap {
		return ErrBudgetExhausted
	}
	return nil
}

func (b *BudgetGuard) RecordUsage(ctx context.Context, runID string, usage model.UsageRecord) error {
	_, err := b.db.RawDB().ExecContext(ctx,
		`UPDATE runs
		 SET input_tokens = input_tokens + ?, output_tokens = output_tokens + ?, updated_at = datetime('now')
		 WHERE id = ?`,
		usage.InputTokens, usage.OutputTokens, runID,
	)
	return err
}

func (b *BudgetGuard) CheckDailyCap(ctx context.Context, _ string) error {
	if b.DailyCostCapUSD <= 0 {
		return nil
	}

	var totalCost float64
	err := b.db.RawDB().QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost_usd), 0)
		 FROM receipts
		 WHERE created_at >= datetime('now', '-24 hours')`,
	).Scan(&totalCost)
	if err != nil {
		return fmt.Errorf("check daily cap: %w", err)
	}
	if totalCost >= b.DailyCostCapUSD {
		return ErrDailyCap
	}
	return nil
}

func (b *BudgetGuard) RecordIdleBurn(context.Context, string, time.Duration) error {
	return nil
}

type Runtime struct {
	store             *store.DB
	convStore         *conversations.ConversationStore
	tools             *tools.Registry
	memory            *memory.Store
	provider          Provider
	eventSink         model.RunEventSink
	budget            BudgetGuard
	contextWindowSize int
	maxActiveChildren int
}

func New(
	db *store.DB,
	cs *conversations.ConversationStore,
	reg *tools.Registry,
	mem *memory.Store,
	prov Provider,
	sink model.RunEventSink,
) *Runtime {
	return &Runtime{
		store:     db,
		convStore: cs,
		tools:     reg,
		memory:    mem,
		provider:  prov,
		eventSink: sink,
		budget: BudgetGuard{
			db:              db,
			PerRunTokenCap:  1000000,
			DailyCostCapUSD: 10.0,
		},
		contextWindowSize: 200000,
	}
}

func (r *Runtime) Start(ctx context.Context, cmd StartRun) (model.Run, error) {
	if err := r.budget.CheckDailyCap(ctx, cmd.AccountID); err != nil {
		return model.Run{}, err
	}

	runID := generateID()
	now := time.Now().UTC()

	_, err := r.store.RawDB().ExecContext(ctx,
		`INSERT INTO runs
		 (id, conversation_id, agent_id, team_id, objective, workspace_root, status, execution_snapshot_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?)`,
		runID, cmd.ConversationID, cmd.AgentID, cmd.TeamID, cmd.Objective, cmd.WorkspaceRoot,
		cmd.ExecutionSnapshotJSON, now, now,
	)
	if err != nil {
		return model.Run{}, fmt.Errorf("create run: %w", err)
	}

	err = r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: cmd.ConversationID,
		RunID:          runID,
		Kind:           "run_started",
		PayloadJSON:    []byte(fmt.Sprintf(`{"objective":%q}`, cmd.Objective)),
	})
	if err != nil {
		return model.Run{}, fmt.Errorf("journal run_started: %w", err)
	}

	if r.eventSink != nil {
		_ = r.eventSink.Emit(ctx, runID, model.ReplayDelta{
			RunID:      runID,
			Kind:       "run_started",
			OccurredAt: now,
		})
	}

	return r.executeRunLoop(ctx, runID, cmd.ConversationID, cmd.Objective)
}

func (r *Runtime) executeRunLoop(ctx context.Context, runID, conversationID, objective string) (model.Run, error) {
	var cumulativeInput int
	var cumulativeOutput int

	for turn := 0; turn < 10; turn++ {
		if err := r.budget.BeforeTurn(ctx, model.RunProfile{
			RunID:        runID,
			InputTokens:  cumulativeInput,
			OutputTokens: cumulativeOutput,
		}); err != nil {
			_ = r.convStore.AppendEvent(ctx, model.Event{
				ID:             generateID(),
				ConversationID: conversationID,
				RunID:          runID,
				Kind:           "budget_exhausted",
			})
			break
		}

		totalTokens := cumulativeInput + cumulativeOutput
		if totalTokens > int(float64(r.contextWindowSize)*0.75) {
			_, _ = r.memory.SummarizeConversation(ctx, conversationID)
			_ = r.convStore.AppendEvent(ctx, model.Event{
				ID:             generateID(),
				ConversationID: conversationID,
				RunID:          runID,
				Kind:           "context_compacted",
			})
		}

		_, _ = r.memory.Search(ctx, model.MemoryQuery{Limit: 10})

		events, _ := r.convStore.ListEvents(ctx, conversationID, 100)
		result, err := r.provider.Generate(ctx, GenerateRequest{
			Instructions:    objective,
			ConversationCtx: events,
		}, nil)
		if err != nil {
			_ = r.convStore.AppendEvent(ctx, model.Event{
				ID:             generateID(),
				ConversationID: conversationID,
				RunID:          runID,
				Kind:           "run_failed",
				PayloadJSON:    []byte(fmt.Sprintf(`{"error":%q}`, err.Error())),
			})
			run, loadErr := r.loadRun(ctx, runID)
			if loadErr != nil {
				return model.Run{}, err
			}
			return run, err
		}

		cumulativeInput += result.InputTokens
		cumulativeOutput += result.OutputTokens
		_ = r.budget.RecordUsage(ctx, runID, model.UsageRecord{
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
		})

		_ = r.convStore.AppendEvent(ctx, model.Event{
			ID:             generateID(),
			ConversationID: conversationID,
			RunID:          runID,
			Kind:           "turn_completed",
			PayloadJSON:    []byte(fmt.Sprintf(`{"content":%q}`, result.Content)),
		})

		if r.eventSink != nil {
			_ = r.eventSink.Emit(ctx, runID, model.ReplayDelta{
				RunID:      runID,
				Kind:       "turn_completed",
				OccurredAt: time.Now().UTC(),
			})
		}

		if result.StopReason == "end_turn" || result.StopReason == "" {
			break
		}
	}

	_ = r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "run_completed",
	})

	if r.eventSink != nil {
		_ = r.eventSink.Emit(ctx, runID, model.ReplayDelta{
			RunID:      runID,
			Kind:       "run_completed",
			OccurredAt: time.Now().UTC(),
		})
	}

	_, err := r.store.RawDB().ExecContext(ctx,
		`INSERT INTO receipts (id, run_id, input_tokens, output_tokens, cost_usd, created_at)
		 VALUES (?, ?, ?, ?, ?, datetime('now'))`,
		generateID(), runID, cumulativeInput, cumulativeOutput, 0.0,
	)
	if err != nil {
		return model.Run{}, fmt.Errorf("write receipt: %w", err)
	}

	return r.loadRun(ctx, runID)
}

func (r *Runtime) Continue(ctx context.Context, cmd ContinueRun) (model.Run, error) {
	return r.loadRun(ctx, cmd.RunID)
}

func (r *Runtime) Delegate(ctx context.Context, cmd DelegateRun) (model.Run, error) {
	return r.createDelegation(ctx, cmd)
}

func (r *Runtime) Resume(ctx context.Context, cmd ResumeRun) (model.Run, error) {
	return r.loadRun(ctx, cmd.RunID)
}

func (r *Runtime) ReconcileInterrupted(ctx context.Context) (ReconcileReport, error) {
	rows, err := r.store.RawDB().QueryContext(ctx,
		"SELECT id, conversation_id FROM runs WHERE status IN ('active', 'pending')",
	)
	if err != nil {
		return ReconcileReport{}, fmt.Errorf("query active runs: %w", err)
	}
	defer rows.Close()

	type runInfo struct {
		id             string
		conversationID string
	}

	var runsToReconcile []runInfo
	var report ReconcileReport
	for rows.Next() {
		var info runInfo
		if err := rows.Scan(&info.id, &info.conversationID); err != nil {
			return ReconcileReport{}, fmt.Errorf("scan run: %w", err)
		}
		runsToReconcile = append(runsToReconcile, info)
	}
	if err := rows.Err(); err != nil {
		return ReconcileReport{}, err
	}
	if err := rows.Close(); err != nil {
		return ReconcileReport{}, err
	}

	for _, info := range runsToReconcile {
		if err := r.convStore.AppendEvent(ctx, model.Event{
			ID:             generateID(),
			ConversationID: info.conversationID,
			RunID:          info.id,
			Kind:           "run_interrupted",
		}); err != nil {
			return report, fmt.Errorf("journal interrupted %s: %w", info.id, err)
		}

		report.ReconciledCount++
		report.RunIDs = append(report.RunIDs, info.id)
	}

	return report, nil
}

func (r *Runtime) loadRun(ctx context.Context, runID string) (model.Run, error) {
	var run model.Run
	var status string
	err := r.store.RawDB().QueryRowContext(ctx,
		`SELECT id, conversation_id, agent_id, COALESCE(team_id, ''), COALESCE(parent_run_id, ''),
		 COALESCE(objective, ''), COALESCE(workspace_root, ''), status,
		 input_tokens, output_tokens, created_at, updated_at
		 FROM runs WHERE id = ?`,
		runID,
	).Scan(
		&run.ID,
		&run.ConversationID,
		&run.AgentID,
		&run.TeamID,
		&run.ParentRunID,
		&run.Objective,
		&run.WorkspaceRoot,
		&status,
		&run.InputTokens,
		&run.OutputTokens,
		&run.CreatedAt,
		&run.UpdatedAt,
	)
	if err != nil {
		return model.Run{}, fmt.Errorf("load run: %w", err)
	}

	run.Status = model.RunStatus(status)
	return run, nil
}

func generateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
