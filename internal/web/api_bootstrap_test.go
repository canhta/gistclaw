package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBootstrapAPIReportsUserFirstNavigationAndProjectContext(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/bootstrap", nil)
	req.Header.Set("Authorization", "Bearer "+h.adminToken)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Project struct {
			ActiveName string `json:"active_name"`
			ActivePath string `json:"active_path"`
		} `json:"project"`
		Navigation []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
			Href  string `json:"href"`
		} `json:"navigation"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Project.ActiveName != "starter-project" {
		t.Fatalf("active_name = %q, want %q", resp.Project.ActiveName, "starter-project")
	}
	if resp.Project.ActivePath != h.workspaceRoot {
		t.Fatalf("active_path = %q, want %q", resp.Project.ActivePath, h.workspaceRoot)
	}

	want := []struct {
		ID    string
		Label string
		Href  string
	}{
		{ID: "work", Label: "Work", Href: "/work"},
		{ID: "team", Label: "Team", Href: "/team"},
		{ID: "knowledge", Label: "Knowledge", Href: "/knowledge"},
		{ID: "recover", Label: "Recover", Href: "/recover"},
		{ID: "conversations", Label: "Conversations", Href: "/conversations"},
		{ID: "automate", Label: "Automate", Href: "/automate"},
		{ID: "history", Label: "History", Href: "/history"},
		{ID: "settings", Label: "Settings", Href: "/settings"},
	}
	if len(resp.Navigation) != len(want) {
		t.Fatalf("navigation length = %d, want %d", len(resp.Navigation), len(want))
	}
	for idx, item := range want {
		if resp.Navigation[idx].ID != item.ID || resp.Navigation[idx].Label != item.Label || resp.Navigation[idx].Href != item.Href {
			t.Fatalf("navigation[%d] = %+v, want %+v", idx, resp.Navigation[idx], item)
		}
	}
}
