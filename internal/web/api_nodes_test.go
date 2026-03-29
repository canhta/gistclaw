package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canhta/gistclaw/internal/nodeinventory"
)

type stubNodeInventorySource struct {
	status nodeinventory.Status
	err    error
}

func (s stubNodeInventorySource) NodeInventoryStatus(context.Context) (nodeinventory.Status, error) {
	if s.err != nil {
		return nodeinventory.Status{}, s.err
	}
	return s.status, nil
}

func TestNodesStatusReturnsInventorySnapshot(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	h.rawServer.nodes = stubNodeInventorySource{
		status: nodeinventory.Status{
			Summary: nodeinventory.Summary{
				Connectors:        2,
				HealthyConnectors: 1,
				RunNodes:          2,
				ApprovalNodes:     1,
				Capabilities:      3,
			},
			Connectors: []nodeinventory.ConnectorStatus{
				{
					ID:             "telegram",
					Aliases:        []string{"tg"},
					Exposure:       "remote",
					State:          "healthy",
					StateLabel:     "healthy",
					Summary:        "polling",
					CheckedAtLabel: "2026-03-29 09:30:00 UTC",
				},
				{
					ID:               "whatsapp",
					Exposure:         "remote",
					State:            "degraded",
					StateLabel:       "degraded",
					Summary:          "token expired",
					RestartSuggested: true,
					CheckedAtLabel:   "2026-03-29 09:31:00 UTC",
				},
			},
			Runs: []nodeinventory.RunNode{
				{
					ID:               "run-root",
					ShortID:          "run-root",
					ParentRunID:      "",
					Kind:             "root",
					AgentID:          "assistant",
					Status:           "active",
					StatusLabel:      "active",
					ObjectivePreview: "Review the repo layout",
					StartedAtLabel:   "2026-03-29 09:30:00 UTC",
					UpdatedAtLabel:   "2026-03-29 09:31:00 UTC",
				},
			},
			Capabilities: []nodeinventory.Capability{
				{
					Name:        "connector_send",
					Family:      "connector",
					Description: "Send a direct message through a configured connector.",
				},
				{
					Name:        "app_action",
					Family:      "app",
					Description: "Execute a runtime app action.",
				},
			},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Summary struct {
			Connectors        int `json:"connectors"`
			HealthyConnectors int `json:"healthy_connectors"`
			RunNodes          int `json:"run_nodes"`
			Capabilities      int `json:"capabilities"`
		} `json:"summary"`
		Connectors []struct {
			ID               string `json:"id"`
			State            string `json:"state"`
			RestartSuggested bool   `json:"restart_suggested"`
		} `json:"connectors"`
		Runs []struct {
			ID          string `json:"id"`
			Kind        string `json:"kind"`
			StatusLabel string `json:"status_label"`
		} `json:"runs"`
		Capabilities []struct {
			Name   string `json:"name"`
			Family string `json:"family"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode nodes response: %v", err)
	}

	if resp.Summary.Connectors != 2 || resp.Summary.HealthyConnectors != 1 || resp.Summary.RunNodes != 2 || resp.Summary.Capabilities != 3 {
		t.Fatalf("unexpected summary: %+v", resp.Summary)
	}
	if len(resp.Connectors) != 2 || resp.Connectors[1].ID != "whatsapp" || !resp.Connectors[1].RestartSuggested {
		t.Fatalf("unexpected connectors: %+v", resp.Connectors)
	}
	if len(resp.Runs) != 1 || resp.Runs[0].Kind != "root" || resp.Runs[0].StatusLabel != "active" {
		t.Fatalf("unexpected runs: %+v", resp.Runs)
	}
	if len(resp.Capabilities) != 2 || resp.Capabilities[0].Name != "connector_send" {
		t.Fatalf("unexpected capabilities: %+v", resp.Capabilities)
	}
}
