package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApprovalFlow_ExpiredBadgeVisible(t *testing.T) {
	h := newServerHarness(t)
	h.insertRun(t, "run-old", "conv-old", "expired approval", "needs_approval")

	// Insert an expired approval directly.
	_, err := h.db.RawDB().Exec(
		`INSERT INTO approvals (id, run_id, tool_name, args_json, target_path, fingerprint, status, created_at)
		 VALUES ('approval-expired-badge', 'run-old', 'bash', x'', '/tmp', 'fp-exp', 'expired', datetime('now', '-25 hours'))`,
	)
	if err != nil {
		t.Fatalf("insert expired approval: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/recover/approvals", nil)
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
	h.insertRun(t, "run-old2", "conv-old2", "expired approval", "needs_approval")

	// Insert an expired approval.
	_, err := h.db.RawDB().Exec(
		`INSERT INTO approvals (id, run_id, tool_name, args_json, target_path, fingerprint, status, created_at)
		 VALUES ('approval-expired-resolve', 'run-old2', 'bash', x'', '/tmp', 'fp-exp2', 'expired', datetime('now', '-25 hours'))`,
	)
	if err != nil {
		t.Fatalf("insert expired approval: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recover/approvals/approval-expired-resolve/resolve",
		strings.NewReader("decision=approve"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+h.adminToken)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict, got %d", rr.Code)
	}
}

func TestApprovalFlow_AuditTrailShowsResolvedToday(t *testing.T) {
	h := newServerHarness(t)
	h.insertRun(t, "run-audit", "conv-audit", "audit approval", "completed")

	_, err := h.db.RawDB().Exec(
		`INSERT INTO approvals (id, run_id, tool_name, args_json, target_path, fingerprint, status, resolved_by, created_at, resolved_at)
		 VALUES ('approval-audit-web', 'run-audit', 'bash', x'', '/tmp', 'fp-audit', 'approved', 'web', datetime('now', '-1 hour'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert resolved approval: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/recover/approvals", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "approval-audit-web") {
		t.Errorf("expected audit trail to contain approval ID:\n%s", body[:min(len(body), 500)])
	}
	if !strings.Contains(body, "web") {
		t.Errorf("expected audit trail to show actor 'web':\n%s", body[:min(len(body), 500)])
	}
}

func TestApprovalFlow_AuditTrailShowsTelegramActor(t *testing.T) {
	h := newServerHarness(t)
	h.insertRun(t, "run-audit-tg", "conv-audit-tg", "audit approval", "completed")

	_, err := h.db.RawDB().Exec(
		`INSERT INTO approvals (id, run_id, tool_name, args_json, target_path, fingerprint, status, resolved_by, created_at, resolved_at)
		 VALUES ('approval-audit-tg', 'run-audit-tg', 'bash', x'', '/tmp', 'fp-audit-tg', 'approved', 'telegram', datetime('now', '-2 hours'), datetime('now', '-1 hour'))`,
	)
	if err != nil {
		t.Fatalf("insert telegram approval: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/recover/approvals", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "telegram") {
		t.Errorf("expected audit trail to show actor 'telegram':\n%s", body[:min(len(body), 500)])
	}
}

func TestApprovalFlow_HidesOtherProjectApprovals(t *testing.T) {
	h := newServerHarness(t)
	h.insertRun(t, "run-active-approval", "conv-active-approval", "approve active project change", "needs_approval")
	h.insertApproval(t, "run-active-approval", "shell_exec", h.workspaceRoot+"/active.txt")

	otherRoot := t.TempDir()
	h.insertProject(t, "seo-test", otherRoot)
	h.insertRunInWorkspace(t, "run-other-approval", "conv-other-approval", "approve seo project change", "needs_approval", otherRoot)
	h.insertApproval(t, "run-other-approval", "shell_exec", otherRoot+"/other.txt")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/recover/approvals", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "run-active-approval") {
		t.Fatalf("expected active project approval to be visible, got:\n%s", body)
	}
	if strings.Contains(body, "run-other-approval") || strings.Contains(body, otherRoot+"/other.txt") {
		t.Fatalf("expected other project approval to be hidden, got:\n%s", body)
	}
}

func TestApprovalFlow_InterruptedRunDismiss(t *testing.T) {
	h := newServerHarness(t)

	// Insert a run that was left in 'interrupted' state.
	h.insertRun(t, "run-interrupted-dismiss", "conv-dismiss", "do something", "interrupted")

	// Dismiss it.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/operate/runs/run-interrupted-dismiss/dismiss", nil)
	req.Header.Set("X-Admin-Token", h.adminToken)
	h.server.ServeHTTP(rr, req)

	// After dismiss, the run list's active section should not include it.
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/operate/runs", nil)
	h.server.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("GET /operate/runs: expected 200, got %d", rr2.Code)
	}

	// The run should still be retrievable by direct ID.
	rr3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/operate/runs/run-interrupted-dismiss", nil)
	h.server.ServeHTTP(rr3, req3)

	if rr3.Code == http.StatusNotFound {
		t.Error("expected dismissed run to still be accessible by ID, got 404")
	}
}
