package tools

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

var ErrTicketExpired = fmt.Errorf("tools: approval ticket expired")

func CreateTicket(ctx context.Context, db *store.DB, req model.ApprovalRequest) (model.ApprovalTicket, error) {
	fingerprint := computeFingerprint(req.ToolName, req.ArgsJSON, req.TargetPath)
	id := toolsGenerateID()
	now := time.Now().UTC()

	_, err := db.RawDB().ExecContext(ctx,
		`INSERT INTO approvals (id, run_id, tool_name, args_json, target_path, fingerprint, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', ?)`,
		id, req.RunID, req.ToolName, req.ArgsJSON, req.TargetPath, fingerprint, now,
	)
	if err != nil {
		return model.ApprovalTicket{}, fmt.Errorf("create ticket: %w", err)
	}

	return model.ApprovalTicket{
		ID:          id,
		RunID:       req.RunID,
		ToolName:    req.ToolName,
		ArgsJSON:    req.ArgsJSON,
		TargetPath:  req.TargetPath,
		Fingerprint: fingerprint,
		Status:      "pending",
		CreatedAt:   now,
	}, nil
}

func ResolveTicket(ctx context.Context, db *store.DB, ticketID string, decision string) error {
	if decision != "approved" && decision != "denied" {
		return fmt.Errorf("tools: invalid decision %q", decision)
	}

	var status string
	err := db.RawDB().QueryRowContext(ctx,
		"SELECT status FROM approvals WHERE id = ?",
		ticketID,
	).Scan(&status)
	if err != nil {
		return fmt.Errorf("resolve ticket: %w", err)
	}
	if status != "pending" {
		return ErrTicketExpired
	}

	result, err := db.RawDB().ExecContext(ctx,
		"UPDATE approvals SET status = ?, resolved_at = datetime('now') WHERE id = ? AND status = 'pending'",
		decision, ticketID,
	)
	if err != nil {
		return fmt.Errorf("resolve ticket: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("resolve ticket: %w", err)
	}
	if affected != 1 {
		return ErrTicketExpired
	}

	return nil
}

// LoadTicket fetches the current state of an approval ticket from the database.
func LoadTicket(ctx context.Context, db *store.DB, ticketID string) (model.ApprovalTicket, error) {
	var t model.ApprovalTicket
	err := db.RawDB().QueryRowContext(ctx,
		`SELECT id, run_id, tool_name, args_json, COALESCE(target_path,''), fingerprint, status, created_at
		 FROM approvals WHERE id = ?`,
		ticketID,
	).Scan(&t.ID, &t.RunID, &t.ToolName, &t.ArgsJSON, &t.TargetPath, &t.Fingerprint, &t.Status, &t.CreatedAt)
	if err != nil {
		return model.ApprovalTicket{}, fmt.Errorf("load ticket: %w", err)
	}
	return t, nil
}

func VerifyTicket(ctx context.Context, db *store.DB, ticketID string, currentFingerprint string) error {
	var storedFingerprint string
	var status string
	err := db.RawDB().QueryRowContext(ctx,
		"SELECT fingerprint, status FROM approvals WHERE id = ?",
		ticketID,
	).Scan(&storedFingerprint, &status)
	if err != nil {
		return fmt.Errorf("verify ticket: %w", err)
	}
	// An approved ticket must have status "approved" and a matching fingerprint.
	if status != "approved" || storedFingerprint != currentFingerprint {
		return ErrTicketExpired
	}
	return nil
}

func computeFingerprint(toolName string, argsJSON []byte, targetPath string) string {
	sum := sha256.Sum256([]byte(toolName + ":" + string(argsJSON) + ":" + targetPath))
	return fmt.Sprintf("%x", sum)
}

func ApprovalTargetPath(_ string, argsJSON []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(argsJSON, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"path", "to", "target", "file"} {
		value, ok := payload[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			return text
		}
	}
	return ""
}

func toolsGenerateID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
