package claudecode

// Config holds all settings specific to the ClaudeCode service.
type Config struct {
	// Dir is the working directory for claude -p invocations.
	Dir string
	// HookServerAddr is the address the hook HTTP server listens on.
	// Default: "127.0.0.1:8765"
	HookServerAddr string
	// SettingsPath is the path to .claude/settings.local.json within Dir.
	// If empty, defaults to filepath.Join(Dir, ".claude/settings.local.json").
	SettingsPath string
}
