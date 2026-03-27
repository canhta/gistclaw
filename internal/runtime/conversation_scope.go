package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
)

func (r *Runtime) scopeConversationKey(
	ctx context.Context,
	key conversations.ConversationKey,
	projectID string,
	cwd string,
) (conversations.ConversationKey, model.Project, error) {
	if key.ProjectID != "" {
		project, err := loadProjectByID(ctx, r.store, key.ProjectID)
		if err == nil {
			return key, project, nil
		}
	}

	projectID = strings.TrimSpace(projectID)
	if projectID != "" {
		project, err := loadProjectByID(ctx, r.store, projectID)
		if err != nil {
			return conversations.ConversationKey{}, model.Project{}, fmt.Errorf("scope conversation key: load project: %w", err)
		}
		key.ProjectID = project.ID
		return key, project, nil
	}

	cwd = normalizePrimaryPath(cwd)
	if cwd != "" {
		project, err := RegisterProjectPath(ctx, r.store, cwd, "", "runtime")
		if err != nil {
			return conversations.ConversationKey{}, model.Project{}, fmt.Errorf("scope conversation key: register cwd: %w", err)
		}
		key.ProjectID = project.ID
		return key, project, nil
	}

	project, err := ActiveProject(ctx, r.store)
	if err != nil {
		return conversations.ConversationKey{}, model.Project{}, fmt.Errorf("scope conversation key: active project: %w", err)
	}
	if project.ID == "" {
		return key, model.Project{}, nil
	}
	key.ProjectID = project.ID
	return key, project, nil
}
