package runtime

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/canhta/gistclaw/internal/authority"
	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/i18n"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/sessions"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

var ErrBudgetExhausted = fmt.Errorf("runtime: budget exhausted")
var ErrDailyCap = fmt.Errorf("runtime: daily cost cap exceeded")

type toolInvocationLogSink struct {
	convStore      *conversations.ConversationStore
	eventSink      model.RunEventSink
	conversationID string
	runID          string
	toolCallID     string
	toolName       string
}

func (s toolInvocationLogSink) Record(ctx context.Context, record tools.ToolLogRecord) error {
	if strings.TrimSpace(record.Text) == "" {
		return nil
	}
	occurredAt := record.OccurredAt.UTC()
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	payload, err := json.Marshal(map[string]any{
		"tool_call_id": s.toolCallID,
		"tool_name":    s.toolName,
		"stream":       record.Stream,
		"text":         record.Text,
	})
	if err != nil {
		return fmt.Errorf("marshal tool_log_recorded payload: %w", err)
	}
	evt := model.Event{
		ID:             generateID(),
		ConversationID: s.conversationID,
		RunID:          s.runID,
		Kind:           "tool_log_recorded",
		PayloadJSON:    payload,
		CreatedAt:      occurredAt,
	}
	if err := s.convStore.AppendEvent(ctx, evt); err != nil {
		return fmt.Errorf("journal tool_log_recorded: %w", err)
	}
	if s.eventSink != nil {
		_ = s.eventSink.Emit(ctx, s.runID, model.ReplayDelta{
			EventID:     evt.ID,
			RunID:       s.runID,
			Kind:        "tool_log_recorded",
			PayloadJSON: payload,
			OccurredAt:  occurredAt,
		})
	}
	return nil
}

type StartRun struct {
	ConversationID        string
	AgentID               string
	SessionID             string
	TeamID                string
	ProjectID             string
	Objective             string
	CWD                   string
	AuthorityJSON         []byte
	AccountID             string
	ExecutionSnapshotJSON []byte
	// PreviewOnly instructs the run engine to skip scoped apply calls and
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
	providerTimeout     time.Duration
	eventSink           model.RunEventSink
	budget              BudgetGuard
	contextWindowSize   int
	contexts            ContextAssembler
	teamDir             string
	storageRoot         string
	defaultSnapshot     model.ExecutionSnapshot
	defaultSnapshotJSON []byte
	asyncCtx            context.Context
	asyncWG             sync.WaitGroup
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
		store:           db,
		convStore:       cs,
		tools:           reg,
		memory:          mem,
		provider:        prov,
		providerTimeout: 45 * time.Second,
		eventSink:       sink,
		budget: BudgetGuard{
			db:              db,
			PerRunTokenCap:  1000000,
			DailyCostCapUSD: 10.0,
		},
		contextWindowSize: 200000,
		contexts:          newDefaultContextAssembler(db, cs, nil),
		asyncCtx:          context.Background(),
	}
}

// Memory exposes the memory store so callers (e.g. web layer) can read facts.
func (r *Runtime) Memory() *memory.Store {
	return r.memory
}

func (r *Runtime) SetStorageRoot(storageRoot string) {
	r.storageRoot = strings.TrimSpace(storageRoot)
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

func (r *Runtime) SetAsyncContext(ctx context.Context) {
	if ctx == nil {
		r.asyncCtx = context.Background()
		return
	}
	r.asyncCtx = ctx
}

func (r *Runtime) WaitAsync() {
	r.asyncWG.Wait()
}

func (r *Runtime) Start(ctx context.Context, cmd StartRun) (model.Run, error) {
	runID := generateID()
	if err := r.createRun(ctx, runID, "", cmd); err != nil {
		return model.Run{}, err
	}

	return r.executeRunLoop(ctx, startRunLoopOpts(runID, cmd))
}

func (r *Runtime) StartAsync(ctx context.Context, cmd StartRun) (model.Run, error) {
	runID := generateID()
	if err := r.createRun(ctx, runID, "", cmd); err != nil {
		return model.Run{}, err
	}

	run, err := r.loadRun(ctx, runID)
	if err != nil {
		return model.Run{}, err
	}

	r.executeRunLoopAsync(startRunLoopOpts(runID, cmd))
	return run, nil
}

func startRunLoopOpts(runID string, cmd StartRun) runLoopOpts {
	return runLoopOpts{
		runID:             runID,
		conversationID:    cmd.ConversationID,
		agentID:           cmd.AgentID,
		sessionID:         cmd.SessionID,
		objective:         cmd.Objective,
		cwd:               cmd.CWD,
		previewOnly:       cmd.PreviewOnly,
		verificationAgent: cmd.VerificationAgent,
	}
}

func (r *Runtime) executeRunLoopAsync(loopOpts runLoopOpts) {
	execCtx := r.asyncCtx
	if execCtx == nil {
		execCtx = context.Background()
	}
	r.asyncWG.Add(1)
	go func() {
		defer r.asyncWG.Done()
		_, _ = r.executeRunLoop(execCtx, loopOpts)
	}()
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

	if err := r.finishRunStart(ctx, runID, parentRunID, cmd, now, event.ID); err != nil {
		return err
	}

	return nil
}

func (r *Runtime) finishRunStart(ctx context.Context, runID, parentRunID string, cmd StartRun, now time.Time, eventID string) error {
	if r.eventSink != nil {
		_ = r.eventSink.Emit(ctx, runID, model.ReplayDelta{
			EventID:    eventID,
			RunID:      runID,
			Kind:       "run_started",
			OccurredAt: now,
		})
	}

	if err := r.promoteRunObjectiveMemory(ctx, model.Run{
		ID:             runID,
		ConversationID: cmd.ConversationID,
		ProjectID:      cmd.ProjectID,
		AgentID:        cmd.AgentID,
		ParentRunID:    parentRunID,
	}, cmd.Objective); err != nil {
		return err
	}

	return nil
}

func (r *Runtime) prepareStartRun(ctx context.Context, parentRunID string, cmd StartRun) (StartRun, error) {
	cmd.CWD = normalizePrimaryPath(cmd.CWD)
	if cmd.ProjectID == "" && cmd.CWD != "" {
		project, err := RegisterProjectPath(ctx, r.store, cmd.CWD, "", "runtime")
		if err != nil {
			return StartRun{}, fmt.Errorf("prepare run start: register cwd: %w", err)
		}
		cmd.ProjectID = project.ID
	}
	if cmd.ProjectID != "" && cmd.CWD == "" {
		project, err := loadProjectByID(ctx, r.store, cmd.ProjectID)
		if err != nil && err != sql.ErrNoRows {
			return StartRun{}, fmt.Errorf("prepare run start: load project cwd: %w", err)
		}
		if err == nil {
			cmd.CWD = project.PrimaryPath
		}
	}
	if cmd.ProjectID == "" || cmd.CWD == "" {
		project, err := ActiveProject(ctx, r.store)
		if err != nil {
			return StartRun{}, fmt.Errorf("prepare run start: resolve active project: %w", err)
		}
		if cmd.ProjectID == "" && project.ID != "" {
			cmd.ProjectID = project.ID
		}
		if cmd.CWD == "" && project.PrimaryPath != "" {
			cmd.CWD = project.PrimaryPath
		}
	}
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
			if len(cmd.AuthorityJSON) == 0 {
				cmd.AuthorityJSON = append([]byte(nil), parent.AuthorityJSON...)
			}
		default:
			snapshot, rawSnapshot, err := r.executionSnapshotForContext(ctx)
			if err != nil {
				return StartRun{}, fmt.Errorf("prepare run start: %w", err)
			}
			if len(rawSnapshot) > 0 {
				cmd.ExecutionSnapshotJSON = append([]byte(nil), rawSnapshot...)
			}
			if cmd.TeamID == "" {
				cmd.TeamID = snapshot.TeamID
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
	authorityJSON, err := r.resolveRuntimeAuthorityJSON(ctx, cmd.AuthorityJSON)
	if err != nil {
		return StartRun{}, fmt.Errorf("prepare run start: resolve authority: %w", err)
	}
	cmd.AuthorityJSON = authorityJSON
	return cmd, nil
}

func (r *Runtime) prepareRunStart(ctx context.Context, parentRunID string, cmd StartRun) error {
	if err := r.budget.CheckDailyCap(ctx, cmd.AccountID); err != nil {
		return err
	}
	runAuthority, err := authority.DecodeEnvelope(cmd.AuthorityJSON)
	if err != nil {
		return fmt.Errorf("prepare run start: decode authority: %w", err)
	}
	if err := r.enforceConversationAuthority(ctx, cmd.ConversationID, runAuthority); err != nil {
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
		"project_id":              cmd.ProjectID,
		"objective":               cmd.Objective,
		"cwd":                     cmd.CWD,
		"authority_json":          json.RawMessage(cmd.AuthorityJSON),
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
	cwd               string
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
	runAuthority, err := authority.DecodeEnvelope(run.AuthorityJSON)
	if err != nil {
		return model.Run{}, fmt.Errorf("decode run authority: %w", err)
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
	cwd := opts.cwd
	if cwd == "" {
		cwd = run.CWD
	}
	projectID := run.ProjectID
	cumulativeInput := run.InputTokens
	cumulativeOutput := run.OutputTokens
	runModelLane := run.ModelLane
	runModelID := run.ModelID
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
			if _, err := r.memory.UpsertWorkingSummary(ctx, runID, conversationID, projectID); err != nil {
				return model.Run{}, err
			}
			_ = r.convStore.AppendEvent(ctx, model.Event{
				ID:             generateID(),
				ConversationID: conversationID,
				RunID:          runID,
				Kind:           "context_compacted",
			})
		}

		contextView, err := r.memory.LoadContext(ctx, runID, projectID, agentID, "local", 10)
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

		agentProfile, err := r.agentProfileForRun(ctx, runID, agentID)
		if err != nil {
			return model.Run{}, err
		}
		providerReq, err := r.contexts.Assemble(ctx, ContextAssemblyInput{
			SessionID:  sessionID,
			AgentID:    agentID,
			Agent:      agentProfile,
			Objective:  objective,
			CWD:        cwd,
			MemoryView: contextView,
		})
		if err != nil {
			return model.Run{}, err
		}
		conversationCtx := combineConversationContext(providerReq.ConversationCtx, runCtxEvents)
		generateCtx := ctx
		cancel := func() {}
		if _, hasDeadline := ctx.Deadline(); !hasDeadline && r.providerTimeout > 0 {
			generateCtx, cancel = context.WithTimeout(ctx, r.providerTimeout)
		}
		result, err := r.provider.Generate(generateCtx, GenerateRequest{
			Instructions:    providerReq.Instructions,
			ConversationCtx: conversationCtx,
			ToolSpecs:       r.visibleToolSpecs(agentProfile),
		}, newReplayStreamSink(r.eventSink, runID))
		cancel()
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
		if result.ModelID != "" {
			runModelID = result.ModelID
		}
		payload, err := json.Marshal(map[string]any{
			"content":       result.Content,
			"input_tokens":  result.InputTokens,
			"output_tokens": result.OutputTokens,
			"model_id":      result.ModelID,
		})
		if err != nil {
			return model.Run{}, fmt.Errorf("marshal turn payload: %w", err)
		}

		completedAt := time.Now().UTC()
		turnEvent := model.Event{
			ID:             generateID(),
			ConversationID: conversationID,
			RunID:          runID,
			Kind:           "turn_completed",
			PayloadJSON:    payload,
			CreatedAt:      completedAt,
		}
		if err := r.convStore.AppendEvent(ctx, turnEvent); err != nil {
			return model.Run{}, fmt.Errorf("journal turn_completed: %w", err)
		}
		runCtxEvents = append(runCtxEvents, turnEvent)
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
				EventID:     turnEvent.ID,
				RunID:       runID,
				Kind:        "turn_completed",
				PayloadJSON: payload,
				OccurredAt:  completedAt,
			})
		}

		if len(result.ToolCalls) > 0 {
			toolOutcome, err := r.executeToolCalls(ctx, runID, conversationID, agentID, sessionID, cwd, runAuthority, result.ToolCalls)
			if err != nil {
				return model.Run{}, err
			}
			runCtxEvents = append(runCtxEvents, toolOutcome.events...)
			if toolOutcome.paused {
				return r.loadRun(ctx, runID)
			}
			continue
		}
		if strings.TrimSpace(result.Content) == "" && result.StopReason != "" && result.StopReason != "end_turn" {
			interrupted, err := r.interruptRun(ctx, run)
			if err != nil {
				return model.Run{}, err
			}
			if err := r.resumeParentAfterChildTerminal(ctx, interrupted); err != nil {
				return interrupted, err
			}
			return interrupted, fmt.Errorf("runtime: empty non-terminal turn with stop reason %q", result.StopReason)
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
		"model_lane":    runModelLane,
		"model_id":      runModelID,
	})
	if err != nil {
		return model.Run{}, fmt.Errorf("marshal run_completed payload: %w", err)
	}
	completedAt := time.Now().UTC()
	completedEvent := model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "run_completed",
		PayloadJSON:    completedPayload,
		CreatedAt:      completedAt,
	}
	if err := r.convStore.AppendEvent(ctx, completedEvent); err != nil {
		return model.Run{}, fmt.Errorf("journal run_completed: %w", err)
	}

	if r.eventSink != nil {
		_ = r.eventSink.Emit(ctx, runID, model.ReplayDelta{
			EventID:    completedEvent.ID,
			RunID:      runID,
			Kind:       "run_completed",
			OccurredAt: completedAt,
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

func (r *Runtime) promoteRunObjectiveMemory(ctx context.Context, run model.Run, objective string) error {
	if r.memory == nil {
		return nil
	}
	candidate, ok := memory.CandidateFromRunObjective(run, objective)
	if !ok {
		return nil
	}
	if err := r.memory.PromoteCandidate(ctx, candidate); err != nil {
		return fmt.Errorf("promote run objective memory: %w", err)
	}
	return nil
}

type toolCallOutcome struct {
	events []model.Event
	paused bool
}

func (r *Runtime) executeToolCalls(
	ctx context.Context,
	runID string,
	conversationID string,
	agentID string,
	sessionID string,
	cwd string,
	runAuthority authority.Envelope,
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
			event, _, err := r.recordToolCall(ctx, conversationID, runID, sessionID, cwd, runAuthority, agent, tc, nil, model.DecisionDeny, "tool not found", "")
			if err != nil {
				return outcome, err
			}
			outcome.events = append(outcome.events, event)
			continue
		}

		toolDecision := policy.DecideCall(agent, runProfile, tool.Spec(), tc.InputJSON)
		switch toolDecision.Mode {
		case model.DecisionAllow:
			event, result, err := r.recordToolCall(ctx, conversationID, runID, sessionID, cwd, runAuthority, agent, tc, tool, model.DecisionAllow, "", "")
			if err != nil {
				return outcome, err
			}
			outcome.events = append(outcome.events, event)
			if tc.ToolName == "session_spawn" && spawnedRunStillActive(result) {
				outcome.paused = true
				return outcome, nil
			}
		case model.DecisionAsk:
			bindingJSON, err := tools.BuildApprovalBindingJSON(tc.ToolName, cwd, tool.Spec(), tc.InputJSON, runAuthority)
			if err != nil {
				return outcome, fmt.Errorf("create approval binding: %w", err)
			}
			ticket, err := tools.CreateTicket(ctx, r.store, model.ApprovalRequest{
				RunID:       runID,
				ToolName:    tc.ToolName,
				ArgsJSON:    tc.InputJSON,
				BindingJSON: bindingJSON,
			})
			if err != nil {
				return outcome, fmt.Errorf("create approval ticket: %w", err)
			}
			if err := r.appendApprovalRequested(ctx, conversationID, runID, tc, ticket, toolDecision.Reason); err != nil {
				return outcome, err
			}
			if err := r.appendApprovalGate(ctx, conversationID, runID, sessionID, tc, ticket, toolDecision.Reason); err != nil {
				return outcome, err
			}
			outcome.paused = true
			return outcome, nil
		default:
			event, _, err := r.recordToolCall(ctx, conversationID, runID, sessionID, cwd, runAuthority, agent, tc, tool, model.DecisionDeny, toolDecision.Reason, "")
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
		 ORDER BY created_at ASC, rowid ASC`,
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
	return latestToolCallEvents(events), nil
}

func latestToolCallEvents(events []model.Event) []model.Event {
	if len(events) == 0 {
		return nil
	}

	keep := make([]bool, len(events))
	for i := range keep {
		keep[i] = true
	}
	latestByToolCallID := make(map[string]int, len(events))
	for i, evt := range events {
		if evt.Kind != "tool_call_recorded" {
			continue
		}
		var payload struct {
			ToolCallID string `json:"tool_call_id"`
		}
		if err := json.Unmarshal(evt.PayloadJSON, &payload); err != nil {
			continue
		}
		payload.ToolCallID = strings.TrimSpace(payload.ToolCallID)
		if payload.ToolCallID == "" {
			continue
		}
		if prior, ok := latestByToolCallID[payload.ToolCallID]; ok {
			keep[prior] = false
		}
		latestByToolCallID[payload.ToolCallID] = i
	}

	filtered := make([]model.Event, 0, len(events))
	for i, evt := range events {
		if keep[i] {
			filtered = append(filtered, evt)
		}
	}
	return filtered
}

func (r *Runtime) recordToolCall(
	ctx context.Context,
	conversationID string,
	runID string,
	sessionID string,
	cwd string,
	runAuthority authority.Envelope,
	agent model.AgentProfile,
	tc model.ToolCallRequest,
	tool tools.Tool,
	decision model.DecisionMode,
	denyReason string,
	approvalID string,
) (model.Event, model.ToolResult, error) {
	result := model.ToolResult{}
	if decision == model.DecisionAllow {
		if tool == nil {
			result.Error = "tool not found"
		} else {
			invokeCtx := tools.WithInvocationContext(ctx, tools.InvocationContext{
				CWD:        cwd,
				SessionID:  sessionID,
				Agent:      agent,
				Authority:  runAuthority,
				ApprovalID: approvalID,
				LogSink: toolInvocationLogSink{
					convStore:      r.convStore,
					eventSink:      r.eventSink,
					conversationID: conversationID,
					runID:          runID,
					toolCallID:     tc.ID,
					toolName:       tc.ToolName,
				},
			})
			invoked, err := tool.Invoke(invokeCtx, model.ToolCall(tc))
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
		return model.Event{}, model.ToolResult{}, fmt.Errorf("marshal tool result: %w", err)
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
		return model.Event{}, model.ToolResult{}, fmt.Errorf("marshal tool_call_recorded payload: %w", err)
	}
	evt := model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "tool_call_recorded",
		PayloadJSON:    payload,
	}
	if err := r.convStore.AppendEvent(ctx, evt); err != nil {
		return model.Event{}, model.ToolResult{}, fmt.Errorf("journal tool_call_recorded: %w", err)
	}
	return evt, result, nil
}

func (r *Runtime) appendApprovalRequested(
	ctx context.Context,
	conversationID string,
	runID string,
	tc model.ToolCallRequest,
	ticket model.ApprovalTicket,
	reason string,
) error {
	now := time.Now().UTC()
	payload, err := json.Marshal(map[string]any{
		"approval_id":  ticket.ID,
		"tool_call_id": tc.ID,
		"tool_name":    tc.ToolName,
		"input_json":   json.RawMessage(tc.InputJSON),
		"binding_json": json.RawMessage(ticket.BindingJSON),
		"reason":       reason,
	})
	if err != nil {
		return fmt.Errorf("marshal approval_requested payload: %w", err)
	}
	evt := model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "approval_requested",
		PayloadJSON:    payload,
		CreatedAt:      now,
	}
	if err := r.convStore.AppendEvent(ctx, evt); err != nil {
		return fmt.Errorf("journal approval_requested: %w", err)
	}
	if r.eventSink != nil {
		_ = r.eventSink.Emit(ctx, runID, model.ReplayDelta{
			EventID:     evt.ID,
			RunID:       runID,
			Kind:        "approval_requested",
			PayloadJSON: payload,
			OccurredAt:  now,
		})
	}
	return nil
}

func buildApprovalPromptTitle(tc model.ToolCallRequest) string {
	return buildApprovalPromptTitleForLanguage("", tc)
}

func buildApprovalPromptTitleForLanguage(languageHint string, tc model.ToolCallRequest) string {
	if strings.TrimSpace(tc.ToolName) == "" {
		return i18n.DefaultCatalog.Format(languageHint, i18n.MessageApprovalPromptTitle, nil)
	}
	return i18n.DefaultCatalog.Format(languageHint, i18n.MessageApprovalPromptTitleWithTool, map[string]string{
		"tool_name": tc.ToolName,
	})
}

func buildApprovalPromptBody(ticket model.ApprovalTicket, reason string) string {
	return buildApprovalPromptBodyForLanguage("", ticket, reason)
}

func buildApprovalPromptBodyForLanguage(languageHint string, ticket model.ApprovalTicket, reason string) string {
	lines := make([]string, 0, 4)
	if summary := authority.BindingSummaryJSON(ticket.BindingJSON); strings.TrimSpace(summary) != "" {
		lines = append(lines, i18n.DefaultCatalog.Format(languageHint, i18n.MessageApprovalBlockedAction, map[string]string{
			"summary": summary,
		}))
	}
	if trimmedReason := strings.TrimSpace(reason); trimmedReason != "" {
		lines = append(lines, i18n.DefaultCatalog.Format(languageHint, i18n.MessageApprovalReason, map[string]string{
			"reason": ensureSentence(trimmedReason),
		}))
	}
	lines = append(lines, i18n.DefaultCatalog.Format(languageHint, i18n.MessageApprovalReplyInstruction, nil))
	lines = append(lines, i18n.DefaultCatalog.Format(languageHint, i18n.MessageApprovalCommandFallback, map[string]string{
		"approval_id": ticket.ID,
	}))
	return strings.Join(lines, "\n")
}

func buildApprovalPromptMetadata(ticket model.ApprovalTicket) ([]byte, error) {
	return buildApprovalPromptMetadataForLanguage("", ticket)
}

func buildApprovalPromptMetadataForLanguage(languageHint string, ticket model.ApprovalTicket) ([]byte, error) {
	return json.Marshal(model.OutboundIntentMetadata{
		ActionButtons: []model.OutboundActionButton{
			{
				Label: i18n.DefaultCatalog.Format(languageHint, i18n.MessageApprovalButtonApprove, nil),
				Value: fmt.Sprintf("/approve %s allow-once", ticket.ID),
			},
			{
				Label: i18n.DefaultCatalog.Format(languageHint, i18n.MessageApprovalButtonDeny, nil),
				Value: fmt.Sprintf("/approve %s deny", ticket.ID),
			},
		},
	})
}

func buildApprovalResolutionBody(decision string) string {
	return buildApprovalResolutionBodyForLanguage("", decision)
}

func buildApprovalResolutionBodyForLanguage(languageHint string, decision string) string {
	switch decision {
	case "approved":
		return i18n.DefaultCatalog.Format(languageHint, i18n.MessageApprovalResolvedApproved, nil)
	case "denied":
		return i18n.DefaultCatalog.Format(languageHint, i18n.MessageApprovalResolvedDenied, nil)
	default:
		return i18n.DefaultCatalog.Format(languageHint, i18n.MessageApprovalResolvedUpdated, nil)
	}
}

func ensureSentence(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	if strings.HasSuffix(trimmed, ".") || strings.HasSuffix(trimmed, "!") || strings.HasSuffix(trimmed, "?") || strings.HasSuffix(trimmed, "…") {
		return trimmed
	}
	return trimmed + "."
}

func (r *Runtime) appendConversationGateOpened(
	ctx context.Context,
	conversationID string,
	runID string,
	sessionID string,
	ticket model.ApprovalTicket,
	title string,
	body string,
	languageHint string,
) error {
	payload, err := json.Marshal(map[string]any{
		"gate_id":       ticket.ID,
		"session_id":    sessionID,
		"kind":          string(model.ConversationGateApproval),
		"status":        string(model.ConversationGatePending),
		"approval_id":   ticket.ID,
		"title":         title,
		"body":          body,
		"language_hint": strings.TrimSpace(languageHint),
		"options":       []string{"approve", "deny"},
		"metadata": map[string]any{
			"tool_name": ticket.ToolName,
		},
	})
	if err != nil {
		return fmt.Errorf("marshal conversation_gate_opened payload: %w", err)
	}
	if err := r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "conversation_gate_opened",
		PayloadJSON:    payload,
		CreatedAt:      time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("journal conversation_gate_opened: %w", err)
	}
	return nil
}

func (r *Runtime) appendConversationGateResolved(
	ctx context.Context,
	conversationID string,
	runID string,
	gateID string,
	status string,
	decision string,
) error {
	payload, err := json.Marshal(map[string]any{
		"gate_id":  gateID,
		"status":   status,
		"decision": decision,
	})
	if err != nil {
		return fmt.Errorf("marshal conversation_gate_resolved payload: %w", err)
	}
	if err := r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "conversation_gate_resolved",
		PayloadJSON:    payload,
		CreatedAt:      time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("journal conversation_gate_resolved: %w", err)
	}
	return nil
}

func (r *Runtime) appendApprovalGate(
	ctx context.Context,
	conversationID string,
	runID string,
	sessionID string,
	tc model.ToolCallRequest,
	ticket model.ApprovalTicket,
	reason string,
) error {
	languageHint, err := r.loadLatestSessionLanguageHint(ctx, sessionID)
	if err != nil {
		return err
	}
	title := buildApprovalPromptTitleForLanguage(languageHint, tc)
	body := buildApprovalPromptBodyForLanguage(languageHint, ticket, reason)
	if err := r.appendConversationGateOpened(ctx, conversationID, runID, sessionID, ticket, title, body, languageHint); err != nil {
		return err
	}
	if sessionID == "" {
		return nil
	}
	messageBody := title
	if strings.TrimSpace(body) != "" {
		messageBody += "\n" + body
	}
	metadataJSON, err := buildApprovalPromptMetadataForLanguage(languageHint, ticket)
	if err != nil {
		return err
	}
	messageID, err := r.appendSessionMessage(
		ctx,
		conversationID,
		runID,
		sessionID,
		sessionID,
		model.MessageAssistant,
		messageBody,
		model.SessionMessageProvenance{
			Kind:        model.MessageProvenanceAssistantTurn,
			SourceRunID: runID,
		},
	)
	if err != nil {
		return err
	}
	if err := r.queueConversationOutboundIntent(ctx, runID, conversationID, sessionID, messageID, messageBody, metadataJSON); err != nil {
		return err
	}
	return nil
}

func (r *Runtime) loadLatestSessionLanguageHint(ctx context.Context, sessionID string) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", nil
	}
	var provenanceJSON []byte
	err := r.store.RawDB().QueryRowContext(ctx,
		`SELECT COALESCE(provenance_json, x'')
		 FROM session_messages
		 WHERE session_id = ? AND kind = 'user'
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		sessionID,
	).Scan(&provenanceJSON)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load session language hint: %w", err)
	}
	if len(provenanceJSON) == 0 {
		return "", nil
	}
	var provenance model.SessionMessageProvenance
	if err := json.Unmarshal(provenanceJSON, &provenance); err != nil {
		return "", fmt.Errorf("decode session language hint: %w", err)
	}
	return strings.TrimSpace(provenance.LanguageHint), nil
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

func (r *Runtime) interruptRun(ctx context.Context, run model.Run) (model.Run, error) {
	if err := r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: run.ConversationID,
		RunID:          run.ID,
		Kind:           "run_interrupted",
	}); err != nil {
		return model.Run{}, fmt.Errorf("journal run_interrupted: %w", err)
	}
	interrupted, err := r.loadRun(ctx, run.ID)
	if err != nil {
		return model.Run{}, err
	}
	return interrupted, nil
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

func spawnedRunStillActive(result model.ToolResult) bool {
	if strings.TrimSpace(result.Output) == "" {
		return false
	}
	var payload struct {
		Status model.RunStatus `json:"status"`
	}
	if err := json.Unmarshal([]byte(result.Output), &payload); err != nil {
		return false
	}
	switch payload.Status {
	case model.RunStatusPending, model.RunStatusActive, model.RunStatusNeedsApproval:
		return true
	default:
		return false
	}
}

func isTerminalRunStatus(status model.RunStatus) bool {
	switch status {
	case model.RunStatusCompleted, model.RunStatusInterrupted, model.RunStatusFailed:
		return true
	default:
		return false
	}
}

func (r *Runtime) childTerminalMessage(ctx context.Context, run model.Run) (string, error) {
	output, err := r.latestAssistantMessage(ctx, run.SessionID)
	if err != nil {
		return "", err
	}
	output = strings.TrimSpace(output)

	switch run.Status {
	case model.RunStatusCompleted:
		if output != "" {
			return fmt.Sprintf("%s completed: %s", run.AgentID, output), nil
		}
		return fmt.Sprintf("%s completed.", run.AgentID), nil
	case model.RunStatusInterrupted:
		if output != "" {
			return fmt.Sprintf("%s was interrupted: %s", run.AgentID, output), nil
		}
		reason, err := r.interruptedRunReason(ctx, run.ID)
		if err != nil {
			return "", err
		}
		if reason != "" {
			return fmt.Sprintf("%s was interrupted: %s", run.AgentID, reason), nil
		}
		return fmt.Sprintf("%s was interrupted.", run.AgentID), nil
	case model.RunStatusFailed:
		if output != "" {
			return fmt.Sprintf("%s failed: %s", run.AgentID, output), nil
		}
		return fmt.Sprintf("%s failed.", run.AgentID), nil
	default:
		return "", nil
	}
}

func (r *Runtime) interruptedRunReason(ctx context.Context, runID string) (string, error) {
	var payloadJSON []byte
	err := r.store.RawDB().QueryRowContext(ctx,
		`SELECT payload_json
		 FROM events
		 WHERE run_id = ? AND kind = 'budget_stop'
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		runID,
	).Scan(&payloadJSON)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("runtime: load interrupted run reason: %w", err)
	}

	var payload struct {
		LimitType  string `json:"limit_type"`
		TokensUsed int    `json:"tokens_used"`
		TokenCap   int    `json:"token_cap"`
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return "", fmt.Errorf("runtime: decode interrupted run reason: %w", err)
	}
	if payload.LimitType != "per_run_tokens" {
		return "", nil
	}
	switch {
	case payload.TokenCap > 0 && payload.TokensUsed > 0:
		return fmt.Sprintf("hit the per-run token budget (%d used / %d cap)", payload.TokensUsed, payload.TokenCap), nil
	case payload.TokenCap > 0:
		return fmt.Sprintf("hit the per-run token budget (cap %d)", payload.TokenCap), nil
	default:
		return "hit the per-run token budget", nil
	}
}

func (r *Runtime) appendUpdatedParentSpawnResult(ctx context.Context, parent model.Run, child model.Run, body string) error {
	rows, err := r.store.RawDB().QueryContext(ctx,
		`SELECT payload_json
		 FROM events
		 WHERE run_id = ? AND kind = 'tool_call_recorded'
		 ORDER BY created_at ASC, rowid ASC`,
		parent.ID,
	)
	if err != nil {
		return fmt.Errorf("load parent spawn tool call: %w", err)
	}
	defer rows.Close()

	var match struct {
		ToolCallID string          `json:"tool_call_id"`
		ToolName   string          `json:"tool_name"`
		InputJSON  json.RawMessage `json:"input_json"`
		Decision   string          `json:"decision"`
		ApprovalID string          `json:"approval_id"`
	}
	found := false
	for rows.Next() {
		var payloadJSON []byte
		if err := rows.Scan(&payloadJSON); err != nil {
			return fmt.Errorf("scan parent spawn tool call: %w", err)
		}
		var payload struct {
			ToolCallID string          `json:"tool_call_id"`
			ToolName   string          `json:"tool_name"`
			InputJSON  json.RawMessage `json:"input_json"`
			Decision   string          `json:"decision"`
			ApprovalID string          `json:"approval_id"`
		}
		if err := json.Unmarshal(payloadJSON, &payload); err != nil {
			return fmt.Errorf("decode parent spawn tool call: %w", err)
		}
		if payload.ToolName != "session_spawn" || payload.Decision != string(model.DecisionAllow) {
			continue
		}
		match.ToolCallID = payload.ToolCallID
		match.ToolName = payload.ToolName
		match.InputJSON = append([]byte(nil), payload.InputJSON...)
		match.Decision = payload.Decision
		match.ApprovalID = payload.ApprovalID
		found = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate parent spawn tool calls: %w", err)
	}
	if !found {
		return nil
	}

	spawnOutputJSON, err := json.Marshal(tools.SessionSpawnResult{
		RunID:     child.ID,
		SessionID: child.SessionID,
		AgentID:   child.AgentID,
		Status:    child.Status,
		Output:    body,
	})
	if err != nil {
		return fmt.Errorf("marshal updated spawn result: %w", err)
	}
	toolResultJSON, err := json.Marshal(model.ToolResult{Output: string(spawnOutputJSON)})
	if err != nil {
		return fmt.Errorf("marshal updated spawn tool result: %w", err)
	}
	payloadJSON, err := json.Marshal(map[string]any{
		"tool_call_id": match.ToolCallID,
		"tool_name":    match.ToolName,
		"input_json":   json.RawMessage(match.InputJSON),
		"output_json":  json.RawMessage(toolResultJSON),
		"decision":     match.Decision,
		"approval_id":  match.ApprovalID,
	})
	if err != nil {
		return fmt.Errorf("marshal updated parent spawn tool call: %w", err)
	}
	if err := r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: parent.ConversationID,
		RunID:          parent.ID,
		Kind:           "tool_call_recorded",
		PayloadJSON:    payloadJSON,
	}); err != nil {
		return fmt.Errorf("journal updated parent spawn tool call: %w", err)
	}
	return nil
}

func (r *Runtime) resumeParentAfterChildTerminal(ctx context.Context, child model.Run) error {
	if child.ParentRunID == "" || !isTerminalRunStatus(child.Status) {
		return nil
	}

	parent, err := r.loadRun(ctx, child.ParentRunID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if isTerminalRunStatus(parent.Status) {
		return nil
	}

	workerSession, err := sessions.NewService(r.store, r.convStore).LoadSession(ctx, child.SessionID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(workerSession.ControllerSessionID) == "" {
		return nil
	}

	body, err := r.childTerminalMessage(ctx, child)
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) != "" {
		if _, err := r.appendSessionMessage(
			ctx,
			child.ConversationID,
			parent.ID,
			workerSession.ControllerSessionID,
			workerSession.ID,
			model.MessageAnnounce,
			body,
			model.SessionMessageProvenance{
				Kind:            model.MessageProvenanceInterSession,
				SourceSessionID: workerSession.ID,
				SourceRunID:     child.ID,
			},
		); err != nil {
			return err
		}
	}
	if err := r.appendUpdatedParentSpawnResult(ctx, parent, child, body); err != nil {
		return err
	}

	if child.Status != model.RunStatusCompleted {
		_, err := r.interruptRun(ctx, parent)
		return err
	}

	_, err = r.executeRunLoop(ctx, runLoopOpts{
		runID:             parent.ID,
		conversationID:    parent.ConversationID,
		agentID:           parent.AgentID,
		sessionID:         parent.SessionID,
		objective:         parent.Objective,
		cwd:               parent.CWD,
		verificationAgent: false,
	})
	return err
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

func (r *Runtime) BudgetGuard() BudgetGuard {
	return r.budget
}

func (r *Runtime) LoadBudgetSettings(ctx context.Context) error {
	rows, err := r.store.RawDB().QueryContext(ctx,
		`SELECT key, value
		 FROM settings
		 WHERE key IN ('per_run_token_budget', 'daily_cost_cap_usd')`,
	)
	if err != nil {
		return fmt.Errorf("runtime: load budget settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string]string, 2)
	for rows.Next() {
		var key string
		var value string
		if err := rows.Scan(&key, &value); err != nil {
			return fmt.Errorf("runtime: scan budget setting: %w", err)
		}
		settings[key] = value
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("runtime: iterate budget settings: %w", err)
	}
	return r.applyBudgetSettings(settings)
}

func (r *Runtime) applyBudgetSettings(settings map[string]string) error {
	if value, ok := settings["per_run_token_budget"]; ok {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			r.budget.PerRunTokenCap = 0
		} else {
			limit, err := strconv.Atoi(trimmed)
			if err != nil {
				return fmt.Errorf("runtime: parse per_run_token_budget: %w", err)
			}
			r.budget.PerRunTokenCap = limit
		}
	}
	if value, ok := settings["daily_cost_cap_usd"]; ok {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			r.budget.DailyCostCapUSD = 0
		} else {
			limit, err := strconv.ParseFloat(trimmed, 64)
			if err != nil {
				return fmt.Errorf("runtime: parse daily_cost_cap_usd: %w", err)
			}
			r.budget.DailyCostCapUSD = limit
		}
	}
	return nil
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
		`UPDATE approvals
		 SET status = 'expired', resolved_at = datetime('now')
		 WHERE status = 'pending'
		   AND (
		     created_at < datetime('now', '-24 hours')
		     OR NOT EXISTS (
		       SELECT 1
		       FROM runs
		       WHERE runs.id = approvals.run_id
		         AND runs.status = 'needs_approval'
		     )
		   )`)
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
		`SELECT id, conversation_id, agent_id, COALESCE(session_id, ''), COALESCE(team_id, ''), COALESCE(project_id, ''), COALESCE(parent_run_id, ''),
		 COALESCE(objective, ''), COALESCE(cwd, ''), COALESCE(authority_json, x'7b7d'), status, COALESCE(execution_snapshot_json, x''),
		 input_tokens, output_tokens, COALESCE(model_lane, ''), COALESCE(model_id, ''), created_at, updated_at
		 FROM runs WHERE id = ?`,
		runID,
	).Scan(
		&run.ID,
		&run.ConversationID,
		&run.AgentID,
		&run.SessionID,
		&run.TeamID,
		&run.ProjectID,
		&run.ParentRunID,
		&run.Objective,
		&run.CWD,
		&run.AuthorityJSON,
		&status,
		&run.ExecutionSnapshotJSON,
		&run.InputTokens,
		&run.OutputTokens,
		&run.ModelLane,
		&run.ModelID,
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

func (r *Runtime) visibleToolSpecs(agent model.AgentProfile) []model.ToolSpec {
	if r == nil || r.tools == nil {
		return nil
	}
	policy := &tools.Policy{}
	specs := r.tools.List()
	visible := make([]model.ToolSpec, 0, len(specs))
	for _, spec := range specs {
		decision := policy.Decide(agent, model.RunProfile{}, spec)
		if decision.Mode == model.DecisionDeny {
			continue
		}
		visible = append(visible, spec)
	}
	return visible
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

func (r *Runtime) queueConversationOutboundIntent(
	ctx context.Context,
	runID string,
	conversationID string,
	sessionID string,
	messageID string,
	body string,
	metadataJSON []byte,
) error {
	route, err := sessions.NewService(r.store, r.convStore).LoadRouteBySession(ctx, sessionID)
	if err == sessions.ErrSessionRouteNotFound {
		route, err = r.loadFrontConversationRoute(ctx, conversationID)
	}
	if err == ErrRouteNotFound || err == sessions.ErrSessionRouteNotFound {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load conversation outbound route: %w", err)
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
		return fmt.Errorf("check conversation outbound intent dedupe: %w", err)
	}
	if existing > 0 {
		return nil
	}

	_, err = r.store.RawDB().ExecContext(ctx,
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, metadata_json, dedupe_key, status, attempts, created_at)
		 VALUES (?, ?, ?, ?, ?, COALESCE(?, '{}'), ?, 'pending', 0, datetime('now'))`,
		generateID(), runID, route.ConnectorID, route.ExternalID, body, metadataJSON, dedupeKey,
	)
	if err != nil {
		return fmt.Errorf("insert conversation outbound intent: %w", err)
	}
	return nil
}

func (r *Runtime) loadFrontConversationRoute(ctx context.Context, conversationID string) (model.SessionRoute, error) {
	var route model.SessionRoute
	err := r.store.RawDB().QueryRowContext(ctx,
		`SELECT bind.id, bind.session_id, bind.thread_id, bind.connector_id, bind.account_id, bind.external_id,
		        bind.status, bind.created_at
		 FROM session_bindings bind
		 JOIN sessions sess ON sess.id = bind.session_id
		 WHERE bind.conversation_id = ? AND bind.status = 'active'
		 ORDER BY CASE sess.role WHEN 'front' THEN 0 ELSE 1 END,
		          bind.created_at DESC,
		          bind.id DESC
		 LIMIT 1`,
		conversationID,
	).Scan(
		&route.ID,
		&route.SessionID,
		&route.ThreadID,
		&route.ConnectorID,
		&route.AccountID,
		&route.ExternalID,
		&route.Status,
		&route.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return model.SessionRoute{}, ErrRouteNotFound
	}
	if err != nil {
		return model.SessionRoute{}, err
	}
	return route, nil
}

// SubmitTask starts a new root run via the web interface, resolving the
// web conversation key internally. This is the canonical entry point for
// write-path web handlers.
func (r *Runtime) SubmitTask(ctx context.Context, objective, cwd string) (model.Run, error) {
	return r.ReceiveInboundMessage(ctx, InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "default",
			ThreadID:    "main",
		},
		FrontAgentID: "assistant",
		Body:         objective,
		CWD:          cwd,
	})
}

// ResolveApproval approves or denies a pending approval ticket by its ID.
// decision must be "approved" or "denied".
func (r *Runtime) ResolveApproval(ctx context.Context, ticketID, decision string) error {
	prepared, err := r.prepareApprovalResolution(ctx, ticketID, decision)
	if err != nil {
		return err
	}
	if !prepared.hasRun {
		return nil
	}
	return r.finishApprovalResolution(ctx, prepared)
}

func (r *Runtime) ResolveApprovalAsync(ctx context.Context, ticketID, decision string) error {
	prepared, err := r.prepareApprovalResolution(ctx, ticketID, decision)
	if err != nil {
		return err
	}
	if !prepared.hasRun {
		return nil
	}

	execCtx := r.asyncCtx
	if execCtx == nil {
		execCtx = context.Background()
	}
	r.asyncWG.Add(1)
	go func(prepared approvalResolution) {
		defer r.asyncWG.Done()
		_ = r.finishApprovalResolution(execCtx, prepared)
	}(prepared)
	return nil
}

type approvalResolution struct {
	ticket   model.ApprovalTicket
	run      model.Run
	call     model.ToolCallRequest
	decision string
	hasRun   bool
}

func (r *Runtime) prepareApprovalResolution(ctx context.Context, ticketID, decision string) (approvalResolution, error) {
	switch decision {
	case "approved", "denied":
	default:
		return approvalResolution{}, fmt.Errorf("runtime: invalid approval decision %q", decision)
	}

	ticket, err := tools.LoadTicket(ctx, r.store, ticketID)
	if err != nil {
		return approvalResolution{}, err
	}
	if err := tools.ResolveTicket(ctx, r.store, ticketID, decision); err != nil {
		return approvalResolution{}, err
	}

	run, err := r.loadRun(ctx, ticket.RunID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return approvalResolution{ticket: ticket, decision: decision}, nil
		}
		return approvalResolution{}, err
	}

	toolCallID, err := r.loadApprovalToolCallID(ctx, run.ID, ticket.ID)
	if err != nil {
		return approvalResolution{}, err
	}
	if toolCallID == "" {
		toolCallID = ticket.ID
	}
	call := model.ToolCallRequest{
		ID:        toolCallID,
		ToolName:  ticket.ToolName,
		InputJSON: ticket.ArgsJSON,
	}
	if decision == "approved" {
		if err := r.appendRunResumed(ctx, run.ConversationID, run.ID, ticket.ID, decision); err != nil {
			return approvalResolution{}, err
		}
	}
	return approvalResolution{
		ticket:   ticket,
		run:      run,
		call:     call,
		decision: decision,
		hasRun:   true,
	}, nil
}

func (r *Runtime) finishApprovalResolution(ctx context.Context, prepared approvalResolution) error {
	run := prepared.run
	ticket := prepared.ticket
	call := prepared.call
	runAuthority, err := authority.DecodeEnvelope(run.AuthorityJSON)
	if err != nil {
		return fmt.Errorf("decode run authority: %w", err)
	}

	switch prepared.decision {
	case "approved":
		if err := r.appendConversationGateResolved(ctx, run.ConversationID, run.ID, ticket.ID, string(model.ConversationGateResolved), prepared.decision); err != nil {
			return err
		}
		if run.SessionID != "" {
			languageHint, err := r.loadLatestSessionLanguageHint(ctx, run.SessionID)
			if err != nil {
				return err
			}
			body := buildApprovalResolutionBodyForLanguage(languageHint, prepared.decision)
			messageID, err := r.appendSessionMessage(
				ctx,
				run.ConversationID,
				run.ID,
				run.SessionID,
				run.SessionID,
				model.MessageAssistant,
				body,
				model.SessionMessageProvenance{
					Kind:        model.MessageProvenanceAssistantTurn,
					SourceRunID: run.ID,
				},
			)
			if err != nil {
				return err
			}
			if err := r.queueConversationOutboundIntent(ctx, run.ID, run.ConversationID, run.SessionID, messageID, body, nil); err != nil {
				return err
			}
		}
		agent, err := r.agentProfileForRun(ctx, run.ID, run.AgentID)
		if err != nil {
			return err
		}
		tool, _ := r.tools.Get(ticket.ToolName)
		_, result, err := r.recordToolCall(ctx, run.ConversationID, run.ID, run.SessionID, run.CWD, runAuthority, agent, call, tool, model.DecisionAllow, "", ticket.ID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(result.Error) != "" {
			interrupted, err := r.interruptRun(ctx, run)
			if err != nil {
				return err
			}
			return r.resumeParentAfterChildTerminal(ctx, interrupted)
		}
		resumed, err := r.executeRunLoop(ctx, runLoopOpts{
			runID:             run.ID,
			conversationID:    run.ConversationID,
			agentID:           run.AgentID,
			sessionID:         run.SessionID,
			objective:         run.Objective,
			cwd:               run.CWD,
			verificationAgent: false,
		})
		if err != nil {
			return err
		}
		return r.resumeParentAfterChildTerminal(ctx, resumed)
	case "denied":
		if err := r.appendConversationGateResolved(ctx, run.ConversationID, run.ID, ticket.ID, string(model.ConversationGateResolved), prepared.decision); err != nil {
			return err
		}
		if run.SessionID != "" {
			languageHint, err := r.loadLatestSessionLanguageHint(ctx, run.SessionID)
			if err != nil {
				return err
			}
			body := buildApprovalResolutionBodyForLanguage(languageHint, prepared.decision)
			messageID, err := r.appendSessionMessage(
				ctx,
				run.ConversationID,
				run.ID,
				run.SessionID,
				run.SessionID,
				model.MessageAssistant,
				body,
				model.SessionMessageProvenance{
					Kind:        model.MessageProvenanceAssistantTurn,
					SourceRunID: run.ID,
				},
			)
			if err != nil {
				return err
			}
			if err := r.queueConversationOutboundIntent(ctx, run.ID, run.ConversationID, run.SessionID, messageID, body, nil); err != nil {
				return err
			}
		}
		agent, err := r.agentProfileForRun(ctx, run.ID, run.AgentID)
		if err != nil {
			return err
		}
		if _, _, err := r.recordToolCall(ctx, run.ConversationID, run.ID, run.SessionID, run.CWD, runAuthority, agent, call, nil, model.DecisionDeny, "approval denied", ticket.ID); err != nil {
			return err
		}
		interrupted, err := r.interruptRun(ctx, run)
		if err != nil {
			return err
		}
		return r.resumeParentAfterChildTerminal(ctx, interrupted)
	default:
		return fmt.Errorf("runtime: invalid approval decision %q", prepared.decision)
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
	if err := r.applyBudgetSettings(updates); err != nil {
		return err
	}
	return nil
}

func generateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
