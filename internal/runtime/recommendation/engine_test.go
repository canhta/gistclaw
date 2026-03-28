package recommendation

import (
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestEngine_Recommend(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		input     Input
		want      Mode
		wantHas   []string
		wantTools []string
	}{
		{
			name: "direct for unread inbox lookup",
			input: Input{
				Objective: "kiểm tra xem có tin nhắn nào chưa đọc trên Zalo không",
				Agent: model.AgentProfile{
					AgentID:         "assistant",
					BaseProfile:     model.BaseProfileOperator,
					ToolFamilies:    []model.ToolFamily{model.ToolFamilyConnectorCapability, model.ToolFamilyDelegate},
					DelegationKinds: []model.DelegationKind{model.DelegationKindResearch},
				},
				VisibleTools: []model.ToolSpec{
					{Name: "connector_inbox_list", Family: model.ToolFamilyConnectorCapability, Intents: []model.ToolIntent{model.ToolIntentInboxList}},
					{Name: "delegate_task", Family: model.ToolFamilyDelegate, Intents: []model.ToolIntent{model.ToolIntentDelegate}},
				},
				Specialists: map[string]model.AgentProfile{
					"researcher": {AgentID: "researcher", BaseProfile: model.BaseProfileResearch},
				},
			},
			want:      ModeDirect,
			wantHas:   []string{"direct"},
			wantTools: []string{"connector_inbox_list"},
		},
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
				VisibleTools: []model.ToolSpec{
					{Name: "connector_directory_list", Family: model.ToolFamilyConnectorCapability, Intents: []model.ToolIntent{model.ToolIntentDirectoryList}},
					{Name: "connector_target_resolve", Family: model.ToolFamilyConnectorCapability, Intents: []model.ToolIntent{model.ToolIntentTargetResolve}},
					{Name: "connector_send", Family: model.ToolFamilyConnectorCapability, Intents: []model.ToolIntent{model.ToolIntentMessageSend}},
					{Name: "delegate_task", Family: model.ToolFamilyDelegate, Intents: []model.ToolIntent{model.ToolIntentDelegate}},
					{Name: "web_fetch", Family: model.ToolFamilyWebRead, Intents: []model.ToolIntent{model.ToolIntentResearchRead}},
				},
				Specialists: map[string]model.AgentProfile{
					"researcher": {AgentID: "researcher", BaseProfile: model.BaseProfileResearch},
					"patcher":    {AgentID: "patcher", BaseProfile: model.BaseProfileWrite},
				},
			},
			want:      ModeDirect,
			wantHas:   []string{"bounded", "connector"},
			wantTools: []string{"connector_directory_list", "connector_send"},
		},
		{
			name: "direct ranking follows tool intents instead of hardcoded tool names",
			input: Input{
				Objective: "Find Anh and send hello.",
				Agent: model.AgentProfile{
					AgentID:         "assistant",
					BaseProfile:     model.BaseProfileOperator,
					ToolFamilies:    []model.ToolFamily{model.ToolFamilyConnectorCapability, model.ToolFamilyDelegate},
					DelegationKinds: []model.DelegationKind{model.DelegationKindResearch},
				},
				VisibleTools: []model.ToolSpec{
					{Name: "people_lookup", Family: model.ToolFamilyConnectorCapability, Intents: []model.ToolIntent{model.ToolIntentTargetResolve, model.ToolIntentDirectoryList}},
					{Name: "direct_message_deliver", Family: model.ToolFamilyConnectorCapability, Intents: []model.ToolIntent{model.ToolIntentMessageSend}},
					{Name: "delegate_task", Family: model.ToolFamilyDelegate, Intents: []model.ToolIntent{model.ToolIntentDelegate}},
				},
				Specialists: map[string]model.AgentProfile{
					"researcher": {AgentID: "researcher", BaseProfile: model.BaseProfileResearch},
				},
			},
			want:      ModeDirect,
			wantHas:   []string{"bounded", "direct"},
			wantTools: []string{"people_lookup", "direct_message_deliver"},
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
			for _, want := range tc.wantTools {
				if !containsString(got.RankedToolNames, want) {
					t.Fatalf("expected ranked tool names %+v to contain %q", got.RankedToolNames, want)
				}
			}
		})
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
