package runtime

import (
	"context"
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
	ControllerRunID string
	AgentID         string
	Prompt          string
}

type AnnounceCommand struct {
	WorkerRunID string
	TargetRunID string
	Body        string
}

type SteerCommand struct {
	ControllerRunID string
	TargetRunID     string
	Body            string
}

type AgentSendCommand struct {
	FromRunID string
	ToRunID   string
	Body      string
}

func (r *Runtime) StartFrontSession(ctx context.Context, cmd StartFrontSession) (model.Run, error) {
	conv, err := r.convStore.Resolve(ctx, cmd.ConversationKey)
	if err != nil {
		return model.Run{}, fmt.Errorf("resolve conversation: %w", err)
	}

	runID := generateID()
	start := StartRun{
		ConversationID: conv.ID,
		AgentID:        cmd.FrontAgentID,
		Objective:      cmd.InitialPrompt,
		WorkspaceRoot:  cmd.WorkspaceRoot,
		AccountID:      cmd.ConversationKey.AccountID,
	}
	if err := r.createRun(ctx, runID, "", start); err != nil {
		return model.Run{}, err
	}
	if err := r.openSession(ctx, conv.ID, runID, "", cmd.FrontAgentID, model.SessionRoleFront, "", ""); err != nil {
		return model.Run{}, err
	}
	if err := r.appendSessionMessage(ctx, conv.ID, runID, runID, "", model.MessageUser, cmd.InitialPrompt); err != nil {
		return model.Run{}, err
	}

	return r.executeRunLoop(ctx, runLoopOpts{
		runID:          runID,
		conversationID: conv.ID,
		agentID:        cmd.FrontAgentID,
		objective:      cmd.InitialPrompt,
	})
}

func (r *Runtime) Spawn(ctx context.Context, cmd SpawnCommand) (model.Run, error) {
	controller, err := r.loadRun(ctx, cmd.ControllerRunID)
	if err != nil {
		return model.Run{}, err
	}

	runID := generateID()
	start := StartRun{
		ConversationID: controller.ConversationID,
		AgentID:        cmd.AgentID,
		Objective:      cmd.Prompt,
		WorkspaceRoot:  controller.WorkspaceRoot,
	}
	if err := r.createRun(ctx, runID, controller.ID, start); err != nil {
		return model.Run{}, err
	}
	if err := r.openSession(
		ctx,
		controller.ConversationID,
		runID,
		controller.ID,
		cmd.AgentID,
		model.SessionRoleWorker,
		controller.ID,
		controller.ID,
	); err != nil {
		return model.Run{}, err
	}
	if err := r.appendSessionMessage(
		ctx,
		controller.ConversationID,
		runID,
		runID,
		controller.ID,
		model.MessageSpawn,
		cmd.Prompt,
	); err != nil {
		return model.Run{}, err
	}

	return r.executeRunLoop(ctx, runLoopOpts{
		runID:          runID,
		conversationID: controller.ConversationID,
		agentID:        cmd.AgentID,
		objective:      cmd.Prompt,
	})
}

func (r *Runtime) Announce(ctx context.Context, cmd AnnounceCommand) error {
	return r.directSessionMessage(ctx, cmd.WorkerRunID, cmd.TargetRunID, model.MessageAnnounce, cmd.Body)
}

func (r *Runtime) Steer(ctx context.Context, cmd SteerCommand) error {
	return r.directSessionMessage(ctx, cmd.ControllerRunID, cmd.TargetRunID, model.MessageSteer, cmd.Body)
}

func (r *Runtime) AgentSend(ctx context.Context, cmd AgentSendCommand) error {
	return r.directSessionMessage(ctx, cmd.FromRunID, cmd.ToRunID, model.MessageAgentSend, cmd.Body)
}

func (r *Runtime) directSessionMessage(
	ctx context.Context,
	sourceRunID string,
	targetRunID string,
	kind model.SessionMessageKind,
	body string,
) error {
	sourceRun, err := r.loadRun(ctx, sourceRunID)
	if err != nil {
		return err
	}
	targetRun, err := r.loadRun(ctx, targetRunID)
	if err != nil {
		return err
	}
	if sourceRun.ConversationID != targetRun.ConversationID {
		return fmt.Errorf("runtime: run %s cannot message run %s across conversations", sourceRunID, targetRunID)
	}
	return r.appendSessionMessage(
		ctx,
		targetRun.ConversationID,
		targetRun.ID,
		targetRunID,
		sourceRunID,
		kind,
		body,
	)
}

func (r *Runtime) openSession(
	ctx context.Context,
	conversationID string,
	runID string,
	parentRunID string,
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
		"session_id":            runID,
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

func (r *Runtime) appendSessionMessage(
	ctx context.Context,
	conversationID string,
	runID string,
	sessionID string,
	senderSessionID string,
	kind model.SessionMessageKind,
	body string,
) error {
	payload, err := json.Marshal(map[string]any{
		"message_id":        generateID(),
		"session_id":        sessionID,
		"sender_session_id": senderSessionID,
		"kind":              kind,
		"body":              body,
	})
	if err != nil {
		return fmt.Errorf("marshal session_message_added payload: %w", err)
	}

	if err := r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "session_message_added",
		PayloadJSON:    payload,
	}); err != nil {
		return fmt.Errorf("journal session_message_added: %w", err)
	}

	return nil
}
