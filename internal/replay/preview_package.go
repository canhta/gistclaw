package replay

import (
	"context"
	"fmt"

	"github.com/canhta/gistclaw/internal/model"
)

type PreviewPackage struct {
	RunID                string
	Summary              string
	GroundedReasons      []string
	ProposedDiff         string
	VerificationEvidence string
	Receipt              model.RunReceipt
	ReplayPath           string
}

func (s *Service) BuildPreviewPackage(ctx context.Context, runID string) (PreviewPackage, error) {
	var status string
	var objective string
	err := s.db.RawDB().QueryRowContext(ctx,
		"SELECT status, COALESCE(objective, '') FROM runs WHERE id = ?",
		runID,
	).Scan(&status, &objective)
	if err != nil {
		return PreviewPackage{}, fmt.Errorf("preview: load run: %w", err)
	}

	receipt, err := s.Build(ctx, runID)
	if err != nil {
		receipt = model.RunReceipt{RunID: runID}
	}

	var eventCount int
	err = s.db.RawDB().QueryRowContext(ctx,
		"SELECT count(*) FROM events WHERE run_id = ?",
		runID,
	).Scan(&eventCount)
	if err != nil {
		eventCount = 0
	}

	return PreviewPackage{
		RunID:      runID,
		Summary:    fmt.Sprintf("Run %s: %s (%s, %d events)", runID, objective, status, eventCount),
		Receipt:    receipt,
		ReplayPath: fmt.Sprintf("/replay/%s", runID),
	}, nil
}
