package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestConversationsAPIListsSessionsAndSessionDetail(t *testing.T) {
	t.Parallel()

	h := newServerHarnessWithConnectorHealth(t, []model.ConnectorHealthSnapshot{
		{
			ConnectorID: "telegram",
			State:       model.ConnectorHealthHealthy,
			Summary:     "webhook activity recent",
		},
	})
	front, route, deliveryID := h.seedRoutesDeliveriesData(t)
	worker := h.spawnWorkerSession(t, front.SessionID, "researcher", "Inspect docs.")
	h.markOutboundIntentTerminal(t, deliveryID)
	h.insertRunWithSession(t, "run-conversation-active", front.ConversationID, front.SessionID, "follow up", "active")

	indexRR := httptest.NewRecorder()
	indexReq := httptest.NewRequest(http.MethodGet, "/api/conversations", nil)
	h.server.ServeHTTP(indexRR, indexReq)

	if indexRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", indexRR.Code, indexRR.Body.String())
	}

	var indexResp struct {
		Summary struct {
			SessionCount       int `json:"session_count"`
			ConnectorCount     int `json:"connector_count"`
			TerminalDeliveries int `json:"terminal_deliveries"`
		} `json:"summary"`
		Sessions []struct {
			ID             string `json:"id"`
			ConversationID string `json:"conversation_id"`
			AgentID        string `json:"agent_id"`
		} `json:"sessions"`
		Health []struct {
			ConnectorID   string `json:"connector_id"`
			TerminalCount int    `json:"terminal_count"`
		} `json:"health"`
		RuntimeConnectors []struct {
			ConnectorID string `json:"connector_id"`
			Summary     string `json:"summary"`
		} `json:"runtime_connectors"`
	}
	if err := json.Unmarshal(indexRR.Body.Bytes(), &indexResp); err != nil {
		t.Fatalf("decode index response: %v", err)
	}

	if indexResp.Summary.SessionCount != 2 || indexResp.Summary.ConnectorCount != 1 || indexResp.Summary.TerminalDeliveries != 1 {
		t.Fatalf("unexpected conversations summary %+v", indexResp.Summary)
	}
	if len(indexResp.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(indexResp.Sessions))
	}
	if indexResp.Sessions[0].ID != worker.SessionID || indexResp.Sessions[1].ID != front.SessionID {
		t.Fatalf("unexpected sessions order %+v", indexResp.Sessions)
	}
	if len(indexResp.Health) != 1 || indexResp.Health[0].ConnectorID != "telegram" {
		t.Fatalf("unexpected health %+v", indexResp.Health)
	}
	if len(indexResp.RuntimeConnectors) != 1 || indexResp.RuntimeConnectors[0].Summary != "webhook activity recent" {
		t.Fatalf("unexpected runtime connectors %+v", indexResp.RuntimeConnectors)
	}

	detailRR := httptest.NewRecorder()
	detailReq := httptest.NewRequest(http.MethodGet, "/api/conversations/"+front.SessionID, nil)
	h.server.ServeHTTP(detailRR, detailReq)

	if detailRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", detailRR.Code, detailRR.Body.String())
	}

	var detailResp struct {
		Session struct {
			ID          string `json:"id"`
			AgentID     string `json:"agent_id"`
			StatusLabel string `json:"status_label"`
		} `json:"session"`
		Route *struct {
			ID          string `json:"id"`
			ConnectorID string `json:"connector_id"`
		} `json:"route"`
		Messages []struct {
			KindLabel string `json:"kind_label"`
		} `json:"messages"`
		Deliveries []struct {
			ID          string `json:"id"`
			ConnectorID string `json:"connector_id"`
			Status      string `json:"status"`
		} `json:"deliveries"`
		ActiveRunID string `json:"active_run_id"`
	}
	if err := json.Unmarshal(detailRR.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}

	if detailResp.Session.ID != front.SessionID || detailResp.Session.AgentID != "assistant" {
		t.Fatalf("unexpected detail session %+v", detailResp.Session)
	}
	if detailResp.Route == nil || detailResp.Route.ID != route.ID || detailResp.Route.ConnectorID != "telegram" {
		t.Fatalf("unexpected route %+v", detailResp.Route)
	}
	if len(detailResp.Messages) == 0 {
		t.Fatal("expected session messages")
	}
	if len(detailResp.Deliveries) != 1 || detailResp.Deliveries[0].ID != deliveryID || detailResp.Deliveries[0].Status != "terminal" {
		t.Fatalf("unexpected deliveries %+v", detailResp.Deliveries)
	}
	if detailResp.ActiveRunID != "run-conversation-active" {
		t.Fatalf("active_run_id = %q, want %q", detailResp.ActiveRunID, "run-conversation-active")
	}
}
