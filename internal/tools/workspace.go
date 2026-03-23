package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
)

var ErrEscapeAttempt = fmt.Errorf("tools: path escapes workspace root")

type WorkspaceApplier struct {
	workspaceRoot string
}

func NewWorkspaceApplier(workspaceRoot string) *WorkspaceApplier {
	return &WorkspaceApplier{workspaceRoot: workspaceRoot}
}

func (a *WorkspaceApplier) Preview(_ context.Context, runID string, changes []model.FileChange) (model.ChangePreview, error) {
	for _, change := range changes {
		if err := a.validatePath(change.Path); err != nil {
			return model.ChangePreview{}, err
		}
	}

	return model.ChangePreview{
		RunID:   runID,
		Changes: changes,
	}, nil
}

func (a *WorkspaceApplier) Apply(_ context.Context, _ string, _ model.ApprovalTicket, changes []model.FileChange) (model.ApplyResult, error) {
	for _, change := range changes {
		if err := a.validatePath(change.Path); err != nil {
			return model.ApplyResult{}, err
		}
	}
	return model.ApplyResult{Applied: true}, nil
}

func (a *WorkspaceApplier) validatePath(relPath string) error {
	if strings.ContainsRune(relPath, 0) {
		return ErrEscapeAttempt
	}

	joined := filepath.Join(a.workspaceRoot, relPath)
	cleaned := filepath.Clean(joined)
	root := filepath.Clean(a.workspaceRoot)

	if cleaned != root && !strings.HasPrefix(cleaned, root+string(filepath.Separator)) {
		return ErrEscapeAttempt
	}

	return nil
}
