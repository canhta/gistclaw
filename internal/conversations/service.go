package conversations

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

var ErrConversationBusy = fmt.Errorf("conversation: competing root run active")

type ConversationStore struct {
	db *store.DB
}

func NewConversationStore(db *store.DB) *ConversationStore {
	return &ConversationStore{db: db}
}

func (s *ConversationStore) Resolve(ctx context.Context, key ConversationKey) (model.Conversation, error) {
	normalized := key.Normalize()

	var conv model.Conversation
	err := s.db.RawDB().QueryRowContext(ctx,
		"SELECT id, key, created_at FROM conversations WHERE key = ?",
		normalized,
	).Scan(&conv.ID, &conv.Key, &conv.CreatedAt)
	if err == nil {
		return conv, nil
	}
	if err != sql.ErrNoRows {
		return model.Conversation{}, fmt.Errorf("resolve conversation: %w", err)
	}

	id := generateID()
	now := time.Now().UTC()
	_, err = s.db.RawDB().ExecContext(ctx,
		"INSERT INTO conversations (id, key, created_at) VALUES (?, ?, ?) ON CONFLICT(key) DO NOTHING",
		id, normalized, now,
	)
	if err != nil {
		return model.Conversation{}, fmt.Errorf("create conversation: %w", err)
	}

	err = s.db.RawDB().QueryRowContext(ctx,
		"SELECT id, key, created_at FROM conversations WHERE key = ?",
		normalized,
	).Scan(&conv.ID, &conv.Key, &conv.CreatedAt)
	if err != nil {
		return model.Conversation{}, fmt.Errorf("re-read conversation: %w", err)
	}

	return conv, nil
}

func (s *ConversationStore) AppendEvent(ctx context.Context, evt model.Event) error {
	return s.db.Tx(ctx, func(tx *sql.Tx) error {
		if evt.CreatedAt.IsZero() {
			evt.CreatedAt = time.Now().UTC()
		}

		_, err := tx.ExecContext(ctx,
			`INSERT INTO events (id, conversation_id, run_id, parent_run_id, kind, payload_json, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			evt.ID, evt.ConversationID, evt.RunID, evt.ParentRunID, evt.Kind, evt.PayloadJSON, evt.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("append event: %w", err)
		}

		err = s.applyProjection(ctx, tx, evt)
		if err != nil {
			return fmt.Errorf("update projection: %w", err)
		}

		return nil
	})
}

type runStartedPayload struct {
	AgentID               string `json:"agent_id"`
	TeamID                string `json:"team_id"`
	Objective             string `json:"objective"`
	WorkspaceRoot         string `json:"workspace_root"`
	ExecutionSnapshotJSON []byte `json:"execution_snapshot_json"`
}

type turnCompletedPayload struct {
	Content      string `json:"content"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	ModelLane    string `json:"model_lane"`
}

type runCompletedPayload struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	ModelLane    string  `json:"model_lane"`
}

type summaryUpsertedPayload struct {
	SummaryID  string `json:"summary_id"`
	RunID      string `json:"run_id"`
	Content    string `json:"content"`
	TokenCount int    `json:"token_count"`
}

type sessionOpenedPayload struct {
	SessionID           string `json:"session_id"`
	Key                 string `json:"key"`
	AgentID             string `json:"agent_id"`
	Role                string `json:"role"`
	ParentSessionID     string `json:"parent_session_id"`
	ControllerSessionID string `json:"controller_session_id"`
	Status              string `json:"status"`
}

type sessionMessageAddedPayload struct {
	MessageID       string `json:"message_id"`
	SessionID       string `json:"session_id"`
	SenderSessionID string `json:"sender_session_id"`
	Kind            string `json:"kind"`
	Body            string `json:"body"`
}

func (s *ConversationStore) applyProjection(ctx context.Context, tx *sql.Tx, evt model.Event) error {
	switch evt.Kind {
	case "run_started":
		var payload runStartedPayload
		if err := decodePayload(evt.PayloadJSON, &payload); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO runs
			 (id, conversation_id, agent_id, team_id, parent_run_id, objective, workspace_root, status, execution_snapshot_json, created_at, updated_at)
			 VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, ?, 'active', ?, ?, ?)`,
			evt.RunID, evt.ConversationID, payload.AgentID, payload.TeamID, evt.ParentRunID,
			payload.Objective, payload.WorkspaceRoot, payload.ExecutionSnapshotJSON, evt.CreatedAt, evt.CreatedAt,
		)
		return err
	case "turn_completed":
		var payload turnCompletedPayload
		if err := decodePayload(evt.PayloadJSON, &payload); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE runs
			 SET input_tokens = input_tokens + ?,
			     output_tokens = output_tokens + ?,
			     model_lane = CASE WHEN ? = '' THEN model_lane ELSE ? END,
			     updated_at = ?
			 WHERE id = ?`,
			payload.InputTokens, payload.OutputTokens, payload.ModelLane, payload.ModelLane, evt.CreatedAt, evt.RunID,
		)
		return err
	case "run_completed":
		var payload runCompletedPayload
		if err := decodePayload(evt.PayloadJSON, &payload); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE runs SET status = 'completed', updated_at = ? WHERE id = ?",
			evt.CreatedAt, evt.RunID,
		); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO receipts (id, run_id, input_tokens, output_tokens, cost_usd, model_lane, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(run_id) DO UPDATE SET
			     input_tokens = excluded.input_tokens,
			     output_tokens = excluded.output_tokens,
			     cost_usd = excluded.cost_usd,
			     model_lane = excluded.model_lane`,
			generateID(), evt.RunID, payload.InputTokens, payload.OutputTokens, payload.CostUSD, payload.ModelLane, evt.CreatedAt,
		)
		return err
	case "run_interrupted":
		_, err := tx.ExecContext(ctx,
			"UPDATE runs SET status = 'interrupted', updated_at = ? WHERE id = ?",
			evt.CreatedAt, evt.RunID,
		)
		return err
	case "run_failed":
		_, err := tx.ExecContext(ctx,
			"UPDATE runs SET status = 'failed', updated_at = ? WHERE id = ?",
			evt.CreatedAt, evt.RunID,
		)
		return err
	case "budget_exhausted":
		_, err := tx.ExecContext(ctx,
			"UPDATE runs SET updated_at = ? WHERE id = ?",
			evt.CreatedAt, evt.RunID,
		)
		return err
	case "summary_upserted":
		var payload summaryUpsertedPayload
		if err := decodePayload(evt.PayloadJSON, &payload); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO run_summaries (id, run_id, content, token_count, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(run_id) DO UPDATE SET
			     content = excluded.content,
			     token_count = excluded.token_count,
			     updated_at = excluded.updated_at`,
			payload.SummaryID, payload.RunID, payload.Content, payload.TokenCount, evt.CreatedAt, evt.CreatedAt,
		)
		return err
	case "session_opened":
		var payload sessionOpenedPayload
		if err := decodePayload(evt.PayloadJSON, &payload); err != nil {
			return err
		}
		status := payload.Status
		if status == "" {
			status = "active"
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO sessions
			 (id, conversation_id, key, agent_id, role, parent_session_id, controller_session_id, status, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			payload.SessionID, evt.ConversationID, payload.Key, payload.AgentID, payload.Role,
			payload.ParentSessionID, payload.ControllerSessionID, status, evt.CreatedAt,
		)
		return err
	case "session_message_added":
		var payload sessionMessageAddedPayload
		if err := decodePayload(evt.PayloadJSON, &payload); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO session_messages
			 (id, session_id, sender_session_id, kind, body, created_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			payload.MessageID, payload.SessionID, payload.SenderSessionID, payload.Kind, payload.Body, evt.CreatedAt,
		)
		return err
	default:
		return nil
	}
}

func decodePayload[T any](payload []byte, target *T) error {
	if len(payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	return nil
}

func (s *ConversationStore) ListEvents(ctx context.Context, conversationID string, limit int) ([]model.Event, error) {
	rows, err := s.db.RawDB().QueryContext(ctx,
		`SELECT id, conversation_id, COALESCE(run_id, ''), COALESCE(parent_run_id, ''), kind,
		 COALESCE(payload_json, x''), created_at
		 FROM events
		 WHERE conversation_id = ?
		 ORDER BY created_at ASC
		 LIMIT ?`,
		conversationID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
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
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

func (s *ConversationStore) ActiveRootRun(ctx context.Context, conversationID string) (model.RunRef, error) {
	var ref model.RunRef
	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT id, status
		 FROM runs
		 WHERE conversation_id = ? AND parent_run_id IS NULL AND status IN ('pending', 'active', 'needs_approval')
		 ORDER BY created_at ASC
		 LIMIT 1`,
		conversationID,
	).Scan(&ref.ID, &ref.Status)
	if err == sql.ErrNoRows {
		return model.RunRef{}, nil
	}
	if err != nil {
		return model.RunRef{}, fmt.Errorf("active root run: %w", err)
	}
	return ref, nil
}

func (s *ConversationStore) DB() *store.DB {
	return s.db
}

func generateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
