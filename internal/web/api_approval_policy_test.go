package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestApprovalPolicyStatusReturnsGatewayNodesAndAllowlists(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	seedSettings(t, h.db, map[string]string{
		"approval_mode":    "auto_approve",
		"host_access_mode": "elevated",
	})
	writeTestFile(t, filepath.Join(h.teamDir, "team.yaml"), `
name: Approval Team
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    base_profile: operator
    tool_families: [repo_read, runtime_capability, connector_capability, delegate]
    allow_tools: [shell_exec, connector_send]
    deny_tools: [app_action]
    delegation_kinds: [write, review]
    can_message: [patcher]
    specialist_summary_visibility: full
  - id: patcher
    soul_file: patcher.soul.yaml
    base_profile: write
    tool_families: [repo_read, repo_write]
    deny_tools: [repo_exec]
    can_message: [assistant]
    specialist_summary_visibility: basic
`)
	writeTestFile(t, filepath.Join(h.teamDir, "assistant.soul.yaml"), "role: front assistant\n")
	writeTestFile(t, filepath.Join(h.teamDir, "patcher.soul.yaml"), "role: scoped write specialist\n")

	if _, err := h.db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, project_id, team_id, objective, cwd, authority_json, status, created_at, updated_at)
		 VALUES
		 (?, ?, 'assistant', ?, 'repo-task-team', ?, ?, ?, 'active', '2026-03-29 10:00:00', '2026-03-29 10:05:00'),
		 (?, ?, 'patcher', ?, 'repo-task-team', ?, ?, ?, 'needs_approval', '2026-03-29 10:02:00', '2026-03-29 10:06:00')`,
		"run-assistant-policy",
		"conv-assistant-policy",
		h.activeProjectID,
		"Coordinate the repair",
		h.workspaceRoot,
		[]byte(`{"approval_mode":"prompt","host_access_mode":"standard"}`),
		"run-patcher-policy",
		"conv-patcher-policy",
		h.activeProjectID,
		"Patch the connector flow",
		h.workspaceRoot,
		[]byte(`{"approval_mode":"auto_approve","host_access_mode":"elevated"}`),
	); err != nil {
		t.Fatalf("insert policy runs: %v", err)
	}
	if _, err := h.db.RawDB().Exec(
		`INSERT INTO approvals
		 (id, run_id, tool_name, args_json, binding_json, fingerprint, status, created_at)
		 VALUES
		 (?, ?, 'repo_exec', '{}', '{}', 'fp-patcher', 'pending', '2026-03-29 10:06:30')`,
		"appr-policy-patcher",
		"run-patcher-policy",
	); err != nil {
		t.Fatalf("insert pending approval: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/approvals/policy", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Summary struct {
			NodeCount      int `json:"node_count"`
			AllowlistCount int `json:"allowlist_count"`
			PendingAgents  int `json:"pending_agents"`
			OverrideAgents int `json:"override_agents"`
		} `json:"summary"`
		Gateway struct {
			ApprovalMode      string `json:"approval_mode"`
			ApprovalModeLabel string `json:"approval_mode_label"`
			HostAccessMode    string `json:"host_access_mode"`
			TeamName          string `json:"team_name"`
			FrontAgentID      string `json:"front_agent_id"`
		} `json:"gateway"`
		Nodes []struct {
			AgentID                string   `json:"agent_id"`
			Role                   string   `json:"role"`
			BaseProfile            string   `json:"base_profile"`
			AllowTools             []string `json:"allow_tools"`
			DenyTools              []string `json:"deny_tools"`
			PendingApprovals       int      `json:"pending_approvals"`
			RecentRuns             int      `json:"recent_runs"`
			ObservedApprovalMode   string   `json:"observed_approval_mode"`
			ObservedHostAccessMode string   `json:"observed_host_access_mode"`
		} `json:"nodes"`
		Allowlists []struct {
			AgentID        string `json:"agent_id"`
			ToolName       string `json:"tool_name"`
			Direction      string `json:"direction"`
			DirectionLabel string `json:"direction_label"`
		} `json:"allowlists"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode approvals policy response: %v", err)
	}

	if resp.Summary.NodeCount != 2 || resp.Summary.AllowlistCount != 4 || resp.Summary.PendingAgents != 1 {
		t.Fatalf("unexpected summary: %+v", resp.Summary)
	}
	if resp.Gateway.ApprovalMode != "auto_approve" || resp.Gateway.TeamName != "Approval Team" || resp.Gateway.FrontAgentID != "assistant" {
		t.Fatalf("unexpected gateway: %+v", resp.Gateway)
	}
	if len(resp.Nodes) != 2 {
		t.Fatalf("expected 2 node rows, got %d", len(resp.Nodes))
	}
	if resp.Nodes[0].AgentID != "assistant" || resp.Nodes[0].ObservedApprovalMode != "prompt" || resp.Nodes[0].ObservedHostAccessMode != "standard" {
		t.Fatalf("unexpected assistant node: %+v", resp.Nodes[0])
	}
	if resp.Nodes[1].AgentID != "patcher" || resp.Nodes[1].PendingApprovals != 1 || resp.Nodes[1].ObservedHostAccessMode != "elevated" {
		t.Fatalf("unexpected patcher node: %+v", resp.Nodes[1])
	}
	if len(resp.Allowlists) != 4 {
		t.Fatalf("expected 4 allowlist entries, got %d", len(resp.Allowlists))
	}
	if resp.Allowlists[0].AgentID != "assistant" || resp.Allowlists[0].Direction != "allow" {
		t.Fatalf("unexpected first allowlist entry: %+v", resp.Allowlists[0])
	}
	if resp.Allowlists[3].AgentID != "patcher" || resp.Allowlists[3].Direction != "deny" || resp.Allowlists[3].ToolName != "repo_exec" {
		t.Fatalf("unexpected last allowlist entry: %+v", resp.Allowlists[3])
	}
}
