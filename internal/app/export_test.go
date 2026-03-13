// internal/app/export_test.go
// Test-only exports. This file is compiled only during `go test`.
package app

import (
	"context"

	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/scheduler"
)

// TestOperatorChatID is the operator chat ID used by NewTestJobTarget in tests.
const TestOperatorChatID int64 = 1234567890

// NewTestJobTarget constructs an appJobTarget with a gateway runner for testing.
// gwRun may be nil to test the nil-runner error path.
// This is in package app (not app_test) so it can access unexported appJobTarget.
func NewTestJobTarget(gwRun func(ctx context.Context, chatID int64, prompt string) error) scheduler.JobTarget {
	t := &appJobTarget{
		cfg: config.Config{AllowedUserIDs: []int64{TestOperatorChatID}},
	}
	t.gwRun = gwRun
	return t
}
