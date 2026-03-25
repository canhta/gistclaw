package runtime

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/sessions"
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
	store               *store.DB
	convStore           *conversations.ConversationStore
	tools               *tools.Registry
	memory              *memory.Store
	provider            Provider
	eventSink           model.RunEventSink
	budget              BudgetGuard
	contextWindowSize   int
	contexts            ContextAssembler
	teamDir             string
	defaultSnapshot     model.ExecutionSnapshot
	defaultSnapshotJSON []byte
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
		contexts:          newDefaultContextAssembler(db, cs, nil),
	}
}

// Memory exposes the memory store so callers (e.g. web layer) can read facts.
func (r *Runtime) Memory() *memory.Store {
	return r.memory
}

func (r *Runtime) SetDefaultExecutionSnapshot(snapshot model.ExecutionSnapshot) error {
	if len(snapshot.Agents) == 0 && snapshot.TeamID == "" {
		r.defaultSnapshot = model.ExecutionSnapshot{}
		r.defaultSnapshotJSON = nil
		return nil
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("runtime: marshal default execution snapshot: %w", err)
	}
	r.defaultSnapshot = snapshot
	r.defaultSnapshotJSON = raw
	return nil
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
		sessionID:         cmd.SessionID,
		objective:         cmd.Objective,
		workspaceRoot:     cmd.WorkspaceRoot,
		previewOnly:       cmd.PreviewOnly,
		verificationAgent: cmd.VerificationAgent,
	})
}

func (r *Runtime) createRun(ctx context.Context, runID, parentRunID string, cmd StartRun) error {
	now := time.Now().UTC()
	return r.createRunAt(ctx, runID, parentRunID, cmd, now)
}

func (r *Runtime) createRunAt(ctx context.Context, runID, parentRunID string, cmd StartRun, now time.Time) error {
	prepared, err := r.prepareStartRun(ctx, parentRunID, cmd)
	if err != nil {
		return err
	}
	cmd = prepared
	if err := r.prepareRunStart(ctx, parentRunID, cmd); err != nil {
		return err
	}

	event, err := newRunStartedEvent(cmd.ConversationID, runID, parentRunID, cmd, now)
	if err != nil {
		return err
	}

	err = r.convStore.AppendEvent(ctx, event)
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

func (r *Runtime) prepareStartRun(ctx context.Context, parentRunID string, cmd StartRun) (StartRun, error) {
	if len(cmd.ExecutionSnapshotJSON) == 0 {
		switch {
		case parentRunID != "":
			parent, err := r.loadRun(ctx, parentRunID)
			if err != nil {
				return StartRun{}, fmt.Errorf("prepare run start: load parent snapshot: %w", err)
			}
			cmd.ExecutionSnapshotJSON = append([]byte(nil), parent.ExecutionSnapshotJSON...)
			if cmd.TeamID == "" {
				cmd.TeamID = parent.TeamID
			}
		case len(r.defaultSnapshotJSON) > 0:
			cmd.ExecutionSnapshotJSON = append([]byte(nil), r.defaultSnapshotJSON...)
			if cmd.TeamID == "" {
				cmd.TeamID = r.defaultSnapshot.TeamID
			}
		}
	}
	if len(cmd.ExecutionSnapshotJSON) > 0 && cmd.TeamID == "" {
		snapshot, err := decodeExecutionSnapshot(cmd.ExecutionSnapshotJSON)
		if err != nil {
			return StartRun{}, fmt.Errorf("prepare run start: %w", err)
		}
		cmd.TeamID = snapshot.TeamID
	}
	return cmd, nil
}

func (r *Runtime) prepareRunStart(ctx context.Context, parentRunID string, cmd StartRun) error {
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
	return nil
}

func newRunStartedEvent(conversationID, runID, parentRunID string, cmd StartRun, now time.Time) (model.Event, error) {
	payload, err := json.Marshal(map[string]any{
		"agent_id":                cmd.AgentID,
		"session_id":              cmd.SessionID,
		"team_id":                 cmd.TeamID,
		"objective":               cmd.Objective,
		"workspace_root":          cmd.WorkspaceRoot,
		"execution_snapshot_json": cmd.ExecutionSnapshotJSON,
	})
	if err != nil {
		return model.Event{}, fmt.Errorf("marshal run_started payload: %w", err)
	}

	return model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		ParentRunID:    parentRunID,
		Kind:           "run_started",
		PayloadJSON:    payload,
		CreatedAt:      now,
	}, nil
}

type runLoopOpts struct {
	runID             string
	conversationID    string
	agentID           string
	sessionID         string
	objective         string
	workspaceRoot     string
	previewOnly       bool
	verificationAgent bool
}

type replayStreamSink struct {
	runID     string
	eventSink model.RunEventSink
}

func newReplayStreamSink(eventSink model.RunEventSink, runID string) StreamSink {
	if eventSink == nil {
		return nil
	}
	if _, ok := eventSink.(*model.NoopEventSink); ok {
		return nil
	}
	return &replayStreamSink{
		runID:     runID,
		eventSink: eventSink,
	}
}

func (s *replayStreamSink) OnDelta(ctx context.Context, text string) error {
	if s == nil || text == "" {
		return nil
	}
	payload, err := json.Marshal(map[string]any{"text": text})
	if err != nil {
		return fmt.Errorf("marshal turn_delta payload: %w", err)
	}
	return s.eventSink.Emit(ctx, s.runID, model.ReplayDelta{
		RunID:       s.runID,
		Kind:        "turn_delta",
		PayloadJSON: payload,
		OccurredAt:  time.Now().UTC(),
	})
}

func (s *replayStreamSink) OnComplete() error {
	return nil
}

func (r *Runtime) executeRunLoop(ctx context.Context, opts runLoopOpts) (model.Run, error) {
	runID := opts.runID
	run, err := r.loadRun(ctx, runID)
	if err != nil {
		return model.Run{}, err
	}

	conversationID := opts.conversationID
	if conversationID == "" {
		conversationID = run.ConversationID
	}
	agentID := opts.agentID
	if agentID == "" {
		agentID = run.AgentID
	}
	sessionID := opts.sessionID
	if sessionID == "" {
		sessionID = run.SessionID
	}
	objective := opts.objective
	if objective == "" {
		objective = run.Objective
	}
	workspaceRoot := opts.workspaceRoot
	if workspaceRoot == "" {
		workspaceRoot = run.WorkspaceRoot
	}
	cumulativeInput := run.InputTokens
	cumulativeOutput := run.OutputTokens
	runCtxEvents, err := r.loadRunContextEvents(ctx, runID)
	if err != nil {
		return model.Run{}, err
	}

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

		providerReq, err := r.contexts.Assemble(ctx, ContextAssemblyInput{
			SessionID:     sessionID,
			AgentID:       agentID,
			Objective:     objective,
			WorkspaceRoot: workspaceRoot,
			MemoryView:    contextView,
		})
		if err != nil {
			return model.Run{}, err
		}
		conversationCtx := combineConversationContext(providerReq.ConversationCtx, runCtxEvents)
		result, err := r.provider.Generate(ctx, GenerateRequest{
			Instructions:    providerReq.Instructions,
			ConversationCtx: conversationCtx,
			ToolSpecs:       r.tools.List(),
		}, newReplayStreamSink(r.eventSink, runID))
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
		runCtxEvents = append(runCtxEvents, model.Event{
			ConversationID: conversationID,
			RunID:          runID,
			Kind:           "turn_completed",
			PayloadJSON:    payload,
		})
		if sessionID != "" && result.Content != "" && (result.StopReason == "end_turn" || result.StopReason == "") {
			messageID, err := r.appendSessionMessage(
				ctx,
				conversationID,
				runID,
				sessionID,
				sessionID,
				model.MessageAssistant,
				result.Content,
				model.SessionMessageProvenance{
					Kind:        model.MessageProvenanceAssistantTurn,
					SourceRunID: runID,
				},
			)
			if err != nil {
				return model.Run{}, err
			}
			if err := r.queueOutboundIntent(ctx, runID, sessionID, messageID, result.Content); err != nil {
				return model.Run{}, err
			}
		}

		if r.eventSink != nil {
			_ = r.eventSink.Emit(ctx, runID, model.ReplayDelta{
				RunID:       runID,
				Kind:        "turn_completed",
				PayloadJSON: payload,
				OccurredAt:  time.Now().UTC(),
			})
		}

		if len(result.ToolCalls) > 0 {
			toolOutcome, err := r.executeToolCalls(ctx, runID, conversationID, agentID, workspaceRoot, result.ToolCalls)
			if err != nil {
				return model.Run{}, err
			}
			runCtxEvents = append(runCtxEvents, toolOutcome.events...)
			if toolOutcome.pausedForApproval {
				return r.loadRun(ctx, runID)
			}
			continue
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

type toolCallOutcome struct {
	events            []model.Event
	pausedForApproval bool
}

func (r *Runtime) executeToolCalls(
	ctx context.Context,
	runID string,
	conversationID string,
	agentID string,
	workspaceRoot string,
	toolCalls []model.ToolCallRequest,
) (toolCallOutcome, error) {
	outcome := toolCallOutcome{
		events: make([]model.Event, 0, len(toolCalls)),
	}
	policy := &tools.Policy{}
	runProfile := model.RunProfile{RunID: runID}
	agent, err := r.agentProfileForRun(ctx, runID, agentID)
	if err != nil {
		return outcome, err
	}

	for _, tc := range toolCalls {
		tool, ok := r.tools.Get(tc.ToolName)
		if !ok {
			event, err := r.recordToolCall(ctx, conversationID, runID, workspaceRoot, tc, nil, model.DecisionDeny, "tool not found", "")
			if err != nil {
				return outcome, err
			}
			outcome.events = append(outcome.events, event)
			continue
		}

		toolDecision := policy.DecideCall(agent, runProfile, tool.Spec(), tc.InputJSON)
		switch toolDecision.Mode {
		case model.DecisionAllow:
			event, err := r.recordToolCall(ctx, conversationID, runID, workspaceRoot, tc, tool, model.DecisionAllow, "", "")
			if err != nil {
				return outcome, err
			}
			outcome.events = append(outcome.events, event)
		case model.DecisionAsk:
			ticket, err := tools.CreateTicket(ctx, r.store, model.ApprovalRequest{
				RunID:      runID,
				ToolName:   tc.ToolName,
				ArgsJSON:   tc.InputJSON,
				TargetPath: tools.ApprovalTargetPath(tc.ToolName, tc.InputJSON),
			})
			if err != nil {
				return outcome, fmt.Errorf("create approval ticket: %w", err)
			}
			if err := r.appendApprovalRequested(ctx, conversationID, runID, tc, ticket, toolDecision.Reason); err != nil {
				return outcome, err
			}
			outcome.pausedForApproval = true
			return outcome, nil
		default:
			event, err := r.recordToolCall(ctx, conversationID, runID, workspaceRoot, tc, tool, model.DecisionDeny, toolDecision.Reason, "")
			if err != nil {
				return outcome, err
			}
			outcome.events = append(outcome.events, event)
		}
	}

	return outcome, nil
}

func (r *Runtime) loadRunContextEvents(ctx context.Context, runID string) ([]model.Event, error) {
	rows, err := r.store.RawDB().QueryContext(ctx,
		`SELECT id, conversation_id, COALESCE(run_id, ''), COALESCE(parent_run_id, ''), kind,
		        COALESCE(payload_json, x''), created_at
		 FROM events
		 WHERE run_id = ? AND kind IN ('turn_completed', 'tool_call_recorded')
		 ORDER BY created_at ASC, id ASC`,
		runID,
	)
	if err != nil {
		return nil, fmt.Errorf("load run context events: %w", err)
	}
	defer rows.Close()

	events := make([]model.Event, 0, 16)
	for rows.Next() {
		var evt model.Event
		if err := rows.Scan(
			&evt.ID,
			&evt.ConversationID,
			&evt.RunID,
			&evt.ParentRunID,
			&evt.Kind,
			&evt.PayloadJSON,
			&evt.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan run context event: %w", err)
		}
		events = append(events, evt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate run context events: %w", err)
	}
	return events, nil
}

func (r *Runtime) recordToolCall(
	ctx context.Context,
	conversationID string,
	runID string,
	workspaceRoot string,
	tc model.ToolCallRequest,
	tool tools.Tool,
	decision model.DecisionMode,
	denyReason string,
	approvalID string,
) (model.Event, error) {
	result := model.ToolResult{}
	if decision == model.DecisionAllow {
		if tool == nil {
			result.Error = "tool not found"
		} else {
			invokeCtx := tools.WithInvocationContext(ctx, tools.InvocationContext{WorkspaceRoot: workspaceRoot})
			invoked, err := tool.Invoke(invokeCtx, model.ToolCall{
				ID:        tc.ID,
				ToolName:  tc.ToolName,
				InputJSON: tc.InputJSON,
			})
			if err != nil {
				invoked.Error = err.Error()
			}
			result = invoked
		}
	} else {
		result.Error = denyReason
	}

	outputJSON, err := json.Marshal(result)
	if err != nil {
		return model.Event{}, fmt.Errorf("marshal tool result: %w", err)
	}
	payload, err := json.Marshal(map[string]any{
		"tool_call_id": tc.ID,
		"tool_name":    tc.ToolName,
		"input_json":   json.RawMessage(tc.InputJSON),
		"output_json":  json.RawMessage(outputJSON),
		"decision":     string(decision),
		"approval_id":  approvalID,
	})
	if err != nil {
		return model.Event{}, fmt.Errorf("marshal tool_call_recorded payload: %w", err)
	}
	evt := model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "tool_call_recorded",
		PayloadJSON:    payload,
	}
	if err := r.convStore.AppendEvent(ctx, evt); err != nil {
		return model.Event{}, fmt.Errorf("journal tool_call_recorded: %w", err)
	}
	return evt, nil
}

func (r *Runtime) appendApprovalRequested(
	ctx context.Context,
	conversationID string,
	runID string,
	tc model.ToolCallRequest,
	ticket model.ApprovalTicket,
	reason string,
) error {
	payload, err := json.Marshal(map[string]any{
		"approval_id":  ticket.ID,
		"tool_call_id": tc.ID,
		"tool_name":    tc.ToolName,
		"input_json":   json.RawMessage(tc.InputJSON),
		"target_path":  ticket.TargetPath,
		"reason":       reason,
	})
	if err != nil {
		return fmt.Errorf("marshal approval_requested payload: %w", err)
	}
	if err := r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "approval_requested",
		PayloadJSON:    payload,
	}); err != nil {
		return fmt.Errorf("journal approval_requested: %w", err)
	}
	return nil
}

func (r *Runtime) appendRunResumed(ctx context.Context, conversationID, runID, approvalID, decision string) error {
	payload, err := json.Marshal(map[string]any{
		"approval_id": approvalID,
		"decision":    decision,
	})
	if err != nil {
		return fmt.Errorf("marshal run_resumed payload: %w", err)
	}
	if err := r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "run_resumed",
		PayloadJSON:    payload,
	}); err != nil {
		return fmt.Errorf("journal run_resumed: %w", err)
	}
	return nil
}

func (r *Runtime) loadApprovalToolCallID(ctx context.Context, runID, approvalID string) (string, error) {
	rows, err := r.store.RawDB().QueryContext(ctx,
		`SELECT payload_json
		 FROM events
		 WHERE run_id = ? AND kind = 'approval_requested'
		 ORDER BY created_at ASC, id ASC`,
		runID,
	)
	if err != nil {
		return "", fmt.Errorf("load approval event: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var payloadJSON []byte
		if err := rows.Scan(&payloadJSON); err != nil {
			return "", fmt.Errorf("scan approval event: %w", err)
		}
		var payload struct {
			ApprovalID string `json:"approval_id"`
			ToolCallID string `json:"tool_call_id"`
		}
		if err := json.Unmarshal(payloadJSON, &payload); err != nil {
			return "", fmt.Errorf("decode approval event: %w", err)
		}
		if payload.ApprovalID == approvalID {
			return payload.ToolCallID, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate approval events: %w", err)
	}
	return "", nil
}

func combineConversationContext(base []model.Event, extra []model.Event) []model.Event {
	if len(extra) == 0 {
		return append([]model.Event(nil), base...)
	}
	combined := make([]model.Event, 0, len(base)+len(extra))
	combined = append(combined, base...)
	combined = append(combined, extra...)
	return combined
}

func mailboxToEvents(messages []model.SessionMessage) []model.Event {
	events := make([]model.Event, 0, len(messages))
	for _, msg := range messages {
		payload, err := json.Marshal(map[string]any{
			"kind":              msg.Kind,
			"body":              msg.Body,
			"sender_session_id": msg.SenderSessionID,
			"provenance":        msg.Provenance,
		})
		if err != nil {
			continue
		}
		events = append(events, model.Event{
			ID:          msg.ID,
			Kind:        "session_message_added",
			PayloadJSON: payload,
			CreatedAt:   msg.CreatedAt,
		})
	}
	return events
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
		 COALESCE(objective, ''), COALESCE(workspace_root, ''), status, COALESCE(execution_snapshot_json, x''),
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
		&run.ExecutionSnapshotJSON,
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

func (r *Runtime) agentProfileForRun(ctx context.Context, runID, agentID string) (model.AgentProfile, error) {
	run, err := r.loadRun(ctx, runID)
	if err != nil {
		return model.AgentProfile{}, fmt.Errorf("runtime: load run agent profile: %w", err)
	}
	return agentProfileFromSnapshot(run.ExecutionSnapshotJSON, agentID)
}

func agentProfileFromSnapshot(snapshotJSON []byte, agentID string) (model.AgentProfile, error) {
	fallback := model.AgentProfile{AgentID: agentID}
	if len(snapshotJSON) == 0 {
		return fallback, nil
	}
	snapshot, err := decodeExecutionSnapshot(snapshotJSON)
	if err != nil {
		return model.AgentProfile{}, err
	}
	profile, ok := snapshot.Agents[agentID]
	if !ok {
		return fallback, nil
	}
	if profile.AgentID == "" {
		profile.AgentID = agentID
	}
	return profile, nil
}

func decodeExecutionSnapshot(snapshotJSON []byte) (model.ExecutionSnapshot, error) {
	var snapshot model.ExecutionSnapshot
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return model.ExecutionSnapshot{}, fmt.Errorf("decode execution snapshot: %w", err)
	}
	if snapshot.Agents == nil {
		snapshot.Agents = map[string]model.AgentProfile{}
	}
	return snapshot, nil
}

func (r *Runtime) queueOutboundIntent(
	ctx context.Context,
	runID string,
	sessionID string,
	messageID string,
	body string,
) error {
	route, err := sessions.NewService(r.store, r.convStore).LoadRouteBySession(ctx, sessionID)
	if err == sessions.ErrSessionRouteNotFound {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load outbound route: %w", err)
	}
	if route.ConnectorID == "" || route.ConnectorID == "web" || route.ExternalID == "" {
		return nil
	}

	dedupeKey := "session-message:" + messageID
	var existing int
	if err := r.store.RawDB().QueryRowContext(ctx,
		"SELECT count(*) FROM outbound_intents WHERE dedupe_key = ?",
		dedupeKey,
	).Scan(&existing); err != nil {
		return fmt.Errorf("check outbound intent dedupe: %w", err)
	}
	if existing > 0 {
		return nil
	}

	_, err = r.store.RawDB().ExecContext(ctx,
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', 0, datetime('now'))`,
		generateID(), runID, route.ConnectorID, route.ExternalID, body, dedupeKey,
	)
	if err != nil {
		return fmt.Errorf("insert outbound intent: %w", err)
	}

	return nil
}

// SubmitTask starts a new root run via the web interface, resolving the
// web conversation key internally. This is the canonical entry point for
// write-path web handlers.
func (r *Runtime) SubmitTask(ctx context.Context, objective, workspaceRoot string) (model.Run, error) {
	return r.ReceiveInboundMessage(ctx, InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "default",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		Body:          objective,
		WorkspaceRoot: workspaceRoot,
	})
}

// ResolveApproval approves or denies a pending approval ticket by its ID.
// decision must be "approved" or "denied".
func (r *Runtime) ResolveApproval(ctx context.Context, ticketID, decision string) error {
	ticket, err := tools.LoadTicket(ctx, r.store, ticketID)
	if err != nil {
		return err
	}
	if err := tools.ResolveTicket(ctx, r.store, ticketID, decision); err != nil {
		return err
	}

	run, err := r.loadRun(ctx, ticket.RunID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}

	toolCallID, err := r.loadApprovalToolCallID(ctx, run.ID, ticket.ID)
	if err != nil {
		return err
	}
	if toolCallID == "" {
		toolCallID = ticket.ID
	}
	call := model.ToolCallRequest{
		ID:        toolCallID,
		ToolName:  ticket.ToolName,
		InputJSON: ticket.ArgsJSON,
	}

	switch decision {
	case "approved":
		if err := r.appendRunResumed(ctx, run.ConversationID, run.ID, ticket.ID, decision); err != nil {
			return err
		}
		tool, _ := r.tools.Get(ticket.ToolName)
		if _, err := r.recordToolCall(ctx, run.ConversationID, run.ID, run.WorkspaceRoot, call, tool, model.DecisionAllow, "", ticket.ID); err != nil {
			return err
		}
		_, err = r.executeRunLoop(ctx, runLoopOpts{
			runID:             run.ID,
			conversationID:    run.ConversationID,
			agentID:           run.AgentID,
			sessionID:         run.SessionID,
			objective:         run.Objective,
			workspaceRoot:     run.WorkspaceRoot,
			verificationAgent: false,
		})
		return err
	case "denied":
		if _, err := r.recordToolCall(ctx, run.ConversationID, run.ID, run.WorkspaceRoot, call, nil, model.DecisionDeny, "approval denied", ticket.ID); err != nil {
			return err
		}
		if err := r.convStore.AppendEvent(ctx, model.Event{
			ID:             generateID(),
			ConversationID: run.ConversationID,
			RunID:          run.ID,
			Kind:           "run_interrupted",
		}); err != nil {
			return fmt.Errorf("journal run_interrupted: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("runtime: invalid approval decision %q", decision)
	}
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
