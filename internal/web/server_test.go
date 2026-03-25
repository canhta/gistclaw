package web

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		h.insertEventWithPayload(
			t,
			"evt-2",
			"conv-1",
			"run-detail",
			"turn_completed",
			[]byte(`{"content":"Draft the rollout plan.","input_tokens":12,"output_tokens":8}`),
		)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/runs/run-detail", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}

		body := rr.Body.String()
		for _, want := range []string{
			"run-detail",
			"run_started",
			"Draft the rollout plan.",
			`id="run-live-output"`,
			`/runs/run-detail/events`,
			`new EventSource(streamURL)`,
		} {
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

	t.Run("html pages mint a host admin session cookie", func(t *testing.T) {
		h := newServerHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/run", nil)

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
		cookie := hostAdminSessionCookie(t, h, "http://localhost/run")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/run", strings.NewReader("task=review"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d\nbody: %s", rr.Code, rr.Body.String())
		}
		if !strings.HasPrefix(rr.Header().Get("Location"), "/runs/") {
			t.Fatalf("expected redirect to /runs/{id}, got %q", rr.Header().Get("Location"))
		}
	})

	t.Run("cross-origin host admin cookie is rejected", func(t *testing.T) {
		h := newServerHarness(t)
		cookie := hostAdminSessionCookie(t, h, "http://localhost/run")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/run", strings.NewReader("task=review"))
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

func TestSSEPayloadsAreStructuredJSON(t *testing.T) {
	h := newServerHarness(t)
	h.insertRun(t, "run-sse-json", "conv-sse-json", "watch events", "active")

	ts := httptest.NewServer(h.server)
	defer ts.Close()

	resp, reader := subscribeSSE(t, ts.URL+"/runs/run-sse-json/events")
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

func TestControlPlanePage(t *testing.T) {
	t.Run("GET /control renders health routes and deliveries", func(t *testing.T) {
		h := newServerHarness(t)
		run, route, intentID := h.seedControlPlaneRoute(t)
		h.markOutboundIntentTerminal(t, intentID)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/control", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{"Control", "Connector Health", "Route Directory", "Delivery Queue", route.ID, run.SessionID, "telegram", "terminal"} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected control page to contain %q:\n%s", want, body)
			}
		}
	})

	t.Run("GET /control applies shared query filters", func(t *testing.T) {
		h := newServerHarness(t)
		_, route, intentID := h.seedControlPlaneRoute(t)
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
		req := httptest.NewRequest(http.MethodGet, "http://localhost/control?connector_id=telegram&q=chat-1&route_status=all&delivery_status=terminal", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{route.ID, "chat-1", "terminal"} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected filtered control page to contain %q:\n%s", want, body)
			}
		}
		for _, unwanted := range []string{"chat-beta", "whatsapp", "thread-2"} {
			if strings.Contains(body, unwanted) {
				t.Fatalf("expected filtered control page to exclude %q:\n%s", unwanted, body)
			}
		}
	})

	t.Run("GET /control renders section pagination links", func(t *testing.T) {
		h := newServerHarness(t)
		_, _, firstIntentID := h.seedControlPlaneRoute(t)
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
		req := httptest.NewRequest(http.MethodGet, "http://localhost/control?connector_id=telegram&route_status=active&active_limit=1&delivery_status=terminal&delivery_limit=1", nil)
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{"chat-2", "active_cursor=", "active_direction=next", "delivery_cursor=", "delivery_direction=next"} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected paginated control page to contain %q:\n%s", want, body)
			}
		}
	})

	t.Run("POST /control/routes/{id}/messages wakes the bound session", func(t *testing.T) {
		h := newServerHarness(t)
		run, route, _ := h.seedControlPlaneRoute(t)
		cookie := hostAdminSessionCookie(t, h, "http://localhost/control")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodPost,
			"http://localhost/control/routes/"+route.ID+"/messages",
			strings.NewReader("body=What+changed%3F"),
		)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d body=%s", rr.Code, rr.Body.String())
		}
		if !strings.HasPrefix(rr.Header().Get("Location"), "/runs/") {
			t.Fatalf("expected redirect to /runs/{id}, got %q", rr.Header().Get("Location"))
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

	t.Run("POST /control/routes/{id}/messages with empty body re-renders the page with an error", func(t *testing.T) {
		h := newServerHarness(t)
		_, route, _ := h.seedControlPlaneRoute(t)
		cookie := hostAdminSessionCookie(t, h, "http://localhost/control")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/control/routes/"+route.ID+"/messages", strings.NewReader("body="))
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

	t.Run("POST /control/routes/{id}/deactivate redirects and clears the active route", func(t *testing.T) {
		h := newServerHarness(t)
		_, route, _ := h.seedControlPlaneRoute(t)
		cookie := hostAdminSessionCookie(t, h, "http://localhost/control")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/control/routes/"+route.ID+"/deactivate", nil)
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d body=%s", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Location") != "/control" {
			t.Fatalf("expected redirect to /control, got %q", rr.Header().Get("Location"))
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

	t.Run("POST /control/deliveries/{id}/retry redirects and requeues terminal delivery", func(t *testing.T) {
		h := newServerHarness(t)
		_, _, intentID := h.seedControlPlaneRoute(t)
		h.markOutboundIntentTerminal(t, intentID)
		cookie := hostAdminSessionCookie(t, h, "http://localhost/control")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/control/deliveries/"+intentID+"/retry", nil)
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d body=%s", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Location") != "/control" {
			t.Fatalf("expected redirect to /control, got %q", rr.Header().Get("Location"))
		}

		delivery, err := h.rt.RetryDelivery(context.Background(), intentID)
		if !errors.Is(err, runtime.ErrDeliveryNotRetryable) {
			t.Fatalf("expected delivery to already be requeued, got delivery=%+v err=%v", delivery, err)
		}
	})
}

func TestSessionPages(t *testing.T) {
	t.Run("GET /sessions renders the recent session directory", func(t *testing.T) {
		h := newServerHarness(t)
		front := h.startFrontSession(t, "Inspect the repo.")
		worker := h.spawnWorkerSession(t, front.SessionID, "researcher", "Inspect docs.")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/sessions", nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{"Sessions", front.SessionID, worker.SessionID, "assistant", "researcher"} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected session directory to contain %q:\n%s", want, body)
			}
		}
	})

	t.Run("GET /sessions applies shared directory filters", func(t *testing.T) {
		h := newServerHarness(t)
		run, _, _ := h.seedControlPlaneRoute(t)
		worker := h.spawnWorkerSession(t, run.SessionID, "researcher", "Inspect docs.")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/sessions?connector_id=telegram&bound_only=1", nil)

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

	t.Run("GET /sessions renders pagination links", func(t *testing.T) {
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
		req := httptest.NewRequest(http.MethodGet, "http://localhost/sessions?limit=1", nil)
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

	t.Run("GET /sessions/{id} renders mailbox route and delivery state", func(t *testing.T) {
		h := newServerHarness(t)
		run, route, intentID := h.seedControlPlaneRoute(t)
		h.markOutboundIntentTerminal(t, intentID)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/sessions/"+run.SessionID, nil)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{"Session Detail", run.SessionID, "Inspect Telegram.", "mock response", route.ID, "terminal", "/sessions/" + run.SessionID + "/messages"} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected session detail to contain %q:\n%s", want, body)
			}
		}
	})

	t.Run("POST /sessions/{id}/messages wakes the session and redirects to the new run", func(t *testing.T) {
		h := newServerHarness(t)
		front := h.startFrontSession(t, "Inspect the repo.")
		cookie := hostAdminSessionCookie(t, h, "http://localhost/sessions/"+front.SessionID)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodPost,
			"http://localhost/sessions/"+front.SessionID+"/messages",
			strings.NewReader("body=What+changed%3F"),
		)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d body=%s", rr.Code, rr.Body.String())
		}
		if !strings.HasPrefix(rr.Header().Get("Location"), "/runs/") {
			t.Fatalf("expected redirect to /runs/{id}, got %q", rr.Header().Get("Location"))
		}
	})

	t.Run("POST /sessions/{id}/messages with empty body re-renders the detail page with an error", func(t *testing.T) {
		h := newServerHarness(t)
		front := h.startFrontSession(t, "Inspect the repo.")
		cookie := hostAdminSessionCookie(t, h, "http://localhost/sessions/"+front.SessionID)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodPost,
			"http://localhost/sessions/"+front.SessionID+"/messages",
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

	t.Run("POST /sessions/{id}/deliveries/{delivery_id}/retry redirects back to session detail", func(t *testing.T) {
		h := newServerHarness(t)
		run, _, intentID := h.seedControlPlaneRoute(t)
		h.markOutboundIntentTerminal(t, intentID)
		cookie := hostAdminSessionCookie(t, h, "http://localhost/sessions/"+run.SessionID)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodPost,
			"http://localhost/sessions/"+run.SessionID+"/deliveries/"+intentID+"/retry",
			nil,
		)
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d body=%s", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Location") != "/sessions/"+run.SessionID {
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

func (h *serverHarness) seedControlPlaneRoute(t *testing.T) (model.Run, model.RouteDirectoryItem, string) {
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

func (h *serverHarness) insertEventWithPayload(t *testing.T, eventID, conversationID, runID, kind string, payload []byte) {
	t.Helper()

	_, err := h.db.RawDB().Exec(
		`INSERT INTO events (id, conversation_id, run_id, kind, payload_json, created_at)
		 VALUES (?, ?, ?, ?, ?, datetime('now'))`,
		eventID, conversationID, runID, kind, payload,
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
