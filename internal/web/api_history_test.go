package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHistoryAPIListsRunEvidenceInterventionsAndDeliveryOutcomes(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	h.insertRunAt(t, "run-history-complete", "conv-history-complete", "Inspect repository health", "completed", "2026-03-25 10:00:00")
	h.insertRunAt(t, "run-history-failed", "conv-history-failed", "Repair connector backlog", "failed", "2026-03-25 09:00:00")
	h.insertRunAt(t, "run-history-child", "conv-history-failed", "Collect failure evidence", "completed", "2026-03-25 09:05:00")
	h.insertApprovalAt(t, "run-history-complete", "apply_patch", h.workspaceRoot+"/history.txt", "approved", "operator", "2026-03-25 10:15:00")
	h.insertApprovalAt(t, "run-history-open", "bash", h.workspaceRoot+"/open.txt", "pending", "", "2026-03-25 10:20:00")

	run, _, intentID := h.seedRoutesDeliveriesData(t)
	h.markOutboundIntentTerminal(t, intentID)
	h.insertApprovalAt(t, run.ID, "bash", h.workspaceRoot+"/route.txt", "denied", "operator", "2026-03-25 08:30:00")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/history", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Summary struct {
			RunCount         int `json:"run_count"`
			CompletedRuns    int `json:"completed_runs"`
			RecoveryRuns     int `json:"recovery_runs"`
			ApprovalEvents   int `json:"approval_events"`
			DeliveryOutcomes int `json:"delivery_outcomes"`
		} `json:"summary"`
		Runs []struct {
			Root struct {
				ID          string `json:"id"`
				Objective   string `json:"objective"`
				Status      string `json:"status"`
				StatusLabel string `json:"status_label"`
			} `json:"root"`
		} `json:"runs"`
		Approvals []struct {
			ID              string `json:"id"`
			RunID           string `json:"run_id"`
			ToolName        string `json:"tool_name"`
			Status          string `json:"status"`
			ResolvedBy      string `json:"resolved_by"`
			ResolvedAtLabel string `json:"resolved_at_label"`
		} `json:"approvals"`
		Deliveries []struct {
			ID                 string `json:"id"`
			RunID              string `json:"run_id"`
			ConnectorID        string `json:"connector_id"`
			Status             string `json:"status"`
			AttemptsLabel      string `json:"attempts_label"`
			LastAttemptAtLabel string `json:"last_attempt_at_label"`
		} `json:"deliveries"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Summary.RunCount < 4 || resp.Summary.CompletedRuns < 2 || resp.Summary.RecoveryRuns < 1 {
		t.Fatalf("unexpected summary %+v", resp.Summary)
	}
	if resp.Summary.ApprovalEvents != 2 || resp.Summary.DeliveryOutcomes != 1 {
		t.Fatalf("unexpected evidence summary %+v", resp.Summary)
	}
	if len(resp.Runs) < 2 || resp.Runs[0].Root.ID != run.ID || !containsHistoryRun(resp.Runs, "run-history-complete") {
		t.Fatalf("unexpected run history %+v", resp.Runs)
	}
	if len(resp.Approvals) != 2 || resp.Approvals[0].Status == "pending" || resp.Approvals[0].ResolvedAtLabel == "" {
		t.Fatalf("unexpected approvals %+v", resp.Approvals)
	}
	if len(resp.Deliveries) != 1 || resp.Deliveries[0].ID != intentID || resp.Deliveries[0].Status != "terminal" {
		t.Fatalf("unexpected deliveries %+v", resp.Deliveries)
	}
	if resp.Deliveries[0].AttemptsLabel == "" || resp.Deliveries[0].LastAttemptAtLabel == "" {
		t.Fatalf("expected delivery evidence labels %+v", resp.Deliveries[0])
	}
}

func containsHistoryRun(items []struct {
	Root struct {
		ID          string `json:"id"`
		Objective   string `json:"objective"`
		Status      string `json:"status"`
		StatusLabel string `json:"status_label"`
	} `json:"root"`
}, runID string) bool {
	for _, item := range items {
		if item.Root.ID == runID {
			return true
		}
	}
	return false
}
