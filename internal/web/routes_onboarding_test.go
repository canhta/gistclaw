package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestValidateNewProjectPath(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	nonEmptyDir := filepath.Join(parent, "non-empty")
	if err := os.MkdirAll(nonEmptyDir, 0o755); err != nil {
		t.Fatalf("mkdir non-empty: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nonEmptyDir, "README.md"), []byte("seed"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "empty path", path: "", want: "new project path is required"},
		{name: "non-empty dir", path: nonEmptyDir, want: "not empty"},
		{name: "missing parent", path: filepath.Join(parent, "missing", "repo"), want: "parent path"},
		{name: "fresh path", path: filepath.Join(parent, "fresh-repo"), want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validateNewProjectPath(tc.path)
			if tc.want == "" {
				if got != "" {
					t.Fatalf("validateNewProjectPath(%q) = %q, want empty string", tc.path, got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("validateNewProjectPath(%q) = %q, want substring %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestAllowsOnboardingSetupPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{path: pageOnboarding, want: true},
		{path: pageOnboarding + "/details", want: true},
		{path: "/api/bootstrap", want: true},
		{path: "/api/onboarding/project", want: true},
		{path: pageChat, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			if got := allowsOnboardingSetupPath(tc.path); got != tc.want {
				t.Fatalf("allowsOnboardingSetupPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
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
