package sessions

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

type Service struct {
	db   *store.DB
	conv *conversations.ConversationStore
}

var ErrThreadMailboxNotFound = fmt.Errorf("sessions: no active session bound to thread")
var ErrSessionNotFound = fmt.Errorf("sessions: session not found")
var ErrSessionRouteNotFound = fmt.Errorf("sessions: no active route bound to session")
var ErrOutboundIntentNotFound = fmt.Errorf("sessions: outbound intent not found")

type OpenFrontSession struct {
	ConversationID string
	AgentID        string
	WorkspaceRoot  string
}

type SpawnWorkerSession struct {
	ConversationID      string
	ParentSessionID     string
	ControllerSessionID string
	AgentID             string
	InitialPrompt       string
}

type BindFollowUp struct {
	ConversationID string
	ThreadID       string
	SessionID      string
	ConnectorID    string
	AccountID      string
	ExternalID     string
}

func NewService(db *store.DB, conv *conversations.ConversationStore) *Service {
	return &Service{db: db, conv: conv}
}

func (s *Service) OpenFrontSession(ctx context.Context, cmd OpenFrontSession) (model.Session, error) {
	now := time.Now().UTC()
	session := model.Session{
		ID:             generateID(),
		ConversationID: cmd.ConversationID,
		Key:            BuildFrontSessionKey(cmd.ConversationID),
		AgentID:        cmd.AgentID,
		Role:           model.SessionRoleFront,
		Status:         "active",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.appendSessionOpened(ctx, "", session); err != nil {
		return model.Session{}, fmt.Errorf("open front session: %w", err)
	}

	return session, nil
}

func (s *Service) SpawnWorkerSession(ctx context.Context, cmd SpawnWorkerSession) (model.Session, error) {
	now := time.Now().UTC()
	session := model.Session{
		ID:                  generateID(),
		ConversationID:      cmd.ConversationID,
		Key:                 BuildWorkerSessionKey(cmd.ParentSessionID, cmd.AgentID),
		AgentID:             cmd.AgentID,
		Role:                model.SessionRoleWorker,
		ParentSessionID:     cmd.ParentSessionID,
		ControllerSessionID: cmd.ControllerSessionID,
		Status:              "active",
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if err := s.appendSessionOpened(ctx, "", session); err != nil {
		return model.Session{}, fmt.Errorf("spawn worker session: %w", err)
	}

	if err := s.AppendMessage(ctx, model.SessionMessage{
		ID:              generateID(),
		SessionID:       session.ID,
		SenderSessionID: cmd.ControllerSessionID,
		Kind:            model.MessageSpawn,
		Body:            cmd.InitialPrompt,
		Provenance: model.SessionMessageProvenance{
			Kind:            model.MessageProvenanceInterSession,
			SourceSessionID: cmd.ControllerSessionID,
		},
		CreatedAt: now,
	}); err != nil {
		return model.Session{}, err
	}

	return session, nil
}

func (s *Service) AppendMessage(ctx context.Context, msg model.SessionMessage) error {
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	if msg.ID == "" {
		msg.ID = generateID()
	}

	if err := s.appendSessionMessage(ctx, "", msg); err != nil {
		return fmt.Errorf("append session message: %w", err)
	}
	return nil
}

func (s *Service) BindFollowUp(ctx context.Context, cmd BindFollowUp) error {
	if err := s.appendSessionBinding(ctx, "", cmd); err != nil {
		return fmt.Errorf("bind follow up: %w", err)
	}
	return nil
}

func (s *Service) LoadThreadMailbox(
	ctx context.Context,
	conversationID string,
	threadID string,
	limit int,
) (model.Session, []model.SessionMessage, error) {
	session, err := s.loadBoundSession(ctx, conversationID, normalizeThreadID(threadID))
	if err != nil {
		return model.Session{}, nil, err
	}

	messages, err := s.listSessionMessages(ctx, session.ID, limit)
	if err != nil {
		return model.Session{}, nil, err
	}

	return session, messages, nil
}

func (s *Service) LoadSessionMailbox(
	ctx context.Context,
	sessionID string,
	limit int,
) (model.Session, []model.SessionMessage, error) {
	session, err := s.loadSession(ctx, sessionID)
	if err != nil {
		return model.Session{}, nil, err
	}
	messages, err := s.listSessionMessages(ctx, session.ID, limit)
	if err != nil {
		return model.Session{}, nil, err
	}
	return session, messages, nil
}

func (s *Service) LoadSession(ctx context.Context, sessionID string) (model.Session, error) {
	session, err := s.loadSession(ctx, sessionID)
	if err != nil {
		return model.Session{}, err
	}
	return session, nil
}

func (s *Service) ListConversationSessions(ctx context.Context, conversationID string, limit int) ([]model.Session, error) {
	return s.listSessions(ctx, conversationID, limit)
}

func (s *Service) ListSessions(ctx context.Context, limit int) ([]model.Session, error) {
	return s.listSessions(ctx, "", limit)
}

func (s *Service) listSessions(ctx context.Context, conversationID string, limit int) ([]model.Session, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `SELECT sess.id, sess.conversation_id, sess.key, sess.agent_id, sess.role,
		        COALESCE(sess.parent_session_id, ''), COALESCE(sess.controller_session_id, ''),
		        sess.status, sess.created_at, COALESCE(activity.updated_at, sess.created_at) AS updated_at
		 FROM sessions sess
		 LEFT JOIN (
		     SELECT session_id, MAX(activity_at) AS updated_at
		     FROM (
		         SELECT session_id, created_at AS activity_at
		         FROM session_messages
		         UNION ALL
		         SELECT session_id, updated_at AS activity_at
		         FROM runs
		         WHERE session_id IS NOT NULL AND session_id != ''
		     )
		     GROUP BY session_id
		 ) activity ON activity.session_id = sess.id`
	args := []any{limit}
	if conversationID != "" {
		query += `
		 WHERE sess.conversation_id = ?`
		args = []any{conversationID, limit}
	}
	query += `
		 ORDER BY updated_at DESC, sess.created_at DESC, sess.id DESC
		 LIMIT ?`

	rows, err := s.db.RawDB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	list := make([]model.Session, 0)
	for rows.Next() {
		var session model.Session
		var role string
		var updatedAt string
		if err := rows.Scan(
			&session.ID,
			&session.ConversationID,
			&session.Key,
			&session.AgentID,
			&role,
			&session.ParentSessionID,
			&session.ControllerSessionID,
			&session.Status,
			&session.CreatedAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan conversation session: %w", err)
		}
		parsedUpdatedAt, err := parseActivityTime(updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse conversation session updated_at: %w", err)
		}
		session.UpdatedAt = parsedUpdatedAt
		session.Role = model.SessionRole(role)
		list = append(list, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sessions: %w", err)
	}
	return list, nil
}

func (s *Service) LoadRouteBySession(ctx context.Context, sessionID string) (model.SessionRoute, error) {
	var route model.SessionRoute
	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT session_id, thread_id, connector_id, account_id, external_id, status, created_at
		 FROM session_bindings
		 WHERE session_id = ? AND status = 'active'
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		sessionID,
	).Scan(
		&route.SessionID,
		&route.ThreadID,
		&route.ConnectorID,
		&route.AccountID,
		&route.ExternalID,
		&route.Status,
		&route.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return model.SessionRoute{}, ErrSessionRouteNotFound
	}
	if err != nil {
		return model.SessionRoute{}, fmt.Errorf("load route by session: %w", err)
	}
	return route, nil
}

func (s *Service) LoadSessionOutboundIntent(ctx context.Context, sessionID string, intentID string) (model.OutboundIntent, error) {
	var intent model.OutboundIntent
	var lastAttempt sql.NullString
	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT oi.id, COALESCE(oi.run_id, ''), oi.connector_id, oi.chat_id, oi.message_text,
		        COALESCE(oi.dedupe_key, ''), oi.status, oi.attempts, oi.created_at, oi.last_attempt_at
		 FROM outbound_intents oi
		 JOIN runs r ON r.id = oi.run_id
		 WHERE r.session_id = ? AND oi.id = ?`,
		sessionID,
		intentID,
	).Scan(
		&intent.ID,
		&intent.RunID,
		&intent.ConnectorID,
		&intent.ChatID,
		&intent.MessageText,
		&intent.DedupeKey,
		&intent.Status,
		&intent.Attempts,
		&intent.CreatedAt,
		&lastAttempt,
	)
	if err == sql.ErrNoRows {
		return model.OutboundIntent{}, ErrOutboundIntentNotFound
	}
	if err != nil {
		return model.OutboundIntent{}, fmt.Errorf("load session outbound intent: %w", err)
	}
	if lastAttempt.Valid && lastAttempt.String != "" {
		parsed, err := parseActivityTime(lastAttempt.String)
		if err != nil {
			return model.OutboundIntent{}, fmt.Errorf("parse outbound intent last_attempt_at: %w", err)
		}
		intent.LastAttemptAt = &parsed
	}
	return intent, nil
}

func (s *Service) ListSessionOutboundIntents(ctx context.Context, sessionID string, limit int) ([]model.OutboundIntent, error) {
	var (
		rows *sql.Rows
		err  error
	)

	if limit > 0 {
		rows, err = s.db.RawDB().QueryContext(ctx,
			`SELECT id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at, last_attempt_at
			 FROM (
			     SELECT oi.id, COALESCE(oi.run_id, '') AS run_id, oi.connector_id, oi.chat_id, oi.message_text,
			            COALESCE(oi.dedupe_key, '') AS dedupe_key, oi.status, oi.attempts, oi.created_at, oi.last_attempt_at
			     FROM outbound_intents oi
			     JOIN runs r ON r.id = oi.run_id
			     WHERE r.session_id = ?
			     ORDER BY oi.created_at DESC, oi.id DESC
			     LIMIT ?
			 )
			 ORDER BY created_at ASC, id ASC`,
			sessionID, limit,
		)
	} else {
		rows, err = s.db.RawDB().QueryContext(ctx,
			`SELECT oi.id, COALESCE(oi.run_id, ''), oi.connector_id, oi.chat_id, oi.message_text,
			        COALESCE(oi.dedupe_key, ''), oi.status, oi.attempts, oi.created_at, oi.last_attempt_at
			 FROM outbound_intents oi
			 JOIN runs r ON r.id = oi.run_id
			 WHERE r.session_id = ?
			 ORDER BY oi.created_at ASC, oi.id ASC`,
			sessionID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list session outbound intents: %w", err)
	}
	defer rows.Close()

	intents := make([]model.OutboundIntent, 0)
	for rows.Next() {
		var intent model.OutboundIntent
		var lastAttempt sql.NullString
		if err := rows.Scan(
			&intent.ID,
			&intent.RunID,
			&intent.ConnectorID,
			&intent.ChatID,
			&intent.MessageText,
			&intent.DedupeKey,
			&intent.Status,
			&intent.Attempts,
			&intent.CreatedAt,
			&lastAttempt,
		); err != nil {
			return nil, fmt.Errorf("scan session outbound intent: %w", err)
		}
		if lastAttempt.Valid && lastAttempt.String != "" {
			parsed, err := parseActivityTime(lastAttempt.String)
			if err != nil {
				return nil, fmt.Errorf("parse outbound intent last_attempt_at: %w", err)
			}
			intent.LastAttemptAt = &parsed
		}
		intents = append(intents, intent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session outbound intents: %w", err)
	}
	return intents, nil
}

func (s *Service) ListSessionDeliveryFailures(ctx context.Context, sessionID string, limit int) ([]model.DeliveryFailure, error) {
	var (
		rows *sql.Rows
		err  error
	)

	if limit > 0 {
		rows, err = s.db.RawDB().QueryContext(ctx,
			`SELECT id, run_id, payload_json, created_at
			 FROM (
			     SELECT e.id, COALESCE(e.run_id, '') AS run_id, COALESCE(e.payload_json, x'') AS payload_json, e.created_at
			     FROM events e
			     JOIN runs r ON r.id = e.run_id
			     WHERE e.kind = 'delivery_failed' AND r.session_id = ?
			     ORDER BY e.created_at DESC, e.id DESC
			     LIMIT ?
			 )
			 ORDER BY created_at ASC, id ASC`,
			sessionID, limit,
		)
	} else {
		rows, err = s.db.RawDB().QueryContext(ctx,
			`SELECT e.id, COALESCE(e.run_id, ''), COALESCE(e.payload_json, x''), e.created_at
			 FROM events e
			 JOIN runs r ON r.id = e.run_id
			 WHERE e.kind = 'delivery_failed' AND r.session_id = ?
			 ORDER BY e.created_at ASC, e.id ASC`,
			sessionID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list session delivery failures: %w", err)
	}
	defer rows.Close()

	failures := make([]model.DeliveryFailure, 0)
	for rows.Next() {
		var failure model.DeliveryFailure
		var payloadJSON []byte
		if err := rows.Scan(
			&failure.ID,
			&failure.RunID,
			&payloadJSON,
			&failure.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan session delivery failure: %w", err)
		}
		var payload struct {
			ConnectorID string `json:"connector_id"`
			ChatID      string `json:"chat_id"`
			EventKind   string `json:"event_kind"`
			Error       string `json:"error"`
		}
		if len(payloadJSON) > 0 {
			if err := json.Unmarshal(payloadJSON, &payload); err != nil {
				return nil, fmt.Errorf("unmarshal session delivery failure payload: %w", err)
			}
		}
		failure.ConnectorID = payload.ConnectorID
		failure.ChatID = payload.ChatID
		failure.EventKind = payload.EventKind
		failure.Error = payload.Error
		failures = append(failures, failure)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session delivery failures: %w", err)
	}
	return failures, nil
}

func parseActivityTime(value string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format %q", value)
}

func (s *Service) loadBoundSession(ctx context.Context, conversationID string, threadID string) (model.Session, error) {
	var session model.Session
	var role string
	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT sess.id, sess.conversation_id, sess.key, sess.agent_id, sess.role,
		        COALESCE(sess.parent_session_id, ''), COALESCE(sess.controller_session_id, ''),
		        sess.status, sess.created_at
		 FROM session_bindings bind
		 JOIN sessions sess ON sess.id = bind.session_id
		 WHERE bind.conversation_id = ? AND bind.thread_id = ? AND bind.status = 'active'
		 ORDER BY bind.created_at DESC, bind.id DESC
		 LIMIT 1`,
		conversationID, threadID,
	).Scan(
		&session.ID,
		&session.ConversationID,
		&session.Key,
		&session.AgentID,
		&role,
		&session.ParentSessionID,
		&session.ControllerSessionID,
		&session.Status,
		&session.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return model.Session{}, ErrThreadMailboxNotFound
	}
	if err != nil {
		return model.Session{}, fmt.Errorf("load bound session: %w", err)
	}

	session.Role = model.SessionRole(role)
	session.UpdatedAt = session.CreatedAt
	return session, nil
}

func (s *Service) loadSession(ctx context.Context, sessionID string) (model.Session, error) {
	var session model.Session
	var role string
	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT id, conversation_id, key, agent_id, role,
		        COALESCE(parent_session_id, ''), COALESCE(controller_session_id, ''),
		        status, created_at
		 FROM sessions
		 WHERE id = ?`,
		sessionID,
	).Scan(
		&session.ID,
		&session.ConversationID,
		&session.Key,
		&session.AgentID,
		&role,
		&session.ParentSessionID,
		&session.ControllerSessionID,
		&session.Status,
		&session.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return model.Session{}, ErrSessionNotFound
	}
	if err != nil {
		return model.Session{}, fmt.Errorf("load session: %w", err)
	}
	session.Role = model.SessionRole(role)
	session.UpdatedAt = session.CreatedAt
	return session, nil
}

func (s *Service) listSessionMessages(ctx context.Context, sessionID string, limit int) ([]model.SessionMessage, error) {
	var (
		rows *sql.Rows
		err  error
	)

	if limit > 0 {
		rows, err = s.db.RawDB().QueryContext(ctx,
			`SELECT id, session_id, sender_session_id, kind, body, COALESCE(provenance_json, '{}'), created_at
				 FROM (
				     SELECT id, session_id, COALESCE(sender_session_id, '') AS sender_session_id, kind, body, COALESCE(provenance_json, '{}') AS provenance_json, created_at
				     FROM session_messages
				     WHERE session_id = ?
				     ORDER BY created_at DESC, id DESC
			     LIMIT ?
			 )
			 ORDER BY created_at ASC, id ASC`,
			sessionID, limit,
		)
	} else {
		rows, err = s.db.RawDB().QueryContext(ctx,
			`SELECT id, session_id, COALESCE(sender_session_id, ''), kind, body, COALESCE(provenance_json, '{}'), created_at
				 FROM session_messages
				 WHERE session_id = ?
				 ORDER BY created_at ASC, id ASC`,
			sessionID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list session messages: %w", err)
	}
	defer rows.Close()

	messages := make([]model.SessionMessage, 0)
	for rows.Next() {
		var msg model.SessionMessage
		var kind string
		var provenanceJSON []byte
		if err := rows.Scan(
			&msg.ID,
			&msg.SessionID,
			&msg.SenderSessionID,
			&kind,
			&msg.Body,
			&provenanceJSON,
			&msg.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan session message: %w", err)
		}
		msg.Kind = model.SessionMessageKind(kind)
		if len(provenanceJSON) > 0 {
			if err := json.Unmarshal(provenanceJSON, &msg.Provenance); err != nil {
				return nil, fmt.Errorf("unmarshal session message provenance: %w", err)
			}
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session messages: %w", err)
	}

	return messages, nil
}

func normalizeThreadID(threadID string) string {
	if threadID == "" {
		return "main"
	}
	return threadID
}

func (s *Service) appendSessionOpened(ctx context.Context, runID string, session model.Session) error {
	payload := map[string]any{
		"session_id":            session.ID,
		"key":                   session.Key,
		"agent_id":              session.AgentID,
		"role":                  session.Role,
		"parent_session_id":     session.ParentSessionID,
		"controller_session_id": session.ControllerSessionID,
		"status":                session.Status,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal session_opened payload: %w", err)
	}
	return s.conv.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: session.ConversationID,
		RunID:          runID,
		Kind:           "session_opened",
		PayloadJSON:    body,
		CreatedAt:      session.CreatedAt,
	})
}

func (s *Service) appendSessionMessage(ctx context.Context, runID string, msg model.SessionMessage) error {
	conversationID, err := s.conversationIDForSession(ctx, msg.SessionID)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"message_id":        msg.ID,
		"session_id":        msg.SessionID,
		"sender_session_id": msg.SenderSessionID,
		"kind":              msg.Kind,
		"body":              msg.Body,
		"provenance":        msg.Provenance,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal session_message_added payload: %w", err)
	}
	return s.conv.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: conversationID,
		RunID:          runID,
		Kind:           "session_message_added",
		PayloadJSON:    body,
		CreatedAt:      msg.CreatedAt,
	})
}

func (s *Service) appendSessionBinding(ctx context.Context, runID string, cmd BindFollowUp) error {
	now := time.Now().UTC()
	payload := map[string]any{
		"thread_id":    normalizeThreadID(cmd.ThreadID),
		"session_id":   cmd.SessionID,
		"connector_id": cmd.ConnectorID,
		"account_id":   cmd.AccountID,
		"external_id":  cmd.ExternalID,
		"status":       "active",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal session_bound payload: %w", err)
	}
	return s.conv.AppendEvent(ctx, model.Event{
		ID:             generateID(),
		ConversationID: cmd.ConversationID,
		RunID:          runID,
		Kind:           "session_bound",
		PayloadJSON:    body,
		CreatedAt:      now,
	})
}

func (s *Service) conversationIDForSession(ctx context.Context, sessionID string) (string, error) {
	var conversationID string
	err := s.db.RawDB().QueryRowContext(ctx,
		"SELECT conversation_id FROM sessions WHERE id = ?",
		sessionID,
	).Scan(&conversationID)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("lookup session conversation: session %s not found", sessionID)
	}
	if err != nil {
		return "", fmt.Errorf("lookup session conversation: %w", err)
	}
	return conversationID, nil
}

func generateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
