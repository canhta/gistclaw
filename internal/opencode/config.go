// internal/opencode/config.go
package opencode

// Config holds all settings specific to the OpenCode service.
// Fields are populated by internal/config and injected at construction time.
type Config struct {
	// Port is the TCP port opencode serve will bind to (default 8766).
	Port int
	// Dir is the working directory passed to opencode serve --dir.
	Dir string
	// StartupTimeout is how long Run waits for the health endpoint after spawning
	// opencode serve (default 3s — callers should inject config.Tuning values).
	StartupTimeout int // seconds; kept as int to avoid importing time in config types
}
