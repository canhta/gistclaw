package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfig_DefaultPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	expected := filepath.Join(home, ".config", "gistclaw", "config.yaml")
	got := DefaultConfigPath()
	if got != expected {
		t.Fatalf("expected default path %q, got %q", expected, got)
	}
}

func TestConfig_MissingWorkspaceRoot(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
provider:
  name: anthropic
  api_key: sk-test-1234
  models:
    cheap: claude-3-haiku
    strong: claude-sonnet-4-20250514
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = LoadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing workspace_root, got nil")
	}
	if !strings.Contains(err.Error(), "workspace_root") {
		t.Fatalf("error should mention workspace_root, got: %s", err.Error())
	}
}

func TestConfig_MissingAPIKey(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	if err := os.Mkdir(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
workspace_root: `+wsDir+`
provider:
  name: anthropic
  models:
    cheap: claude-3-haiku
    strong: claude-sonnet-4-20250514
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = LoadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing api_key, got nil")
	}
	if !strings.Contains(err.Error(), "api_key") {
		t.Fatalf("error should mention api_key, got: %s", err.Error())
	}
}

func TestConfig_UnknownProvider(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	if err := os.Mkdir(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
workspace_root: `+wsDir+`
provider:
  name: cohere
  api_key: sk-test-1234
  models:
    cheap: command-r
    strong: command-r-plus
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = LoadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
	if !strings.Contains(err.Error(), "provider") {
		t.Fatalf("error should mention provider, got: %s", err.Error())
	}
}

func TestConfig_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	if err := os.Mkdir(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
workspace_root: `+wsDir+`
provider:
  name: anthropic
  api_key: sk-test-1234
  models:
    cheap: claude-3-haiku
    strong: claude-sonnet-4-20250514
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WorkspaceRoot != wsDir {
		t.Fatalf("expected workspace_root %q, got %q", wsDir, cfg.WorkspaceRoot)
	}
	if cfg.Provider.Name != "anthropic" {
		t.Fatalf("expected provider name %q, got %q", "anthropic", cfg.Provider.Name)
	}
	if cfg.Provider.APIKey != "sk-test-1234" {
		t.Fatalf("expected api_key %q, got %q", "sk-test-1234", cfg.Provider.APIKey)
	}
	if cfg.StateDir == "" {
		t.Fatal("expected StateDir to have a default value")
	}
	if cfg.DatabasePath == "" {
		t.Fatal("expected DatabasePath to have a default value")
	}
	if cfg.Web.ListenAddr == "" {
		t.Fatal("expected Web.ListenAddr to have a default value")
	}
	if cfg.WhatsApp.AgentID == "" {
		t.Fatal("expected WhatsApp.AgentID to have a default value")
	}
}

func TestConfig_OpenAIWireAPI(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	if err := os.Mkdir(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
workspace_root: `+wsDir+`
provider:
  name: openai
  api_key: sk-test-1234
  base_url: https://9router.quickdemo.site/v1
  wire_api: responses
  models:
    cheap: cx/gpt-5.4
    strong: cx/gpt-5.4
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Provider.Name != "openai" {
		t.Fatalf("expected provider name %q, got %q", "openai", cfg.Provider.Name)
	}
	if cfg.Provider.BaseURL != "https://9router.quickdemo.site/v1" {
		t.Fatalf("expected base_url to round-trip, got %q", cfg.Provider.BaseURL)
	}
	if cfg.Provider.WireAPI != "responses" {
		t.Fatalf("expected wire_api %q, got %q", "responses", cfg.Provider.WireAPI)
	}
}
