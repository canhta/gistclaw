package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/canhta/gistclaw/internal/teams"
)

// validateTeamDir parses and validates teams/default/team.yaml and all soul
// files it references. It returns a descriptive error on any schema violation
// or missing file. If teamDir is empty the function is a no-op.
func validateTeamDir(teamDir string) error {
	if teamDir == "" {
		return nil
	}

	teamYAMLPath := filepath.Join(teamDir, "team.yaml")
	data, err := os.ReadFile(teamYAMLPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("team validation: team.yaml not found at %s", teamYAMLPath)
		}
		return fmt.Errorf("team validation: read team.yaml: %w", err)
	}

	spec, err := teams.LoadSpec(data)
	if err != nil {
		return fmt.Errorf("team validation: %w", err)
	}

	// Verify every referenced soul file exists on disk.
	for _, agent := range spec.Agents {
		soulPath := filepath.Join(teamDir, agent.SoulFile)
		if _, err := os.Stat(soulPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("team validation: agent %q soul file %q not found", agent.ID, agent.SoulFile)
			}
			return fmt.Errorf("team validation: stat soul file %q: %w", agent.SoulFile, err)
		}
	}

	return nil
}
