package app

import (
	"fmt"

	"github.com/canhta/gistclaw/internal/teams"
)

// validateTeamDir parses and validates teams/default/team.yaml and all soul
// files it references. It returns a descriptive error on any schema violation
// or missing file. If teamDir is empty the function is a no-op.
func validateTeamDir(teamDir string) error {
	if teamDir == "" {
		return nil
	}
	if _, err := teams.LoadExecutionSnapshot(teamDir); err != nil {
		return fmt.Errorf("team validation: %w", err)
	}
	return nil
}
