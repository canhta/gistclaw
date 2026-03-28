package capabilities

import (
	"context"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

type stubConnector struct {
	id              string
	directoryCalls  []DirectoryListRequest
	resolveCalls    []TargetResolveRequest
	sendCalls       []SendRequest
	statusCalls     []StatusRequest
	directoryResult DirectoryListResult
	resolveResult   TargetResolveResult
	sendResult      SendResult
	statusResult    ConnectorStatus
	snapshot        model.ConnectorHealthSnapshot
	directoryErr    error
	resolveErr      error
	sendErr         error
	statusErr       error
}

func (c *stubConnector) Metadata() model.ConnectorMetadata {
	return model.NormalizeConnectorMetadata(model.ConnectorMetadata{ID: c.id})
}

func (c *stubConnector) Start(context.Context) error { return nil }

func (c *stubConnector) Notify(context.Context, string, model.ReplayDelta, string) error { return nil }

func (c *stubConnector) Drain(context.Context) error { return nil }

func (c *stubConnector) CapabilityListDirectory(_ context.Context, req DirectoryListRequest) (DirectoryListResult, error) {
	c.directoryCalls = append(c.directoryCalls, req)
	return c.directoryResult, c.directoryErr
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
