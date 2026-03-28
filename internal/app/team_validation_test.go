package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	runtimepkg "github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/teams"
)

// writeFile writes content to path, creating parent dirs as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func validTeamDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "team.yaml"), `
name: default
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    base_profile: operator
    tool_families: [repo_read, runtime_capability, connector_capability, delegate]
    delegation_kinds: [write]
    can_message: [patcher]
    specialist_summary_visibility: full
  - id: patcher
    soul_file: patcher.soul.yaml
    base_profile: write
    tool_families: [repo_read, repo_write]
    can_message: [assistant]
    specialist_summary_visibility: basic
`)
	writeFile(t, filepath.Join(dir, "assistant.soul.yaml"), "role: front assistant\n")
	writeFile(t, filepath.Join(dir, "patcher.soul.yaml"), "role: scoped write specialist\n")
	return dir
}

func bootstrapWithTeamDir(t *testing.T, teamDir string) error {
	t.Helper()
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		TeamDir:      teamDir,
	}
	a, err := Bootstrap(cfg)
	if err == nil && a != nil {
		_ = a.db.Close()
	}
	return err
}

func storageRootWithDefaultTeam(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	teamDir := filepath.Join(root, "teams", "default")
	writeFile(t, filepath.Join(teamDir, "team.yaml"), `
name: default
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    base_profile: operator
    tool_families: [repo_read, runtime_capability, connector_capability, delegate]
    delegation_kinds: [write]
    can_message: [patcher]
    specialist_summary_visibility: full
  - id: patcher
    soul_file: patcher.soul.yaml
    base_profile: write
    tool_families: [repo_read, repo_write]
    can_message: [assistant]
    specialist_summary_visibility: basic
`)
	writeFile(t, filepath.Join(teamDir, "assistant.soul.yaml"), "role: front assistant\n")
	writeFile(t, filepath.Join(teamDir, "patcher.soul.yaml"), "role: scoped write specialist\n")
	return root
}

// TestTeamValidation_ValidTeam verifies Bootstrap succeeds with a valid team dir.
func TestTeamValidation_ValidTeam(t *testing.T) {
	dir := validTeamDir(t)
	if err := bootstrapWithTeamDir(t, dir); err != nil {
		t.Fatalf("expected no error for valid team, got: %v", err)
	}
}

// TestTeamValidation_MissingTeamYAML verifies Bootstrap fails when team.yaml does not exist.
func TestTeamValidation_MissingTeamYAML(t *testing.T) {
	dir := t.TempDir() // empty — no team.yaml
	err := bootstrapWithTeamDir(t, dir)
	if err == nil {
		t.Fatal("expected error when team.yaml is missing, got nil")
	}
	if !strings.Contains(err.Error(), "team.yaml") {
		t.Fatalf("expected error to mention team.yaml, got: %v", err)
	}
}

// TestTeamValidation_MissingRequiredField verifies Bootstrap fails when team.yaml
// is missing front_agent.
func TestTeamValidation_MissingRequiredField(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "team.yaml"), "name: default\nagents: []\n")
	err := bootstrapWithTeamDir(t, dir)
	if err == nil {
		t.Fatal("expected error for missing front_agent, got nil")
	}
	if !strings.Contains(err.Error(), "front_agent") {
		t.Fatalf("expected error to mention 'front_agent', got: %v", err)
	}
}

// TestTeamValidation_SoulFileMissing verifies Bootstrap fails when a soul file
// referenced in team.yaml does not exist on disk.
func TestTeamValidation_SoulFileMissing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "team.yaml"), `
name: default
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    base_profile: operator
    tool_families: [repo_read, delegate]
    can_message: []
`)
	// assistant.soul.yaml intentionally not written
	err := bootstrapWithTeamDir(t, dir)
	if err == nil {
		t.Fatal("expected error when soul file is missing, got nil")
	}
	if !strings.Contains(err.Error(), "assistant.soul.yaml") {
		t.Fatalf("expected error to mention missing soul file, got: %v", err)
	}
}

// TestTeamValidation_UnknownMessageTarget verifies Bootstrap fails when an
// agent references a message target that is not declared in the team.
func TestTeamValidation_UnknownMessageTarget(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "team.yaml"), `
name: default
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    base_profile: operator
    tool_families: [repo_read, delegate]
    can_message: [ghost]
`)
	writeFile(t, filepath.Join(dir, "assistant.soul.yaml"), "role: front assistant\n")
	err := bootstrapWithTeamDir(t, dir)
	if err == nil {
		t.Fatal("expected error for unknown message target, got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("expected error to mention 'ghost', got: %v", err)
	}
}

// TestTeamValidation_EmptyTeamDir verifies Bootstrap seeds the default team when
// TeamDir is not configured.
func TestTeamValidation_EmptyTeamDir(t *testing.T) {
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  t.TempDir(),
		TeamDir:      "", // not configured
	}
	a, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("expected no error when TeamDir is empty, got: %v", err)
	}
	if a != nil {
		_ = a.db.Close()
	}
}

func TestResolveTeamDir_UsesStorageDefaultWhenPresent(t *testing.T) {
	storageRoot := storageRootWithDefaultTeam(t)
	cfg := Config{StorageRoot: storageRoot}

	got := resolveTeamDir(cfg)
	want := filepath.Join(storageRoot, "teams", "default")
	if got != want {
		t.Fatalf("expected resolved team dir %q, got %q", want, got)
	}
}

func TestBootstrap_UsesStorageOwnedTeamDirWhenPresent(t *testing.T) {
	storageRoot := storageRootWithDefaultTeam(t)
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  storageRoot,
	}

	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = app.db.Close() })

	storageTeamDir := filepath.Join(storageRoot, "teams", "default")
	for _, name := range []string{"team.yaml", "assistant.soul.yaml", "patcher.soul.yaml"} {
		if _, err := os.Stat(filepath.Join(storageTeamDir, name)); err != nil {
			t.Fatalf("expected storage-owned team file %q to exist: %v", name, err)
		}
	}

	sourceCfg, err := teams.LoadConfig(storageTeamDir)
	if err != nil {
		t.Fatalf("load storage team: %v", err)
	}
	runtimeCfg, err := app.runtime.TeamConfig(context.Background())
	if err != nil {
		t.Fatalf("load runtime team: %v", err)
	}
	if runtimeCfg.Name != sourceCfg.Name || runtimeCfg.FrontAgent != sourceCfg.FrontAgent {
		t.Fatalf("expected storage-owned team to match runtime config, got %+v want %+v", runtimeCfg, sourceCfg)
	}
}

func TestBootstrap_SeedsStorageOwnedTeamDirFromShippedDefaultWhenStorageIsEmpty(t *testing.T) {
	storageRoot := t.TempDir()
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  storageRoot,
	}

	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = app.db.Close() })

	storageTeamDir := filepath.Join(storageRoot, "teams", "default")
	for _, name := range []string{"team.yaml", "assistant.soul.yaml", "patcher.soul.yaml"} {
		if _, err := os.Stat(filepath.Join(storageTeamDir, name)); err != nil {
			t.Fatalf("expected storage-owned team file %q to exist: %v", name, err)
		}
	}

	runtimeCfg, err := app.runtime.TeamConfig(context.Background())
	if err != nil {
		t.Fatalf("load runtime team: %v", err)
	}
	if runtimeCfg.Name == "" {
		t.Fatal("expected empty storage root to receive shipped default team")
	}
}

func TestBootstrap_UsesStorageOwnedTeamDirForEdits(t *testing.T) {
	storageRoot := storageRootWithDefaultTeam(t)
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  storageRoot,
	}

	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = app.db.Close() })

	runtimeCfg, err := app.runtime.TeamConfig(context.Background())
	if err != nil {
		t.Fatalf("load runtime team: %v", err)
	}
	runtimeCfg.Name = "Workspace Operators"
	runtimeCfg.Agents[0].Role = "workspace owner"

	if err := app.runtime.UpdateTeam(context.Background(), runtimeCfg); err != nil {
		t.Fatalf("update runtime team: %v", err)
	}

	project, err := runtimepkg.ActiveProject(context.Background(), app.db)
	if err != nil {
		t.Fatalf("load active project: %v", err)
	}
	storageCfg, err := teams.LoadConfig(filepath.Join(storageRoot, "projects", project.ID, "teams", "default"))
	if err != nil {
		t.Fatalf("reload storage team: %v", err)
	}
	if storageCfg.Name != "Workspace Operators" {
		t.Fatalf("expected storage-owned team to receive edit, got %q", storageCfg.Name)
	}
	if storageCfg.Agents[0].Role != "workspace owner" {
		t.Fatalf("expected storage-owned soul edit, got %q", storageCfg.Agents[0].Role)
	}
}

func TestTeamValidation_DefaultTeamDirIncludesResearcher(t *testing.T) {
	defaultTeamDir := filepath.Join(findModuleRootForAppTest(t), "teams", "default")
	if err := validateTeamDir(defaultTeamDir); err != nil {
		t.Fatalf("expected default team dir to validate, got %v", err)
	}
}

func TestTeamValidation_ResearcherSoulFileMissing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "team.yaml"), `
name: default
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    base_profile: operator
    tool_families: [repo_read, delegate]
    delegation_kinds: [research]
    can_message: [researcher]
  - id: researcher
    soul_file: researcher.soul.yaml
    base_profile: research
    tool_families: [repo_read, web_read]
    can_message: [assistant]
`)
	writeFile(t, filepath.Join(dir, "assistant.soul.yaml"), "role: front assistant\n")

	err := bootstrapWithTeamDir(t, dir)
	if err == nil || !strings.Contains(err.Error(), "researcher.soul.yaml") {
		t.Fatalf("expected missing researcher soul error, got %v", err)
	}
}

func TestTeamValidation_RejectsUnknownBaseProfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "team.yaml"), `
name: default
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    base_profile: unsafe_mode
    tool_families: [repo_read]
    can_message: []
`)
	writeFile(t, filepath.Join(dir, "assistant.soul.yaml"), "role: front assistant\n")

	err := bootstrapWithTeamDir(t, dir)
	if err == nil || !strings.Contains(err.Error(), "unsafe_mode") {
		t.Fatalf("expected unknown base profile error, got %v", err)
	}
}

func findModuleRootForAppTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine caller path")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
