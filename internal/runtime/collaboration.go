package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	sessionkeys "github.com/canhta/gistclaw/internal/sessions"
)

type StartFrontSession struct {
	ConversationKey conversations.ConversationKey
	FrontAgentID    string
	InitialPrompt   string
	WorkspaceRoot   string
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

func (r *Runtime) StartFrontSession(ctx context.Context, cmd StartFrontSession) (model.Run, error) {
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
		Objective:      cmd.InitialPrompt,
		WorkspaceRoot:  cmd.WorkspaceRoot,
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
	})
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
		Objective:      cmd.Prompt,
		WorkspaceRoot:  controllerRun.WorkspaceRoot,
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
		`SELECT id, conversation_id, agent_id, COALESCE(session_id, ''), COALESCE(team_id, ''), COALESCE(parent_run_id, ''),
		        COALESCE(objective, ''), COALESCE(workspace_root, ''), status,
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
		&run.ParentRunID,
		&run.Objective,
		&run.WorkspaceRoot,
		&status,
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
	key := sessionkeys.BuildFrontSessionKey(conversationID)
	if role == model.SessionRoleWorker {
		key = sessionkeys.BuildWorkerSessionKey(parentSessionID, agentID)
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
		return fmt.Errorf("marshal session_opened payload: %w", err)
	}

	if err := r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		ParentRunID:    parentRunID,
		Kind:           "session_opened",
		PayloadJSON:    payload,
	}); err != nil {
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
	payload, err := json.Marshal(map[string]any{
		"thread_id":    normalizeThreadID(key.ThreadID),
		"session_id":   sessionID,
		"connector_id": key.ConnectorID,
		"account_id":   key.AccountID,
		"external_id":  key.ExternalID,
		"status":       "active",
	})
	if err != nil {
		return fmt.Errorf("marshal session_bound payload: %w", err)
	}

	if err := r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "session_bound",
		PayloadJSON:    payload,
	}); err != nil {
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
	payload, err := json.Marshal(map[string]any{
		"message_id":        messageID,
		"session_id":        sessionID,
		"sender_session_id": senderSessionID,
		"kind":              kind,
		"body":              body,
		"provenance":        provenance,
	})
	if err != nil {
		return "", fmt.Errorf("marshal session_message_added payload: %w", err)
	}

	if err := r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "session_message_added",
		PayloadJSON:    payload,
	}); err != nil {
		return "", fmt.Errorf("journal session_message_added: %w", err)
	}

	return messageID, nil
}
