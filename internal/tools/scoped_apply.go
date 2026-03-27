package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

var ErrEscapeAttempt = fmt.Errorf("tools: path escapes allowed root")
var ErrNoApproval = fmt.Errorf("tools: scoped apply requires an approved ticket")

type ScopedApplier struct {
	writeRoot string
	db        *store.DB // optional; enables fingerprint validation when set
}

func NewScopedApplier(writeRoot string) *ScopedApplier {
	return &ScopedApplier{writeRoot: writeRoot}
}

// NewScopedApplierWithDB returns a ScopedApplier that also validates
// the approval ticket fingerprint against the database before applying.
func NewScopedApplierWithDB(writeRoot string, db *store.DB) *ScopedApplier {
	return &ScopedApplier{writeRoot: writeRoot, db: db}
}

func (a *ScopedApplier) Preview(_ context.Context, runID string, changes []model.FileChange) (model.ChangePreview, error) {
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
func (a *ScopedApplier) Apply(ctx context.Context, runID string, ticket model.ApprovalTicket, changes []model.FileChange) (model.ApplyResult, error) {
	if ticket.ID == "" || ticket.Status != "approved" {
		return model.ApplyResult{}, ErrNoApproval
	}
	if ticket.RunID != runID {
		return model.ApplyResult{}, ErrNoApproval
	}

	if a.db != nil {
		bindingJSON, err := scopedApplyBindingJSON(ticket.ToolName, changes)
		if err != nil {
			return model.ApplyResult{}, fmt.Errorf("tools: encode scoped apply binding: %w", err)
		}
		expectedFP := computeFingerprint(ticket.ToolName, ticket.ArgsJSON, bindingJSON)
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

func (a *ScopedApplier) validatePath(relPath string) error {
	if strings.ContainsRune(relPath, 0) {
		return ErrEscapeAttempt
	}

	joined := filepath.Join(a.writeRoot, relPath)
	cleaned := filepath.Clean(joined)
	root := filepath.Clean(a.writeRoot)

	if cleaned != root && !strings.HasPrefix(cleaned, root+string(filepath.Separator)) {
		return ErrEscapeAttempt
	}

	return nil
}

func scopedApplyBindingJSON(toolName string, changes []model.FileChange) ([]byte, error) {
	operands := make([]string, 0, len(changes))
	for _, change := range changes {
		if strings.TrimSpace(change.Path) == "" {
			continue
		}
		operands = append(operands, change.Path)
	}
	return json.Marshal(struct {
		ToolName string   `json:"tool_name"`
		Operands []string `json:"operands,omitempty"`
	}{
		ToolName: toolName,
		Operands: operands,
	})
}
