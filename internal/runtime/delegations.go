package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

var ErrInvalidHandoff = fmt.Errorf("runtime: invalid handoff edge")

var defaultMaxActiveChildren = 3

type executionSnapshot struct {
	HandoffEdges      []handoffEdge `json:"handoff_edges"`
	MaxActiveChildren int           `json:"max_active_children"`
}

type handoffEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (r *Runtime) createDelegation(ctx context.Context, cmd DelegateRun) (model.Run, error) {
	var parentAgentID string
	var conversationID string
	var snapshotJSON []byte
	err := r.store.RawDB().QueryRowContext(ctx,
		`SELECT agent_id, conversation_id, execution_snapshot_json
		 FROM runs
		 WHERE id = ?`,
		cmd.ParentRunID,
	).Scan(&parentAgentID, &conversationID, &snapshotJSON)
	if err != nil {
		return model.Run{}, fmt.Errorf("load parent run: %w", err)
	}

	var snapshot executionSnapshot
	if len(snapshotJSON) > 0 {
		if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
			return model.Run{}, fmt.Errorf("parse snapshot: %w", err)
		}
	}

	edgeValid := false
	for _, edge := range snapshot.HandoffEdges {
		if edge.From == parentAgentID && edge.To == cmd.TargetAgentID {
			edgeValid = true
			break
		}
	}
	if !edgeValid {
		_ = r.convStore.AppendEvent(ctx, model.Event{
			ID:             generateID(),
			ConversationID: conversationID,
			RunID:          cmd.ParentRunID,
			Kind:           "delegation_rejected",
			PayloadJSON: []byte(fmt.Sprintf(
				`{"target":"%s","reason":"undeclared handoff edge"}`,
				cmd.TargetAgentID,
			)),
		})
		return model.Run{}, ErrInvalidHandoff
	}

	maxChildren := r.maxActiveChildren
	if maxChildren == 0 {
		maxChildren = defaultMaxActiveChildren
	}
	if snapshot.MaxActiveChildren > 0 {
		maxChildren = snapshot.MaxActiveChildren
	}

	rootRunID, err := r.rootRunID(ctx, cmd.ParentRunID)
	if err != nil {
		return model.Run{}, err
	}

	var activeChildren int
	err = r.store.RawDB().QueryRowContext(ctx,
		"SELECT count(*) FROM delegations WHERE root_run_id = ? AND status = 'active'",
		rootRunID,
	).Scan(&activeChildren)
	if err != nil {
		return model.Run{}, fmt.Errorf("count active children: %w", err)
	}

	delegationID := generateID()
	childRunID := generateID()
	now := time.Now().UTC()

	if activeChildren >= maxChildren {
		payload, err := json.Marshal(map[string]any{
			"root_run_id":     rootRunID,
			"target_agent_id": cmd.TargetAgentID,
		})
		if err != nil {
			return model.Run{}, fmt.Errorf("marshal delegation_queued payload: %w", err)
		}
		err = r.convStore.AppendEvent(ctx, model.Event{
			ID:             delegationID,
			ConversationID: conversationID,
			RunID:          cmd.ParentRunID,
			ParentRunID:    cmd.ParentRunID,
			Kind:           "delegation_queued",
			PayloadJSON:    payload,
			CreatedAt:      now,
		})
		if err != nil {
			return model.Run{}, fmt.Errorf("queue delegation: %w", err)
		}
		return model.Run{
			ID:          delegationID,
			ParentRunID: cmd.ParentRunID,
			Status:      model.RunStatusPending,
		}, nil
	}

	payload, err := json.Marshal(map[string]any{
		"root_run_id":             rootRunID,
		"target_agent_id":         cmd.TargetAgentID,
		"objective":               cmd.Objective,
		"execution_snapshot_json": snapshotJSON,
	})
	if err != nil {
		return model.Run{}, fmt.Errorf("marshal delegation_created payload: %w", err)
	}
	err = r.convStore.AppendEvent(ctx, model.Event{
		ID:             delegationID,
		ConversationID: conversationID,
		RunID:          childRunID,
		ParentRunID:    cmd.ParentRunID,
		Kind:           "delegation_created",
		PayloadJSON:    payload,
		CreatedAt:      now,
	})
	if err != nil {
		return model.Run{}, fmt.Errorf("create delegation: %w", err)
	}

	return model.Run{
		ID:          childRunID,
		ParentRunID: cmd.ParentRunID,
		AgentID:     cmd.TargetAgentID,
		Status:      model.RunStatusActive,
	}, nil
}

func (r *Runtime) rootRunID(ctx context.Context, runID string) (string, error) {
	current := runID
	for {
		var parentRunID sql.NullString
		err := r.store.RawDB().QueryRowContext(ctx,
			"SELECT parent_run_id FROM runs WHERE id = ?",
			current,
		).Scan(&parentRunID)
		if err != nil {
			return "", fmt.Errorf("load root lineage: %w", err)
		}
		if !parentRunID.Valid || parentRunID.String == "" {
			return current, nil
		}
		current = parentRunID.String
	}
}
