package web

import (
	"net/http"
	"net/url"
	"sort"

	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/sessions"
)

type instancesResponse struct {
	Summary    instancesSummaryResponse     `json:"summary"`
	Lanes      []instancesLaneResponse      `json:"lanes"`
	Connectors []instancesConnectorResponse `json:"connectors"`
	Sources    instancesSourcesResponse     `json:"sources"`
}

type instancesSummaryResponse struct {
	FrontLaneCount       int `json:"front_lane_count"`
	SpecialistLaneCount  int `json:"specialist_lane_count"`
	LiveConnectorCount   int `json:"live_connector_count"`
	PendingDeliveryCount int `json:"pending_delivery_count"`
}

type instancesLaneResponse struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	AgentID           string `json:"agent_id"`
	Objective         string `json:"objective"`
	Status            string `json:"status"`
	StatusLabel       string `json:"status_label"`
	StatusClass       string `json:"status_class"`
	ModelDisplay      string `json:"model_display"`
	TokenSummary      string `json:"token_summary"`
	LastActivityShort string `json:"last_activity_short"`
}

type instancesConnectorResponse struct {
	ConnectorID      string `json:"connector_id"`
	State            string `json:"state"`
	StateLabel       string `json:"state_label"`
	StateClass       string `json:"state_class"`
	Summary          string `json:"summary"`
	CheckedAtLabel   string `json:"checked_at_label,omitempty"`
	RestartSuggested bool   `json:"restart_suggested"`
	PendingCount     int    `json:"pending_count"`
	RetryingCount    int    `json:"retrying_count"`
	TerminalCount    int    `json:"terminal_count"`
}

type instancesSourcesResponse struct {
	QueueHeadline      string `json:"queue_headline"`
	RootRuns           int    `json:"root_runs"`
	ActiveRuns         int    `json:"active_runs"`
	NeedsApprovalRuns  int    `json:"needs_approval_runs"`
	SessionCount       int    `json:"session_count"`
	ConnectorCount     int    `json:"connector_count"`
	TerminalDeliveries int    `json:"terminal_deliveries"`
}

func (s *Server) handleInstancesStatus(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	activeProject, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load active project", http.StatusInternalServerError)
		return
	}

	runs, err := s.loadRunsPageData(r.Context(), url.Values{})
	if err != nil {
		http.Error(w, "failed to load instances work lanes", http.StatusInternalServerError)
		return
	}

	sessionPage, err := s.rt.ListAllSessionsPage(r.Context(), sessions.SessionListFilter{
		ProjectID: activeProject.ID,
		Limit:     100,
	})
	if err != nil {
		http.Error(w, "failed to load sessions", http.StatusInternalServerError)
		return
	}

	health, err := s.rt.ConnectorDeliveryHealth(r.Context())
	if err != nil {
		http.Error(w, "failed to load connector delivery health", http.StatusInternalServerError)
		return
	}

	runtimeHealth, err := s.loadRuntimeConnectorHealth(r.Context(), routesDeliveriesPageFilters{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	lanes := buildInstancesLaneResponses(runs.Clusters)
	connectors := buildInstancesConnectorResponses(
		buildRoutesDeliveriesRuntimeHealthViews(runtimeHealth),
		buildRoutesDeliveriesDeliveryHealthViews(health),
	)

	writeJSON(w, http.StatusOK, instancesResponse{
		Summary: instancesSummaryResponse{
			FrontLaneCount:       countInstancesLanesByKind(lanes, "front"),
			SpecialistLaneCount:  countInstancesLanesByKind(lanes, "specialist"),
			LiveConnectorCount:   countActiveInstanceConnectors(connectors),
			PendingDeliveryCount: countPendingInstanceDeliveries(connectors),
		},
		Lanes:      lanes,
		Connectors: connectors,
		Sources: instancesSourcesResponse{
			QueueHeadline:      runs.QueueStrip.Headline,
			RootRuns:           runs.QueueStrip.RootRuns,
			ActiveRuns:         runs.QueueStrip.Summary.Active,
			NeedsApprovalRuns:  runs.QueueStrip.Summary.NeedsApproval,
			SessionCount:       len(sessionPage.Items),
			ConnectorCount:     len(connectors),
			TerminalDeliveries: countTerminalDeliveries(health),
		},
	})
}

func buildInstancesLaneResponses(clusters []runListClusterView) []instancesLaneResponse {
	lanes := make([]instancesLaneResponse, 0, len(clusters))
	for _, cluster := range clusters {
		lanes = appendInstanceLane(lanes, cluster.Root)
		for _, child := range cluster.Children {
			lanes = appendInstanceLane(lanes, child)
		}
	}

	sort.Slice(lanes, func(i, j int) bool {
		left := lanes[i]
		right := lanes[j]

		kindDelta := instancesLaneKindRank(left.Kind) - instancesLaneKindRank(right.Kind)
		if kindDelta != 0 {
			return kindDelta < 0
		}

		statusDelta := instancesLaneStatusRank(right.Status) - instancesLaneStatusRank(left.Status)
		if statusDelta != 0 {
			return statusDelta < 0
		}

		return left.AgentID < right.AgentID
	})

	return lanes
}

func appendInstanceLane(lanes []instancesLaneResponse, item runListItem) []instancesLaneResponse {
	if !isInstancePresenceStatus(item.Status) {
		return lanes
	}

	kind := "specialist"
	if item.Depth == 0 {
		kind = "front"
	}

	return append(lanes, instancesLaneResponse{
		ID:                item.ID,
		Kind:              kind,
		AgentID:           item.AgentID,
		Objective:         item.Objective,
		Status:            item.Status,
		StatusLabel:       item.StatusLabel,
		StatusClass:       item.StatusClass,
		ModelDisplay:      item.ModelDisplay,
		TokenSummary:      item.TokenSummary,
		LastActivityShort: item.LastActivityShort,
	})
}

func isInstancePresenceStatus(status string) bool {
	return status == "active" || status == "pending" || status == "needs_approval"
}

func instancesLaneKindRank(kind string) int {
	if kind == "front" {
		return 0
	}
	return 1
}

func instancesLaneStatusRank(status string) int {
	switch status {
	case "active":
		return 3
	case "needs_approval":
		return 2
	case "pending":
		return 1
	default:
		return 0
	}
}

func buildInstancesConnectorResponses(runtimeItems []routesDeliveriesRuntimeHealthView, queueItems []routesDeliveriesDeliveryHealthView) []instancesConnectorResponse {
	runtimeByID := make(map[string]routesDeliveriesRuntimeHealthView, len(runtimeItems))
	for _, item := range runtimeItems {
		runtimeByID[item.ConnectorID] = item
	}

	queueByID := make(map[string]routesDeliveriesDeliveryHealthView, len(queueItems))
	for _, item := range queueItems {
		queueByID[item.ConnectorID] = item
	}

	connectorIDs := make(map[string]struct{}, len(runtimeItems)+len(queueItems))
	for _, item := range runtimeItems {
		connectorIDs[item.ConnectorID] = struct{}{}
	}
	for _, item := range queueItems {
		connectorIDs[item.ConnectorID] = struct{}{}
	}

	items := make([]instancesConnectorResponse, 0, len(connectorIDs))
	for connectorID := range connectorIDs {
		runtimeItem, hasRuntime := runtimeByID[connectorID]
		queueItem, hasQueue := queueByID[connectorID]

		state := "unknown"
		stateLabel := "Unknown"
		stateClass := "is-muted"
		summary := "No runtime snapshot yet."
		checkedAtLabel := ""
		restartSuggested := false
		if hasRuntime {
			state = runtimeItem.State
			stateLabel = runtimeItem.StateLabel
			stateClass = runtimeItem.StateClass
			summary = runtimeItem.Summary
			checkedAtLabel = runtimeItem.CheckedAtLabel
			restartSuggested = runtimeItem.RestartSuggested
		} else if hasQueue && queueItem.StateClass != "" {
			stateClass = queueItem.StateClass
		}

		item := instancesConnectorResponse{
			ConnectorID:      connectorID,
			State:            state,
			StateLabel:       stateLabel,
			StateClass:       stateClass,
			Summary:          summary,
			CheckedAtLabel:   checkedAtLabel,
			RestartSuggested: restartSuggested,
		}
		if hasQueue {
			item.PendingCount = queueItem.PendingCount
			item.RetryingCount = queueItem.RetryingCount
			item.TerminalCount = queueItem.TerminalCount
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]

		issueDelta := instanceConnectorIssueRank(right) - instanceConnectorIssueRank(left)
		if issueDelta != 0 {
			return issueDelta < 0
		}

		activityDelta := boolToInt(instanceConnectorIsActive(right)) - boolToInt(instanceConnectorIsActive(left))
		if activityDelta != 0 {
			return activityDelta < 0
		}

		return left.ConnectorID < right.ConnectorID
	})

	return items
}

func countInstancesLanesByKind(lanes []instancesLaneResponse, kind string) int {
	total := 0
	for _, lane := range lanes {
		if lane.Kind == kind {
			total++
		}
	}
	return total
}

func countActiveInstanceConnectors(items []instancesConnectorResponse) int {
	total := 0
	for _, item := range items {
		if instanceConnectorIsActive(item) {
			total++
		}
	}
	return total
}

func countPendingInstanceDeliveries(items []instancesConnectorResponse) int {
	total := 0
	for _, item := range items {
		total += item.PendingCount
	}
	return total
}

func instanceConnectorIsActive(item instancesConnectorResponse) bool {
	return item.State == "active" || item.StateClass == "is-success" || item.StateClass == "is-active"
}

func instanceConnectorIssueRank(item instancesConnectorResponse) int {
	rank := 0
	if item.RestartSuggested {
		rank += 8
	}
	if item.TerminalCount > 0 {
		rank += 4
	}
	if item.RetryingCount > 0 {
		rank += 2
	}
	if item.PendingCount > 0 {
		rank += 1
	}
	if item.StateClass == "is-error" {
		rank += 4
	}
	return rank
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
