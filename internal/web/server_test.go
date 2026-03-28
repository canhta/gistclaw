package web

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	authpkg "github.com/canhta/gistclaw/internal/auth"
	"github.com/canhta/gistclaw/internal/authority"
	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/replay"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/sessions"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/teams"
	"github.com/canhta/gistclaw/internal/tools"
)

func TestRuns(t *testing.T) {
	t.Run("detail browser api hides non-active project run", func(t *testing.T) {
		h := newServerHarness(t)
		otherRoot := t.TempDir()
		h.insertProject(t, "seo-test", otherRoot)
		h.insertRunInWorkspace(t, "run-other-project-detail", "conv-other-project-detail", "review seo project", "active", otherRoot)
		h.insertEventAt(t, "evt-other-project-detail", "conv-other-project-detail", "run-other-project-detail", "run_started", "2026-03-25 08:00:00")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/work/run-other-project-detail", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for non-active project run detail, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("graph endpoint renders topology snapshot with semantic edges", func(t *testing.T) {
		h := newServerHarness(t)
		h.insertRunWithSnapshotAt(t, "082b1c314823744cc779ece2f90e80e7", "conv-graph", "review the repo", "active", "2026-03-25 08:00:00", "2026-03-25 08:02:00", model.ExecutionSnapshot{
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
			 VALUES (?, ?, 'researcher', 'sess-worker-4ed077c29497f4c95a19125b86096953', ?, 'repo-task-team', ?, ?, ?, ?, ?, ?)`,
			"4ed077c29497f4c95a19125b86096953",
			"conv-graph",
			h.activeProjectID,
			"082b1c314823744cc779ece2f90e80e7",
			"Inspect docs.",
			h.workspaceRoot,
			"needs_approval",
			"2026-03-25 08:03:00",
			"2026-03-25 08:05:00",
		); err != nil {
			t.Fatalf("insert worker run: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/work/082b1c314823744cc779ece2f90e80e7/graph", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			RootRunID string `json:"root_run_id"`
			Summary   struct {
				Total         int `json:"total"`
				Active        int `json:"active"`
				NeedsApproval int `json:"needs_approval"`
			} `json:"summary"`
			Edges []struct {
				From  string `json:"from"`
				To    string `json:"to"`
				Kind  string `json:"kind"`
				Label string `json:"label"`
			} `json:"edges"`
			Lanes []struct {
				ID       string `json:"id"`
				Branches []struct {
					RootNodeID string `json:"root_node_id"`
				} `json:"branches"`
			} `json:"lanes"`
			Nodes []struct {
				ID             string `json:"id"`
				ShortID        string `json:"short_id"`
				Kind           string `json:"kind"`
				LaneID         string `json:"lane_id"`
				Status         string `json:"status"`
				StatusClass    string `json:"status_class"`
				StatusLabel    string `json:"status_label"`
				UpdatedAtLabel string `json:"updated_at_label"`
			} `json:"nodes"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal graph response: %v", err)
		}
		if resp.RootRunID != "082b1c314823744cc779ece2f90e80e7" {
			t.Fatalf("expected root run id 082b1c314823744cc779ece2f90e80e7, got %q", resp.RootRunID)
		}
		if resp.Summary.Total != 2 || resp.Summary.Active != 1 || resp.Summary.NeedsApproval != 1 {
			t.Fatalf("unexpected graph summary: %+v", resp.Summary)
		}
		if len(resp.Nodes) != 2 {
			t.Fatalf("expected 2 graph nodes, got %d", len(resp.Nodes))
		}
		if len(resp.Lanes) != 2 || resp.Lanes[0].ID != "coordination" || resp.Lanes[1].ID != "research" {
			t.Fatalf("expected coordination and research lanes, got %+v", resp.Lanes)
		}
		if resp.Nodes[0].ID != "082b1c314823744cc779ece2f90e80e7" || resp.Nodes[1].ID != "4ed077c29497f4c95a19125b86096953" {
			t.Fatalf("unexpected graph nodes: %+v", resp.Nodes)
		}
		if len(resp.Edges) != 1 || resp.Edges[0].From != "082b1c314823744cc779ece2f90e80e7" || resp.Edges[0].To != "4ed077c29497f4c95a19125b86096953" {
			t.Fatalf("unexpected graph edges: %+v", resp.Edges)
		}
		if resp.Edges[0].Kind != "blocked" || resp.Edges[0].Label != "approve" {
			t.Fatalf("expected semantic blocked edge, got %+v", resp.Edges[0])
		}
		if resp.Nodes[1].StatusClass != "is-approval" {
			t.Fatalf("expected approval status class for worker node, got %+v", resp.Nodes[1])
		}
		if resp.Nodes[1].Kind != "worker" || resp.Nodes[1].LaneID != "research" {
			t.Fatalf("expected worker node metadata, got %+v", resp.Nodes[1])
		}
		if resp.Nodes[0].ShortID != "082b1c31…80e7" {
			t.Fatalf("expected short root id, got %+v", resp.Nodes[0])
		}
		if resp.Nodes[1].StatusLabel != "needs approval" {
			t.Fatalf("expected humanized status label, got %+v", resp.Nodes[1])
		}
		if resp.Nodes[1].UpdatedAtLabel != "2026-03-25 08:05:00 UTC" {
			t.Fatalf("expected updated-at label, got %+v", resp.Nodes[1])
		}
	})

	t.Run("node detail endpoint returns modal data and codex logs", func(t *testing.T) {
		h := newServerHarness(t)
		h.insertRunAt(t, "run-root-node", "conv-node", "Coordinate the launch", "active", "2026-03-25 08:00:00")
		if _, err := h.db.RawDB().Exec(
			`INSERT INTO runs
			 (id, conversation_id, agent_id, project_id, parent_run_id, session_id, team_id, objective, cwd, status, model_lane, model_id, input_tokens, output_tokens, created_at, updated_at)
			 VALUES
			 ('run-child-node', 'conv-node', 'patcher', ?, 'run-root-node', 'sess-child-node', 'repo-task-team', ?, ?, 'completed', 'build', 'gpt-5.4', 2730, 183, '2026-03-25 08:05:00', '2026-03-25 08:09:00')`,
			h.activeProjectID,
			"1. Create the launch page\n2. Refine the copy\n3. Ship the result",
			h.workspaceRoot,
		); err != nil {
			t.Fatalf("insert child node run: %v", err)
		}
		h.insertEventAtWithPayload(
			t,
			"evt-child-turn",
			"conv-node",
			"run-child-node",
			"turn_completed",
			[]byte(`{"content":"Created index.html\n\n- Added FAQ\n- Added comparison table"}`),
			"2026-03-25 08:08:00",
		)
		h.insertEventAtWithPayload(
			t,
			"evt-tool-log-node",
			"conv-node",
			"run-child-node",
			"tool_log_recorded",
			[]byte(`{"tool_call_id":"call-coder","tool_name":"coder_exec","stream":"stdout","text":"Planning files\n"}`),
			"2026-03-25 08:07:30",
		)
		toolOutput := []byte(`{"Output":"{\"backend\":\"codex\",\"command\":\"codex exec --sandbox workspace-write\",\"cwd\":\"/Users/canh/Desktop\",\"stdout\":\"Planning files\\nWriting index.html\\nDone\",\"stderr\":\"warning: dry run disabled\",\"exit_code\":0}","Error":""}`)
		if _, err := h.db.RawDB().Exec(
			`INSERT INTO tool_calls
			 (id, run_id, tool_name, input_json, output_json, decision, approval_id, created_at)
			 VALUES
			 ('evt-tool-node', 'run-child-node', 'coder_exec', ?, ?, 'allow', '', '2026-03-25 08:07:00')`,
			[]byte(`{"backend":"codex","prompt":"Create the launch page"}`),
			toolOutput,
		); err != nil {
			t.Fatalf("insert tool call: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/work/run-root-node/nodes/run-child-node", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			ID                string `json:"id"`
			AgentID           string `json:"agent_id"`
			Status            string `json:"status"`
			ModelDisplay      string `json:"model_display"`
			TokenSummary      string `json:"token_summary"`
			StartedAtLabel    string `json:"started_at_label"`
			LastActivityLabel string `json:"last_activity_label"`
			Task              struct {
				PreviewText string `json:"preview_text"`
				HasOverflow bool   `json:"has_overflow"`
				HTML        string `json:"html"`
				Blocks      []struct {
					Kind  string   `json:"kind"`
					Text  string   `json:"text"`
					Items []string `json:"items"`
				} `json:"blocks"`
			} `json:"task"`
			Output struct {
				HTML   string `json:"html"`
				Blocks []struct {
					Kind  string   `json:"kind"`
					Text  string   `json:"text"`
					Items []string `json:"items"`
				} `json:"blocks"`
			} `json:"output"`
			Chain struct {
				Path []struct {
					RunID   string `json:"run_id"`
					AgentID string `json:"agent_id"`
				} `json:"path"`
			} `json:"chain"`
			Logs []struct {
				Title    string `json:"title"`
				Body     string `json:"body"`
				BodyHTML string `json:"body_html"`
				Stream   string `json:"stream"`
				ToolName string `json:"tool_name"`
			} `json:"logs"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal node detail: %v", err)
		}
		if resp.ID != "run-child-node" || resp.AgentID != "patcher" || resp.Status != "completed" {
			t.Fatalf("unexpected node detail identity: %+v", resp)
		}
		if resp.ModelDisplay != "gpt-5.4" {
			t.Fatalf("expected model display, got %+v", resp)
		}
		if resp.TokenSummary != "2.7K in / 183 out" {
			t.Fatalf("expected compact token summary, got %+v", resp)
		}
		if resp.StartedAtLabel != "2026-03-25 08:05:00 UTC" || resp.LastActivityLabel != "2026-03-25 08:09:00 UTC" {
			t.Fatalf("expected exact modal timestamps, got %+v", resp)
		}
		if resp.Task.PreviewText != "1. Create the launch page\n2. Refine the copy\n3. Ship the result" || resp.Task.HasOverflow {
			t.Fatalf("unexpected task preview: %+v", resp.Task)
		}
		if len(resp.Task.Blocks) != 1 || resp.Task.Blocks[0].Kind != "ordered_list" {
			t.Fatalf("expected ordered-list task blocks, got %+v", resp.Task.Blocks)
		}
		if !strings.Contains(resp.Task.HTML, "<ol") {
			t.Fatalf("expected rendered task HTML, got %+v", resp.Task)
		}
		if len(resp.Output.Blocks) != 2 || resp.Output.Blocks[1].Kind != "unordered_list" {
			t.Fatalf("expected formatted output blocks, got %+v", resp.Output.Blocks)
		}
		if !strings.Contains(resp.Output.HTML, "<ul") {
			t.Fatalf("expected rendered output HTML, got %+v", resp.Output)
		}
		if len(resp.Chain.Path) != 2 || resp.Chain.Path[0].RunID != "run-root-node" || resp.Chain.Path[1].RunID != "run-child-node" {
			t.Fatalf("expected root-to-node chain, got %+v", resp.Chain.Path)
		}
		if len(resp.Logs) < 3 {
			t.Fatalf("expected command logs, got %+v", resp.Logs)
		}
		if resp.Logs[0].ToolName != "coder_exec" || resp.Logs[0].Stream != "stdout" || !strings.Contains(resp.Logs[0].Body, "Planning files") {
			t.Fatalf("expected codex stdout log entry, got %+v", resp.Logs[0])
		}
		if !strings.Contains(resp.Logs[0].BodyHTML, "Planning files") {
			t.Fatalf("expected rendered log HTML, got %+v", resp.Logs[0])
		}
	})

	t.Run("node detail endpoint returns approval action metadata", func(t *testing.T) {
		h := newServerHarness(t)
		h.insertRunAt(t, "run-root-approval-node", "conv-node-approval", "Coordinate the launch", "active", "2026-03-25 08:00:00")
		if _, err := h.db.RawDB().Exec(
			`INSERT INTO runs
			 (id, conversation_id, agent_id, project_id, parent_run_id, session_id, team_id, objective, cwd, status, model_lane, model_id, input_tokens, output_tokens, created_at, updated_at)
			 VALUES
			 ('run-child-approval-node', 'conv-node-approval', 'patcher', ?, 'run-root-approval-node', 'sess-child-approval-node', 'repo-task-team', ?, ?, 'needs_approval', 'build', 'gpt-5.4', 2730, 183, '2026-03-25 08:05:00', '2026-03-25 08:09:00')`,
			h.activeProjectID,
			"Apply the launch page changes",
			h.workspaceRoot,
		); err != nil {
			t.Fatalf("insert approval child node run: %v", err)
		}
		approvalID := h.insertApprovalAt(t, "run-child-approval-node", "coder_exec", h.workspaceRoot+"/openclaw-launch-coder-final", "pending", "", "2026-03-25 08:07:00")
		h.insertEventAtWithPayload(
			t,
			"evt-child-approval-requested",
			"conv-node-approval",
			"run-child-approval-node",
			"approval_requested",
			[]byte(`{"approval_id":"`+approvalID+`","tool_call_id":"call-coder-approval","tool_name":"coder_exec","binding_json":{"tool_name":"coder_exec","operands":["`+h.workspaceRoot+`/openclaw-launch-coder-final"],"mutating":true},"reason":"Need confirmation before writing files into the project workspace."}`),
			"2026-03-25 08:07:00",
		)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/work/run-root-approval-node/nodes/run-child-approval-node", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			ID         string `json:"id"`
			SessionURL string `json:"session_url"`
			Approval   struct {
				ID               string `json:"id"`
				ToolName         string `json:"tool_name"`
				BindingSummary   string `json:"binding_summary"`
				Reason           string `json:"reason"`
				Status           string `json:"status"`
				StatusLabel      string `json:"status_label"`
				RequestedAtLabel string `json:"requested_at_label"`
				ResolveURL       string `json:"resolve_url"`
				CanResolve       bool   `json:"can_resolve"`
			} `json:"approval"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal node approval detail: %v", err)
		}
		if resp.ID != "run-child-approval-node" {
			t.Fatalf("unexpected node detail identity: %+v", resp)
		}
		if resp.SessionURL != "/conversations/sess-child-approval-node" {
			t.Fatalf("expected session detail URL, got %+v", resp)
		}
		if resp.Approval.ID != approvalID {
			t.Fatalf("expected approval id %q, got %+v", approvalID, resp.Approval)
		}
		if resp.Approval.ToolName != "coder_exec" || resp.Approval.Status != "pending" || !resp.Approval.CanResolve {
			t.Fatalf("expected actionable pending approval, got %+v", resp.Approval)
		}
		if resp.Approval.BindingSummary != h.workspaceRoot+"/openclaw-launch-coder-final" {
			t.Fatalf("expected binding summary, got %+v", resp.Approval)
		}
		if resp.Approval.Reason != "Need confirmation before writing files into the project workspace." {
			t.Fatalf("expected approval reason, got %+v", resp.Approval)
		}
		if resp.Approval.StatusLabel != "pending" {
			t.Fatalf("expected approval status label, got %+v", resp.Approval)
		}
		if resp.Approval.RequestedAtLabel != "2026-03-25 08:07:00 UTC" {
			t.Fatalf("expected requested-at label, got %+v", resp.Approval)
		}
		if resp.Approval.ResolveURL != "/api/recover/approvals/"+approvalID+"/resolve" {
			t.Fatalf("expected approval resolve URL, got %+v", resp.Approval)
		}
	})

	t.Run("detail missing run returns not found", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/work/missing", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rr.Code)
		}
	})
}

func TestSessionsProjectScoping(t *testing.T) {
	t.Run("conversations index hides other project sessions", func(t *testing.T) {
		h := newServerHarness(t)
		mainRun := h.startFrontSession(t, "review the main project")

		otherRoot := t.TempDir()
		h.insertProject(t, "seo-test", otherRoot)
		otherRun, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-other",
				ExternalID:  "chat-other",
				ThreadID:    "thread-other",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "review the seo project",
			CWD:           otherRoot,
		})
		if err != nil {
			t.Fatalf("start other-project front session: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/conversations", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Sessions []struct {
				ID             string `json:"id"`
				ConversationID string `json:"conversation_id"`
			} `json:"sessions"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode conversations index: %v", err)
		}

		foundMain := false
		for _, item := range resp.Sessions {
			if item.ID == mainRun.SessionID {
				foundMain = true
			}
			if item.ID == otherRun.SessionID || item.ConversationID == otherRun.ConversationID {
				t.Fatalf("expected other project session to be hidden, got %+v", resp.Sessions)
			}
		}
		if !foundMain {
			t.Fatalf("expected active project session to be visible, got %+v", resp.Sessions)
		}
	})

	t.Run("conversation detail hides other project session", func(t *testing.T) {
		h := newServerHarness(t)
		otherRoot := t.TempDir()
		h.insertProject(t, "seo-test", otherRoot)
		otherRun, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-other",
				ExternalID:  "chat-other",
				ThreadID:    "thread-other",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "review the seo project",
			CWD:           otherRoot,
		})
		if err != nil {
			t.Fatalf("start other-project front session: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/conversations/"+otherRun.SessionID, nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for non-active project session detail, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("team api follows active project storage", func(t *testing.T) {
		h := newServerHarness(t)
		otherRoot := t.TempDir()
		otherProjectID := h.insertProject(t, "seo-test", otherRoot)
		writeTestFile(t, filepath.Join(h.storageRoot, "projects", otherProjectID, "teams", "default", "team.yaml"), `
name: SEO Task Team
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    base_profile: operator
    tool_families: [repo_read, runtime_capability]
    specialist_summary_visibility: basic
`)
		writeTestFile(t, filepath.Join(h.storageRoot, "projects", otherProjectID, "teams", "default", "assistant.soul.yaml"), "role: SEO lead\n")
		if err := runtime.SetActiveProject(context.Background(), h.db, otherProjectID); err != nil {
			t.Fatalf("set active project: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/team", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "SEO Task Team") {
			t.Fatalf("expected active project team config, got:\n%s", rr.Body.String())
		}
	})
}

func TestRoutesDeliveriesProjectScoping(t *testing.T) {
	h := newServerHarness(t)
	_, route, intentID := h.seedRoutesDeliveriesData(t)

	otherRoot := t.TempDir()
	h.insertProject(t, "seo-test", otherRoot)
	otherRun, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-other",
			ExternalID:  "chat-other",
			ThreadID:    "thread-other",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "inspect other delivery",
		CWD:           otherRoot,
	})
	if err != nil {
		t.Fatalf("start other-project route flow: %v", err)
	}

	var otherRouteID string
	if err := h.db.RawDB().QueryRow(
		`SELECT id FROM session_bindings WHERE conversation_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`,
		otherRun.ConversationID,
	).Scan(&otherRouteID); err != nil {
		t.Fatalf("load other route id: %v", err)
	}
	var otherIntentID string
	if err := h.db.RawDB().QueryRow(
		`SELECT id FROM outbound_intents WHERE run_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`,
		otherRun.ID,
	).Scan(&otherIntentID); err != nil {
		t.Fatalf("load other delivery id: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/recover?connector_id=telegram&route_status=all&delivery_status=all", nil)

	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, route.ID) || !strings.Contains(body, intentID) {
		t.Fatalf("expected active project route and delivery to be visible, got:\n%s", body)
	}
	if strings.Contains(body, otherRouteID) || strings.Contains(body, otherIntentID) || strings.Contains(body, "chat-other") {
		t.Fatalf("expected other project routes and deliveries to be hidden, got:\n%s", body)
	}
}

func TestProjectScopedAPIAccess(t *testing.T) {
	t.Run("session detail api hides other project session", func(t *testing.T) {
		h := newServerHarness(t)
		otherRoot := t.TempDir()
		h.insertProject(t, "seo-test", otherRoot)
		otherRun, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-other",
				ExternalID:  "chat-other",
				ThreadID:    "thread-other",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "review the seo project",
			CWD:           otherRoot,
		})
		if err != nil {
			t.Fatalf("start other-project front session: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/conversations/"+otherRun.SessionID, nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for non-active project session api detail, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("route send api hides other project route", func(t *testing.T) {
		h := newServerHarness(t)
		otherRoot := t.TempDir()
		h.insertProject(t, "seo-test", otherRoot)
		otherRun, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-other",
				ExternalID:  "chat-other",
				ThreadID:    "thread-other",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "inspect other route",
			CWD:           otherRoot,
		})
		if err != nil {
			t.Fatalf("start other-project route flow: %v", err)
		}
		var otherRouteID string
		if err := h.db.RawDB().QueryRow(
			`SELECT id FROM session_bindings WHERE conversation_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`,
			otherRun.ConversationID,
		).Scan(&otherRouteID); err != nil {
			t.Fatalf("load other route id: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/routes/"+otherRouteID+"/messages", strings.NewReader(`{"body":"hi"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+h.adminToken)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for non-active project route send, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("delivery retry api hides other project delivery", func(t *testing.T) {
		h := newServerHarness(t)
		otherRoot := t.TempDir()
		h.insertProject(t, "seo-test", otherRoot)
		otherRun, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-other",
				ExternalID:  "chat-other",
				ThreadID:    "thread-other",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "inspect other delivery",
			CWD:           otherRoot,
		})
		if err != nil {
			t.Fatalf("start other-project delivery flow: %v", err)
		}
		var otherIntentID string
		if err := h.db.RawDB().QueryRow(
			`SELECT id FROM outbound_intents WHERE run_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`,
			otherRun.ID,
		).Scan(&otherIntentID); err != nil {
			t.Fatalf("load other delivery id: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/deliveries/"+otherIntentID+"/retry", nil)
		req.Header.Set("Authorization", "Bearer "+h.adminToken)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for non-active project delivery retry, got %d body=%s", rr.Code, rr.Body.String())
		}
	})
}

func TestPageRouteMap(t *testing.T) {
	t.Run("root serves the spa entry document", func(t *testing.T) {
		h := newServerHarness(t)
		wantBody, err := readSPAAsset("index.html")
		if err != nil {
			t.Fatalf("read spa index: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if rr.Body.String() != string(wantBody) {
			t.Fatalf("expected root to serve spa index")
		}
	})

	t.Run("user first spa routes render the app shell", func(t *testing.T) {
		h := newServerHarness(t)
		wantBody, err := readSPAAsset("index.html")
		if err != nil {
			t.Fatalf("read spa index: %v", err)
		}

		for _, path := range []string{
			"/work",
			"/team",
			"/knowledge",
			"/recover",
			"/conversations",
			"/automate",
			"/history",
			"/settings",
		} {
			t.Run(path, func(t *testing.T) {
				rr := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, path, nil)

				h.server.ServeHTTP(rr, req)

				if rr.Code != http.StatusOK {
					t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
				}
				if rr.Body.String() != string(wantBody) {
					t.Fatalf("expected %s to serve spa index", path)
				}
			})
		}
	})

}

func TestApprovalsResolve(t *testing.T) {
	t.Run("approve returns json", func(t *testing.T) {
		h := newServerHarness(t)
		ticketID := h.insertApproval(t, "run-approve", "bash", "echo hi")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/recover/approvals/"+ticketID+"/resolve",
			strings.NewReader("decision=approved"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("deny returns json", func(t *testing.T) {
		h := newServerHarness(t)
		ticketID := h.insertApproval(t, "run-deny", "bash", "rm -rf /")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/recover/approvals/"+ticketID+"/resolve",
			strings.NewReader("decision=denied"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("approve returns json for modal action requests", func(t *testing.T) {
		h := newServerHarness(t)
		ticketID := h.insertApproval(t, "run-approve-json", "bash", "echo hi")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/recover/approvals/"+ticketID+"/resolve",
			strings.NewReader("decision=approved"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
		if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
			t.Fatalf("expected JSON content type, got %q", got)
		}
		var resp struct {
			ApprovalID string `json:"approval_id"`
			Decision   string `json:"decision"`
			Status     string `json:"status"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal approval response: %v", err)
		}
		if resp.ApprovalID != ticketID || resp.Decision != "approved" || resp.Status != "approved" {
			t.Fatalf("unexpected approval json response: %+v", resp)
		}
	})

	t.Run("approve returns promptly while approved work continues in background", func(t *testing.T) {
		blocker := &blockingCoderExecTool{
			started: make(chan struct{}),
			release: make(chan struct{}),
		}
		prov := runtime.NewMockProvider([]runtime.GenerateResult{
			{
				ToolCalls: []model.ToolCallRequest{
					{
						ID:        "call-blocking-coder",
						ToolName:  "coder_exec",
						InputJSON: []byte(`{"backend":"codex","prompt":"Create created.txt"}`),
					},
				},
				StopReason: "tool_calls",
			},
			{Content: "done", StopReason: "end_turn"},
		}, nil)
		h := newServerHarnessWithProviderAndTools(t, prov, blocker)

		run, err := h.rt.Start(context.Background(), runtime.StartRun{
			ConversationID: "conv-approval-web-async",
			AgentID:        "patcher",
			Objective:      "mutate via coder",
			CWD:            h.workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		if run.Status != model.RunStatusNeedsApproval {
			t.Fatalf("expected needs_approval run, got %q", run.Status)
		}

		var ticketID string
		if err := h.db.RawDB().QueryRow(
			"SELECT id FROM approvals WHERE run_id = ? AND status = 'pending' LIMIT 1",
			run.ID,
		).Scan(&ticketID); err != nil {
			t.Fatalf("query approval ticket: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/recover/approvals/"+ticketID+"/resolve", strings.NewReader("decision=approved"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		done := make(chan struct{})
		go func() {
			h.server.ServeHTTP(rr, req)
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(200 * time.Millisecond):
			close(blocker.release)
			t.Fatal("expected approval resolve request to return before approved work finishes")
		}

		var status string
		if err := h.db.RawDB().QueryRow(
			"SELECT status FROM approvals WHERE id = ?",
			ticketID,
		).Scan(&status); err != nil {
			close(blocker.release)
			t.Fatalf("query approval status: %v", err)
		}
		if status != "approved" {
			close(blocker.release)
			t.Fatalf("expected approval row to be marked approved, got %q", status)
		}

		select {
		case <-blocker.started:
		case <-time.After(time.Second):
			close(blocker.release)
			t.Fatal("expected approved tool work to continue in the background")
		}

		runStatus := ""
		if err := h.db.RawDB().QueryRow("SELECT status FROM runs WHERE id = ?", run.ID).Scan(&runStatus); err != nil {
			close(blocker.release)
			t.Fatalf("query run status: %v", err)
		}
		if runStatus != "active" {
			close(blocker.release)
			t.Fatalf("expected run to stay active while approved tool is blocked, got %q", runStatus)
		}

		if rr.Code != http.StatusOK {
			close(blocker.release)
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		close(blocker.release)
		waitForRunStatus(t, h.db, run.ID, "completed")
		if _, err := os.Stat(filepath.Join(h.workspaceRoot, "created.txt")); err != nil {
			t.Fatalf("expected background approved tool to create file: %v", err)
		}
	})

	t.Run("invalid decision returns 400", func(t *testing.T) {
		h := newServerHarness(t)
		ticketID := h.insertApproval(t, "run-bad", "bash", "echo")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/recover/approvals/"+ticketID+"/resolve",
			strings.NewReader("decision=maybe"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rr.Code)
		}
	})
}

func TestProjectSwitcher(t *testing.T) {
	t.Run("switching project updates the active project", func(t *testing.T) {
		h := newServerHarness(t)
		otherRoot := t.TempDir()
		otherProjectID := h.insertProject(t, "seo-test", otherRoot)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/projects/activate",
			strings.NewReader("project_id="+url.QueryEscape(otherProjectID)+"&redirect_to=%2Fwork"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d body=%s", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Location") != "/work" {
			t.Fatalf("expected redirect back to /work, got %q", rr.Header().Get("Location"))
		}
		if got := lookupSetting(h.db, "active_project_id"); got != otherProjectID {
			t.Fatalf("expected active_project_id %q, got %q", otherProjectID, got)
		}
	})

	t.Run("missing project id returns bad request", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/projects/activate", strings.NewReader("redirect_to=%2Fwork"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("invalid redirect falls back to runs", func(t *testing.T) {
		h := newServerHarness(t)
		otherRoot := t.TempDir()
		otherProjectID := h.insertProject(t, "seo-test", otherRoot)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/projects/activate",
			strings.NewReader("project_id="+url.QueryEscape(otherProjectID)+"&redirect_to=https%3A%2F%2Fevil.example"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d body=%s", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Location") != "/work" {
			t.Fatalf("expected invalid redirect to fall back to /work, got %q", rr.Header().Get("Location"))
		}
	})
}

func TestRequestPathWithQuery(t *testing.T) {
	if got := requestPathWithQuery(nil); got != "/work" {
		t.Fatalf("expected nil request fallback %q, got %q", "/work", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/work?scope=all&q=fix", nil)
	if got := requestPathWithQuery(req); got != "/work?scope=all&q=fix" {
		t.Fatalf("expected request path with query, got %q", got)
	}

	req = &http.Request{URL: &url.URL{Path: "/work"}}
	if got := requestPathWithQuery(req); got != "/work" {
		t.Fatalf("expected request path fallback without request URI, got %q", got)
	}
}

func TestRequestBool(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  string
		want bool
	}{
		{name: "one", raw: "1", want: true},
		{name: "true", raw: "true", want: true},
		{name: "yes", raw: "yes", want: true},
		{name: "on", raw: "on", want: true},
		{name: "false", raw: "false", want: false},
		{name: "empty", raw: "", want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/work?flag="+url.QueryEscape(tt.raw), nil)
			if got := requestBool(req, "flag"); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestActionPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{name: "run detail", got: runDetailPath("run 1"), want: "/work/run%201"},
		{name: "run graph", got: runGraphPath("run 1"), want: "/api/work/run%201/graph"},
		{name: "run events", got: runEventsPath("run 1"), want: "/api/work/run%201/events"},
		{name: "run node detail template", got: runNodeDetailTemplatePath("run 1"), want: "/api/work/run%201/nodes/__RUN_ID__"},
		{name: "work dismiss", got: workDismissPath("run 1", "interrupted"), want: "/api/work/run%201/dismiss"},
		{name: "session detail", got: sessionDetailPath("session 1"), want: "/conversations/session%201"},
		{name: "session message", got: sessionMessagePath("session 1"), want: "/api/conversations/session%201/messages"},
		{name: "session retry delivery", got: sessionRetryDeliveryPath("session 1", "delivery/1"), want: "/api/conversations/session%201/deliveries/delivery%2F1/retry"},
		{name: "approval resolve", got: approvalResolvePath("approval/1"), want: "/api/recover/approvals/approval%2F1/resolve"},
		{name: "route send", got: routeSendPath("route 1"), want: "/api/recover/routes/route%201/messages"},
		{name: "route deactivate", got: routeDeactivatePath("route 1"), want: "/api/recover/routes/route%201/deactivate"},
		{name: "delivery retry", got: deliveryRetryPath("delivery/1"), want: "/api/recover/deliveries/delivery%2F1/retry"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, tc.got)
			}
		})
	}
}

func TestSameOriginRequest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		origin  string
		referer string
		host    string
		want    bool
	}{
		{name: "matching origin", origin: "http://localhost:8080", host: "localhost:8080", want: true},
		{name: "matching referer fallback", referer: "http://localhost:8080/work", host: "localhost:8080", want: true},
		{name: "origin wins over referer", origin: "http://evil.example", referer: "http://localhost:8080/work", host: "localhost:8080", want: false},
		{name: "invalid referer", referer: "://bad url", host: "localhost:8080", want: false},
		{name: "missing origin and referer", host: "localhost:8080", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "http://"+tc.host+"/work", nil)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}

			if got := sameOriginRequest(req); got != tc.want {
				t.Fatalf("expected sameOriginRequest=%t, got %t", tc.want, got)
			}
		})
	}
}

func TestProjectLayoutData_UsesActiveProjectOnly(t *testing.T) {
	h := newServerHarness(t)
	if _, err := h.db.RawDB().Exec("DELETE FROM projects"); err != nil {
		t.Fatalf("delete projects: %v", err)
	}
	if _, err := h.db.RawDB().Exec("DELETE FROM settings WHERE key = 'active_project_id'"); err != nil {
		t.Fatalf("delete active_project_id: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/work", nil)
	layout, err := h.server.projectLayoutData(req)
	if err != nil {
		t.Fatalf("projectLayoutData: %v", err)
	}
	if layout.ActiveName != "" {
		t.Fatalf("expected empty active project name without an active project, got %q", layout.ActiveName)
	}
	if len(layout.Options) != 0 {
		t.Fatalf("expected no switcher options without an active project, got %d", len(layout.Options))
	}
}

func TestAdminToken(t *testing.T) {
	t.Run("authorized work create starts a run and returns accepted", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/work", strings.NewReader(`{"task":"review"}`))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/json")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusAccepted {
			t.Fatalf("expected 202, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), `"run_id"`) {
			t.Fatalf("expected create response to include run_id, got %s", rr.Body.String())
		}
	})

	t.Run("missing authorization is rejected", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/work", strings.NewReader(`{"task":"review"}`))
		req.Header.Set("Content-Type", "application/json")

		h.rawServer.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
		if strings.TrimSpace(rr.Body.String()) != "unauthorized" {
			t.Fatalf("expected plain-text unauthorized, got %q", rr.Body.String())
		}
	})

	t.Run("wrong authorization is rejected", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/work", strings.NewReader(`{"task":"review"}`))
		req.Header.Set("Authorization", "Bearer wrong-token")
		req.Header.Set("Content-Type", "application/json")

		h.rawServer.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
		if strings.TrimSpace(rr.Body.String()) != "unauthorized" {
			t.Fatalf("expected plain-text unauthorized, got %q", rr.Body.String())
		}
	})

	t.Run("run submit checks current token from settings", func(t *testing.T) {
		h := newServerHarness(t)
		h.setAdminToken(t, "rotated-admin-token")

		oldTokenResp := httptest.NewRecorder()
		oldTokenReq := httptest.NewRequest(http.MethodPost, "/api/work", strings.NewReader(`{"task":"review"}`))
		oldTokenReq.Header.Set("Authorization", "Bearer "+h.adminToken)
		oldTokenReq.Header.Set("Content-Type", "application/json")

		h.rawServer.ServeHTTP(oldTokenResp, oldTokenReq)

		if oldTokenResp.Code != http.StatusUnauthorized {
			t.Fatalf("expected stale token to be rejected with 401, got %d", oldTokenResp.Code)
		}

		newTokenResp := httptest.NewRecorder()
		newTokenReq := httptest.NewRequest(http.MethodPost, "/api/work", strings.NewReader(`{"task":"review"}`))
		newTokenReq.Header.Set("Authorization", "Bearer rotated-admin-token")
		newTokenReq.Header.Set("Content-Type", "application/json")

		h.rawServer.ServeHTTP(newTokenResp, newTokenReq)

		if newTokenResp.Code == http.StatusUnauthorized {
			t.Fatalf("expected current token to be accepted, got 401")
		}
	})

	t.Run("login issues browser session cookies", func(t *testing.T) {
		h := newServerHarness(t)
		if err := authpkg.SetPassword(context.Background(), h.db, "test-password", time.Now().UTC()); err != nil {
			t.Fatalf("set password: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/api/auth/login", strings.NewReader(`{"password":"test-password"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")

		h.rawServer.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 303, got %d", rr.Code)
		}

		sessionCookie := findCookie(rr.Result().Cookies(), sessionCookieName)
		deviceCookie := findCookie(rr.Result().Cookies(), deviceCookieName)
		if sessionCookie == nil {
			t.Fatalf("expected %s cookie to be set", sessionCookieName)
		}
		if deviceCookie == nil {
			t.Fatalf("expected %s cookie to be set", deviceCookieName)
		}
		if !sessionCookie.HttpOnly || !deviceCookie.HttpOnly {
			t.Fatal("expected browser auth cookies to be HttpOnly")
		}
		if sessionCookie.SameSite != http.SameSiteLaxMode || deviceCookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("expected SameSite=Lax auth cookies, got session=%v device=%v", sessionCookie.SameSite, deviceCookie.SameSite)
		}
	})

	t.Run("same-origin browser session can create work", func(t *testing.T) {
		h := newServerHarness(t)
		cookie := hostAdminSessionCookie(t, h, "http://localhost/work")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/api/work", strings.NewReader(`{"task":"review"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusAccepted {
			t.Fatalf("expected 202, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("cross-origin browser session create is rejected", func(t *testing.T) {
		h := newServerHarness(t)
		cookie := hostAdminSessionCookie(t, h, "http://localhost/work")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/api/work", strings.NewReader(`{"task":"review"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://evil.test")
		req.AddCookie(cookie)

		h.rawServer.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", rr.Code)
		}
		if strings.TrimSpace(rr.Body.String()) != "forbidden" {
			t.Fatalf("expected plain-text forbidden, got %q", rr.Body.String())
		}
	})
}

func TestSSE(t *testing.T) {
	h := newServerHarness(t)
	h.insertRun(t, "run-sse", "conv-sse", "watch events", "active")

	ts := httptest.NewServer(h.server)
	defer ts.Close()

	respOne, readerOne := subscribeSSE(t, ts.URL+"/api/work/run-sse/events")
	respTwo, readerTwo := subscribeSSE(t, ts.URL+"/api/work/run-sse/events")

	first := model.ReplayDelta{
		RunID:      "run-sse",
		Kind:       "turn_completed",
		OccurredAt: time.Unix(1711267200, 0).UTC(),
	}
	if err := h.broadcaster.Emit(context.Background(), "run-sse", first); err != nil {
		t.Fatalf("emit first: %v", err)
	}

	assertEventContains(t, readerOne, "turn_completed")
	assertEventContains(t, readerTwo, "turn_completed")

	if err := respTwo.Body.Close(); err != nil {
		t.Fatalf("close second client: %v", err)
	}
	waitForSubscribers(t, h.broadcaster, "run-sse", 1)

	second := model.ReplayDelta{
		RunID:      "run-sse",
		Kind:       "run_completed",
		OccurredAt: time.Unix(1711267260, 0).UTC(),
	}
	if err := h.broadcaster.Emit(context.Background(), "run-sse", second); err != nil {
		t.Fatalf("emit second: %v", err)
	}

	assertEventContains(t, readerOne, "run_completed")

	if err := respOne.Body.Close(); err != nil {
		t.Fatalf("close first client: %v", err)
	}
	waitForSubscribers(t, h.broadcaster, "run-sse", 0)

	t.Run("replays event history after cursor", func(t *testing.T) {
		h := newServerHarness(t)
		h.insertRun(t, "run-sse-backfill", "conv-sse-backfill", "watch events", "completed")
		h.insertEventAt(t, "evt-start", "conv-sse-backfill", "run-sse-backfill", "run_started", "2026-03-25 08:00:00")
		h.insertEventAtWithPayload(
			t,
			"evt-turn",
			"conv-sse-backfill",
			"run-sse-backfill",
			"turn_completed",
			[]byte(`{"content":"Backfilled answer","input_tokens":10,"output_tokens":5}`),
			"2026-03-25 08:00:01",
		)
		h.insertEventAtWithPayload(
			t,
			"evt-complete",
			"conv-sse-backfill",
			"run-sse-backfill",
			"run_completed",
			[]byte(`{"input_tokens":10,"output_tokens":5}`),
			"2026-03-25 08:00:02",
		)

		ts := httptest.NewServer(h.server)
		defer ts.Close()

		resp, reader := subscribeSSE(t, ts.URL+"/api/work/run-sse-backfill/events?after=2026-03-25T08%3A00%3A00Z%7Cevt-start")
		defer resp.Body.Close()

		first := readSSEEventWithin(t, resp, reader, time.Second)
		if got := first["kind"]; got != "turn_completed" {
			t.Fatalf("expected first backfilled event kind turn_completed, got %#v", got)
		}
		if got := first["event_id"]; got != "evt-turn" {
			t.Fatalf("expected first backfilled event id %q, got %#v", "evt-turn", got)
		}
		payload, ok := first["payload"].(map[string]any)
		if !ok {
			t.Fatalf("expected structured backfilled payload, got %#v", first["payload"])
		}
		if got := payload["content"]; got != "Backfilled answer" {
			t.Fatalf("expected backfilled content %q, got %#v", "Backfilled answer", got)
		}

		second := readSSEEventWithin(t, resp, reader, time.Second)
		if got := second["kind"]; got != "run_completed" {
			t.Fatalf("expected second backfilled event kind run_completed, got %#v", got)
		}
		if got := second["event_id"]; got != "evt-complete" {
			t.Fatalf("expected second backfilled event id %q, got %#v", "evt-complete", got)
		}
	})
}

func TestSSEPayloadsAreStructuredJSON(t *testing.T) {
	h := newServerHarness(t)
	h.insertRun(t, "run-sse-json", "conv-sse-json", "watch events", "active")

	ts := httptest.NewServer(h.server)
	defer ts.Close()

	resp, reader := subscribeSSE(t, ts.URL+"/api/work/run-sse-json/events")
	defer resp.Body.Close()

	payload := []byte(`{"text":"Hel"}`)
	if err := h.broadcaster.Emit(context.Background(), "run-sse-json", model.ReplayDelta{
		RunID:       "run-sse-json",
		Kind:        "turn_delta",
		PayloadJSON: payload,
		OccurredAt:  time.Unix(1711267200, 0).UTC(),
	}); err != nil {
		t.Fatalf("emit turn_delta: %v", err)
	}

	event := readSSEEvent(t, reader)
	if got := event["kind"]; got != "turn_delta" {
		t.Fatalf("expected kind turn_delta, got %#v", got)
	}
	rawPayload, ok := event["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured payload object, got %#v", event["payload"])
	}
	if got := rawPayload["text"]; got != "Hel" {
		t.Fatalf("expected payload text %q, got %#v", "Hel", got)
	}
}

func TestSSEToolLogPayloadsCarryAggregatedHTML(t *testing.T) {
	h := newServerHarness(t)
	h.insertRun(t, "run-sse-log", "conv-sse-log", "watch tool logs", "active")
	h.insertEventAtWithPayload(
		t,
		"evt-sse-log-history",
		"conv-sse-log",
		"run-sse-log",
		"tool_log_recorded",
		[]byte(`{"tool_call_id":"call-coder","tool_name":"coder_exec","stream":"terminal","text":"\u001b[31mFA"}`),
		"2026-03-26 05:00:00",
	)

	ts := httptest.NewServer(h.server)
	defer ts.Close()

	resp, reader := subscribeSSE(t, ts.URL+"/api/work/run-sse-log/events")
	defer resp.Body.Close()

	if err := h.broadcaster.Emit(context.Background(), "run-sse-log", model.ReplayDelta{
		RunID:       "run-sse-log",
		Kind:        "tool_log_recorded",
		PayloadJSON: []byte(`{"tool_call_id":"call-coder","tool_name":"coder_exec","stream":"terminal","text":"IL\u001b[0m"}`),
		OccurredAt:  time.Unix(1711267200, 0).UTC(),
	}); err != nil {
		t.Fatalf("emit tool_log_recorded: %v", err)
	}

	event := readSSEEvent(t, reader)
	if got := event["kind"]; got != "tool_log_recorded" {
		t.Fatalf("expected kind tool_log_recorded, got %#v", got)
	}
	rawPayload, ok := event["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured payload object, got %#v", event["payload"])
	}
	if got := rawPayload["entry_key"]; got != "call-coder::terminal" {
		t.Fatalf("expected stable entry key, got %#v", got)
	}
	if got := rawPayload["body"]; got != "\u001b[31mFAIL\u001b[0m" {
		t.Fatalf("expected aggregated terminal body, got %#v", got)
	}
	bodyHTML, ok := rawPayload["body_html"].(string)
	if !ok || !strings.Contains(bodyHTML, "FAIL") {
		t.Fatalf("expected rendered terminal HTML, got %#v", rawPayload["body_html"])
	}
}

func TestSessionAPI(t *testing.T) {
	t.Run("delivery health returns connector queue summary as JSON", func(t *testing.T) {
		h := newServerHarness(t)
		frontTelegram, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-1",
				ExternalID:  "chat-1",
				ThreadID:    "thread-1",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "Inspect the repo.",
			CWD:           h.workspaceRoot,
		})
		if err != nil {
			t.Fatalf("StartFrontSession telegram failed: %v", err)
		}
		frontWhatsApp, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "whatsapp",
				AccountID:   "acct-2",
				ExternalID:  "chat-2",
				ThreadID:    "thread-2",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "Inspect WhatsApp.",
			CWD:           h.workspaceRoot,
		})
		if err != nil {
			t.Fatalf("StartFrontSession whatsapp failed: %v", err)
		}

		if _, err := h.db.RawDB().Exec(
			`UPDATE outbound_intents
			 SET status='retrying', attempts=2, last_attempt_at=datetime('now', '-2 minutes')
			 WHERE run_id = ?`,
			frontTelegram.ID,
		); err != nil {
			t.Fatalf("update telegram outbound intent: %v", err)
		}
		if _, err := h.db.RawDB().Exec(
			`INSERT INTO outbound_intents
			 (id, run_id, connector_id, chat_id, message_text, dedupe_key, status, attempts, created_at)
			 VALUES ('intent-extra-terminal', ?, 'telegram', 'chat-1', 'reply extra', 'dedupe-extra', 'terminal', 5, datetime('now', '-1 minutes'))`,
			frontTelegram.ID,
		); err != nil {
			t.Fatalf("insert telegram terminal outbound intent: %v", err)
		}
		if _, err := h.db.RawDB().Exec(
			`UPDATE outbound_intents
			 SET status='pending', attempts=0, last_attempt_at=NULL
			 WHERE run_id = ?`,
			frontWhatsApp.ID,
		); err != nil {
			t.Fatalf("update whatsapp outbound intent: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/deliveries/health", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
			t.Fatalf("expected JSON content type, got %q", got)
		}

		var resp struct {
			Connectors []model.ConnectorDeliveryHealth `json:"connectors"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(resp.Connectors) != 2 {
			t.Fatalf("expected 2 connector summaries, got %d", len(resp.Connectors))
		}
		if resp.Connectors[0].ConnectorID != "telegram" || resp.Connectors[0].RetryingCount != 1 || resp.Connectors[0].TerminalCount != 1 {
			t.Fatalf("unexpected telegram connector summary: %+v", resp.Connectors[0])
		}
		if resp.Connectors[1].ConnectorID != "whatsapp" || resp.Connectors[1].PendingCount != 1 {
			t.Fatalf("unexpected whatsapp connector summary: %+v", resp.Connectors[1])
		}
	})

	t.Run("delivery health includes runtime connector snapshots", func(t *testing.T) {
		h := newServerHarnessWithConnectorHealth(t, []model.ConnectorHealthSnapshot{
			{
				ConnectorID: "telegram",
				State:       model.ConnectorHealthDegraded,
				Summary:     "poll loop stale",
			},
			{
				ConnectorID: "whatsapp",
				State:       model.ConnectorHealthHealthy,
				Summary:     "webhook activity recent",
			},
		})

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/deliveries/health", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			RuntimeConnectors []model.ConnectorHealthSnapshot `json:"runtime_connectors"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(resp.RuntimeConnectors) != 2 {
			t.Fatalf("expected 2 runtime connector summaries, got %d", len(resp.RuntimeConnectors))
		}
		if resp.RuntimeConnectors[0].ConnectorID != "telegram" || resp.RuntimeConnectors[0].Summary != "poll loop stale" {
			t.Fatalf("unexpected telegram runtime summary: %+v", resp.RuntimeConnectors[0])
		}
	})

	t.Run("delivery index filters queue and top-level retry requeues by delivery id", func(t *testing.T) {
		h := newServerHarness(t)
		run, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-1",
				ExternalID:  "chat-1",
				ThreadID:    "thread-1",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "Inspect the repo.",
			CWD:           h.workspaceRoot,
		})
		if err != nil {
			t.Fatalf("StartFrontSession failed: %v", err)
		}

		var intentID string
		if err := h.db.RawDB().QueryRow(
			`SELECT id
			 FROM outbound_intents
			 WHERE run_id = ?
			 ORDER BY created_at DESC, id DESC
			 LIMIT 1`,
			run.ID,
		).Scan(&intentID); err != nil {
			t.Fatalf("load outbound intent: %v", err)
		}
		if _, err := h.db.RawDB().Exec(
			`UPDATE outbound_intents
			 SET status='terminal', attempts=5, last_attempt_at=datetime('now')
			 WHERE id = ?`,
			intentID,
		); err != nil {
			t.Fatalf("mark terminal intent: %v", err)
		}

		listRR := httptest.NewRecorder()
		listReq := httptest.NewRequest(http.MethodGet, "/api/deliveries?connector_id=telegram&status=terminal", nil)
		h.server.ServeHTTP(listRR, listReq)
		if listRR.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", listRR.Code, listRR.Body.String())
		}

		var listResp struct {
			Deliveries []model.DeliveryQueueItem `json:"deliveries"`
		}
		if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
			t.Fatalf("decode list response: %v", err)
		}
		if len(listResp.Deliveries) != 1 {
			t.Fatalf("expected 1 filtered delivery, got %d", len(listResp.Deliveries))
		}
		if listResp.Deliveries[0].ID != intentID || listResp.Deliveries[0].SessionID != run.SessionID {
			t.Fatalf("unexpected delivery queue item: %+v", listResp.Deliveries[0])
		}

		retryRR := httptest.NewRecorder()
		retryReq := httptest.NewRequest(http.MethodPost, "/api/deliveries/"+intentID+"/retry", nil)
		retryReq.Header.Set("Authorization", "Bearer "+h.adminToken)
		h.server.ServeHTTP(retryRR, retryReq)
		if retryRR.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", retryRR.Code, retryRR.Body.String())
		}

		var retryResp struct {
			Delivery model.OutboundIntent `json:"delivery"`
		}
		if err := json.Unmarshal(retryRR.Body.Bytes(), &retryResp); err != nil {
			t.Fatalf("decode retry response: %v", err)
		}
		if retryResp.Delivery.ID != intentID || retryResp.Delivery.Status != "pending" {
			t.Fatalf("unexpected retried delivery: %+v", retryResp.Delivery)
		}
	})

	t.Run("route index returns active bindings as JSON", func(t *testing.T) {
		h := newServerHarness(t)
		telegramRun, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-1",
				ExternalID:  "chat-1",
				ThreadID:    "thread-1",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "Inspect Telegram.",
			CWD:           h.workspaceRoot,
		})
		if err != nil {
			t.Fatalf("StartFrontSession telegram failed: %v", err)
		}
		if _, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "whatsapp",
				AccountID:   "acct-2",
				ExternalID:  "chat-2",
				ThreadID:    "thread-2",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "Inspect WhatsApp.",
			CWD:           h.workspaceRoot,
		}); err != nil {
			t.Fatalf("StartFrontSession whatsapp failed: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/routes?connector_id=telegram", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Routes []model.RouteDirectoryItem `json:"routes"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(resp.Routes) != 1 {
			t.Fatalf("expected 1 route, got %d", len(resp.Routes))
		}
		if resp.Routes[0].ID == "" {
			t.Fatalf("expected route id, got %+v", resp.Routes[0])
		}
		if resp.Routes[0].ConnectorID != "telegram" || resp.Routes[0].SessionID != telegramRun.SessionID {
			t.Fatalf("unexpected route payload: %+v", resp.Routes[0])
		}
	})

	t.Run("route create binds an existing session", func(t *testing.T) {
		h := newServerHarness(t)
		sessionSvc := sessions.NewService(h.db, conversations.NewConversationStore(h.db))
		front, err := sessionSvc.OpenFrontSession(context.Background(), sessions.OpenFrontSession{
			ConversationID: "conv-manual-bind",
			AgentID:        "assistant",
		})
		if err != nil {
			t.Fatalf("OpenFrontSession failed: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/routes",
			strings.NewReader(`{"session_id":"`+front.ID+`","connector_id":"telegram","account_id":"acct-1","external_id":"chat-1","thread_id":"thread-1"}`))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/json")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Route model.RouteDirectoryItem `json:"route"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Route.ID == "" || resp.Route.SessionID != front.ID {
			t.Fatalf("unexpected route identity: %+v", resp.Route)
		}
		if resp.Route.ConnectorID != "telegram" || resp.Route.ThreadID != "thread-1" {
			t.Fatalf("unexpected route target: %+v", resp.Route)
		}
	})

	t.Run("route deactivate removes binding from route directory", func(t *testing.T) {
		h := newServerHarness(t)
		if _, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-1",
				ExternalID:  "chat-1",
				ThreadID:    "thread-1",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "Inspect Telegram.",
			CWD:           h.workspaceRoot,
		}); err != nil {
			t.Fatalf("StartFrontSession telegram failed: %v", err)
		}

		listRR := httptest.NewRecorder()
		listReq := httptest.NewRequest(http.MethodGet, "/api/routes?connector_id=telegram", nil)
		h.server.ServeHTTP(listRR, listReq)
		if listRR.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", listRR.Code, listRR.Body.String())
		}

		var listResp struct {
			Routes []model.RouteDirectoryItem `json:"routes"`
		}
		if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
			t.Fatalf("decode list response: %v", err)
		}
		if len(listResp.Routes) != 1 {
			t.Fatalf("expected 1 route, got %d", len(listResp.Routes))
		}

		deactivateRR := httptest.NewRecorder()
		deactivateReq := httptest.NewRequest(http.MethodPost, "/api/routes/"+listResp.Routes[0].ID+"/deactivate", nil)
		deactivateReq.Header.Set("Authorization", "Bearer "+h.adminToken)
		h.server.ServeHTTP(deactivateRR, deactivateReq)
		if deactivateRR.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", deactivateRR.Code, deactivateRR.Body.String())
		}

		afterRR := httptest.NewRecorder()
		afterReq := httptest.NewRequest(http.MethodGet, "/api/routes?connector_id=telegram", nil)
		h.server.ServeHTTP(afterRR, afterReq)
		if afterRR.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", afterRR.Code, afterRR.Body.String())
		}

		var afterResp struct {
			Routes []model.RouteDirectoryItem `json:"routes"`
		}
		if err := json.Unmarshal(afterRR.Body.Bytes(), &afterResp); err != nil {
			t.Fatalf("decode post-deactivate response: %v", err)
		}
		if len(afterResp.Routes) != 0 {
			t.Fatalf("expected route directory to be empty after deactivation, got %d", len(afterResp.Routes))
		}

		historyRR := httptest.NewRecorder()
		historyReq := httptest.NewRequest(http.MethodGet, "/api/routes?connector_id=telegram&status=all", nil)
		h.server.ServeHTTP(historyRR, historyReq)
		if historyRR.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", historyRR.Code, historyRR.Body.String())
		}

		var historyResp struct {
			Routes []model.RouteDirectoryItem `json:"routes"`
		}
		if err := json.Unmarshal(historyRR.Body.Bytes(), &historyResp); err != nil {
			t.Fatalf("decode route history response: %v", err)
		}
		if len(historyResp.Routes) != 1 {
			t.Fatalf("expected 1 historical route, got %d", len(historyResp.Routes))
		}
		if historyResp.Routes[0].Status != "inactive" || historyResp.Routes[0].DeactivatedAt == nil || historyResp.Routes[0].DeactivationReason != "deactivated" {
			t.Fatalf("expected inactive historical route with deactivation metadata, got %+v", historyResp.Routes[0])
		}
	})

	t.Run("route send wakes the bound session", func(t *testing.T) {
		h := newServerHarness(t)
		run, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-1",
				ExternalID:  "chat-1",
				ThreadID:    "thread-1",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "Inspect Telegram.",
			CWD:           h.workspaceRoot,
		})
		if err != nil {
			t.Fatalf("StartFrontSession telegram failed: %v", err)
		}

		listRR := httptest.NewRecorder()
		listReq := httptest.NewRequest(http.MethodGet, "/api/routes?connector_id=telegram", nil)
		h.server.ServeHTTP(listRR, listReq)
		if listRR.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", listRR.Code, listRR.Body.String())
		}

		var listResp struct {
			Routes []model.RouteDirectoryItem `json:"routes"`
		}
		if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
			t.Fatalf("decode list response: %v", err)
		}
		if len(listResp.Routes) != 1 {
			t.Fatalf("expected 1 route, got %d", len(listResp.Routes))
		}

		sendRR := httptest.NewRecorder()
		sendReq := httptest.NewRequest(http.MethodPost, "/api/routes/"+listResp.Routes[0].ID+"/messages",
			strings.NewReader(`{"body":"What changed?"}`))
		sendReq.Header.Set("Authorization", "Bearer "+h.adminToken)
		sendReq.Header.Set("Content-Type", "application/json")
		h.server.ServeHTTP(sendRR, sendReq)
		if sendRR.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", sendRR.Code, sendRR.Body.String())
		}

		var sendResp struct {
			Run model.Run `json:"run"`
		}
		if err := json.Unmarshal(sendRR.Body.Bytes(), &sendResp); err != nil {
			t.Fatalf("decode send response: %v", err)
		}
		if sendResp.Run.SessionID != run.SessionID || sendResp.Run.ID == run.ID {
			t.Fatalf("unexpected route send run: %+v", sendResp.Run)
		}
	})

	t.Run("list returns recent sessions as JSON", func(t *testing.T) {
		h := newServerHarness(t)
		front := h.startFrontSession(t, "Inspect the repo.")
		worker := h.spawnWorkerSession(t, front.SessionID, "researcher", "Inspect docs.")
		if err := h.rt.Announce(context.Background(), runtime.AnnounceCommand{
			WorkerSessionID: worker.SessionID,
			TargetSessionID: front.SessionID,
			Body:            "Docs inspected.",
		}); err != nil {
			t.Fatalf("Announce failed: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/conversations", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
			t.Fatalf("expected JSON content type, got %q", got)
		}

		var resp struct {
			Sessions []struct {
				ID string `json:"id"`
			} `json:"sessions"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(resp.Sessions) != 2 {
			t.Fatalf("expected 2 sessions, got %d", len(resp.Sessions))
		}
		if resp.Sessions[0].ID != front.SessionID || resp.Sessions[1].ID != worker.SessionID {
			t.Fatalf("expected front session first and worker second, got %q then %q", resp.Sessions[0].ID, resp.Sessions[1].ID)
		}
	})

	t.Run("list applies query filters for sessions", func(t *testing.T) {
		h := newServerHarness(t)
		front := h.startFrontSession(t, "Inspect the repo.")
		worker := h.spawnWorkerSession(t, front.SessionID, "researcher", "Inspect docs.")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/conversations?role=worker&q=researcher", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Sessions []struct {
				ID string `json:"id"`
			} `json:"sessions"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(resp.Sessions) != 1 || resp.Sessions[0].ID != worker.SessionID {
			t.Fatalf("expected only researcher worker session, got %+v", resp.Sessions)
		}
	})

	t.Run("list paginates sessions with next and previous cursors", func(t *testing.T) {
		h := newServerHarness(t)
		svc := sessions.NewService(h.db, conversations.NewConversationStore(h.db))
		base := time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC)

		runs := make([]model.Run, 0, 3)
		for i, key := range []conversations.ConversationKey{
			{ConnectorID: "web", AccountID: "local", ExternalID: "assistant-1", ThreadID: "main-1"},
			{ConnectorID: "web", AccountID: "local", ExternalID: "assistant-2", ThreadID: "main-2"},
			{ConnectorID: "web", AccountID: "local", ExternalID: "assistant-3", ThreadID: "main-3"},
		} {
			run, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
				ConversationKey: key,
				FrontAgentID:    "assistant",
				InitialPrompt:   "Inspect the repo.",
				CWD:             h.workspaceRoot,
			})
			if err != nil {
				t.Fatalf("StartFrontSession %d failed: %v", i, err)
			}
			if err := svc.AppendMessage(context.Background(), model.SessionMessage{
				ID:        "msg-page-" + run.SessionID,
				SessionID: run.SessionID,
				Kind:      model.MessageAssistant,
				Body:      "page marker",
				CreatedAt: base.Add(time.Duration(i) * time.Minute),
			}); err != nil {
				t.Fatalf("AppendMessage %d failed: %v", i, err)
			}
			runs = append(runs, run)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/conversations?limit=1", nil)
		h.server.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var first struct {
			Sessions []struct {
				ID string `json:"id"`
			} `json:"sessions"`
			Paging struct {
				NextURL string `json:"next_url"`
				PrevURL string `json:"prev_url"`
				HasNext bool   `json:"has_next"`
				HasPrev bool   `json:"has_prev"`
			} `json:"paging"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &first); err != nil {
			t.Fatalf("decode first page: %v", err)
		}
		if len(first.Sessions) != 1 || first.Sessions[0].ID != runs[2].SessionID {
			t.Fatalf("expected newest session first, got %+v", first.Sessions)
		}
		if !first.Paging.HasNext || first.Paging.NextURL == "" || first.Paging.HasPrev || first.Paging.PrevURL != "" {
			t.Fatalf("unexpected first page metadata: %+v", first)
		}

		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, first.Paging.NextURL, nil)
		h.server.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var second struct {
			Sessions []struct {
				ID string `json:"id"`
			} `json:"sessions"`
			Paging struct {
				NextURL string `json:"next_url"`
				PrevURL string `json:"prev_url"`
				HasNext bool   `json:"has_next"`
				HasPrev bool   `json:"has_prev"`
			} `json:"paging"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &second); err != nil {
			t.Fatalf("decode second page: %v", err)
		}
		if len(second.Sessions) != 1 || second.Sessions[0].ID != runs[1].SessionID {
			t.Fatalf("expected middle session second, got %+v", second.Sessions)
		}
		if !second.Paging.HasPrev || second.Paging.PrevURL == "" {
			t.Fatalf("expected previous cursor on second page, got %+v", second)
		}
	})

	t.Run("detail returns session mailbox as JSON", func(t *testing.T) {
		h := newServerHarness(t)
		front := h.startFrontSession(t, "Inspect the repo.")
		worker := h.spawnWorkerSession(t, front.SessionID, "researcher", "Inspect docs.")
		if err := h.rt.Announce(context.Background(), runtime.AnnounceCommand{
			WorkerSessionID: worker.SessionID,
			TargetSessionID: front.SessionID,
			Body:            "Docs inspected.",
		}); err != nil {
			t.Fatalf("Announce failed: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/conversations/"+front.SessionID, nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Session struct {
				ID string `json:"id"`
			} `json:"session"`
			Messages []struct {
				Body runStructuredTextView `json:"body"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Session.ID != front.SessionID {
			t.Fatalf("expected session %q, got %q", front.SessionID, resp.Session.ID)
		}
		if len(resp.Messages) != 3 {
			t.Fatalf("expected 3 mailbox messages, got %d", len(resp.Messages))
		}
		if resp.Messages[0].Body.PlainText != "Inspect the repo." || resp.Messages[1].Body.PlainText != "mock response" || resp.Messages[2].Body.PlainText != "Docs inspected." {
			t.Fatalf("unexpected mailbox bodies: %q / %q / %q", resp.Messages[0].Body.PlainText, resp.Messages[1].Body.PlainText, resp.Messages[2].Body.PlainText)
		}
	})

	t.Run("detail includes active route when session is externally bound", func(t *testing.T) {
		h := newServerHarness(t)
		run, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-1",
				ExternalID:  "chat-1",
				ThreadID:    "thread-1",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "Inspect the repo.",
			CWD:           h.workspaceRoot,
		})
		if err != nil {
			t.Fatalf("StartFrontSession failed: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/conversations/"+run.SessionID, nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Route *struct {
				ConnectorID string `json:"connector_id"`
				ThreadID    string `json:"thread_id"`
				ExternalID  string `json:"external_id"`
			} `json:"route"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Route == nil {
			t.Fatal("expected active route in session detail response")
		}
		if resp.Route.ConnectorID != "telegram" || resp.Route.ThreadID != "thread-1" || resp.Route.ExternalID != "chat-1" {
			t.Fatalf(
				"unexpected route payload: connector_id=%q thread_id=%q external_id=%q",
				resp.Route.ConnectorID,
				resp.Route.ThreadID,
				resp.Route.ExternalID,
			)
		}
	})

	t.Run("detail includes outbound deliveries and failures for the session", func(t *testing.T) {
		h := newServerHarness(t)
		run, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-1",
				ExternalID:  "chat-1",
				ThreadID:    "thread-1",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "Inspect the repo.",
			CWD:           h.workspaceRoot,
		})
		if err != nil {
			t.Fatalf("StartFrontSession failed: %v", err)
		}

		var intentID string
		if err := h.db.RawDB().QueryRow(
			`SELECT id
			 FROM outbound_intents
			 WHERE run_id = ?
			 ORDER BY created_at DESC, id DESC
			 LIMIT 1`,
			run.ID,
		).Scan(&intentID); err != nil {
			t.Fatalf("load outbound intent: %v", err)
		}

		if _, err := h.db.RawDB().Exec(
			`UPDATE outbound_intents
			 SET status='terminal', attempts=3, last_attempt_at=datetime('now')
			 WHERE run_id = ?`,
			run.ID,
		); err != nil {
			t.Fatalf("update outbound intent: %v", err)
		}
		payload := `{"intent_id":"` + intentID + `","chat_id":"chat-1","connector_id":"telegram","event_kind":"run_completed","error":"delivery failed"}`
		if err := conversations.NewConversationStore(h.db).AppendEvent(context.Background(), model.Event{
			ID:             "evt-session-delivery-failed",
			ConversationID: run.ConversationID,
			RunID:          run.ID,
			Kind:           "delivery_failed",
			PayloadJSON:    []byte(payload),
		}); err != nil {
			t.Fatalf("append delivery_failed event: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/conversations/"+run.SessionID, nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Deliveries []struct {
				ID          string `json:"id"`
				ConnectorID string `json:"connector_id"`
				Status      string `json:"status"`
			} `json:"deliveries"`
			DeliveryFailures []struct {
				ConnectorID string `json:"connector_id"`
				Error       string `json:"error"`
			} `json:"delivery_failures"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(resp.Deliveries) != 1 {
			t.Fatalf("expected 1 delivery, got %d", len(resp.Deliveries))
		}
		if resp.Deliveries[0].ID != intentID || resp.Deliveries[0].Status != "terminal" || resp.Deliveries[0].ConnectorID != "telegram" {
			t.Fatalf("unexpected delivery payload: %+v", resp.Deliveries[0])
		}
		if len(resp.DeliveryFailures) != 1 {
			t.Fatalf("expected 1 delivery failure, got %d", len(resp.DeliveryFailures))
		}
		if resp.DeliveryFailures[0].ConnectorID != "telegram" || resp.DeliveryFailures[0].Error != "delivery failed" {
			t.Fatalf("unexpected delivery failure error: %+v", resp.DeliveryFailures[0])
		}
	})

	t.Run("detail missing session returns not found", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/conversations/missing", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rr.Code)
		}
	})

	t.Run("send wakes session and returns follow-up run JSON", func(t *testing.T) {
		h := newServerHarness(t)
		front := h.startFrontSession(t, "Inspect the repo.")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/conversations/"+front.SessionID+"/messages",
			strings.NewReader(`{"body":"What changed?"}`))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/json")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Run model.Run `json:"run"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Run.SessionID != front.SessionID {
			t.Fatalf("expected run to reuse session %q, got %q", front.SessionID, resp.Run.SessionID)
		}
		if resp.Run.ID == front.ID {
			t.Fatalf("expected a new follow-up run, got original %q", resp.Run.ID)
		}

		detail := httptest.NewRecorder()
		detailReq := httptest.NewRequest(http.MethodGet, "/api/conversations/"+front.SessionID, nil)
		h.server.ServeHTTP(detail, detailReq)
		if detail.Code != http.StatusOK {
			t.Fatalf("expected 200 from detail endpoint, got %d body=%s", detail.Code, detail.Body.String())
		}

		var mailbox struct {
			Messages []struct {
				Body runStructuredTextView `json:"body"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(detail.Body.Bytes(), &mailbox); err != nil {
			t.Fatalf("decode mailbox: %v", err)
		}
		if len(mailbox.Messages) != 4 {
			t.Fatalf("expected 4 mailbox messages after send, got %d", len(mailbox.Messages))
		}
		if mailbox.Messages[2].Body.PlainText != "What changed?" || mailbox.Messages[3].Body.PlainText != "mock response" {
			t.Fatalf("unexpected follow-up mailbox bodies: %q / %q", mailbox.Messages[2].Body.PlainText, mailbox.Messages[3].Body.PlainText)
		}
	})

	t.Run("retry delivery requeues terminal outbound intent", func(t *testing.T) {
		h := newServerHarness(t)
		run, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-1",
				ExternalID:  "chat-1",
				ThreadID:    "thread-1",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "Inspect the repo.",
			CWD:           h.workspaceRoot,
		})
		if err != nil {
			t.Fatalf("StartFrontSession failed: %v", err)
		}

		var intentID string
		if err := h.db.RawDB().QueryRow(
			`SELECT id
			 FROM outbound_intents
			 WHERE run_id = ?
			 ORDER BY created_at DESC, id DESC
			 LIMIT 1`,
			run.ID,
		).Scan(&intentID); err != nil {
			t.Fatalf("load outbound intent: %v", err)
		}
		if _, err := h.db.RawDB().Exec(
			`UPDATE outbound_intents
			 SET status='terminal', attempts=3, last_attempt_at=datetime('now')
			 WHERE id = ?`,
			intentID,
		); err != nil {
			t.Fatalf("mark terminal intent: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/conversations/"+run.SessionID+"/deliveries/"+intentID+"/retry", nil)
		req.Header.Set("Authorization", "Bearer "+h.adminToken)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Delivery model.OutboundIntent `json:"delivery"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Delivery.ID != intentID || resp.Delivery.Status != "pending" || resp.Delivery.Attempts != 0 {
			t.Fatalf("unexpected delivery retry response: %+v", resp.Delivery)
		}
		if resp.Delivery.LastAttemptAt != nil {
			t.Fatalf("expected cleared last_attempt_at, got %+v", resp.Delivery)
		}

		detail := httptest.NewRecorder()
		detailReq := httptest.NewRequest(http.MethodGet, "/api/conversations/"+run.SessionID, nil)
		h.server.ServeHTTP(detail, detailReq)
		if detail.Code != http.StatusOK {
			t.Fatalf("expected 200 from detail endpoint, got %d body=%s", detail.Code, detail.Body.String())
		}

		var detailResp struct {
			DeliveryFailures []struct {
				ConnectorID string `json:"connector_id"`
			} `json:"delivery_failures"`
		}
		if err := json.Unmarshal(detail.Body.Bytes(), &detailResp); err != nil {
			t.Fatalf("decode detail response: %v", err)
		}
		if len(detailResp.DeliveryFailures) != 0 {
			t.Fatalf("expected no actionable delivery failures after retry, got %d", len(detailResp.DeliveryFailures))
		}
	})

	t.Run("retry delivery rejects non-terminal outbound intent", func(t *testing.T) {
		h := newServerHarness(t)
		run, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-1",
				ExternalID:  "chat-1",
				ThreadID:    "thread-1",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "Inspect the repo.",
			CWD:           h.workspaceRoot,
		})
		if err != nil {
			t.Fatalf("StartFrontSession failed: %v", err)
		}

		var intentID string
		if err := h.db.RawDB().QueryRow(
			`SELECT id
			 FROM outbound_intents
			 WHERE run_id = ?
			 ORDER BY created_at DESC, id DESC
			 LIMIT 1`,
			run.ID,
		).Scan(&intentID); err != nil {
			t.Fatalf("load outbound intent: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/conversations/"+run.SessionID+"/deliveries/"+intentID+"/retry", nil)
		req.Header.Set("Authorization", "Bearer "+h.adminToken)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d body=%s", rr.Code, rr.Body.String())
		}
	})
}

type serverHarness struct {
	db              *store.DB
	server          *testServer
	rawServer       *Server
	broadcaster     *SSEBroadcaster
	rt              *runtime.Runtime
	adminToken      string
	activeProjectID string
	teamDir         string
	storageRoot     string
	workspaceRoot   string
}

type testServer struct {
	raw        *Server
	adminToken string
}

func (h *serverHarness) projectProfilesRoot() string {
	return filepath.Join(h.storageRoot, "projects", h.activeProjectID, "teams")
}

func (h *serverHarness) projectProfileDir(profile string) string {
	return filepath.Join(h.projectProfilesRoot(), profile)
}

func (s *testServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req := r.Clone(r.Context())
	if req.Header.Get("X-Gistclaw-Test-No-Auto-Auth") == "" && req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer "+s.adminToken)
	}
	s.raw.ServeHTTP(w, req)
}

func (s *testServer) projectLayoutData(r *http.Request) (shellProjectLayout, error) {
	return s.raw.projectLayoutData(r)
}

type stubConnectorHealthSource struct {
	snapshots []model.ConnectorHealthSnapshot
	err       error
}

func (s stubConnectorHealthSource) ConnectorHealth(context.Context) ([]model.ConnectorHealthSnapshot, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.snapshots, nil
}

func newServerHarness(t *testing.T) *serverHarness {
	t.Helper()
	return newServerHarnessWithProviderAndTools(t, runtime.NewMockProvider(nil, nil))
}

func newServerHarnessWithConnectorHealth(t *testing.T, snapshots []model.ConnectorHealthSnapshot) *serverHarness {
	t.Helper()
	return newServerHarnessWithProviderAndConnectorHealth(t, runtime.NewMockProvider(nil, nil), stubConnectorHealthSource{snapshots: snapshots})
}

type blockingProvider struct {
	started chan struct{}
	release chan struct{}
}

func (p *blockingProvider) ID() string {
	return "blocking"
}

func (p *blockingProvider) Generate(ctx context.Context, _ runtime.GenerateRequest, _ runtime.StreamSink) (runtime.GenerateResult, error) {
	select {
	case <-p.started:
	default:
		close(p.started)
	}

	select {
	case <-p.release:
		return runtime.GenerateResult{
			Content:      "mock response",
			InputTokens:  10,
			OutputTokens: 20,
			StopReason:   "end_turn",
		}, nil
	case <-ctx.Done():
		return runtime.GenerateResult{}, ctx.Err()
	}
}

func newServerHarnessWithProvider(t *testing.T, prov runtime.Provider) *serverHarness {
	t.Helper()
	return newServerHarnessWithProviderAndTools(t, prov)
}

func newServerHarnessWithProviderAndConnectorHealth(t *testing.T, prov runtime.Provider, source stubConnectorHealthSource, extraTools ...tools.Tool) *serverHarness {
	t.Helper()

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	adminToken := "test-admin-token"
	storageRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	teamDir := writeTeamFixture(t)
	const activeProjectID = "proj-primary"
	if _, err := db.RawDB().Exec(
		`INSERT INTO projects (id, name, primary_path, roots_json, policy_json, source, created_at, last_used_at)
		 VALUES (?, ?, ?, '{}', '{}', 'starter', datetime('now'), datetime('now'))`,
		activeProjectID, "starter-project", workspaceRoot,
	); err != nil {
		t.Fatalf("seed primary project: %v", err)
	}
	seedSettings(t, db, map[string]string{
		"admin_token":             adminToken,
		"active_project_id":       activeProjectID,
		"team_name":               "Repo Task Team",
		"onboarding_completed_at": "2026-03-25 00:00:00",
	})

	cs := conversations.NewConversationStore(db)
	mem := memory.NewStore(db, cs)
	reg := tools.NewRegistry()
	for _, tool := range extraTools {
		if tool == nil {
			continue
		}
		reg.Register(tool)
	}
	broadcaster := NewSSEBroadcaster()
	rt := runtime.New(db, cs, reg, nil, mem, prov, broadcaster)
	t.Cleanup(func() {
		rt.WaitAsync()
		_ = db.Close()
	})
	rt.SetStorageRoot(storageRoot)
	rt.SetTeamDir(teamDir)
	snapshot, err := teams.LoadExecutionSnapshot(teamDir)
	if err != nil {
		t.Fatalf("load execution snapshot: %v", err)
	}
	if err := rt.SetDefaultExecutionSnapshot(snapshot); err != nil {
		t.Fatalf("set default execution snapshot: %v", err)
	}

	server, err := NewServer(Options{
		DB:              db,
		Replay:          replay.NewService(db),
		Broadcaster:     broadcaster,
		Runtime:         rt,
		StorageRoot:     storageRoot,
		ConnectorHealth: source,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	return &serverHarness{
		db:              db,
		server:          &testServer{raw: server, adminToken: adminToken},
		rawServer:       server,
		broadcaster:     broadcaster,
		rt:              rt,
		adminToken:      adminToken,
		activeProjectID: activeProjectID,
		teamDir:         teamDir,
		storageRoot:     storageRoot,
		workspaceRoot:   workspaceRoot,
	}
}

func newServerHarnessWithProviderAndTools(t *testing.T, prov runtime.Provider, extraTools ...tools.Tool) *serverHarness {
	t.Helper()

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	adminToken := "test-admin-token"
	storageRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	teamDir := writeTeamFixture(t)
	const activeProjectID = "proj-primary"
	if _, err := db.RawDB().Exec(
		`INSERT INTO projects (id, name, primary_path, roots_json, policy_json, source, created_at, last_used_at)
		 VALUES (?, ?, ?, '{}', '{}', 'starter', datetime('now'), datetime('now'))`,
		activeProjectID, "starter-project", workspaceRoot,
	); err != nil {
		t.Fatalf("seed primary project: %v", err)
	}
	seedSettings(t, db, map[string]string{
		"admin_token":             adminToken,
		"active_project_id":       activeProjectID,
		"team_name":               "Repo Task Team",
		"onboarding_completed_at": "2026-03-25 00:00:00",
	})

	cs := conversations.NewConversationStore(db)
	mem := memory.NewStore(db, cs)
	reg := tools.NewRegistry()
	for _, tool := range extraTools {
		if tool == nil {
			continue
		}
		reg.Register(tool)
	}
	broadcaster := NewSSEBroadcaster()
	rt := runtime.New(db, cs, reg, nil, mem, prov, broadcaster)
	t.Cleanup(func() {
		rt.WaitAsync()
		_ = db.Close()
	})
	rt.SetStorageRoot(storageRoot)
	rt.SetTeamDir(teamDir)
	snapshot, err := teams.LoadExecutionSnapshot(teamDir)
	if err != nil {
		t.Fatalf("load execution snapshot: %v", err)
	}
	if err := rt.SetDefaultExecutionSnapshot(snapshot); err != nil {
		t.Fatalf("set default execution snapshot: %v", err)
	}

	server, err := NewServer(Options{
		DB:          db,
		Replay:      replay.NewService(db),
		Broadcaster: broadcaster,
		Runtime:     rt,
		StorageRoot: storageRoot,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	return &serverHarness{
		db:              db,
		server:          &testServer{raw: server, adminToken: adminToken},
		rawServer:       server,
		broadcaster:     broadcaster,
		rt:              rt,
		adminToken:      adminToken,
		activeProjectID: activeProjectID,
		teamDir:         teamDir,
		storageRoot:     storageRoot,
		workspaceRoot:   workspaceRoot,
	}
}

type blockingCoderExecTool struct {
	started chan struct{}
	release chan struct{}
}

func (t *blockingCoderExecTool) Name() string { return "coder_exec" }

func (t *blockingCoderExecTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:       t.Name(),
		Family:     model.ToolFamilyRepoWrite,
		Risk:       model.RiskHigh,
		SideEffect: "exec_write",
		Approval:   "maybe",
	}
}

func (t *blockingCoderExecTool) Invoke(ctx context.Context, _ model.ToolCall) (model.ToolResult, error) {
	meta, ok := tools.InvocationContextFrom(ctx)
	if !ok || meta.CWD == "" {
		return model.ToolResult{}, tools.ErrCWDRequired
	}
	select {
	case <-t.started:
	default:
		close(t.started)
	}
	select {
	case <-t.release:
	case <-ctx.Done():
		return model.ToolResult{}, ctx.Err()
	}
	if err := os.WriteFile(filepath.Join(meta.CWD, "created.txt"), []byte("created by blocking coder\n"), 0o644); err != nil {
		return model.ToolResult{}, err
	}
	return model.ToolResult{
		Output: `{"backend":"codex","command":"codex exec --sandbox workspace-write","cwd":".","stdout":"created created.txt","stderr":"","exit_code":0,"timed_out":false,"truncated":false,"effect":"exec_write"}`,
	}, nil
}

func (h *serverHarness) insertProject(t *testing.T, name, workspaceRoot string) string {
	t.Helper()

	projectID := "proj-" + strconv.Itoa(time.Now().Nanosecond())
	if _, err := h.db.RawDB().Exec(
		`INSERT INTO projects (id, name, primary_path, roots_json, policy_json, source, created_at, last_used_at)
		 VALUES (?, ?, ?, '{}', '{}', 'operator', datetime('now'), datetime('now'))`,
		projectID, name, workspaceRoot,
	); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return projectID
}

func waitForRunStatus(t *testing.T, db *store.DB, runID string, want string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		err := db.RawDB().QueryRow("SELECT status FROM runs WHERE id = ?", runID).Scan(&status)
		if err == nil && status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	var status string
	if err := db.RawDB().QueryRow("SELECT status FROM runs WHERE id = ?", runID).Scan(&status); err != nil {
		t.Fatalf("query final run status: %v", err)
	}
	t.Fatalf("expected run %s to reach status %q, got %q", runID, want, status)
}

func writeTeamFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "team.yaml"), `
name: Repo Task Team
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    base_profile: operator
    tool_families: [repo_read, runtime_capability, connector_capability, delegate]
    delegation_kinds: [write, review]
    can_message: [patcher, reviewer]
    specialist_summary_visibility: full
  - id: patcher
    soul_file: patcher.soul.yaml
    base_profile: write
    tool_families: [repo_read, repo_write]
    can_message: [assistant, reviewer]
    specialist_summary_visibility: basic
  - id: reviewer
    soul_file: reviewer.soul.yaml
    base_profile: review
    tool_families: [repo_read, diff_review]
    can_message: [assistant, patcher]
    specialist_summary_visibility: basic
`)
	writeTestFile(t, filepath.Join(dir, "assistant.soul.yaml"), "role: front assistant\n")
	writeTestFile(t, filepath.Join(dir, "patcher.soul.yaml"), "role: scoped write specialist\n")
	writeTestFile(t, filepath.Join(dir, "reviewer.soul.yaml"), "role: diff reviewer\n")
	return dir
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func seedSettings(t *testing.T, db *store.DB, values map[string]string) {
	t.Helper()

	for key, value := range values {
		_, err := db.RawDB().Exec(
			`INSERT INTO settings (key, value, updated_at)
			 VALUES (?, ?, datetime('now'))
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			key, value,
		)
		if err != nil {
			t.Fatalf("seed setting %s: %v", key, err)
		}
	}
}

func (h *serverHarness) setAdminToken(t *testing.T, token string) {
	t.Helper()
	seedSettings(t, h.db, map[string]string{"admin_token": token})
}

func hostAdminSessionCookie(t *testing.T, h *serverHarness, pageURL string) *http.Cookie {
	t.Helper()

	if err := authpkg.SetPassword(context.Background(), h.db, "test-password", time.Now().UTC()); err != nil {
		t.Fatalf("set password: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/api/auth/login", strings.NewReader(`{"password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected login to succeed before POST %s, got %d body=%s", pageURL, rr.Code, rr.Body.String())
	}

	cookie := findCookie(rr.Result().Cookies(), sessionCookieName)
	if cookie == nil {
		t.Fatalf("expected %s cookie after login before %s", sessionCookieName, pageURL)
	}
	return cookie
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func (h *serverHarness) insertRun(t *testing.T, runID, conversationID, objective, status string) {
	t.Helper()

	_, err := h.db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, project_id, team_id, objective, cwd, status, created_at, updated_at)
		 VALUES (?, ?, 'agent-1', ?, 'repo-task-team', ?, ?, ?, datetime('now'), datetime('now'))`,
		runID, conversationID, h.activeProjectID, objective, h.workspaceRoot, status,
	)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}
}

func (h *serverHarness) insertRunAt(t *testing.T, runID, conversationID, objective, status, createdAt string) {
	t.Helper()

	_, err := h.db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, project_id, team_id, objective, cwd, status, created_at, updated_at)
		 VALUES (?, ?, 'agent-1', ?, 'repo-task-team', ?, ?, ?, ?, ?)`,
		runID, conversationID, h.activeProjectID, objective, h.workspaceRoot, status, createdAt, createdAt,
	)
	if err != nil {
		t.Fatalf("insert run at %s: %v", createdAt, err)
	}
}

func (h *serverHarness) insertRunInWorkspace(t *testing.T, runID, conversationID, objective, status, workspaceRoot string) {
	t.Helper()

	projectID := h.lookupProjectID(t, workspaceRoot)

	_, err := h.db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, project_id, team_id, objective, cwd, status, created_at, updated_at)
		 VALUES (?, ?, 'agent-1', ?, 'repo-task-team', ?, ?, ?, datetime('now'), datetime('now'))`,
		runID, conversationID, projectID, objective, workspaceRoot, status,
	)
	if err != nil {
		t.Fatalf("insert run in workspace: %v", err)
	}
}

func (h *serverHarness) insertRunWithSnapshotAt(t *testing.T, runID, conversationID, objective, status, createdAt, updatedAt string, snapshot model.ExecutionSnapshot) {
	t.Helper()

	rawSnapshot, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal execution snapshot: %v", err)
	}

	_, err = h.db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, project_id, team_id, objective, cwd, status, execution_snapshot_json, created_at, updated_at)
		 VALUES (?, ?, 'agent-1', ?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, conversationID, h.activeProjectID, snapshot.TeamID, objective, h.workspaceRoot, status, rawSnapshot, createdAt, updatedAt,
	)
	if err != nil {
		t.Fatalf("insert run with snapshot: %v", err)
	}
}

func (h *serverHarness) insertRunWithSession(t *testing.T, runID, conversationID, sessionID, objective, status string) {
	t.Helper()

	_, err := h.db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, session_id, project_id, team_id, objective, cwd, status, created_at, updated_at)
		 VALUES (?, ?, 'agent-1', ?, ?, 'repo-task-team', ?, ?, ?, datetime('now'), datetime('now'))`,
		runID, conversationID, sessionID, h.activeProjectID, objective, h.workspaceRoot, status,
	)
	if err != nil {
		t.Fatalf("insert session run: %v", err)
	}
}

func (h *serverHarness) lookupProjectID(t *testing.T, workspaceRoot string) string {
	t.Helper()

	var projectID string
	_ = h.db.RawDB().QueryRow(`SELECT id FROM projects WHERE primary_path = ?`, workspaceRoot).Scan(&projectID)
	if strings.TrimSpace(projectID) != "" {
		return projectID
	}
	return h.insertProject(t, filepath.Base(workspaceRoot), workspaceRoot)
}

func (h *serverHarness) seedRoutesDeliveriesData(t *testing.T) (model.Run, model.RouteDirectoryItem, string) {
	t.Helper()

	run, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "telegram",
			AccountID:   "acct-1",
			ExternalID:  "chat-1",
			ThreadID:    "thread-1",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "Inspect Telegram.",
		CWD:           h.workspaceRoot,
	})
	if err != nil {
		t.Fatalf("StartFrontSession telegram failed: %v", err)
	}

	routes, err := h.rt.ListRoutes(context.Background(), sessions.RouteListFilter{
		ConnectorID: "telegram",
		Status:      "active",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("list routes: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 active telegram route, got %d", len(routes))
	}

	var intentID string
	if err := h.db.RawDB().QueryRow(
		`SELECT id
		 FROM outbound_intents
		 WHERE run_id = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		run.ID,
	).Scan(&intentID); err != nil {
		t.Fatalf("load outbound intent: %v", err)
	}

	return run, routes[0], intentID
}

func (h *serverHarness) markOutboundIntentTerminal(t *testing.T, intentID string) {
	t.Helper()

	if _, err := h.db.RawDB().Exec(
		`UPDATE outbound_intents
		 SET status='terminal', attempts=3, last_attempt_at=datetime('now')
		 WHERE id = ?`,
		intentID,
	); err != nil {
		t.Fatalf("mark outbound intent terminal: %v", err)
	}
}

func (h *serverHarness) insertApproval(t *testing.T, runID, toolName, targetPath string) string {
	t.Helper()
	h.ensureRunForApproval(t, runID)

	id := "ticket-" + runID
	bindingJSON := approvalBindingJSONForTarget(t, toolName, targetPath)
	_, err := h.db.RawDB().Exec(
		`INSERT INTO approvals (id, run_id, tool_name, args_json, binding_json, fingerprint, status, created_at)
		 VALUES (?, ?, ?, x'', ?, 'fp-test', 'pending', datetime('now'))`,
		id, runID, toolName, bindingJSON,
	)
	if err != nil {
		t.Fatalf("insert approval: %v", err)
	}
	return id
}

func (h *serverHarness) insertApprovalAt(t *testing.T, runID, toolName, targetPath, status, resolvedBy, createdAt string) string {
	t.Helper()
	h.ensureRunForApproval(t, runID)

	id := "ticket-" + runID
	var resolvedAt any
	if resolvedBy != "" {
		resolvedAt = createdAt
	}
	bindingJSON := approvalBindingJSONForTarget(t, toolName, targetPath)

	_, err := h.db.RawDB().Exec(
		`INSERT INTO approvals (id, run_id, tool_name, args_json, binding_json, fingerprint, status, resolved_by, created_at, resolved_at)
		 VALUES (?, ?, ?, x'', ?, 'fp-test', ?, NULLIF(?, ''), ?, ?)`,
		id, runID, toolName, bindingJSON, status, resolvedBy, createdAt, resolvedAt,
	)
	if err != nil {
		t.Fatalf("insert approval at %s: %v", createdAt, err)
	}
	return id
}

func (h *serverHarness) ensureRunForApproval(t *testing.T, runID string) {
	t.Helper()

	var count int
	if err := h.db.RawDB().QueryRow(`SELECT count(*) FROM runs WHERE id = ?`, runID).Scan(&count); err != nil {
		t.Fatalf("count approval run: %v", err)
	}
	if count > 0 {
		return
	}
	h.insertRun(t, runID, "conv-"+runID, "approval test run", "needs_approval")
}

func approvalBindingJSONForTarget(t *testing.T, toolName, targetPath string) []byte {
	t.Helper()

	payload, err := json.Marshal(authority.Binding{
		ToolName: toolName,
		Operands: []string{targetPath},
		Mutating: true,
	})
	if err != nil {
		t.Fatalf("marshal approval binding: %v", err)
	}
	return payload
}

func (h *serverHarness) insertEventAt(t *testing.T, eventID, conversationID, runID, kind, createdAt string) {
	t.Helper()
	h.insertEventAtWithPayload(t, eventID, conversationID, runID, kind, nil, createdAt)
}

func (h *serverHarness) insertEventAtWithPayload(t *testing.T, eventID, conversationID, runID, kind string, payload []byte, createdAt string) {
	t.Helper()
	if len(payload) == 0 {
		payload = []byte{}
	}
	_, err := h.db.RawDB().Exec(
		`INSERT INTO events (id, conversation_id, run_id, kind, payload_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		eventID, conversationID, runID, kind, payload, createdAt,
	)
	if err != nil {
		t.Fatalf("insert event with payload: %v", err)
	}
}

func (h *serverHarness) startFrontSession(t *testing.T, prompt string) model.Run {
	t.Helper()

	run, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: prompt,
		CWD:           h.workspaceRoot,
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}
	return run
}

func (h *serverHarness) spawnWorkerSession(t *testing.T, controllerSessionID, agentID, prompt string) model.Run {
	t.Helper()

	run, err := h.rt.Spawn(context.Background(), runtime.SpawnCommand{
		ControllerSessionID: controllerSessionID,
		AgentID:             agentID,
		Prompt:              prompt,
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	return run
}

func subscribeSSE(t *testing.T, url string) (*http.Response, *bufio.Reader) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new sse request: %v", err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("subscribe sse: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	return res, bufio.NewReader(res.Body)
}

func assertEventContains(t *testing.T, reader *bufio.Reader, want string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read sse line: %v", err)
		}
		if strings.Contains(line, want) {
			return
		}
	}

	t.Fatalf("did not receive SSE line containing %q", want)
}

func readSSEEvent(t *testing.T, reader *bufio.Reader) map[string]any {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read sse line: %v", err)
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		payloadLine := strings.TrimPrefix(strings.TrimSpace(line), "data: ")
		var event map[string]any
		if err := json.Unmarshal([]byte(payloadLine), &event); err != nil {
			t.Fatalf("decode sse event: %v", err)
		}
		return event
	}

	t.Fatal("did not receive SSE event")
	return nil
}

func readSSEEventWithin(t *testing.T, resp *http.Response, reader *bufio.Reader, timeout time.Duration) map[string]any {
	t.Helper()

	type result struct {
		event map[string]any
		err   error
	}

	ch := make(chan result, 1)
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				ch <- result{err: err}
				return
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			payloadLine := strings.TrimPrefix(strings.TrimSpace(line), "data: ")
			var event map[string]any
			if err := json.Unmarshal([]byte(payloadLine), &event); err != nil {
				ch <- result{err: err}
				return
			}
			ch <- result{event: event}
			return
		}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatalf("read sse event: %v", res.err)
		}
		return res.event
	case <-time.After(timeout):
		_ = resp.Body.Close()
		t.Fatalf("did not receive SSE event within %s", timeout)
	}

	return nil
}

func waitForSubscribers(t *testing.T, broadcaster *SSEBroadcaster, runID string, want int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		broadcaster.mu.RLock()
		got := len(broadcaster.subscribers[runID])
		broadcaster.mu.RUnlock()
		if got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	broadcaster.mu.RLock()
	got := len(broadcaster.subscribers[runID])
	broadcaster.mu.RUnlock()
	t.Fatalf("expected %d subscribers, got %d", want, got)
}
