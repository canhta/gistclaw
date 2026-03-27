package web

import "testing"

func TestServer_HumanizesZaloPersonalTriggerLabel(t *testing.T) {
	t.Parallel()

	if got := humanizeTriggerLabel("zalo_personal"); got != "Zalo Personal" {
		t.Fatalf("expected Zalo Personal, got %q", got)
	}
}
