package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
