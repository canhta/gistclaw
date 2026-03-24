package sessions

import (
	"context"
	"crypto/rand"
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

func generateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
