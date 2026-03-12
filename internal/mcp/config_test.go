// internal/mcp/config_test.go
package mcp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/canhta/gistclaw/internal/mcp"
)

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "gistclaw.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp yaml: %v", err)
	}
	return p
}

func TestLoadMCPConfig_ParsesStdioServer(t *testing.T) {
	yaml := `
mcp_servers:
  filesystem:
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/home/user"]
    env: ["HOME=/home/user"]
`
	path := writeTempYAML(t, yaml)
	configs, err := mcp.LoadMCPConfig(path)
	if err != nil {
		t.Fatalf("LoadMCPConfig: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 server, got %d", len(configs))
	}
	fs, ok := configs["filesystem"]
	if !ok {
		t.Fatal("expected 'filesystem' key in configs")
	}
	if fs.Command != "npx" {
		t.Errorf("Command = %q, want %q", fs.Command, "npx")
	}
	if len(fs.Args) != 3 {
		t.Errorf("Args len = %d, want 3; args = %v", len(fs.Args), fs.Args)
	}
	if fs.Args[0] != "-y" {
		t.Errorf("Args[0] = %q, want %q", fs.Args[0], "-y")
	}
	if len(fs.Env) != 1 || fs.Env[0] != "HOME=/home/user" {
		t.Errorf("Env = %v, want [HOME=/home/user]", fs.Env)
	}
}

func TestLoadMCPConfig_ParsesSSEServer(t *testing.T) {
	yaml := `
mcp_servers:
  github:
    url: https://mcp.example.com/github/sse
    headers:
      Authorization: Bearer token123
      X-Custom: custom-value
`
	path := writeTempYAML(t, yaml)
	configs, err := mcp.LoadMCPConfig(path)
	if err != nil {
		t.Fatalf("LoadMCPConfig: %v", err)
	}
	gh, ok := configs["github"]
	if !ok {
		t.Fatal("expected 'github' key")
	}
	if gh.URL != "https://mcp.example.com/github/sse" {
		t.Errorf("URL = %q", gh.URL)
	}
	if gh.Headers["Authorization"] != "Bearer token123" {
		t.Errorf("Authorization header = %q", gh.Headers["Authorization"])
	}
	if gh.Headers["X-Custom"] != "custom-value" {
		t.Errorf("X-Custom header = %q", gh.Headers["X-Custom"])
	}
}

func TestLoadMCPConfig_ParsesMultipleServers(t *testing.T) {
	yaml := `
mcp_servers:
  server1:
    command: cmd1
  server2:
    url: https://example.com
  server3:
    command: cmd3
    env_file: /path/to/.env
`
	path := writeTempYAML(t, yaml)
	configs, err := mcp.LoadMCPConfig(path)
	if err != nil {
		t.Fatalf("LoadMCPConfig: %v", err)
	}
	if len(configs) != 3 {
		t.Fatalf("expected 3 servers, got %d", len(configs))
	}
	if configs["server3"].EnvFile != "/path/to/.env" {
		t.Errorf("server3.EnvFile = %q", configs["server3"].EnvFile)
	}
}

func TestLoadMCPConfig_MissingFileReturnsEmptyMap(t *testing.T) {
	configs, err := mcp.LoadMCPConfig("/nonexistent/path/gistclaw.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if configs == nil {
		t.Fatal("expected non-nil map")
	}
	if len(configs) != 0 {
		t.Errorf("expected empty map, got %d entries", len(configs))
	}
}

func TestLoadMCPConfig_EmptySection(t *testing.T) {
	yaml := "mcp_servers:\n"
	path := writeTempYAML(t, yaml)
	configs, err := mcp.LoadMCPConfig(path)
	if err != nil {
		t.Fatalf("LoadMCPConfig: %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("expected empty map, got %d entries", len(configs))
	}
}

func TestLoadMCPConfig_InvalidYAMLReturnsError(t *testing.T) {
	yaml := "mcp_servers: [unclosed"
	path := writeTempYAML(t, yaml)
	_, err := mcp.LoadMCPConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
