package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
)

func memoryFilterArg(agentID, scope string) memory.MemoryFilter {
	return memory.MemoryFilter{AgentID: agentID, Scope: scope}
}

func seedMemoryFact(t *testing.T, h *serverHarness, agentID, scope, content, source string) string {
	t.Helper()
	item := model.MemoryItem{
		AgentID: agentID,
		Scope:   scope,
		Content: content,
		Source:  source,
	}
	if err := h.rt.Memory().WriteFact(context.Background(), item); err != nil {
		t.Fatalf("seedMemoryFact: %v", err)
	}
	// Retrieve ID by searching.
	facts, err := h.rt.Memory().Filter(context.Background(), memoryFilterArg(agentID, ""))
	if err != nil {
		t.Fatalf("seedMemoryFact filter: %v", err)
	}
	for _, f := range facts {
		if f.Content == content {
			return f.ID
		}
	}
	t.Fatalf("could not find seeded fact with content %q", content)
	return ""
}

func TestMemoryInspector(t *testing.T) {
	t.Run("GET /memory returns all non-forgotten facts with scope and provenance", func(t *testing.T) {
		h := newServerHarness(t)
		seedMemoryFact(t, h, "coordinator", "local", "fact one", "model")
		seedMemoryFact(t, h, "patcher", "team", "fact two", "model")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/memory", nil)
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		if !strings.Contains(body, "fact one") {
			t.Error("expected fact one in response")
		}
		if !strings.Contains(body, "fact two") {
			t.Error("expected fact two in response")
		}
		if !strings.Contains(body, "local") {
			t.Error("expected scope to be visible")
		}
	})

	t.Run("GET /memory?scope=local returns only local-scoped facts", func(t *testing.T) {
		h := newServerHarness(t)
		seedMemoryFact(t, h, "coordinator", "local", "local only", "model")
		seedMemoryFact(t, h, "coordinator", "team", "team only", "model")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/memory?scope=local", nil)
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "local only") {
			t.Error("expected local fact in filtered response")
		}
		if strings.Contains(body, "team only") {
			t.Error("team fact must not appear in scope=local filter")
		}
	})

	t.Run("GET /memory?agent_id=X returns only facts from agent X", func(t *testing.T) {
		h := newServerHarness(t)
		seedMemoryFact(t, h, "agent-x", "local", "x fact", "model")
		seedMemoryFact(t, h, "agent-y", "local", "y fact", "model")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/memory?agent_id=agent-x", nil)
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "x fact") {
			t.Error("expected agent-x fact in filtered response")
		}
		if strings.Contains(body, "y fact") {
			t.Error("agent-y fact must not appear in agent_id=agent-x filter")
		}
	})

	t.Run("POST /memory/{id}/forget with confirm=yes forgets and redirects", func(t *testing.T) {
		h := newServerHarness(t)
		id := seedMemoryFact(t, h, "coordinator", "local", "to be forgotten", "model")
		cookie := hostAdminSessionCookie(t, h, "http://localhost/memory")

		form := url.Values{"confirm": {"yes"}}
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/memory/"+id+"/forget",
			strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d: %s", rr.Code, rr.Body.String())
		}

		// Fact must now be absent from Filter results.
		facts, err := h.rt.Memory().Filter(context.Background(), memoryFilterArg("coordinator", ""))
		if err != nil {
			t.Fatalf("Filter after forget: %v", err)
		}
		for _, f := range facts {
			if f.ID == id {
				t.Error("forgotten fact still appears in Filter results")
			}
		}
	})

	t.Run("POST /memory/{id}/forget without confirm shows confirmation prompt", func(t *testing.T) {
		h := newServerHarness(t)
		id := seedMemoryFact(t, h, "coordinator", "local", "pending forget", "model")
		cookie := hostAdminSessionCookie(t, h, "http://localhost/memory")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/memory/"+id+"/forget",
			strings.NewReader(""))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)
		h.server.ServeHTTP(rr, req)

		// Must NOT redirect; must show confirmation.
		if rr.Code == http.StatusSeeOther {
			t.Fatal("must not redirect without confirmation")
		}
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 confirmation page, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "pending forget") {
			t.Error("confirmation page should show the fact value")
		}

		// Fact must still exist.
		facts, err := h.rt.Memory().Filter(context.Background(), memoryFilterArg("coordinator", ""))
		if err != nil {
			t.Fatalf("Filter: %v", err)
		}
		var found bool
		for _, f := range facts {
			if f.ID == id {
				found = true
			}
		}
		if !found {
			t.Error("fact was forgotten without confirmation")
		}
	})

	t.Run("POST /memory/{id}/edit updates value and redirects", func(t *testing.T) {
		h := newServerHarness(t)
		id := seedMemoryFact(t, h, "coordinator", "local", "old value", "model")
		cookie := hostAdminSessionCookie(t, h, "http://localhost/memory")

		form := url.Values{"value": {"new human value"}}
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/memory/"+id+"/edit",
			strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://localhost")
		req.AddCookie(cookie)
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d: %s", rr.Code, rr.Body.String())
		}

		facts, err := h.rt.Memory().Filter(context.Background(), memoryFilterArg("coordinator", ""))
		if err != nil {
			t.Fatalf("Filter after edit: %v", err)
		}
		for _, f := range facts {
			if f.ID == id {
				if f.Content != "new human value" {
					t.Errorf("expected new human value, got %q", f.Content)
				}
				return
			}
		}
		t.Fatal("edited fact not found")
	})

	t.Run("POST /memory/{id}/forget rejects anonymous writes", func(t *testing.T) {
		h := newServerHarness(t)
		id := seedMemoryFact(t, h, "coordinator", "local", "anonymous forget", "model")

		form := url.Values{"confirm": {"yes"}}
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/memory/"+id+"/forget",
			strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
		}
	})
}
