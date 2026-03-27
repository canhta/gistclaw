package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// findModuleRoot walks up from the current file to find the go.mod root.
func findModuleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine caller path")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

// makeValidConfig writes a minimal valid config.yaml and returns the path.
func makeValidConfig(t *testing.T, dbPath, storageRoot string) string {
	t.Helper()
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	content := "provider:\n  name: anthropic\n  api_key: sk-test\ndatabase_path: " + dbPath + "\nstorage_root: " + storageRoot + "\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return cfgPath
}

func testOptions(configPath string) globalOptions {
	return globalOptions{ConfigPath: configPath}
}
