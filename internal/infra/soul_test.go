// internal/infra/soul_test.go
package infra_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/infra"
)

func TestSOULLoaderMissingFile(t *testing.T) {
	loader := infra.NewSOULLoader("/nonexistent/SOUL.md")
	content, err := loader.Load()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if content != "" {
		t.Errorf("expected empty content on error, got %q", content)
	}
}

func TestSOULLoaderLoadsContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SOUL.md")
	if err := os.WriteFile(path, []byte("you are a helpful assistant"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	loader := infra.NewSOULLoader(path)
	content, err := loader.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if content != "you are a helpful assistant" {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestSOULLoaderCachesOnMtime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SOUL.md")
	if err := os.WriteFile(path, []byte("v1"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	loader := infra.NewSOULLoader(path)
	content1, _ := loader.Load()

	// Call Load again without changing the file — mtime unchanged, cache must return v1.
	content2, _ := loader.Load()
	if content1 != content2 {
		t.Errorf("expected cached result; content changed without mtime change")
	}
}

func TestSOULLoaderReloadsOnMtimeChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SOUL.md")
	if err := os.WriteFile(path, []byte("v1"), 0600); err != nil {
		t.Fatalf("write v1: %v", err)
	}

	loader := infra.NewSOULLoader(path)
	content1, _ := loader.Load()
	if content1 != "v1" {
		t.Fatalf("initial load: got %q", content1)
	}

	// Ensure mtime changes (sleep 10ms to guarantee a different mtime on fast systems).
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("v2"), 0600); err != nil {
		t.Fatalf("write v2: %v", err)
	}

	content2, err := loader.Load()
	if err != nil {
		t.Fatalf("Load after update: %v", err)
	}
	if content2 != "v2" {
		t.Errorf("expected reload; got %q", content2)
	}
}
