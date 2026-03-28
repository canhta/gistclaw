package tools

import (
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestPolicy_Decide_AdaptiveLayers(t *testing.T) {
	tests := []struct {
		name  string
		agent model.AgentProfile
		spec  model.ToolSpec
		want  model.DecisionMode
	}{
		{
			name: "operator denies repo write by base profile",
			agent: model.AgentProfile{
				BaseProfile:  model.BaseProfileOperator,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyConnectorCapability},
			},
			spec: model.ToolSpec{
				Name:       "write_new_file",
				Family:     model.ToolFamilyRepoWrite,
				SideEffect: effectCreate,
				Approval:   "required",
			},
			want: model.DecisionDeny,
		},
		{
			name: "allow tools override base family denial",
			agent: model.AgentProfile{
				BaseProfile:  model.BaseProfileOperator,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead},
				AllowTools:   []string{"connector_directory_list"},
			},
			spec: model.ToolSpec{
				Name:       "connector_directory_list",
				Family:     model.ToolFamilyConnectorCapability,
				SideEffect: effectRead,
				Approval:   "never",
			},
			want: model.DecisionAllow,
		},
		{
			name: "deny tools override write allowance",
			agent: model.AgentProfile{
				BaseProfile:  model.BaseProfileWrite,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyRepoWrite},
				DenyTools:    []string{"shell_exec"},
			},
			spec: model.ToolSpec{
				Name:       "shell_exec",
				Family:     model.ToolFamilyRepoWrite,
				SideEffect: effectExecWrite,
				Approval:   "maybe",
			},
			want: model.DecisionDeny,
		},
		{
			name: "connector capability allowed for operator",
			agent: model.AgentProfile{
				BaseProfile:  model.BaseProfileOperator,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyConnectorCapability},
			},
			spec: model.ToolSpec{
				Name:       "connector_status",
				Family:     model.ToolFamilyConnectorCapability,
				SideEffect: effectRead,
				Approval:   "never",
			},
			want: model.DecisionAllow,
		},
		{
			name: "write profile asks for mutating shell execution",
			agent: model.AgentProfile{
				BaseProfile:  model.BaseProfileWrite,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyRepoWrite},
			},
			spec: model.ToolSpec{
				Name:       "shell_exec",
				Family:     model.ToolFamilyRepoWrite,
				SideEffect: effectExecWrite,
				Approval:   "maybe",
			},
			want: model.DecisionAsk,
		},
		{
			name: "delegate requires declared delegation kinds",
			agent: model.AgentProfile{
				BaseProfile:  model.BaseProfileOperator,
				ToolFamilies: []model.ToolFamily{model.ToolFamilyDelegate},
			},
			spec: model.ToolSpec{
				Name:       "session_spawn",
				Family:     model.ToolFamilyDelegate,
				SideEffect: effectRead,
				Approval:   "never",
			},
			want: model.DecisionDeny,
		},
		{
			name: "raw session spawn requires explicit allow tool",
			agent: model.AgentProfile{
				BaseProfile:     model.BaseProfileOperator,
				ToolFamilies:    []model.ToolFamily{model.ToolFamilyDelegate},
				DelegationKinds: []model.DelegationKind{model.DelegationKindResearch},
			},
			spec: model.ToolSpec{
				Name:       "session_spawn",
				Family:     model.ToolFamilyDelegate,
				SideEffect: effectRead,
				Approval:   "never",
			},
			want: model.DecisionDeny,
		},
		{
			name: "delegate task allowed with declared delegation kinds",
			agent: model.AgentProfile{
				BaseProfile:     model.BaseProfileOperator,
				ToolFamilies:    []model.ToolFamily{model.ToolFamilyDelegate},
				DelegationKinds: []model.DelegationKind{model.DelegationKindResearch},
			},
			spec: model.ToolSpec{
				Name:       "delegate_task",
				Family:     model.ToolFamilyDelegate,
				SideEffect: effectRead,
				Approval:   "never",
			},
			want: model.DecisionAllow,
		},
	}

	p := &Policy{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := p.Decide(tc.agent, model.RunProfile{}, tc.spec)
			if got.Mode != tc.want {
				t.Fatalf("expected %s, got %s (%s)", tc.want, got.Mode, got.Reason)
			}
		})
	}
}
