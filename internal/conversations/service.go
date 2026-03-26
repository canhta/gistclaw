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
var ErrDuplicateInboundMessage = fmt.Errorf("conversation: duplicate inbound message")
var ErrDeliveryNotRetryable = fmt.Errorf("conversation: delivery not retryable")

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

func (s *ConversationStore) Find(ctx context.Context, key ConversationKey) (model.Conversation, bool, error) {
	normalized := key.Normalize()

	var conv model.Conversation
	err := s.db.RawDB().QueryRowContext(ctx,
		"SELECT id, key, created_at FROM conversations WHERE key = ?",
		normalized,
	).Scan(&conv.ID, &conv.Key, &conv.CreatedAt)
	if err == sql.ErrNoRows {
		return model.Conversation{}, false, nil
	}
	if err != nil {
		return model.Conversation{}, false, fmt.Errorf("find conversation: %w", err)
	}
	return conv, true, nil
}

func (s *ConversationStore) AppendEvent(ctx context.Context, evt model.Event) error {
	return s.AppendEvents(ctx, []model.Event{evt})
}

func (s *ConversationStore) AppendEvents(ctx context.Context, events []model.Event) error {
	return s.db.Tx(ctx, func(tx *sql.Tx) error {
		for _, evt := range events {
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
		}
		return nil
	})
}

type runStartedPayload struct {
	AgentID               string `json:"agent_id"`
	SessionID             string `json:"session_id"`
	TeamID                string `json:"team_id"`
	ProjectID             string `json:"project_id"`
	Objective             string `json:"objective"`
	WorkspaceRoot         string `json:"workspace_root"`
	ExecutionSnapshotJSON []byte `json:"execution_snapshot_json"`
}

type turnCompletedPayload struct {
	Content      string `json:"content"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	ModelLane    string `json:"model_lane"`
	ModelID      string `json:"model_id"`
}

type runCompletedPayload struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	ModelLane    string  `json:"model_lane"`
	ModelID      string  `json:"model_id"`
}

type toolCallRecordedPayload struct {
	ToolCallID string          `json:"tool_call_id"`
	ToolName   string          `json:"tool_name"`
	InputJSON  json.RawMessage `json:"input_json"`
	OutputJSON json.RawMessage `json:"output_json"`
	Decision   string          `json:"decision"`
	ApprovalID string          `json:"approval_id"`
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
	MessageID       string          `json:"message_id"`
	SessionID       string          `json:"session_id"`
	SenderSessionID string          `json:"sender_session_id"`
	Kind            string          `json:"kind"`
	Body            string          `json:"body"`
	Provenance      json.RawMessage `json:"provenance"`
}

type sessionBoundPayload struct {
	ThreadID    string `json:"thread_id"`
	SessionID   string `json:"session_id"`
	ConnectorID string `json:"connector_id"`
	AccountID   string `json:"account_id"`
	ExternalID  string `json:"external_id"`
	Status      string `json:"status"`
}

type sessionUnboundPayload struct {
	RouteID string `json:"route_id"`
}

type inboundMessageRecordedPayload struct {
	ConnectorID      string `json:"connector_id"`
	AccountID        string `json:"account_id"`
	ThreadID         string `json:"thread_id"`
	SourceMessageID  string `json:"source_message_id"`
	RunID            string `json:"run_id"`
	SessionID        string `json:"session_id"`
	SessionMessageID string `json:"session_message_id"`
}

type deliveryRedriveRequestedPayload struct {
	IntentID string `json:"intent_id"`
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
			 (id, conversation_id, agent_id, session_id, team_id, project_id, parent_run_id, objective, workspace_root, status, execution_snapshot_json, created_at, updated_at)
			 VALUES (?, ?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, 'active', ?, ?, ?)`,
			evt.RunID, evt.ConversationID, payload.AgentID, payload.SessionID, payload.TeamID, payload.ProjectID, evt.ParentRunID,
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
			     model_id = CASE WHEN ? = '' THEN model_id ELSE ? END,
			     updated_at = ?
			 WHERE id = ?`,
			payload.InputTokens, payload.OutputTokens, payload.ModelLane, payload.ModelLane, payload.ModelID, payload.ModelID, evt.CreatedAt, evt.RunID,
		)
		return err
	case "run_completed":
		var payload runCompletedPayload
		if err := decodePayload(evt.PayloadJSON, &payload); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE runs
			 SET status = 'completed',
			     model_lane = CASE WHEN ? = '' THEN model_lane ELSE ? END,
			     model_id = CASE WHEN ? = '' THEN model_id ELSE ? END,
			     updated_at = ?
			 WHERE id = ?`,
			payload.ModelLane, payload.ModelLane, payload.ModelID, payload.ModelID, evt.CreatedAt, evt.RunID,
		); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO receipts (id, run_id, input_tokens, output_tokens, cost_usd, model_lane, model_id, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(run_id) DO UPDATE SET
			     input_tokens = excluded.input_tokens,
			     output_tokens = excluded.output_tokens,
			     cost_usd = excluded.cost_usd,
			     model_lane = excluded.model_lane,
			     model_id = excluded.model_id`,
			generateID(), evt.RunID, payload.InputTokens, payload.OutputTokens, payload.CostUSD, payload.ModelLane, payload.ModelID, evt.CreatedAt,
		)
		return err
	case "tool_call_recorded":
		var payload toolCallRecordedPayload
		if err := decodePayload(evt.PayloadJSON, &payload); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tool_calls
			 (id, run_id, tool_name, input_json, output_json, decision, approval_id, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)`,
			evt.ID,
			evt.RunID,
			payload.ToolName,
			payload.InputJSON,
			payload.OutputJSON,
			payload.Decision,
			payload.ApprovalID,
			evt.CreatedAt,
		)
		return err
	case "approval_requested":
		_, err := tx.ExecContext(ctx,
			"UPDATE runs SET status = 'needs_approval', updated_at = ? WHERE id = ?",
			evt.CreatedAt, evt.RunID,
		)
		return err
	case "run_resumed":
		_, err := tx.ExecContext(ctx,
			"UPDATE runs SET status = 'active', updated_at = ? WHERE id = ?",
			evt.CreatedAt, evt.RunID,
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
			 (id, session_id, sender_session_id, kind, body, provenance_json, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			payload.MessageID,
			payload.SessionID,
			payload.SenderSessionID,
			payload.Kind,
			payload.Body,
			payload.Provenance,
			evt.CreatedAt,
		)
		return err
	case "session_bound":
		var payload sessionBoundPayload
		if err := decodePayload(evt.PayloadJSON, &payload); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE session_bindings
			 SET status = 'inactive',
			     deactivated_at = ?,
			     deactivation_reason = 'replaced',
			     replaced_by_route_id = ?
			 WHERE conversation_id = ? AND thread_id = ? AND status = 'active'`,
			evt.CreatedAt, evt.ID, evt.ConversationID, payload.ThreadID,
		); err != nil {
			return err
		}
		status := payload.Status
		if status == "" {
			status = "active"
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO session_bindings
			 (id, conversation_id, thread_id, session_id, connector_id, account_id, external_id, status, created_at, deactivated_at, deactivation_reason, replaced_by_route_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, '', '')`,
			evt.ID,
			evt.ConversationID,
			payload.ThreadID,
			payload.SessionID,
			payload.ConnectorID,
			payload.AccountID,
			payload.ExternalID,
			status,
			evt.CreatedAt,
		)
		return err
	case "session_unbound":
		var payload sessionUnboundPayload
		if err := decodePayload(evt.PayloadJSON, &payload); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE session_bindings
			 SET status = 'inactive',
			     deactivated_at = ?,
			     deactivation_reason = 'deactivated',
			     replaced_by_route_id = ''
			 WHERE id = ? AND status = 'active'`,
			evt.CreatedAt, payload.RouteID,
		)
		return err
	case "inbound_message_recorded":
		var payload inboundMessageRecordedPayload
		if err := decodePayload(evt.PayloadJSON, &payload); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO inbound_receipts
			 (id, conversation_id, connector_id, account_id, thread_id, source_message_id, run_id, session_id, session_message_id, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			evt.ID,
			evt.ConversationID,
			payload.ConnectorID,
			payload.AccountID,
			payload.ThreadID,
			payload.SourceMessageID,
			payload.RunID,
			payload.SessionID,
			payload.SessionMessageID,
			evt.CreatedAt,
		)
		if store.IsSQLiteConstraintUnique(err) {
			return ErrDuplicateInboundMessage
		}
		return err
	case "delivery_redrive_requested":
		var payload deliveryRedriveRequestedPayload
		if err := decodePayload(evt.PayloadJSON, &payload); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx,
			`UPDATE outbound_intents
			 SET status = 'pending',
			     attempts = 0,
			     last_attempt_at = NULL
			 WHERE id = ? AND status = 'terminal'`,
			payload.IntentID,
		)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return ErrDeliveryNotRetryable
		}
		return nil
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

func (s *ConversationStore) ResetConversation(ctx context.Context, conversationID string) error {
	deletes := []struct {
		name  string
		query string
	}{
		{
			name:  "tool_calls",
			query: `DELETE FROM tool_calls WHERE run_id IN (SELECT id FROM runs WHERE conversation_id = ?)`,
		},
		{
			name:  "approvals",
			query: `DELETE FROM approvals WHERE run_id IN (SELECT id FROM runs WHERE conversation_id = ?)`,
		},
		{
			name:  "receipts",
			query: `DELETE FROM receipts WHERE run_id IN (SELECT id FROM runs WHERE conversation_id = ?)`,
		},
		{
			name:  "outbound_intents",
			query: `DELETE FROM outbound_intents WHERE run_id IN (SELECT id FROM runs WHERE conversation_id = ?)`,
		},
		{
			name:  "run_summaries",
			query: `DELETE FROM run_summaries WHERE run_id IN (SELECT id FROM runs WHERE conversation_id = ?)`,
		},
		{
			name:  "session_messages",
			query: `DELETE FROM session_messages WHERE session_id IN (SELECT id FROM sessions WHERE conversation_id = ?)`,
		},
		{
			name:  "session_bindings",
			query: `DELETE FROM session_bindings WHERE conversation_id = ?`,
		},
		{
			name:  "inbound_receipts",
			query: `DELETE FROM inbound_receipts WHERE conversation_id = ?`,
		},
		{
			name:  "events",
			query: `DELETE FROM events WHERE conversation_id = ?`,
		},
		{
			name:  "sessions",
			query: `DELETE FROM sessions WHERE conversation_id = ?`,
		},
		{
			name:  "runs",
			query: `DELETE FROM runs WHERE conversation_id = ?`,
		},
		{
			name:  "conversations",
			query: `DELETE FROM conversations WHERE id = ?`,
		},
	}

	return s.db.Tx(ctx, func(tx *sql.Tx) error {
		for _, del := range deletes {
			if _, err := tx.ExecContext(ctx, del.query, conversationID); err != nil {
				return fmt.Errorf("delete %s: %w", del.name, err)
			}
		}
		return nil
	})
}

func (s *ConversationStore) DB() *store.DB {
	return s.db
}

func generateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
