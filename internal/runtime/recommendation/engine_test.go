package recommendation

import (
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestEngine_Recommend(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   Input
		want    Mode
		wantHas []string
	}{
		{
			name: "direct for bounded connector action",
			input: Input{
				Objective: "List my Zalo contacts and send hello to Anh on Zalo.",
				Agent: model.AgentProfile{
					AgentID:         "assistant",
					BaseProfile:     model.BaseProfileOperator,
					ToolFamilies:    []model.ToolFamily{model.ToolFamilyConnectorCapability, model.ToolFamilyDelegate},
					DelegationKinds: []model.DelegationKind{model.DelegationKindResearch, model.DelegationKindWrite},
				},
				Specialists: map[string]model.AgentProfile{
					"researcher": {AgentID: "researcher", BaseProfile: model.BaseProfileResearch},
					"patcher":    {AgentID: "patcher", BaseProfile: model.BaseProfileWrite},
				},
			},
			want:    ModeDirect,
			wantHas: []string{"bounded", "connector"},
		},
		{
			name: "delegate for research-heavy work",
			input: Input{
				Objective: "Research the latest Zalo personal API limitations and summarize the findings.",
				Agent: model.AgentProfile{
					AgentID:         "assistant",
					BaseProfile:     model.BaseProfileOperator,
					ToolFamilies:    []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyDelegate},
					DelegationKinds: []model.DelegationKind{model.DelegationKindResearch},
				},
				Specialists: map[string]model.AgentProfile{
					"researcher": {AgentID: "researcher", BaseProfile: model.BaseProfileResearch},
				},
			},
			want:    ModeDelegate,
			wantHas: []string{"research", "specialist"},
		},
		{
			name: "parallelize for independent research and verification",
			input: Input{
				Objective: "Research the latest Telegram restrictions and verify our docs still match the current limits.",
				Agent: model.AgentProfile{
					AgentID:         "assistant",
					BaseProfile:     model.BaseProfileOperator,
					ToolFamilies:    []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyDelegate},
					DelegationKinds: []model.DelegationKind{model.DelegationKindResearch, model.DelegationKindVerify},
				},
				Specialists: map[string]model.AgentProfile{
					"researcher": {AgentID: "researcher", BaseProfile: model.BaseProfileResearch},
					"verifier":   {AgentID: "verifier", BaseProfile: model.BaseProfileVerify},
				},
			},
			want:    ModeParallelize,
			wantHas: []string{"independent", "parallel"},
		},
	}

	engine := Engine{}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := engine.Recommend(tc.input)
			if got.Mode != tc.want {
				t.Fatalf("expected mode %q, got %q", tc.want, got.Mode)
			}
			for _, want := range tc.wantHas {
				if !containsFold(got.Rationale, want) {
					t.Fatalf("expected rationale %q to contain %q", got.Rationale, want)
				}
			}
			if got.Confidence <= 0 {
				t.Fatalf("expected confidence to be set, got %f", got.Confidence)
			}
		})
	}
}

func containsFold(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && containsFoldSlow(haystack, needle))
}

func containsFoldSlow(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if equalFoldASCII(haystack[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		aa := a[i]
		bb := b[i]
		if aa >= 'A' && aa <= 'Z' {
			aa += 'a' - 'A'
		}
		if bb >= 'A' && bb <= 'Z' {
			bb += 'a' - 'A'
		}
		if aa != bb {
			return false
		}
	}
	return true
}
