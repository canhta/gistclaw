package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

// newServerHarnessOnboardingPending returns a harness with an active project
// but no completed onboarding state.
func newServerHarnessOnboardingPending(t *testing.T) *serverHarness {
	t.Helper()
	h := newServerHarness(t)
	if _, err := h.db.RawDB().Exec("DELETE FROM settings WHERE key = 'onboarding_completed_at'"); err != nil {
		t.Fatalf("remove onboarding_completed_at: %v", err)
	}
	return h
}

// newServerHarnessNoWorkspace returns a harness with no active project
// and no completed onboarding state.
func newServerHarnessNoWorkspace(t *testing.T) *serverHarness {
	t.Helper()
	h := newServerHarnessOnboardingPending(t)
	if _, err := h.db.RawDB().Exec("DELETE FROM settings WHERE key = 'active_project_id'"); err != nil {
		t.Fatalf("remove active_project_id: %v", err)
	}
	return h
}

// TestOnboarding_RedirectsWhenIncomplete verifies that onboarding gating keys
// off explicit onboarding state rather than only a missing workspace.
func TestOnboarding_RedirectsWhenIncomplete(t *testing.T) {
	h := newServerHarnessOnboardingPending(t)

	paths := []string{"/operate/runs", "/operate/start-task", "/recover/approvals", "/configure/settings"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			h.server.ServeHTTP(w, req)
			got := w.Code
			if got != http.StatusSeeOther {
				t.Fatalf("%s: expected redirect 303 when no workspace bound, got %d", path, got)
			}
			loc := w.Header().Get("Location")
			if !strings.HasPrefix(loc, "/onboarding") {
				t.Fatalf("%s: expected redirect to /onboarding, got %q", path, loc)
			}
		})
	}
}

// TestOnboarding_RedirectsWhenNoWorkspace verifies that when onboarding is
// incomplete and no workspace is available, non-static requests redirect to
// /onboarding.
func TestOnboarding_RedirectsWhenNoWorkspace(t *testing.T) {
	h := newServerHarnessNoWorkspace(t)

	paths := []string{"/operate/runs", "/operate/start-task", "/recover/approvals", "/configure/settings"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			h.server.ServeHTTP(w, req)
			got := w.Code
			if got != http.StatusSeeOther {
				t.Fatalf("%s: expected redirect 303 when no workspace bound, got %d", path, got)
			}
			loc := w.Header().Get("Location")
			if !strings.HasPrefix(loc, "/onboarding") {
				t.Fatalf("%s: expected redirect to /onboarding, got %q", path, loc)
			}
		})
	}
}

// TestOnboardingStep1_Renders verifies that GET /onboarding renders a form.
func TestOnboardingStep1_Renders(t *testing.T) {
	h := newServerHarnessNoWorkspace(t)
	req := httptest.NewRequest(http.MethodGet, "/onboarding", nil)
	w := httptest.NewRecorder()
	h.server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "form") {
		t.Fatalf("expected form element in onboarding page, got: %s", truncate(body, 200))
	}
}

func TestOnboardingStep1_RendersWhenStarterProjectExists(t *testing.T) {
	h := newServerHarnessOnboardingPending(t)
	req := httptest.NewRequest(http.MethodGet, "/onboarding", nil)
	w := httptest.NewRecorder()
	h.server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when onboarding is pending, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "form") {
		t.Fatalf("expected onboarding page to render even with starter project, got: %s", truncate(w.Body.String(), 200))
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "starter project") {
		t.Fatalf("expected onboarding page to mention starter project, got: %s", truncate(w.Body.String(), 300))
	}
}

// TestOnboardingStep1_PathNotExist verifies that submitting a non-existent path
// re-renders step 1 with an inline error.
func TestOnboardingStep1_PathNotExist(t *testing.T) {
	h := newServerHarnessNoWorkspace(t)
	form := url.Values{"project_path": {"/this/path/does/not/exist/ever"}}
	req := httptest.NewRequest(http.MethodPost, "/onboarding", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.server.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for non-existent path, got %d", w.Code)
	}
}

// TestOnboardingStep1_NotAGitRepo verifies that submitting a path that exists
// but has no .git directory returns an inline error.
func TestOnboardingStep1_NotAGitRepo(t *testing.T) {
	h := newServerHarnessNoWorkspace(t)
	dir := t.TempDir() // exists, but no .git
	form := url.Values{"project_path": {dir}}
	req := httptest.NewRequest(http.MethodPost, "/onboarding", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.server.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for non-git dir, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "git") {
		t.Fatalf("expected error mentioning git, got: %s", truncate(w.Body.String(), 200))
	}
}

// TestOnboardingStep1_ValidGitRepo verifies that submitting a valid git repo
// activates the project path and redirects to step 2.
func TestOnboardingStep1_ValidGitRepo(t *testing.T) {
	h := newServerHarnessNoWorkspace(t)
	dir := makeGitRepo(t)
	form := url.Values{"project_path": {dir}}
	req := httptest.NewRequest(http.MethodPost, "/onboarding", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.server.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303, got %d body=%s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if loc != "/onboarding/step/2" {
		t.Fatalf("expected redirect to /onboarding/step/2, got %q", loc)
	}
	project, err := runtime.ActiveProject(req.Context(), h.db)
	if err != nil {
		t.Fatalf("ActiveProject: %v", err)
	}
	if project.PrimaryPath != dir {
		t.Fatalf("expected primary_path=%q, got %q", dir, project.PrimaryPath)
	}
}

// TestOnboardingStep2_RepoScanReturnsCandidates verifies that the scan returns
// at least 3 task candidates for a non-empty git repo.
func TestOnboardingStep2_RepoScanReturnsCandidates(t *testing.T) {
	dir := makeGitRepo(t)
	// Add a file so the scan has something to work with.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	candidates := scanRepoSignals(dir)
	if len(candidates) < 3 {
		t.Fatalf("expected >= 3 candidates, got %d: %v", len(candidates), candidates)
	}
}

// TestOnboardingStep2_FallbackTrioForEmptyRepo verifies that an empty repo
// returns exactly 3 fallback candidates, one per type.
func TestOnboardingStep2_FallbackTrioForEmptyRepo(t *testing.T) {
	dir := makeGitRepo(t) // empty — no files beyond .git
	candidates := scanRepoSignals(dir)
	if len(candidates) != 3 {
		t.Fatalf("expected 3 fallback candidates for empty repo, got %d", len(candidates))
	}
	types := map[string]bool{}
	for _, c := range candidates {
		types[c.Kind] = true
	}
	for _, kind := range []string{"explain", "review", "improve"} {
		if !types[kind] {
			t.Fatalf("expected candidate of kind %q in fallback trio, got kinds: %v", kind, types)
		}
	}
}

// TestOnboardingStep2_BalancedTrio verifies that the shortlist for a non-empty
// repo contains at least one candidate of each type.
func TestOnboardingStep2_BalancedTrio(t *testing.T) {
	dir := makeGitRepo(t)
	// Add files so all three heuristics can fire.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "internal"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	candidates := scanRepoSignals(dir)
	types := map[string]bool{}
	for _, c := range candidates {
		types[c.Kind] = true
	}
	for _, kind := range []string{"explain", "review", "improve"} {
		if !types[kind] {
			t.Fatalf("expected balanced trio to include kind %q, got: %v", kind, types)
		}
	}
}

// TestOnboardingStep3_TaskPickDispatchesPreviewRun verifies that picking a task
// at step 3 dispatches a preview-only run and redirects to step 4.
func TestOnboardingStep3_TaskPickDispatchesPreviewRun(t *testing.T) {
	h := newServerHarness(t) // has an active project already set
	form := url.Values{"task": {"Explain the main package structure"}}
	req := httptest.NewRequest(http.MethodPost, "/onboarding/step/3", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.server.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303, got %d body=%s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "/onboarding/step/4/") {
		t.Fatalf("expected redirect to /onboarding/step/4/{runID}, got %q", loc)
	}
}

func TestOnboardingStep3_RendersAccessibleTaskChooser(t *testing.T) {
	h := newServerHarnessOnboardingPending(t)

	req := httptest.NewRequest(http.MethodGet, "/onboarding/step/3", nil)
	w := httptest.NewRecorder()
	h.server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	for _, want := range []string{
		"<fieldset",
		"<legend>Pick a Preview Task</legend>",
		`type="radio"`,
		`for="task-0"`,
		`id="task-0"`,
		`checked`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected onboarding task chooser to contain %q:\n%s", want, body)
		}
	}
}

func TestOnboardingStep3_TaskPickRedirectsBeforeProviderCompletes(t *testing.T) {
	prov := &blockingProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	h := newServerHarnessWithProvider(t, prov)

	form := url.Values{"task": {"Explain the main package structure"}}
	req := httptest.NewRequest(http.MethodPost, "/onboarding/step/3", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.server.ServeHTTP(w, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		close(prov.release)
		<-done
		t.Fatal("expected onboarding preview to redirect before the provider completes")
	}

	if w.Code != http.StatusSeeOther {
		close(prov.release)
		t.Fatalf("expected 303 redirect, got %d body=%s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "/onboarding/step/4/") {
		close(prov.release)
		t.Fatalf("expected redirect to /onboarding/step/4/{runID}, got %q", loc)
	}

	select {
	case <-prov.started:
	case <-time.After(time.Second):
		close(prov.release)
		t.Fatal("expected provider work to continue in the background")
	}

	runID := strings.TrimPrefix(loc, "/onboarding/step/4/")
	var status string
	if err := h.db.RawDB().QueryRow("SELECT status FROM runs WHERE id = ?", runID).Scan(&status); err != nil {
		close(prov.release)
		t.Fatalf("query run status: %v", err)
	}
	if status != "active" {
		close(prov.release)
		t.Fatalf("expected background preview run to stay active while provider is blocked, got %q", status)
	}

	close(prov.release)
	waitForRunStatus(t, h.db, runID, "completed")
}

func TestOnboardingStep3_TaskPickRedirectsWhenProviderAuthFails(t *testing.T) {
	prov := runtime.NewMockProvider(nil, []error{
		&model.ProviderError{
			Code:    model.ProviderErrorCode("authentication_error"),
			Message: "invalid x-api-key",
		},
	})
	h := newServerHarnessWithProvider(t, prov)

	form := url.Values{"task": {"Explain the main package structure"}}
	req := httptest.NewRequest(http.MethodPost, "/onboarding/step/3", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.server.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d body=%s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "/onboarding/step/4/") {
		t.Fatalf("expected redirect to /onboarding/step/4/{runID}, got %q", loc)
	}

	runID := strings.TrimPrefix(loc, "/onboarding/step/4/")
	waitForRunStatus(t, h.db, runID, "failed")
}

// TestOnboardingStep3_NoModelCallsDuringScan verifies that repo scanning does
// not trigger any model API calls. scanRepoSignals is a pure heuristic function
// that reads only the local filesystem.
func TestOnboardingStep3_NoModelCallsDuringScan(t *testing.T) {
	dir := makeGitRepo(t)
	// scanRepoSignals must be a pure filesystem scan — no provider calls.
	// We verify it doesn't panic or error; model isolation is structural (no
	// provider reference is passed to the function).
	candidates := scanRepoSignals(dir)
	if candidates == nil {
		t.Fatal("scanRepoSignals returned nil")
	}
}

// TestOnboarding_RedirectsToOperateRunsWhenCompleted verifies that GET
// /onboarding redirects to /operate/runs only after onboarding has been
// explicitly completed.
func TestOnboarding_RedirectsToOperateRunsWhenCompleted(t *testing.T) {
	h := newServerHarness(t)
	req := httptest.NewRequest(http.MethodGet, "/onboarding", nil)
	w := httptest.NewRecorder()
	h.server.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect when onboarding is completed, got %d body=%s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if loc != "/operate/runs" {
		t.Fatalf("expected redirect to /operate/runs, got %q", loc)
	}
}

// TestOnboardingStep1_NotWritable verifies that submitting a path that exists
// but is not writable returns a plain-English error (not a raw Go error string).
func TestOnboardingStep1_NotWritable(t *testing.T) {
	h := newServerHarnessNoWorkspace(t)
	dir := t.TempDir()
	// Create .git before making the directory read-only.
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skip("cannot chmod temp dir (may be root):", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	form := url.Values{"project_path": {dir}}
	req := httptest.NewRequest(http.MethodPost, "/onboarding", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.server.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for non-writable dir, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "permission denied") {
		t.Errorf("error must not expose raw Go error string, got: %s", truncate(body, 300))
	}
	if !strings.Contains(strings.ToLower(body), "write") && !strings.Contains(strings.ToLower(body), "permission") {
		t.Errorf("expected error mentioning write access, got: %s", truncate(body, 300))
	}
}

// makeGitRepo creates a temp directory with a .git subdirectory to simulate
// a git repo (without running git init — just the directory marker is sufficient
// for the path validator).
func makeGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("makeGitRepo mkdir .git: %v", err)
	}
	return dir
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
