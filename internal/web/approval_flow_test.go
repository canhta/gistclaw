package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApprovalFlow_ExpiredBadgeVisible(t *testing.T) {
	h := newServerHarness(t)

	// Insert an expired approval directly.
	_, err := h.db.RawDB().Exec(
		`INSERT INTO approvals (id, run_id, tool_name, args_json, target_path, fingerprint, status, created_at)
		 VALUES ('approval-expired-badge', 'run-old', 'bash', x'', '/tmp', 'fp-exp', 'expired', datetime('now', '-25 hours'))`,
	)
	if err != nil {
		t.Fatalf("insert expired approval: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/approvals", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Expired") {
		t.Errorf("expected 'Expired' badge in response body:\n%s", rr.Body.String())
	}
}

func TestApprovalFlow_ExpiredResolveReturns409(t *testing.T) {
	h := newServerHarness(t)

	// Insert an expired approval.
	_, err := h.db.RawDB().Exec(
		`INSERT INTO approvals (id, run_id, tool_name, args_json, target_path, fingerprint, status, created_at)
		 VALUES ('approval-expired-resolve', 'run-old2', 'bash', x'', '/tmp', 'fp-exp2', 'expired', datetime('now', '-25 hours'))`,
	)
	if err != nil {
		t.Fatalf("insert expired approval: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/approvals/approval-expired-resolve/resolve",
		strings.NewReader("decision=approve"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+h.adminToken)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict, got %d", rr.Code)
	}
}

func TestApprovalFlow_InterruptedRunDismiss(t *testing.T) {
	h := newServerHarness(t)

	// Insert a run that was left in 'interrupted' state.
	h.insertRun(t, "run-interrupted-dismiss", "conv-dismiss", "do something", "interrupted")

	// Dismiss it.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/runs/run-interrupted-dismiss/dismiss", nil)
	req.Header.Set("X-Admin-Token", h.adminToken)
	h.server.ServeHTTP(rr, req)

	// After dismiss, the run list's active section should not include it.
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/runs", nil)
	h.server.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("GET /runs: expected 200, got %d", rr2.Code)
	}

	// The run should still be retrievable by direct ID.
	rr3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/runs/run-interrupted-dismiss", nil)
	h.server.ServeHTTP(rr3, req3)

	if rr3.Code == http.StatusNotFound {
		t.Error("expected dismissed run to still be accessible by ID, got 404")
	}
}
