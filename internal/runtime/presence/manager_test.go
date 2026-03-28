package presence

import (
	"context"
	"testing"
	"time"
)

func TestManager_ReusesOneControllerPerRoute(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	route := Route{
		ConversationID: "conv-1",
		ConnectorID:    "zalo_personal",
		AccountID:      "acct-1",
		ExternalID:     "user-1",
	}
	opts := Options{
		StartupDelay: 50 * time.Millisecond,
		StartFn:      func(context.Context) error { return nil },
	}

	first := mgr.Start(route, opts)
	second := mgr.Start(route, opts)

	if first != second {
		t.Fatalf("expected the same controller for duplicate route start")
	}
	if got := len(mgr.controllers); got != 1 {
		t.Fatalf("expected one controller, got %d", got)
	}

	mgr.Stop(route)
	if got := len(mgr.controllers); got != 0 {
		t.Fatalf("expected no controllers after stop, got %d", got)
	}
}
