package runtime

import (
	"context"
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

	var activeChildren int
	err = r.store.RawDB().QueryRowContext(ctx,
		"SELECT count(*) FROM delegations WHERE parent_run_id = ? AND status = 'active'",
		cmd.ParentRunID,
	).Scan(&activeChildren)
	if err != nil {
		return model.Run{}, fmt.Errorf("count active children: %w", err)
	}

	delegationID := generateID()
	now := time.Now().UTC()

	if activeChildren >= maxChildren {
		_, err = r.store.RawDB().ExecContext(ctx,
			`INSERT INTO delegations (id, root_run_id, parent_run_id, target_agent_id, status, created_at)
			 VALUES (?, ?, ?, ?, 'queued', ?)`,
			delegationID, cmd.ParentRunID, cmd.ParentRunID, cmd.TargetAgentID, now,
		)
		if err != nil {
			return model.Run{}, fmt.Errorf("queue delegation: %w", err)
		}
		return model.Run{
			ID:          delegationID,
			ParentRunID: cmd.ParentRunID,
			Status:      model.RunStatusPending,
		}, nil
	}

	childRunID := generateID()
	_, err = r.store.RawDB().ExecContext(ctx,
		`INSERT INTO runs (id, conversation_id, agent_id, parent_run_id, objective, status, execution_snapshot_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'active', ?, ?, ?)`,
		childRunID, conversationID, cmd.TargetAgentID, cmd.ParentRunID, cmd.Objective, snapshotJSON, now, now,
	)
	if err != nil {
		return model.Run{}, fmt.Errorf("create child run: %w", err)
	}

	_, err = r.store.RawDB().ExecContext(ctx,
		`INSERT INTO delegations (id, root_run_id, parent_run_id, child_run_id, target_agent_id, status, created_at)
		 VALUES (?, ?, ?, ?, ?, 'active', ?)`,
		delegationID, cmd.ParentRunID, cmd.ParentRunID, childRunID, cmd.TargetAgentID, now,
	)
	if err != nil {
		return model.Run{}, fmt.Errorf("create delegation: %w", err)
	}

	_ = r.convStore.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          childRunID,
		ParentRunID:    cmd.ParentRunID,
		Kind:           "delegation_created",
		PayloadJSON: []byte(fmt.Sprintf(
			`{"parent":"%s","child":"%s","target":"%s"}`,
			cmd.ParentRunID, childRunID, cmd.TargetAgentID,
		)),
	})

	return model.Run{
		ID:          childRunID,
		ParentRunID: cmd.ParentRunID,
		AgentID:     cmd.TargetAgentID,
		Status:      model.RunStatusActive,
	}, nil
}
