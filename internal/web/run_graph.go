package web

import (
	"fmt"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/replay"
)

type runGraphView struct {
	RootRunID  string                   `json:"root_run_id"`
	Headline   string                   `json:"headline"`
	Summary    runGraphSummaryView      `json:"summary"`
	Lanes      []runGraphLaneView       `json:"lanes,omitempty"`
	Nodes      []runGraphNodeView       `json:"nodes"`
	Edges      []runGraphEdgeView       `json:"edges"`
	Legend     []runGraphLegendItemView `json:"legend,omitempty"`
	ActivePath []string                 `json:"active_path,omitempty"`
}

type runGraphLaneView struct {
	ID       string               `json:"id"`
	Label    string               `json:"label"`
	Branches []runGraphBranchView `json:"branches,omitempty"`
}

type runGraphBranchView struct {
	RootNodeID       string             `json:"root_node_id"`
	Label            string             `json:"label"`
	DefaultCollapsed bool               `json:"default_collapsed"`
	VisibleCount     int                `json:"visible_count"`
	HiddenCount      int                `json:"hidden_count"`
	StatusClass      string             `json:"status_class"`
	Nodes            []runGraphNodeView `json:"nodes,omitempty"`
}

type runGraphLegendItemView struct {
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	StatusClass string `json:"status_class"`
}

type runGraphEdgeView struct {
	ID          string `json:"id"`
	From        string `json:"from"`
	To          string `json:"to"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	StatusClass string `json:"status_class,omitempty"`
}

type runGraphSummaryView struct {
	Total         int    `json:"total"`
	Pending       int    `json:"pending"`
	Active        int    `json:"active"`
	NeedsApproval int    `json:"needs_approval"`
	Completed     int    `json:"completed"`
	Failed        int    `json:"failed"`
	Interrupted   int    `json:"interrupted"`
	RootStatus    string `json:"root_status"`
}

type runGraphNodeView struct {
	ID                   string                `json:"id"`
	ShortID              string                `json:"short_id"`
	ShortLabel           string                `json:"short_label"`
	ParentRunID          string                `json:"parent_run_id"`
	AgentID              string                `json:"agent_id"`
	SessionID            string                `json:"session_id,omitempty"`
	SessionShortID       string                `json:"session_short_id,omitempty"`
	Objective            string                `json:"objective"`
	ObjectivePreview     string                `json:"objective_preview"`
	Preview              runStructuredTextView `json:"preview,omitempty"`
	HasObjectiveOverflow bool                  `json:"has_objective_overflow"`
	Status               string                `json:"status"`
	StatusLabel          string                `json:"status_label"`
	StatusClass          string                `json:"status_class"`
	Kind                 string                `json:"kind"`
	LaneID               string                `json:"lane_id"`
	ModelDisplay         string                `json:"model_display"`
	TokenSummary         string                `json:"token_summary"`
	TimeLabel            string                `json:"time_label"`
	StartedAtLabel       string                `json:"started_at_label"`
	UpdatedAtLabel       string                `json:"updated_at_label"`
	Depth                int                   `json:"depth"`
	IsRoot               bool                  `json:"is_root"`
	IsActivePath         bool                  `json:"is_active_path"`
	BranchRootID         string                `json:"branch_root_id,omitempty"`
	ChildCount           int                   `json:"child_count"`
	ParentLabel          string                `json:"parent_label,omitempty"`
}

func buildRunGraphView(snapshot replay.RunGraphSnapshot) runGraphView {
	nodes := buildGraphNodes(snapshot)
	activePath := buildActivePath(nodes)
	markActivePath(nodes, activePath)
	summary := summarizeRunGraph(nodes)

	return runGraphView{
		RootRunID:  snapshot.RootRunID,
		Headline:   runGraphHeadline(summary),
		Summary:    summary,
		Lanes:      buildGraphLanes(nodes, snapshot.RootRunID),
		Nodes:      nodes,
		Edges:      buildGraphEdges(nodes),
		Legend:     buildGraphLegend(),
		ActivePath: activePath,
	}
}

func buildGraphNodes(snapshot replay.RunGraphSnapshot) []runGraphNodeView {
	childCountByID := make(map[string]int, len(snapshot.Nodes))
	parentByID := make(map[string]string, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		parentByID[node.ID] = node.ParentRunID
		if node.ParentRunID != "" {
			childCountByID[node.ParentRunID]++
		}
	}

	nodes := make([]runGraphNodeView, 0, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		preview := buildStructuredTextView(node.Objective, 2)
		kind := graphNodeKind(snapshot.RootRunID, node.ID, node.AgentID)
		laneID := graphLaneID(kind, node.AgentID)
		graphNode := runGraphNodeView{
			ID:                   node.ID,
			ShortID:              compactIdentifier(node.ID),
			ShortLabel:           compactIdentifier(node.ID),
			ParentRunID:          node.ParentRunID,
			AgentID:              node.AgentID,
			SessionID:            node.SessionID,
			SessionShortID:       compactIdentifier(node.SessionID),
			Objective:            node.Objective,
			ObjectivePreview:     preview.PreviewText,
			Preview:              preview,
			HasObjectiveOverflow: preview.HasOverflow,
			Status:               string(node.Status),
			StatusLabel:          humanizeRunStatus(string(node.Status)),
			StatusClass:          runStatusClass(string(node.Status)),
			Kind:                 kind,
			LaneID:               laneID,
			ModelDisplay:         formatRunModelDisplay(node.ModelID, node.ModelLane),
			TokenSummary:         formatRunTokenSummary(node.InputTokens, node.OutputTokens),
			TimeLabel:            formatRunCompactTimestamp(node.CreatedAt),
			StartedAtLabel:       formatRunCompactTimestamp(node.CreatedAt),
			UpdatedAtLabel:       formatRunTimestamp(node.UpdatedAt),
			Depth:                node.Depth,
			IsRoot:               node.ID == snapshot.RootRunID,
			BranchRootID:         graphBranchRootID(snapshot.RootRunID, node.ID, parentByID),
			ChildCount:           childCountByID[node.ID],
		}
		if graphNode.ParentRunID != "" {
			graphNode.ParentLabel = "from " + compactIdentifier(graphNode.ParentRunID)
		}
		nodes = append(nodes, graphNode)
	}
	return nodes
}

func summarizeRunGraph(nodes []runGraphNodeView) runGraphSummaryView {
	var summary runGraphSummaryView
	for _, node := range nodes {
		switch node.Status {
		case string(model.RunStatusPending):
			summary.Pending++
		case string(model.RunStatusActive):
			summary.Active++
		case string(model.RunStatusNeedsApproval):
			summary.NeedsApproval++
		case string(model.RunStatusCompleted):
			summary.Completed++
		case string(model.RunStatusFailed):
			summary.Failed++
		case string(model.RunStatusInterrupted):
			summary.Interrupted++
		}
		summary.Total++
		if node.IsRoot {
			summary.RootStatus = node.Status
		}
	}
	return summary
}

func buildGraphEdges(nodes []runGraphNodeView) []runGraphEdgeView {
	edges := make([]runGraphEdgeView, 0, len(nodes)*2)
	for _, node := range nodes {
		if node.ParentRunID == "" {
			continue
		}
		kind, label := graphDelegationEdge(node.Status)
		edges = append(edges, runGraphEdgeView{
			ID:          fmt.Sprintf("%s->%s:%s", node.ParentRunID, node.ID, kind),
			From:        node.ParentRunID,
			To:          node.ID,
			Kind:        kind,
			Label:       label,
			StatusClass: graphEdgeStatusClass(kind, node.StatusClass),
		})
		if isSettledGraphStatus(node.Status) {
			edges = append(edges, runGraphEdgeView{
				ID:          fmt.Sprintf("%s->%s:reports", node.ID, node.ParentRunID),
				From:        node.ID,
				To:          node.ParentRunID,
				Kind:        "reports",
				Label:       "reported back",
				StatusClass: "is-muted",
			})
		}
	}
	return edges
}

func buildGraphLegend() []runGraphLegendItemView {
	return []runGraphLegendItemView{
		{Kind: "delegates", Label: "Delegated work", StatusClass: ""},
		{Kind: "reports", Label: "Reported back", StatusClass: "is-muted"},
		{Kind: "blocked", Label: "Waiting on approval", StatusClass: "is-approval"},
	}
}

func buildActivePath(nodes []runGraphNodeView) []string {
	if len(nodes) == 0 {
		return nil
	}

	nodeByID := make(map[string]runGraphNodeView, len(nodes))
	for _, node := range nodes {
		nodeByID[node.ID] = node
	}

	targetID := nodes[0].ID
	targetPriority := -1
	for _, node := range nodes {
		priority := graphStatusPriority(node.Status)
		if priority > targetPriority {
			targetPriority = priority
			targetID = node.ID
		}
	}

	reversed := make([]string, 0, len(nodes))
	currentID := targetID
	for currentID != "" {
		reversed = append(reversed, currentID)
		current, ok := nodeByID[currentID]
		if !ok {
			break
		}
		currentID = current.ParentRunID
	}

	path := make([]string, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		path = append(path, reversed[i])
	}
	return path
}

func markActivePath(nodes []runGraphNodeView, activePath []string) {
	if len(activePath) == 0 {
		return
	}
	active := make(map[string]struct{}, len(activePath))
	for _, nodeID := range activePath {
		active[nodeID] = struct{}{}
	}
	for i := range nodes {
		_, ok := active[nodes[i].ID]
		nodes[i].IsActivePath = ok
	}
}

func buildGraphLanes(nodes []runGraphNodeView, rootRunID string) []runGraphLaneView {
	laneNodes := make(map[string][]runGraphNodeView)
	for _, node := range nodes {
		laneNodes[node.LaneID] = append(laneNodes[node.LaneID], node)
	}

	laneOrder := []runGraphLaneView{
		{ID: "coordination", Label: "Coordination"},
		{ID: "research", Label: "Research"},
		{ID: "build", Label: "Build"},
		{ID: "review", Label: "Review"},
		{ID: "verify", Label: "Verify"},
		{ID: "workers", Label: "Workers"},
	}

	lanes := make([]runGraphLaneView, 0, len(laneOrder))
	for _, lane := range laneOrder {
		nodesInLane := laneNodes[lane.ID]
		if len(nodesInLane) == 0 {
			continue
		}
		lane.Branches = buildGraphBranches(nodesInLane, rootRunID)
		lanes = append(lanes, lane)
	}
	return lanes
}

func buildGraphBranches(nodes []runGraphNodeView, rootRunID string) []runGraphBranchView {
	order := make([]string, 0, len(nodes))
	branches := make(map[string]*runGraphBranchView)
	for _, node := range nodes {
		branchRootID := node.BranchRootID
		if branchRootID == "" {
			branchRootID = node.ID
		}
		branch := branches[branchRootID]
		if branch == nil {
			branch = &runGraphBranchView{
				RootNodeID: branchRootID,
				Label:      graphBranchLabel(node, rootRunID),
			}
			branches[branchRootID] = branch
			order = append(order, branchRootID)
		}
		branch.Nodes = append(branch.Nodes, node)
		branch.VisibleCount++
		if graphStatusPriority(node.Status) >= graphStatusPriority(string(model.RunStatusPending)) {
			branch.DefaultCollapsed = false
		}
		if branch.StatusClass == "" || graphStatusPriority(node.Status) > graphStatusPriority(statusFromClass(branch.StatusClass)) {
			branch.StatusClass = node.StatusClass
		}
	}

	result := make([]runGraphBranchView, 0, len(order))
	for _, branchRootID := range order {
		branch := branches[branchRootID]
		if branch.RootNodeID == rootRunID {
			branch.DefaultCollapsed = false
		} else {
			branch.DefaultCollapsed = !branchHasBusyStatus(branch.Nodes)
		}
		result = append(result, *branch)
	}
	return result
}

func graphNodeKind(rootRunID, nodeID, agentID string) string {
	if nodeID == rootRunID {
		return "root"
	}
	switch agentID {
	case "reviewer":
		return "review"
	case "verifier":
		return "verify"
	default:
		return "worker"
	}
}

func graphLaneID(kind, agentID string) string {
	if kind == "root" {
		return "coordination"
	}
	switch agentID {
	case "assistant":
		return "coordination"
	case "researcher":
		return "research"
	case "patcher":
		return "build"
	case "reviewer":
		return "review"
	case "verifier":
		return "verify"
	default:
		return "workers"
	}
}

func graphBranchRootID(rootRunID, nodeID string, parentByID map[string]string) string {
	if nodeID == rootRunID {
		return rootRunID
	}
	currentID := nodeID
	for currentID != "" {
		parentID := parentByID[currentID]
		if parentID == "" || parentID == rootRunID {
			return currentID
		}
		currentID = parentID
	}
	return nodeID
}

func graphBranchLabel(node runGraphNodeView, rootRunID string) string {
	if node.ID == rootRunID || node.BranchRootID == rootRunID || node.IsRoot {
		return "Front session"
	}
	if node.AgentID == "" {
		return "Worker branch"
	}
	return fmt.Sprintf("%s branch", node.AgentID)
}

func graphDelegationEdge(status string) (kind, label string) {
	if status == string(model.RunStatusNeedsApproval) {
		return "blocked", "waiting approval"
	}
	return "delegates", "delegated"
}

func graphEdgeStatusClass(kind, statusClass string) string {
	if kind == "blocked" {
		return "is-approval"
	}
	return statusClass
}

func graphStatusPriority(status string) int {
	switch status {
	case string(model.RunStatusNeedsApproval):
		return 6
	case string(model.RunStatusActive):
		return 5
	case string(model.RunStatusPending):
		return 4
	case string(model.RunStatusFailed):
		return 3
	case string(model.RunStatusInterrupted):
		return 2
	case string(model.RunStatusCompleted):
		return 1
	default:
		return 0
	}
}

func branchHasBusyStatus(nodes []runGraphNodeView) bool {
	for _, node := range nodes {
		switch node.Status {
		case string(model.RunStatusNeedsApproval), string(model.RunStatusActive), string(model.RunStatusPending):
			return true
		}
	}
	return false
}

func isSettledGraphStatus(status string) bool {
	switch status {
	case string(model.RunStatusCompleted), string(model.RunStatusFailed), string(model.RunStatusInterrupted):
		return true
	default:
		return false
	}
}

func statusFromClass(className string) string {
	switch className {
	case "is-pending":
		return string(model.RunStatusPending)
	case "is-active":
		return string(model.RunStatusActive)
	case "is-approval":
		return string(model.RunStatusNeedsApproval)
	case "is-success":
		return string(model.RunStatusCompleted)
	case "is-error":
		return string(model.RunStatusFailed)
	case "is-muted":
		return string(model.RunStatusInterrupted)
	default:
		return ""
	}
}
