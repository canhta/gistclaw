package web

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
)

// TaskCandidate is a suggested task from the repo-signal scan.
type TaskCandidate struct {
	Kind        string // "explain", "review", or "improve"
	Description string
	Signal      string // why this candidate was suggested
}

type onboardingStep1Data struct {
	Error               string
	ActiveProjectName   string
	ActiveWorkspaceRoot string
}

type onboardingStep3Data struct {
	Candidates   []TaskCandidate
	Error        string
	SelectedTask string
}

// scanRepoSignals reads the local filesystem of workspaceRoot and produces
// a heuristic list of task candidates. It makes no model calls.
func scanRepoSignals(workspaceRoot string) []TaskCandidate {
	var candidates []TaskCandidate

	entries, err := os.ReadDir(workspaceRoot)
	if err != nil {
		return fallbackTrio(workspaceRoot)
	}

	// Heuristic 1 (explain): directories named after known subsystems.
	knownSubsystems := map[string]bool{
		"internal": true, "pkg": true, "cmd": true, "api": true,
		"service": true, "handler": true, "lib": true, "core": true,
		"server": true, "client": true, "db": true, "store": true,
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if knownSubsystems[e.Name()] {
			candidates = append(candidates, TaskCandidate{
				Kind:        "explain",
				Description: fmt.Sprintf("Explain what the %s package does", e.Name()),
				Signal:      fmt.Sprintf("directory %q matches known subsystem name", e.Name()),
			})
			break
		}
	}

	// Heuristic 2 (review): files with uncommitted changes (approximate: any
	// non-hidden regular file in root with a known code extension).
	codeExtensions := map[string]bool{
		".go": true, ".ts": true, ".js": true, ".py": true,
		".rs": true, ".java": true, ".rb": true, ".c": true, ".cpp": true,
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if codeExtensions[filepath.Ext(e.Name())] {
			candidates = append(candidates, TaskCandidate{
				Kind:        "review",
				Description: fmt.Sprintf("Review changes in %s", e.Name()),
				Signal:      fmt.Sprintf("code file %q found in workspace root", e.Name()),
			})
			break
		}
	}

	// Heuristic 3 (improve): most recently touched file without a nearby test.
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		ext := filepath.Ext(e.Name())
		if !codeExtensions[ext] {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ext)
		if strings.HasSuffix(base, "_test") || strings.HasSuffix(base, ".test") {
			continue
		}
		testName := base + "_test" + ext
		if _, err := os.Stat(filepath.Join(workspaceRoot, testName)); os.IsNotExist(err) {
			candidates = append(candidates, TaskCandidate{
				Kind:        "improve",
				Description: fmt.Sprintf("Find the safest next improvement in %s", e.Name()),
				Signal:      fmt.Sprintf("%q has no adjacent test file", e.Name()),
			})
			break
		}
	}

	// Fill in missing types with generic fallbacks.
	types := map[string]bool{}
	for _, c := range candidates {
		types[c.Kind] = true
	}
	for _, fb := range fallbackTrio(workspaceRoot) {
		if !types[fb.Kind] {
			candidates = append(candidates, fb)
			types[fb.Kind] = true
		}
	}

	return candidates
}

func fallbackTrio(workspaceRoot string) []TaskCandidate {
	dir := filepath.Base(workspaceRoot)
	return []TaskCandidate{
		{
			Kind:        "explain",
			Description: fmt.Sprintf("Explain what %s does", dir),
			Signal:      "generic fallback: no subsystem directories detected",
		},
		{
			Kind:        "review",
			Description: "Review uncommitted changes in the working tree",
			Signal:      "generic fallback: no changed files detected",
		},
		{
			Kind:        "improve",
			Description: "Find the safest next improvement in this codebase",
			Signal:      "generic fallback: no recently modified files detected",
		},
	}
}

// handleOnboarding renders step 1 of the onboarding wizard.
// Once onboarding is complete, it redirects to the main Operate queue.
func (s *Server) handleOnboarding(w http.ResponseWriter, r *http.Request) {
	if onboardingCompleted(s.db) {
		http.Redirect(w, r, pageOperateRuns, http.StatusSeeOther)
		return
	}
	s.renderOnboardingStep1(w, r, http.StatusOK, "")
}

// handleOnboardingStep1Submit validates and persists the submitted workspace path.
func (s *Server) handleOnboardingStep1Submit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	action := strings.TrimSpace(r.FormValue("action"))
	switch action {
	case "use_starter":
		project, err := runtime.ActiveProject(r.Context(), s.db)
		if err != nil {
			http.Error(w, "failed to load starter project", http.StatusInternalServerError)
			return
		}
		if project.PrimaryPath == "" {
			s.renderOnboardingStep1(w, r, http.StatusUnprocessableEntity, "starter project is not ready yet")
			return
		}
	case "create_new":
		workspaceRoot := strings.TrimSpace(r.FormValue("new_workspace_root"))
		if workspaceRoot == "" {
			s.renderOnboardingStep1(w, r, http.StatusUnprocessableEntity, "new project path is required")
			return
		}
		if errMsg := validateNewWorkspacePath(workspaceRoot); errMsg != "" {
			s.renderOnboardingStep1(w, r, http.StatusUnprocessableEntity, errMsg)
			return
		}
		if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
			http.Error(w, "failed to create project directory", http.StatusInternalServerError)
			return
		}
		if _, err := runtime.ActivateProjectPath(r.Context(), s.db, workspaceRoot, "", "operator"); err != nil {
			http.Error(w, "failed to save project", http.StatusInternalServerError)
			return
		}
	default:
		workspaceRoot := strings.TrimSpace(r.FormValue("workspace_root"))
		if workspaceRoot == "" {
			s.renderOnboardingStep1(w, r, http.StatusUnprocessableEntity, "workspace path is required")
			return
		}

		if errMsg := validateWorkspacePath(workspaceRoot); errMsg != "" {
			s.renderOnboardingStep1(w, r, http.StatusUnprocessableEntity, errMsg)
			return
		}

		if _, err := runtime.ActivateProjectPath(r.Context(), s.db, workspaceRoot, "", "operator"); err != nil {
			http.Error(w, "failed to save workspace", http.StatusInternalServerError)
			return
		}
	}

	if err := markOnboardingCompleted(r.Context(), s.db); err != nil {
		http.Error(w, "failed to save onboarding state", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/onboarding/step/2", http.StatusSeeOther)
}

// validateWorkspacePath checks that path exists, is a git repo, and is writable.
// Returns an empty string on success, or a human-readable error message.
func validateWorkspacePath(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("path %q does not exist", path)
		}
		return fmt.Sprintf("cannot access path: %v", err)
	}
	if !info.IsDir() {
		return fmt.Sprintf("%q is not a directory", path)
	}
	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return fmt.Sprintf("%q is not a git repository (no .git directory found)", path)
	}
	tmp, err := os.CreateTemp(path, ".gistclaw-write-check-*")
	if err != nil {
		return fmt.Sprintf("%q does not have write permission for this process", path)
	}
	tmp.Close()
	os.Remove(tmp.Name())
	return ""
}

func validateNewWorkspacePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "new project path is required"
	}
	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			return fmt.Sprintf("%q is not a directory", path)
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return fmt.Sprintf("cannot inspect %q", path)
		}
		if len(entries) > 0 {
			return fmt.Sprintf("%q is not empty; bind it as an existing repo instead", path)
		}
		return ""
	} else if !os.IsNotExist(err) {
		return fmt.Sprintf("cannot access path: %v", err)
	}

	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("parent path %q does not exist", parent)
		}
		return fmt.Sprintf("cannot access parent path: %v", err)
	}
	if !info.IsDir() {
		return fmt.Sprintf("parent path %q is not a directory", parent)
	}
	return ""
}

// handleOnboardingStep2 renders the repo-signal scan results.
func (s *Server) handleOnboardingStep2(w http.ResponseWriter, r *http.Request) {
	project, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load active project", http.StatusInternalServerError)
		return
	}
	candidates := scanRepoSignals(project.PrimaryPath)
	s.renderTemplate(w, r, "Choose a Task", "onboarding_step2_body", map[string]any{
		"Candidates": candidates,
	})
}

// handleOnboardingStep3 renders the balanced trio for task selection.
func (s *Server) handleOnboardingStep3(w http.ResponseWriter, r *http.Request) {
	s.renderOnboardingStep3(w, r, http.StatusOK, "", "")
}

// handleOnboardingStep3Submit dispatches a preview-only run for the selected task.
func (s *Server) handleOnboardingStep3Submit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	task := strings.TrimSpace(r.FormValue("task"))
	if task == "" {
		s.renderOnboardingStep3(w, r, http.StatusUnprocessableEntity, "Choose a preview task before starting the run.", "")
		return
	}
	if s.rt == nil {
		s.renderOnboardingStep3(w, r, http.StatusServiceUnavailable, "Preview runs are unavailable right now. Check the runtime configuration and try again.", task)
		return
	}

	project, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		s.renderOnboardingStep3(w, r, http.StatusServiceUnavailable, "Unable to load the active project. Check the runtime configuration and try again.", task)
		return
	}
	run, err := s.rt.StartAsync(r.Context(), runtime.StartRun{
		ConversationID: "onboarding",
		AgentID:        "coordinator",
		Objective:      task,
		ProjectID:      project.ID,
		CWD:            project.PrimaryPath,
		AccountID:      "local",
		PreviewOnly:    true,
	})
	if err != nil {
		s.renderOnboardingStep3(w, r, http.StatusServiceUnavailable, onboardingPreviewStartError(err), task)
		return
	}
	http.Redirect(w, r, "/onboarding/step/4/"+run.ID, http.StatusSeeOther)
}

// handleOnboardingStep4 renders the live preview view for the onboarding run.
func (s *Server) handleOnboardingStep4(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	s.renderTemplate(w, r, "Preview", "onboarding_step4_body", map[string]any{
		"RunID": runID,
	})
}

// onboardingMiddleware returns a handler that redirects to /onboarding when
// no workspace is bound, except for /onboarding paths themselves.
func (s *Server) onboardingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Allow onboarding paths and static assets through unconditionally.
		if strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/onboarding") || strings.HasPrefix(path, "/webhooks/") {
			next.ServeHTTP(w, r)
			return
		}
		if !onboardingCompleted(s.db) {
			http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) renderOnboardingStep1(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	project, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load onboarding state", http.StatusInternalServerError)
		return
	}
	s.renderTemplateStatus(w, r, status, "Choose Project", "onboarding_step1_body", onboardingStep1Data{
		Error:               errMsg,
		ActiveProjectName:   project.Name,
		ActiveWorkspaceRoot: project.PrimaryPath,
	})
}

func (s *Server) renderOnboardingStep3(w http.ResponseWriter, r *http.Request, status int, errMsg, selectedTask string) {
	project, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load active project", http.StatusInternalServerError)
		return
	}
	candidates := scanRepoSignals(project.PrimaryPath)
	s.renderTemplateStatus(w, r, status, "Select Task", "onboarding_step3_body", onboardingStep3Data{
		Candidates:   candidates,
		Error:        errMsg,
		SelectedTask: selectedTask,
	})
}

func onboardingPreviewStartError(err error) string {
	switch {
	case err == nil:
		return ""
	case err == runtime.ErrDailyCap:
		return "Preview runs are blocked by the daily cost cap. Raise the cap and try again."
	case err == runtime.ErrBudgetExhausted:
		return "Preview runs are blocked by the per-run token budget. Raise the budget and try again."
	default:
		return "Unable to start the preview run right now. Check the runtime configuration and try again."
	}
}

func onboardingCompleted(db *store.DB) bool {
	return lookupSetting(db, "onboarding_completed_at") != ""
}

func markOnboardingCompleted(ctx context.Context, db *store.DB) error {
	_, err := db.RawDB().ExecContext(ctx,
		`INSERT INTO settings (key, value, updated_at) VALUES ('onboarding_completed_at', datetime('now'), datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
	)
	if err != nil {
		return fmt.Errorf("save onboarding_completed_at: %w", err)
	}
	return nil
}
