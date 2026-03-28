package teams

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
	"go.yaml.in/yaml/v4"
)

type Config struct {
	Name       string
	FrontAgent string
	Agents     []AgentConfig
}

type AgentConfig struct {
	ID                          string
	SoulFile                    string
	Role                        string
	BaseProfile                 model.BaseProfile
	ToolFamilies                []model.ToolFamily
	AllowTools                  []string
	DenyTools                   []string
	DelegationKinds             []model.DelegationKind
	CanMessage                  []string
	SpecialistSummaryVisibility model.SpecialistSummaryVisibility
	Soul                        SoulSpec
}

type SoulSpec struct {
	Role  string         `yaml:"role,omitempty"`
	Extra map[string]any `yaml:",inline"`
}

type editableFile struct {
	Name       string              `yaml:"name"`
	FrontAgent string              `yaml:"front_agent"`
	Agents     []editableFileAgent `yaml:"agents"`
}

type editableFileAgent struct {
	ID                          string                            `yaml:"id"`
	SoulFile                    string                            `yaml:"soul_file,omitempty"`
	Role                        string                            `yaml:"role,omitempty"`
	BaseProfile                 model.BaseProfile                 `yaml:"base_profile"`
	ToolFamilies                []model.ToolFamily                `yaml:"tool_families"`
	AllowTools                  []string                          `yaml:"allow_tools,omitempty"`
	DenyTools                   []string                          `yaml:"deny_tools,omitempty"`
	DelegationKinds             []model.DelegationKind            `yaml:"delegation_kinds,omitempty"`
	CanMessage                  []string                          `yaml:"can_message"`
	SpecialistSummaryVisibility model.SpecialistSummaryVisibility `yaml:"specialist_summary_visibility,omitempty"`
	SoulExtra                   map[string]any                    `yaml:"soul_extra,omitempty"`
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
			ID:                          agent.ID,
			SoulFile:                    agent.SoulFile,
			Role:                        soul.Role,
			BaseProfile:                 agent.BaseProfile,
			ToolFamilies:                append([]model.ToolFamily(nil), agent.ToolFamilies...),
			AllowTools:                  append([]string(nil), agent.AllowTools...),
			DenyTools:                   append([]string(nil), agent.DenyTools...),
			DelegationKinds:             append([]model.DelegationKind(nil), agent.DelegationKinds...),
			CanMessage:                  append([]string(nil), agent.CanMessage...),
			SpecialistSummaryVisibility: agent.SpecialistSummaryVisibility,
			Soul:                        soul,
		})
	}

	return cfg, nil
}

func WriteConfig(teamDir string, cfg Config) error {
	if teamDir == "" {
		return fmt.Errorf("team: team dir is required")
	}
	oldSoulFiles := referencedSoulFiles(teamDir)
	for i := range cfg.Agents {
		if cfg.Agents[i].SoulFile == "" {
			cfg.Agents[i].SoulFile = SuggestedSoulFile(cfg.Agents[i].ID)
		}
		if cfg.Agents[i].Soul.Extra == nil {
			cfg.Agents[i].Soul.Extra = map[string]any{}
		}
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
		spec.Agents = append(spec.Agents, AgentSpec{
			ID:                          agent.ID,
			SoulFile:                    agent.SoulFile,
			BaseProfile:                 agent.BaseProfile,
			ToolFamilies:                append([]model.ToolFamily(nil), agent.ToolFamilies...),
			AllowTools:                  append([]string(nil), agent.AllowTools...),
			DenyTools:                   append([]string(nil), agent.DenyTools...),
			DelegationKinds:             append([]model.DelegationKind(nil), agent.DelegationKinds...),
			CanMessage:                  append([]string(nil), agent.CanMessage...),
			SpecialistSummaryVisibility: agent.SpecialistSummaryVisibility,
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
		if err := writeSoulSpec(filepath.Join(teamDir, agent.SoulFile), soul); err != nil {
			return fmt.Errorf("team: write soul for %q: %w", agent.ID, err)
		}
	}
	for _, soulFile := range oldSoulFiles {
		if !currentSoulFile(spec.Agents, soulFile) {
			if err := os.Remove(filepath.Join(teamDir, soulFile)); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("team: remove orphan soul %q: %w", soulFile, err)
			}
		}
	}

	return nil
}

func ExportEditableYAML(cfg Config) ([]byte, error) {
	file := editableFile{
		Name:       cfg.Name,
		FrontAgent: cfg.FrontAgent,
		Agents:     make([]editableFileAgent, 0, len(cfg.Agents)),
	}
	for _, agent := range cfg.Agents {
		fileAgent := editableFileAgent{
			ID:                          agent.ID,
			SoulFile:                    agent.SoulFile,
			Role:                        agent.Role,
			BaseProfile:                 agent.BaseProfile,
			ToolFamilies:                append([]model.ToolFamily(nil), agent.ToolFamilies...),
			AllowTools:                  append([]string(nil), agent.AllowTools...),
			DenyTools:                   append([]string(nil), agent.DenyTools...),
			DelegationKinds:             append([]model.DelegationKind(nil), agent.DelegationKinds...),
			CanMessage:                  append([]string(nil), agent.CanMessage...),
			SpecialistSummaryVisibility: agent.SpecialistSummaryVisibility,
		}
		if len(agent.Soul.Extra) > 0 {
			fileAgent.SoulExtra = cloneSoulExtra(agent.Soul.Extra)
		}
		file.Agents = append(file.Agents, fileAgent)
	}
	raw, err := yaml.Marshal(&file)
	if err != nil {
		return nil, fmt.Errorf("team: marshal editable yaml: %w", err)
	}
	return raw, nil
}

func LoadEditableYAML(data []byte) (Config, error) {
	var file editableFile
	if err := decodeKnownFieldsYAML(data, &file); err != nil {
		return Config{}, fmt.Errorf("team: parse editable yaml: %w", err)
	}

	cfg := Config{
		Name:       file.Name,
		FrontAgent: file.FrontAgent,
		Agents:     make([]AgentConfig, 0, len(file.Agents)),
	}
	for _, agent := range file.Agents {
		if agent.ID == "" {
			return Config{}, fmt.Errorf("team: agent id is required")
		}
		soulFile := agent.SoulFile
		if soulFile == "" {
			soulFile = SuggestedSoulFile(agent.ID)
		}
		if agent.BaseProfile == "" {
			return Config{}, fmt.Errorf("team: agent %q: base_profile is required", agent.ID)
		}
		soul := SoulSpec{
			Role:  agent.Role,
			Extra: cloneSoulExtra(agent.SoulExtra),
		}
		cfg.Agents = append(cfg.Agents, AgentConfig{
			ID:                          agent.ID,
			SoulFile:                    soulFile,
			Role:                        agent.Role,
			BaseProfile:                 agent.BaseProfile,
			ToolFamilies:                append([]model.ToolFamily(nil), agent.ToolFamilies...),
			AllowTools:                  append([]string(nil), agent.AllowTools...),
			DenyTools:                   append([]string(nil), agent.DenyTools...),
			DelegationKinds:             append([]model.DelegationKind(nil), agent.DelegationKinds...),
			CanMessage:                  append([]string(nil), agent.CanMessage...),
			SpecialistSummaryVisibility: agent.SpecialistSummaryVisibility,
			Soul:                        soul,
		})
	}

	spec := Spec{
		Name:       cfg.Name,
		FrontAgent: cfg.FrontAgent,
		Agents:     make([]AgentSpec, 0, len(cfg.Agents)),
	}
	for _, agent := range cfg.Agents {
		spec.Agents = append(spec.Agents, AgentSpec{
			ID:                          agent.ID,
			SoulFile:                    agent.SoulFile,
			BaseProfile:                 agent.BaseProfile,
			ToolFamilies:                append([]model.ToolFamily(nil), agent.ToolFamilies...),
			AllowTools:                  append([]string(nil), agent.AllowTools...),
			DenyTools:                   append([]string(nil), agent.DenyTools...),
			DelegationKinds:             append([]model.DelegationKind(nil), agent.DelegationKinds...),
			CanMessage:                  append([]string(nil), agent.CanMessage...),
			SpecialistSummaryVisibility: agent.SpecialistSummaryVisibility,
		})
	}
	if err := validateSpec(&spec); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Snapshot() (model.ExecutionSnapshot, error) {
	snapshot := model.ExecutionSnapshot{
		TeamID: c.Name,
		Agents: make(map[string]model.AgentProfile, len(c.Agents)),
	}
	for _, agent := range c.Agents {
		profile, err := buildAgentProfile(agent)
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
	if err := rejectUnsupportedSoulFields(data); err != nil {
		return SoulSpec{}, fmt.Errorf("parse soul yaml: %w", err)
	}
	var soul SoulSpec
	if err := yaml.Unmarshal(data, &soul); err != nil {
		return SoulSpec{}, fmt.Errorf("parse soul yaml: %w", err)
	}
	if soul.Extra == nil {
		soul.Extra = map[string]any{}
	}
	return soul, nil
}

func writeSoulSpec(path string, soul SoulSpec) error {
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

func SuggestedSoulFile(agentID string) string {
	normalized := strings.ToLower(strings.TrimSpace(agentID))
	if normalized == "" {
		normalized = "agent"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range normalized {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	base := strings.Trim(b.String(), "-_")
	if base == "" {
		base = "agent"
	}
	return base + ".soul.yaml"
}

func referencedSoulFiles(teamDir string) []string {
	data, err := os.ReadFile(filepath.Join(teamDir, "team.yaml"))
	if err != nil {
		return nil
	}
	spec, err := LoadSpec(data)
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(spec.Agents))
	for _, agent := range spec.Agents {
		if agent.SoulFile == "" {
			continue
		}
		files = append(files, agent.SoulFile)
	}
	return files
}

func currentSoulFile(agents []AgentSpec, soulFile string) bool {
	for _, agent := range agents {
		if agent.SoulFile == soulFile {
			return true
		}
	}
	return false
}

func cloneSoulExtra(extra map[string]any) map[string]any {
	if len(extra) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(extra))
	for key, value := range extra {
		cloned[key] = value
	}
	return cloned
}

func decodeKnownFieldsYAML(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	return dec.Decode(out)
}

func rejectUnsupportedSoulFields(data []byte) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "tool_posture" {
			return fmt.Errorf("field %q is not supported", "tool_posture")
		}
	}
	return nil
}
