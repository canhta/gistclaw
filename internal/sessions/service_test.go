package sessions

import (
	"context"
	"testing"
	"time"

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

func TestService_LoadThreadMailboxReturnsLatestMessagesInChronologicalOrder(t *testing.T) {
	svc := newTestSessionService(t)
	ctx := context.Background()
	front := openFrontSession(t, svc)

	if err := svc.BindFollowUp(ctx, BindFollowUp{
		ConversationID: front.ConversationID,
		ThreadID:       "main",
		SessionID:      front.ID,
	}); err != nil {
		t.Fatalf("BindFollowUp failed: %v", err)
	}

	base := time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC)
	messages := []model.SessionMessage{
		{ID: "msg-1", SessionID: front.ID, Kind: model.MessageUser, Body: "first", CreatedAt: base},
		{ID: "msg-2", SessionID: front.ID, Kind: model.MessageAssistant, Body: "second", CreatedAt: base.Add(time.Minute)},
		{ID: "msg-3", SessionID: front.ID, Kind: model.MessageAnnounce, Body: "third", CreatedAt: base.Add(2 * time.Minute)},
	}
	for _, msg := range messages {
		if err := svc.AppendMessage(ctx, msg); err != nil {
			t.Fatalf("AppendMessage %s failed: %v", msg.ID, err)
		}
	}

	session, mailbox, err := svc.LoadThreadMailbox(ctx, front.ConversationID, "", 2)
	if err != nil {
		t.Fatalf("LoadThreadMailbox failed: %v", err)
	}
	if session.ID != front.ID {
		t.Fatalf("expected bound session %q, got %q", front.ID, session.ID)
	}
	if len(mailbox) != 2 {
		t.Fatalf("expected 2 mailbox messages, got %d", len(mailbox))
	}
	if mailbox[0].ID != "msg-2" || mailbox[1].ID != "msg-3" {
		t.Fatalf("expected latest messages in chronological order, got %q then %q", mailbox[0].ID, mailbox[1].ID)
	}
}

func TestService_LoadThreadMailboxWithoutBindingFails(t *testing.T) {
	svc := newTestSessionService(t)

	_, _, err := svc.LoadThreadMailbox(context.Background(), "conv-1", "main", 10)
	if err == nil {
		t.Fatal("expected missing binding error, got nil")
	}
}
