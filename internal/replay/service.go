package replay

import (
	"context"
	"database/sql"
	"fmt"
	"time"

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

type GraphNode struct {
	ID          string
	ParentRunID string
	AgentID     string
	SessionID   string
	Objective   string
	Status      model.RunStatus
	Depth       int
	UpdatedAt   time.Time
}

type RunGraphSnapshot struct {
	RootRunID string
	Nodes     []GraphNode
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
	snapshot, err := s.LoadGraphSnapshot(ctx, rootRunID)
	if err != nil {
		return ReplayGraph{}, err
	}

	return ReplayGraph{
		RootRunID: snapshot.RootRunID,
		Edges:     snapshot.Edges,
	}, nil
}

func (s *Service) LoadGraphSnapshot(ctx context.Context, rootRunID string) (RunGraphSnapshot, error) {
	rows, err := s.db.RawDB().QueryContext(ctx,
		`WITH RECURSIVE lineage(id, parent_run_id, agent_id, session_id, objective, status, updated_at, created_at, depth) AS (
			SELECT id,
			       COALESCE(parent_run_id, ''),
			       agent_id,
			       COALESCE(session_id, ''),
			       COALESCE(objective, ''),
			       status,
			       updated_at,
			       created_at,
			       0
			FROM runs
			WHERE id = ?
			UNION ALL
			SELECT r.id,
			       COALESCE(r.parent_run_id, ''),
			       r.agent_id,
			       COALESCE(r.session_id, ''),
			       COALESCE(r.objective, ''),
			       r.status,
			       r.updated_at,
			       r.created_at,
			       lineage.depth + 1
			FROM runs r
			INNER JOIN lineage ON r.parent_run_id = lineage.id
		)
		SELECT id, parent_run_id, agent_id, session_id, objective, status, updated_at, depth
		FROM lineage
		ORDER BY depth ASC, created_at ASC, id ASC`,
		rootRunID,
	)
	if err != nil {
		return RunGraphSnapshot{}, fmt.Errorf("replay: load graph lineage: %w", err)
	}
	defer rows.Close()

	graph := RunGraphSnapshot{RootRunID: rootRunID}
	for rows.Next() {
		var node GraphNode
		var status string
		if err := rows.Scan(
			&node.ID,
			&node.ParentRunID,
			&node.AgentID,
			&node.SessionID,
			&node.Objective,
			&status,
			&node.UpdatedAt,
			&node.Depth,
		); err != nil {
			return RunGraphSnapshot{}, fmt.Errorf("replay: scan graph lineage: %w", err)
		}
		node.Status = model.RunStatus(status)
		graph.Nodes = append(graph.Nodes, node)
		if node.ParentRunID != "" {
			graph.Edges = append(graph.Edges, GraphEdge{From: node.ParentRunID, To: node.ID})
		}
	}
	if err := rows.Err(); err != nil {
		return RunGraphSnapshot{}, fmt.Errorf("replay: iterate graph lineage: %w", err)
	}
	if len(graph.Nodes) == 0 {
		return RunGraphSnapshot{}, sql.ErrNoRows
	}

	return graph, nil
}
