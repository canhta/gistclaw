package telegram

import (
	"context"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
)

// stubStarter records the StartRun calls made to it.
type stubStarter struct {
	calls []runtime.StartRun
}

func (s *stubStarter) Start(_ context.Context, req runtime.StartRun) (model.Run, error) {
	s.calls = append(s.calls, req)
	return model.Run{ID: "test-run"}, nil
}

func setupDispatchDB(t *testing.T) (*store.DB, *conversations.ConversationStore) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cs := conversations.NewConversationStore(db)
	return db, cs
}

func TestInboundDispatcher_DispatchesEnvelopeToRuntime(t *testing.T) {
	_, cs := setupDispatchDB(t)
	starter := &stubStarter{}
	dispatcher := NewInboundDispatcher(cs, starter, "cli-operator", "")

	env := model.Envelope{
		ConnectorID:    "telegram",
		AccountID:      "123456",
		ActorID:        "123456",
		ConversationID: "123456",
		ThreadID:       "main",
		MessageID:      "42",
		Text:           "review the auth module",
	}

	if err := dispatcher.Dispatch(context.Background(), env); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if len(starter.calls) != 1 {
		t.Fatalf("expected 1 rt.Start call, got %d", len(starter.calls))
	}
	call := starter.calls[0]
	if call.Objective != env.Text {
		t.Errorf("Objective: expected %q, got %q", env.Text, call.Objective)
	}
	if call.AccountID != env.AccountID {
		t.Errorf("AccountID: expected %q, got %q", env.AccountID, call.AccountID)
	}
	if call.ConversationID == "" {
		t.Error("ConversationID must be resolved and non-empty")
	}
}

func TestInboundDispatcher_EmptyTextIsRejected(t *testing.T) {
	_, cs := setupDispatchDB(t)
	starter := &stubStarter{}
	dispatcher := NewInboundDispatcher(cs, starter, "cli-operator", "")

	env := model.Envelope{
		ConnectorID: "telegram",
		AccountID:   "123456",
		ThreadID:    "main",
		Text:        "",
	}

	if err := dispatcher.Dispatch(context.Background(), env); err == nil {
		t.Fatal("expected error for empty text, got nil")
	}
	if len(starter.calls) != 0 {
		t.Fatalf("expected no rt.Start calls for empty text, got %d", len(starter.calls))
	}
}
