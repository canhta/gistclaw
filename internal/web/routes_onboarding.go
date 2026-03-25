package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/canhta/gistclaw/internal/runtime"
)

// TaskCandidate is a suggested task from the repo-signal scan.
type TaskCandidate struct {
	Kind        string // "explain", "review", or "improve"
	Description string
	Signal      string // why this candidate was suggested
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

// handleOnboarding renders step 1 of the onboarding wizard (workspace bind).
// If a workspace is already bound, redirects to /runs.
func (s *Server) handleOnboarding(w http.ResponseWriter, r *http.Request) {
	if lookupSetting(s.db, "workspace_root") != "" {
		http.Redirect(w, r, "/runs", http.StatusSeeOther)
		return
	}
	s.renderTemplate(w, "Bind Workspace", "onboarding_step1_body", nil)
}

// handleOnboardingStep1Submit validates and persists the submitted workspace path.
func (s *Server) handleOnboardingStep1Submit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	workspaceRoot := strings.TrimSpace(r.FormValue("workspace_root"))
	if workspaceRoot == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		s.renderTemplate(w, "Bind Workspace", "onboarding_step1_body", map[string]any{
			"Error": "workspace path is required",
		})
		return
	}

	if errMsg := validateWorkspacePath(workspaceRoot); errMsg != "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		s.renderTemplate(w, "Bind Workspace", "onboarding_step1_body", map[string]any{
			"Error": errMsg,
		})
		return
	}

	if _, err := s.db.RawDB().ExecContext(r.Context(),
		`INSERT INTO settings (key, value, updated_at) VALUES ('workspace_root', ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		workspaceRoot,
	); err != nil {
		http.Error(w, "failed to save workspace", http.StatusInternalServerError)
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

// handleOnboardingStep2 renders the repo-signal scan results.
func (s *Server) handleOnboardingStep2(w http.ResponseWriter, r *http.Request) {
	workspaceRoot := lookupSetting(s.db, "workspace_root")
	candidates := scanRepoSignals(workspaceRoot)
	s.renderTemplate(w, "Choose a Task", "onboarding_step2_body", map[string]any{
		"Candidates": candidates,
	})
}

// handleOnboardingStep3 renders the balanced trio for task selection.
func (s *Server) handleOnboardingStep3(w http.ResponseWriter, r *http.Request) {
	workspaceRoot := lookupSetting(s.db, "workspace_root")
	candidates := scanRepoSignals(workspaceRoot)
	s.renderTemplate(w, "Select Task", "onboarding_step3_body", map[string]any{
		"Candidates": candidates,
	})
}

// handleOnboardingStep3Submit dispatches a preview-only run for the selected task.
func (s *Server) handleOnboardingStep3Submit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	task := strings.TrimSpace(r.FormValue("task"))
	if task == "" {
		http.Error(w, "task is required", http.StatusBadRequest)
		return
	}

	workspaceRoot := lookupSetting(s.db, "workspace_root")
	run, err := s.rt.Start(r.Context(), runtime.StartRun{
		ConversationID: "onboarding",
		AgentID:        "coordinator",
		Objective:      task,
		WorkspaceRoot:  workspaceRoot,
		AccountID:      "local",
		PreviewOnly:    true,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("start run: %v", err), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/onboarding/step/4/"+run.ID, http.StatusSeeOther)
}

// handleOnboardingStep4 renders the live preview view for the onboarding run.
func (s *Server) handleOnboardingStep4(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	s.renderTemplate(w, "Preview", "onboarding_step4_body", map[string]any{
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
		// If no workspace is bound, redirect to onboarding.
		if lookupSetting(s.db, "workspace_root") == "" {
			http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}
