package projectscope

import (
	"fmt"

	"github.com/canhta/gistclaw/internal/model"
)

func RunCondition(project model.Project, alias string) (string, []any) {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}

	switch {
	case project.ID != "" && project.WorkspaceRoot != "":
		return fmt.Sprintf("(%sproject_id = ? OR ((%sproject_id IS NULL OR %sproject_id = '') AND COALESCE(%sworkspace_root, '') = ?))",
			prefix, prefix, prefix, prefix,
		), []any{project.ID, project.WorkspaceRoot}
	case project.ID != "":
		return prefix + "project_id = ?", []any{project.ID}
	case project.WorkspaceRoot != "":
		return "COALESCE(" + prefix + "workspace_root, '') = ?", []any{project.WorkspaceRoot}
	default:
		return "1 = 1", nil
	}
}
