package control

import "testing"

func TestRegistryParse(t *testing.T) {
	registry := NewRegistry(
		CommandSpec{Name: "start", Description: "Show help and how to use the bot"},
		CommandSpec{Name: "help", Description: "Show the available commands"},
		CommandSpec{Name: "status", Description: "Show the latest status for this chat"},
		CommandSpec{Name: "reset", Description: "Clear the current chat history and temp state"},
	)

	tests := []struct {
		name string
		text string
		want Command
		ok   bool
	}{
		{
			name: "plain text ignored",
			text: "review the repo",
			ok:   false,
		},
		{
			name: "known command",
			text: "/status",
			want: Command{Name: "status"},
			ok:   true,
		},
		{
			name: "known command with args",
			text: "/start onboarding",
			want: Command{Name: "start", Args: "onboarding"},
			ok:   true,
		},
		{
			name: "reset command",
			text: "/reset",
			want: Command{Name: "reset"},
			ok:   true,
		},
		{
			name: "bot mention suffix",
			text: "/help@gistclaw_bot",
			want: Command{Name: "help"},
			ok:   true,
		},
		{
			name: "unknown slash text",
			text: "/review the repo",
			ok:   false,
		},
		{
			name: "path-like slash text",
			text: "/Users/canh/Projects/OSS/gistclaw",
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := registry.Parse(tt.text)
			if ok != tt.ok {
				t.Fatalf("Parse(%q) ok = %v, want %v", tt.text, ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("Parse(%q) = %+v, want %+v", tt.text, got, tt.want)
			}
		})
	}
}

func TestRegistrySpecsPreserveDescriptions(t *testing.T) {
	registry := NewRegistry(
		CommandSpec{Name: "start", Description: "Show help and how to use the bot"},
		CommandSpec{Name: "help", Description: "Show the available commands"},
		CommandSpec{Name: "status", Description: "Show the latest status for this chat"},
		CommandSpec{Name: "reset", Description: "Clear the current chat history and temp state"},
	)

	specs := registry.Specs()
	if len(specs) != 4 {
		t.Fatalf("expected 4 specs, got %d", len(specs))
	}
	if specs[0].Name != "start" || specs[0].Description == "" {
		t.Fatalf("expected first spec to preserve name and description, got %+v", specs[0])
	}
	if specs[2].Name != "status" || specs[2].Description != "Show the latest status for this chat" {
		t.Fatalf("expected status description to be preserved, got %+v", specs[2])
	}
	if specs[3].Name != "reset" || specs[3].Description != "Clear the current chat history and temp state" {
		t.Fatalf("expected reset description to be preserved, got %+v", specs[3])
	}
}
