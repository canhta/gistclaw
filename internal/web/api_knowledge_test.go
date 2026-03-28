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
