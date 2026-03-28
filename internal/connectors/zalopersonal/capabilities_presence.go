package zalopersonal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/runtime/capabilities"
)

const (
	presenceStartupDelay      = 800 * time.Millisecond
	presenceKeepaliveInterval = 4 * time.Second
	presenceMaxDuration       = 60 * time.Second
	presenceMaxFailures       = 2
)

func (c *Connector) CapabilityPresencePolicy(context.Context) capabilities.PresencePolicy {
	return capabilities.PresencePolicy{
		StartupDelay:           presenceStartupDelay,
		KeepaliveInterval:      presenceKeepaliveInterval,
		MaxDuration:            presenceMaxDuration,
		MaxConsecutiveFailures: presenceMaxFailures,
		SupportsStop:           false,
	}
}

func (c *Connector) CapabilityEmitPresence(ctx context.Context, req capabilities.PresenceEmitRequest) error {
	threadID := strings.TrimSpace(req.ThreadID)
	if threadID == "" {
		return fmt.Errorf("zalo personal capabilities: thread_id is required")
	}
	if req.Mode != "" && req.Mode != capabilities.PresenceModeTyping {
		return fmt.Errorf("zalo personal capabilities: unsupported presence mode %q", req.Mode)
	}
	if c.emitPresence == nil {
		return fmt.Errorf("zalo personal capabilities: presence is not configured")
	}

	creds, err := c.loadCapabilityCredentials(ctx)
	if err != nil {
		return err
	}

	threadType := c.threadTypeForChat(threadID)
	if strings.TrimSpace(req.ThreadType) != "" {
		threadType, err = capabilityThreadType(req.ThreadType)
		if err != nil {
			return err
		}
	}
	return c.emitPresence(ctx, creds, threadID, threadType)
}
