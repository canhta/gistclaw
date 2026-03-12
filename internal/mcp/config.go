// internal/mcp/config.go
package mcp

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// MCPServerConfig holds the configuration for a single MCP server.
// Exactly one of Command (stdio transport) or URL (SSE/HTTP transport) must be set.
type MCPServerConfig struct {
	// Stdio transport
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Env     []string `yaml:"env"`      // KEY=VALUE pairs passed as environment variables
	EnvFile string   `yaml:"env_file"` // path to a .env file for this server

	// SSE / HTTP transport
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers"`
}

// gistclawYAML is the top-level structure of gistclaw.yaml.
type gistclawYAML struct {
	MCPServers map[string]MCPServerConfig `yaml:"mcp_servers"`
}

// LoadMCPConfig reads a gistclaw.yaml file at path and returns the named server
// configurations. If the file does not exist, an empty (non-nil) map is returned
// with no error. Any other I/O error or YAML parse error is returned as an error.
func LoadMCPConfig(path string) (map[string]MCPServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]MCPServerConfig), nil
		}
		return nil, fmt.Errorf("mcp: read config %q: %w", path, err)
	}

	var doc gistclawYAML
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("mcp: parse config %q: %w", path, err)
	}

	if doc.MCPServers == nil {
		return make(map[string]MCPServerConfig), nil
	}
	return doc.MCPServers, nil
}
