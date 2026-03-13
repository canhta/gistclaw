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
	// Dir is the working directory passed to opencode serve via cmd.Dir.
	Dir string
	// StartupTimeout is how long Run waits for the health endpoint after spawning
	// opencode serve. Use DefaultStartupTimeout when no override is needed.
	StartupTimeout int // seconds; kept as int to avoid importing time in config types
	// Username is the HTTP Basic Auth username for opencode serve
	// (OPENCODE_SERVER_USERNAME). Empty means no auth.
	Username string
	// Password is the HTTP Basic Auth password for opencode serve
	// (OPENCODE_SERVER_PASSWORD). Empty means no auth.
	Password string
}
