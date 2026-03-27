package authority

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
	ApprovalMode   ApprovalMode
	HostAccessMode HostAccessMode
	Capabilities   CapabilitySet
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
