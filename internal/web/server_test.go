package web

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/replay"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

func TestRuns(t *testing.T) {
	t.Run("list renders runs", func(t *testing.T) {
		h := newServerHarness(t)
		h.insertRun(t, "run-known", "conv-1", "review the repo", "active")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/runs", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}

		body := rr.Body.String()
		for _, want := range []string{"Runs", "run-known", "review the repo"} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected body to contain %q:\n%s", want, body)
			}
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
		req := httptest.NewRequest(http.MethodGet, "/runs", nil)

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
		req := httptest.NewRequest(http.MethodGet, "/runs", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "You haven't run any tasks yet.") {
			t.Fatalf("expected empty state, got:\n%s", rr.Body.String())
		}
	})

	t.Run("detail renders known run", func(t *testing.T) {
		h := newServerHarness(t)
		h.insertRun(t, "run-detail", "conv-1", "review the repo", "completed")
		h.insertEvent(t, "evt-1", "conv-1", "run-detail", "run_started")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/runs/run-detail", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}

		body := rr.Body.String()
		for _, want := range []string{"run-detail", "run_started"} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected body to contain %q:\n%s", want, body)
			}
		}
	})

	t.Run("detail missing run returns not found", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/runs/missing", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rr.Code)
		}
	})
}

func TestApprovals(t *testing.T) {
	h := newServerHarness(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/approvals", nil)

	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Approvals") {
		t.Fatalf("expected approvals page, got:\n%s", rr.Body.String())
	}
}

func TestSettings(t *testing.T) {
	h := newServerHarness(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)

	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Settings") {
		t.Fatalf("expected settings page, got:\n%s", rr.Body.String())
	}
}

func TestRunSubmit(t *testing.T) {
	t.Run("form renders on GET", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/run", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "Submit a Task") {
			t.Fatalf("expected submit form, got:\n%s", rr.Body.String())
		}
	})

	t.Run("empty task shows inline error", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/run", strings.NewReader("task="))
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
		req := httptest.NewRequest(http.MethodPost, "/run", strings.NewReader("task=review+the+repo"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
		loc := rr.Header().Get("Location")
		if !strings.HasPrefix(loc, "/runs/") {
			t.Fatalf("expected redirect to /runs/{id}, got %q", loc)
		}
	})
}

func TestApprovalsResolve(t *testing.T) {
	t.Run("approve redirects to approvals list", func(t *testing.T) {
		h := newServerHarness(t)
		ticketID := h.insertApproval(t, "run-approve", "bash", "echo hi")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/approvals/"+ticketID+"/resolve",
			strings.NewReader("decision=approved"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Location") != "/approvals" {
			t.Fatalf("expected redirect to /approvals, got %q", rr.Header().Get("Location"))
		}
	})

	t.Run("deny redirects to approvals list", func(t *testing.T) {
		h := newServerHarness(t)
		ticketID := h.insertApproval(t, "run-deny", "bash", "rm -rf /")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/approvals/"+ticketID+"/resolve",
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
		req := httptest.NewRequest(http.MethodPost, "/approvals/"+ticketID+"/resolve",
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
	t.Run("update team_name redirects to settings", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/settings",
			strings.NewReader("team_name=My+Team"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Location") != "/settings" {
			t.Fatalf("expected redirect to /settings, got %q", rr.Header().Get("Location"))
		}
	})

	t.Run("settings page masks admin token", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/settings", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		// The raw admin token must not appear verbatim in the page
		if strings.Contains(rr.Body.String(), h.adminToken) {
			t.Fatalf("raw admin token should not appear in settings page")
		}
	})
}

func TestSettingsBudget(t *testing.T) {
	t.Run("settings page shows budget fields", func(t *testing.T) {
		h := newServerHarness(t)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/settings", nil)
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
		req := httptest.NewRequest(http.MethodPost, "/settings",
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
		req := httptest.NewRequest(http.MethodPost, "/run", strings.NewReader("task=review"))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
		if !strings.HasPrefix(rr.Header().Get("Location"), "/runs/") {
			t.Fatalf("expected redirect to /runs/{id}, got %q", rr.Header().Get("Location"))
		}
	})

	t.Run("missing authorization is rejected", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/run", strings.NewReader("task=review"))
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
		req := httptest.NewRequest(http.MethodPost, "/run", strings.NewReader("task=review"))
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
		oldTokenReq := httptest.NewRequest(http.MethodPost, "/run", strings.NewReader("task=review"))
		oldTokenReq.Header.Set("Authorization", "Bearer "+h.adminToken)
		oldTokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(oldTokenResp, oldTokenReq)

		if oldTokenResp.Code != http.StatusUnauthorized {
			t.Fatalf("expected stale token to be rejected with 401, got %d", oldTokenResp.Code)
		}

		newTokenResp := httptest.NewRecorder()
		newTokenReq := httptest.NewRequest(http.MethodPost, "/run", strings.NewReader("task=review"))
		newTokenReq.Header.Set("Authorization", "Bearer rotated-admin-token")
		newTokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(newTokenResp, newTokenReq)

		// The handler succeeds (redirect to /runs/{id}) — not 401
		if newTokenResp.Code == http.StatusUnauthorized {
			t.Fatalf("expected current token to be accepted, got 401")
		}
	})
}

func TestSSE(t *testing.T) {
	h := newServerHarness(t)
	h.insertRun(t, "run-sse", "conv-sse", "watch events", "active")

	ts := httptest.NewServer(h.server)
	defer ts.Close()

	respOne, readerOne := subscribeSSE(t, ts.URL+"/runs/run-sse/events")
	respTwo, readerTwo := subscribeSSE(t, ts.URL+"/runs/run-sse/events")

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

func TestSessionAPI(t *testing.T) {
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
}

type serverHarness struct {
	db            *store.DB
	server        *Server
	broadcaster   *SSEBroadcaster
	rt            *runtime.Runtime
	adminToken    string
	workspaceRoot string
}

func newServerHarness(t *testing.T) *serverHarness {
	t.Helper()

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	adminToken := "test-admin-token"
	workspaceRoot := t.TempDir()
	seedSettings(t, db, map[string]string{
		"admin_token":    adminToken,
		"workspace_root": workspaceRoot,
		"team_name":      "Repo Task Team",
	})

	cs := conversations.NewConversationStore(db)
	mem := memory.NewStore(db, cs)
	reg := tools.NewRegistry()
	prov := runtime.NewMockProvider(nil, nil)
	broadcaster := NewSSEBroadcaster()
	rt := runtime.New(db, cs, reg, mem, prov, broadcaster)

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
		workspaceRoot: workspaceRoot,
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

func (h *serverHarness) insertApproval(t *testing.T, runID, toolName, targetPath string) string {
	t.Helper()

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

func (h *serverHarness) insertEvent(t *testing.T, eventID, conversationID, runID, kind string) {
	t.Helper()

	_, err := h.db.RawDB().Exec(
		`INSERT INTO events (id, conversation_id, run_id, kind, payload_json, created_at)
		 VALUES (?, ?, ?, ?, x'', datetime('now'))`,
		eventID, conversationID, runID, kind,
	)
	if err != nil {
		t.Fatalf("insert event: %v", err)
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
