package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	SessionID             string
	TeamID                string
	Objective             string
	WorkspaceRoot         string
	AccountID             string
	ExecutionSnapshotJSON []byte
	// PreviewOnly instructs the run engine to skip workspace apply calls and
	// emit a preview_completed event instead of mutating any files.
	PreviewOnly bool
	// VerificationAgent marks this run as a verification agent turn; on
	// completion the engine emits a verification_completed event.
	VerificationAgent bool
}

type ContinueRun struct {
	RunID string
	Input string
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

// Memory exposes the memory store so callers (e.g. web layer) can read facts.
func (r *Runtime) Memory() *memory.Store {
	return r.memory
}

func (r *Runtime) Start(ctx context.Context, cmd StartRun) (model.Run, error) {
	runID := generateID()
	if err := r.createRun(ctx, runID, "", cmd); err != nil {
		return model.Run{}, err
	}

	return r.executeRunLoop(ctx, runLoopOpts{
		runID:             runID,
		conversationID:    cmd.ConversationID,
		agentID:           cmd.AgentID,
		objective:         cmd.Objective,
		previewOnly:       cmd.PreviewOnly,
		verificationAgent: cmd.VerificationAgent,
	})
}

func (r *Runtime) createRun(ctx context.Context, runID, parentRunID string, cmd StartRun) error {
	if err := r.budget.CheckDailyCap(ctx, cmd.AccountID); err != nil {
		return err
	}
	if parentRunID == "" {
		if active, err := r.convStore.ActiveRootRun(ctx, cmd.ConversationID); err != nil {
			return err
		} else if active.ID != "" {
			return conversations.ErrConversationBusy
		}
	}

	now := time.Now().UTC()

	payload, err := json.Marshal(map[string]any{
		"agent_id":                cmd.AgentID,
		"session_id":              cmd.SessionID,
		"team_id":                 cmd.TeamID,
		"objective":               cmd.Objective,
		"workspace_root":          cmd.WorkspaceRoot,
		"execution_snapshot_json": cmd.ExecutionSnapshotJSON,
	})
	if err != nil {
		return fmt.Errorf("marshal run_started payload: %w", err)
	}

	err = r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: cmd.ConversationID,
		RunID:          runID,
		ParentRunID:    parentRunID,
		Kind:           "run_started",
		PayloadJSON:    payload,
		CreatedAt:      now,
	})
	if err != nil {
		return fmt.Errorf("journal run_started: %w", err)
	}

	if r.eventSink != nil {
		_ = r.eventSink.Emit(ctx, runID, model.ReplayDelta{
			RunID:      runID,
			Kind:       "run_started",
			OccurredAt: now,
		})
	}

	return nil
}

type runLoopOpts struct {
	runID             string
	conversationID    string
	agentID           string
	objective         string
	previewOnly       bool
	verificationAgent bool
}

func (r *Runtime) executeRunLoop(ctx context.Context, opts runLoopOpts) (model.Run, error) {
	runID := opts.runID
	conversationID := opts.conversationID
	agentID := opts.agentID
	objective := opts.objective
	var cumulativeInput int
	var cumulativeOutput int

	budgetStopped := false
	for turn := 0; turn < 10; turn++ {
		if err := r.budget.BeforeTurn(ctx, model.RunProfile{
			RunID:        runID,
			InputTokens:  cumulativeInput,
			OutputTokens: cumulativeOutput,
		}); err != nil {
			stopPayload, _ := json.Marshal(map[string]any{
				"limit_type":  "per_run_tokens",
				"tokens_used": cumulativeInput + cumulativeOutput,
				"token_cap":   r.budget.PerRunTokenCap,
			})
			_ = r.convStore.AppendEvent(ctx, model.Event{
				ID:             generateID(),
				ConversationID: conversationID,
				RunID:          runID,
				Kind:           "budget_stop",
				PayloadJSON:    stopPayload,
			})
			_ = r.convStore.AppendEvent(ctx, model.Event{
				ID:             generateID(),
				ConversationID: conversationID,
				RunID:          runID,
				Kind:           "run_interrupted",
			})
			budgetStopped = true
			break
		}

		totalTokens := cumulativeInput + cumulativeOutput
		if totalTokens > int(float64(r.contextWindowSize)*0.75) {
			if _, err := r.memory.UpsertWorkingSummary(ctx, runID, conversationID); err != nil {
				return model.Run{}, err
			}
			_ = r.convStore.AppendEvent(ctx, model.Event{
				ID:             generateID(),
				ConversationID: conversationID,
				RunID:          runID,
				Kind:           "context_compacted",
			})
		}

		contextView, err := r.memory.LoadContext(ctx, runID, agentID, "local", 10)
		if err != nil {
			return model.Run{}, fmt.Errorf("load memory context: %w", err)
		}
		readPayload, err := json.Marshal(map[string]any{
			"scope":        "local",
			"memory_count": len(contextView.Items),
			"summary_id":   contextView.Summary.ID,
		})
		if err != nil {
			return model.Run{}, fmt.Errorf("marshal memory context payload: %w", err)
		}
		if err := r.convStore.AppendEvent(ctx, model.Event{
			ID:             generateID(),
			ConversationID: conversationID,
			RunID:          runID,
			Kind:           "memory_context_loaded",
			PayloadJSON:    readPayload,
		}); err != nil {
			return model.Run{}, fmt.Errorf("journal memory read: %w", err)
		}

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
		payload, err := json.Marshal(map[string]any{
			"content":       result.Content,
			"input_tokens":  result.InputTokens,
			"output_tokens": result.OutputTokens,
		})
		if err != nil {
			return model.Run{}, fmt.Errorf("marshal turn payload: %w", err)
		}

		if err := r.convStore.AppendEvent(ctx, model.Event{
			ID:             generateID(),
			ConversationID: conversationID,
			RunID:          runID,
			Kind:           "turn_completed",
			PayloadJSON:    payload,
		}); err != nil {
			return model.Run{}, fmt.Errorf("journal turn_completed: %w", err)
		}

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

	// If the budget guard stopped the run, it has already been marked
	// interrupted — do not emit run_completed.
	if budgetStopped {
		return r.loadRun(ctx, runID)
	}

	completedPayload, err := json.Marshal(map[string]any{
		"input_tokens":  cumulativeInput,
		"output_tokens": cumulativeOutput,
		"cost_usd":      0.0,
		"model_lane":    "",
	})
	if err != nil {
		return model.Run{}, fmt.Errorf("marshal run_completed payload: %w", err)
	}
	if err := r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "run_completed",
		PayloadJSON:    completedPayload,
	}); err != nil {
		return model.Run{}, fmt.Errorf("journal run_completed: %w", err)
	}

	if r.eventSink != nil {
		_ = r.eventSink.Emit(ctx, runID, model.ReplayDelta{
			RunID:      runID,
			Kind:       "run_completed",
			OccurredAt: time.Now().UTC(),
		})
	}

	// Emit supplementary events based on run mode.
	if opts.previewOnly {
		_ = r.convStore.AppendEvent(ctx, model.Event{
			ID:             generateID(),
			ConversationID: conversationID,
			RunID:          runID,
			Kind:           "preview_completed",
		})
	}
	if opts.verificationAgent {
		_ = r.convStore.AppendEvent(ctx, model.Event{
			ID:             generateID(),
			ConversationID: conversationID,
			RunID:          runID,
			Kind:           "verification_completed",
		})
	}

	return r.loadRun(ctx, runID)
}

func (r *Runtime) Continue(ctx context.Context, cmd ContinueRun) (model.Run, error) {
	return r.loadRun(ctx, cmd.RunID)
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

// ExpireStaleApprovals marks any pending approvals older than 24 hours as
// 'expired'. Returns the number of approvals expired.
func (r *Runtime) ExpireStaleApprovals(ctx context.Context) (int, error) {
	res, err := r.store.RawDB().ExecContext(ctx,
		`UPDATE approvals SET status = 'expired'
		 WHERE status = 'pending' AND created_at < datetime('now', '-24 hours')`)
	if err != nil {
		return 0, fmt.Errorf("expire stale approvals: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *Runtime) loadRun(ctx context.Context, runID string) (model.Run, error) {
	var run model.Run
	var status string
	err := r.store.RawDB().QueryRowContext(ctx,
		`SELECT id, conversation_id, agent_id, COALESCE(session_id, ''), COALESCE(team_id, ''), COALESCE(parent_run_id, ''),
		 COALESCE(objective, ''), COALESCE(workspace_root, ''), status,
		 input_tokens, output_tokens, created_at, updated_at
		 FROM runs WHERE id = ?`,
		runID,
	).Scan(
		&run.ID,
		&run.ConversationID,
		&run.AgentID,
		&run.SessionID,
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

// SubmitTask starts a new root run via the web interface, resolving the
// web conversation key internally. This is the canonical entry point for
// write-path web handlers.
func (r *Runtime) SubmitTask(ctx context.Context, objective, workspaceRoot string) (model.Run, error) {
	return r.StartFrontSession(ctx, StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "default",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: objective,
		WorkspaceRoot: workspaceRoot,
	})
}

// ResolveApproval approves or denies a pending approval ticket by its ID.
// decision must be "approved" or "denied".
func (r *Runtime) ResolveApproval(ctx context.Context, ticketID, decision string) error {
	return tools.ResolveTicket(ctx, r.store, ticketID, decision)
}

// UpdateSettings persists operator-editable settings to the database.
// The admin_token key is explicitly rejected to prevent accidental exposure.
func (r *Runtime) UpdateSettings(ctx context.Context, updates map[string]string) error {
	for key, value := range updates {
		if key == "admin_token" {
			return fmt.Errorf("runtime: admin_token cannot be updated via settings")
		}
		_, err := r.store.RawDB().ExecContext(ctx,
			`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			key, value,
		)
		if err != nil {
			return fmt.Errorf("runtime: update setting %q: %w", key, err)
		}
	}
	return nil
}

func generateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
