package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

func TestWorkIndexReturnsQueueAndProjectSummary(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	h.insertRunAt(t, "run-work-root", "conv-work", "Review the repo", "active", "2026-03-25 10:00:00")
	if _, err := h.db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, project_id, parent_run_id, team_id, objective, cwd, status, created_at, updated_at)
		 VALUES
		 ('run-work-child', 'conv-work', 'patcher', ?, 'run-work-root', 'repo-task-team', ?, ?, 'needs_approval', '2026-03-25 10:05:00', '2026-03-25 10:07:00')`,
		h.activeProjectID,
		"Apply the patch",
		h.workspaceRoot,
	); err != nil {
		t.Fatalf("insert work child run: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/work", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ActiveProjectName string `json:"active_project_name"`
		ActiveProjectPath string `json:"active_project_path"`
		QueueStrip        struct {
			Headline     string `json:"headline"`
			RootRuns     int    `json:"root_runs"`
			WorkerRuns   int    `json:"worker_runs"`
			RecoveryRuns int    `json:"recovery_runs"`
			Summary      struct {
				NeedsApproval int `json:"needs_approval"`
			} `json:"summary"`
		} `json:"queue_strip"`
		Clusters []struct {
			Root struct {
				ID      string `json:"id"`
				Status  string `json:"status"`
				AgentID string `json:"agent_id"`
			} `json:"root"`
			ChildCount   int    `json:"child_count"`
			BlockerLabel string `json:"blocker_label"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.ActiveProjectName != "starter-project" {
		t.Fatalf("active_project_name = %q, want %q", resp.ActiveProjectName, "starter-project")
	}
	if resp.ActiveProjectPath != h.workspaceRoot {
		t.Fatalf("active_project_path = %q, want %q", resp.ActiveProjectPath, h.workspaceRoot)
	}
	if resp.QueueStrip.Headline != "Some work is waiting on you." {
		t.Fatalf("queue headline = %q", resp.QueueStrip.Headline)
	}
	if resp.QueueStrip.RootRuns != 1 || resp.QueueStrip.WorkerRuns != 1 || resp.QueueStrip.RecoveryRuns != 1 {
		t.Fatalf("unexpected queue strip counts: %+v", resp.QueueStrip)
	}
	if resp.QueueStrip.Summary.NeedsApproval != 1 {
		t.Fatalf("needs_approval = %d, want 1", resp.QueueStrip.Summary.NeedsApproval)
	}
	if len(resp.Clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(resp.Clusters))
	}
	if resp.Clusters[0].Root.ID != "run-work-root" || resp.Clusters[0].Root.Status != "needs_approval" {
		t.Fatalf("unexpected root cluster %+v", resp.Clusters[0].Root)
	}
	if resp.Clusters[0].ChildCount != 1 {
		t.Fatalf("child_count = %d, want 1", resp.Clusters[0].ChildCount)
	}
	if resp.Clusters[0].BlockerLabel != "patcher waiting on approval" {
		t.Fatalf("blocker_label = %q", resp.Clusters[0].BlockerLabel)
	}
}

func TestWorkDetailReturnsRunSummaryGraphAndInspectorSeed(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	h.insertRunWithSnapshotAt(t, "run-work-detail", "conv-work-detail", "Review the repo", "active", "2026-03-25 08:00:00", "2026-03-25 08:02:00", model.ExecutionSnapshot{
		TeamID:       "repo-task-team",
		FrontAgentID: "assistant",
		Agents: map[string]model.AgentProfile{
			"assistant": {
				AgentID:      "assistant",
				BaseProfile:  model.BaseProfileOperator,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyDelegate},
			},
			"researcher": {
				AgentID:      "researcher",
				BaseProfile:  model.BaseProfileResearch,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyWebRead},
			},
		},
	})
	if _, err := h.db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, session_id, project_id, team_id, parent_run_id, objective, cwd, status, created_at, updated_at)
		 VALUES (?, ?, 'researcher', 'sess-work-child', ?, 'repo-task-team', ?, ?, ?, 'needs_approval', '2026-03-25 08:03:00', '2026-03-25 08:05:00')`,
		"run-work-child",
		"conv-work-detail",
		h.activeProjectID,
		"run-work-detail",
		"Inspect docs",
		h.workspaceRoot,
	); err != nil {
		t.Fatalf("insert work child run: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/work/run-work-detail", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Run struct {
			ID             string `json:"id"`
			ShortID        string `json:"short_id"`
			ObjectiveText  string `json:"objective_text"`
			StateLabel     string `json:"state_label"`
			Status         string `json:"status"`
			StreamURL      string `json:"stream_url"`
			ModelDisplay   string `json:"model_display"`
			TokenSummary   string `json:"token_summary"`
			TriggerLabel   string `json:"trigger_label"`
			LastActivity   string `json:"last_activity_label"`
			StartedAtLabel string `json:"started_at_label"`
		} `json:"run"`
		Graph struct {
			RootRunID  string   `json:"root_run_id"`
			ActivePath []string `json:"active_path"`
			Nodes      []struct {
				ID       string `json:"id"`
				AgentID  string `json:"agent_id"`
				Status   string `json:"status"`
				IsActive bool   `json:"is_active_path"`
			} `json:"nodes"`
		} `json:"graph"`
		InspectorSeed *struct {
			ID      string `json:"id"`
			AgentID string `json:"agent_id"`
			Status  string `json:"status"`
		} `json:"inspector_seed"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Run.ID != "run-work-detail" {
		t.Fatalf("run.id = %q, want %q", resp.Run.ID, "run-work-detail")
	}
	if resp.Run.ObjectiveText != "Review the repo" {
		t.Fatalf("objective_text = %q, want %q", resp.Run.ObjectiveText, "Review the repo")
	}
	if resp.Run.StateLabel != "1 task waiting on you." {
		t.Fatalf("state_label = %q", resp.Run.StateLabel)
	}
	if resp.Run.StreamURL != "/api/work/run-work-detail/events" {
		t.Fatalf("stream_url = %q", resp.Run.StreamURL)
	}
	if resp.Graph.RootRunID != "run-work-detail" {
		t.Fatalf("graph.root_run_id = %q", resp.Graph.RootRunID)
	}
	if len(resp.Graph.Nodes) != 2 {
		t.Fatalf("expected 2 graph nodes, got %d", len(resp.Graph.Nodes))
	}
	if len(resp.Graph.ActivePath) != 2 || resp.Graph.ActivePath[1] != "run-work-child" {
		t.Fatalf("unexpected active_path %+v", resp.Graph.ActivePath)
	}
	if resp.InspectorSeed == nil {
		t.Fatal("expected inspector_seed")
	}
	if resp.InspectorSeed.ID != "run-work-child" || resp.InspectorSeed.AgentID != "researcher" {
		t.Fatalf("unexpected inspector_seed %+v", resp.InspectorSeed)
	}
}

func TestCreateWorkTaskStartsRunAndReturnsRunID(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)

	body := bytes.NewBufferString(`{"task":"review the repo"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/work", body)
	req.Header.Set("Content-Type", "application/json")
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		RunID     string `json:"run_id"`
		Objective string `json:"objective"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if strings.TrimSpace(resp.RunID) == "" {
		t.Fatal("expected run_id in response")
	}
	if resp.Objective != "review the repo" {
		t.Fatalf("objective = %q, want %q", resp.Objective, "review the repo")
	}

	var storedObjective string
	if err := h.db.RawDB().QueryRow(`SELECT objective FROM runs WHERE id = ?`, resp.RunID).Scan(&storedObjective); err != nil {
		t.Fatalf("load created run: %v", err)
	}
	if storedObjective != "review the repo" {
		t.Fatalf("stored objective = %q, want %q", storedObjective, "review the repo")
	}
}

func TestCreateWorkTaskReturnsBeforeProviderCompletes(t *testing.T) {
	t.Parallel()

	prov := &blockingProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	h := newServerHarnessWithProvider(t, prov)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/work", bytes.NewBufferString(`{"task":"review the repo"}`))
	req.Header.Set("Content-Type", "application/json")

	done := make(chan struct{})
	go func() {
		h.server.ServeHTTP(rr, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		close(prov.release)
		<-done
		t.Fatal("expected /api/work to return before the provider completes")
	}

	if rr.Code != http.StatusAccepted {
		close(prov.release)
		t.Fatalf("expected 202, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		close(prov.release)
		t.Fatalf("decode response: %v", err)
	}
	if strings.TrimSpace(resp.RunID) == "" {
		close(prov.release)
		t.Fatal("expected run_id in response")
	}

	select {
	case <-prov.started:
	case <-time.After(time.Second):
		close(prov.release)
		t.Fatal("expected provider work to continue in the background")
	}

	var status string
	if err := h.db.RawDB().QueryRow("SELECT status FROM runs WHERE id = ?", resp.RunID).Scan(&status); err != nil {
		close(prov.release)
		t.Fatalf("query run status: %v", err)
	}
	if status != "active" {
		close(prov.release)
		t.Fatalf("expected background run to stay active while provider is blocked, got %q", status)
	}

	close(prov.release)
	waitForRunStatus(t, h.db, resp.RunID, "completed")
}

func TestWorkDetailReturnsNotFoundForUnknownRun(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/work/missing-run", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateWorkTaskRejectsInvalidBodies(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)

	t.Run("invalid json", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/work", bytes.NewBufferString(`{`))
		req.Header.Set("Content-Type", "application/json")
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("blank task", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/work", bytes.NewBufferString(`{"task":"   "}`))
		req.Header.Set("Content-Type", "application/json")
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
	})
}

func TestDismissWorkMarksInterruptedRunDismissed(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	h.insertRun(t, "run-work-dismiss", "conv-work-dismiss", "Review the repo", "interrupted")

	dismissRR := httptest.NewRecorder()
	dismissReq := httptest.NewRequest(http.MethodPost, "/api/work/run-work-dismiss/dismiss", nil)
	h.server.ServeHTTP(dismissRR, dismissReq)

	if dismissRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", dismissRR.Code, dismissRR.Body.String())
	}

	var dismissResp struct {
		Dismissed bool   `json:"dismissed"`
		RunID     string `json:"run_id"`
		Status    string `json:"status"`
		NextHref  string `json:"next_href"`
	}
	if err := json.Unmarshal(dismissRR.Body.Bytes(), &dismissResp); err != nil {
		t.Fatalf("decode dismiss response: %v", err)
	}
	if !dismissResp.Dismissed || dismissResp.RunID != "run-work-dismiss" || dismissResp.Status != "dismissed" {
		t.Fatalf("unexpected dismiss response %+v", dismissResp)
	}
	if dismissResp.NextHref != "/work" {
		t.Fatalf("next_href = %q, want %q", dismissResp.NextHref, "/work")
	}

	indexRR := httptest.NewRecorder()
	indexReq := httptest.NewRequest(http.MethodGet, "/api/work", nil)
	h.server.ServeHTTP(indexRR, indexReq)

	if indexRR.Code != http.StatusOK {
		t.Fatalf("expected work index 200, got %d body=%s", indexRR.Code, indexRR.Body.String())
	}

	var indexResp struct {
		Clusters []struct {
			Root struct {
				ID string `json:"id"`
			} `json:"root"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal(indexRR.Body.Bytes(), &indexResp); err != nil {
		t.Fatalf("decode work index: %v", err)
	}
	for _, cluster := range indexResp.Clusters {
		if cluster.Root.ID == "run-work-dismiss" {
			t.Fatalf("expected dismissed run to be absent from work index, got %+v", indexResp.Clusters)
		}
	}

	detailRR := httptest.NewRecorder()
	detailReq := httptest.NewRequest(http.MethodGet, "/api/work/run-work-dismiss", nil)
	h.server.ServeHTTP(detailRR, detailReq)

	if detailRR.Code != http.StatusOK {
		t.Fatalf("expected work detail 200, got %d body=%s", detailRR.Code, detailRR.Body.String())
	}

	var detailResp struct {
		Run struct {
			ID          string `json:"id"`
			Status      string `json:"status"`
			Dismissible bool   `json:"dismissible"`
			DismissURL  string `json:"dismiss_url"`
		} `json:"run"`
	}
	if err := json.Unmarshal(detailRR.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("decode work detail: %v", err)
	}
	if detailResp.Run.ID != "run-work-dismiss" || detailResp.Run.Status != "dismissed" {
		t.Fatalf("unexpected dismissed run detail %+v", detailResp.Run)
	}
	if detailResp.Run.Dismissible {
		t.Fatalf("expected dismissed run to stop exposing dismiss action, got %+v", detailResp.Run)
	}
	if detailResp.Run.DismissURL != "" {
		t.Fatalf("expected dismissed run to omit dismiss_url, got %q", detailResp.Run.DismissURL)
	}
}

func TestBuildWorkQueueStripAndInspectorSeedFallbacks(t *testing.T) {
	t.Parallel()

	empty := buildWorkQueueStrip(nil)
	if empty.Headline != "No active work yet. Start a task to see progress here." {
		t.Fatalf("empty headline = %q", empty.Headline)
	}

	active := buildWorkQueueStrip([]runListClusterView{
		{
			Root: runListItem{Status: "active"},
		},
	})
	if active.Headline != "See what is running, waiting on you, or done." {
		t.Fatalf("active headline = %q", active.Headline)
	}
	if active.Summary.Active != 1 || active.RootRuns != 1 {
		t.Fatalf("unexpected active summary %+v", active)
	}

	settled := buildWorkQueueStrip([]runListClusterView{
		{
			Root: runListItem{Status: "completed"},
		},
	})
	if settled.Headline != "Recent work is settled." {
		t.Fatalf("settled headline = %q", settled.Headline)
	}
	if settled.Summary.Completed != 1 {
		t.Fatalf("unexpected settled summary %+v", settled)
	}

	seed := buildWorkInspectorSeed(runGraphView{
		RootRunID:  "root",
		ActivePath: []string{"missing-node"},
		Nodes: []runGraphNodeView{
			{ID: "root", AgentID: "assistant", Status: "active"},
			{ID: "child", AgentID: "patcher", Status: "completed"},
		},
	})
	if seed == nil {
		t.Fatal("expected inspector seed")
	}
	if seed.ID != "root" || seed.AgentID != "assistant" {
		t.Fatalf("unexpected fallback inspector seed %+v", seed)
	}
}
