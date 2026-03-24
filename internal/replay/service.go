package replay

import (
	"context"
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
	rows, err := s.db.RawDB().QueryContext(ctx,
		`WITH RECURSIVE lineage(id, parent_run_id, created_at) AS (
			SELECT id, COALESCE(parent_run_id, ''), created_at
			FROM runs
			WHERE id = ?
			UNION ALL
			SELECT r.id, COALESCE(r.parent_run_id, ''), r.created_at
			FROM runs r
			INNER JOIN lineage l ON r.parent_run_id = l.id
		)
		SELECT id, parent_run_id
		FROM lineage
		WHERE id != ?
		ORDER BY created_at ASC`,
		rootRunID, rootRunID,
	)
	if err != nil {
		return ReplayGraph{}, fmt.Errorf("replay: load graph lineage: %w", err)
	}
	defer rows.Close()

	graph := ReplayGraph{RootRunID: rootRunID}
	for rows.Next() {
		var runID string
		var parentRunID string
		if err := rows.Scan(&runID, &parentRunID); err != nil {
			return ReplayGraph{}, fmt.Errorf("replay: scan graph lineage: %w", err)
		}
		if parentRunID == "" {
			continue
		}
		graph.Edges = append(graph.Edges, GraphEdge{From: parentRunID, To: runID})
	}
	if err := rows.Err(); err != nil {
		return ReplayGraph{}, fmt.Errorf("replay: iterate graph lineage: %w", err)
	}

	return graph, nil
}
