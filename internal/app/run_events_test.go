package app

import (
	"context"
	"sync"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

type recordingNotifyConnector struct {
	id     string
	mu     sync.Mutex
	chatID string
	event  model.ReplayDelta
	calls  int
}

func (c *recordingNotifyConnector) ID() string { return c.id }

func (c *recordingNotifyConnector) Start(context.Context) error { return nil }

func (c *recordingNotifyConnector) Drain(context.Context) error { return nil }

func (c *recordingNotifyConnector) Notify(_ context.Context, chatID string, evt model.ReplayDelta, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.chatID = chatID
	c.event = evt
	c.calls++
	return nil
}

func TestConnectorRouteNotifier_EmitsTurnDeltasToBoundConnector(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	cs := conversations.NewConversationStore(db)
	conv, err := cs.Resolve(context.Background(), conversations.ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct-1",
		ExternalID:  "chat-1",
		ThreadID:    "main",
	})
	if err != nil {
		t.Fatalf("resolve conversation: %v", err)
	}

	if _, err := db.RawDB().Exec(
		`INSERT INTO sessions (id, conversation_id, key, agent_id, role, status, created_at)
		 VALUES ('session-front', ?, 'front:'+?, 'assistant', 'front', 'active', datetime('now'))`,
		conv.ID, conv.ID,
	); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := db.RawDB().Exec(
		`INSERT INTO session_bindings
		 (id, conversation_id, thread_id, session_id, connector_id, account_id, external_id, status, created_at)
		 VALUES ('route-1', ?, 'main', 'session-front', 'telegram', 'acct-1', 'chat-1', 'active', datetime('now'))`,
		conv.ID,
	); err != nil {
		t.Fatalf("insert route: %v", err)
	}
	if _, err := db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, session_id, objective, cwd, status, created_at, updated_at)
		 VALUES ('run-1', ?, 'assistant', 'session-front', 'reply', ?, 'active', datetime('now'), datetime('now'))`,
		conv.ID, t.TempDir(),
	); err != nil {
		t.Fatalf("insert run: %v", err)
	}

	connector := &recordingNotifyConnector{id: "telegram"}
	notifier := newConnectorRouteNotifier(db)
	notifier.SetConnectors([]model.Connector{connector})

	err = notifier.Emit(context.Background(), "run-1", model.ReplayDelta{
		RunID:       "run-1",
		Kind:        "turn_delta",
		PayloadJSON: []byte(`{"text":"Hel"}`),
	})
	if err != nil {
		t.Fatalf("emit route event: %v", err)
	}

	if connector.calls != 1 {
		t.Fatalf("expected 1 connector notify call, got %d", connector.calls)
	}
	if connector.chatID != "chat-1" {
		t.Fatalf("expected chat id %q, got %q", "chat-1", connector.chatID)
	}
	if connector.event.Kind != "turn_delta" {
		t.Fatalf("expected turn_delta event, got %q", connector.event.Kind)
	}
}

func TestConnectorRouteNotifier_RoutesWorkerApprovalRequestsToFrontBinding(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	cs := conversations.NewConversationStore(db)
	conv, err := cs.Resolve(context.Background(), conversations.ConversationKey{
		ConnectorID: "telegram",
		AccountID:   "acct-1",
		ExternalID:  "chat-1",
		ThreadID:    "main",
	})
	if err != nil {
		t.Fatalf("resolve conversation: %v", err)
	}

	if _, err := db.RawDB().Exec(
		`INSERT INTO sessions (id, conversation_id, key, agent_id, role, status, created_at)
		 VALUES ('session-front', ?, 'front:'+?, 'assistant', 'front', 'active', datetime('now')),
		        ('session-worker', ?, 'worker:patcher', 'patcher', 'worker', 'active', datetime('now'))`,
		conv.ID, conv.ID, conv.ID,
	); err != nil {
		t.Fatalf("insert sessions: %v", err)
	}
	if _, err := db.RawDB().Exec(
		`INSERT INTO session_bindings
		 (id, conversation_id, thread_id, session_id, connector_id, account_id, external_id, status, created_at)
		 VALUES ('route-1', ?, 'main', 'session-front', 'telegram', 'acct-1', 'chat-1', 'active', datetime('now'))`,
		conv.ID,
	); err != nil {
		t.Fatalf("insert route: %v", err)
	}
	if _, err := db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, session_id, parent_run_id, objective, cwd, status, created_at, updated_at)
		 VALUES ('run-front', ?, 'assistant', 'session-front', NULL, 'coordinate', ?, 'active', datetime('now'), datetime('now')),
		        ('run-worker', ?, 'patcher', 'session-worker', 'run-front', 'patch', ?, 'needs_approval', datetime('now'), datetime('now'))`,
		conv.ID, t.TempDir(), conv.ID, t.TempDir(),
	); err != nil {
		t.Fatalf("insert runs: %v", err)
	}

	connector := &recordingNotifyConnector{id: "telegram"}
	notifier := newConnectorRouteNotifier(db)
	notifier.SetConnectors([]model.Connector{connector})

	err = notifier.Emit(context.Background(), "run-worker", model.ReplayDelta{
		RunID:       "run-worker",
		Kind:        "approval_requested",
		PayloadJSON: []byte(`{"approval_id":"ticket-1","tool_name":"shell_exec"}`),
	})
	if err != nil {
		t.Fatalf("emit worker approval event: %v", err)
	}

	if connector.calls != 1 {
		t.Fatalf("expected 1 connector notify call, got %d", connector.calls)
	}
	if connector.chatID != "chat-1" {
		t.Fatalf("expected approval request to route to front chat %q, got %q", "chat-1", connector.chatID)
	}
	if connector.event.Kind != "approval_requested" {
		t.Fatalf("expected approval_requested event, got %q", connector.event.Kind)
	}
}
