package authority

import "testing"

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
			raw:  []byte(`{"ApprovalMode":"auto_approve"}`),
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
