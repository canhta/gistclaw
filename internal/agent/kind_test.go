package agent_test

import (
	"testing"

	"github.com/canhta/gistclaw/internal/agent"
)

func TestKindString(t *testing.T) {
	cases := []struct {
		kind agent.Kind
		want string
	}{
		{agent.KindOpenCode, "opencode"},
		{agent.KindClaudeCode, "claudecode"},
		{agent.KindChat, "chat"},
		{agent.KindGateway, "gateway"},
		{agent.KindUnknown, "unknown(-1)"},
	}
	for _, c := range cases {
		if got := c.kind.String(); got != c.want {
			t.Errorf("Kind(%d).String() = %q, want %q", c.kind, got, c.want)
		}
	}
}

func TestKindFromString(t *testing.T) {
	cases := []struct {
		s    string
		want agent.Kind
		ok   bool
	}{
		{"opencode", agent.KindOpenCode, true},
		{"claudecode", agent.KindClaudeCode, true},
		{"chat", agent.KindChat, true},
		{"gateway", agent.KindGateway, true},
		{"unknown", agent.KindUnknown, false},
		{"", agent.KindUnknown, false},
	}
	for _, c := range cases {
		got, err := agent.KindFromString(c.s)
		if c.ok && err != nil {
			t.Errorf("KindFromString(%q) unexpected error: %v", c.s, err)
		}
		if !c.ok && err == nil {
			t.Errorf("KindFromString(%q) expected error, got nil", c.s)
		}
		if got != c.want {
			t.Errorf("KindFromString(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestKindGateway_RoundTrip(t *testing.T) {
	if got := agent.KindGateway.String(); got != "gateway" {
		t.Errorf("KindGateway.String() = %q, want %q", got, "gateway")
	}
	k, err := agent.KindFromString("gateway")
	if err != nil {
		t.Fatalf("KindFromString(\"gateway\") error: %v", err)
	}
	if k != agent.KindGateway {
		t.Errorf("KindFromString(\"gateway\") = %v, want KindGateway", k)
	}
}
