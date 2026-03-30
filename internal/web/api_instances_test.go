package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestInstancesStatusReturnsInventorySnapshot(t *testing.T) {
	t.Parallel()

	h := newServerHarnessWithConnectorHealth(t, []model.ConnectorHealthSnapshot{
		{
			ConnectorID: "telegram",
			State:       model.ConnectorHealthHealthy,
			Summary:     "Presence beacon healthy",
		},
	})

	run, _, _ := h.seedRoutesDeliveriesData(t)
	if _, err := h.db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, project_id, team_id, parent_run_id, objective, cwd, status, created_at, updated_at)
		 VALUES ('run-worker', ?, 'researcher', ?, 'repo-task-team', ?, 'Inspect test failures', ?, 'needs_approval', datetime('now'), datetime('now'))`,
		run.ConversationID, h.activeProjectID, run.ID, h.workspaceRoot,
	); err != nil {
		t.Fatalf("insert worker run: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Summary struct {
			FrontLaneCount       int `json:"front_lane_count"`
			SpecialistLaneCount  int `json:"specialist_lane_count"`
			LiveConnectorCount   int `json:"live_connector_count"`
			PendingDeliveryCount int `json:"pending_delivery_count"`
		} `json:"summary"`
		Lanes []struct {
			ID      string `json:"id"`
			Kind    string `json:"kind"`
			AgentID string `json:"agent_id"`
			Status  string `json:"status"`
		} `json:"lanes"`
		Connectors []struct {
			ConnectorID  string `json:"connector_id"`
			State        string `json:"state"`
			PendingCount int    `json:"pending_count"`
		} `json:"connectors"`
		Sources struct {
			QueueHeadline  string `json:"queue_headline"`
			RootRuns       int    `json:"root_runs"`
			SessionCount   int    `json:"session_count"`
			ConnectorCount int    `json:"connector_count"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode instances response: %v", err)
	}

	if resp.Summary.FrontLaneCount != 1 || resp.Summary.SpecialistLaneCount != 1 {
		t.Fatalf("unexpected lane summary: %+v", resp.Summary)
	}
	if resp.Summary.LiveConnectorCount != 1 || resp.Summary.PendingDeliveryCount != 1 {
		t.Fatalf("unexpected connector summary: %+v", resp.Summary)
	}
	if len(resp.Lanes) != 2 || resp.Lanes[0].Kind != "front" || resp.Lanes[1].Kind != "specialist" {
		t.Fatalf("unexpected lanes: %+v", resp.Lanes)
	}
	if resp.Lanes[0].AgentID == "" || resp.Lanes[1].AgentID != "researcher" {
		t.Fatalf("unexpected lane agents: %+v", resp.Lanes)
	}
	if len(resp.Connectors) != 1 || resp.Connectors[0].ConnectorID != "telegram" || resp.Connectors[0].PendingCount != 1 {
		t.Fatalf("unexpected connectors: %+v", resp.Connectors)
	}
	if resp.Connectors[0].State != "healthy" {
		t.Fatalf("unexpected connector state: %+v", resp.Connectors[0])
	}
	if resp.Sources.RootRuns != 1 || resp.Sources.SessionCount != 1 || resp.Sources.ConnectorCount != 1 {
		t.Fatalf("unexpected sources: %+v", resp.Sources)
	}
	if resp.Sources.QueueHeadline == "" {
		t.Fatalf("expected queue headline, got %+v", resp.Sources)
	}
}
