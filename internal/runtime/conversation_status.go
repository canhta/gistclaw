package runtime

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
)

type ConversationStatus struct {
	Exists           bool
	ConversationID   string
	ActiveRun        model.Run
	LatestRootRun    model.Run
	PendingApprovals int
}

type ConversationResetOutcome string

const (
	ConversationResetCleared ConversationResetOutcome = "cleared"
	ConversationResetMissing ConversationResetOutcome = "missing"
	ConversationResetBusy    ConversationResetOutcome = "busy"
)

func (r *Runtime) InspectConversation(ctx context.Context, key conversations.ConversationKey) (ConversationStatus, error) {
	scopedKey, _, err := r.scopeConversationKey(ctx, key, "", "")
	if err != nil {
		return ConversationStatus{}, fmt.Errorf("inspect conversation: %w", err)
	}

	conv, found, err := r.convStore.Find(ctx, scopedKey)
	if err != nil {
		return ConversationStatus{}, fmt.Errorf("inspect conversation: %w", err)
	}
	if !found {
		return ConversationStatus{}, nil
	}

	status := ConversationStatus{
		Exists:         true,
		ConversationID: conv.ID,
	}

	status.ActiveRun, err = r.loadConversationRun(ctx, conv.ID, `
		SELECT id
		FROM runs
		WHERE conversation_id = ? AND parent_run_id IS NULL AND status IN ('pending', 'active', 'needs_approval')
		ORDER BY created_at ASC, id ASC
		LIMIT 1`)
	if err != nil {
		return ConversationStatus{}, err
	}

	status.LatestRootRun, err = r.loadConversationRun(ctx, conv.ID, `
		SELECT id
		FROM runs
		WHERE conversation_id = ? AND parent_run_id IS NULL
		ORDER BY created_at DESC, id DESC
		LIMIT 1`)
	if err != nil {
		return ConversationStatus{}, err
	}

	err = r.store.RawDB().QueryRowContext(ctx, `
		SELECT count(*)
		FROM approvals
		INNER JOIN runs ON runs.id = approvals.run_id
		WHERE runs.conversation_id = ? AND approvals.status = 'pending'`,
		conv.ID,
	).Scan(&status.PendingApprovals)
	if err != nil {
		return ConversationStatus{}, fmt.Errorf("inspect conversation approvals: %w", err)
	}

	return status, nil
}

func (r *Runtime) ResetConversation(ctx context.Context, key conversations.ConversationKey) (ConversationResetOutcome, error) {
	scopedKey, _, err := r.scopeConversationKey(ctx, key, "", "")
	if err != nil {
		return "", fmt.Errorf("reset conversation: %w", err)
	}

	conv, found, err := r.convStore.Find(ctx, scopedKey)
	if err != nil {
		return "", fmt.Errorf("reset conversation: %w", err)
	}
	if !found {
		return ConversationResetMissing, nil
	}

	active, err := r.convStore.ActiveRootRun(ctx, conv.ID)
	if err != nil {
		return "", fmt.Errorf("reset conversation: %w", err)
	}
	if active.ID != "" {
		return ConversationResetBusy, nil
	}

	if err := r.convStore.ResetConversation(ctx, conv.ID); err != nil {
		return "", fmt.Errorf("reset conversation: %w", err)
	}
	return ConversationResetCleared, nil
}

func (r *Runtime) loadConversationRun(ctx context.Context, conversationID, query string) (model.Run, error) {
	var runID string
	err := r.store.RawDB().QueryRowContext(ctx, query, conversationID).Scan(&runID)
	if err == sql.ErrNoRows {
		return model.Run{}, nil
	}
	if err != nil {
		return model.Run{}, fmt.Errorf("load conversation run ref: %w", err)
	}
	run, err := r.loadRun(ctx, runID)
	if err != nil {
		return model.Run{}, fmt.Errorf("load conversation run %s: %w", runID, err)
	}
	return run, nil
}
