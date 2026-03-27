package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	sessionkeys "github.com/canhta/gistclaw/internal/sessions"
)

type StartFrontSession struct {
	ConversationKey conversations.ConversationKey
	FrontAgentID    string
	InitialPrompt   string
	ProjectID       string
	CWD             string
}

type InboundMessageCommand struct {
	ConversationKey conversations.ConversationKey
	FrontAgentID    string
	Body            string
	SourceMessageID string
	ProjectID       string
	CWD             string
}

type SpawnCommand struct {
	ControllerSessionID string
	AgentID             string
	Prompt              string
}

type AnnounceCommand struct {
	WorkerSessionID string
	TargetSessionID string
	Body            string
}

type SteerCommand struct {
	ControllerSessionID string
	TargetSessionID     string
	Body                string
}

type AgentSendCommand struct {
	FromSessionID string
	ToSessionID   string
	Body          string
}

type SendSessionCommand struct {
	FromSessionID string
	ToSessionID   string
	Body          string
}

func (r *Runtime) StartFrontSession(ctx context.Context, cmd StartFrontSession) (model.Run, error) {
	scopedKey, project, err := r.scopeConversationKey(ctx, cmd.ConversationKey, cmd.ProjectID, cmd.CWD)
	if err != nil {
		return model.Run{}, err
	}
	cmd.ConversationKey = scopedKey
	if cmd.ProjectID == "" {
		cmd.ProjectID = project.ID
	}
	if cmd.CWD == "" {
		cmd.CWD = project.PrimaryPath
	}

	conv, err := r.convStore.Resolve(ctx, cmd.ConversationKey)
	if err != nil {
		return model.Run{}, fmt.Errorf("resolve conversation: %w", err)
	}

	threadID := normalizeThreadID(cmd.ConversationKey.ThreadID)
	sessionID, createSession, createBinding, err := r.resolveFrontSession(ctx, conv.ID, threadID)
	if err != nil {
		return model.Run{}, err
	}

	runID := generateID()
	start := StartRun{
		ConversationID: conv.ID,
		AgentID:        cmd.FrontAgentID,
		SessionID:      sessionID,
		ProjectID:      cmd.ProjectID,
		Objective:      cmd.InitialPrompt,
		CWD:            cmd.CWD,
		AccountID:      cmd.ConversationKey.AccountID,
	}
	if err := r.createRun(ctx, runID, "", start); err != nil {
		return model.Run{}, err
	}
	if createSession {
		if err := r.openSession(ctx, conv.ID, runID, "", sessionID, cmd.FrontAgentID, model.SessionRoleFront, "", ""); err != nil {
			return model.Run{}, err
		}
	}
	if createBinding {
		if err := r.bindSession(ctx, conv.ID, runID, cmd.ConversationKey, sessionID); err != nil {
			return model.Run{}, err
		}
	}
	if _, err := r.appendSessionMessage(
		ctx,
		conv.ID,
		runID,
		sessionID,
		"",
		model.MessageUser,
		cmd.InitialPrompt,
		model.SessionMessageProvenance{
			Kind:              model.MessageProvenanceInbound,
			SourceConnectorID: cmd.ConversationKey.ConnectorID,
			SourceThreadID:    normalizeThreadID(cmd.ConversationKey.ThreadID),
		},
	); err != nil {
		return model.Run{}, err
	}

	return r.executeRunLoop(ctx, runLoopOpts{
		runID:          runID,
		conversationID: conv.ID,
		agentID:        cmd.FrontAgentID,
		sessionID:      sessionID,
		objective:      cmd.InitialPrompt,
		cwd:            cmd.CWD,
	})
}

func (r *Runtime) ReceiveInboundMessage(ctx context.Context, cmd InboundMessageCommand) (model.Run, error) {
	return r.receiveInboundMessage(ctx, cmd, false)
}

func (r *Runtime) ReceiveInboundMessageAsync(ctx context.Context, cmd InboundMessageCommand) (model.Run, error) {
	return r.receiveInboundMessage(ctx, cmd, true)
}

func (r *Runtime) receiveInboundMessage(ctx context.Context, cmd InboundMessageCommand, detached bool) (model.Run, error) {
	scopedKey, project, err := r.scopeConversationKey(ctx, cmd.ConversationKey, cmd.ProjectID, cmd.CWD)
	if err != nil {
		return model.Run{}, err
	}
	cmd.ConversationKey = scopedKey
	if cmd.ProjectID == "" {
		cmd.ProjectID = project.ID
	}
	if cmd.CWD == "" {
		cmd.CWD = project.PrimaryPath
	}

	conv, err := r.convStore.Resolve(ctx, cmd.ConversationKey)
	if err != nil {
		return model.Run{}, fmt.Errorf("resolve conversation: %w", err)
	}

	threadID := normalizeThreadID(cmd.ConversationKey.ThreadID)
	if cmd.SourceMessageID != "" {
		existing, err := r.loadInboundReceiptRun(
			ctx,
			conv.ID,
			cmd.ConversationKey.ConnectorID,
			cmd.ConversationKey.AccountID,
			threadID,
			cmd.SourceMessageID,
		)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return model.Run{}, err
		}
	}
	sessionID, createSession, createBinding, err := r.resolveFrontSession(ctx, conv.ID, threadID)
	if err != nil {
		return model.Run{}, err
	}

	opts := inboundRunOptions{
		conversationID:  conv.ID,
		sessionID:       sessionID,
		createSession:   createSession,
		createBinding:   createBinding,
		agentID:         cmd.FrontAgentID,
		body:            cmd.Body,
		projectID:       cmd.ProjectID,
		cwd:             cmd.CWD,
		key:             cmd.ConversationKey,
		threadID:        threadID,
		sourceMessageID: cmd.SourceMessageID,
	}
	var run model.Run
	if detached {
		run, err = r.startInboundRunAsync(ctx, opts)
	} else {
		run, err = r.startInboundRun(ctx, opts)
	}
	if errors.Is(err, conversations.ErrDuplicateInboundMessage) && cmd.SourceMessageID != "" {
		return r.loadInboundReceiptRun(
			ctx,
			conv.ID,
			cmd.ConversationKey.ConnectorID,
			cmd.ConversationKey.AccountID,
			threadID,
			cmd.SourceMessageID,
		)
	}
	return run, err
}

func (r *Runtime) Spawn(ctx context.Context, cmd SpawnCommand) (model.Run, error) {
	controllerSession, controllerRun, err := r.loadSessionRun(ctx, cmd.ControllerSessionID)
	if err != nil {
		return model.Run{}, err
	}

	runID := generateID()
	workerSessionID := generateID()
	start := StartRun{
		ConversationID: controllerSession.ConversationID,
		AgentID:        cmd.AgentID,
		SessionID:      workerSessionID,
		ProjectID:      controllerRun.ProjectID,
		Objective:      cmd.Prompt,
		CWD:            controllerRun.CWD,
	}
	if err := r.createRun(ctx, runID, controllerRun.ID, start); err != nil {
		return model.Run{}, err
	}
	if err := r.openSession(
		ctx,
		controllerSession.ConversationID,
		runID,
		controllerRun.ID,
		workerSessionID,
		cmd.AgentID,
		model.SessionRoleWorker,
		controllerSession.ID,
		controllerSession.ID,
	); err != nil {
		return model.Run{}, err
	}
	if _, err := r.appendSessionMessage(
		ctx,
		controllerSession.ConversationID,
		runID,
		workerSessionID,
		controllerSession.ID,
		model.MessageSpawn,
		cmd.Prompt,
		model.SessionMessageProvenance{
			Kind:            model.MessageProvenanceInterSession,
			SourceSessionID: controllerSession.ID,
			SourceRunID:     controllerRun.ID,
		},
	); err != nil {
		return model.Run{}, err
	}

	return r.executeRunLoop(ctx, runLoopOpts{
		runID:          runID,
		conversationID: controllerSession.ConversationID,
		agentID:        cmd.AgentID,
		sessionID:      workerSessionID,
		objective:      cmd.Prompt,
		cwd:            controllerRun.CWD,
	})
}

func (r *Runtime) Announce(ctx context.Context, cmd AnnounceCommand) error {
	return r.directSessionMessage(ctx, cmd.WorkerSessionID, cmd.TargetSessionID, model.MessageAnnounce, cmd.Body)
}

func (r *Runtime) Steer(ctx context.Context, cmd SteerCommand) error {
	return r.directSessionMessage(ctx, cmd.ControllerSessionID, cmd.TargetSessionID, model.MessageSteer, cmd.Body)
}

func (r *Runtime) AgentSend(ctx context.Context, cmd AgentSendCommand) error {
	return r.directSessionMessage(ctx, cmd.FromSessionID, cmd.ToSessionID, model.MessageAgentSend, cmd.Body)
}

func (r *Runtime) SendSession(ctx context.Context, cmd SendSessionCommand) (model.Run, error) {
	opts := sendSessionOptions{
		fromSessionID: cmd.FromSessionID,
		toSessionID:   cmd.ToSessionID,
		body:          cmd.Body,
	}
	if cmd.FromSessionID == "" {
		opts.kind = model.MessageUser
		opts.provenance = model.SessionMessageProvenance{
			Kind:              model.MessageProvenanceInbound,
			SourceConnectorID: "web",
		}
	}
	return r.sendSession(ctx, opts)
}

type sendSessionOptions struct {
	fromSessionID string
	toSessionID   string
	body          string
	kind          model.SessionMessageKind
	provenance    model.SessionMessageProvenance
}

func (r *Runtime) sendSession(ctx context.Context, opts sendSessionOptions) (model.Run, error) {
	if strings.TrimSpace(opts.body) == "" {
		return model.Run{}, fmt.Errorf("runtime: session message body is required")
	}

	targetSession, targetRun, err := r.loadSessionRun(ctx, opts.toSessionID)
	if err != nil {
		return model.Run{}, err
	}

	kind := opts.kind
	if kind == "" {
		kind = model.MessageAgentSend
	}
	senderSessionID := ""
	provenance := opts.provenance
	if opts.fromSessionID != "" {
		sourceSession, sourceRun, err := r.loadSessionRun(ctx, opts.fromSessionID)
		if err != nil {
			return model.Run{}, err
		}
		if sourceSession.ConversationID != targetSession.ConversationID {
			return model.Run{}, fmt.Errorf(
				"runtime: session %s cannot message session %s across conversations",
				opts.fromSessionID,
				opts.toSessionID,
			)
		}
		senderSessionID = sourceSession.ID
		if provenance == (model.SessionMessageProvenance{}) {
			provenance = model.SessionMessageProvenance{
				Kind:            model.MessageProvenanceInterSession,
				SourceSessionID: sourceSession.ID,
				SourceRunID:     sourceRun.ID,
			}
		}
	}

	parentRunID := ""
	if targetSession.Role == model.SessionRoleWorker {
		parentRunID = targetRun.ParentRunID
		if parentRunID == "" {
			parentRunID = targetRun.ID
		}
	}

	runID := generateID()
	start := StartRun{
		ConversationID: targetSession.ConversationID,
		AgentID:        targetSession.AgentID,
		SessionID:      targetSession.ID,
		ProjectID:      targetRun.ProjectID,
		Objective:      opts.body,
		CWD:            targetRun.CWD,
	}
	if err := r.createRun(ctx, runID, parentRunID, start); err != nil {
		return model.Run{}, err
	}
	if _, err := r.appendSessionMessage(
		ctx,
		targetSession.ConversationID,
		runID,
		targetSession.ID,
		senderSessionID,
		kind,
		opts.body,
		provenance,
	); err != nil {
		return model.Run{}, err
	}

	return r.executeRunLoop(ctx, runLoopOpts{
		runID:          runID,
		conversationID: targetSession.ConversationID,
		agentID:        targetSession.AgentID,
		sessionID:      targetSession.ID,
		objective:      opts.body,
		cwd:            targetRun.CWD,
	})
}

func (r *Runtime) directSessionMessage(
	ctx context.Context,
	sourceSessionID string,
	targetSessionID string,
	kind model.SessionMessageKind,
	body string,
) error {
	sourceSession, sourceRun, err := r.loadSessionRun(ctx, sourceSessionID)
	if err != nil {
		return err
	}
	targetSession, targetRun, err := r.loadSessionRun(ctx, targetSessionID)
	if err != nil {
		return err
	}
	if sourceSession.ConversationID != targetSession.ConversationID {
		return fmt.Errorf("runtime: session %s cannot message session %s across conversations", sourceSessionID, targetSessionID)
	}
	_, err = r.appendSessionMessage(
		ctx,
		targetSession.ConversationID,
		targetRun.ID,
		targetSession.ID,
		sourceSession.ID,
		kind,
		body,
		model.SessionMessageProvenance{
			Kind:            model.MessageProvenanceInterSession,
			SourceSessionID: sourceSession.ID,
			SourceRunID:     sourceRun.ID,
		},
	)
	return err
}

func (r *Runtime) loadSessionRun(ctx context.Context, sessionID string) (model.Session, model.Run, error) {
	sessionSvc := sessionkeys.NewService(r.store, r.convStore)
	session, err := sessionSvc.LoadSession(ctx, sessionID)
	if err != nil {
		return model.Session{}, model.Run{}, err
	}

	run, err := r.loadPreferredRunForSession(ctx, sessionID)
	if err != nil {
		return model.Session{}, model.Run{}, err
	}
	return session, run, nil
}

func (r *Runtime) loadPreferredRunForSession(ctx context.Context, sessionID string) (model.Run, error) {
	var run model.Run
	var status string
	err := r.store.RawDB().QueryRowContext(ctx,
		`SELECT id, conversation_id, agent_id, COALESCE(session_id, ''), COALESCE(team_id, ''), COALESCE(project_id, ''), COALESCE(parent_run_id, ''),
		        COALESCE(objective, ''), COALESCE(cwd, ''), COALESCE(authority_json, x'7b7d'), status, COALESCE(execution_snapshot_json, x''),
		        input_tokens, output_tokens, created_at, updated_at
		 FROM runs
		 WHERE session_id = ?
		 ORDER BY CASE status
		              WHEN 'active' THEN 0
		              WHEN 'pending' THEN 1
		              ELSE 2
		          END,
		          updated_at DESC,
		          created_at DESC,
		          id DESC
		 LIMIT 1`,
		sessionID,
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
		&run.CreatedAt,
		&run.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return model.Run{}, fmt.Errorf("runtime: session %s has no runs", sessionID)
	}
	if err != nil {
		return model.Run{}, fmt.Errorf("load run for session %s: %w", sessionID, err)
	}

	run.Status = model.RunStatus(status)
	return run, nil
}

func (r *Runtime) openSession(
	ctx context.Context,
	conversationID string,
	runID string,
	parentRunID string,
	sessionID string,
	agentID string,
	role model.SessionRole,
	parentSessionID string,
	controllerSessionID string,
) error {
	event, err := newSessionOpenedEvent(
		conversationID,
		runID,
		parentRunID,
		sessionID,
		agentID,
		role,
		parentSessionID,
		controllerSessionID,
		time.Time{},
	)
	if err != nil {
		return err
	}

	if err := r.convStore.AppendEvent(ctx, event); err != nil {
		return fmt.Errorf("journal session_opened: %w", err)
	}

	return nil
}

func (r *Runtime) resolveFrontSession(ctx context.Context, conversationID string, threadID string) (string, bool, bool, error) {
	var sessionID string
	err := r.store.RawDB().QueryRowContext(ctx,
		`SELECT session_id
		 FROM session_bindings
		 WHERE conversation_id = ? AND thread_id = ? AND status = 'active'
		 ORDER BY created_at DESC
		 LIMIT 1`,
		conversationID,
		threadID,
	).Scan(&sessionID)
	if err == nil {
		return sessionID, false, false, nil
	}
	if err != sql.ErrNoRows {
		return "", false, false, fmt.Errorf("runtime: load session binding: %w", err)
	}

	key := sessionkeys.BuildFrontSessionKey(conversationID)
	err = r.store.RawDB().QueryRowContext(ctx,
		"SELECT id FROM sessions WHERE key = ?",
		key,
	).Scan(&sessionID)
	if err == nil {
		return sessionID, false, true, nil
	}
	if err != sql.ErrNoRows {
		return "", false, false, fmt.Errorf("runtime: load front session: %w", err)
	}

	return generateID(), true, true, nil
}

func (r *Runtime) bindSession(
	ctx context.Context,
	conversationID string,
	runID string,
	key conversations.ConversationKey,
	sessionID string,
) error {
	event, err := newSessionBoundEvent(conversationID, runID, key, sessionID, time.Time{})
	if err != nil {
		return err
	}

	if err := r.convStore.AppendEvent(ctx, event); err != nil {
		return fmt.Errorf("journal session_bound: %w", err)
	}

	return nil
}

func normalizeThreadID(threadID string) string {
	if threadID == "" {
		return "main"
	}
	return threadID
}

type inboundRunOptions struct {
	conversationID  string
	sessionID       string
	createSession   bool
	createBinding   bool
	agentID         string
	body            string
	projectID       string
	cwd             string
	key             conversations.ConversationKey
	threadID        string
	sourceMessageID string
}

func (r *Runtime) startInboundRun(ctx context.Context, opts inboundRunOptions) (model.Run, error) {
	prepared, err := r.prepareInboundRun(ctx, opts)
	if err != nil {
		return model.Run{}, err
	}
	return r.executeRunLoop(ctx, prepared.loopOpts)
}

type preparedInboundRun struct {
	run      model.Run
	loopOpts runLoopOpts
}

func (r *Runtime) startInboundRunAsync(ctx context.Context, opts inboundRunOptions) (model.Run, error) {
	prepared, err := r.prepareInboundRun(ctx, opts)
	if err != nil {
		return model.Run{}, err
	}

	r.executeRunLoopAsync(prepared.loopOpts)
	return prepared.run, nil
}

func (r *Runtime) prepareInboundRun(ctx context.Context, opts inboundRunOptions) (preparedInboundRun, error) {
	now := time.Now().UTC()
	runID := generateID()
	start := StartRun{
		ConversationID: opts.conversationID,
		AgentID:        opts.agentID,
		SessionID:      opts.sessionID,
		ProjectID:      opts.projectID,
		Objective:      opts.body,
		CWD:            opts.cwd,
		AccountID:      opts.key.AccountID,
	}
	start, err := r.prepareStartRun(ctx, "", start)
	if err != nil {
		return preparedInboundRun{}, err
	}
	if err := r.prepareRunStart(ctx, "", start); err != nil {
		return preparedInboundRun{}, err
	}

	events := make([]model.Event, 0, 5)
	runEvent, err := newRunStartedEvent(opts.conversationID, runID, "", start, now)
	if err != nil {
		return preparedInboundRun{}, err
	}
	events = append(events, runEvent)
	if opts.createSession {
		event, err := newSessionOpenedEvent(
			opts.conversationID,
			runID,
			"",
			opts.sessionID,
			opts.agentID,
			model.SessionRoleFront,
			"",
			"",
			now,
		)
		if err != nil {
			return preparedInboundRun{}, err
		}
		events = append(events, event)
	}
	if opts.createBinding {
		event, err := newSessionBoundEvent(opts.conversationID, runID, opts.key, opts.sessionID, now)
		if err != nil {
			return preparedInboundRun{}, err
		}
		events = append(events, event)
	}

	provenance := model.SessionMessageProvenance{
		Kind:              model.MessageProvenanceInbound,
		SourceConnectorID: opts.key.ConnectorID,
		SourceThreadID:    opts.threadID,
		SourceMessageID:   opts.sourceMessageID,
	}
	messageID := generateID()
	messageEvent, err := newSessionMessageAddedEvent(
		opts.conversationID,
		runID,
		opts.sessionID,
		"",
		model.MessageUser,
		opts.body,
		provenance,
		messageID,
		now,
	)
	if err != nil {
		return preparedInboundRun{}, err
	}
	events = append(events, messageEvent)
	if opts.sourceMessageID != "" {
		event, err := newInboundMessageRecordedEvent(
			opts.conversationID,
			runID,
			opts.key.ConnectorID,
			opts.key.AccountID,
			opts.threadID,
			opts.sourceMessageID,
			opts.sessionID,
			messageID,
			now,
		)
		if err != nil {
			return preparedInboundRun{}, err
		}
		events = append(events, event)
	}

	if err := r.convStore.AppendEvents(ctx, events); err != nil {
		return preparedInboundRun{}, err
	}
	if err := r.finishRunStart(ctx, runID, "", start, now, runEvent.ID); err != nil {
		return preparedInboundRun{}, err
	}

	run, err := r.loadRun(ctx, runID)
	if err != nil {
		return preparedInboundRun{}, err
	}

	return preparedInboundRun{
		run: run,
		loopOpts: runLoopOpts{
			runID:          runID,
			conversationID: opts.conversationID,
			agentID:        opts.agentID,
			sessionID:      opts.sessionID,
			objective:      opts.body,
			cwd:            opts.cwd,
		},
	}, nil
}

func (r *Runtime) loadInboundReceiptRun(
	ctx context.Context,
	conversationID string,
	connectorID string,
	accountID string,
	threadID string,
	sourceMessageID string,
) (model.Run, error) {
	var runID string
	err := r.store.RawDB().QueryRowContext(ctx,
		`SELECT run_id
		 FROM inbound_receipts
		 WHERE conversation_id = ? AND connector_id = ? AND account_id = ? AND thread_id = ? AND source_message_id = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		conversationID,
		connectorID,
		accountID,
		threadID,
		sourceMessageID,
	).Scan(&runID)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.Run{}, sql.ErrNoRows
		}
		return model.Run{}, fmt.Errorf("runtime: load inbound receipt: %w", err)
	}
	run, err := r.loadRun(ctx, runID)
	if err != nil {
		return model.Run{}, fmt.Errorf("runtime: load inbound receipt run: %w", err)
	}
	return run, nil
}

func (r *Runtime) appendSessionMessage(
	ctx context.Context,
	conversationID string,
	runID string,
	sessionID string,
	senderSessionID string,
	kind model.SessionMessageKind,
	body string,
	provenance model.SessionMessageProvenance,
) (string, error) {
	messageID := generateID()
	event, err := newSessionMessageAddedEvent(
		conversationID,
		runID,
		sessionID,
		senderSessionID,
		kind,
		body,
		provenance,
		messageID,
		time.Time{},
	)
	if err != nil {
		return "", err
	}

	if err := r.convStore.AppendEvent(ctx, event); err != nil {
		return "", fmt.Errorf("journal session_message_added: %w", err)
	}

	return messageID, nil
}

func newSessionOpenedEvent(
	conversationID string,
	runID string,
	parentRunID string,
	sessionID string,
	agentID string,
	role model.SessionRole,
	parentSessionID string,
	controllerSessionID string,
	now time.Time,
) (model.Event, error) {
	key := sessionkeys.BuildFrontSessionKey(conversationID)
	if role == model.SessionRoleWorker {
		key = sessionkeys.BuildWorkerSessionKey(parentSessionID, agentID, sessionID)
	}

	payload, err := json.Marshal(map[string]any{
		"session_id":            sessionID,
		"key":                   key,
		"agent_id":              agentID,
		"role":                  role,
		"parent_session_id":     parentSessionID,
		"controller_session_id": controllerSessionID,
		"status":                "active",
	})
	if err != nil {
		return model.Event{}, fmt.Errorf("marshal session_opened payload: %w", err)
	}

	return model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		ParentRunID:    parentRunID,
		Kind:           "session_opened",
		PayloadJSON:    payload,
		CreatedAt:      now,
	}, nil
}

func newSessionBoundEvent(
	conversationID string,
	runID string,
	key conversations.ConversationKey,
	sessionID string,
	now time.Time,
) (model.Event, error) {
	payload, err := json.Marshal(map[string]any{
		"thread_id":    normalizeThreadID(key.ThreadID),
		"session_id":   sessionID,
		"connector_id": key.ConnectorID,
		"account_id":   key.AccountID,
		"external_id":  key.ExternalID,
		"status":       "active",
	})
	if err != nil {
		return model.Event{}, fmt.Errorf("marshal session_bound payload: %w", err)
	}

	return model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "session_bound",
		PayloadJSON:    payload,
		CreatedAt:      now,
	}, nil
}

func newSessionMessageAddedEvent(
	conversationID string,
	runID string,
	sessionID string,
	senderSessionID string,
	kind model.SessionMessageKind,
	body string,
	provenance model.SessionMessageProvenance,
	messageID string,
	now time.Time,
) (model.Event, error) {
	payload, err := json.Marshal(map[string]any{
		"message_id":        messageID,
		"session_id":        sessionID,
		"sender_session_id": senderSessionID,
		"kind":              kind,
		"body":              body,
		"provenance":        provenance,
	})
	if err != nil {
		return model.Event{}, fmt.Errorf("marshal session_message_added payload: %w", err)
	}

	return model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "session_message_added",
		PayloadJSON:    payload,
		CreatedAt:      now,
	}, nil
}

func newInboundMessageRecordedEvent(
	conversationID string,
	runID string,
	connectorID string,
	accountID string,
	threadID string,
	sourceMessageID string,
	sessionID string,
	sessionMessageID string,
	now time.Time,
) (model.Event, error) {
	payload, err := json.Marshal(map[string]any{
		"connector_id":       connectorID,
		"account_id":         accountID,
		"thread_id":          threadID,
		"source_message_id":  sourceMessageID,
		"run_id":             runID,
		"session_id":         sessionID,
		"session_message_id": sessionMessageID,
	})
	if err != nil {
		return model.Event{}, fmt.Errorf("marshal inbound_message_recorded payload: %w", err)
	}

	return model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "inbound_message_recorded",
		PayloadJSON:    payload,
		CreatedAt:      now,
	}, nil
}
