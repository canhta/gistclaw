package app

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v4"
)

type Config struct {
	WorkspaceRoot string         `yaml:"workspace_root"`
	StateDir      string         `yaml:"state_dir"`
	DatabasePath  string         `yaml:"database_path"`
	TeamDir       string         `yaml:"team_dir"`
	Provider      ProviderConfig `yaml:"provider"`
	AdminToken    string         `yaml:"-"`
}

type ProviderConfig struct {
	Name    string     `yaml:"name"`
	APIKey  string     `yaml:"api_key"`
	BaseURL string     `yaml:"base_url"` // optional; overrides the default endpoint (e.g. for Ollama, Groq, Azure)
	Models  ModelLanes `yaml:"models"`
}

type ModelLanes struct {
	Cheap  string `yaml:"cheap"`
	Strong string `yaml:"strong"`
}

func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.yaml"
	}

	return filepath.Join(home, ".config", "gistclaw", "config.yaml")
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	cfg.applyDefaults()
	return cfg, nil
}

func (c *Config) validate() error {
	if c.WorkspaceRoot == "" {
		return fmt.Errorf("config validation: workspace_root is required")
	}

	info, err := os.Stat(c.WorkspaceRoot)
	if err != nil {
		return fmt.Errorf("config validation: workspace_root %q: %w", c.WorkspaceRoot, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("config validation: workspace_root %q is not a directory", c.WorkspaceRoot)
	}

	if c.Provider.Name == "" {
		return fmt.Errorf("config validation: provider name is required")
	}
	if c.Provider.Name != "anthropic" && c.Provider.Name != "openai" {
		return fmt.Errorf("config validation: unknown provider %q", c.Provider.Name)
	}
	if c.Provider.APIKey == "" {
		return fmt.Errorf("config validation: provider api_key is required")
	}

	return nil
}

func (c *Config) applyDefaults() {
	if c.StateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			c.StateDir = filepath.Join(".local", "share", "gistclaw")
		} else {
			c.StateDir = filepath.Join(home, ".local", "share", "gistclaw")
		}
	}

	if c.DatabasePath == "" {
		c.DatabasePath = filepath.Join(c.StateDir, "runtime.db")
	}
}
