package web

import (
	"context"
	"fmt"
	"net/http"

	"github.com/canhta/gistclaw/internal/authority"
	"github.com/canhta/gistclaw/internal/runtime"
)

type approvalPolicyResponse struct {
	Summary    approvalPolicySummaryResponse     `json:"summary"`
	Gateway    approvalPolicyGatewayResponse     `json:"gateway"`
	Nodes      []approvalPolicyNodeResponse      `json:"nodes"`
	Allowlists []approvalPolicyAllowlistResponse `json:"allowlists"`
}

type approvalPolicySummaryResponse struct {
	NodeCount      int `json:"node_count"`
	AllowlistCount int `json:"allowlist_count"`
	PendingAgents  int `json:"pending_agents"`
	OverrideAgents int `json:"override_agents"`
}

type approvalPolicyGatewayResponse struct {
	ApprovalMode        string `json:"approval_mode"`
	ApprovalModeLabel   string `json:"approval_mode_label"`
	HostAccessMode      string `json:"host_access_mode"`
	HostAccessModeLabel string `json:"host_access_mode_label"`
	TeamName            string `json:"team_name"`
	FrontAgentID        string `json:"front_agent_id"`
}

type approvalPolicyNodeResponse struct {
	AgentID                     string   `json:"agent_id"`
	Role                        string   `json:"role"`
	BaseProfile                 string   `json:"base_profile"`
	IsFront                     bool     `json:"is_front"`
	ToolFamilies                []string `json:"tool_families"`
	DelegationKinds             []string `json:"delegation_kinds"`
	CanMessage                  []string `json:"can_message"`
	AllowTools                  []string `json:"allow_tools"`
	DenyTools                   []string `json:"deny_tools"`
	PendingApprovals            int      `json:"pending_approvals"`
	RecentRuns                  int      `json:"recent_runs"`
	OverrideRuns                int      `json:"override_runs"`
	ObservedApprovalMode        string   `json:"observed_approval_mode"`
	ObservedApprovalModeLabel   string   `json:"observed_approval_mode_label"`
	ObservedHostAccessMode      string   `json:"observed_host_access_mode"`
	ObservedHostAccessModeLabel string   `json:"observed_host_access_mode_label"`
}

type approvalPolicyAllowlistResponse struct {
	AgentID        string `json:"agent_id"`
	Role           string `json:"role"`
	ToolName       string `json:"tool_name"`
	Direction      string `json:"direction"`
	DirectionLabel string `json:"direction_label"`
}

type approvalPolicyRunStats struct {
	RecentRuns             int
	OverrideRuns           int
	ObservedApprovalMode   string
	ObservedHostAccessMode string
	Observed               bool
}

func (s *Server) handleApprovalPolicyStatus(w http.ResponseWriter, r *http.Request) {
	resp, err := s.loadApprovalPolicyStatus(r.Context())
	if err != nil {
		http.Error(w, "failed to load approval policy", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) loadApprovalPolicyStatus(ctx context.Context) (approvalPolicyResponse, error) {
	state, err := s.loadTeamState(ctx)
	if err != nil {
		return approvalPolicyResponse{}, fmt.Errorf("load team state: %w", err)
	}

	gateway := approvalPolicyGatewayResponse{
		ApprovalMode:   lookupSetting(s.db, "approval_mode"),
		HostAccessMode: lookupSetting(s.db, "host_access_mode"),
		TeamName:       state.Config.Name,
		FrontAgentID:   state.Config.FrontAgent,
	}
	if gateway.ApprovalMode == "" {
		gateway.ApprovalMode = string(authority.ApprovalModePrompt)
	}
	if gateway.HostAccessMode == "" {
		gateway.HostAccessMode = string(authority.HostAccessModeStandard)
	}
	gateway.ApprovalModeLabel = humanizeWebLabel(gateway.ApprovalMode)
	gateway.HostAccessModeLabel = humanizeWebLabel(gateway.HostAccessMode)

	project, err := runtime.ActiveProject(ctx, s.db)
	if err != nil {
		return approvalPolicyResponse{}, fmt.Errorf("load active project: %w", err)
	}

	runStats, err := s.loadApprovalPolicyRunStats(ctx, project.ID, gateway)
	if err != nil {
		return approvalPolicyResponse{}, err
	}
	pendingByAgent, err := s.loadApprovalPolicyPendingCounts(ctx, project.ID)
	if err != nil {
		return approvalPolicyResponse{}, err
	}

	nodes := make([]approvalPolicyNodeResponse, 0, len(state.Config.Agents))
	allowlists := make([]approvalPolicyAllowlistResponse, 0)
	summary := approvalPolicySummaryResponse{}

	for _, agent := range state.Config.Agents {
		stats := runStats[agent.ID]
		observedApprovalMode := stats.ObservedApprovalMode
		if observedApprovalMode == "" {
			observedApprovalMode = gateway.ApprovalMode
		}
		observedHostAccessMode := stats.ObservedHostAccessMode
		if observedHostAccessMode == "" {
			observedHostAccessMode = gateway.HostAccessMode
		}

		node := approvalPolicyNodeResponse{
			AgentID:                     agent.ID,
			Role:                        agent.Role,
			BaseProfile:                 string(agent.BaseProfile),
			IsFront:                     agent.ID == state.Config.FrontAgent,
			ToolFamilies:                approvalPolicyStrings(agent.ToolFamilies),
			DelegationKinds:             approvalPolicyStrings(agent.DelegationKinds),
			CanMessage:                  append([]string(nil), agent.CanMessage...),
			AllowTools:                  append([]string(nil), agent.AllowTools...),
			DenyTools:                   append([]string(nil), agent.DenyTools...),
			PendingApprovals:            pendingByAgent[agent.ID],
			RecentRuns:                  stats.RecentRuns,
			OverrideRuns:                stats.OverrideRuns,
			ObservedApprovalMode:        observedApprovalMode,
			ObservedApprovalModeLabel:   humanizeWebLabel(observedApprovalMode),
			ObservedHostAccessMode:      observedHostAccessMode,
			ObservedHostAccessModeLabel: humanizeWebLabel(observedHostAccessMode),
		}
		nodes = append(nodes, node)
		summary.NodeCount++
		if node.PendingApprovals > 0 {
			summary.PendingAgents++
		}
		if node.OverrideRuns > 0 {
			summary.OverrideAgents++
		}

		for _, toolName := range agent.AllowTools {
			allowlists = append(allowlists, approvalPolicyAllowlistResponse{
				AgentID:        agent.ID,
				Role:           agent.Role,
				ToolName:       toolName,
				Direction:      "allow",
				DirectionLabel: humanizeWebLabel("allow"),
			})
		}
		for _, toolName := range agent.DenyTools {
			allowlists = append(allowlists, approvalPolicyAllowlistResponse{
				AgentID:        agent.ID,
				Role:           agent.Role,
				ToolName:       toolName,
				Direction:      "deny",
				DirectionLabel: humanizeWebLabel("deny"),
			})
		}
	}
	summary.AllowlistCount = len(allowlists)

	return approvalPolicyResponse{
		Summary:    summary,
		Gateway:    gateway,
		Nodes:      nodes,
		Allowlists: allowlists,
	}, nil
}

func (s *Server) loadApprovalPolicyRunStats(
	ctx context.Context,
	projectID string,
	gateway approvalPolicyGatewayResponse,
) (map[string]approvalPolicyRunStats, error) {
	stats := make(map[string]approvalPolicyRunStats)
	if projectID == "" {
		return stats, nil
	}

	rows, err := s.db.RawDB().QueryContext(
		ctx,
		`SELECT agent_id, COALESCE(authority_json, x'7b7d')
		   FROM runs
		  WHERE project_id = ?
		  ORDER BY updated_at DESC, id DESC
		  LIMIT 64`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("query approval policy runs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var agentID string
		var raw []byte
		if err := rows.Scan(&agentID, &raw); err != nil {
			return nil, fmt.Errorf("scan approval policy run: %w", err)
		}
		env, err := authority.DecodeEnvelope(raw)
		if err != nil {
			return nil, fmt.Errorf("decode authority envelope: %w", err)
		}

		item := stats[agentID]
		item.RecentRuns++
		if !item.Observed {
			item.ObservedApprovalMode = string(env.ApprovalMode)
			item.ObservedHostAccessMode = string(env.HostAccessMode)
			item.Observed = true
		}
		if string(env.ApprovalMode) != gateway.ApprovalMode || string(env.HostAccessMode) != gateway.HostAccessMode {
			item.OverrideRuns++
		}
		stats[agentID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate approval policy runs: %w", err)
	}
	return stats, nil
}

func (s *Server) loadApprovalPolicyPendingCounts(ctx context.Context, projectID string) (map[string]int, error) {
	counts := make(map[string]int)
	if projectID == "" {
		return counts, nil
	}

	rows, err := s.db.RawDB().QueryContext(
		ctx,
		`SELECT runs.agent_id, COUNT(approvals.id)
		   FROM approvals
		   JOIN runs ON runs.id = approvals.run_id
		  WHERE runs.project_id = ?
		    AND approvals.status = 'pending'
		  GROUP BY runs.agent_id`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("query approval policy pending counts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var agentID string
		var count int
		if err := rows.Scan(&agentID, &count); err != nil {
			return nil, fmt.Errorf("scan approval policy pending count: %w", err)
		}
		counts[agentID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate approval policy pending counts: %w", err)
	}
	return counts, nil
}

func approvalPolicyStrings[T ~string](values []T) []string {
	if len(values) == 0 {
		return nil
	}
	resp := make([]string, 0, len(values))
	for _, value := range values {
		resp = append(resp, string(value))
	}
	return resp
}
