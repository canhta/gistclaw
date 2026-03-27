package teams

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfiles_CreateProfileSeedsShippedDefault(t *testing.T) {
	profilesRoot := t.TempDir()

	if err := CreateProfile(profilesRoot, "review"); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	cfg, err := LoadConfig(filepath.Join(profilesRoot, "review"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Name != "default" {
		t.Fatalf("expected shipped default team name %q, got %q", "default", cfg.Name)
	}
}

func TestProfiles_CloneProfileCopiesSourceFiles(t *testing.T) {
	profilesRoot := t.TempDir()
	sourceDir := filepath.Join(profilesRoot, "review")
	writeTeamFile(t, filepath.Join(sourceDir, "team.yaml"), `
name: Review Team
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    can_spawn: []
    can_message: []
`)
	writeTeamFile(t, filepath.Join(sourceDir, "assistant.soul.yaml"), "role: reviewer\ntool_posture: read_heavy\n")

	if err := CloneProfile(profilesRoot, "review", "ops"); err != nil {
		t.Fatalf("CloneProfile: %v", err)
	}

	cfg, err := LoadConfig(filepath.Join(profilesRoot, "ops"))
	if err != nil {
		t.Fatalf("LoadConfig clone: %v", err)
	}
	if cfg.Name != "Review Team" {
		t.Fatalf("expected cloned team name %q, got %q", "Review Team", cfg.Name)
	}
}

func TestProfiles_ListProfilesReturnsSortedNames(t *testing.T) {
	profilesRoot := t.TempDir()
	for _, profile := range []string{"review", "default", "ops"} {
		profileDir := filepath.Join(profilesRoot, profile)
		writeTeamFile(t, filepath.Join(profileDir, "team.yaml"), `
name: `+profile+`
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    can_spawn: []
    can_message: []
`)
		writeTeamFile(t, filepath.Join(profileDir, "assistant.soul.yaml"), "role: coordinator\ntool_posture: read_heavy\n")
	}

	profiles, err := ListProfiles(profilesRoot)
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(profiles))
	}
	want := []string{"default", "ops", "review"}
	for i, profile := range profiles {
		if profile.Name != want[i] {
			t.Fatalf("expected profile %d to be %q, got %q", i, want[i], profile.Name)
		}
	}
}

func TestProfiles_DeleteProfileRemovesDirectory(t *testing.T) {
	profilesRoot := t.TempDir()
	profileDir := filepath.Join(profilesRoot, "review")
	writeTeamFile(t, filepath.Join(profileDir, "team.yaml"), `
name: Review Team
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    can_spawn: []
    can_message: []
`)
	writeTeamFile(t, filepath.Join(profileDir, "assistant.soul.yaml"), "role: reviewer\ntool_posture: read_heavy\n")

	if err := DeleteProfile(profilesRoot, "review"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	if _, err := os.Stat(profileDir); !os.IsNotExist(err) {
		t.Fatalf("expected deleted profile dir to be removed, err=%v", err)
	}
}
