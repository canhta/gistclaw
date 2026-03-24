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
	var parentRunID string
	err := s.db.RawDB().QueryRowContext(ctx,
		"SELECT status, COALESCE(objective, ''), COALESCE(parent_run_id, '') FROM runs WHERE id = ?",
		runID,
	).Scan(&status, &objective, &parentRunID)
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

	var workerCount int
	err = s.db.RawDB().QueryRowContext(ctx,
		"SELECT count(*) FROM runs WHERE parent_run_id = ?",
		runID,
	).Scan(&workerCount)
	if err != nil {
		workerCount = 0
	}

	summary := fmt.Sprintf("Run %s: %s (%s, %d events)", runID, objective, status, eventCount)
	if parentRunID == "" {
		summary = fmt.Sprintf(
			"Run %s: front session with %d %s (%s, %d events)",
			runID,
			workerCount,
			workerLabel(workerCount),
			status,
			eventCount,
		)
	} else {
		summary = fmt.Sprintf(
			"Run %s: worker session under %s (%s, %d events)",
			runID,
			parentRunID,
			status,
			eventCount,
		)
	}

	return PreviewPackage{
		RunID:      runID,
		Summary:    summary,
		Receipt:    receipt,
		ReplayPath: fmt.Sprintf("/replay/%s", runID),
	}, nil
}

func workerLabel(count int) string {
	if count == 1 {
		return "worker run"
	}
	return "worker runs"
}
