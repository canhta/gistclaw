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

func TestService_ListSessionOutboundIntentsAndFailures(t *testing.T) {
	svc := newTestSessionService(t)
	ctx := context.Background()
	front := openFrontSession(t, svc)

	if _, err := svc.db.RawDB().ExecContext(
		ctx,
		`INSERT INTO runs
		 (id, conversation_id, agent_id, session_id, objective, workspace_root, status, created_at, updated_at)
		 VALUES ('run-front', ?, 'assistant', ?, 'Inspect the repo', ?, 'completed', datetime('now', '-2 minutes'), datetime('now', '-2 minutes'))`,
		front.ConversationID,
		front.ID,
		t.TempDir(),
	); err != nil {
		t.Fatalf("insert run: %v", err)
	}

	if _, err := svc.db.RawDB().ExecContext(
		ctx,
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at, last_attempt_at)
		 VALUES ('intent-1', 'run-front', 'telegram', 'chat-1', 'reply one', 'dedupe-1', 'terminal', 3, datetime('now', '-90 seconds'), datetime('now', '-80 seconds'))`,
	); err != nil {
		t.Fatalf("insert outbound intent: %v", err)
	}

	if err := svc.conv.AppendEvent(ctx, model.Event{
		ID:             "evt-delivery-failed",
		ConversationID: front.ConversationID,
		RunID:          "run-front",
		Kind:           "delivery_failed",
		PayloadJSON:    []byte(`{"intent_id":"intent-1","chat_id":"chat-1","connector_id":"telegram","event_kind":"run_completed","error":"provider offline"}`),
		CreatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append delivery_failed event: %v", err)
	}

	deliveries, err := svc.ListSessionOutboundIntents(ctx, front.ID, 10)
	if err != nil {
		t.Fatalf("ListSessionOutboundIntents failed: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 outbound intent, got %d", len(deliveries))
	}
	if deliveries[0].ID != "intent-1" || deliveries[0].RunID != "run-front" {
		t.Fatalf("unexpected outbound intent identity: %+v", deliveries[0])
	}
	if deliveries[0].ConnectorID != "telegram" || deliveries[0].ChatID != "chat-1" {
		t.Fatalf("unexpected outbound target: %+v", deliveries[0])
	}
	if deliveries[0].Status != "terminal" || deliveries[0].Attempts != 3 {
		t.Fatalf("unexpected outbound delivery status: %+v", deliveries[0])
	}

	failures, err := svc.ListSessionDeliveryFailures(ctx, front.ID, 10)
	if err != nil {
		t.Fatalf("ListSessionDeliveryFailures failed: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("expected 1 delivery failure, got %d", len(failures))
	}
	if failures[0].RunID != "run-front" || failures[0].ConnectorID != "telegram" {
		t.Fatalf("unexpected delivery failure identity: %+v", failures[0])
	}
	if failures[0].ChatID != "chat-1" || failures[0].EventKind != "run_completed" || failures[0].Error != "provider offline" {
		t.Fatalf("unexpected delivery failure payload: %+v", failures[0])
	}
}

func TestService_ListSessionDeliveryFailuresHidesResolvedFailures(t *testing.T) {
	svc := newTestSessionService(t)
	ctx := context.Background()
	front := openFrontSession(t, svc)

	if _, err := svc.db.RawDB().ExecContext(
		ctx,
		`INSERT INTO runs
		 (id, conversation_id, agent_id, session_id, objective, workspace_root, status, created_at, updated_at)
		 VALUES ('run-front', ?, 'assistant', ?, 'Inspect the repo', ?, 'completed', datetime('now', '-2 minutes'), datetime('now', '-2 minutes'))`,
		front.ConversationID,
		front.ID,
		t.TempDir(),
	); err != nil {
		t.Fatalf("insert run: %v", err)
	}

	if _, err := svc.db.RawDB().ExecContext(
		ctx,
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at, last_attempt_at)
		 VALUES ('intent-1', 'run-front', 'telegram', 'chat-1', 'reply one', 'dedupe-1', 'pending', 0, datetime('now', '-90 seconds'), NULL)`,
	); err != nil {
		t.Fatalf("insert outbound intent: %v", err)
	}

	if err := svc.conv.AppendEvent(ctx, model.Event{
		ID:             "evt-delivery-failed",
		ConversationID: front.ConversationID,
		RunID:          "run-front",
		Kind:           "delivery_failed",
		PayloadJSON:    []byte(`{"intent_id":"intent-1","chat_id":"chat-1","connector_id":"telegram","event_kind":"run_completed","error":"provider offline"}`),
		CreatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append delivery_failed event: %v", err)
	}

	failures, err := svc.ListSessionDeliveryFailures(ctx, front.ID, 10)
	if err != nil {
		t.Fatalf("ListSessionDeliveryFailures failed: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("expected resolved delivery failure to be hidden, got %d", len(failures))
	}
}

func TestService_ListConnectorDeliveryHealth(t *testing.T) {
	svc := newTestSessionService(t)
	ctx := context.Background()

	frontTelegram := openFrontSession(t, svc)
	frontWhatsApp, err := svc.OpenFrontSession(ctx, OpenFrontSession{
		ConversationID: "conv-whatsapp",
		AgentID:        "assistant",
		WorkspaceRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("OpenFrontSession whatsapp failed: %v", err)
	}

	if _, err := svc.db.RawDB().ExecContext(
		ctx,
		`INSERT INTO runs
		 (id, conversation_id, agent_id, session_id, objective, workspace_root, status, created_at, updated_at)
		 VALUES
		 ('run-telegram', ?, 'assistant', ?, 'Inspect Telegram', ?, 'completed', datetime('now', '-5 minutes'), datetime('now', '-5 minutes')),
		 ('run-whatsapp', ?, 'assistant', ?, 'Inspect WhatsApp', ?, 'completed', datetime('now', '-5 minutes'), datetime('now', '-5 minutes'))`,
		frontTelegram.ConversationID,
		frontTelegram.ID,
		t.TempDir(),
		frontWhatsApp.ConversationID,
		frontWhatsApp.ID,
		t.TempDir(),
	); err != nil {
		t.Fatalf("insert runs: %v", err)
	}

	if _, err := svc.db.RawDB().ExecContext(
		ctx,
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at, last_attempt_at)
		 VALUES
		 ('intent-telegram-pending', 'run-telegram', 'telegram', 'chat-1', 'reply one', 'dedupe-1', 'pending', 0, datetime('now', '-4 minutes'), NULL),
		 ('intent-telegram-retrying', 'run-telegram', 'telegram', 'chat-1', 'reply two', 'dedupe-2', 'retrying', 2, datetime('now', '-3 minutes'), datetime('now', '-2 minutes')),
		 ('intent-telegram-terminal', 'run-telegram', 'telegram', 'chat-1', 'reply three', 'dedupe-3', 'terminal', 5, datetime('now', '-2 minutes'), datetime('now', '-1 minutes')),
		 ('intent-whatsapp-pending', 'run-whatsapp', 'whatsapp', 'chat-2', 'reply four', 'dedupe-4', 'pending', 0, datetime('now', '-90 seconds'), NULL)`,
	); err != nil {
		t.Fatalf("insert outbound intents: %v", err)
	}

	health, err := svc.ListConnectorDeliveryHealth(ctx)
	if err != nil {
		t.Fatalf("ListConnectorDeliveryHealth failed: %v", err)
	}
	if len(health) != 2 {
		t.Fatalf("expected 2 connector summaries, got %d", len(health))
	}

	if health[0].ConnectorID != "telegram" {
		t.Fatalf("expected telegram first, got %+v", health[0])
	}
	if health[0].PendingCount != 1 || health[0].RetryingCount != 1 || health[0].TerminalCount != 1 {
		t.Fatalf("unexpected telegram counts: %+v", health[0])
	}
	if health[0].OldestPendingAt == nil || health[0].OldestRetryingAt == nil {
		t.Fatalf("expected telegram oldest timestamps, got %+v", health[0])
	}

	if health[1].ConnectorID != "whatsapp" {
		t.Fatalf("expected whatsapp second, got %+v", health[1])
	}
	if health[1].PendingCount != 1 || health[1].RetryingCount != 0 || health[1].TerminalCount != 0 {
		t.Fatalf("unexpected whatsapp counts: %+v", health[1])
	}
	if health[1].OldestPendingAt == nil {
		t.Fatalf("expected whatsapp pending timestamp, got %+v", health[1])
	}
	if health[1].OldestRetryingAt != nil {
		t.Fatalf("expected whatsapp retrying timestamp to be nil, got %+v", health[1])
	}
}

func TestService_ListDeliveryQueue(t *testing.T) {
	svc := newTestSessionService(t)
	ctx := context.Background()

	frontTelegram := openFrontSession(t, svc)
	frontWhatsApp, err := svc.OpenFrontSession(ctx, OpenFrontSession{
		ConversationID: "conv-whatsapp",
		AgentID:        "assistant",
		WorkspaceRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("OpenFrontSession whatsapp failed: %v", err)
	}

	if _, err := svc.db.RawDB().ExecContext(
		ctx,
		`INSERT INTO runs
		 (id, conversation_id, agent_id, session_id, objective, workspace_root, status, created_at, updated_at)
		 VALUES
		 ('run-telegram', ?, 'assistant', ?, 'Inspect Telegram', ?, 'completed', datetime('now', '-5 minutes'), datetime('now', '-5 minutes')),
		 ('run-whatsapp', ?, 'assistant', ?, 'Inspect WhatsApp', ?, 'completed', datetime('now', '-4 minutes'), datetime('now', '-4 minutes'))`,
		frontTelegram.ConversationID,
		frontTelegram.ID,
		t.TempDir(),
		frontWhatsApp.ConversationID,
		frontWhatsApp.ID,
		t.TempDir(),
	); err != nil {
		t.Fatalf("insert runs: %v", err)
	}

	if _, err := svc.db.RawDB().ExecContext(
		ctx,
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at, last_attempt_at)
		 VALUES
		 ('intent-telegram-terminal', 'run-telegram', 'telegram', 'chat-1', 'reply one', 'dedupe-1', 'terminal', 5, datetime('now', '-3 minutes'), datetime('now', '-2 minutes')),
		 ('intent-telegram-retrying', 'run-telegram', 'telegram', 'chat-1', 'reply two', 'dedupe-2', 'retrying', 2, datetime('now', '-2 minutes'), datetime('now', '-1 minutes')),
		 ('intent-whatsapp-pending', 'run-whatsapp', 'whatsapp', 'chat-2', 'reply three', 'dedupe-3', 'pending', 0, datetime('now', '-90 seconds'), NULL)`,
	); err != nil {
		t.Fatalf("insert outbound intents: %v", err)
	}

	items, err := svc.ListDeliveryQueue(ctx, DeliveryQueueFilter{
		ConnectorID: "telegram",
		Status:      "terminal",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("ListDeliveryQueue failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 delivery queue item, got %d", len(items))
	}
	if items[0].ID != "intent-telegram-terminal" || items[0].SessionID != frontTelegram.ID {
		t.Fatalf("unexpected delivery queue identity: %+v", items[0])
	}
	if items[0].ConversationID != frontTelegram.ConversationID || items[0].Status != "terminal" {
		t.Fatalf("unexpected delivery queue status: %+v", items[0])
	}
}

func TestService_ListDeliveryQueueAppliesQueryAndAllStatus(t *testing.T) {
	svc := newTestSessionService(t)
	ctx := context.Background()

	frontTelegram := openFrontSession(t, svc)
	frontWhatsApp, err := svc.OpenFrontSession(ctx, OpenFrontSession{
		ConversationID: "conv-whatsapp",
		AgentID:        "assistant",
		WorkspaceRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("OpenFrontSession whatsapp failed: %v", err)
	}

	if _, err := svc.db.RawDB().ExecContext(
		ctx,
		`INSERT INTO runs
		 (id, conversation_id, agent_id, session_id, team_id, objective, workspace_root, status, created_at, updated_at)
		 VALUES
		 ('run-telegram', ?, 'assistant', ?, 'team-a', 'Inspect Telegram', ?, 'completed', datetime('now'), datetime('now')),
		 ('run-whatsapp', ?, 'assistant', ?, 'team-a', 'Inspect WhatsApp', ?, 'completed', datetime('now'), datetime('now'))`,
		frontTelegram.ConversationID,
		frontTelegram.ID,
		t.TempDir(),
		frontWhatsApp.ConversationID,
		frontWhatsApp.ID,
		t.TempDir(),
	); err != nil {
		t.Fatalf("insert runs: %v", err)
	}

	if _, err := svc.db.RawDB().ExecContext(
		ctx,
		`INSERT INTO outbound_intents
		 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at)
		 VALUES
		 ('intent-telegram-terminal', 'run-telegram', 'telegram', 'chat-alpha', 'reply alpha', 'dedupe-1', 'terminal', 5, datetime('now', '-3 minutes')),
		 ('intent-whatsapp-pending', 'run-whatsapp', 'whatsapp', 'chat-beta', 'reply beta', 'dedupe-2', 'pending', 0, datetime('now', '-90 seconds'))`,
	); err != nil {
		t.Fatalf("insert outbound intents: %v", err)
	}

	items, err := svc.ListDeliveryQueue(ctx, DeliveryQueueFilter{
		Status: "all",
		Query:  "chat-alpha",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("ListDeliveryQueue failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 delivery queue item, got %d", len(items))
	}
	if items[0].ID != "intent-telegram-terminal" || items[0].Status != "terminal" {
		t.Fatalf("unexpected delivery queue item: %+v", items[0])
	}
}

func TestService_ListRoutes(t *testing.T) {
	svc := newTestSessionService(t)
	ctx := context.Background()

	frontTelegram := openFrontSession(t, svc)
	frontWhatsApp, err := svc.OpenFrontSession(ctx, OpenFrontSession{
		ConversationID: "conv-whatsapp",
		AgentID:        "assistant",
		WorkspaceRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("OpenFrontSession whatsapp failed: %v", err)
	}

	if err := svc.BindFollowUp(ctx, BindFollowUp{
		ConversationID: frontTelegram.ConversationID,
		ThreadID:       "thread-1",
		SessionID:      frontTelegram.ID,
		ConnectorID:    "telegram",
		AccountID:      "acct-1",
		ExternalID:     "chat-1",
	}); err != nil {
		t.Fatalf("BindFollowUp telegram failed: %v", err)
	}
	if err := svc.BindFollowUp(ctx, BindFollowUp{
		ConversationID: frontWhatsApp.ConversationID,
		ThreadID:       "thread-2",
		SessionID:      frontWhatsApp.ID,
		ConnectorID:    "whatsapp",
		AccountID:      "acct-2",
		ExternalID:     "chat-2",
	}); err != nil {
		t.Fatalf("BindFollowUp whatsapp failed: %v", err)
	}

	routes, err := svc.ListRoutes(ctx, RouteListFilter{
		ConnectorID: "telegram",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("ListRoutes failed: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0].SessionID != frontTelegram.ID || routes[0].ConversationID != frontTelegram.ConversationID {
		t.Fatalf("unexpected route identity: %+v", routes[0])
	}
	if routes[0].ConnectorID != "telegram" || routes[0].ThreadID != "thread-1" {
		t.Fatalf("unexpected route target: %+v", routes[0])
	}
}

func TestService_ListRoutesAppliesQueryFilter(t *testing.T) {
	svc := newTestSessionService(t)
	ctx := context.Background()

	frontTelegram := openFrontSession(t, svc)
	frontWhatsApp, err := svc.OpenFrontSession(ctx, OpenFrontSession{
		ConversationID: "conv-whatsapp",
		AgentID:        "assistant",
		WorkspaceRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("OpenFrontSession whatsapp failed: %v", err)
	}

	if err := svc.BindFollowUp(ctx, BindFollowUp{
		ConversationID: frontTelegram.ConversationID,
		ThreadID:       "thread-alpha",
		SessionID:      frontTelegram.ID,
		ConnectorID:    "telegram",
		AccountID:      "acct-1",
		ExternalID:     "chat-alpha",
	}); err != nil {
		t.Fatalf("BindFollowUp telegram failed: %v", err)
	}
	if err := svc.BindFollowUp(ctx, BindFollowUp{
		ConversationID: frontWhatsApp.ConversationID,
		ThreadID:       "thread-beta",
		SessionID:      frontWhatsApp.ID,
		ConnectorID:    "whatsapp",
		AccountID:      "acct-2",
		ExternalID:     "chat-beta",
	}); err != nil {
		t.Fatalf("BindFollowUp whatsapp failed: %v", err)
	}

	routes, err := svc.ListRoutes(ctx, RouteListFilter{
		Status: "all",
		Query:  "chat-alpha",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("ListRoutes failed: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0].SessionID != frontTelegram.ID || routes[0].ConnectorID != "telegram" {
		t.Fatalf("unexpected route payload: %+v", routes[0])
	}
}

func TestService_ListRoutesIncludesInactiveHistory(t *testing.T) {
	svc := newTestSessionService(t)
	ctx := context.Background()

	front := openFrontSession(t, svc)
	if err := svc.BindFollowUp(ctx, BindFollowUp{
		ConversationID: front.ConversationID,
		ThreadID:       "thread-1",
		SessionID:      front.ID,
		ConnectorID:    "telegram",
		AccountID:      "acct-1",
		ExternalID:     "chat-1",
	}); err != nil {
		t.Fatalf("BindFollowUp failed: %v", err)
	}

	activeRoutes, err := svc.ListRoutes(ctx, RouteListFilter{
		ConnectorID: "telegram",
		Status:      "active",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("ListRoutes active failed: %v", err)
	}
	if len(activeRoutes) != 1 {
		t.Fatalf("expected 1 active route, got %d", len(activeRoutes))
	}

	if err := svc.conv.AppendEvent(ctx, model.Event{
		ID:             "evt-route-unbound",
		ConversationID: front.ConversationID,
		Kind:           "session_unbound",
		PayloadJSON:    []byte(`{"route_id":"` + activeRoutes[0].ID + `"}`),
	}); err != nil {
		t.Fatalf("AppendEvent session_unbound failed: %v", err)
	}

	allRoutes, err := svc.ListRoutes(ctx, RouteListFilter{
		ConnectorID: "telegram",
		Status:      "all",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("ListRoutes all failed: %v", err)
	}
	if len(allRoutes) != 1 {
		t.Fatalf("expected 1 historical route, got %d", len(allRoutes))
	}
	if allRoutes[0].Status != "inactive" || allRoutes[0].DeactivatedAt == nil || allRoutes[0].DeactivationReason != "deactivated" {
		t.Fatalf("expected inactive historical route with deactivation metadata, got %+v", allRoutes[0])
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

	list, err := svc.ListSessions(ctx, SessionListFilter{Limit: 10})
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

func TestService_ListSessionsAppliesDirectoryFilters(t *testing.T) {
	svc := newTestSessionService(t)
	ctx := context.Background()

	frontTelegram, err := svc.OpenFrontSession(ctx, OpenFrontSession{
		ConversationID: "conv-telegram",
		AgentID:        "assistant",
		WorkspaceRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("OpenFrontSession telegram failed: %v", err)
	}
	worker, err := svc.SpawnWorkerSession(ctx, SpawnWorkerSession{
		ConversationID:      frontTelegram.ConversationID,
		ParentSessionID:     frontTelegram.ID,
		ControllerSessionID: frontTelegram.ID,
		AgentID:             "researcher",
		InitialPrompt:       "Inspect docs.",
	})
	if err != nil {
		t.Fatalf("SpawnWorkerSession failed: %v", err)
	}
	frontArchive, err := svc.OpenFrontSession(ctx, OpenFrontSession{
		ConversationID: "conv-archive",
		AgentID:        "archivist",
		WorkspaceRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("OpenFrontSession archive failed: %v", err)
	}

	if err := svc.BindFollowUp(ctx, BindFollowUp{
		ConversationID: frontTelegram.ConversationID,
		ThreadID:       "thread-1",
		SessionID:      frontTelegram.ID,
		ConnectorID:    "telegram",
		AccountID:      "acct-1",
		ExternalID:     "chat-alpha",
	}); err != nil {
		t.Fatalf("BindFollowUp failed: %v", err)
	}

	if _, err := svc.db.RawDB().ExecContext(ctx, `UPDATE sessions SET status = 'archived' WHERE id = ?`, frontArchive.ID); err != nil {
		t.Fatalf("update archived session status: %v", err)
	}

	list, err := svc.ListSessions(ctx, SessionListFilter{
		ConnectorID: "telegram",
		BoundOnly:   true,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("ListSessions telegram bound failed: %v", err)
	}
	if len(list) != 1 || list[0].ID != frontTelegram.ID {
		t.Fatalf("expected only telegram-bound front session, got %+v", list)
	}

	list, err = svc.ListSessions(ctx, SessionListFilter{
		Role:  string(model.SessionRoleWorker),
		Query: "researcher",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListSessions worker failed: %v", err)
	}
	if len(list) != 1 || list[0].ID != worker.ID {
		t.Fatalf("expected only worker session, got %+v", list)
	}

	list, err = svc.ListSessions(ctx, SessionListFilter{
		Status: "archived",
		Query:  "conv-archive",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("ListSessions archived failed: %v", err)
	}
	if len(list) != 1 || list[0].ID != frontArchive.ID {
		t.Fatalf("expected only archived session, got %+v", list)
	}
}
