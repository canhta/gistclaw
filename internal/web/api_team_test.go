package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/teams"
)

func TestTeamAPIListsProfilesAndEditableMembers(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)

	if err := teams.CreateProfile(h.projectProfilesRoot(), "review"); err != nil {
		t.Fatalf("create review profile: %v", err)
	}
	reviewDir := teams.ProfileDir(h.projectProfilesRoot(), "review")
	cfg, err := teams.LoadConfig(reviewDir)
	if err != nil {
		t.Fatalf("load review config: %v", err)
	}
	cfg.Name = "Review Crew"
	if err := teams.WriteConfig(reviewDir, cfg); err != nil {
		t.Fatalf("write review config: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/team", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ActiveProfile struct {
			ID       string `json:"id"`
			SavePath string `json:"save_path"`
		} `json:"active_profile"`
		Profiles []struct {
			ID     string `json:"id"`
			Active bool   `json:"active"`
		} `json:"profiles"`
		Team struct {
			Name         string `json:"name"`
			FrontAgentID string `json:"front_agent_id"`
			MemberCount  int    `json:"member_count"`
			Members      []struct {
				ID                          string   `json:"id"`
				Role                        string   `json:"role"`
				BaseProfile                 string   `json:"base_profile"`
				ToolFamilies                []string `json:"tool_families"`
				DelegationKinds             []string `json:"delegation_kinds"`
				CanMessage                  []string `json:"can_message"`
				SpecialistSummaryVisibility string   `json:"specialist_summary_visibility"`
				IsFront                     bool     `json:"is_front"`
			} `json:"members"`
		} `json:"team"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.ActiveProfile.ID != "default" {
		t.Fatalf("active_profile.id = %q, want %q", resp.ActiveProfile.ID, "default")
	}
	if resp.ActiveProfile.SavePath != filepath.Join(h.projectProfileDir("default"), "team.yaml") {
		t.Fatalf("active_profile.save_path = %q", resp.ActiveProfile.SavePath)
	}
	if len(resp.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(resp.Profiles))
	}
	if resp.Profiles[0].ID != "default" || !resp.Profiles[0].Active {
		t.Fatalf("unexpected default profile %+v", resp.Profiles[0])
	}
	if resp.Profiles[1].ID != "review" || resp.Profiles[1].Active {
		t.Fatalf("unexpected review profile %+v", resp.Profiles[1])
	}
	if resp.Team.Name != "Repo Task Team" {
		t.Fatalf("team.name = %q, want %q", resp.Team.Name, "Repo Task Team")
	}
	if resp.Team.FrontAgentID != "assistant" {
		t.Fatalf("team.front_agent_id = %q, want %q", resp.Team.FrontAgentID, "assistant")
	}
	if resp.Team.MemberCount != 3 || len(resp.Team.Members) != 3 {
		t.Fatalf("unexpected member count %+v", resp.Team)
	}
	front := resp.Team.Members[0]
	if front.ID != "assistant" || !front.IsFront {
		t.Fatalf("unexpected front member %+v", front)
	}
	if front.Role != "front assistant" {
		t.Fatalf("front.role = %q", front.Role)
	}
	if front.BaseProfile != string(model.BaseProfileOperator) {
		t.Fatalf("front.base_profile = %q", front.BaseProfile)
	}
	if len(front.ToolFamilies) != 4 {
		t.Fatalf("expected 4 tool families, got %d", len(front.ToolFamilies))
	}
	if len(front.DelegationKinds) != 2 {
		t.Fatalf("expected 2 delegation kinds, got %d", len(front.DelegationKinds))
	}
	if len(front.CanMessage) != 2 || front.CanMessage[0] != "patcher" || front.CanMessage[1] != "reviewer" {
		t.Fatalf("unexpected can_message %+v", front.CanMessage)
	}
}

func TestTeamMutationAPISelectsAndSavesProfiles(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)

	createBody := bytes.NewBufferString(`{"profile_id":"review"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/team/create", createBody)
	createReq.Header.Set("Content-Type", "application/json")

	createRR := httptest.NewRecorder()
	h.server.ServeHTTP(createRR, createReq)

	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", createRR.Code, createRR.Body.String())
	}
	if _, err := os.Stat(filepath.Join(h.projectProfileDir("review"), "team.yaml")); err != nil {
		t.Fatalf("expected review profile to exist: %v", err)
	}

	var created struct {
		ActiveProfile struct {
			ID string `json:"id"`
		} `json:"active_profile"`
	}
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ActiveProfile.ID != "review" {
		t.Fatalf("created active profile = %q, want %q", created.ActiveProfile.ID, "review")
	}

	saveBody := bytes.NewBufferString(`{
		"team": {
			"name": "Review Crew",
			"front_agent_id": "reviewer",
			"members": [
				{
					"id": "assistant",
					"role": "front assistant",
					"soul_file": "assistant.soul.yaml",
					"base_profile": "operator",
					"tool_families": ["repo_read", "runtime_capability", "delegate"],
					"delegation_kinds": ["write", "review"],
					"can_message": ["patcher", "reviewer"],
					"specialist_summary_visibility": "full",
					"soul_extra": {}
				},
				{
					"id": "patcher",
					"role": "scoped write specialist",
					"soul_file": "patcher.soul.yaml",
					"base_profile": "write",
					"tool_families": ["repo_read", "repo_write"],
					"delegation_kinds": [],
					"can_message": ["assistant", "reviewer"],
					"specialist_summary_visibility": "basic",
					"soul_extra": {}
				},
				{
					"id": "reviewer",
					"role": "diff reviewer",
					"soul_file": "reviewer.soul.yaml",
					"base_profile": "review",
					"tool_families": ["repo_read", "diff_review"],
					"delegation_kinds": [],
					"can_message": ["assistant", "patcher"],
					"specialist_summary_visibility": "basic",
					"soul_extra": {}
				}
			]
		}
	}`)
	saveReq := httptest.NewRequest(http.MethodPost, "/api/team/save", saveBody)
	saveReq.Header.Set("Content-Type", "application/json")

	saveRR := httptest.NewRecorder()
	h.server.ServeHTTP(saveRR, saveReq)

	if saveRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", saveRR.Code, saveRR.Body.String())
	}

	saved, err := teams.LoadConfig(h.projectProfileDir("review"))
	if err != nil {
		t.Fatalf("load saved review profile: %v", err)
	}
	if saved.Name != "Review Crew" {
		t.Fatalf("saved team name = %q, want %q", saved.Name, "Review Crew")
	}
	if saved.FrontAgent != "reviewer" {
		t.Fatalf("saved front agent = %q, want %q", saved.FrontAgent, "reviewer")
	}

	run, err := h.rt.Start(context.Background(), runtime.StartRun{
		ConversationID: "conv-team-api-save",
		AgentID:        "reviewer",
		Objective:      "confirm saved profile",
		CWD:            h.workspaceRoot,
		PreviewOnly:    true,
	})
	if err != nil {
		t.Fatalf("start preview run: %v", err)
	}
	if run.TeamID != "Review Crew" {
		t.Fatalf("run.TeamID = %q, want %q", run.TeamID, "Review Crew")
	}

	selectBody := bytes.NewBufferString(`{"profile_id":"default"}`)
	selectReq := httptest.NewRequest(http.MethodPost, "/api/team/select", selectBody)
	selectReq.Header.Set("Content-Type", "application/json")

	selectRR := httptest.NewRecorder()
	h.server.ServeHTTP(selectRR, selectReq)

	if selectRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", selectRR.Code, selectRR.Body.String())
	}
	profile, err := h.rt.ActiveTeamProfile(context.Background())
	if err != nil {
		t.Fatalf("load active profile: %v", err)
	}
	if profile != "default" {
		t.Fatalf("active profile = %q, want %q", profile, "default")
	}
}

func TestTeamMutationAPIClonesDeletesAndImportsProfiles(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)

	cloneReq := httptest.NewRequest(http.MethodPost, "/api/team/clone", bytes.NewBufferString(`{
		"source_profile_id": "default",
		"profile_id": "ops"
	}`))
	cloneReq.Header.Set("Content-Type", "application/json")

	cloneRR := httptest.NewRecorder()
	h.server.ServeHTTP(cloneRR, cloneReq)

	if cloneRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", cloneRR.Code, cloneRR.Body.String())
	}

	var cloneResp struct {
		Notice        string `json:"notice"`
		ActiveProfile struct {
			ID string `json:"id"`
		} `json:"active_profile"`
	}
	if err := json.Unmarshal(cloneRR.Body.Bytes(), &cloneResp); err != nil {
		t.Fatalf("decode clone response: %v", err)
	}
	if cloneResp.Notice != "Profile ops cloned from default." {
		t.Fatalf("clone notice = %q", cloneResp.Notice)
	}
	if cloneResp.ActiveProfile.ID != "ops" {
		t.Fatalf("clone active profile = %q, want %q", cloneResp.ActiveProfile.ID, "ops")
	}

	cloned, err := teams.LoadConfig(h.projectProfileDir("ops"))
	if err != nil {
		t.Fatalf("load cloned profile: %v", err)
	}
	if cloned.Name != "Repo Task Team" {
		t.Fatalf("cloned team name = %q, want %q", cloned.Name, "Repo Task Team")
	}

	selectReq := httptest.NewRequest(http.MethodPost, "/api/team/select", bytes.NewBufferString(`{"profile_id":"default"}`))
	selectReq.Header.Set("Content-Type", "application/json")
	selectRR := httptest.NewRecorder()
	h.server.ServeHTTP(selectRR, selectReq)
	if selectRR.Code != http.StatusOK {
		t.Fatalf("expected 200 selecting default, got %d body=%s", selectRR.Code, selectRR.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodPost, "/api/team/delete", bytes.NewBufferString(`{"profile_id":"ops"}`))
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteRR := httptest.NewRecorder()
	h.server.ServeHTTP(deleteRR, deleteReq)

	if deleteRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", deleteRR.Code, deleteRR.Body.String())
	}
	if _, err := os.Stat(h.projectProfileDir("ops")); !os.IsNotExist(err) {
		t.Fatalf("expected cloned profile to be removed, err=%v", err)
	}

	var deleteResp struct {
		Notice   string `json:"notice"`
		Profiles []struct {
			ID string `json:"id"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(deleteRR.Body.Bytes(), &deleteResp); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	if deleteResp.Notice != "Profile ops deleted." {
		t.Fatalf("delete notice = %q", deleteResp.Notice)
	}
	for _, profile := range deleteResp.Profiles {
		if profile.ID == "ops" {
			t.Fatalf("deleted profile still present in response: %+v", deleteResp.Profiles)
		}
	}

	defaultCfg, err := teams.LoadConfig(h.teamDir)
	if err != nil {
		t.Fatalf("load default config: %v", err)
	}
	defaultCfg.Name = "Imported Team"
	defaultCfg.FrontAgent = "reviewer"
	importYAML, err := teams.ExportEditableYAML(defaultCfg)
	if err != nil {
		t.Fatalf("export editable yaml: %v", err)
	}

	importReq := httptest.NewRequest(http.MethodPost, "/api/team/import", bytes.NewBufferString(`{"yaml":`+string(jsonMustMarshal(t, string(importYAML)))+`}`))
	importReq.Header.Set("Content-Type", "application/json")
	importRR := httptest.NewRecorder()
	h.server.ServeHTTP(importRR, importReq)

	if importRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", importRR.Code, importRR.Body.String())
	}

	var importResp struct {
		Notice string `json:"notice"`
		Team   struct {
			Name         string `json:"name"`
			FrontAgentID string `json:"front_agent_id"`
		} `json:"team"`
	}
	if err := json.Unmarshal(importRR.Body.Bytes(), &importResp); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	if importResp.Notice != "Imported file loaded. Save Team to apply the change." {
		t.Fatalf("import notice = %q", importResp.Notice)
	}
	if importResp.Team.Name != "Imported Team" {
		t.Fatalf("imported team name = %q, want %q", importResp.Team.Name, "Imported Team")
	}
	if importResp.Team.FrontAgentID != "reviewer" {
		t.Fatalf("imported front agent = %q, want %q", importResp.Team.FrontAgentID, "reviewer")
	}

	saved, err := teams.LoadConfig(h.teamDir)
	if err != nil {
		t.Fatalf("reload default config: %v", err)
	}
	if saved.Name != "Repo Task Team" {
		t.Fatalf("import should not persist until save, got name %q", saved.Name)
	}
	if saved.FrontAgent != "assistant" {
		t.Fatalf("import should not persist until save, got front agent %q", saved.FrontAgent)
	}
}

func TestTeamMutationAPIRejectsInvalidCloneDeleteAndImportRequests(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)

	t.Run("clone requires profile ids", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/team/clone", bytes.NewBufferString(`{"source_profile_id":"default"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("delete rejects active profile", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/team/delete", bytes.NewBufferString(`{"profile_id":"default"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("import rejects invalid yaml", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/team/import", bytes.NewBufferString(`{"yaml":"name: bad\nagents:\n  - id: only"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d body=%s", rr.Code, rr.Body.String())
		}
	})
}

func TestTeamMutationAPIRejectsInvalidSelectCreateAndSaveRequests(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)

	t.Run("select rejects invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/team/select", bytes.NewBufferString(`{`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("select rejects missing profile", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/team/select", bytes.NewBufferString(`{"profile_id":"missing"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("create rejects duplicate profile", func(t *testing.T) {
		createReq := httptest.NewRequest(http.MethodPost, "/api/team/create", bytes.NewBufferString(`{"profile_id":"review"}`))
		createReq.Header.Set("Content-Type", "application/json")
		createRR := httptest.NewRecorder()
		h.server.ServeHTTP(createRR, createReq)
		if createRR.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d body=%s", createRR.Code, createRR.Body.String())
		}

		dupReq := httptest.NewRequest(http.MethodPost, "/api/team/create", bytes.NewBufferString(`{"profile_id":"review"}`))
		dupReq.Header.Set("Content-Type", "application/json")
		dupRR := httptest.NewRecorder()
		h.server.ServeHTTP(dupRR, dupReq)

		if dupRR.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d body=%s", dupRR.Code, dupRR.Body.String())
		}
	})

	t.Run("save rejects invalid team config", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/team/save", bytes.NewBufferString(`{
			"team": {
				"name": "Broken Team",
				"front_agent_id": "assistant",
				"members": [
					{
						"id": "",
						"role": "front assistant",
						"base_profile": "operator",
						"tool_families": ["repo_read"],
						"delegation_kinds": ["write"],
						"can_message": ["patcher"],
						"specialist_summary_visibility": "full",
						"soul_extra": {}
					}
				]
			}
		}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d body=%s", rr.Code, rr.Body.String())
		}
	})
}

func jsonMustMarshal(t *testing.T, value string) []byte {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json string: %v", err)
	}
	return raw
}
