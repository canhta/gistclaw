package sessions

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
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

	_, err := s.db.RawDB().ExecContext(ctx,
		`INSERT INTO sessions
		 (id, conversation_id, key, agent_id, role, parent_session_id, controller_session_id, status, created_at)
		 VALUES (?, ?, ?, ?, ?, '', '', ?, ?)`,
		session.ID, session.ConversationID, session.Key, session.AgentID, session.Role, session.Status, session.CreatedAt,
	)
	if err != nil {
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

	_, err := s.db.RawDB().ExecContext(ctx,
		`INSERT INTO sessions
		 (id, conversation_id, key, agent_id, role, parent_session_id, controller_session_id, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.ConversationID, session.Key, session.AgentID, session.Role,
		session.ParentSessionID, session.ControllerSessionID, session.Status, session.CreatedAt,
	)
	if err != nil {
		return model.Session{}, fmt.Errorf("spawn worker session: %w", err)
	}

	if err := s.AppendMessage(ctx, model.SessionMessage{
		ID:              generateID(),
		SessionID:       session.ID,
		SenderSessionID: cmd.ControllerSessionID,
		Kind:            model.MessageSpawn,
		Body:            cmd.InitialPrompt,
		CreatedAt:       now,
	}); err != nil {
		return model.Session{}, err
	}

	return session, nil
}

func (s *Service) AppendMessage(ctx context.Context, msg model.SessionMessage) error {
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.RawDB().ExecContext(ctx,
		`INSERT INTO session_messages
		 (id, session_id, sender_session_id, kind, body, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.SessionID, msg.SenderSessionID, msg.Kind, msg.Body, msg.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("append session message: %w", err)
	}
	return nil
}

func (s *Service) BindFollowUp(ctx context.Context, cmd BindFollowUp) error {
	_, err := s.db.RawDB().ExecContext(ctx,
		`INSERT INTO session_bindings (id, conversation_id, thread_id, session_id, status, created_at)
		 VALUES (?, ?, ?, ?, 'active', ?)`,
		generateID(), cmd.ConversationID, cmd.ThreadID, cmd.SessionID, time.Now().UTC(),
	)
	if err != nil {
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

func (s *Service) listSessionMessages(ctx context.Context, sessionID string, limit int) ([]model.SessionMessage, error) {
	var (
		rows *sql.Rows
		err  error
	)

	if limit > 0 {
		rows, err = s.db.RawDB().QueryContext(ctx,
			`SELECT id, session_id, sender_session_id, kind, body, created_at
			 FROM (
			     SELECT id, session_id, COALESCE(sender_session_id, '') AS sender_session_id, kind, body, created_at
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
			`SELECT id, session_id, COALESCE(sender_session_id, ''), kind, body, created_at
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
		if err := rows.Scan(
			&msg.ID,
			&msg.SessionID,
			&msg.SenderSessionID,
			&kind,
			&msg.Body,
			&msg.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan session message: %w", err)
		}
		msg.Kind = model.SessionMessageKind(kind)
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

func generateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
