package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/authority"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/tools"
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

func TestConfig_RequiresStorageRoot(t *testing.T) {
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

	cfg, err := LoadConfig(cfgPath)
	if err == nil || !strings.Contains(err.Error(), "storage_root") {
		t.Fatalf("expected storage_root error, got %v", err)
	}
	if cfg.StateDir != "" {
		t.Fatalf("expected zero config on load failure, got %#v", cfg)
	}
}

func TestConfig_DefaultPermissionModes(t *testing.T) {
	dir := t.TempDir()
	storageRoot := filepath.Join(dir, "storage")
	if err := os.Mkdir(storageRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
storage_root: `+storageRoot+`
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
		t.Fatalf("unexpected error for default permission modes: %v", err)
	}
	if cfg.ApprovalMode != authority.ApprovalModePrompt {
		t.Fatalf("expected default approval_mode %q, got %q", authority.ApprovalModePrompt, cfg.ApprovalMode)
	}
	if cfg.HostAccessMode != authority.HostAccessModeStandard {
		t.Fatalf("expected default host_access_mode %q, got %q", authority.HostAccessModeStandard, cfg.HostAccessMode)
	}
	if cfg.StateDir == "" {
		t.Fatal("expected StateDir to have a default value")
	}
}

func TestConfig_MissingAPIKey(t *testing.T) {
	dir := t.TempDir()
	storageRoot := filepath.Join(dir, "storage")
	if err := os.Mkdir(storageRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
storage_root: `+storageRoot+`
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
	storageRoot := filepath.Join(dir, "storage")
	if err := os.Mkdir(storageRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
storage_root: `+storageRoot+`
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
	storageRoot := filepath.Join(dir, "storage")
	if err := os.Mkdir(storageRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
storage_root: `+storageRoot+`
approval_mode: auto_approve
host_access_mode: elevated
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
	if cfg.StorageRoot != storageRoot {
		t.Fatalf("expected storage_root %q, got %q", storageRoot, cfg.StorageRoot)
	}
	if cfg.ApprovalMode != authority.ApprovalModeAutoApprove {
		t.Fatalf("expected approval_mode %q, got %q", authority.ApprovalModeAutoApprove, cfg.ApprovalMode)
	}
	if cfg.HostAccessMode != authority.HostAccessModeElevated {
		t.Fatalf("expected host_access_mode %q, got %q", authority.HostAccessModeElevated, cfg.HostAccessMode)
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
	storageRoot := filepath.Join(dir, "storage")
	if err := os.Mkdir(storageRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
storage_root: `+storageRoot+`
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

func TestLoadInstallConfig_RequiresExplicitRuntimePaths(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
provider:
  name: openai
  api_key: sk-test-1234
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = LoadInstallConfig(cfgPath)
	if err == nil || !strings.Contains(err.Error(), "database_path") {
		t.Fatalf("expected install config to require database_path, got %v", err)
	}
}

func TestLoadInstallConfig_AllowsMissingOwnedPaths(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	dbPath := filepath.Join(dir, "srv", "data", "runtime.db")
	storageRoot := filepath.Join(dir, "srv", "storage")
	err := os.WriteFile(cfgPath, []byte(`
provider:
  name: openai
  api_key: sk-test-1234
database_path: `+dbPath+`
storage_root: `+storageRoot+`
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadInstallConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabasePath != dbPath {
		t.Fatalf("expected database_path %q, got %q", dbPath, cfg.DatabasePath)
	}
	if cfg.StorageRoot != storageRoot {
		t.Fatalf("expected storage_root %q, got %q", storageRoot, cfg.StorageRoot)
	}
}

func TestLoadInstallConfig_RejectsStorageRootFile(t *testing.T) {
	dir := t.TempDir()
	storageFile := filepath.Join(dir, "storage-file")
	if err := os.WriteFile(storageFile, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
provider:
  name: openai
  api_key: sk-test-1234
database_path: `+filepath.Join(dir, "runtime.db")+`
storage_root: `+storageFile+`
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = LoadInstallConfig(cfgPath)
	if err == nil || !strings.Contains(err.Error(), "storage_root") {
		t.Fatalf("expected storage_root install validation error, got %v", err)
	}
}

func TestConfig_UnknownResearchProvider(t *testing.T) {
	dir := t.TempDir()
	storageRoot := filepath.Join(dir, "storage")
	if err := os.Mkdir(storageRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
storage_root: `+storageRoot+`
provider:
  name: anthropic
  api_key: sk-test-1234
research:
  provider: bing
  api_key: research-key
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = LoadConfig(cfgPath)
	if err == nil || !strings.Contains(err.Error(), "research") {
		t.Fatalf("expected research validation error, got %v", err)
	}
}

func TestConfig_ResearchProviderRequiresAPIKey(t *testing.T) {
	dir := t.TempDir()
	storageRoot := filepath.Join(dir, "storage")
	if err := os.Mkdir(storageRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
storage_root: `+storageRoot+`
provider:
  name: anthropic
  api_key: sk-test-1234
research:
  provider: tavily
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = LoadConfig(cfgPath)
	if err == nil || !strings.Contains(err.Error(), "api_key") {
		t.Fatalf("expected research api_key error, got %v", err)
	}
}

func TestConfig_RejectsUnknownMCPTransport(t *testing.T) {
	cfg := Config{
		StorageRoot: t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
		MCP: tools.MCPOptions{
			Servers: []tools.MCPServerConfig{
				{
					ID:        "github",
					Transport: "sse",
					Command:   []string{"mcp-server"},
				},
			},
		},
	}

	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "transport") {
		t.Fatalf("expected transport validation error, got %v", err)
	}
}

func TestConfig_RejectsDuplicateMCPAliases(t *testing.T) {
	cfg := Config{
		StorageRoot: t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
		MCP: tools.MCPOptions{
			Servers: []tools.MCPServerConfig{
				{
					ID:        "github",
					Transport: "stdio",
					Command:   []string{"mcp-server"},
					Tools: []tools.MCPToolConfig{
						{Name: "one", Alias: "dup", Risk: model.RiskLow, Enabled: true},
						{Name: "two", Alias: "dup", Risk: model.RiskLow, Enabled: true},
					},
				},
			},
		},
	}

	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate alias validation error, got %v", err)
	}
}

func TestConfig_RejectsHighRiskMCPTool(t *testing.T) {
	cfg := Config{
		StorageRoot: t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
		MCP: tools.MCPOptions{
			Servers: []tools.MCPServerConfig{
				{
					ID:        "github",
					Transport: "stdio",
					Command:   []string{"mcp-server"},
					Tools: []tools.MCPToolConfig{
						{Name: "danger", Alias: "danger", Risk: model.RiskHigh, Enabled: true},
					},
				},
			},
		},
	}

	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "risk") {
		t.Fatalf("expected risk validation error, got %v", err)
	}
}

func TestConfig_RejectsUnknownApprovalMode(t *testing.T) {
	cfg := Config{
		StorageRoot:  t.TempDir(),
		ApprovalMode: authority.ApprovalMode("skip_all"),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
	}

	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "approval_mode") {
		t.Fatalf("expected approval_mode validation error, got %v", err)
	}
}

func TestConfig_RejectsUnknownHostAccessMode(t *testing.T) {
	cfg := Config{
		StorageRoot:    t.TempDir(),
		HostAccessMode: authority.HostAccessMode("god_mode"),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
	}

	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "host_access_mode") {
		t.Fatalf("expected host_access_mode validation error, got %v", err)
	}
}
