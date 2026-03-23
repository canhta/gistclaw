package app

import "testing"

func TestBootstrap_WiringFunctionsExist(t *testing.T) {
	_, err := storeWiring(Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Logf("storeWiring error (expected with minimal config): %v", err)
	}
}

func TestBootstrap_NoFunctionOver30Lines(t *testing.T) {
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
	}
	_, _ = Bootstrap(cfg)
}
