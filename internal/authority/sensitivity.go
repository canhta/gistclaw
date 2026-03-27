package authority

func IsSensitiveClass(class SensitiveClass) bool {
	switch class {
	case SensitiveSSHKeys, SensitiveCloudCreds, SensitiveBrowserProfiles, SensitiveShellDotfiles, SensitiveSystemConfig:
		return true
	default:
		return false
	}
}
