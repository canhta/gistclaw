package projectscope

import (
	"github.com/canhta/gistclaw/internal/model"
)

func RunCondition(project model.Project, alias string) (string, []any) {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}

	switch {
	case project.ID != "":
		return prefix + "project_id = ?", []any{project.ID}
	default:
		return "1 = 1", nil
	}
}
