package memory

import "testing"

func TestAuthorizeEscalation(t *testing.T) {
	tests := []struct {
		name        string
		from, to    string
		wantErr     bool
		errContains string
	}{
		{name: "local to team is permitted", from: "local", to: "team", wantErr: false},
		{name: "team to local is not an escalation", from: "team", to: "local", wantErr: true, errContains: "not an escalation"},
		{name: "same scope is not an escalation", from: "local", to: "local", wantErr: true, errContains: "not an escalation"},
		{name: "unknown current scope", from: "global", to: "team", wantErr: true, errContains: "unknown current scope"},
		{name: "unknown target scope", from: "local", to: "galaxy", wantErr: true, errContains: "unknown target scope"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := AuthorizeEscalation(tc.from, tc.to)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if tc.errContains != "" && err != nil {
				if msg := err.Error(); len(msg) == 0 || !containsStr(msg, tc.errContains) {
					t.Errorf("error %q does not contain %q", msg, tc.errContains)
				}
			}
		})
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
