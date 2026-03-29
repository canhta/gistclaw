package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKnowledgeAPIListsAndMutatesMemoryItems(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	otherProjectID := h.insertProject(t, "seo-test", t.TempDir())
	targetID := seedMemoryFact(t, h, "assistant", "local", "capture operator preference", "model")
	seedMemoryFact(t, h, "patcher", "team", "shared repo rule", "system")
	seedMemoryFactInProject(t, h, otherProjectID, "assistant", "local", "foreign project fact", "model")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/knowledge", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Filters struct {
			Scope   string `json:"scope"`
			AgentID string `json:"agent_id"`
			Query   string `json:"query"`
			Limit   int    `json:"limit"`
		} `json:"filters"`
		Summary struct {
			VisibleCount int `json:"visible_count"`
		} `json:"summary"`
		Items []struct {
			ID      string `json:"id"`
			AgentID string `json:"agent_id"`
			Scope   string `json:"scope"`
			Content string `json:"content"`
			Source  string `json:"source"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Filters.Limit != 20 {
		t.Fatalf("filters.limit = %d, want 20", resp.Filters.Limit)
	}
	if resp.Summary.VisibleCount != 2 {
		t.Fatalf("visible_count = %d, want 2", resp.Summary.VisibleCount)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	for _, item := range resp.Items {
		if item.Content == "foreign project fact" {
			t.Fatalf("expected foreign project fact to be hidden: %+v", item)
		}
	}

	editBody := bytes.NewBufferString(`{"content":"captured operator preference"}`)
	editReq := httptest.NewRequest(http.MethodPost, "/api/knowledge/"+targetID+"/edit", editBody)
	editReq.Header.Set("Content-Type", "application/json")

	editRR := httptest.NewRecorder()
	h.server.ServeHTTP(editRR, editReq)

	if editRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", editRR.Code, editRR.Body.String())
	}

	item, err := h.rt.Memory().GetByID(context.Background(), h.activeProjectID, targetID)
	if err != nil {
		t.Fatalf("load edited memory item: %v", err)
	}
	if item.Content != "captured operator preference" {
		t.Fatalf("edited content = %q, want %q", item.Content, "captured operator preference")
	}

	forgetReq := httptest.NewRequest(http.MethodPost, "/api/knowledge/"+targetID+"/forget", bytes.NewBufferString(`{}`))
	forgetReq.Header.Set("Content-Type", "application/json")

	forgetRR := httptest.NewRecorder()
	h.server.ServeHTTP(forgetRR, forgetReq)

	if forgetRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", forgetRR.Code, forgetRR.Body.String())
	}

	items, err := h.rt.Memory().Filter(context.Background(), memoryFilterArg(h.activeProjectID, "assistant", ""))
	if err != nil {
		t.Fatalf("filter memory after forget: %v", err)
	}
	for _, candidate := range items {
		if candidate.ID == targetID {
			t.Fatalf("expected forgotten item %q to be absent", targetID)
		}
	}
}

func TestKnowledgeAPIAppliesFiltersAndMissingItemResponses(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	seedMemoryFact(t, h, "assistant", "local", "capture operator preference", "model")
	seedMemoryFact(t, h, "assistant", "team", "shared repo rule", "system")
	seedMemoryFact(t, h, "patcher", "local", "patch queue detail", "model")

	filteredRR := httptest.NewRecorder()
	filteredReq := httptest.NewRequest(http.MethodGet, "/api/knowledge?scope=local&agent_id=assistant&q=operator&limit=5", nil)
	h.server.ServeHTTP(filteredRR, filteredReq)

	if filteredRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", filteredRR.Code, filteredRR.Body.String())
	}

	var filtered struct {
		Headline string `json:"headline"`
		Filters  struct {
			Scope   string `json:"scope"`
			AgentID string `json:"agent_id"`
			Query   string `json:"query"`
			Limit   int    `json:"limit"`
		} `json:"filters"`
		Summary struct {
			VisibleCount int `json:"visible_count"`
		} `json:"summary"`
		Items []struct {
			AgentID string `json:"agent_id"`
			Scope   string `json:"scope"`
			Content string `json:"content"`
		} `json:"items"`
	}
	if err := json.Unmarshal(filteredRR.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("decode filtered response: %v", err)
	}
	if filtered.Headline != "Filtered knowledge for the current project." {
		t.Fatalf("headline = %q", filtered.Headline)
	}
	if filtered.Filters.Scope != "local" || filtered.Filters.AgentID != "assistant" || filtered.Filters.Query != "operator" {
		t.Fatalf("unexpected filters %+v", filtered.Filters)
	}
	if filtered.Filters.Limit != 5 {
		t.Fatalf("filters.limit = %d, want 5", filtered.Filters.Limit)
	}
	if filtered.Summary.VisibleCount != 1 || len(filtered.Items) != 1 {
		t.Fatalf("expected 1 filtered item, got summary=%d len=%d", filtered.Summary.VisibleCount, len(filtered.Items))
	}
	if filtered.Items[0].Content != "capture operator preference" {
		t.Fatalf("unexpected filtered item %+v", filtered.Items[0])
	}

	emptyRR := httptest.NewRecorder()
	emptyReq := httptest.NewRequest(http.MethodGet, "/api/knowledge?q=missing", nil)
	h.server.ServeHTTP(emptyRR, emptyReq)

	if emptyRR.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty state, got %d body=%s", emptyRR.Code, emptyRR.Body.String())
	}

	var empty struct {
		Headline string `json:"headline"`
		Summary  struct {
			VisibleCount int `json:"visible_count"`
		} `json:"summary"`
		Items []knowledgeItemResponse `json:"items"`
	}
	if err := json.Unmarshal(emptyRR.Body.Bytes(), &empty); err != nil {
		t.Fatalf("decode empty response: %v", err)
	}
	if empty.Headline != "No saved knowledge is shaping work yet." {
		t.Fatalf("empty headline = %q", empty.Headline)
	}
	if empty.Summary.VisibleCount != 0 || len(empty.Items) != 0 {
		t.Fatalf("expected empty knowledge state, got %+v", empty)
	}

	editReq := httptest.NewRequest(http.MethodPost, "/api/knowledge/missing/edit", bytes.NewBufferString(`{"content":"updated"}`))
	editReq.Header.Set("Content-Type", "application/json")
	editRR := httptest.NewRecorder()
	h.server.ServeHTTP(editRR, editReq)

	if editRR.Code != http.StatusNotFound {
		t.Fatalf("expected 404 editing missing item, got %d body=%s", editRR.Code, editRR.Body.String())
	}

	forgetReq := httptest.NewRequest(http.MethodPost, "/api/knowledge/missing/forget", bytes.NewBufferString(`{}`))
	forgetReq.Header.Set("Content-Type", "application/json")
	forgetRR := httptest.NewRecorder()
	h.server.ServeHTTP(forgetRR, forgetReq)

	if forgetRR.Code != http.StatusNotFound {
		t.Fatalf("expected 404 forgetting missing item, got %d body=%s", forgetRR.Code, forgetRR.Body.String())
	}
}

func TestKnowledgeAPIWithoutActiveProjectReturnsEmptySurface(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	if _, err := h.db.RawDB().Exec("DELETE FROM settings WHERE key = 'active_project_id'"); err != nil {
		t.Fatalf("remove active_project_id: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/knowledge?scope=local&agent_id=assistant&q=operator&limit=5", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Notice   string `json:"notice"`
		Headline string `json:"headline"`
		Filters  struct {
			Scope   string `json:"scope"`
			AgentID string `json:"agent_id"`
			Query   string `json:"query"`
			Limit   int    `json:"limit"`
		} `json:"filters"`
		Summary struct {
			VisibleCount int `json:"visible_count"`
		} `json:"summary"`
		Items  []knowledgeItemResponse `json:"items"`
		Paging struct {
			HasNext bool `json:"has_next"`
			HasPrev bool `json:"has_prev"`
		} `json:"paging"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode empty response: %v", err)
	}
	if resp.Notice != "Choose an active project to load saved knowledge." {
		t.Fatalf("notice = %q", resp.Notice)
	}
	if resp.Headline != "No saved knowledge is shaping work yet." {
		t.Fatalf("headline = %q", resp.Headline)
	}
	if resp.Filters.Scope != "local" || resp.Filters.AgentID != "assistant" || resp.Filters.Query != "operator" {
		t.Fatalf("unexpected filters %+v", resp.Filters)
	}
	if resp.Filters.Limit != 5 {
		t.Fatalf("filters.limit = %d, want 5", resp.Filters.Limit)
	}
	if resp.Summary.VisibleCount != 0 {
		t.Fatalf("visible_count = %d, want 0", resp.Summary.VisibleCount)
	}
	if len(resp.Items) != 0 {
		t.Fatalf("expected no items, got %d", len(resp.Items))
	}
	if resp.Paging.HasNext || resp.Paging.HasPrev {
		t.Fatalf("expected empty paging, got %+v", resp.Paging)
	}
}

func TestKnowledgeEditRejectsInvalidBodies(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	id := seedMemoryFact(t, h, "assistant", "local", "capture operator preference", "model")

	t.Run("invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/knowledge/"+id+"/edit", bytes.NewBufferString(`{`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("blank content", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/knowledge/"+id+"/edit", bytes.NewBufferString(`{"content":"   "}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
	})
}
