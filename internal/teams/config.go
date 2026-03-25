package teams

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/canhta/gistclaw/internal/model"
	"go.yaml.in/yaml/v4"
)

type Config struct {
	Name       string
	FrontAgent string
	Agents     []AgentConfig
}

type AgentConfig struct {
	ID          string
	SoulFile    string
	Role        string
	ToolPosture string
	CanSpawn    []string
	CanMessage  []string
	Soul        SoulSpec
}

type SoulSpec struct {
	Role        string         `yaml:"role,omitempty"`
	ToolPosture string         `yaml:"tool_posture"`
	Extra       map[string]any `yaml:",inline"`
}

func LoadConfig(teamDir string) (Config, error) {
	if teamDir == "" {
		return Config{}, fmt.Errorf("team: team dir is required")
	}

	data, err := os.ReadFile(filepath.Join(teamDir, "team.yaml"))
	if err != nil {
		return Config{}, fmt.Errorf("team: read team.yaml: %w", err)
	}
	spec, err := LoadSpec(data)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Name:       spec.Name,
		FrontAgent: spec.FrontAgent,
		Agents:     make([]AgentConfig, 0, len(spec.Agents)),
	}
	for _, agent := range spec.Agents {
		soul, err := loadSoulSpec(filepath.Join(teamDir, agent.SoulFile))
		if err != nil {
			return Config{}, fmt.Errorf("team: agent %q: %w", agent.ID, err)
		}
		cfg.Agents = append(cfg.Agents, AgentConfig{
			ID:          agent.ID,
			SoulFile:    agent.SoulFile,
			Role:        soul.Role,
			ToolPosture: soul.ToolPosture,
			CanSpawn:    append([]string(nil), agent.CanSpawn...),
			CanMessage:  append([]string(nil), agent.CanMessage...),
			Soul:        soul,
		})
	}

	return cfg, nil
}

func WriteConfig(teamDir string, cfg Config) error {
	if teamDir == "" {
		return fmt.Errorf("team: team dir is required")
	}
	spec := Spec{
		Name:       cfg.Name,
		FrontAgent: cfg.FrontAgent,
		Agents:     make([]AgentSpec, 0, len(cfg.Agents)),
	}
	for _, agent := range cfg.Agents {
		if agent.ID == "" {
			return fmt.Errorf("team: agent id is required")
		}
		if agent.SoulFile == "" {
			return fmt.Errorf("team: agent %q is missing soul file", agent.ID)
		}
		spec.Agents = append(spec.Agents, AgentSpec{
			ID:         agent.ID,
			SoulFile:   agent.SoulFile,
			CanSpawn:   append([]string(nil), agent.CanSpawn...),
			CanMessage: append([]string(nil), agent.CanMessage...),
		})
	}
	if err := validateSpec(&spec); err != nil {
		return err
	}

	rawSpec, err := yaml.Marshal(&spec)
	if err != nil {
		return fmt.Errorf("team: marshal team spec: %w", err)
	}
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		return fmt.Errorf("team: mkdir %s: %w", teamDir, err)
	}
	if err := os.WriteFile(filepath.Join(teamDir, "team.yaml"), rawSpec, 0o644); err != nil {
		return fmt.Errorf("team: write team.yaml: %w", err)
	}

	for _, agent := range cfg.Agents {
		soul := agent.Soul
		soul.Role = agent.Role
		soul.ToolPosture = agent.ToolPosture
		if err := writeSoulSpec(filepath.Join(teamDir, agent.SoulFile), soul); err != nil {
			return fmt.Errorf("team: write soul for %q: %w", agent.ID, err)
		}
	}

	return nil
}

func (c Config) Snapshot() (model.ExecutionSnapshot, error) {
	snapshot := model.ExecutionSnapshot{
		TeamID: c.Name,
		Agents: make(map[string]model.AgentProfile, len(c.Agents)),
	}
	for _, agent := range c.Agents {
		profile, err := buildAgentProfile(AgentSpec{
			ID:         agent.ID,
			SoulFile:   agent.SoulFile,
			CanSpawn:   agent.CanSpawn,
			CanMessage: agent.CanMessage,
		}, soulSpec{ToolPosture: agent.ToolPosture})
		if err != nil {
			return model.ExecutionSnapshot{}, fmt.Errorf("team: agent %q: %w", agent.ID, err)
		}
		snapshot.Agents[agent.ID] = profile
	}
	return snapshot, nil
}

func loadSoulSpec(path string) (SoulSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SoulSpec{}, fmt.Errorf("read soul %q: %w", filepath.Base(path), err)
	}

	var soul SoulSpec
	if err := yaml.Unmarshal(data, &soul); err != nil {
		return SoulSpec{}, fmt.Errorf("parse soul yaml: %w", err)
	}
	if soul.ToolPosture == "" {
		return SoulSpec{}, fmt.Errorf("tool_posture is required")
	}
	if soul.Extra == nil {
		soul.Extra = map[string]any{}
	}
	return soul, nil
}

func writeSoulSpec(path string, soul SoulSpec) error {
	if soul.ToolPosture == "" {
		return fmt.Errorf("tool_posture is required")
	}
	raw, err := yaml.Marshal(&soul)
	if err != nil {
		return fmt.Errorf("marshal soul yaml: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
