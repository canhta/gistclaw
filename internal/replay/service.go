package replay

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

type RunReplay struct {
	RunID  string
	Status model.RunStatus
	Events []model.Event
}

type ReplayGraph struct {
	RootRunID string
	Edges     []GraphEdge
}

type GraphEdge struct {
	From string
	To   string
}

type Service struct {
	db *store.DB
}

func NewService(db *store.DB) *Service {
	return &Service{db: db}
}

func (s *Service) LoadRun(ctx context.Context, runID string) (RunReplay, error) {
	var status string
	err := s.db.RawDB().QueryRowContext(ctx,
		"SELECT status FROM runs WHERE id = ?",
		runID,
	).Scan(&status)
	if err != nil {
		return RunReplay{}, fmt.Errorf("replay: load run: %w", err)
	}

	rows, err := s.db.RawDB().QueryContext(ctx,
		`SELECT id, conversation_id, COALESCE(run_id, ''), COALESCE(parent_run_id, ''), kind,
		 COALESCE(payload_json, x''), created_at
		 FROM events
		 WHERE run_id = ?
		 ORDER BY created_at ASC`,
		runID,
	)
	if err != nil {
		return RunReplay{}, fmt.Errorf("replay: load events: %w", err)
	}
	defer rows.Close()

	events := make([]model.Event, 0)
	for rows.Next() {
		var event model.Event
		if err := rows.Scan(
			&event.ID,
			&event.ConversationID,
			&event.RunID,
			&event.ParentRunID,
			&event.Kind,
			&event.PayloadJSON,
			&event.CreatedAt,
		); err != nil {
			return RunReplay{}, fmt.Errorf("replay: scan event: %w", err)
		}
		events = append(events, event)
	}

	return RunReplay{
		RunID:  runID,
		Status: model.RunStatus(status),
		Events: events,
	}, rows.Err()
}

func (s *Service) LoadGraph(ctx context.Context, rootRunID string) (ReplayGraph, error) {
	var snapshotJSON []byte
	err := s.db.RawDB().QueryRowContext(ctx,
		"SELECT execution_snapshot_json FROM runs WHERE id = ?",
		rootRunID,
	).Scan(&snapshotJSON)
	if err != nil {
		return ReplayGraph{}, fmt.Errorf("replay: load snapshot: %w", err)
	}

	graph := ReplayGraph{RootRunID: rootRunID}
	if len(snapshotJSON) == 0 {
		return graph, nil
	}

	var snapshot struct {
		HandoffEdges []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"handoff_edges"`
	}
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return ReplayGraph{}, fmt.Errorf("replay: parse snapshot: %w", err)
	}

	for _, edge := range snapshot.HandoffEdges {
		graph.Edges = append(graph.Edges, GraphEdge{From: edge.From, To: edge.To})
	}

	return graph, nil
}
