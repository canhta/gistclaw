package runtime

import (
	"context"
	"fmt"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
)

func (r *Runtime) scopeConversationKey(ctx context.Context, key conversations.ConversationKey, workspaceRoot string) (conversations.ConversationKey, model.Project, error) {
	if key.ProjectID != "" {
		project, err := loadProjectByID(ctx, r.store, key.ProjectID)
		if err == nil {
			return key, project, nil
		}
	}

	workspaceRoot = normalizeWorkspaceRoot(workspaceRoot)
	if workspaceRoot != "" {
		project, err := RegisterProject(ctx, r.store, workspaceRoot, "", "runtime")
		if err != nil {
			return conversations.ConversationKey{}, model.Project{}, fmt.Errorf("scope conversation key: register workspace: %w", err)
		}
		key.ProjectID = project.ID
		return key, project, nil
	}

	project, err := ActiveProject(ctx, r.store)
	if err != nil {
		return conversations.ConversationKey{}, model.Project{}, fmt.Errorf("scope conversation key: active project: %w", err)
	}
	if project.WorkspaceRoot == "" {
		return key, model.Project{}, nil
	}
	if project.ID == "" {
		project, err = RegisterProject(ctx, r.store, project.WorkspaceRoot, project.Name, "runtime")
		if err != nil {
			return conversations.ConversationKey{}, model.Project{}, fmt.Errorf("scope conversation key: register active project: %w", err)
		}
	}
	key.ProjectID = project.ID
	return key, project, nil
}
