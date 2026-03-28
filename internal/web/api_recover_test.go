package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

func TestRecoverAPIListsApprovalsRoutesAndDeliveries(t *testing.T) {
	t.Parallel()

	h := newServerHarnessWithConnectorHealth(t, []model.ConnectorHealthSnapshot{
		{
			ConnectorID: "telegram",
			State:       model.ConnectorHealthDegraded,
			Summary:     "poll loop stale",
		},
	})
	run, route, deliveryID := h.seedRoutesDeliveriesData(t)
	h.markOutboundIntentTerminal(t, deliveryID)
	approvalID := h.insertApprovalAt(t, run.ID, "bash", h.workspaceRoot+"/recover.txt", "pending", "", "2026-03-25 10:00:00")
	h.insertApprovalAt(t, "run-recover-approved", "git", h.workspaceRoot+"/approved.txt", "approved", "operator", "2026-03-25 09:00:00")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/recover", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Summary struct {
			OpenApprovals      int `json:"open_approvals"`
			PendingApprovals   int `json:"pending_approvals"`
			ConnectorCount     int `json:"connector_count"`
			ActiveRoutes       int `json:"active_routes"`
			TerminalDeliveries int `json:"terminal_deliveries"`
		} `json:"summary"`
		Approvals []struct {
			ID       string `json:"id"`
			ToolName string `json:"tool_name"`
			Status   string `json:"status"`
		} `json:"approvals"`
		Repair struct {
			Health []struct {
				ConnectorID   string `json:"connector_id"`
				TerminalCount int    `json:"terminal_count"`
			} `json:"health"`
			RuntimeConnectors []struct {
				ConnectorID string `json:"connector_id"`
				State       string `json:"state"`
				Summary     string `json:"summary"`
			} `json:"runtime_connectors"`
			ActiveRoutes []struct {
				ID          string `json:"id"`
				SessionID   string `json:"session_id"`
				ConnectorID string `json:"connector_id"`
			} `json:"active_routes"`
			Deliveries []struct {
				ID          string `json:"id"`
				SessionID   string `json:"session_id"`
				ConnectorID string `json:"connector_id"`
				Status      string `json:"status"`
			} `json:"deliveries"`
		} `json:"repair"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Summary.OpenApprovals != 1 || resp.Summary.PendingApprovals != 1 {
		t.Fatalf("unexpected approval summary %+v", resp.Summary)
	}
	if resp.Summary.ConnectorCount != 1 || resp.Summary.ActiveRoutes != 1 || resp.Summary.TerminalDeliveries != 1 {
		t.Fatalf("unexpected repair summary %+v", resp.Summary)
	}
	if len(resp.Approvals) == 0 || resp.Approvals[0].ID != approvalID || resp.Approvals[0].Status != "pending" {
		t.Fatalf("unexpected approvals %+v", resp.Approvals)
	}
	if len(resp.Repair.ActiveRoutes) != 1 || resp.Repair.ActiveRoutes[0].ID != route.ID {
		t.Fatalf("unexpected active routes %+v", resp.Repair.ActiveRoutes)
	}
	if len(resp.Repair.Deliveries) != 1 || resp.Repair.Deliveries[0].ID != deliveryID || resp.Repair.Deliveries[0].Status != "terminal" {
		t.Fatalf("unexpected deliveries %+v", resp.Repair.Deliveries)
	}
	if len(resp.Repair.Health) != 1 || resp.Repair.Health[0].ConnectorID != "telegram" || resp.Repair.Health[0].TerminalCount != 1 {
		t.Fatalf("unexpected health %+v", resp.Repair.Health)
	}
	if len(resp.Repair.RuntimeConnectors) != 1 || resp.Repair.RuntimeConnectors[0].Summary != "poll loop stale" {
		t.Fatalf("unexpected runtime connectors %+v", resp.Repair.RuntimeConnectors)
	}
}

func TestRecoverApproveAndRetryMutationsFlowThroughRuntime(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	approvalID := h.insertApproval(t, "run-recover-approve", "bash", "echo hi")
	_, _, deliveryID := h.seedRoutesDeliveriesData(t)
	h.markOutboundIntentTerminal(t, deliveryID)

	approvalRR := httptest.NewRecorder()
	approvalReq := httptest.NewRequest(http.MethodPost, "/api/recover/approvals/"+approvalID+"/resolve", strings.NewReader("decision=approved"))
	approvalReq.Header.Set("Authorization", "Bearer "+h.adminToken)
	approvalReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	approvalReq.Header.Set("Accept", "application/json")
	h.server.ServeHTTP(approvalRR, approvalReq)

	if approvalRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", approvalRR.Code, approvalRR.Body.String())
	}

	var approvalResp struct {
		ApprovalID string `json:"approval_id"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(approvalRR.Body.Bytes(), &approvalResp); err != nil {
		t.Fatalf("decode approval response: %v", err)
	}
	if approvalResp.ApprovalID != approvalID || approvalResp.Status != "approved" {
		t.Fatalf("unexpected approval response %+v", approvalResp)
	}

	retryRR := httptest.NewRecorder()
	retryReq := httptest.NewRequest(http.MethodPost, "/api/recover/deliveries/"+deliveryID+"/retry", nil)
	retryReq.Header.Set("Authorization", "Bearer "+h.adminToken)
	h.server.ServeHTTP(retryRR, retryReq)

	if retryRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", retryRR.Code, retryRR.Body.String())
	}

	var retryResp struct {
		Delivery struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"delivery"`
	}
	if err := json.Unmarshal(retryRR.Body.Bytes(), &retryResp); err != nil {
		t.Fatalf("decode retry response: %v", err)
	}
	if retryResp.Delivery.ID != deliveryID || retryResp.Delivery.Status != "pending" {
		t.Fatalf("unexpected retry response %+v", retryResp.Delivery)
	}

	delivery, err := h.rt.RetryDelivery(context.Background(), deliveryID)
	if err != runtime.ErrDeliveryNotRetryable {
		t.Fatalf("expected delivery to already be requeued, got delivery=%+v err=%v", delivery, err)
	}
}
