package control

import "testing"

func TestRegistryParse(t *testing.T) {
	registry := NewRegistry("start", "help", "status")

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
		{
			name: "legacy run alias not recognized",
			text: "/run review the repo",
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
