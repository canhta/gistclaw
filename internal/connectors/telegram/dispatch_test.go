package telegram

import (
	"context"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
)

// stubStarter records the StartFrontSession calls made to it.
type stubStarter struct {
	calls []runtime.StartFrontSession
}

func (s *stubStarter) StartFrontSession(_ context.Context, req runtime.StartFrontSession) (model.Run, error) {
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
	starter := &stubStarter{}
	dispatcher := NewInboundDispatcher(starter, "assistant", "")

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
		t.Fatalf("expected 1 StartFrontSession call, got %d", len(starter.calls))
	}
	call := starter.calls[0]
	if call.InitialPrompt != env.Text {
		t.Errorf("InitialPrompt: expected %q, got %q", env.Text, call.InitialPrompt)
	}
	if call.FrontAgentID != "assistant" {
		t.Errorf("FrontAgentID: expected %q, got %q", "assistant", call.FrontAgentID)
	}
	if call.ConversationKey.ConnectorID != env.ConnectorID {
		t.Errorf("ConnectorID: expected %q, got %q", env.ConnectorID, call.ConversationKey.ConnectorID)
	}
	if call.ConversationKey.AccountID != env.AccountID {
		t.Errorf("AccountID: expected %q, got %q", env.AccountID, call.ConversationKey.AccountID)
	}
	if call.ConversationKey.ExternalID != env.ConversationID {
		t.Errorf("ExternalID: expected %q, got %q", env.ConversationID, call.ConversationKey.ExternalID)
	}
	if call.ConversationKey.ThreadID != env.ThreadID {
		t.Errorf("ThreadID: expected %q, got %q", env.ThreadID, call.ConversationKey.ThreadID)
	}
}

func TestInboundDispatcher_EmptyTextIsRejected(t *testing.T) {
	starter := &stubStarter{}
	dispatcher := NewInboundDispatcher(starter, "assistant", "")

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
		t.Fatalf("expected no StartFrontSession calls for empty text, got %d", len(starter.calls))
	}
}
