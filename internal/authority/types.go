package authority

import (
	"bytes"
	"encoding/json"
)

type ApprovalMode string

const (
	ApprovalModePrompt      ApprovalMode = "prompt"
	ApprovalModeAutoApprove ApprovalMode = "auto_approve"
)

type HostAccessMode string

const (
	HostAccessModeStandard HostAccessMode = "standard"
	HostAccessModeElevated HostAccessMode = "elevated"
)

type Capability string

const (
	CapabilityFSRead  Capability = "fs.read"
	CapabilityFSWrite Capability = "fs.write"
	CapabilityExec    Capability = "exec"
	CapabilityGit     Capability = "git"
	CapabilityNetwork Capability = "network"
)

type CapabilitySet map[Capability]bool

type Envelope struct {
	ApprovalMode   ApprovalMode   `json:"approval_mode,omitempty"`
	HostAccessMode HostAccessMode `json:"host_access_mode,omitempty"`
	Capabilities   CapabilitySet  `json:"capabilities,omitempty"`
}

func NormalizeEnvelope(env Envelope) Envelope {
	if env.ApprovalMode == "" {
		env.ApprovalMode = ApprovalModePrompt
	}
	if env.HostAccessMode == "" {
		env.HostAccessMode = HostAccessModeStandard
	}
	return env
}

func DecodeEnvelope(raw []byte) (Envelope, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return NormalizeEnvelope(Envelope{}), nil
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Envelope{}, err
	}
	return NormalizeEnvelope(env), nil
}

type SensitiveClass string

const (
	SensitiveSSHKeys         SensitiveClass = "ssh_keys"
	SensitiveCloudCreds      SensitiveClass = "cloud_credentials"
	SensitiveBrowserProfiles SensitiveClass = "browser_profiles"
	SensitiveShellDotfiles   SensitiveClass = "shell_dotfiles"
	SensitiveSystemConfig    SensitiveClass = "system_config"
)

type Intent struct {
	ReadRoots  []string
	WriteRoots []string
	Sensitive  []SensitiveClass
	Mutating   bool
	Network    bool
}
