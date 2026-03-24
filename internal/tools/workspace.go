package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

var ErrEscapeAttempt = fmt.Errorf("tools: path escapes workspace root")
var ErrNoApproval = fmt.Errorf("tools: workspace apply requires an approved ticket")

type WorkspaceApplier struct {
	workspaceRoot string
	db            *store.DB // optional; enables fingerprint validation when set
}

func NewWorkspaceApplier(workspaceRoot string) *WorkspaceApplier {
	return &WorkspaceApplier{workspaceRoot: workspaceRoot}
}

// NewWorkspaceApplierWithDB returns a WorkspaceApplier that also validates
// the approval ticket fingerprint against the database before applying.
func NewWorkspaceApplierWithDB(workspaceRoot string, db *store.DB) *WorkspaceApplier {
	return &WorkspaceApplier{workspaceRoot: workspaceRoot, db: db}
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

// Apply validates the approval ticket before writing any files. It requires:
//   - ticket.ID is non-empty and its status is "approved"
//   - if the applier was constructed with a DB, the stored fingerprint must
//     match the fingerprint computed from ticket.ToolName, ticket.ArgsJSON,
//     and each change's target path (single-use: the ticket is consumed here)
func (a *WorkspaceApplier) Apply(ctx context.Context, runID string, ticket model.ApprovalTicket, changes []model.FileChange) (model.ApplyResult, error) {
	if ticket.ID == "" || ticket.Status != "approved" {
		return model.ApplyResult{}, ErrNoApproval
	}

	// If the applier has a DB reference, verify the stored fingerprint.
	if a.db != nil {
		targetPath := ""
		if len(changes) > 0 {
			targetPath = changes[0].Path
		}
		expectedFP := computeFingerprint(ticket.ToolName, ticket.ArgsJSON, targetPath)
		if err := VerifyTicket(ctx, a.db, ticket.ID, expectedFP); err != nil {
			return model.ApplyResult{}, fmt.Errorf("tools: fingerprint verification: %w", err)
		}
	}

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
