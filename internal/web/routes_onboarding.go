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

// scanRepoSignals reads the local filesystem of the candidate project path and produces
// a heuristic list of task candidates. It makes no model calls.
func scanRepoSignals(projectPath string) []TaskCandidate {
	var candidates []TaskCandidate

	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return fallbackTrio(projectPath)
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
				Signal:      fmt.Sprintf("code file %q found in project root", e.Name()),
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
		if _, err := os.Stat(filepath.Join(projectPath, testName)); os.IsNotExist(err) {
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
	for _, fb := range fallbackTrio(projectPath) {
		if !types[fb.Kind] {
			candidates = append(candidates, fb)
			types[fb.Kind] = true
		}
	}

	return candidates
}

func fallbackTrio(projectPath string) []TaskCandidate {
	dir := filepath.Base(projectPath)
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

// validateProjectPath checks that path exists, is a git repo, and is writable.
// Returns an empty string on success, or a human-readable error message.
func validateProjectPath(path string) string {
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

func validateNewProjectPath(path string) string {
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
// onboardingMiddleware returns a handler that redirects to /onboarding when
// no project is bound, except for /onboarding paths themselves.
func (s *Server) onboardingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Public auth, onboarding, assets, and webhook paths must bypass onboarding
		// gating or unauthenticated browsers can get stuck in a redirect loop.
		if isPublicPath(path) || allowsOnboardingSetupPath(path) {
			next.ServeHTTP(w, r)
			return
		}
		if !onboardingCompleted(s.db) {
			http.Redirect(w, r, pageOnboarding, http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func onboardingPreviewStartError(err error) string {
	switch err {
	case nil:
		return ""
	case runtime.ErrDailyCap:
		return "Preview runs are blocked by the daily cost cap. Raise the cap and try again."
	case runtime.ErrBudgetExhausted:
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

func ensureNewProjectPath(projectPath string) error {
	return os.MkdirAll(projectPath, 0o755)
}

func allowsOnboardingSetupPath(path string) bool {
	switch {
	case path == pageOnboarding, strings.HasPrefix(path, pageOnboarding+"/"):
		return true
	case path == "/api/bootstrap":
		return true
	case path == "/api/onboarding", strings.HasPrefix(path, "/api/onboarding/"):
		return true
	default:
		return false
	}
}

func (s *Server) defaultEntryPath() string {
	if !onboardingCompleted(s.db) {
		return pageOnboarding
	}
	return pageWork
}
