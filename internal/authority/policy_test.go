package authority

import (
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestDecide_ModeMatrix(t *testing.T) {
	tests := []struct {
		name   string
		env    Envelope
		intent Intent
		want   model.DecisionMode
	}{
		{
			name: "standard mode denies sensitive access",
			env: Envelope{
				ApprovalMode:   ApprovalModePrompt,
				HostAccessMode: HostAccessModeStandard,
			},
			intent: Intent{Sensitive: []SensitiveClass{SensitiveSSHKeys}},
			want:   model.DecisionDeny,
		},
		{
			name: "auto approve does not bypass standard denials",
			env: Envelope{
				ApprovalMode:   ApprovalModeAutoApprove,
				HostAccessMode: HostAccessModeStandard,
			},
			intent: Intent{Sensitive: []SensitiveClass{SensitiveSSHKeys}},
			want:   model.DecisionDeny,
		},
		{
			name: "prompt mode asks on mutating host access",
			env: Envelope{
				ApprovalMode:   ApprovalModePrompt,
				HostAccessMode: HostAccessModeStandard,
			},
			intent: Intent{
				WriteRoots: []string{"/Users/test/Desktop"},
				Mutating:   true,
			},
			want: model.DecisionAsk,
		},
		{
			name: "auto approve allows mutating non-sensitive access",
			env: Envelope{
				ApprovalMode:   ApprovalModeAutoApprove,
				HostAccessMode: HostAccessModeStandard,
			},
			intent: Intent{
				WriteRoots: []string{"/Users/test/Desktop"},
				Mutating:   true,
			},
			want: model.DecisionAllow,
		},
		{
			name: "elevated mode widens access without changing prompt behavior",
			env: Envelope{
				ApprovalMode:   ApprovalModePrompt,
				HostAccessMode: HostAccessModeElevated,
			},
			intent: Intent{
				Sensitive: []SensitiveClass{SensitiveSSHKeys},
				WriteRoots: []string{
					"/Users/test/.ssh",
				},
				Mutating: true,
			},
			want: model.DecisionAsk,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Decide(tt.env, tt.intent)
			if got.Mode != tt.want {
				t.Fatalf("Decide().Mode = %q, want %q", got.Mode, tt.want)
			}
		})
	}
}
