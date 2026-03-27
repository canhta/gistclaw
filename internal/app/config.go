package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/canhta/gistclaw/internal/authority"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/tools"
	"go.yaml.in/yaml/v4"
)

type Config struct {
	StorageRoot    string                   `yaml:"storage_root"`
	StateDir       string                   `yaml:"state_dir"`
	DatabasePath   string                   `yaml:"database_path"`
	TeamDir        string                   `yaml:"team_dir"`
	ApprovalMode   authority.ApprovalMode   `yaml:"approval_mode"`
	HostAccessMode authority.HostAccessMode `yaml:"host_access_mode"`
	Provider       ProviderConfig           `yaml:"provider"`
	Research       tools.ResearchConfig     `yaml:"research"`
	MCP            tools.MCPOptions         `yaml:"mcp"`
	Telegram       TelegramConfig           `yaml:"telegram"`
	WhatsApp       WhatsAppConfig           `yaml:"whatsapp"`
	ZaloPersonal   ZaloPersonalConfig       `yaml:"zalo_personal"`
	Web            WebConfig                `yaml:"web"`
	AdminToken     string                   `yaml:"-"`
}

type ProviderConfig struct {
	Name    string     `yaml:"name"`
	APIKey  string     `yaml:"api_key"`
	BaseURL string     `yaml:"base_url"` // optional; overrides the default endpoint (e.g. for Ollama, Groq, Azure)
	WireAPI string     `yaml:"wire_api"`
	Models  ModelLanes `yaml:"models"`
}

type ModelLanes struct {
	Cheap  string `yaml:"cheap"`
	Strong string `yaml:"strong"`
}

type TelegramConfig struct {
	BotToken string `yaml:"bot_token"`
	AgentID  string `yaml:"agent_id"`
}

type WhatsAppConfig struct {
	PhoneNumberID string `yaml:"phone_number_id"`
	AccessToken   string `yaml:"access_token"`
	VerifyToken   string `yaml:"verify_token"`
	AgentID       string `yaml:"agent_id"`
}

type ZaloPersonalConfig struct {
	Enabled bool   `yaml:"enabled"`
	AgentID string `yaml:"agent_id"`
}

type WebConfig struct {
	ListenAddr string `yaml:"listen_addr"`
}

func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.yaml"
	}

	return filepath.Join(home, ".config", "gistclaw", "config.yaml")
}

func LoadConfig(path string) (Config, error) {
	cfg, err := readConfig(path)
	if err != nil {
		return Config{}, err
	}
	cfg.applyDefaults(defaultStateDir())
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// LoadConfigRaw parses the config file without running validation. Used by
// operator commands (doctor) that check fields individually.
func LoadConfigRaw(path string) (Config, error) {
	cfg, err := readConfig(path)
	if err != nil {
		return Config{}, err
	}
	cfg.applyDefaults(defaultStateDir())
	return cfg, nil
}

// LoadInstallConfig parses the config file for installer use. It applies
// service-install defaults and validates semantic fields without requiring
// installer-owned paths to exist yet.
func LoadInstallConfig(path string) (Config, error) {
	cfg, err := readConfig(path)
	if err != nil {
		return Config{}, err
	}
	databasePathProvided := cfg.DatabasePath != ""
	storageRootProvided := strings.TrimSpace(cfg.StorageRoot) != ""
	cfg.applyDefaults(SystemdWorkingDirectory)
	if !databasePathProvided {
		return Config{}, fmt.Errorf("config validation: database_path is required for install config")
	}
	if !storageRootProvided {
		return Config{}, fmt.Errorf("config validation: storage_root is required for install config")
	}
	if err := cfg.validateForInstall(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if err := c.validateCommon(); err != nil {
		return err
	}
	return c.validateRuntimePaths()
}

func (c *Config) validateForInstall() error {
	if err := c.validateCommon(); err != nil {
		return err
	}
	return c.validateInstallPaths()
}

func (c *Config) validateCommon() error {
	if strings.TrimSpace(c.StorageRoot) == "" {
		return fmt.Errorf("config validation: storage_root is required")
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
	if c.Provider.WireAPI != "" &&
		c.Provider.WireAPI != "chat_completions" &&
		c.Provider.WireAPI != "responses" {
		return fmt.Errorf("config validation: unknown provider wire_api %q", c.Provider.WireAPI)
	}
	if c.Research.Provider != "" {
		if c.Research.Provider != "tavily" {
			return fmt.Errorf("config validation: unknown research provider %q", c.Research.Provider)
		}
		if c.Research.APIKey == "" {
			return fmt.Errorf("config validation: research api_key is required for provider %q", c.Research.Provider)
		}
	}
	if err := validateMCPConfig(c.MCP); err != nil {
		return err
	}
	if c.ApprovalMode != "" &&
		c.ApprovalMode != authority.ApprovalModePrompt &&
		c.ApprovalMode != authority.ApprovalModeAutoApprove {
		return fmt.Errorf("config validation: unknown approval_mode %q", c.ApprovalMode)
	}
	if c.HostAccessMode != "" &&
		c.HostAccessMode != authority.HostAccessModeStandard &&
		c.HostAccessMode != authority.HostAccessModeElevated {
		return fmt.Errorf("config validation: unknown host_access_mode %q", c.HostAccessMode)
	}

	return nil
}

func (c *Config) validateRuntimePaths() error {
	info, err := os.Stat(c.StorageRoot)
	if err != nil {
		return fmt.Errorf("config validation: storage_root %q: %w", c.StorageRoot, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("config validation: storage_root %q is not a directory", c.StorageRoot)
	}
	return nil
}

func (c *Config) validateInstallPaths() error {
	if err := validateInstallDirectory("storage_root", c.StorageRoot); err != nil {
		return err
	}
	if err := validateInstallDirectory("state_dir", c.StateDir); err != nil {
		return err
	}
	if err := validateInstallDirectory("database_path parent", filepath.Dir(c.DatabasePath)); err != nil {
		return err
	}
	return nil
}

func (c *Config) applyDefaults(defaultStateDir string) {
	if c.StateDir == "" {
		c.StateDir = defaultStateDir
	}

	if c.DatabasePath == "" {
		c.DatabasePath = filepath.Join(c.StateDir, "runtime.db")
	}
	if c.ApprovalMode == "" {
		c.ApprovalMode = authority.ApprovalModePrompt
	}
	if c.HostAccessMode == "" {
		c.HostAccessMode = authority.HostAccessModeStandard
	}
	if c.Provider.Name == "openai" && c.Provider.WireAPI == "" {
		c.Provider.WireAPI = "chat_completions"
	}
	c.Research = normalizeResearchConfig(c.Research)
	for i := range c.MCP.Servers {
		if c.MCP.Servers[i].Transport == "" {
			c.MCP.Servers[i].Transport = "stdio"
		}
	}

	if c.Telegram.AgentID == "" {
		c.Telegram.AgentID = "assistant"
	}
	if c.WhatsApp.AgentID == "" {
		c.WhatsApp.AgentID = "assistant"
	}
	if c.ZaloPersonal.AgentID == "" {
		c.ZaloPersonal.AgentID = "assistant"
	}
	if c.Web.ListenAddr == "" {
		c.Web.ListenAddr = "127.0.0.1:8080"
	}
}

func readConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

func defaultStateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".local", "share", "gistclaw")
	}
	return filepath.Join(home, ".local", "share", "gistclaw")
}

func validateInstallDirectory(label, path string) error {
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if !info.IsDir() {
			return fmt.Errorf("config validation: %s %q is not a directory", label, path)
		}
	case os.IsNotExist(err):
		return nil
	default:
		return fmt.Errorf("config validation: %s %q: %w", label, path, err)
	}
	return nil
}

func defaultProjectsRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".gistclaw", "projects")
	}
	return filepath.Join(home, ".gistclaw", "projects")
}

func validateMCPConfig(cfg tools.MCPOptions) error {
	aliases := make(map[string]bool)
	for _, server := range cfg.Servers {
		if server.ID == "" {
			return fmt.Errorf("config validation: mcp server id is required")
		}
		transport := server.Transport
		if transport == "" {
			transport = "stdio"
		}
		if transport != "stdio" {
			return fmt.Errorf("config validation: unknown mcp transport %q", server.Transport)
		}
		if len(server.Command) == 0 {
			return fmt.Errorf("config validation: mcp server %q command is required", server.ID)
		}
		for _, tool := range server.Tools {
			if !tool.Enabled {
				continue
			}
			if tool.Name == "" {
				return fmt.Errorf("config validation: mcp server %q tool name is required", server.ID)
			}
			if tool.Alias == "" {
				return fmt.Errorf("config validation: mcp server %q tool %q alias is required", server.ID, tool.Name)
			}
			if tool.Risk != model.RiskLow {
				return fmt.Errorf("config validation: mcp server %q tool %q risk must be %q", server.ID, tool.Name, model.RiskLow)
			}
			if aliases[tool.Alias] {
				return fmt.Errorf("config validation: duplicate mcp tool alias %q", tool.Alias)
			}
			aliases[tool.Alias] = true
		}
	}
	return nil
}

func normalizeResearchConfig(cfg tools.ResearchConfig) tools.ResearchConfig {
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = 5
	}
	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 10
	}
	return cfg
}
