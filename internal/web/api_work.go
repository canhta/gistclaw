package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type workIndexResponse struct {
	ActiveProjectName string                 `json:"active_project_name"`
	ActiveProjectPath string                 `json:"active_project_path"`
	QueueStrip        workQueueStripResponse `json:"queue_strip"`
	Clusters          []workClusterResponse  `json:"clusters"`
}

type workQueueStripResponse struct {
	Headline     string              `json:"headline"`
	RootRuns     int                 `json:"root_runs"`
	WorkerRuns   int                 `json:"worker_runs"`
	RecoveryRuns int                 `json:"recovery_runs"`
	Summary      runGraphSummaryView `json:"summary"`
}

type workClusterResponse struct {
	Root            workClusterRunResponse   `json:"root"`
	Children        []workClusterRunResponse `json:"children,omitempty"`
	ChildCount      int                      `json:"child_count"`
	ChildCountLabel string                   `json:"child_count_label"`
	BlockerLabel    string                   `json:"blocker_label"`
	HasChildren     bool                     `json:"has_children"`
}

type workClusterRunResponse struct {
	ID                string `json:"id"`
	Objective         string `json:"objective"`
	AgentID           string `json:"agent_id"`
	Status            string `json:"status"`
	StatusLabel       string `json:"status_label"`
	StatusClass       string `json:"status_class"`
	ModelDisplay      string `json:"model_display"`
	TokenSummary      string `json:"token_summary"`
	StartedAtShort    string `json:"started_at_short"`
	StartedAtExact    string `json:"started_at_exact"`
	StartedAtISO      string `json:"started_at_iso"`
	LastActivityShort string `json:"last_activity_short"`
	LastActivityExact string `json:"last_activity_exact"`
	LastActivityISO   string `json:"last_activity_iso"`
	Depth             int    `json:"depth"`
}

type workDetailResponse struct {
	Run           workRunDetailResponse     `json:"run"`
	Graph         runGraphView              `json:"graph"`
	InspectorSeed *workInspectorSeedReponse `json:"inspector_seed,omitempty"`
}

type workRunDetailResponse struct {
	ID                    string `json:"id"`
	ShortID               string `json:"short_id"`
	ObjectiveText         string `json:"objective_text"`
	TriggerLabel          string `json:"trigger_label"`
	Status                string `json:"status"`
	StatusLabel           string `json:"status_label"`
	StatusClass           string `json:"status_class"`
	StateLabel            string `json:"state_label"`
	StartedAtLabel        string `json:"started_at_label"`
	LastActivityLabel     string `json:"last_activity_label"`
	ModelDisplay          string `json:"model_display"`
	TokenSummary          string `json:"token_summary"`
	EventCount            int    `json:"event_count"`
	TurnCount             int    `json:"turn_count"`
	StreamURL             string `json:"stream_url"`
	GraphURL              string `json:"graph_url"`
	NodeDetailURLTemplate string `json:"node_detail_url_template"`
	Dismissible           bool   `json:"dismissible"`
	DismissURL            string `json:"dismiss_url,omitempty"`
}

type workInspectorSeedReponse struct {
	ID      string `json:"id"`
	AgentID string `json:"agent_id"`
	Status  string `json:"status"`
}

type workCreateRequest struct {
	Task string `json:"task"`
}

type workCreateResponse struct {
	RunID     string `json:"run_id"`
	Objective string `json:"objective"`
}

type workDismissResponse struct {
	Dismissed bool   `json:"dismissed"`
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
	NextHref  string `json:"next_href"`
}

func (s *Server) handleWorkIndex(w http.ResponseWriter, r *http.Request) {
	pageData, err := s.loadRunsPageData(r.Context(), r.URL.Query())
	if err != nil {
		http.Error(w, "failed to load work queue", http.StatusInternalServerError)
		return
	}

	resp := workIndexResponse{
		ActiveProjectName: pageData.ActiveProjectName,
		ActiveProjectPath: pageData.ActiveProjectPath,
		QueueStrip:        buildWorkQueueStrip(pageData.Clusters),
		Clusters:          make([]workClusterResponse, 0, len(pageData.Clusters)),
	}
	for _, cluster := range pageData.Clusters {
		resp.Clusters = append(resp.Clusters, buildWorkClusterResponse(cluster))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleWorkDetail(w http.ResponseWriter, r *http.Request) {
	pageData, err := s.loadRunDetailPageData(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to load work detail", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, workDetailResponse{
		Run: workRunDetailResponse{
			ID:                    pageData.RunID,
			ShortID:               pageData.RunShortID,
			ObjectiveText:         pageData.ObjectiveText,
			TriggerLabel:          pageData.TriggerLabel,
			Status:                pageData.Status,
			StatusLabel:           pageData.StatusLabel,
			StatusClass:           pageData.StatusClass,
			StateLabel:            pageData.StateLabel,
			StartedAtLabel:        pageData.StartedAtLabel,
			LastActivityLabel:     pageData.LastActivityLabel,
			ModelDisplay:          pageData.ModelDisplay,
			TokenSummary:          pageData.TokenSummary,
			EventCount:            pageData.EventCount,
			TurnCount:             pageData.TurnCount,
			StreamURL:             workEventsPath(pageData.RunID),
			GraphURL:              workGraphPath(pageData.RunID),
			NodeDetailURLTemplate: workNodeDetailTemplatePath(pageData.RunID),
			Dismissible:           pageData.Status == "interrupted",
			DismissURL:            workDismissPath(pageData.RunID, pageData.Status),
		},
		Graph:         pageData.Graph,
		InspectorSeed: buildWorkInspectorSeed(pageData.Graph),
	})
}

func (s *Server) handleWorkCreate(w http.ResponseWriter, r *http.Request) {
	var req workCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	task := strings.TrimSpace(req.Task)
	if task == "" {
		http.Error(w, "task is required", http.StatusBadRequest)
		return
	}

	runID, err := s.startWorkRun(r.Context(), task)
	if err != nil {
		http.Error(w, "failed to start run: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusAccepted, workCreateResponse{
		RunID:     runID,
		Objective: task,
	})
}

func (s *Server) handleWorkDismiss(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	if err := s.dismissRun(r.Context(), runID); err != nil {
		http.Error(w, "failed to dismiss run", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, workDismissResponse{
		Dismissed: true,
		RunID:     runID,
		Status:    "dismissed",
		NextHref:  pageWork,
	})
}

func buildWorkClusterResponse(cluster runListClusterView) workClusterResponse {
	resp := workClusterResponse{
		Root:            buildWorkClusterRunResponse(cluster.Root),
		Children:        make([]workClusterRunResponse, 0, len(cluster.Children)),
		ChildCount:      cluster.ChildCount,
		ChildCountLabel: cluster.ChildCountLabel,
		BlockerLabel:    cluster.BlockerLabel,
		HasChildren:     cluster.HasChildren,
	}
	for _, child := range cluster.Children {
		resp.Children = append(resp.Children, buildWorkClusterRunResponse(child))
	}
	return resp
}

func buildWorkQueueStrip(clusters []runListClusterView) workQueueStripResponse {
	resp := workQueueStripResponse{}
	for _, cluster := range clusters {
		resp.RootRuns++
		resp.WorkerRuns += len(cluster.Children)
		resp.Summary.Total++
		switch cluster.Root.Status {
		case "pending":
			resp.Summary.Pending++
		case "active":
			resp.Summary.Active++
		case "needs_approval":
			resp.Summary.NeedsApproval++
			resp.RecoveryRuns++
		case "completed":
			resp.Summary.Completed++
		case "failed":
			resp.Summary.Failed++
			resp.RecoveryRuns++
		case "interrupted":
			resp.Summary.Interrupted++
			resp.RecoveryRuns++
		}
	}

	switch {
	case resp.Summary.Total == 0:
		resp.Headline = "No active work yet. Start a task to see progress here."
	case resp.RecoveryRuns > 0:
		resp.Headline = "Some work is waiting on you."
	case resp.Summary.Active > 0 || resp.Summary.Pending > 0:
		resp.Headline = "See what is running, waiting on you, or done."
	default:
		resp.Headline = "Recent work is settled."
	}

	return resp
}

func buildWorkClusterRunResponse(item runListItem) workClusterRunResponse {
	return workClusterRunResponse{
		ID:                item.ID,
		Objective:         item.Objective,
		AgentID:           item.AgentID,
		Status:            item.Status,
		StatusLabel:       item.StatusLabel,
		StatusClass:       item.StatusClass,
		ModelDisplay:      item.ModelDisplay,
		TokenSummary:      item.TokenSummary,
		StartedAtShort:    item.StartedAtShort,
		StartedAtExact:    item.StartedAtExact,
		StartedAtISO:      item.StartedAtISO,
		LastActivityShort: item.LastActivityShort,
		LastActivityExact: item.LastActivityExact,
		LastActivityISO:   item.LastActivityISO,
		Depth:             item.Depth,
	}
}

func buildWorkInspectorSeed(graph runGraphView) *workInspectorSeedReponse {
	if len(graph.Nodes) == 0 {
		return nil
	}

	nodeID := graph.RootRunID
	if len(graph.ActivePath) > 0 {
		nodeID = graph.ActivePath[len(graph.ActivePath)-1]
	}

	for _, node := range graph.Nodes {
		if node.ID != nodeID {
			continue
		}
		return &workInspectorSeedReponse{
			ID:      node.ID,
			AgentID: node.AgentID,
			Status:  node.Status,
		}
	}

	root := graph.Nodes[0]
	return &workInspectorSeedReponse{
		ID:      root.ID,
		AgentID: root.AgentID,
		Status:  root.Status,
	}
}
