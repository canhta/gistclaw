package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/authority"
	"github.com/canhta/gistclaw/internal/store"
)

func TestExport_ProducesValidJSON(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "gistclaw.db")
	seedDB(t, dbPath)
	outPath := filepath.Join(t.TempDir(), "export.json")

	var stdout, stderr bytes.Buffer
	code := runExport([]string{"--db", dbPath, "--out", outPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("export exited %d: %s", code, stderr.String())
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read export file: %v", err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Errorf("export output is not valid JSON: %v", err)
	}
}

func TestExport_ContainsExpectedKeys(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "gistclaw.db")
	seedDB(t, dbPath)
	outPath := filepath.Join(t.TempDir(), "export.json")

	var stdout, stderr bytes.Buffer
	runExport([]string{"--db", dbPath, "--out", outPath}, &stdout, &stderr)

	data, _ := os.ReadFile(outPath)
	body := string(data)
	for _, key := range []string{"runs", "receipts", "approvals"} {
		if !strings.Contains(body, key) {
			t.Errorf("expected key %q in export JSON", key)
		}
	}
}

func TestExport_ContainsSchemaVersion(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "gistclaw.db")
	seedDB(t, dbPath)
	outPath := filepath.Join(t.TempDir(), "export.json")

	var stdout, stderr bytes.Buffer
	runExport([]string{"--db", dbPath, "--out", outPath}, &stdout, &stderr)

	data, _ := os.ReadFile(outPath)
	var env exportEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.SchemaVersion != "1.0" {
		t.Errorf("expected schema_version=1.0, got %q", env.SchemaVersion)
	}
}

func TestExport_ApprovalsUseBindingSummary(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "gistclaw.db")
	seedDB(t, dbPath)
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	bindingJSON, err := json.Marshal(authority.Binding{
		ToolName: "shell_exec",
		Operands: []string{"/tmp/demo.txt"},
		Mutating: true,
	})
	if err != nil {
		t.Fatalf("marshal binding: %v", err)
	}
	if _, err := db.RawDB().Exec(
		`INSERT INTO approvals (id, run_id, tool_name, args_json, binding_json, fingerprint, status, created_at)
		 VALUES ('ticket-export', 'run-export', 'shell_exec', x'', ?, 'fp-test', 'pending', datetime('now'))`,
		bindingJSON,
	); err != nil {
		t.Fatalf("insert approval: %v", err)
	}
	_ = db.Close()
	outPath := filepath.Join(t.TempDir(), "export.json")

	var stdout, stderr bytes.Buffer
	code := runExport([]string{"--db", dbPath, "--out", outPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("export exited %d: %s", code, stderr.String())
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read export file: %v", err)
	}
	var payload struct {
		Approvals []map[string]any `json:"approvals"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Approvals) == 0 {
		t.Fatal("expected exported approvals")
	}
	if _, ok := payload.Approvals[0]["binding_summary"]; !ok {
		t.Fatalf("expected binding_summary in approval export, got %+v", payload.Approvals[0])
	}
	if _, ok := payload.Approvals[0]["target_path"]; ok {
		t.Fatalf("did not expect target_path in approval export, got %+v", payload.Approvals[0])
	}
}

func TestExport_MissingOutFlagErrors(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "gistclaw.db")
	seedDB(t, dbPath)

	var stdout, stderr bytes.Buffer
	code := runExport([]string{"--db", dbPath}, &stdout, &stderr)
	if code == 0 {
		t.Error("expected non-zero exit when --out is missing")
	}
}

func TestExport_MissingDBFlagErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runExport([]string{"--out", "/tmp/x.json"}, &stdout, &stderr)
	if code == 0 {
		t.Error("expected non-zero exit when --db is missing")
	}
}
