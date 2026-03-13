// internal/opencode/config.go
package opencode

// DefaultStartupTimeout is how long Run waits for the health endpoint after
// spawning opencode serve. 30 seconds is a realistic upper bound for a cold
// start; callers may override via Config.StartupTimeout.
const DefaultStartupTimeout = 30 // seconds

// Config holds all settings specific to the OpenCode service.
// Fields are populated by internal/config and injected at construction time.
type Config struct {
	// Port is the TCP port opencode serve will bind to (default 8766).
	Port int
	// Dir is the working directory passed to opencode serve --dir.
	Dir string
	// StartupTimeout is how long Run waits for the health endpoint after spawning
	// opencode serve. Use DefaultStartupTimeout when no override is needed.
	StartupTimeout int // seconds; kept as int to avoid importing time in config types
}
