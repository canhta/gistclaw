package authority

import (
	"encoding/json"
	"testing"
)

func TestDecodeEnvelope_NormalizesDefaults(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Envelope
	}{
		{
			name: "empty defaults to prompt and standard",
			raw:  nil,
			want: Envelope{
				ApprovalMode:   ApprovalModePrompt,
				HostAccessMode: HostAccessModeStandard,
			},
		},
		{
			name: "partial payload fills missing defaults",
			raw:  []byte(`{"approval_mode":"auto_approve"}`),
			want: Envelope{
				ApprovalMode:   ApprovalModeAutoApprove,
				HostAccessMode: HostAccessModeStandard,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeEnvelope(tt.raw)
			if err != nil {
				t.Fatalf("DecodeEnvelope: %v", err)
			}
			if got.ApprovalMode != tt.want.ApprovalMode || got.HostAccessMode != tt.want.HostAccessMode {
				t.Fatalf("DecodeEnvelope() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEnvelope_MarshalUsesStableWireKeys(t *testing.T) {
	raw, err := json.Marshal(Envelope{
		ApprovalMode:   ApprovalModeAutoApprove,
		HostAccessMode: HostAccessModeElevated,
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(raw) != `{"approval_mode":"auto_approve","host_access_mode":"elevated"}` {
		t.Fatalf("marshal envelope = %s", raw)
	}
}
