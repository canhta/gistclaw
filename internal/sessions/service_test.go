package sessions

import (
	"context"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

func newTestSessionService(t *testing.T) *Service {
	t.Helper()

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return &Service{
		db:   db,
		conv: conversations.NewConversationStore(db),
	}
}

func openFrontSession(t *testing.T, svc *Service) model.Session {
	t.Helper()

	sess, err := svc.OpenFrontSession(context.Background(), OpenFrontSession{
		ConversationID: "conv-1",
		AgentID:        "assistant",
		WorkspaceRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("OpenFrontSession failed: %v", err)
	}
	return sess
}

func TestService_OpenFrontSession(t *testing.T) {
	svc := newTestSessionService(t)

	sess, err := svc.OpenFrontSession(context.Background(), OpenFrontSession{
		ConversationID: "conv-1",
		AgentID:        "assistant",
		WorkspaceRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if sess.Role != model.SessionRoleFront {
		t.Fatalf("expected front session, got %s", sess.Role)
	}
}

func TestService_SpawnWorkerSessionPersistsMessage(t *testing.T) {
	svc := newTestSessionService(t)
	parent := openFrontSession(t, svc)

	child, err := svc.SpawnWorkerSession(context.Background(), SpawnWorkerSession{
		ConversationID:      parent.ConversationID,
		ParentSessionID:     parent.ID,
		ControllerSessionID: parent.ID,
		AgentID:             "researcher",
		InitialPrompt:       "Investigate the repo layout.",
	})
	if err != nil {
		t.Fatal(err)
	}

	var kind string
	err = svc.db.RawDB().QueryRowContext(
		context.Background(),
		"SELECT kind FROM session_messages WHERE session_id = ? ORDER BY created_at ASC LIMIT 1",
		child.ID,
	).Scan(&kind)
	if err != nil {
		t.Fatalf("query session message: %v", err)
	}
	if kind != string(model.MessageSpawn) {
		t.Fatalf("expected first session message kind %q, got %q", model.MessageSpawn, kind)
	}
}
