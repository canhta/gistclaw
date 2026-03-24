package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/canhta/gistclaw/internal/runtime"
)

// allowedCapabilityFlags is the exhaustive set of capability flags that may
// appear in a team.yaml file. Any value outside this set is a schema error.
var allowedCapabilityFlags = map[string]bool{
	"operator_facing": true,
	"workspace_write": true,
	"read_heavy":      true,
	"propose_only":    true,
}

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

	spec, err := runtime.LoadTeamSpec(data)
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

	// Validate all capability flags are from the allowed set.
	for agentID, flags := range spec.CapabilityFlags {
		for _, flag := range flags {
			if !allowedCapabilityFlags[flag] {
				return fmt.Errorf("team validation: agent %q has unknown capability flag %q", agentID, flag)
			}
		}
	}

	return nil
}
