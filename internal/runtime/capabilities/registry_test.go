package capabilities

import (
	"context"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

type stubConnector struct {
	id              string
	aliases         []string
	presenceCalls   []PresenceEmitRequest
	inboxCalls      []InboxListRequest
	inboxUpdates    []InboxUpdateRequest
	directoryCalls  []DirectoryListRequest
	resolveCalls    []TargetResolveRequest
	sendCalls       []SendRequest
	statusCalls     []StatusRequest
	presencePolicy  PresencePolicy
	inboxResult     InboxListResult
	inboxUpdate     InboxUpdateResult
	directoryResult DirectoryListResult
	resolveResult   TargetResolveResult
	sendResult      SendResult
	statusResult    ConnectorStatus
	snapshot        model.ConnectorHealthSnapshot
	presenceErr     error
	inboxErr        error
	inboxUpdateErr  error
	directoryErr    error
	resolveErr      error
	sendErr         error
	statusErr       error
}

func (c *stubConnector) Metadata() model.ConnectorMetadata {
	return model.NormalizeConnectorMetadata(model.ConnectorMetadata{ID: c.id, Aliases: append([]string(nil), c.aliases...)})
}

func (c *stubConnector) Start(context.Context) error { return nil }

func (c *stubConnector) Notify(context.Context, string, model.ReplayDelta, string) error { return nil }

func (c *stubConnector) Drain(context.Context) error { return nil }

func (c *stubConnector) CapabilityListDirectory(_ context.Context, req DirectoryListRequest) (DirectoryListResult, error) {
	c.directoryCalls = append(c.directoryCalls, req)
	return c.directoryResult, c.directoryErr
}

func (c *stubConnector) CapabilityPresencePolicy(context.Context) PresencePolicy {
	return c.presencePolicy
}

func (c *stubConnector) CapabilityEmitPresence(_ context.Context, req PresenceEmitRequest) error {
	c.presenceCalls = append(c.presenceCalls, req)
	return c.presenceErr
}

func (c *stubConnector) CapabilityListInbox(_ context.Context, req InboxListRequest) (InboxListResult, error) {
	c.inboxCalls = append(c.inboxCalls, req)
	return c.inboxResult, c.inboxErr
}

func (c *stubConnector) CapabilityUpdateInbox(_ context.Context, req InboxUpdateRequest) (InboxUpdateResult, error) {
	c.inboxUpdates = append(c.inboxUpdates, req)
	return c.inboxUpdate, c.inboxUpdateErr
}

func (c *stubConnector) CapabilityResolveTarget(_ context.Context, req TargetResolveRequest) (TargetResolveResult, error) {
	c.resolveCalls = append(c.resolveCalls, req)
	return c.resolveResult, c.resolveErr
}

func (c *stubConnector) CapabilitySend(_ context.Context, req SendRequest) (SendResult, error) {
	c.sendCalls = append(c.sendCalls, req)
	return c.sendResult, c.sendErr
}

func (c *stubConnector) CapabilityStatus(_ context.Context, req StatusRequest) (ConnectorStatus, error) {
	c.statusCalls = append(c.statusCalls, req)
	return c.statusResult, c.statusErr
}

func (c *stubConnector) ConnectorHealthSnapshot() model.ConnectorHealthSnapshot {
	return c.snapshot
}

type snapshotOnlyConnector struct {
	id       string
	snapshot model.ConnectorHealthSnapshot
}

func (c *snapshotOnlyConnector) Metadata() model.ConnectorMetadata {
	return model.NormalizeConnectorMetadata(model.ConnectorMetadata{ID: c.id})
}

func (c *snapshotOnlyConnector) Start(context.Context) error { return nil }

func (c *snapshotOnlyConnector) Notify(context.Context, string, model.ReplayDelta, string) error {
	return nil
}

func (c *snapshotOnlyConnector) Drain(context.Context) error { return nil }

func (c *snapshotOnlyConnector) ConnectorHealthSnapshot() model.ConnectorHealthSnapshot {
	return c.snapshot
}

type stubAppActions struct {
	calls  []AppActionRequest
	result AppActionResult
	err    error
}

func (s *stubAppActions) CapabilityAppAction(_ context.Context, req AppActionRequest) (AppActionResult, error) {
	s.calls = append(s.calls, req)
	return s.result, s.err
}

func TestCapabilityRegistry_InvokesRegisteredConnectorAdapters(t *testing.T) {
	reg := NewRegistry()
	connector := &stubConnector{
		id: "zalo_personal",
		presencePolicy: PresencePolicy{
			StartupDelay:           800 * time.Millisecond,
			KeepaliveInterval:      8 * time.Second,
			MaxDuration:            60 * time.Second,
			MaxConsecutiveFailures: 2,
			SupportsStop:           false,
		},
		inboxResult: InboxListResult{
			ConnectorID: "zalo_personal",
			Entries: []InboxEntry{
				{ThreadID: "user-1", ThreadType: "contact", Title: "Anh An", IsUnread: true, UnreadCount: 1},
			},
		},
		inboxUpdate: InboxUpdateResult{
			ConnectorID: "zalo_personal",
			ThreadID:    "user-1",
			ThreadType:  "contact",
			Action:      "mark_read",
			Applied:     true,
			Summary:     "conversation updated",
		},
		directoryResult: DirectoryListResult{
			ConnectorID: "zalo_personal",
			Scope:       "contacts",
			Entries: []DirectoryEntry{
				{ID: "user-1", Title: "Anh An", Kind: "contact"},
			},
		},
		resolveResult: TargetResolveResult{
			ConnectorID: "zalo_personal",
			Query:       "Anh An",
			Matches: []TargetMatch{
				{ID: "user-1", Title: "Anh An", Kind: "contact", Score: 1},
			},
		},
		sendResult: SendResult{
			ConnectorID: "zalo_personal",
			TargetID:    "user-1",
			TargetType:  "contact",
			Accepted:    true,
			Summary:     "message accepted",
		},
		statusResult: ConnectorStatus{
			ConnectorID: "zalo_personal",
			State:       model.ConnectorHealthHealthy,
			Summary:     "connected",
			CheckedAt:   time.Unix(1700000000, 0).UTC(),
		},
	}
	reg.RegisterConnector(connector)

	presencePolicy, err := reg.PresencePolicy(context.Background(), "zalo_personal")
	if err != nil {
		t.Fatalf("PresencePolicy: %v", err)
	}
	if presencePolicy.StartupDelay != 800*time.Millisecond || presencePolicy.KeepaliveInterval != 8*time.Second {
		t.Fatalf("unexpected presence policy: %+v", presencePolicy)
	}

	if err := reg.EmitPresence(context.Background(), PresenceEmitRequest{
		ConnectorID:    "zalo_personal",
		ConversationID: "conv-1",
		ThreadID:       "user-1",
		ThreadType:     "contact",
		Mode:           PresenceModeTyping,
	}); err != nil {
		t.Fatalf("EmitPresence: %v", err)
	}
	if len(connector.presenceCalls) != 1 || connector.presenceCalls[0].Mode != PresenceModeTyping {
		t.Fatalf("unexpected presence calls: %+v", connector.presenceCalls)
	}

	inboxResult, err := reg.InboxList(context.Background(), InboxListRequest{
		ConnectorID: "zalo_personal",
		UnreadOnly:  true,
		Limit:       5,
	})
	if err != nil {
		t.Fatalf("InboxList: %v", err)
	}
	if len(inboxResult.Entries) != 1 || inboxResult.Entries[0].ThreadID != "user-1" {
		t.Fatalf("unexpected inbox result: %+v", inboxResult)
	}
	if len(connector.inboxCalls) != 1 || !connector.inboxCalls[0].UnreadOnly {
		t.Fatalf("unexpected inbox calls: %+v", connector.inboxCalls)
	}

	updateResult, err := reg.InboxUpdate(context.Background(), InboxUpdateRequest{
		ConnectorID: "zalo_personal",
		ThreadID:    "user-1",
		ThreadType:  "contact",
		Action:      "mark_read",
	})
	if err != nil {
		t.Fatalf("InboxUpdate: %v", err)
	}
	if !updateResult.Applied || updateResult.Action != "mark_read" {
		t.Fatalf("unexpected inbox update result: %+v", updateResult)
	}
	if len(connector.inboxUpdates) != 1 || connector.inboxUpdates[0].Action != "mark_read" {
		t.Fatalf("unexpected inbox update calls: %+v", connector.inboxUpdates)
	}

	dirResult, err := reg.DirectoryList(context.Background(), DirectoryListRequest{
		ConnectorID: "zalo_personal",
		Scope:       "contacts",
		Query:       "Anh",
		Limit:       5,
	})
	if err != nil {
		t.Fatalf("DirectoryList: %v", err)
	}
	if len(dirResult.Entries) != 1 || dirResult.Entries[0].ID != "user-1" {
		t.Fatalf("unexpected directory result: %+v", dirResult)
	}
	if len(connector.directoryCalls) != 1 || connector.directoryCalls[0].Query != "Anh" {
		t.Fatalf("unexpected directory calls: %+v", connector.directoryCalls)
	}

	resolveResult, err := reg.ResolveTarget(context.Background(), TargetResolveRequest{
		ConnectorID: "zalo_personal",
		Query:       "Anh An",
	})
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if len(resolveResult.Matches) != 1 || resolveResult.Matches[0].Score != 1 {
		t.Fatalf("unexpected resolve result: %+v", resolveResult)
	}
	if len(connector.resolveCalls) != 1 || connector.resolveCalls[0].Query != "Anh An" {
		t.Fatalf("unexpected resolve calls: %+v", connector.resolveCalls)
	}

	sendResult, err := reg.Send(context.Background(), SendRequest{
		ConnectorID: "zalo_personal",
		TargetID:    "user-1",
		TargetType:  "contact",
		Message:     "hello",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !sendResult.Accepted || sendResult.TargetID != "user-1" {
		t.Fatalf("unexpected send result: %+v", sendResult)
	}
	if len(connector.sendCalls) != 1 || connector.sendCalls[0].Message != "hello" {
		t.Fatalf("unexpected send calls: %+v", connector.sendCalls)
	}

	statusResult, err := reg.Status(context.Background(), StatusRequest{ConnectorID: "zalo_personal"})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(statusResult.Connectors) != 1 || statusResult.Connectors[0].Summary != "connected" {
		t.Fatalf("unexpected status result: %+v", statusResult)
	}
	if len(connector.statusCalls) != 1 || connector.statusCalls[0].ConnectorID != "zalo_personal" {
		t.Fatalf("unexpected status calls: %+v", connector.statusCalls)
	}
}

func TestCapabilityRegistry_ResolvesConnectorAliases(t *testing.T) {
	reg := NewRegistry()
	connector := &stubConnector{
		id:      "zalo_personal",
		aliases: []string{"zalo", "zalo personal"},
		inboxResult: InboxListResult{
			ConnectorID: "zalo_personal",
			Entries: []InboxEntry{
				{ThreadID: "user-1", ThreadType: "contact", Title: "Anh An", IsUnread: true, UnreadCount: 1},
			},
		},
		inboxUpdate: InboxUpdateResult{
			ConnectorID: "zalo_personal",
			ThreadID:    "user-1",
			ThreadType:  "contact",
			Action:      "pin",
			Applied:     true,
			Summary:     "conversation updated",
		},
		directoryResult: DirectoryListResult{
			ConnectorID: "zalo_personal",
			Scope:       "contacts",
			Entries: []DirectoryEntry{
				{ID: "user-1", Title: "Anh An", Kind: "contact"},
			},
		},
		statusResult: ConnectorStatus{
			ConnectorID: "zalo_personal",
			State:       model.ConnectorHealthHealthy,
			Summary:     "connected",
		},
		sendResult: SendResult{
			ConnectorID: "zalo_personal",
			TargetID:    "user-1",
			Accepted:    true,
			Summary:     "message accepted",
		},
	}
	reg.RegisterConnector(connector)

	if _, err := reg.InboxList(context.Background(), InboxListRequest{ConnectorID: "zalo"}); err != nil {
		t.Fatalf("InboxList alias lookup: %v", err)
	}
	if len(connector.inboxCalls) != 1 || connector.inboxCalls[0].ConnectorID != "zalo_personal" {
		t.Fatalf("expected inbox alias to resolve to canonical connector id, got %+v", connector.inboxCalls)
	}

	if _, err := reg.InboxUpdate(context.Background(), InboxUpdateRequest{
		ConnectorID: "zalo",
		ThreadID:    "user-1",
		Action:      "pin",
	}); err != nil {
		t.Fatalf("InboxUpdate alias lookup: %v", err)
	}
	if len(connector.inboxUpdates) != 1 || connector.inboxUpdates[0].ConnectorID != "zalo_personal" {
		t.Fatalf("expected inbox update alias to resolve to canonical connector id, got %+v", connector.inboxUpdates)
	}

	if _, err := reg.DirectoryList(context.Background(), DirectoryListRequest{ConnectorID: "zalo"}); err != nil {
		t.Fatalf("DirectoryList alias lookup: %v", err)
	}
	if len(connector.directoryCalls) != 1 || connector.directoryCalls[0].ConnectorID != "zalo_personal" {
		t.Fatalf("expected alias to resolve to canonical connector id, got %+v", connector.directoryCalls)
	}

	statusResult, err := reg.Status(context.Background(), StatusRequest{ConnectorID: "zalo"})
	if err != nil {
		t.Fatalf("Status alias lookup: %v", err)
	}
	if len(statusResult.Connectors) != 1 || statusResult.Connectors[0].ConnectorID != "zalo_personal" {
		t.Fatalf("unexpected alias status result: %+v", statusResult)
	}

	if _, err := reg.Send(context.Background(), SendRequest{
		ConnectorID: "zalo",
		TargetID:    "user-1",
		Message:     "hello",
	}); err != nil {
		t.Fatalf("Send alias lookup: %v", err)
	}
	if len(connector.sendCalls) != 1 || connector.sendCalls[0].ConnectorID != "zalo_personal" {
		t.Fatalf("expected send alias to resolve to canonical connector id, got %+v", connector.sendCalls)
	}
}

func TestCapabilityRegistry_UsesHealthSnapshotFallbackForStatus(t *testing.T) {
	reg := NewRegistry()
	reg.RegisterConnector(&snapshotOnlyConnector{
		id: "telegram",
		snapshot: model.ConnectorHealthSnapshot{
			ConnectorID:      "telegram",
			State:            model.ConnectorHealthDegraded,
			Summary:          "poll loop stale",
			CheckedAt:        time.Unix(1700000100, 0).UTC(),
			RestartSuggested: true,
		},
	})
	reg.RegisterConnector(&snapshotOnlyConnector{
		id: "whatsapp",
		snapshot: model.ConnectorHealthSnapshot{
			ConnectorID: "whatsapp",
			State:       model.ConnectorHealthHealthy,
			Summary:     "webhook healthy",
			CheckedAt:   time.Unix(1700000200, 0).UTC(),
		},
	})

	result, err := reg.Status(context.Background(), StatusRequest{})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(result.Connectors) != 2 {
		t.Fatalf("expected 2 connector statuses, got %+v", result)
	}
	if result.Connectors[0].ConnectorID != "telegram" || result.Connectors[1].ConnectorID != "whatsapp" {
		t.Fatalf("expected sorted connector statuses, got %+v", result.Connectors)
	}
}

func TestCapabilityRegistry_AppAction(t *testing.T) {
	reg := NewRegistry()
	actions := &stubAppActions{
		result: AppActionResult{
			Name:    "status",
			Summary: "status loaded",
			Data: map[string]any{
				"active_runs": 1,
			},
		},
	}
	reg.RegisterAppAction("status", actions)

	result, err := reg.AppAction(context.Background(), AppActionRequest{
		Name: "status",
	})
	if err != nil {
		t.Fatalf("AppAction: %v", err)
	}
	if result.Name != "status" || result.Summary != "status loaded" {
		t.Fatalf("unexpected app action result: %+v", result)
	}
	if len(actions.calls) != 1 || actions.calls[0].Name != "status" {
		t.Fatalf("unexpected app action calls: %+v", actions.calls)
	}
}

func TestCapabilityRegistry_RejectsUnsupportedConnectorOperation(t *testing.T) {
	reg := NewRegistry()
	reg.RegisterConnector(&snapshotOnlyConnector{
		id: "telegram",
		snapshot: model.ConnectorHealthSnapshot{
			ConnectorID: "telegram",
			State:       model.ConnectorHealthHealthy,
			Summary:     "healthy",
		},
	})

	_, err := reg.DirectoryList(context.Background(), DirectoryListRequest{ConnectorID: "telegram"})
	if err == nil || err.Error() != `capabilities: connector "telegram" does not support directory listing` {
		t.Fatalf("expected unsupported operation error, got %v", err)
	}
}
