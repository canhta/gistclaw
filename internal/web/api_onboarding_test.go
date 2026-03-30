package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	authpkg "github.com/canhta/gistclaw/internal/auth"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

func TestOnboardingAPIReturnsPendingStarterProjectState(t *testing.T) {
	t.Parallel()

	h := newServerHarnessOnboardingPending(t)
	if err := os.MkdirAll(filepath.Join(h.workspaceRoot, "internal"), 0o755); err != nil {
		t.Fatalf("mkdir internal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(h.workspaceRoot, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/onboarding", nil)
	req.Header.Set("Authorization", "Bearer "+h.adminToken)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Completed bool   `json:"completed"`
		EntryHref string `json:"entry_href"`
		Project   *struct {
			ActiveName string `json:"active_name"`
			ActivePath string `json:"active_path"`
		} `json:"project"`
		Preview struct {
			Available   bool   `json:"available"`
			StatusLabel string `json:"status_label"`
			Detail      string `json:"detail"`
		} `json:"preview"`
		SuggestedTasks []struct {
			Kind string `json:"kind"`
		} `json:"suggested_tasks"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Completed {
		t.Fatal("expected onboarding to remain pending")
	}
	if resp.EntryHref != pageOnboarding {
		t.Fatalf("entry_href = %q, want %q", resp.EntryHref, pageOnboarding)
	}
	if resp.Project == nil {
		t.Fatal("expected starter project in onboarding response")
	}
	if resp.Project.ActiveName != "starter-project" {
		t.Fatalf("active_name = %q, want %q", resp.Project.ActiveName, "starter-project")
	}
	if resp.Project.ActivePath != h.workspaceRoot {
		t.Fatalf("active_path = %q, want %q", resp.Project.ActivePath, h.workspaceRoot)
	}
	if !resp.Preview.Available || resp.Preview.StatusLabel != "Ready to launch" {
		t.Fatalf("unexpected preview readiness: %+v", resp.Preview)
	}
	if len(resp.SuggestedTasks) < 3 {
		t.Fatalf("expected at least 3 suggested tasks, got %d", len(resp.SuggestedTasks))
	}

	kinds := make([]string, 0, len(resp.SuggestedTasks))
	for _, task := range resp.SuggestedTasks {
		kinds = append(kinds, task.Kind)
	}
	for _, want := range []string{"explain", "review", "improve"} {
		if !slices.Contains(kinds, want) {
			t.Fatalf("expected suggested task kind %q in %v", want, kinds)
		}
	}
}

func TestOnboardingAPIReturnsBlockedPreviewStateWhenRuntimeUnavailable(t *testing.T) {
	t.Parallel()

	h := newServerHarnessOnboardingPending(t)
	h.rawServer.rt = nil

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/onboarding", nil)
	req.Header.Set("Authorization", "Bearer "+h.adminToken)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Preview struct {
			Available   bool   `json:"available"`
			StatusLabel string `json:"status_label"`
			Detail      string `json:"detail"`
			Actions     []struct {
				Label string `json:"label"`
				Href  string `json:"href"`
			} `json:"actions"`
			Checks []struct {
				Label   string `json:"label"`
				Command string `json:"command"`
			} `json:"checks"`
		} `json:"preview"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Preview.Available {
		t.Fatalf("expected preview to be blocked, got %+v", resp.Preview)
	}
	if resp.Preview.StatusLabel != "Runtime unavailable" {
		t.Fatalf("unexpected preview label: %+v", resp.Preview)
	}
	if resp.Preview.Detail != "Preview runs are unavailable right now. Check the runtime configuration and try again." {
		t.Fatalf("unexpected preview detail: %+v", resp.Preview)
	}
	if len(resp.Preview.Actions) != 1 || resp.Preview.Actions[0].Label != "Open Update board" || resp.Preview.Actions[0].Href != "/update" {
		t.Fatalf("unexpected preview actions: %+v", resp.Preview.Actions)
	}
	if len(resp.Preview.Checks) != 3 {
		t.Fatalf("unexpected preview checks: %+v", resp.Preview.Checks)
	}
	if resp.Preview.Checks[0].Command != "gistclaw doctor --config /etc/gistclaw/config.yaml" {
		t.Fatalf("unexpected first preview check: %+v", resp.Preview.Checks[0])
	}
	if resp.Preview.Checks[1].Command != "gistclaw inspect status --config /etc/gistclaw/config.yaml" {
		t.Fatalf("unexpected second preview check: %+v", resp.Preview.Checks[1])
	}
	if resp.Preview.Checks[2].Command != "gistclaw inspect status --config /opt/homebrew/etc/gistclaw/config.yaml" {
		t.Fatalf("unexpected third preview check: %+v", resp.Preview.Checks[2])
	}
}

func TestOnboardingProjectAPIActivatesRepoAndMarksCompleted(t *testing.T) {
	t.Parallel()

	h := newServerHarnessNoWorkspace(t)
	repo := makeGitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/onboarding/project", bytes.NewBufferString(`{"source":"existing_repo","project_path":"`+repo+`"}`))
	req.Header.Set("Authorization", "Bearer "+h.adminToken)
	req.Header.Set("Content-Type", "application/json")
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Completed bool   `json:"completed"`
		EntryHref string `json:"entry_href"`
		Project   struct {
			ActivePath string `json:"active_path"`
		} `json:"project"`
		SuggestedTasks []struct {
			Description string `json:"description"`
		} `json:"suggested_tasks"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Completed {
		t.Fatal("expected onboarding to be completed after binding a repo")
	}
	if resp.EntryHref != pageChat {
		t.Fatalf("entry_href = %q, want %q", resp.EntryHref, pageChat)
	}
	if resp.Project.ActivePath != repo {
		t.Fatalf("active_path = %q, want %q", resp.Project.ActivePath, repo)
	}
	if len(resp.SuggestedTasks) == 0 {
		t.Fatal("expected suggested tasks after binding project")
	}

	project, err := runtime.ActiveProject(context.Background(), h.db)
	if err != nil {
		t.Fatalf("load active project: %v", err)
	}
	if project.PrimaryPath != repo {
		t.Fatalf("primary_path = %q, want %q", project.PrimaryPath, repo)
	}
	if !onboardingCompleted(h.db) {
		t.Fatal("expected onboarding state to be marked complete")
	}
}

func TestOnboardingProjectAPIUsesStarterProject(t *testing.T) {
	t.Parallel()

	h := newServerHarnessOnboardingPending(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/onboarding/project", bytes.NewBufferString(`{"source":"starter"}`))
	req.Header.Set("Authorization", "Bearer "+h.adminToken)
	req.Header.Set("Content-Type", "application/json")
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Completed bool   `json:"completed"`
		EntryHref string `json:"entry_href"`
		Project   struct {
			ActivePath string `json:"active_path"`
		} `json:"project"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Completed {
		t.Fatal("expected onboarding to complete with starter project")
	}
	if resp.EntryHref != pageChat {
		t.Fatalf("entry_href = %q, want %q", resp.EntryHref, pageChat)
	}
	if resp.Project.ActivePath != h.workspaceRoot {
		t.Fatalf("active_path = %q, want %q", resp.Project.ActivePath, h.workspaceRoot)
	}
}

func TestOnboardingProjectAPICreatesNewProjectDirectory(t *testing.T) {
	t.Parallel()

	h := newServerHarnessNoWorkspace(t)
	parent := t.TempDir()
	projectPath := filepath.Join(parent, "fresh-repo")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/onboarding/project", bytes.NewBufferString(`{"source":"new_project","project_path":"`+projectPath+`"}`))
	req.Header.Set("Authorization", "Bearer "+h.adminToken)
	req.Header.Set("Content-Type", "application/json")
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if info, err := os.Stat(projectPath); err != nil {
		t.Fatalf("stat created project path: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", projectPath)
	}
}

func TestOnboardingProjectAPIRejectsInvalidBodies(t *testing.T) {
	t.Parallel()

	h := newServerHarnessOnboardingPending(t)

	t.Run("invalid json", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/onboarding/project", bytes.NewBufferString(`{`))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/json")
		h.rawServer.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("unknown source", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/onboarding/project", bytes.NewBufferString(`{"source":"mystery"}`))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/json")
		h.rawServer.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
	})
}

func TestOnboardingPreviewAPIStartsPreviewRunAndReturnsWorkPath(t *testing.T) {
	t.Parallel()

	prov := &blockingProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	h := newServerHarnessWithProvider(t, prov)
	if _, err := h.db.RawDB().Exec("DELETE FROM settings WHERE key = 'onboarding_completed_at'"); err != nil {
		t.Fatalf("remove onboarding_completed_at: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/onboarding/preview", bytes.NewBufferString(`{"task":"Explain the main package structure"}`))
	req.Header.Set("Authorization", "Bearer "+h.adminToken)
	req.Header.Set("Content-Type", "application/json")

	done := make(chan struct{})
	go func() {
		h.rawServer.ServeHTTP(rr, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		close(prov.release)
		<-done
		t.Fatal("expected preview api to return before provider completes")
	}

	if rr.Code != http.StatusAccepted {
		close(prov.release)
		t.Fatalf("expected 202, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		RunID    string `json:"run_id"`
		NextHref string `json:"next_href"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		close(prov.release)
		t.Fatalf("decode response: %v", err)
	}
	if resp.RunID == "" {
		close(prov.release)
		t.Fatal("expected run_id in preview response")
	}
	if resp.NextHref != pageChat {
		close(prov.release)
		t.Fatalf("next_href = %q, want %q", resp.NextHref, pageChat)
	}

	select {
	case <-prov.started:
	case <-time.After(time.Second):
		close(prov.release)
		t.Fatal("expected provider to keep running in the background")
	}

	var status string
	if err := h.db.RawDB().QueryRow("SELECT status FROM runs WHERE id = ?", resp.RunID).Scan(&status); err != nil {
		close(prov.release)
		t.Fatalf("query run status: %v", err)
	}
	if status != "active" {
		close(prov.release)
		t.Fatalf("expected preview run to stay active while provider is blocked, got %q", status)
	}

	close(prov.release)
	waitForRunStatus(t, h.db, resp.RunID, "completed")
}

func TestOnboardingPreviewAPIRejectsInvalidState(t *testing.T) {
	t.Parallel()

	t.Run("invalid json", func(t *testing.T) {
		h := newServerHarnessOnboardingPending(t)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/onboarding/preview", bytes.NewBufferString(`{`))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/json")
		h.rawServer.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("blank task", func(t *testing.T) {
		h := newServerHarnessOnboardingPending(t)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/onboarding/preview", bytes.NewBufferString(`{"task":"   "}`))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/json")
		h.rawServer.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("no bound project", func(t *testing.T) {
		h := newServerHarnessNoWorkspace(t)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/onboarding/preview", bytes.NewBufferString(`{"task":"Explain the repo"}`))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/json")
		h.rawServer.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("runtime unavailable", func(t *testing.T) {
		h := newServerHarnessOnboardingPending(t)
		h.rawServer.rt = nil
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/onboarding/preview", bytes.NewBufferString(`{"task":"Explain the repo"}`))
		req.Header.Set("Authorization", "Bearer "+h.adminToken)
		req.Header.Set("Content-Type", "application/json")
		h.rawServer.ServeHTTP(rr, req)

		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
		}
	})
}

func TestAuthenticatedRootServesSPAIndex(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	if err := authpkg.SetPassword(context.Background(), h.db, "secret-pass", time.Now().UTC()); err != nil {
		t.Fatalf("set password: %v", err)
	}
	sessionCookie, deviceCookie := loginForTest(t, h, "secret-pass")

	wantBody, err := readSPAAsset("index.html")
	if err != nil {
		t.Fatalf("read spa index: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(sessionCookie)
	req.AddCookie(deviceCookie)
	h.rawServer.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != string(wantBody) {
		t.Fatalf("expected root route to serve spa index")
	}
}

func TestOnboardingPreviewStartErrorMessages(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "daily cap",
			err:  runtime.ErrDailyCap,
			want: "Preview runs are blocked by the daily cost cap. Raise the cap and try again.",
		},
		{
			name: "budget exhausted",
			err:  runtime.ErrBudgetExhausted,
			want: "Preview runs are blocked by the per-run token budget. Raise the budget and try again.",
		},
		{
			name: "provider auth",
			err: &model.ProviderError{
				Code:    model.ProviderErrorCode("authentication_error"),
				Message: "invalid api key",
			},
			want: "Unable to start the preview run right now. Check the runtime configuration and try again.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := onboardingPreviewStartError(tc.err); got != tc.want {
				t.Fatalf("onboardingPreviewStartError(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestBootstrapProjectPointerHandlesEmptyAndLoadedProjects(t *testing.T) {
	t.Parallel()

	if got := bootstrapProjectPointer(model.Project{}); got != nil {
		t.Fatalf("expected nil for empty project, got %+v", got)
	}

	got := bootstrapProjectPointer(model.Project{
		ID:          "proj-primary",
		Name:        "starter-project",
		PrimaryPath: "/tmp/starter-project",
	})
	if got == nil {
		t.Fatal("expected project pointer")
	}
	if got.ActiveID != "proj-primary" || got.ActiveName != "starter-project" || got.ActivePath != "/tmp/starter-project" {
		t.Fatalf("unexpected project pointer %+v", got)
	}
}
