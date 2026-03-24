package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
agents:
  - id: coordinator
    soul_file: coordinator.soul.yaml
  - id: patcher
    soul_file: patcher.soul.yaml
capability_flags:
  coordinator: [operator_facing]
  patcher: [workspace_write]
handoff_edges:
  - from: coordinator
    to: patcher
`)
	writeFile(t, filepath.Join(dir, "coordinator.soul.yaml"), "role: coordinator\n")
	writeFile(t, filepath.Join(dir, "patcher.soul.yaml"), "role: patcher\n")
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
// is missing a required top-level field.
func TestTeamValidation_MissingRequiredField(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "team.yaml"), "name: default\nagents: []\ncapability_flags: {}\n")
	err := bootstrapWithTeamDir(t, dir)
	if err == nil {
		t.Fatal("expected error for missing handoff_edges, got nil")
	}
	if !strings.Contains(err.Error(), "handoff_edges") {
		t.Fatalf("expected error to mention 'handoff_edges', got: %v", err)
	}
}

// TestTeamValidation_SoulFileMissing verifies Bootstrap fails when a soul file
// referenced in team.yaml does not exist on disk.
func TestTeamValidation_SoulFileMissing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "team.yaml"), `
name: default
agents:
  - id: coordinator
    soul_file: coordinator.soul.yaml
capability_flags:
  coordinator: [operator_facing]
handoff_edges: []
`)
	// coordinator.soul.yaml intentionally not written
	err := bootstrapWithTeamDir(t, dir)
	if err == nil {
		t.Fatal("expected error when soul file is missing, got nil")
	}
	if !strings.Contains(err.Error(), "coordinator.soul.yaml") {
		t.Fatalf("expected error to mention missing soul file, got: %v", err)
	}
}

// TestTeamValidation_UnknownCapabilityFlag verifies Bootstrap fails when a
// capability flag is not in the allowed set.
func TestTeamValidation_UnknownCapabilityFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "team.yaml"), `
name: default
agents:
  - id: coordinator
    soul_file: coordinator.soul.yaml
capability_flags:
  coordinator: [super_admin]
handoff_edges: []
`)
	writeFile(t, filepath.Join(dir, "coordinator.soul.yaml"), "role: coordinator\n")
	err := bootstrapWithTeamDir(t, dir)
	if err == nil {
		t.Fatal("expected error for unknown capability flag, got nil")
	}
	if !strings.Contains(err.Error(), "super_admin") {
		t.Fatalf("expected error to mention 'super_admin', got: %v", err)
	}
}

// TestTeamValidation_EmptyTeamDir verifies Bootstrap skips validation when
// TeamDir is not configured (backward-compatible default).
func TestTeamValidation_EmptyTeamDir(t *testing.T) {
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
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
