package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/authority"
	"github.com/canhta/gistclaw/internal/store"
)

type exportEnvelope struct {
	SchemaVersion string           `json:"schema_version"`
	ExportedAt    time.Time        `json:"exported_at"`
	Runs          []exportRun      `json:"runs"`
	Receipts      []exportReceipt  `json:"receipts"`
	Approvals     []exportApproval `json:"approvals"`
}

type exportRun struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	AgentID        string    `json:"agent_id"`
	Objective      string    `json:"objective"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type exportReceipt struct {
	ID           string    `json:"id"`
	RunID        string    `json:"run_id"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	CostUSD      float64   `json:"cost_usd"`
	CreatedAt    time.Time `json:"created_at"`
}

type exportApproval struct {
	ID         string     `json:"id"`
	RunID      string     `json:"run_id"`
	ToolName   string     `json:"tool_name"`
	TargetPath string     `json:"target_path"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

// runExport exports projected entities (runs, receipts, approvals) to a JSON file.
// Raw journal events are excluded per the guardrail.
func runExport(args []string, stdout, stderr io.Writer) int {
	srcPath, _ := parseFlag(args, "--db")
	outPath, _ := parseFlag(args, "--out")

	if srcPath == "" {
		fmt.Fprintln(stderr, "Usage: gistclaw export --db <path> --out <output.json>")
		return 1
	}
	if outPath == "" {
		fmt.Fprintln(stderr, "error: --out <output.json> is required")
		return 1
	}

	db, err := store.Open(srcPath)
	if err != nil {
		fmt.Fprintf(stderr, "export: open db: %v\n", err)
		return 1
	}
	defer db.Close()

	ctx := context.Background()
	env := exportEnvelope{
		SchemaVersion: "1.0",
		ExportedAt:    time.Now().UTC(),
	}

	// Runs.
	runsRows, err := db.RawDB().QueryContext(ctx,
		`SELECT id, conversation_id, agent_id, COALESCE(objective,''), status, created_at, updated_at FROM runs ORDER BY created_at`)
	if err != nil {
		fmt.Fprintf(stderr, "export: query runs: %v\n", err)
		return 1
	}
	defer runsRows.Close()
	for runsRows.Next() {
		var r exportRun
		if err := runsRows.Scan(&r.ID, &r.ConversationID, &r.AgentID, &r.Objective, &r.Status, &r.CreatedAt, &r.UpdatedAt); err != nil {
			fmt.Fprintf(stderr, "export: scan run: %v\n", err)
			return 1
		}
		env.Runs = append(env.Runs, r)
	}
	_ = runsRows.Close()

	// Receipts.
	recRows, err := db.RawDB().QueryContext(ctx,
		`SELECT id, run_id, COALESCE(input_tokens,0), COALESCE(output_tokens,0), COALESCE(cost_usd,0), created_at FROM receipts ORDER BY created_at`)
	if err != nil {
		fmt.Fprintf(stderr, "export: query receipts: %v\n", err)
		return 1
	}
	defer recRows.Close()
	for recRows.Next() {
		var r exportReceipt
		if err := recRows.Scan(&r.ID, &r.RunID, &r.InputTokens, &r.OutputTokens, &r.CostUSD, &r.CreatedAt); err != nil {
			fmt.Fprintf(stderr, "export: scan receipt: %v\n", err)
			return 1
		}
		env.Receipts = append(env.Receipts, r)
	}
	_ = recRows.Close()

	// Approvals.
	appRows, err := db.RawDB().QueryContext(ctx,
		`SELECT id, run_id, tool_name, binding_json, status, created_at, resolved_at FROM approvals ORDER BY created_at`)
	if err != nil {
		fmt.Fprintf(stderr, "export: query approvals: %v\n", err)
		return 1
	}
	defer appRows.Close()
	for appRows.Next() {
		var a exportApproval
		var bindingJSON []byte
		var resolvedAt sql.NullTime
		if err := appRows.Scan(&a.ID, &a.RunID, &a.ToolName, &bindingJSON, &a.Status, &a.CreatedAt, &resolvedAt); err != nil {
			fmt.Fprintf(stderr, "export: scan approval: %v\n", err)
			return 1
		}
		a.TargetPath = exportApprovalTarget(bindingJSON)
		if resolvedAt.Valid {
			t := resolvedAt.Time
			a.ResolvedAt = &t
		}
		env.Approvals = append(env.Approvals, a)
	}
	_ = appRows.Close()

	// Ensure slices are never nil in JSON.
	if env.Runs == nil {
		env.Runs = []exportRun{}
	}
	if env.Receipts == nil {
		env.Receipts = []exportReceipt{}
	}
	if env.Approvals == nil {
		env.Approvals = []exportApproval{}
	}

	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "export: marshal: %v\n", err)
		return 1
	}

	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		fmt.Fprintf(stderr, "export: write: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "exported to %s (runs=%d receipts=%d approvals=%d)\n",
		outPath, len(env.Runs), len(env.Receipts), len(env.Approvals))
	return 0
}

func exportApprovalTarget(bindingJSON []byte) string {
	var binding authority.Binding
	if err := json.Unmarshal(bindingJSON, &binding); err != nil {
		return ""
	}
	for _, operand := range binding.Operands {
		if strings.TrimSpace(operand) != "" {
			return strings.TrimSpace(operand)
		}
	}
	if strings.TrimSpace(binding.CWD) != "" {
		return strings.TrimSpace(binding.CWD)
	}
	for _, root := range binding.WriteRoots {
		if strings.TrimSpace(root) != "" {
			return strings.TrimSpace(root)
		}
	}
	for _, root := range binding.ReadRoots {
		if strings.TrimSpace(root) != "" {
			return strings.TrimSpace(root)
		}
	}
	return ""
}
