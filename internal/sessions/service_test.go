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
		ConnectorID:    "telegram",
		AccountID:      "acct-1",
		ExternalID:     "chat-1",
	}); err != nil {
		t.Fatalf("BindFollowUp failed: %v", err)
	}

	base := time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC)
	messages := []model.SessionMessage{
		{
			ID:        "msg-1",
			SessionID: front.ID,
			Kind:      model.MessageUser,
			Body:      "first",
			Provenance: model.SessionMessageProvenance{
				Kind:              model.MessageProvenanceInbound,
				SourceConnectorID: "telegram",
				SourceThreadID:    "main",
			},
			CreatedAt: base,
		},
		{
			ID:        "msg-2",
			SessionID: front.ID,
			Kind:      model.MessageAssistant,
			Body:      "second",
			Provenance: model.SessionMessageProvenance{
				Kind:        model.MessageProvenanceAssistantTurn,
				SourceRunID: "run-front",
			},
			CreatedAt: base.Add(time.Minute),
		},
		{
			ID:              "msg-3",
			SessionID:       front.ID,
			SenderSessionID: "sess-worker",
			Kind:            model.MessageAnnounce,
			Body:            "third",
			Provenance: model.SessionMessageProvenance{
				Kind:            model.MessageProvenanceInterSession,
				SourceSessionID: "sess-worker",
				SourceRunID:     "run-worker",
			},
			CreatedAt: base.Add(2 * time.Minute),
		},
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
	if mailbox[0].Provenance.Kind != model.MessageProvenanceAssistantTurn || mailbox[0].Provenance.SourceRunID != "run-front" {
		t.Fatalf("expected assistant-turn provenance, got %+v", mailbox[0].Provenance)
	}
	if mailbox[1].Provenance.Kind != model.MessageProvenanceInterSession || mailbox[1].Provenance.SourceSessionID != "sess-worker" {
		t.Fatalf("expected inter-session provenance, got %+v", mailbox[1].Provenance)
	}
}

func TestService_LoadThreadMailboxWithoutBindingFails(t *testing.T) {
	svc := newTestSessionService(t)

	_, _, err := svc.LoadThreadMailbox(context.Background(), "conv-1", "main", 10)
	if err == nil {
		t.Fatal("expected missing binding error, got nil")
	}
}

func TestService_LoadRouteBySessionReturnsBoundDeliveryTarget(t *testing.T) {
	svc := newTestSessionService(t)
	ctx := context.Background()
	front := openFrontSession(t, svc)

	if err := svc.BindFollowUp(ctx, BindFollowUp{
		ConversationID: front.ConversationID,
		ThreadID:       "main",
		SessionID:      front.ID,
		ConnectorID:    "telegram",
		AccountID:      "acct-1",
		ExternalID:     "chat-1",
	}); err != nil {
		t.Fatalf("BindFollowUp failed: %v", err)
	}

	route, err := svc.LoadRouteBySession(ctx, front.ID)
	if err != nil {
		t.Fatalf("LoadRouteBySession failed: %v", err)
	}
	if route.SessionID != front.ID || route.ThreadID != "main" {
		t.Fatalf("unexpected route identity: session_id=%q thread_id=%q", route.SessionID, route.ThreadID)
	}
	if route.ConnectorID != "telegram" || route.AccountID != "acct-1" || route.ExternalID != "chat-1" {
		t.Fatalf(
			"unexpected route target: connector_id=%q account_id=%q external_id=%q",
			route.ConnectorID,
			route.AccountID,
			route.ExternalID,
		)
	}
}

func TestService_ListConversationSessionsOrdersByLatestActivity(t *testing.T) {
	svc := newTestSessionService(t)
	ctx := context.Background()
	front := openFrontSession(t, svc)
	worker, err := svc.SpawnWorkerSession(ctx, SpawnWorkerSession{
		ConversationID:      front.ConversationID,
		ParentSessionID:     front.ID,
		ControllerSessionID: front.ID,
		AgentID:             "researcher",
		InitialPrompt:       "Inspect docs.",
	})
	if err != nil {
		t.Fatalf("SpawnWorkerSession failed: %v", err)
	}

	base := time.Now().UTC().Add(time.Minute)
	if err := svc.AppendMessage(ctx, model.SessionMessage{
		ID:        "worker-msg",
		SessionID: worker.ID,
		Kind:      model.MessageAssistant,
		Body:      "Worker update",
		CreatedAt: base,
	}); err != nil {
		t.Fatalf("AppendMessage worker failed: %v", err)
	}
	if err := svc.AppendMessage(ctx, model.SessionMessage{
		ID:        "front-msg",
		SessionID: front.ID,
		Kind:      model.MessageAnnounce,
		Body:      "Front update",
		CreatedAt: base.Add(time.Minute),
	}); err != nil {
		t.Fatalf("AppendMessage front failed: %v", err)
	}

	list, err := svc.ListConversationSessions(ctx, front.ConversationID, 10)
	if err != nil {
		t.Fatalf("ListConversationSessions failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(list))
	}
	if list[0].ID != front.ID || list[1].ID != worker.ID {
		t.Fatalf("expected front session first and worker second, got %q then %q", list[0].ID, list[1].ID)
	}
	if !list[0].UpdatedAt.After(list[1].UpdatedAt) {
		t.Fatalf("expected front session updated_at %v to be after worker %v", list[0].UpdatedAt, list[1].UpdatedAt)
	}
}

func TestService_ListSessionsOrdersByLatestActivityAcrossConversations(t *testing.T) {
	svc := newTestSessionService(t)
	ctx := context.Background()

	first, err := svc.OpenFrontSession(ctx, OpenFrontSession{
		ConversationID: "conv-1",
		AgentID:        "assistant",
		WorkspaceRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("OpenFrontSession first failed: %v", err)
	}
	second, err := svc.OpenFrontSession(ctx, OpenFrontSession{
		ConversationID: "conv-2",
		AgentID:        "assistant",
		WorkspaceRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("OpenFrontSession second failed: %v", err)
	}

	base := time.Now().UTC().Add(time.Minute)
	if err := svc.AppendMessage(ctx, model.SessionMessage{
		ID:        "msg-first",
		SessionID: first.ID,
		Kind:      model.MessageAssistant,
		Body:      "older activity",
		CreatedAt: base,
	}); err != nil {
		t.Fatalf("AppendMessage first failed: %v", err)
	}
	if err := svc.AppendMessage(ctx, model.SessionMessage{
		ID:        "msg-second",
		SessionID: second.ID,
		Kind:      model.MessageAssistant,
		Body:      "newer activity",
		CreatedAt: base.Add(time.Minute),
	}); err != nil {
		t.Fatalf("AppendMessage second failed: %v", err)
	}

	list, err := svc.ListSessions(ctx, 10)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(list))
	}
	if list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("expected newer session first, got %q then %q", list[0].ID, list[1].ID)
	}
}
