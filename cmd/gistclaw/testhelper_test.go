package main

import (
	"os"
	"path/filepath"
	"testing"
)

// makeValidConfig writes a minimal valid config.yaml and returns the path.
func makeValidConfig(t *testing.T, dbPath, workspaceRoot string) string {
	t.Helper()
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	content := "provider:\n  name: anthropic\n  api_key: sk-test\ndatabase_path: " + dbPath + "\nworkspace_root: " + workspaceRoot + "\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return cfgPath
}
