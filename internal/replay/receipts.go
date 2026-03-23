package replay

import (
	"context"
	"fmt"

	"github.com/canhta/gistclaw/internal/model"
)

type ReceiptComparison struct {
	Left  model.RunReceipt
	Right model.RunReceipt
}

func (s *Service) Build(ctx context.Context, rootRunID string) (model.RunReceipt, error) {
	var receipt model.RunReceipt
	err := s.db.RawDB().QueryRowContext(ctx,
		`SELECT id, run_id, input_tokens, output_tokens, cost_usd,
		 COALESCE(model_lane, ''), COALESCE(verification_status, ''),
		 COALESCE(approval_count, 0), COALESCE(budget_status, ''), created_at
		 FROM receipts
		 WHERE run_id = ?`,
		rootRunID,
	).Scan(
		&receipt.ID,
		&receipt.RunID,
		&receipt.InputTokens,
		&receipt.OutputTokens,
		&receipt.CostUSD,
		&receipt.ModelLane,
		&receipt.VerificationStatus,
		&receipt.ApprovalCount,
		&receipt.BudgetStatus,
		&receipt.CreatedAt,
	)
	if err != nil {
		return model.RunReceipt{}, fmt.Errorf("replay: build receipt: %w", err)
	}
	return receipt, nil
}

func (s *Service) Compare(ctx context.Context, leftRunID, rightRunID string) (ReceiptComparison, error) {
	left, err := s.Build(ctx, leftRunID)
	if err != nil {
		return ReceiptComparison{}, fmt.Errorf("compare left: %w", err)
	}
	right, err := s.Build(ctx, rightRunID)
	if err != nil {
		return ReceiptComparison{}, fmt.Errorf("compare right: %w", err)
	}
	return ReceiptComparison{Left: left, Right: right}, nil
}
