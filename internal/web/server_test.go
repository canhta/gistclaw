package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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
	t.Run("list renders runs", func(t *testing.T) {
		h := newServerHarness(t)
		h.insertRun(t, "run-known", "conv-1", "review the repo", "active")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/operate/runs", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}

		body := rr.Body.String()
		for _, want := range []string{"Runs", "Live orchestration strip", "run-known", "review the repo"} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected body to contain %q:\n%s", want, body)
			}
		}
		if !strings.Contains(body, `href="/operate/runs/run-known"`) {
			t.Fatalf("expected runs list to link to run detail:\n%s", body)
		}
	})

	t.Run("list renders front session summary", func(t *testing.T) {
		h := newServerHarness(t)
		h.insertRun(t, "run-front", "conv-1", "review the repo", "active")
		_, err := h.db.RawDB().Exec(
			`INSERT INTO runs
			 (id, conversation_id, agent_id, parent_run_id, team_id, objective, workspace_root, status, created_at, updated_at)
			 VALUES ('run-worker', 'conv-1', 'researcher', 'run-front', 'repo-task-team', 'inspect docs', ?, 'completed', datetime('now'), datetime('now'))`,
			h.workspaceRoot,
		)
		if err != nil {
			t.Fatalf("insert worker run: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/operate/runs", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "front session with 1 worker run") {
			t.Fatalf("expected front-session summary, got:\n%s", rr.Body.String())
		}
	})

	t.Run("list renders empty state", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/operate/runs", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		for _, want := range []string{"Live orchestration strip", "No runs yet.", `href="/configure/team"`} {
			if !strings.Contains(rr.Body.String(), want) {
				t.Fatalf("expected empty state to contain %q, got:\n%s", want, rr.Body.String())
			}
		}
		if strings.Contains(rr.Body.String(), `href="/operate/start-task"`) {
			t.Fatalf("expected runs empty state to omit start-task CTA, got:\n%s", rr.Body.String())
		}
	})

	t.Run("list filters and paginates runs", func(t *testing.T) {
		h := newServerHarness(t)
		h.insertRunAt(t, "run-newest", "conv-runs-1", "fix graph cards", "active", "2026-03-25 10:00:00")
		h.insertRunAt(t, "run-middle", "conv-runs-2", "review docs", "completed", "2026-03-25 09:00:00")
		h.insertRunAt(t, "run-oldest", "conv-runs-3", "fix pagination", "active", "2026-03-25 08:00:00")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/operate/runs?q=fix&status=active&limit=1", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "run-newest") {
			t.Fatalf("expected first filtered page to contain newest run, got:\n%s", body)
		}
		if strings.Contains(body, "run-middle") || strings.Contains(body, "run-oldest") {
			t.Fatalf("expected first filtered page to contain only newest filtered run, got:\n%s", body)
		}
		if !strings.Contains(body, "direction=next") || !strings.Contains(body, "limit=1") {
			t.Fatalf("expected next-page controls in first page, got:\n%s", body)
		}

		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/operate/runs?q=fix&status=active&limit=1&cursor=2026-03-25+10%3A00%3A00%7Crun-newest&direction=next", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected second page 200, got %d", rr.Code)
		}
		body = rr.Body.String()
		if !strings.Contains(body, `href="/operate/runs/run-oldest"`) {
			t.Fatalf("expected second filtered page to contain oldest run, got:\n%s", body)
		}
		if strings.Contains(body, `href="/operate/runs/run-newest"`) || strings.Contains(body, `href="/operate/runs/run-middle"`) {
			t.Fatalf("expected second filtered page to contain only oldest filtered run, got:\n%s", body)
		}
	})

	t.Run("list defaults to active project", func(t *testing.T) {
		h := newServerHarness(t)
		otherRoot := t.TempDir()
		h.insertProject(t, "seo-test", otherRoot)
		h.insertRun(t, "run-active-project", "conv-active-project", "review main project", "active")
		h.insertRunInWorkspace(t, "run-other-project", "conv-other-project", "review seo project", "active", otherRoot)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/operate/runs", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "run-active-project") {
			t.Fatalf("expected active project run to be visible, got:\n%s", body)
		}
		if strings.Contains(body, "run-other-project") {
			t.Fatalf("expected non-active project runs to be hidden, got:\n%s", body)
		}
	})

	t.Run("detail renders known run", func(t *testing.T) {
		h := newServerHarness(t)
		snapshot, err := teams.LoadExecutionSnapshot(h.teamDir)
		if err != nil {
			t.Fatalf("load execution snapshot: %v", err)
		}
		h.insertRunWithSnapshotAt(
			t,
			"082b1c314823744cc779ece2f90e80e7",
			"conv-1",
			"review the repo",
			"completed",
			"2026-03-25 08:00:00",
			"2026-03-25 08:06:00",
			snapshot,
		)
		h.insertEventAt(t, "evt-1", "conv-1", "082b1c314823744cc779ece2f90e80e7", "run_started", "2026-03-25 08:00:00")
		h.insertEventWithPayload(
			t,
			"evt-2",
			"conv-1",
			"082b1c314823744cc779ece2f90e80e7",
			"turn_completed",
			[]byte(`{"content":"Draft the rollout plan.","input_tokens":12,"output_tokens":8}`),
		)
		if _, err := h.db.RawDB().Exec(
			`UPDATE events SET created_at = ? WHERE id = ?`,
			"2026-03-25 08:06:00",
			"evt-2",
		); err != nil {
			t.Fatalf("set event timestamp: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/operate/runs/082b1c314823744cc779ece2f90e80e7", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}

		body := rr.Body.String()
		for _, want := range []string{
			"082b1c314823744cc779ece2f90e80e7",
			"Execution Snapshot",
			"Repo Task Team",
			"assistant",
			"workspace_write",
			"Started",
			"Last activity",
			"Recorded events",
			"Completed turns",
			"2026-03-25 08:00:00 UTC",
			"2026-03-25 08:06:00 UTC",
			"Run started",
			"082b1c31…80e7",
			"Updated",
			"Draft the rollout plan.",
			`id="run-live-output"`,
			`id="run-started-at"`,
			`id="run-last-activity"`,
			`id="run-graph-diagram"`,
			`id="run-graph-board"`,
			`data-graph-url="/operate/runs/082b1c314823744cc779ece2f90e80e7/graph"`,
			`/operate/runs/082b1c314823744cc779ece2f90e80e7/events`,
			`window.cytoscape`,
			`fetch(graphURL`,
			`new EventSource(streamURL)`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected body to contain %q:\n%s", want, body)
			}
		}
	})

	t.Run("detail hides non-active project run", func(t *testing.T) {
		h := newServerHarness(t)
		otherRoot := t.TempDir()
		h.insertProject(t, "seo-test", otherRoot)
		h.insertRunInWorkspace(t, "run-other-project-detail", "conv-other-project-detail", "review seo project", "active", otherRoot)
		h.insertEventAt(t, "evt-other-project-detail", "conv-other-project-detail", "run-other-project-detail", "run_started", "2026-03-25 08:00:00")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/operate/runs/run-other-project-detail", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for non-active project run detail, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("graph endpoint renders lineage snapshot with statuses", func(t *testing.T) {
		h := newServerHarness(t)
		h.insertRunAt(t, "082b1c314823744cc779ece2f90e80e7", "conv-graph", "review the repo", "active", "2026-03-25 08:00:00")
		if _, err := h.db.RawDB().Exec(
			`UPDATE runs SET updated_at = ? WHERE id = ?`,
			"2026-03-25 08:02:00",
			"082b1c314823744cc779ece2f90e80e7",
		); err != nil {
			t.Fatalf("set root updated_at: %v", err)
		}
		if _, err := h.db.RawDB().Exec(
			`INSERT INTO runs
			 (id, conversation_id, agent_id, session_id, team_id, parent_run_id, objective, workspace_root, status, created_at, updated_at)
			 VALUES (?, ?, 'researcher', 'sess-worker-4ed077c29497f4c95a19125b86096953', 'repo-task-team', ?, ?, ?, ?, ?, ?)`,
			"4ed077c29497f4c95a19125b86096953",
			"conv-graph",
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
		req := httptest.NewRequest(http.MethodGet, "/operate/runs/082b1c314823744cc779ece2f90e80e7/graph", nil)

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
				From string `json:"from"`
				To   string `json:"to"`
			} `json:"edges"`
			Columns []struct {
				Depth int `json:"depth"`
				Nodes []struct {
					ID             string `json:"id"`
					ShortID        string `json:"short_id"`
					Status         string `json:"status"`
					StatusClass    string `json:"status_class"`
					StatusLabel    string `json:"status_label"`
					UpdatedAtLabel string `json:"updated_at_label"`
				} `json:"nodes"`
			} `json:"columns"`
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
		if len(resp.Columns) != 2 {
			t.Fatalf("expected 2 graph columns, got %d", len(resp.Columns))
		}
		if resp.Columns[0].Nodes[0].ID != "082b1c314823744cc779ece2f90e80e7" || resp.Columns[1].Nodes[0].ID != "4ed077c29497f4c95a19125b86096953" {
			t.Fatalf("unexpected graph nodes: %+v", resp.Columns)
		}
		if len(resp.Edges) != 1 || resp.Edges[0].From != "082b1c314823744cc779ece2f90e80e7" || resp.Edges[0].To != "4ed077c29497f4c95a19125b86096953" {
			t.Fatalf("unexpected graph edges: %+v", resp.Edges)
		}
		if resp.Columns[1].Nodes[0].StatusClass != "is-approval" {
			t.Fatalf("expected approval status class for worker node, got %+v", resp.Columns[1].Nodes[0])
		}
		if resp.Columns[0].Nodes[0].ShortID != "082b1c31…80e7" {
			t.Fatalf("expected short root id, got %+v", resp.Columns[0].Nodes[0])
		}
		if resp.Columns[1].Nodes[0].StatusLabel != "needs approval" {
			t.Fatalf("expected humanized status label, got %+v", resp.Columns[1].Nodes[0])
		}
		if resp.Columns[1].Nodes[0].UpdatedAtLabel != "2026-03-25 08:05:00 UTC" {
			t.Fatalf("expected updated-at label, got %+v", resp.Columns[1].Nodes[0])
		}
	})

	t.Run("detail missing run returns not found", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/operate/runs/missing", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rr.Code)
		}
	})
}

func TestSessionsProjectScoping(t *testing.T) {
	t.Run("sessions page hides other project sessions", func(t *testing.T) {
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
			WorkspaceRoot: otherRoot,
		})
		if err != nil {
			t.Fatalf("start other-project front session: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/operate/sessions", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, mainRun.SessionID) {
			t.Fatalf("expected active project session to be visible, got:\n%s", body)
		}
		if strings.Contains(body, otherRun.SessionID) || strings.Contains(body, otherRun.ConversationID) {
			t.Fatalf("expected other project session to be hidden, got:\n%s", body)
		}
	})

	t.Run("session detail hides other project session", func(t *testing.T) {
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
			WorkspaceRoot: otherRoot,
		})
		if err != nil {
			t.Fatalf("start other-project front session: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/operate/sessions/"+otherRun.SessionID, nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for non-active project session detail, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("team page follows active project workspace", func(t *testing.T) {
		h := newServerHarness(t)
		otherRoot := t.TempDir()
		otherProjectID := h.insertProject(t, "seo-test", otherRoot)
		writeTestFile(t, filepath.Join(otherRoot, ".gistclaw", "teams", "default", "team.yaml"), `
name: SEO Task Team
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    role: coordinator
    tool_posture: read_heavy
`)
		writeTestFile(t, filepath.Join(otherRoot, ".gistclaw", "teams", "default", "assistant.soul.yaml"), "role: coordinator\ntool_posture: read_heavy\n")
		if err := runtime.SetActiveProject(context.Background(), h.db, otherProjectID); err != nil {
			t.Fatalf("set active project: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/configure/team", nil)

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
		WorkspaceRoot: otherRoot,
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
	req := httptest.NewRequest(http.MethodGet, "/recover/routes-deliveries?connector_id=telegram&route_status=all&delivery_status=all", nil)

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
			WorkspaceRoot: otherRoot,
		})
		if err != nil {
			t.Fatalf("start other-project front session: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+otherRun.SessionID, nil)

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
			WorkspaceRoot: otherRoot,
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
			WorkspaceRoot: otherRoot,
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
	t.Run("root redirects to operate runs", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303, got %d", rr.Code)
		}
		if rr.Header().Get("Location") != "/operate/runs" {
			t.Fatalf("expected redirect to /operate/runs, got %q", rr.Header().Get("Location"))
		}
	})

	t.Run("new grouped page routes render without legacy aliases", func(t *testing.T) {
		h := newServerHarness(t)

		cases := []struct {
			path string
			want []string
		}{
			{path: "/operate/runs", want: []string{"Operate", "Configure", "Recover", `href="/operate/runs"`, `href="/operate/sessions"`, `name="project_id"`}},
			{path: "/configure/team", want: []string{"Team", `href="/configure/team"`, `href="/configure/settings"`}},
			{path: "/recover/routes-deliveries", want: []string{"Routes &amp; Deliveries", `href="/recover/approvals"`}},
			{path: "/operate/start-task", want: []string{"Start Task"}},
		}

		for _, tc := range cases {
			t.Run(tc.path, func(t *testing.T) {
				rr := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, tc.path, nil)

				h.server.ServeHTTP(rr, req)

				if rr.Code != http.StatusOK {
					t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
				}
				body := rr.Body.String()
				for _, want := range tc.want {
					if !strings.Contains(body, want) {
						t.Fatalf("expected %s to contain %q:\n%s", tc.path, want, body)
					}
				}
			})
		}
	})

	t.Run("legacy page routes are gone", func(t *testing.T) {
		h := newServerHarness(t)

		for _, path := range []string{
			"/runs",
			"/sessions",
			"/team",
			"/control",
			"/approvals",
			"/settings",
			"/memory",
			"/run",
		} {
			t.Run(path, func(t *testing.T) {
				rr := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, path, nil)

				h.server.ServeHTTP(rr, req)

				if rr.Code != http.StatusNotFound {
					t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
				}
			})
		}
	})
}

func TestApprovals(t *testing.T) {
	t.Run("page renders approvals", func(t *testing.T) {
		h := newServerHarness(t)
		h.insertApprovalAt(t, "run-approval-ui", "apply_patch", "internal/web/templates/team.html", "pending", "", "2026-03-25 10:00:00")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/recover/approvals", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		for _, want := range []string{
			"Approvals",
			`class="approval-filter-grid"`,
			`filter-action-group`,
			`class="approval-filter-actions"`,
			`class="field-label field-label-ghost"`,
			`approval-card-actions`,
			`data-confirm="Approve this approval ticket? This action resolves it immediately."`,
			`data-confirm="Deny this approval ticket? This action resolves it immediately."`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected approvals page to contain %q:\n%s", want, body)
			}
		}
	})

	t.Run("page renders empty recovery state", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/recover/approvals", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		for _, want := range []string{"No approval work right now.", `href="/operate/runs"`} {
			if !strings.Contains(rr.Body.String(), want) {
				t.Fatalf("expected empty approvals state to contain %q, got:\n%s", want, rr.Body.String())
			}
		}
	})

	t.Run("page filters and paginates approvals", func(t *testing.T) {
		h := newServerHarness(t)
		h.insertApprovalAt(t, "run-approval-new", "bash", "/tmp/new", "pending", "", "2026-03-25 10:00:00")
		h.insertApprovalAt(t, "run-approval-mid", "git", "/tmp/mid", "approved", "operator", "2026-03-25 09:00:00")
		h.insertApprovalAt(t, "run-approval-old", "bash", "/tmp/old", "pending", "", "2026-03-25 08:00:00")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/recover/approvals?q=bash&status=pending&limit=1", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, `action="/recover/approvals/ticket-run-approval-new/resolve"`) {
			t.Fatalf("expected first approval page to contain newest pending approval, got:\n%s", body)
		}
		if strings.Contains(body, `action="/recover/approvals/ticket-run-approval-mid/resolve"`) || strings.Contains(body, `action="/recover/approvals/ticket-run-approval-old/resolve"`) {
			t.Fatalf("expected first approval page to contain only newest filtered approval, got:\n%s", body)
		}
		if !strings.Contains(body, "direction=next") || !strings.Contains(body, "limit=1") {
			t.Fatalf("expected next-page controls in first approval page, got:\n%s", body)
		}

		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/recover/approvals?q=bash&status=pending&limit=1&cursor=2026-03-25+10%3A00%3A00%7Cticket-run-approval-new&direction=next", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected second approval page 200, got %d", rr.Code)
		}
		body = rr.Body.String()
		if !strings.Contains(body, `action="/recover/approvals/ticket-run-approval-old/resolve"`) {
			t.Fatalf("expected second approval page to contain older pending approval, got:\n%s", body)
		}
		if strings.Contains(body, `action="/recover/approvals/ticket-run-approval-new/resolve"`) || strings.Contains(body, `action="/recover/approvals/ticket-run-approval-mid/resolve"`) {
			t.Fatalf("expected second approval page to contain only older filtered approval, got:\n%s", body)
		}
	})
}

func TestSettings(t *testing.T) {
	h := newServerHarness(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/configure/settings", nil)

	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Settings") {
		t.Fatalf("expected settings page, got:\n%s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Team composition and agent collaboration defaults now live in Configure &gt; Team.") {
		t.Fatalf("expected settings page to use non-interactive team guidance, got:\n%s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `Team composition and agent collaboration defaults now live on the <a href="/configure/team">Team</a> page.`) {
		t.Fatalf("expected settings page to avoid duplicate inline Team navigation, got:\n%s", rr.Body.String())
	}
}

func TestTeam(t *testing.T) {
	t.Run("team page renders configured default team", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/configure/team", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}

		body := rr.Body.String()
		for _, want := range []string{
			"Team",
			"Repo Task Team",
			"assistant",
			"patcher",
			"reviewer",
			"operator_facing",
			"workspace_write",
			`name="agent_0_tool_posture"`,
			`name="agent_0_can_spawn"`,
			`name="agent_0_can_message"`,
			`name="agent_0_id"`,
			`name="agent_0_soul_file"`,
			`type="checkbox"`,
			`<option value="read_heavy"`,
			`value="reviewer" checked`,
			`Add Member`,
			`/configure/team/export`,
			`name="import_file"`,
			`type="hidden" name="agent_count" value="3"`,
			`class="team-summary-head"`,
			`class="team-file-tools"`,
			`class="team-primary-actions"`,
			`class="team-utility-bar"`,
			`data-confirm="Add a new team member to the editor?"`,
			`data-confirm="Import this team file into the editor? Unsaved changes in the current editor will be replaced."`,
			`data-confirm="Remove assistant from this team? This stays in the editor until you save."`,
			`data-confirm="Save this team to the workspace-owned runtime copy?"`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected body to contain %q:\n%s", want, body)
			}
		}
		for _, unwanted := range []string{
			`name="agent_0_can_spawn" type="text"`,
			`name="agent_0_can_message" type="text"`,
		} {
			if strings.Contains(body, unwanted) {
				t.Fatalf("expected team page to avoid free-text relationship fields %q:\n%s", unwanted, body)
			}
		}
	})

	t.Run("team update rewrites default team and refreshes runtime snapshot", func(t *testing.T) {
		h := newServerHarness(t)
		cookie := hostAdminSessionCookie(t, h, "/configure/team")

		cfg, err := teams.LoadConfig(h.teamDir)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		form := teamFormValues(cfg)
		form.Set("intent", "save")
		form.Set("name", "Platform Crew")
		form.Set("front_agent", "reviewer")
		form.Set("agent_0_role", "operator-facing coordinator")
		form.Set("agent_0_tool_posture", "operator_facing")
		form.Del("agent_0_can_spawn")
		form.Add("agent_0_can_spawn", "patcher")
		form.Add("agent_0_can_spawn", "reviewer")
		form.Del("agent_0_can_message")
		form.Add("agent_0_can_message", "patcher")
		form.Add("agent_0_can_message", "reviewer")
		form.Set("agent_1_role", "workspace write specialist")
		form.Set("agent_1_tool_posture", "workspace_write")
		form.Del("agent_1_can_message")
		form.Add("agent_1_can_message", "assistant")
		form.Add("agent_1_can_message", "reviewer")
		form.Set("agent_2_role", "diff reviewer")
		form.Set("agent_2_tool_posture", "read_heavy")
		form.Del("agent_2_can_message")
		form.Add("agent_2_can_message", "assistant")
		form.Add("agent_2_can_message", "patcher")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/configure/team", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://example.com")
		req.Host = "example.com"
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d body=%s", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Location") != "/configure/team" {
			t.Fatalf("expected redirect to /configure/team, got %q", rr.Header().Get("Location"))
		}

		specData, err := os.ReadFile(filepath.Join(h.teamDir, "team.yaml"))
		if err != nil {
			t.Fatalf("read team file: %v", err)
		}
		spec, err := teams.LoadSpec(specData)
		if err != nil {
			t.Fatalf("load team spec: %v", err)
		}
		if spec.Name != "Platform Crew" {
			t.Fatalf("expected updated team name, got %q", spec.Name)
		}
		if spec.FrontAgent != "reviewer" {
			t.Fatalf("expected updated front agent, got %q", spec.FrontAgent)
		}
		if got := spec.Agents[0].CanSpawn; len(got) != 2 || got[0] != "patcher" || got[1] != "reviewer" {
			t.Fatalf("expected assistant can_spawn [patcher reviewer], got %#v", got)
		}
		if got := spec.Agents[1].CanSpawn; len(got) != 0 {
			t.Fatalf("expected patcher can_spawn empty, got %#v", got)
		}
		if got := spec.Agents[2].CanMessage; len(got) != 2 || got[0] != "assistant" || got[1] != "patcher" {
			t.Fatalf("expected reviewer can_message [assistant patcher], got %#v", got)
		}

		run, err := h.rt.Start(context.Background(), runtime.StartRun{
			ConversationID: "conv-team-refresh",
			AgentID:        "reviewer",
			Objective:      "confirm refreshed snapshot",
			WorkspaceRoot:  h.workspaceRoot,
			PreviewOnly:    true,
		})
		if err != nil {
			t.Fatalf("start run with refreshed snapshot: %v", err)
		}
		if run.TeamID != "Platform Crew" {
			t.Fatalf("expected refreshed runtime team id, got %q", run.TeamID)
		}
	})

	t.Run("add member reshapes editor without persisting until save", func(t *testing.T) {
		h := newServerHarness(t)
		cookie := hostAdminSessionCookie(t, h, "/configure/team")

		cfg, err := teams.LoadConfig(h.teamDir)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		form := teamFormValues(cfg)
		form.Set("intent", "add_member")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/configure/team", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://example.com")
		req.Host = "example.com"
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{
			"New member added. Save Team to apply the change.",
			`name="agent_3_id"`,
			`value="agent_1"`,
			`value="agent_1.soul.yaml"`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected add-member response to contain %q:\n%s", want, body)
			}
		}

		reloaded, err := teams.LoadConfig(h.teamDir)
		if err != nil {
			t.Fatalf("reload config: %v", err)
		}
		if len(reloaded.Agents) != 3 {
			t.Fatalf("expected add-member action to keep stored team unchanged until save, got %d agents", len(reloaded.Agents))
		}
	})

	t.Run("remove member blocks deleting current front agent without reassignment", func(t *testing.T) {
		h := newServerHarness(t)
		cookie := hostAdminSessionCookie(t, h, "/configure/team")

		cfg, err := teams.LoadConfig(h.teamDir)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		form := teamFormValues(cfg)
		form.Set("intent", "remove_member")
		form.Set("remove_agent_index", "0")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/configure/team", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://example.com")
		req.Host = "example.com"
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d body=%s", rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "Choose another front agent before removing assistant.") {
			t.Fatalf("expected front-agent guardrail, got:\n%s", rr.Body.String())
		}
	})

	t.Run("remove member with reassigned front agent updates editor state", func(t *testing.T) {
		h := newServerHarness(t)
		cookie := hostAdminSessionCookie(t, h, "/configure/team")

		cfg, err := teams.LoadConfig(h.teamDir)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		form := teamFormValues(cfg)
		form.Set("front_agent", "patcher")
		form.Set("intent", "remove_member")
		form.Set("remove_agent_index", "0")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/configure/team", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://example.com")
		req.Host = "example.com"
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		if strings.Contains(body, `name="agent_0_id" value="assistant"`) {
			t.Fatalf("expected removed assistant to be absent from unsaved editor state:\n%s", body)
		}
		for _, want := range []string{
			"Member removed. Save Team to apply the change.",
			`type="hidden" name="agent_count" value="2"`,
			`<option value="patcher" selected>`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected remove-member response to contain %q:\n%s", want, body)
			}
		}
	})

	t.Run("team export downloads editable team file", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/configure/team/export", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		if got := rr.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") || !strings.Contains(got, "team.yaml") {
			t.Fatalf("expected attachment header for team export, got %q", got)
		}
		for _, want := range []string{
			"name: Repo Task Team",
			"role: operator-facing coordinator",
			"tool_posture: operator_facing",
			"soul_file: coordinator.soul.yaml",
		} {
			if !strings.Contains(rr.Body.String(), want) {
				t.Fatalf("expected export to contain %q:\n%s", want, rr.Body.String())
			}
		}
	})

	t.Run("team import loads editable team file into the editor", func(t *testing.T) {
		h := newServerHarness(t)
		cookie := hostAdminSessionCookie(t, h, "/configure/team")

		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if err := writer.WriteField("intent", "import"); err != nil {
			t.Fatalf("write intent field: %v", err)
		}
		part, err := writer.CreateFormFile("import_file", "team.yaml")
		if err != nil {
			t.Fatalf("create import file: %v", err)
		}
		imported := `
name: Imported Crew
front_agent: reviewer
agents:
  - id: reviewer
    soul_file: reviewer.soul.yaml
    role: imported reviewer
    tool_posture: read_heavy
    can_spawn: [assistant]
    can_message: [assistant, patcher]
  - id: patcher
    soul_file: patcher.soul.yaml
    role: imported patcher
    tool_posture: workspace_write
    can_spawn: []
    can_message: [reviewer]
  - id: assistant
    soul_file: coordinator.soul.yaml
    role: imported coordinator
    tool_posture: operator_facing
    can_spawn: [patcher, reviewer]
    can_message: [patcher, reviewer]
  - id: researcher
    role: imported researcher
    tool_posture: read_heavy
    can_spawn: []
    can_message: [assistant]
`
		if _, err := part.Write([]byte(imported)); err != nil {
			t.Fatalf("write import file: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("close multipart writer: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/configure/team", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Origin", "http://example.com")
		req.Host = "example.com"
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		html := rr.Body.String()
		for _, want := range []string{
			"Imported file loaded. Save Team to apply the change.",
			"Imported Crew",
			`name="agent_3_id"`,
			`value="researcher"`,
			`value="researcher.soul.yaml"`,
			`<option value="reviewer" selected>`,
			`value="assistant" checked`,
		} {
			if !strings.Contains(html, want) {
				t.Fatalf("expected import response to contain %q:\n%s", want, html)
			}
		}

		reloaded, err := teams.LoadConfig(h.teamDir)
		if err != nil {
			t.Fatalf("reload config: %v", err)
		}
		if reloaded.Name != "Repo Task Team" || len(reloaded.Agents) != 3 {
			t.Fatalf("expected import to keep stored team unchanged until save, got %+v", reloaded)
		}
	})
}

func TestWebhooks(t *testing.T) {
	t.Run("whatsapp webhook route is absent when not configured", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/webhooks/whatsapp", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rr.Code)
		}
	})
}

func TestRunSubmit(t *testing.T) {
	t.Run("form renders on GET", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/operate/start-task", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "Start Task") {
			t.Fatalf("expected submit form, got:\n%s", rr.Body.String())
		}
	})

	t.Run("empty task shows inline error", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/operate/start-task", strings.NewReader("task="))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "Task is required.") {
			t.Fatalf("expected error message, got:\n%s", rr.Body.String())
		}
	})

	t.Run("valid task redirects to run detail", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/operate/start-task", strings.NewReader("task=review+the+repo"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
		loc := rr.Header().Get("Location")
		if !strings.HasPrefix(loc, "/operate/runs/") {
			t.Fatalf("expected redirect to /operate/runs/{id}, got %q", loc)
		}
	})

	t.Run("valid task redirects before provider completes", func(t *testing.T) {
		prov := &blockingProvider{
			started: make(chan struct{}),
			release: make(chan struct{}),
		}
		h := newServerHarnessWithProvider(t, prov)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/operate/start-task", strings.NewReader("task=review+the+repo"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

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
			t.Fatal("expected start-task request to redirect before the provider completes")
		}

		if rr.Code != http.StatusSeeOther {
			close(prov.release)
			<-done
			t.Fatalf("expected 303 redirect, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
		loc := rr.Header().Get("Location")
		if !strings.HasPrefix(loc, "/operate/runs/") {
			close(prov.release)
			<-done
			t.Fatalf("expected redirect to /operate/runs/{id}, got %q", loc)
		}

		select {
		case <-prov.started:
		case <-time.After(time.Second):
			close(prov.release)
			t.Fatal("expected provider work to continue in the background")
		}

		runID := strings.TrimPrefix(loc, "/operate/runs/")
		var status string
		if err := h.db.RawDB().QueryRow("SELECT status FROM runs WHERE id = ?", runID).Scan(&status); err != nil {
			close(prov.release)
			t.Fatalf("query run status: %v", err)
		}
		if status != "active" {
			close(prov.release)
			t.Fatalf("expected background run to stay active while provider is blocked, got %q", status)
		}

		close(prov.release)
		waitForRunStatus(t, h.db, runID, "completed")
	})
}

func TestApprovalsResolve(t *testing.T) {
	t.Run("approve redirects to approvals list", func(t *testing.T) {
		h := newServerHarness(t)
		ticketID := h.insertApproval(t, "run-approve", "bash", "echo hi")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/recover/approvals/"+ticketID+"/resolve",
			strings.NewReader("decision=approved"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Location") != "/recover/approvals" {
			t.Fatalf("expected redirect to /recover/approvals, got %q", rr.Header().Get("Location"))
		}
	})

	t.Run("deny redirects to approvals list", func(t *testing.T) {
		h := newServerHarness(t)
		ticketID := h.insertApproval(t, "run-deny", "bash", "rm -rf /")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/recover/approvals/"+ticketID+"/resolve",
			strings.NewReader("decision=denied"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("invalid decision returns 400", func(t *testing.T) {
		h := newServerHarness(t)
		ticketID := h.insertApproval(t, "run-bad", "bash", "echo")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/recover/approvals/"+ticketID+"/resolve",
			strings.NewReader("decision=maybe"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rr.Code)
		}
	})
}

func TestSettingsUpdate(t *testing.T) {
	t.Run("update workspace root redirects to settings", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/configure/settings",
			strings.NewReader("workspace_root=%2Ftmp%2Fworkspace"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Location") != "/configure/settings" {
			t.Fatalf("expected redirect to /configure/settings, got %q", rr.Header().Get("Location"))
		}
	})

	t.Run("settings page masks admin token", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/configure/settings", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		// The raw admin token must not appear verbatim in the page
		if strings.Contains(rr.Body.String(), h.adminToken) {
			t.Fatalf("raw admin token should not appear in settings page")
		}
	})

	t.Run("settings page demotes workspace editing to advanced override copy", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/configure/settings", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "advanced workspace override") {
			t.Fatalf("expected advanced workspace override copy, got:\n%s", rr.Body.String())
		}
	})
}

func TestProjectSwitcher(t *testing.T) {
	t.Run("runs page renders shell project switcher", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/operate/runs", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		for _, want := range []string{`name="project_id"`, "starter-project"} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected runs page to contain %q, got:\n%s", want, body)
			}
		}
	})

	t.Run("runs page uses compact shell controls without visible project label", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/operate/runs", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		for _, want := range []string{
			`--shell-control-height: 36px;`,
			`.shell-project-select.shell-toolbar-control {`,
			`class="shell-project-select shell-toolbar-control"`,
			`onchange="this.form.requestSubmit()"`,
			`aria-label="Active project"`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected runs page to contain %q, got:\n%s", want, body)
			}
		}
		if strings.Contains(body, `class="shell-project-label"`) {
			t.Fatalf("expected no visible shell project label, got:\n%s", body)
		}
		if strings.Contains(body, ">Switch<") {
			t.Fatalf("expected no shell switch button, got:\n%s", body)
		}
		if strings.Contains(body, `class="btn btn-primary shell-start-task`) {
			t.Fatalf("expected no shell start task button, got:\n%s", body)
		}
		if strings.Contains(body, `href="/operate/start-task"`) {
			t.Fatalf("expected no runs-page start task CTA, got:\n%s", body)
		}
	})

	t.Run("switching project updates active workspace", func(t *testing.T) {
		h := newServerHarness(t)
		otherRoot := t.TempDir()
		otherProjectID := h.insertProject(t, "seo-test", otherRoot)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/projects/activate",
			strings.NewReader("project_id="+url.QueryEscape(otherProjectID)+"&redirect_to=%2Foperate%2Fruns"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d body=%s", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Location") != "/operate/runs" {
			t.Fatalf("expected redirect back to runs, got %q", rr.Header().Get("Location"))
		}
		if got := lookupSetting(h.db, "active_project_id"); got != otherProjectID {
			t.Fatalf("expected active_project_id %q, got %q", otherProjectID, got)
		}
		if got := lookupSetting(h.db, "workspace_root"); got != otherRoot {
			t.Fatalf("expected workspace_root %q, got %q", otherRoot, got)
		}
	})

	t.Run("missing project id returns bad request", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/projects/activate", strings.NewReader("redirect_to=%2Foperate%2Fruns"))
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
		if rr.Header().Get("Location") != "/operate/runs" {
			t.Fatalf("expected invalid redirect to fall back to /operate/runs, got %q", rr.Header().Get("Location"))
		}
	})
}

func TestRequestPathWithQuery(t *testing.T) {
	if got := requestPathWithQuery(nil); got != "/operate/runs" {
		t.Fatalf("expected nil request fallback %q, got %q", "/operate/runs", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/operate/runs?scope=all&q=fix", nil)
	if got := requestPathWithQuery(req); got != "/operate/runs?scope=all&q=fix" {
		t.Fatalf("expected request path with query, got %q", got)
	}

	req = &http.Request{URL: &url.URL{Path: "/operate/runs"}}
	if got := requestPathWithQuery(req); got != "/operate/runs" {
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
			req := httptest.NewRequest(http.MethodGet, "/operate/runs?flag="+url.QueryEscape(tt.raw), nil)
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
		{name: "run dismiss", got: runDismissPath("run 1"), want: "/operate/runs/run%201/dismiss"},
		{name: "session message", got: sessionMessagePath("session 1"), want: "/operate/sessions/session%201/messages"},
		{name: "session retry delivery", got: sessionRetryDeliveryPath("session 1", "delivery/1"), want: "/operate/sessions/session%201/deliveries/delivery%2F1/retry"},
		{name: "approval resolve", got: approvalResolvePath("approval/1"), want: "/recover/approvals/approval%2F1/resolve"},
		{name: "route send", got: routeSendPath("route 1"), want: "/recover/routes-deliveries/routes/route%201/messages"},
		{name: "route deactivate", got: routeDeactivatePath("route 1"), want: "/recover/routes-deliveries/routes/route%201/deactivate"},
		{name: "delivery retry", got: deliveryRetryPath("delivery/1"), want: "/recover/routes-deliveries/deliveries/delivery%2F1/retry"},
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
		{name: "matching referer fallback", referer: "http://localhost:8080/operate/runs", host: "localhost:8080", want: true},
		{name: "origin wins over referer", origin: "http://evil.example", referer: "http://localhost:8080/operate/runs", host: "localhost:8080", want: false},
		{name: "invalid referer", referer: "://bad url", host: "localhost:8080", want: false},
		{name: "missing origin and referer", host: "localhost:8080", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "http://"+tc.host+"/operate/runs", nil)
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

func TestProjectLayoutData_FallsBackToLegacyWorkspaceSetting(t *testing.T) {
	h := newServerHarness(t)
	if _, err := h.db.RawDB().Exec("DELETE FROM projects"); err != nil {
		t.Fatalf("delete projects: %v", err)
	}
	if _, err := h.db.RawDB().Exec("DELETE FROM settings WHERE key = 'active_project_id'"); err != nil {
		t.Fatalf("delete active_project_id: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/operate/runs", nil)
	layout, err := h.server.projectLayoutData(req)
	if err != nil {
		t.Fatalf("projectLayoutData: %v", err)
	}
	if layout.ActiveName != filepath.Base(h.workspaceRoot) {
		t.Fatalf("expected legacy active project name %q, got %q", filepath.Base(h.workspaceRoot), layout.ActiveName)
	}
	if len(layout.Options) != 0 {
		t.Fatalf("expected no switcher options for legacy fallback, got %d", len(layout.Options))
	}
}

func TestSettingsBudget(t *testing.T) {
	t.Run("settings page shows budget fields", func(t *testing.T) {
		h := newServerHarness(t)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/configure/settings", nil)
		h.server.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		for _, want := range []string{"per_run_token_budget", "daily_cost_cap_usd"} {
			if !strings.Contains(body, want) {
				t.Fatalf("settings page missing %q field", want)
			}
		}
	})

	t.Run("update budget settings redirects to settings", func(t *testing.T) {
		h := newServerHarness(t)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/configure/settings",
			strings.NewReader("per_run_token_budget=100000&daily_cost_cap_usd=5.0"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.server.ServeHTTP(rr, req)
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d body=%s", rr.Code, rr.Body.String())
		}
	})
}

func TestAdminToken(t *testing.T) {
	t.Run("authorized run submit starts a run and redirects", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/operate/start-task", strings.NewReader("task=review"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
		if !strings.HasPrefix(rr.Header().Get("Location"), "/operate/runs/") {
			t.Fatalf("expected redirect to /operate/runs/{id}, got %q", rr.Header().Get("Location"))
		}
	})

	t.Run("missing authorization is rejected", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/operate/start-task", strings.NewReader("task=review"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

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
		req := httptest.NewRequest(http.MethodPost, "/operate/start-task", strings.NewReader("task=review"))
		req.Header.Set("Authorization", "Bearer wrong-token")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

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
		oldTokenReq := httptest.NewRequest(http.MethodPost, "/operate/start-task", strings.NewReader("task=review"))
		oldTokenReq.Header.Set("Authorization", "Bearer "+h.adminToken)
		oldTokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(oldTokenResp, oldTokenReq)

		if oldTokenResp.Code != http.StatusUnauthorized {
			t.Fatalf("expected stale token to be rejected with 401, got %d", oldTokenResp.Code)
		}

		newTokenResp := httptest.NewRecorder()
		newTokenReq := httptest.NewRequest(http.MethodPost, "/operate/start-task", strings.NewReader("task=review"))
		newTokenReq.Header.Set("Authorization", "Bearer rotated-admin-token")
		newTokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(newTokenResp, newTokenReq)

		// The handler succeeds (redirect to /operate/runs/{id}) — not 401
		if newTokenResp.Code == http.StatusUnauthorized {
			t.Fatalf("expected current token to be accepted, got 401")
		}
	})

	t.Run("html pages mint a host admin session cookie", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/operate/start-task", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}

		cookie := findCookie(rr.Result().Cookies(), hostAdminCookieName)
		if cookie == nil {
			t.Fatalf("expected %s cookie to be set", hostAdminCookieName)
		}
		if !cookie.HttpOnly {
			t.Fatal("expected host admin cookie to be HttpOnly")
		}
		if cookie.SameSite != http.SameSiteStrictMode {
			t.Fatalf("expected SameSite=Strict, got %v", cookie.SameSite)
		}
	})

	t.Run("same-origin host admin cookie can submit run form", func(t *testing.T) {
		h := newServerHarness(t)
		cookie := hostAdminSessionCookie(t, h, "http://localhost/operate/start-task")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/operate/start-task", strings.NewReader("task=review"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
		if !strings.HasPrefix(rr.Header().Get("Location"), "/operate/runs/") {
			t.Fatalf("expected redirect to /operate/runs/{id}, got %q", rr.Header().Get("Location"))
		}
	})

	t.Run("cross-origin host admin cookie is rejected", func(t *testing.T) {
		h := newServerHarness(t)
		cookie := hostAdminSessionCookie(t, h, "http://localhost/operate/start-task")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/operate/start-task", strings.NewReader("task=review"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://evil.test")
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

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

	respOne, readerOne := subscribeSSE(t, ts.URL+"/operate/runs/run-sse/events")
	respTwo, readerTwo := subscribeSSE(t, ts.URL+"/operate/runs/run-sse/events")

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
}

func TestSSEPayloadsAreStructuredJSON(t *testing.T) {
	h := newServerHarness(t)
	h.insertRun(t, "run-sse-json", "conv-sse-json", "watch events", "active")

	ts := httptest.NewServer(h.server)
	defer ts.Close()

	resp, reader := subscribeSSE(t, ts.URL+"/operate/runs/run-sse-json/events")
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
			WorkspaceRoot: h.workspaceRoot,
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
			WorkspaceRoot: h.workspaceRoot,
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
			WorkspaceRoot: h.workspaceRoot,
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
			WorkspaceRoot: h.workspaceRoot,
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
			WorkspaceRoot: h.workspaceRoot,
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
			WorkspaceRoot:  h.workspaceRoot,
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
			WorkspaceRoot: h.workspaceRoot,
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
			WorkspaceRoot: h.workspaceRoot,
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
		req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
			t.Fatalf("expected JSON content type, got %q", got)
		}

		var resp struct {
			Sessions []model.Session `json:"sessions"`
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
		req := httptest.NewRequest(http.MethodGet, "/api/sessions?role=worker&q=researcher", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Sessions []model.Session `json:"sessions"`
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
				WorkspaceRoot:   h.workspaceRoot,
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
		req := httptest.NewRequest(http.MethodGet, "/api/sessions?limit=1", nil)
		h.server.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var first struct {
			Sessions   []model.Session `json:"sessions"`
			NextCursor string          `json:"next_cursor"`
			PrevCursor string          `json:"prev_cursor"`
			HasNext    bool            `json:"has_next"`
			HasPrev    bool            `json:"has_prev"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &first); err != nil {
			t.Fatalf("decode first page: %v", err)
		}
		if len(first.Sessions) != 1 || first.Sessions[0].ID != runs[2].SessionID {
			t.Fatalf("expected newest session first, got %+v", first.Sessions)
		}
		if !first.HasNext || first.NextCursor == "" || first.HasPrev || first.PrevCursor != "" {
			t.Fatalf("unexpected first page metadata: %+v", first)
		}

		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/sessions?limit=1&cursor="+url.QueryEscape(first.NextCursor)+"&direction=next", nil)
		h.server.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var second struct {
			Sessions   []model.Session `json:"sessions"`
			NextCursor string          `json:"next_cursor"`
			PrevCursor string          `json:"prev_cursor"`
			HasNext    bool            `json:"has_next"`
			HasPrev    bool            `json:"has_prev"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &second); err != nil {
			t.Fatalf("decode second page: %v", err)
		}
		if len(second.Sessions) != 1 || second.Sessions[0].ID != runs[1].SessionID {
			t.Fatalf("expected middle session second, got %+v", second.Sessions)
		}
		if !second.HasPrev || second.PrevCursor == "" {
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
		req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+front.SessionID, nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Session  model.Session          `json:"session"`
			Messages []model.SessionMessage `json:"messages"`
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
		if resp.Messages[0].Body != "Inspect the repo." || resp.Messages[1].Body != "mock response" || resp.Messages[2].Body != "Docs inspected." {
			t.Fatalf("unexpected mailbox bodies: %q / %q / %q", resp.Messages[0].Body, resp.Messages[1].Body, resp.Messages[2].Body)
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
			WorkspaceRoot: h.workspaceRoot,
		})
		if err != nil {
			t.Fatalf("StartFrontSession failed: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+run.SessionID, nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Route *model.SessionRoute `json:"route"`
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
			WorkspaceRoot: h.workspaceRoot,
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
		req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+run.SessionID, nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Deliveries       []model.OutboundIntent  `json:"deliveries"`
			DeliveryFailures []model.DeliveryFailure `json:"delivery_failures"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(resp.Deliveries) != 1 {
			t.Fatalf("expected 1 delivery, got %d", len(resp.Deliveries))
		}
		if resp.Deliveries[0].RunID != run.ID || resp.Deliveries[0].Status != "terminal" {
			t.Fatalf("unexpected delivery payload: %+v", resp.Deliveries[0])
		}
		if len(resp.DeliveryFailures) != 1 {
			t.Fatalf("expected 1 delivery failure, got %d", len(resp.DeliveryFailures))
		}
		if resp.DeliveryFailures[0].RunID != run.ID || resp.DeliveryFailures[0].ConnectorID != "telegram" {
			t.Fatalf("unexpected delivery failure identity: %+v", resp.DeliveryFailures[0])
		}
		if resp.DeliveryFailures[0].Error != "delivery failed" {
			t.Fatalf("unexpected delivery failure error: %+v", resp.DeliveryFailures[0])
		}
	})

	t.Run("detail missing session returns not found", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sessions/missing", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rr.Code)
		}
	})

	t.Run("send wakes session and returns follow-up run JSON", func(t *testing.T) {
		h := newServerHarness(t)
		front := h.startFrontSession(t, "Inspect the repo.")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+front.SessionID+"/messages",
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
		detailReq := httptest.NewRequest(http.MethodGet, "/api/sessions/"+front.SessionID, nil)
		h.server.ServeHTTP(detail, detailReq)
		if detail.Code != http.StatusOK {
			t.Fatalf("expected 200 from detail endpoint, got %d body=%s", detail.Code, detail.Body.String())
		}

		var mailbox struct {
			Messages []model.SessionMessage `json:"messages"`
		}
		if err := json.Unmarshal(detail.Body.Bytes(), &mailbox); err != nil {
			t.Fatalf("decode mailbox: %v", err)
		}
		if len(mailbox.Messages) != 4 {
			t.Fatalf("expected 4 mailbox messages after send, got %d", len(mailbox.Messages))
		}
		if mailbox.Messages[2].Body != "What changed?" || mailbox.Messages[3].Body != "mock response" {
			t.Fatalf("unexpected follow-up mailbox bodies: %q / %q", mailbox.Messages[2].Body, mailbox.Messages[3].Body)
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
			WorkspaceRoot: h.workspaceRoot,
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
		req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+run.SessionID+"/deliveries/"+intentID+"/retry", nil)
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
		detailReq := httptest.NewRequest(http.MethodGet, "/api/sessions/"+run.SessionID, nil)
		h.server.ServeHTTP(detail, detailReq)
		if detail.Code != http.StatusOK {
			t.Fatalf("expected 200 from detail endpoint, got %d body=%s", detail.Code, detail.Body.String())
		}

		var detailResp struct {
			DeliveryFailures []model.DeliveryFailure `json:"delivery_failures"`
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
			WorkspaceRoot: h.workspaceRoot,
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
		req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+run.SessionID+"/deliveries/"+intentID+"/retry", nil)
		req.Header.Set("Authorization", "Bearer "+h.adminToken)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d body=%s", rr.Code, rr.Body.String())
		}
	})
}

func TestRoutesDeliveriesPage(t *testing.T) {
	t.Run("GET /recover/routes-deliveries renders health routes and deliveries", func(t *testing.T) {
		h := newServerHarness(t)
		run, route, intentID := h.seedRoutesDeliveriesData(t)
		h.markOutboundIntentTerminal(t, intentID)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/recover/routes-deliveries", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{"Routes &amp; Deliveries", "Connector Health", "Route Directory", "Delivery Queue", route.ID, run.SessionID, "telegram", "terminal"} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected routes and deliveries page to contain %q:\n%s", want, body)
			}
		}
		for _, want := range []string{"directory-card-list", "directory-card"} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected responsive routes and deliveries markup %q:\n%s", want, body)
			}
		}
	})

	t.Run("GET /recover/routes-deliveries applies shared query filters", func(t *testing.T) {
		h := newServerHarness(t)
		_, route, intentID := h.seedRoutesDeliveriesData(t)
		h.markOutboundIntentTerminal(t, intentID)
		if _, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "whatsapp",
				AccountID:   "acct-2",
				ExternalID:  "chat-beta",
				ThreadID:    "thread-2",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "Inspect WhatsApp.",
			WorkspaceRoot: h.workspaceRoot,
		}); err != nil {
			t.Fatalf("StartFrontSession whatsapp failed: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/recover/routes-deliveries?connector_id=telegram&q=chat-1&route_status=all&delivery_status=terminal", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{route.ID, "chat-1", "terminal"} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected filtered routes and deliveries page to contain %q:\n%s", want, body)
			}
		}
		for _, unwanted := range []string{"chat-beta", "whatsapp", "thread-2"} {
			if strings.Contains(body, unwanted) {
				t.Fatalf("expected filtered routes and deliveries page to exclude %q:\n%s", unwanted, body)
			}
		}
	})

	t.Run("GET /recover/routes-deliveries renders section pagination links", func(t *testing.T) {
		h := newServerHarness(t)
		_, _, firstIntentID := h.seedRoutesDeliveriesData(t)
		h.markOutboundIntentTerminal(t, firstIntentID)

		secondRun, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
			ConversationKey: conversations.ConversationKey{
				ConnectorID: "telegram",
				AccountID:   "acct-1",
				ExternalID:  "chat-2",
				ThreadID:    "thread-2",
			},
			FrontAgentID:  "assistant",
			InitialPrompt: "Inspect Telegram 2.",
			WorkspaceRoot: h.workspaceRoot,
		})
		if err != nil {
			t.Fatalf("StartFrontSession second telegram failed: %v", err)
		}

		var secondIntentID string
		if err := h.db.RawDB().QueryRow(
			`SELECT id FROM outbound_intents WHERE run_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`,
			secondRun.ID,
		).Scan(&secondIntentID); err != nil {
			t.Fatalf("load second outbound intent: %v", err)
		}
		h.markOutboundIntentTerminal(t, secondIntentID)

		if _, err := h.db.RawDB().Exec(
			`UPDATE session_bindings
			 SET created_at = CASE external_id
			 	WHEN 'chat-1' THEN '2026-03-25 08:00:00'
			 	WHEN 'chat-2' THEN '2026-03-25 08:01:00'
			 	ELSE created_at
			 END
			 WHERE connector_id = 'telegram'`,
		); err != nil {
			t.Fatalf("update route created_at values: %v", err)
		}
		if _, err := h.db.RawDB().Exec(
			`UPDATE outbound_intents
			 SET created_at = CASE id
			 	WHEN ? THEN '2026-03-25 08:00:00'
			 	WHEN ? THEN '2026-03-25 08:01:00'
			 	ELSE created_at
			 END
			 WHERE id IN (?, ?)`,
			firstIntentID, secondIntentID, firstIntentID, secondIntentID,
		); err != nil {
			t.Fatalf("update outbound intent created_at values: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/recover/routes-deliveries?connector_id=telegram&route_status=active&active_limit=1&delivery_status=terminal&delivery_limit=1", nil)
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{"chat-2", "active_cursor=", "active_direction=next", "delivery_cursor=", "delivery_direction=next"} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected paginated routes and deliveries page to contain %q:\n%s", want, body)
			}
		}
	})

	t.Run("POST /recover/routes-deliveries/routes/{id}/messages wakes the bound session", func(t *testing.T) {
		h := newServerHarness(t)
		run, route, _ := h.seedRoutesDeliveriesData(t)
		cookie := hostAdminSessionCookie(t, h, "http://localhost/recover/routes-deliveries")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodPost,
			"http://localhost/recover/routes-deliveries/routes/"+route.ID+"/messages",
			strings.NewReader("body=What+changed%3F"),
		)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d body=%s", rr.Code, rr.Body.String())
		}
		if !strings.HasPrefix(rr.Header().Get("Location"), "/operate/runs/") {
			t.Fatalf("expected redirect to /operate/runs/{id}, got %q", rr.Header().Get("Location"))
		}

		runs, err := h.db.RawDB().Query(
			`SELECT id, session_id FROM runs WHERE session_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`,
			run.SessionID,
		)
		if err != nil {
			t.Fatalf("query latest run: %v", err)
		}
		defer runs.Close()

		if !runs.Next() {
			t.Fatal("expected follow-up run for bound session")
		}
		var latestRunID, sessionID string
		if err := runs.Scan(&latestRunID, &sessionID); err != nil {
			t.Fatalf("scan latest run: %v", err)
		}
		if latestRunID == run.ID || sessionID != run.SessionID {
			t.Fatalf("expected new run on same session, got run=%s session=%s", latestRunID, sessionID)
		}
	})

	t.Run("POST /recover/routes-deliveries/routes/{id}/messages with empty body re-renders the page with an error", func(t *testing.T) {
		h := newServerHarness(t)
		_, route, _ := h.seedRoutesDeliveriesData(t)
		cookie := hostAdminSessionCookie(t, h, "http://localhost/recover/routes-deliveries")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/recover/routes-deliveries/routes/"+route.ID+"/messages", strings.NewReader("body="))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d body=%s", rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "Route send body is required.") {
			t.Fatalf("expected inline control-page error, got:\n%s", rr.Body.String())
		}
	})

	t.Run("POST /recover/routes-deliveries/routes/{id}/deactivate redirects and clears the active route", func(t *testing.T) {
		h := newServerHarness(t)
		_, route, _ := h.seedRoutesDeliveriesData(t)
		cookie := hostAdminSessionCookie(t, h, "http://localhost/recover/routes-deliveries")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/recover/routes-deliveries/routes/"+route.ID+"/deactivate", nil)
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d body=%s", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Location") != "/recover/routes-deliveries" {
			t.Fatalf("expected redirect to /recover/routes-deliveries, got %q", rr.Header().Get("Location"))
		}

		routes, err := h.rt.ListRoutes(context.Background(), sessions.RouteListFilter{
			ConnectorID: "telegram",
			Status:      "active",
			Limit:       10,
		})
		if err != nil {
			t.Fatalf("list active routes: %v", err)
		}
		if len(routes) != 0 {
			t.Fatalf("expected no active telegram routes after deactivate, got %+v", routes)
		}
	})

	t.Run("POST /recover/routes-deliveries/deliveries/{id}/retry redirects and requeues terminal delivery", func(t *testing.T) {
		h := newServerHarness(t)
		_, _, intentID := h.seedRoutesDeliveriesData(t)
		h.markOutboundIntentTerminal(t, intentID)
		cookie := hostAdminSessionCookie(t, h, "http://localhost/recover/routes-deliveries")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/recover/routes-deliveries/deliveries/"+intentID+"/retry", nil)
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d body=%s", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Location") != "/recover/routes-deliveries" {
			t.Fatalf("expected redirect to /recover/routes-deliveries, got %q", rr.Header().Get("Location"))
		}

		delivery, err := h.rt.RetryDelivery(context.Background(), intentID)
		if !errors.Is(err, runtime.ErrDeliveryNotRetryable) {
			t.Fatalf("expected delivery to already be requeued, got delivery=%+v err=%v", delivery, err)
		}
	})
}

func TestSessionPages(t *testing.T) {
	t.Run("GET /operate/sessions renders the recent session directory", func(t *testing.T) {
		h := newServerHarness(t)
		front := h.startFrontSession(t, "Inspect the repo.")
		worker := h.spawnWorkerSession(t, front.SessionID, "researcher", "Inspect docs.")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/operate/sessions", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{
			"Sessions",
			front.SessionID,
			worker.SessionID,
			"assistant",
			"researcher",
			"Any",
			"Bound",
			"Unbound",
			`class="panel filter-panel"`,
			`class="session-filter-grid"`,
			`class="session-filter-footer"`,
			`filter-action-group`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected session directory to contain %q:\n%s", want, body)
			}
		}
		for _, want := range []string{"directory-card-list", "directory-card"} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected responsive session markup %q:\n%s", want, body)
			}
		}
		if strings.Contains(body, "Bound only") {
			t.Fatalf("expected segmented binding filter instead of legacy checkbox:\n%s", body)
		}
	})

	t.Run("GET /operate/sessions applies shared directory filters", func(t *testing.T) {
		h := newServerHarness(t)
		run, _, _ := h.seedRoutesDeliveriesData(t)
		worker := h.spawnWorkerSession(t, run.SessionID, "researcher", "Inspect docs.")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/operate/sessions?connector_id=telegram&binding=bound", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		if !strings.Contains(body, run.SessionID) {
			t.Fatalf("expected bound front session to appear:\n%s", body)
		}
		if strings.Contains(body, worker.SessionID) {
			t.Fatalf("expected unbound worker session to be filtered out:\n%s", body)
		}
	})

	t.Run("GET /operate/sessions can filter for unbound sessions", func(t *testing.T) {
		h := newServerHarness(t)
		run, _, _ := h.seedRoutesDeliveriesData(t)
		worker := h.spawnWorkerSession(t, run.SessionID, "researcher", "Inspect docs.")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/operate/sessions?binding=unbound", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		if strings.Contains(body, run.SessionID) {
			t.Fatalf("expected bound front session to be filtered out:\n%s", body)
		}
		if !strings.Contains(body, worker.SessionID) {
			t.Fatalf("expected unbound worker session to appear:\n%s", body)
		}
	})

	t.Run("GET /operate/sessions renders pagination links", func(t *testing.T) {
		h := newServerHarness(t)
		svc := sessions.NewService(h.db, conversations.NewConversationStore(h.db))
		base := time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC)

		runs := make([]model.Run, 0, 2)
		for i, key := range []conversations.ConversationKey{
			{ConnectorID: "web", AccountID: "local", ExternalID: "assistant-a", ThreadID: "main-a"},
			{ConnectorID: "web", AccountID: "local", ExternalID: "assistant-b", ThreadID: "main-b"},
		} {
			run, err := h.rt.StartFrontSession(context.Background(), runtime.StartFrontSession{
				ConversationKey: key,
				FrontAgentID:    "assistant",
				InitialPrompt:   "Inspect the repo.",
				WorkspaceRoot:   h.workspaceRoot,
			})
			if err != nil {
				t.Fatalf("StartFrontSession %d failed: %v", i, err)
			}
			if err := svc.AppendMessage(context.Background(), model.SessionMessage{
				ID:        "msg-page-ui-" + run.SessionID,
				SessionID: run.SessionID,
				Kind:      model.MessageAssistant,
				Body:      "ui marker",
				CreatedAt: base.Add(time.Duration(i) * time.Minute),
			}); err != nil {
				t.Fatalf("AppendMessage %d failed: %v", i, err)
			}
			runs = append(runs, run)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/operate/sessions?limit=1", nil)
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		if !strings.Contains(body, runs[1].SessionID) || strings.Contains(body, runs[0].SessionID) {
			t.Fatalf("expected first page to contain only newest session:\n%s", body)
		}
		if !strings.Contains(body, "cursor=") || !strings.Contains(body, "direction=next") || !strings.Contains(body, "Next") {
			t.Fatalf("expected pagination link on sessions page:\n%s", body)
		}
	})

	t.Run("GET /operate/sessions/{id} renders mailbox route and delivery state", func(t *testing.T) {
		h := newServerHarness(t)
		run, route, intentID := h.seedRoutesDeliveriesData(t)
		h.markOutboundIntentTerminal(t, intentID)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/operate/sessions/"+run.SessionID, nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{"Session Detail", run.SessionID, "Inspect Telegram.", "mock response", route.ID, "terminal", "/operate/sessions/" + run.SessionID + "/messages"} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected session detail to contain %q:\n%s", want, body)
			}
		}
	})

	t.Run("GET /operate/sessions/{id} renders live mailbox streaming hooks for an active run", func(t *testing.T) {
		h := newServerHarness(t)
		run := h.startFrontSession(t, "Inspect the repo.")
		svc := sessions.NewService(h.db, conversations.NewConversationStore(h.db))
		if err := svc.AppendMessage(context.Background(), model.SessionMessage{
			ID:        "msg-stream-assistant",
			SessionID: run.SessionID,
			Kind:      model.MessageAssistant,
			Body:      "Previous reply.",
			Provenance: model.SessionMessageProvenance{
				Kind:        model.MessageProvenanceAssistantTurn,
				SourceRunID: run.ID,
			},
		}); err != nil {
			t.Fatalf("AppendMessage failed: %v", err)
		}
		h.insertRunWithSession(t, "run-session-active", run.ConversationID, run.SessionID, "follow up", "active")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/operate/sessions/"+run.SessionID, nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{
			`id="session-live-root"`,
			`data-active-run-id="run-session-active"`,
			`/api/sessions/` + run.SessionID + `/messages`,
			"`/operate/runs/${encodeURIComponent(runID)}/events`",
			`new EventSource(streamURL)`,
			`data-source-run-id="` + run.ID + `"`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected session detail to contain %q:\n%s", want, body)
			}
		}
	})

	t.Run("POST /operate/sessions/{id}/messages wakes the session and redirects to the new run", func(t *testing.T) {
		h := newServerHarness(t)
		front := h.startFrontSession(t, "Inspect the repo.")
		cookie := hostAdminSessionCookie(t, h, "http://localhost/operate/sessions/"+front.SessionID)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodPost,
			"http://localhost/operate/sessions/"+front.SessionID+"/messages",
			strings.NewReader("body=What+changed%3F"),
		)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d body=%s", rr.Code, rr.Body.String())
		}
		if !strings.HasPrefix(rr.Header().Get("Location"), "/operate/runs/") {
			t.Fatalf("expected redirect to /operate/runs/{id}, got %q", rr.Header().Get("Location"))
		}
	})

	t.Run("POST /operate/sessions/{id}/messages with empty body re-renders the detail page with an error", func(t *testing.T) {
		h := newServerHarness(t)
		front := h.startFrontSession(t, "Inspect the repo.")
		cookie := hostAdminSessionCookie(t, h, "http://localhost/operate/sessions/"+front.SessionID)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodPost,
			"http://localhost/operate/sessions/"+front.SessionID+"/messages",
			strings.NewReader("body="),
		)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d body=%s", rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "Session message body is required.") {
			t.Fatalf("expected inline session-detail error, got:\n%s", rr.Body.String())
		}
	})

	t.Run("POST /operate/sessions/{id}/deliveries/{delivery_id}/retry redirects back to session detail", func(t *testing.T) {
		h := newServerHarness(t)
		run, _, intentID := h.seedRoutesDeliveriesData(t)
		h.markOutboundIntentTerminal(t, intentID)
		cookie := hostAdminSessionCookie(t, h, "http://localhost/operate/sessions/"+run.SessionID)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodPost,
			"http://localhost/operate/sessions/"+run.SessionID+"/deliveries/"+intentID+"/retry",
			nil,
		)
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d body=%s", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Location") != "/operate/sessions/"+run.SessionID {
			t.Fatalf("expected redirect to session detail, got %q", rr.Header().Get("Location"))
		}

		delivery, err := h.rt.RetrySessionDelivery(context.Background(), run.SessionID, intentID)
		if !errors.Is(err, runtime.ErrDeliveryNotRetryable) {
			t.Fatalf("expected delivery to already be requeued, got delivery=%+v err=%v", delivery, err)
		}
	})
}

type serverHarness struct {
	db            *store.DB
	server        *Server
	broadcaster   *SSEBroadcaster
	rt            *runtime.Runtime
	adminToken    string
	teamDir       string
	workspaceRoot string
}

func newServerHarness(t *testing.T) *serverHarness {
	t.Helper()
	return newServerHarnessWithProvider(t, runtime.NewMockProvider(nil, nil))
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

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	adminToken := "test-admin-token"
	workspaceRoot := t.TempDir()
	teamDir := writeTeamFixture(t)
	const activeProjectID = "proj-primary"
	if _, err := db.RawDB().Exec(
		`INSERT INTO projects (id, name, workspace_root, source, created_at, last_used_at)
		 VALUES (?, ?, ?, 'starter', datetime('now'), datetime('now'))`,
		activeProjectID, "starter-project", workspaceRoot,
	); err != nil {
		t.Fatalf("seed primary project: %v", err)
	}
	seedSettings(t, db, map[string]string{
		"admin_token":             adminToken,
		"active_project_id":       activeProjectID,
		"workspace_root":          workspaceRoot,
		"team_name":               "Repo Task Team",
		"onboarding_completed_at": "2026-03-25 00:00:00",
	})

	cs := conversations.NewConversationStore(db)
	mem := memory.NewStore(db, cs)
	reg := tools.NewRegistry()
	broadcaster := NewSSEBroadcaster()
	rt := runtime.New(db, cs, reg, mem, prov, broadcaster)
	t.Cleanup(func() {
		rt.WaitAsync()
		_ = db.Close()
	})
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
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	return &serverHarness{
		db:            db,
		server:        server,
		broadcaster:   broadcaster,
		rt:            rt,
		adminToken:    adminToken,
		teamDir:       teamDir,
		workspaceRoot: workspaceRoot,
	}
}

func (h *serverHarness) insertProject(t *testing.T, name, workspaceRoot string) string {
	t.Helper()

	projectID := "proj-" + strconv.Itoa(time.Now().Nanosecond())
	if _, err := h.db.RawDB().Exec(
		`INSERT INTO projects (id, name, workspace_root, source, created_at, last_used_at)
		 VALUES (?, ?, ?, 'operator', datetime('now'), datetime('now'))`,
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
    soul_file: coordinator.soul.yaml
    can_spawn: [patcher, reviewer]
    can_message: [patcher, reviewer]
  - id: patcher
    soul_file: patcher.soul.yaml
    can_spawn: []
    can_message: [assistant, reviewer]
  - id: reviewer
    soul_file: reviewer.soul.yaml
    can_spawn: []
    can_message: [assistant, patcher]
`)
	writeTestFile(t, filepath.Join(dir, "coordinator.soul.yaml"), "role: operator-facing coordinator\ntool_posture: operator_facing\n")
	writeTestFile(t, filepath.Join(dir, "patcher.soul.yaml"), "role: workspace write specialist\ntool_posture: workspace_write\n")
	writeTestFile(t, filepath.Join(dir, "reviewer.soul.yaml"), "role: diff reviewer\ntool_posture: read_heavy\n")
	return dir
}

func teamFormValues(cfg teams.Config) url.Values {
	form := url.Values{
		"name":        {cfg.Name},
		"front_agent": {cfg.FrontAgent},
		"agent_count": {strconv.Itoa(len(cfg.Agents))},
	}
	for idx, agent := range cfg.Agents {
		prefix := "agent_" + strconv.Itoa(idx) + "_"
		form.Set(prefix+"id", agent.ID)
		form.Set(prefix+"soul_file", agent.SoulFile)
		form.Set(prefix+"role", agent.Role)
		form.Set(prefix+"tool_posture", agent.ToolPosture)
		rawExtra, err := json.Marshal(agent.Soul.Extra)
		if err == nil {
			form.Set(prefix+"soul_extra_json", string(rawExtra))
		} else {
			form.Set(prefix+"soul_extra_json", "{}")
		}
		for _, value := range agent.CanSpawn {
			form.Add(prefix+"can_spawn", value)
		}
		for _, value := range agent.CanMessage {
			form.Add(prefix+"can_message", value)
		}
	}
	return form
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

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, pageURL, nil)

	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected GET %s to succeed, got %d", pageURL, rr.Code)
	}

	cookie := findCookie(rr.Result().Cookies(), hostAdminCookieName)
	if cookie == nil {
		t.Fatalf("expected %s cookie after GET %s", hostAdminCookieName, pageURL)
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
		 (id, conversation_id, agent_id, team_id, objective, workspace_root, status, created_at, updated_at)
		 VALUES (?, ?, 'agent-1', 'repo-task-team', ?, ?, ?, datetime('now'), datetime('now'))`,
		runID, conversationID, objective, h.workspaceRoot, status,
	)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}
}

func (h *serverHarness) insertRunAt(t *testing.T, runID, conversationID, objective, status, createdAt string) {
	t.Helper()

	_, err := h.db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, team_id, objective, workspace_root, status, created_at, updated_at)
		 VALUES (?, ?, 'agent-1', 'repo-task-team', ?, ?, ?, ?, ?)`,
		runID, conversationID, objective, h.workspaceRoot, status, createdAt, createdAt,
	)
	if err != nil {
		t.Fatalf("insert run at %s: %v", createdAt, err)
	}
}

func (h *serverHarness) insertRunInWorkspace(t *testing.T, runID, conversationID, objective, status, workspaceRoot string) {
	t.Helper()

	_, err := h.db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, team_id, objective, workspace_root, status, created_at, updated_at)
		 VALUES (?, ?, 'agent-1', 'repo-task-team', ?, ?, ?, datetime('now'), datetime('now'))`,
		runID, conversationID, objective, workspaceRoot, status,
	)
	if err != nil {
		t.Fatalf("insert run in workspace: %v", err)
	}
}

func (h *serverHarness) insertRunWithSnapshot(t *testing.T, runID, conversationID, objective, status string, snapshot model.ExecutionSnapshot) {
	t.Helper()
	h.insertRunWithSnapshotAt(t, runID, conversationID, objective, status, "2026-03-25 00:00:00", "2026-03-25 00:00:00", snapshot)
}

func (h *serverHarness) insertRunWithSnapshotAt(t *testing.T, runID, conversationID, objective, status, createdAt, updatedAt string, snapshot model.ExecutionSnapshot) {
	t.Helper()

	rawSnapshot, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal execution snapshot: %v", err)
	}

	_, err = h.db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, team_id, objective, workspace_root, status, execution_snapshot_json, created_at, updated_at)
		 VALUES (?, ?, 'agent-1', ?, ?, ?, ?, ?, ?, ?)`,
		runID, conversationID, snapshot.TeamID, objective, h.workspaceRoot, status, rawSnapshot, createdAt, updatedAt,
	)
	if err != nil {
		t.Fatalf("insert run with snapshot: %v", err)
	}
}

func (h *serverHarness) insertRunWithSession(t *testing.T, runID, conversationID, sessionID, objective, status string) {
	t.Helper()

	_, err := h.db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, session_id, team_id, objective, workspace_root, status, created_at, updated_at)
		 VALUES (?, ?, 'agent-1', ?, 'repo-task-team', ?, ?, ?, datetime('now'), datetime('now'))`,
		runID, conversationID, sessionID, objective, h.workspaceRoot, status,
	)
	if err != nil {
		t.Fatalf("insert session run: %v", err)
	}
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
		WorkspaceRoot: h.workspaceRoot,
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
	_, err := h.db.RawDB().Exec(
		`INSERT INTO approvals (id, run_id, tool_name, args_json, target_path, fingerprint, status, created_at)
		 VALUES (?, ?, ?, x'', ?, 'fp-test', 'pending', datetime('now'))`,
		id, runID, toolName, targetPath,
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

	_, err := h.db.RawDB().Exec(
		`INSERT INTO approvals (id, run_id, tool_name, args_json, target_path, fingerprint, status, resolved_by, created_at, resolved_at)
		 VALUES (?, ?, ?, x'', ?, 'fp-test', ?, NULLIF(?, ''), ?, ?)`,
		id, runID, toolName, targetPath, status, resolvedBy, createdAt, resolvedAt,
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

func (h *serverHarness) insertEvent(t *testing.T, eventID, conversationID, runID, kind string) {
	t.Helper()
	h.insertEventAtWithPayload(t, eventID, conversationID, runID, kind, nil, "2026-03-25 00:00:00")
}

func (h *serverHarness) insertEventAt(t *testing.T, eventID, conversationID, runID, kind, createdAt string) {
	t.Helper()
	h.insertEventAtWithPayload(t, eventID, conversationID, runID, kind, nil, createdAt)
}

func (h *serverHarness) insertEventWithPayload(t *testing.T, eventID, conversationID, runID, kind string, payload []byte) {
	t.Helper()
	h.insertEventAtWithPayload(t, eventID, conversationID, runID, kind, payload, "2026-03-25 00:00:00")
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
		WorkspaceRoot: h.workspaceRoot,
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
