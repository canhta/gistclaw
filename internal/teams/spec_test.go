package teams

import (
	"strings"
	"testing"
)

func TestLoadSpec_RequiresFrontAgent(t *testing.T) {
	_, err := LoadSpec([]byte("name: default\nagents: []\n"))
	if err == nil {
		t.Fatal("expected front_agent validation error, got nil")
	}
	if !strings.Contains(err.Error(), "front_agent") {
		t.Fatalf("expected error to mention front_agent, got %v", err)
	}
}

func TestLoadSpec_RejectsLegacySpawnField(t *testing.T) {
	data := []byte(`
name: default
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    base_profile: operator
    tool_families: [repo_read, delegate]
    can_message: []
    can_spawn: ["ghost"]
`)

	_, err := LoadSpec(data)
	if err == nil {
		t.Fatal("expected legacy can_spawn error, got nil")
	}
	if !strings.Contains(err.Error(), "can_spawn") {
		t.Fatalf("expected error to mention can_spawn, got %v", err)
	}
}

func TestLoadSpec_RejectsUnknownMessageTarget(t *testing.T) {
	data := []byte(`
name: default
front_agent: assistant
agents:
  - id: assistant
    soul_file: assistant.soul.yaml
    base_profile: operator
    tool_families: [repo_read]
    can_message: [ghost]
`)

	_, err := LoadSpec(data)
	if err == nil {
		t.Fatal("expected unknown message target error, got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("expected error to mention ghost, got %v", err)
	}
}
