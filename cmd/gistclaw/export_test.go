package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/store"
)

func seedDBWithData(t *testing.T, dbPath string) {
	t.Helper()
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Insert a minimal conversation + run + receipt + approval.
	_, err = db.RawDB().Exec(
		`INSERT INTO conversations (id, key, created_at) VALUES ('conv-export', 'test:acct:ext:main', datetime('now'))`)
	if err != nil {
		t.Fatalf("insert conv: %v", err)
	}
	_, err = db.RawDB().Exec(
		`INSERT INTO runs (id, conversation_id, agent_id, objective, status, created_at, updated_at)
		 VALUES ('run-export', 'conv-export', 'coordinator', 'export test', 'completed', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}
	_, err = db.RawDB().Exec(
		`INSERT INTO receipts (id, run_id, created_at) VALUES ('receipt-export', 'run-export', datetime('now'))`)
	if err != nil {
		t.Fatalf("insert receipt: %v", err)
	}
	_, err = db.RawDB().Exec(
		`INSERT INTO approvals (id, run_id, tool_name, args_json, target_path, fingerprint, status, created_at)
		 VALUES ('approval-export', 'run-export', 'bash', x'', '/tmp', 'fp', 'pending', datetime('now'))`)
	if err != nil {
		t.Fatalf("insert approval: %v", err)
	}
}

func TestExport_ProducesValidJSON(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	seedDBWithData(t, dbPath)
	outPath := filepath.Join(t.TempDir(), "export.json")

	cmd := exec.Command(bin, "export", "--db", dbPath, "--out", outPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("export failed: %v\n%s", err, out)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, data)
	}
}

func TestExport_ContainsExpectedKeys(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	seedDBWithData(t, dbPath)
	outPath := filepath.Join(t.TempDir(), "export.json")

	cmd := exec.Command(bin, "export", "--db", dbPath, "--out", outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("export failed: %v\n%s", err, out)
	}

	data, _ := os.ReadFile(outPath)
	var result map[string]any
	_ = json.Unmarshal(data, &result)

	for _, key := range []string{"runs", "receipts", "approvals"} {
		if _, ok := result[key]; !ok {
			t.Errorf("expected key %q in export JSON", key)
		}
	}
	if _, ok := result["events"]; ok {
		t.Error("export must not include raw journal 'events' key")
	}
}

func TestExport_ContainsSchemaVersion(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	seedDBWithData(t, dbPath)
	outPath := filepath.Join(t.TempDir(), "export.json")

	cmd := exec.Command(bin, "export", "--db", dbPath, "--out", outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("export failed: %v\n%s", err, out)
	}

	data, _ := os.ReadFile(outPath)
	var result map[string]any
	_ = json.Unmarshal(data, &result)

	if result["schema_version"] == "" || result["schema_version"] == nil {
		t.Errorf("expected schema_version in export JSON, got %v", result["schema_version"])
	}
}

func TestExport_MissingOutFlagErrors(t *testing.T) {
	bin := buildBinary(t)
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	seedDBWithData(t, dbPath)

	cmd := exec.Command(bin, "export", "--db", dbPath)
	out, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatal("expected non-zero exit code when --out is missing")
	}
	if !strings.Contains(string(out), "--out") && !strings.Contains(string(out), "output") {
		t.Errorf("expected usage error mentioning --out, got:\n%s", out)
	}
}
